// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/assets/tofubin"
	"github.com/exasol/exasol-personal/internal/util"
	"gopkg.in/yaml.v3"
)

const lockFileName = ".terraform.lock.hcl"

var defaultPlatforms = []string{
	"linux_amd64",
	"linux_arm64",
	"windows_amd64",
	"darwin_amd64",
	"darwin_arm64",
}

type stringListFlag []string

func (s *stringListFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringListFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	*s = append(*s, value)
	return nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	assetsInfraDir := flag.String("infra-assets-dir", "./assets/infrastructure", "Path to the infrastructure assets root directory")
	var onlyPresets stringListFlag
	flag.Var(&onlyPresets, "preset", "Only update the given infrastructure preset directory name (repeatable)")
	var platforms stringListFlag
	flag.Var(&platforms, "platform", "Platform to lock for, e.g. windows_amd64 (repeatable)")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	if err := ensureEmbeddedTofuIsNotPlaceholder(); err != nil {
		log.Fatal(err)
	}

	platformList := []string(platforms)
	if len(platformList) == 0 {
		platformList = append([]string{}, defaultPlatforms...)
	}
	platformList = uniqueSorted(platformList)

	presetDirs, err := discoverInfrastructurePresetDirs(*assetsInfraDir, []string(onlyPresets))
	if err != nil {
		log.Fatal(err)
	}
	if len(presetDirs) == 0 {
		log.Fatalf("no infrastructure presets found under %q", *assetsInfraDir)
	}

	for _, presetDir := range presetDirs {
		if err := updateLockfileForPreset(ctx, presetDir, platformList, *verbose); err != nil {
			log.Fatal(err)
		}
	}
}

func ensureEmbeddedTofuIsNotPlaceholder() error {
	// In dev workflows the tofu binaries are downloaded into assets/tofubin/generated.
	// In some situations we only have placeholder files (for go:embed), which would
	// produce a non-functional tofu binary.
	if len(tofubin.TofuBinary) == 0 {
		return errors.New("embedded tofu binary is empty; run `task generate` to download it")
	}
	if len(tofubin.TofuBinary) < 256 && bytes.Contains(bytes.ToLower(tofubin.TofuBinary), []byte("placeholder")) {
		return errors.New("embedded tofu binary appears to be a placeholder; run `task generate` (or tofu download task) to fetch OpenTofu")
	}
	return nil
}

func discoverInfrastructurePresetDirs(infraAssetsDir string, onlyPresets []string) ([]string, error) {
	infraAssetsDir = filepath.Clean(infraAssetsDir)
	entries, err := os.ReadDir(infraAssetsDir)
	if err != nil {
		return nil, fmt.Errorf("read infra assets dir %q: %w", infraAssetsDir, err)
	}

	allowed := map[string]bool{}
	if len(onlyPresets) > 0 {
		for _, p := range onlyPresets {
			allowed[p] = true
		}
	}

	var presetDirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if len(allowed) > 0 && !allowed[name] {
			continue
		}
		candidate := filepath.Join(infraAssetsDir, name)
		if _, err := os.Stat(filepath.Join(candidate, "infrastructure.yaml")); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("stat infrastructure.yaml for %q: %w", candidate, err)
		}
		presetDirs = append(presetDirs, candidate)
	}

	sort.Strings(presetDirs)
	return presetDirs, nil
}

func updateLockfileForPreset(ctx context.Context, presetDir string, platforms []string, verbose bool) error {
	presetDir = filepath.Clean(presetDir)
	requires, reason, err := presetRequiresTofuLockfile(presetDir)
	if err != nil {
		return err
	}
	if !requires {
		if verbose {
			log.Printf("skipping %s (%s)", presetDir, reason)
		}
		return nil
	}

	if verbose {
		log.Printf("updating lockfile for preset %s", presetDir)
	}

	tmpRoot, err := os.MkdirTemp("", "exasol-tofu-lock-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpRoot) // best-effort cleanup

	tmpModuleDir := filepath.Join(tmpRoot, "module")
	if err := util.CopyDir(presetDir, tmpModuleDir); err != nil {
		return fmt.Errorf("copy preset dir to temp: %w", err)
	}

	tofuPath := filepath.Join(tmpModuleDir, tofubin.TofuBinaryName)
	if err := os.WriteFile(tofuPath, tofubin.TofuBinary, 0o744); err != nil { // nolint: gosec
		return fmt.Errorf("write tofu binary: %w", err)
	}

	args := []string{"providers", "lock"}
	for _, p := range platforms {
		args = append(args, "-platform="+p)
	}

	cmd := exec.CommandContext(ctx, tofuPath, args...)
	cmd.Dir = tmpModuleDir
	cmd.Env = append(os.Environ(),
		"TF_IN_AUTOMATION=1",
		"TF_INPUT=0",
	)
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tofu providers lock failed for %q: %w", presetDir, err)
	}

	srcLock := filepath.Join(tmpModuleDir, lockFileName)
	data, err := os.ReadFile(srcLock)
	if err != nil {
		return fmt.Errorf("read generated lockfile %q: %w", srcLock, err)
	}

	dstLock := filepath.Join(presetDir, lockFileName)
	if err := os.WriteFile(dstLock, data, 0o644); err != nil { // nolint: gosec
		return fmt.Errorf("write lockfile %q: %w", dstLock, err)
	}

	log.Printf("updated %s", dstLock)
	return nil
}

func presetRequiresTofuLockfile(presetDir string) (bool, string, error) {
	// Condition 1: infrastructure.yaml declares tofu configuration.
	manifestPath := filepath.Join(presetDir, "infrastructure.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, "missing infrastructure.yaml", nil
		}
		return false, "", fmt.Errorf("read %q: %w", manifestPath, err)
	}
	var m struct {
		Tofu *yaml.Node `yaml:"tofu"`
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return false, "", fmt.Errorf("parse %q: %w", manifestPath, err)
	}
	if m.Tofu == nil {
		return false, "no tofu section in infrastructure.yaml", nil
	}

	// Condition 2: preset contains at least one .tf file (otherwise running `tofu providers lock` is pointless).
	hasTf, err := hasTerraformFiles(presetDir)
	if err != nil {
		return false, "", err
	}
	if !hasTf {
		return false, "no .tf files", nil
	}

	return true, "tofu preset", nil
}

func hasTerraformFiles(root string) (bool, error) {
	var found bool
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			// Skip common junk dirs if they exist in a copied preset or developer workspace.
			switch name {
			case ".terraform", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, ".tf") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("scan for .tf files under %q: %w", root, err)
	}
	return found, nil
}

func uniqueSorted(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
