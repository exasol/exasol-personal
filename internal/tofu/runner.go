// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"context"
	"io"
	"log/slog"
	"os/exec"
	"strings"
)

var execCommandContext = exec.CommandContext

type LockfileMode int

const (
	LockfileReadonly LockfileMode = iota
	LockfileUpdate
)

type TofuRunner interface {
	Init(ctx context.Context, lockfileMode LockfileMode) error
	Apply(ctx context.Context, opts ApplyOptions) error
	Plan(ctx context.Context, planFilePath, varsFilePath, stateFilePath string) error
	Destroy(ctx context.Context, varsFilePath, stateFilePath string) error

	GetOutput() io.Writer
	SetOutput(out io.Writer)

	GetErrorOutput() io.Writer
	SetErrorOutput(errOut io.Writer)
}

type tofuRunnerImpl struct {
	binPath     string
	workDir     string
	out, outErr io.Writer
}

type ApplyOptions struct {
	PlanFilePath  string
	VarsFilePath  string
	VarArgs       []string
	StateFilePath string
}

// NewTofuRunner creates a runner that executes tofu commands.
// workDir is the directory containing the .tf files (e.g. the infrastructure dir).
// All state/plan paths passed to individual commands should be absolute.
func NewTofuRunner(cfg Config, out, outErr io.Writer) TofuRunner {
	return &tofuRunnerImpl{
		binPath: cfg.TofuBinaryPath(),
		workDir: cfg.WorkDir(),
		out:     out,
		outErr:  outErr,
	}
}

func (s *tofuRunnerImpl) SetOutput(out io.Writer) {
	s.out = out
}

func (s *tofuRunnerImpl) SetErrorOutput(errOut io.Writer) {
	s.outErr = errOut
}

func (s *tofuRunnerImpl) GetOutput() io.Writer {
	return s.out
}

func (s *tofuRunnerImpl) GetErrorOutput() io.Writer {
	return s.outErr
}

func (s *tofuRunnerImpl) Init(ctx context.Context, lockfileMode LockfileMode) error {
	mode := "readonly"
	if lockfileMode == LockfileUpdate {
		mode = "update"
	}

	return s.exec(ctx, []string{"init", "-lockfile=" + mode})
}

func (s *tofuRunnerImpl) Plan(
	ctx context.Context,
	planFilePath, varsFilePath, stateFilePath string,
) error {
	args := []string{
		"plan",
		"-out=" + planFilePath,
		"-var-file=" + varsFilePath,
	}
	if stateFilePath != "" {
		args = append(args, "-state="+stateFilePath, "-state-out="+stateFilePath)
	}

	return s.exec(ctx, args)
}

func (s *tofuRunnerImpl) Apply(ctx context.Context, opts ApplyOptions) error {
	args := []string{"apply", "--auto-approve"}
	for _, v := range opts.VarArgs {
		if v != "" {
			args = append(args, "-var="+v)
		}
	}
	if opts.VarsFilePath != "" {
		args = append(args, "-var-file="+opts.VarsFilePath)
	}
	if opts.StateFilePath != "" {
		args = append(args, "-state="+opts.StateFilePath)
	}
	if opts.PlanFilePath != "" {
		args = append(args, opts.PlanFilePath)
	}

	return s.exec(ctx, args)
}

func (s *tofuRunnerImpl) Destroy(ctx context.Context, varsFilePath, stateFilePath string) error {
	args := []string{
		"destroy",
		"-var-file=" + varsFilePath,
		"--auto-approve",
	}
	if stateFilePath != "" {
		args = append(args, "-state="+stateFilePath)
	}

	return s.exec(ctx, args)
}

func (s *tofuRunnerImpl) exec(ctx context.Context, args []string) error {
	slog.Debug(
		"executing tofu command",
		"binpath",
		s.binPath,
		"args",
		strings.Join(args, " "),
		"workDir",
		s.workDir,
	)
	cmd := execCommandContext(ctx, s.binPath, args...)
	cmd.Dir = s.workDir
	cmd.Stdout = s.out
	cmd.Stderr = s.outErr

	return cmd.Run()
}
