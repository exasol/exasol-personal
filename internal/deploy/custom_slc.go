// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect"
	connecttypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/customslc"
	"github.com/exasol/exasol-personal/internal/slc"
)

const (
	// A custom SLC goes in the default BucketFS bucket so its activation URI matches the
	// documented Exasol shape.
	customSLCBucketFS = "bfsdefault"
	customSLCBucket   = "default"

	scriptLanguagesQuery = "SELECT SYSTEM_VALUE FROM EXA_PARAMETERS " +
		"WHERE PARAMETER_NAME='SCRIPT_LANGUAGES'"
)

// Activation goes through ALTER SYSTEM, which needs a live database.
var ErrCustomSLCDatabaseNotRunning = errors.New(
	"the database must be running for custom SLC operations; run `exasol start` first",
)

// customAliasPattern restricts a custom alias to a valid unquoted Exasol identifier (letter
// first, letters/digits/underscores, max 128), ASCII-only because the alias is also a BucketFS
// directory name and URI component; this keeps it usable in `CREATE <alias> SCALAR SCRIPT`.
var customAliasPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,127}$`)

// Exactly one of File or URL must be set.
type CustomSLCInstallOpts struct {
	Alias    string
	Language string
	File     string
	URL      string
}

// A nil value means pre-approved (e.g. --auto-approve).
type CustomSLCConfirm func(prompt string) (bool, error)

type CustomSLCInstallResult struct {
	Alias            string
	Language         string
	AlreadyInstalled bool
	Replaced         bool
}

type CustomSLCUpdateResult struct {
	Found     bool
	Unchanged bool
	Alias     string
}

type CustomSLCRemoveResult struct {
	Found bool
	Alias string
}

type CustomSLCStatus struct {
	Alias    string
	Language string
	Source   string
}

func InstallCustomSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	opts CustomSLCInstallOpts,
	confirm CustomSLCConfirm,
) (*CustomSLCInstallResult, error) {
	var result *CustomSLCInstallResult
	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			var err error
			result, err = installCustomSLCLocked(ctx, deployment, opts, confirm)

			return err
		})

	return result, err
}

// Alias collisions are resolved before any download, so a blocked or declined install never
// fetches the (large) container.
func installCustomSLCLocked(
	ctx context.Context,
	deployment config.DeploymentDir,
	opts CustomSLCInstallOpts,
	confirm CustomSLCConfirm,
) (*CustomSLCInstallResult, error) {
	request, err := validateCustomSLCOpts(opts)
	if err != nil {
		return nil, err
	}
	if err := requireCustomSLCPreconditions(ctx, deployment); err != nil {
		return nil, err
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, err
	}
	entries, err := readScriptLanguages(ctx, deployment)
	if err != nil {
		return nil, err
	}

	customIdx := findInstalledCustomSLC(state.InstalledCustomSLCs, request.alias)
	replaced := false
	if customIdx < 0 {
		if err := confirmOfficialAliasReuse(
			deployment, state, entries, request.alias, confirm,
		); err != nil {
			return nil, err
		}
	} else {
		prompt := fmt.Sprintf(
			"a custom SLC is already installed under alias %q; installing replaces it",
			request.alias,
		)
		if err := confirmCustom(confirm, prompt); err != nil {
			return nil, err
		}
		replaced = true
	}

	tarball, err := acquireCustomTarball(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer tarball.cleanup()

	if customIdx >= 0 && customSLCUnchanged(
		state.InstalledCustomSLCs[customIdx], tarball.sha256, request.language,
	) {
		return &CustomSLCInstallResult{Alias: request.alias, AlreadyInstalled: true}, nil
	}

	err = applyCustomSLC(
		ctx, deployment, request.alias, request.language, request.source, tarball, state,
	)
	if err != nil {
		return nil, err
	}

	slog.Info(request.alias + " custom script language container is installed")

	return &CustomSLCInstallResult{
		Alias:    request.alias,
		Language: string(request.language),
		Replaced: replaced,
	}, nil
}

func UpdateCustomSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	opts CustomSLCInstallOpts,
) (*CustomSLCUpdateResult, error) {
	var result *CustomSLCUpdateResult
	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			var err error
			result, err = updateCustomSLCLocked(ctx, deployment, opts)

			return err
		})

	return result, err
}

// A no-op when the new container is byte-identical (same digest).
func updateCustomSLCLocked(
	ctx context.Context,
	deployment config.DeploymentDir,
	opts CustomSLCInstallOpts,
) (*CustomSLCUpdateResult, error) {
	alias, err := validateCustomAlias(opts.Alias)
	if err != nil {
		return nil, err
	}
	if _, sourceErr := validateCustomSource(opts); sourceErr != nil {
		return nil, sourceErr
	}
	if err := requireCustomSLCPreconditions(ctx, deployment); err != nil {
		return nil, err
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, err
	}
	idx := findInstalledCustomSLC(state.InstalledCustomSLCs, alias)
	if idx < 0 {
		return &CustomSLCUpdateResult{Found: false, Alias: alias}, nil
	}

	// An update keeps the recorded language unless the caller explicitly overrides it.
	languageInput := opts.Language
	if strings.TrimSpace(languageInput) == "" {
		languageInput = state.InstalledCustomSLCs[idx].Language
	}
	language, err := customslc.NormalizeLanguage(languageInput)
	if err != nil {
		return nil, err
	}

	tarball, err := acquireCustomTarball(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer tarball.cleanup()

	if customSLCUnchanged(state.InstalledCustomSLCs[idx], tarball.sha256, language) {
		return &CustomSLCUpdateResult{Found: true, Unchanged: true, Alias: alias}, nil
	}

	source, _ := validateCustomSource(opts)
	if err := applyCustomSLC(ctx, deployment, alias, language, source, tarball, state); err != nil {
		return nil, err
	}

	slog.Info(alias + " custom script language container is updated")

	return &CustomSLCUpdateResult{Found: true, Alias: alias}, nil
}

func RemoveCustomSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	alias string,
) (*CustomSLCRemoveResult, error) {
	var result *CustomSLCRemoveResult
	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			var err error
			result, err = removeCustomSLCLocked(ctx, deployment, alias)

			return err
		})

	return result, err
}

func removeCustomSLCLocked(
	ctx context.Context,
	deployment config.DeploymentDir,
	alias string,
) (*CustomSLCRemoveResult, error) {
	normalized, err := validateCustomAlias(alias)
	if err != nil {
		return nil, err
	}
	if err := requireCustomSLCPreconditions(ctx, deployment); err != nil {
		return nil, err
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, err
	}
	idx := findInstalledCustomSLC(state.InstalledCustomSLCs, normalized)
	if idx < 0 {
		return &CustomSLCRemoveResult{Found: false, Alias: normalized}, nil
	}
	removed := state.InstalledCustomSLCs[idx]

	backend, err := newDeploymentBackendForDeployment(deployment)
	if err != nil {
		return nil, err
	}

	slog.Info("deactivating the " + removed.Alias + " custom script language container")
	err = deactivateCustomSLC(ctx, deployment, removed.Alias, removed.DisplacedURI)
	if err != nil {
		return nil, err
	}

	state.InstalledCustomSLCs = append(
		state.InstalledCustomSLCs[:idx:idx],
		state.InstalledCustomSLCs[idx+1:]...,
	)
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		return nil, err
	}

	slog.Info("removing the custom script language container files from the deployment")
	if err := backend.removeCustomSLCFiles(ctx, customSLCDirOf(removed)); err != nil {
		slog.Warn(
			"failed to remove the custom SLC files; delete them manually if needed",
			"dir", customSLCDirOf(removed), "error", err,
		)
	}

	slog.Info(removed.Alias + " custom script language container is removed")

	return &CustomSLCRemoveResult{Found: true, Alias: removed.Alias}, nil
}

// A missing state file is treated as "none installed" so listing works before the deployment
// is initialized.
func CustomSLCStatuses(deployment config.DeploymentDir) ([]CustomSLCStatus, error) {
	has, err := config.HasExasolPersonalStateFile(deployment)
	if err != nil {
		return nil, err
	}
	if !has {
		return []CustomSLCStatus{}, nil
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, err
	}

	statuses := make([]CustomSLCStatus, 0, len(state.InstalledCustomSLCs))
	for _, inst := range state.InstalledCustomSLCs {
		statuses = append(statuses, CustomSLCStatus{
			Alias:    inst.Alias,
			Language: inst.Language,
			Source:   inst.Source,
		})
	}

	return statuses, nil
}

// Lets the CLI route remove/update to the custom path by alias ownership.
func IsCustomSLCAlias(deployment config.DeploymentDir, alias string) (bool, error) {
	has, err := config.HasExasolPersonalStateFile(deployment)
	if err != nil {
		return false, err
	}
	if !has {
		return false, nil
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return false, err
	}

	return findInstalledCustomSLC(state.InstalledCustomSLCs, alias) >= 0, nil
}

type customSLCRequest struct {
	alias    string
	language customslc.Language
	source   string
}

func validateCustomSLCOpts(opts CustomSLCInstallOpts) (customSLCRequest, error) {
	alias, err := validateCustomAlias(opts.Alias)
	if err != nil {
		return customSLCRequest{}, err
	}
	language, err := customslc.NormalizeLanguage(opts.Language)
	if err != nil {
		return customSLCRequest{}, err
	}
	source, err := validateCustomSource(opts)
	if err != nil {
		return customSLCRequest{}, err
	}

	return customSLCRequest{alias: alias, language: language, source: source}, nil
}

func requireCustomSLCPreconditions(ctx context.Context, deployment config.DeploymentDir) error {
	if !isLocalDeployment(deployment) {
		return ErrSLCNotSupported
	}
	if err := requireDeploymentPresent(deployment); err != nil {
		return err
	}
	if !isLocalDeploymentRunning(ctx, deployment) {
		return ErrCustomSLCDatabaseNotRunning
	}

	return nil
}

func confirmOfficialAliasReuse(
	deployment config.DeploymentDir,
	state *config.ExasolPersonalState,
	entries []customslc.AliasEntry,
	alias string,
	confirm CustomSLCConfirm,
) error {
	if flavor := officialOwner(state.InstalledSLCs, alias); flavor != "" {
		return fmt.Errorf(
			"alias %q is provided by the installed official SLC %q; remove it with "+
				"`exasol slc remove %s` or choose a different --alias",
			alias, flavor, flavor,
		)
	}

	officialNames := officialAliasNamespace(deployment, entries)
	if !officialNames[alias] {
		return nil
	}

	prompt := fmt.Sprintf(
		"%q is an official alias (%s); installing a custom SLC under it overrides the "+
			"built-in for new sessions",
		alias, strings.Join(sortedKeys(officialNames), ", "),
	)

	return confirmCustom(confirm, prompt)
}

//nolint:revive // arguments carry the resolved install inputs, not internal control flags.
func applyCustomSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	alias string,
	language customslc.Language,
	source string,
	tarball acquiredTarball,
	state *config.ExasolPersonalState,
) error {
	backend, err := newDeploymentBackendForDeployment(deployment)
	if err != nil {
		return err
	}

	file, err := os.Open(
		tarball.path,
	) //nolint:gosec // path is launcher-owned (download temp or user file)
	if err != nil {
		return err
	}
	defer file.Close()

	// Validate the whole archive on the host before writing anything into the deployment, so a
	// corrupt or non-SLC container is rejected before the database's BucketFS ever sees it.
	if err := customslc.ValidateArchive(file); err != nil {
		return fmt.Errorf("invalid custom SLC container: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	var previousDir string
	var previous *config.InstalledCustomSLC
	if idx := findInstalledCustomSLC(state.InstalledCustomSLCs, alias); idx >= 0 {
		previousDir = customSLCDirOf(state.InstalledCustomSLCs[idx])
		previous = &state.InstalledCustomSLCs[idx]
	}

	dir := customSLCDir(alias, tarball.sha256)
	if err := ensureCustomSLCDelivered(
		ctx, backend, deployment, alias, dir, previousDir, file,
	); err != nil {
		return err
	}

	// Delivered files are left in place on activation failure: the directory is content-addressed,
	// so it is either unreferenced (a retry re-extracts it) or the one the database now points at
	// (deleting it would break the live alias).
	slog.Info("activating the custom script language container")
	uri := customslc.BuildActivationURI(customSLCBucketFS, customSLCBucket, dir, language)
	displacedBuiltin, err := activateCustomSLC(ctx, deployment, alias, uri)
	if err != nil {
		return err
	}

	state.InstalledCustomSLCs = upsertInstalledCustomSLC(
		state.InstalledCustomSLCs,
		config.InstalledCustomSLC{
			Alias:        alias,
			Language:     string(language),
			BucketPath:   customSLCBucketFS + "/" + customSLCBucket + "/" + dir,
			Sha256:       tarball.sha256,
			Source:       source,
			DisplacedURI: carriedDisplacedURI(previous, displacedBuiltin),
		},
	)
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		return err
	}

	if previousDir != "" && previousDir != dir {
		if err := backend.removeCustomSLCFiles(ctx, previousDir); err != nil {
			slog.Warn(
				"failed to remove the previous custom SLC files; delete them manually if needed",
				"dir", previousDir, "error", err,
			)
		}
	}

	return nil
}

func ensureCustomSLCDelivered(
	ctx context.Context,
	backend deploymentBackend,
	deployment config.DeploymentDir,
	alias, dir, previousDir string,
	file io.Reader,
) error {
	current, err := readScriptLanguages(ctx, deployment)
	if err != nil {
		return err
	}
	activeDir := ""
	if entry, ok := customslc.FindAlias(current, alias); ok {
		activeDir = customslc.DirFromURI(entry.URI)
	}

	// Re-delivering a dir that is already recorded or live would overwrite the live container.
	if dir == previousDir || dir == activeDir {
		return nil
	}

	slog.Info("unpacking the custom script language container into the deployment " +
		"(this may take a few minutes)")

	return backend.deliverCustomSLC(ctx, dir, file)
}

func customSLCDir(alias, digestHex string) string {
	const digestLen = 16

	digest := digestHex
	if len(digest) > digestLen {
		digest = digest[:digestLen]
	}

	return strings.ToLower(alias) + "-" + digest
}

func customSLCDirOf(inst config.InstalledCustomSLC) string {
	if inst.BucketPath != "" {
		return path.Base(inst.BucketPath)
	}

	return strings.ToLower(inst.Alias)
}

func carriedDisplacedURI(previous *config.InstalledCustomSLC, displacedBuiltin string) string {
	if previous != nil {
		return previous.DisplacedURI
	}

	return displacedBuiltin
}

func customSLCUnchanged(
	recorded config.InstalledCustomSLC,
	digest string,
	language customslc.Language,
) bool {
	return recorded.Sha256 == digest && recorded.Language == string(language)
}

func aliasResolvesTo(entries []customslc.AliasEntry, alias, uri string) bool {
	entry, ok := customslc.FindAlias(entries, alias)

	return ok && entry.URI == uri
}

func activateCustomSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	alias, uri string,
) (string, error) {
	var displaced string
	err := withLocalDatabase(ctx, deployment, func(database connecttypes.Databaser) error {
		entries, err := currentScriptLanguages(ctx, database)
		if err != nil {
			return err
		}
		if existing, ok := customslc.FindAlias(entries, alias); ok &&
			customslc.IsBuiltinURI(existing.URI) {
			displaced = existing.URI
		}
		if err := setScriptLanguages(
			ctx, database, customslc.SetAlias(entries, alias, uri),
		); err != nil {
			return err
		}

		// Read back so a silently-ignored ALTER SYSTEM surfaces as an error here rather than
		// as a confusing "no such language" failure at the user's first UDF call.
		updated, err := currentScriptLanguages(ctx, database)
		if err != nil {
			return err
		}
		if !aliasResolvesTo(updated, alias, uri) {
			return fmt.Errorf(
				"activation did not take effect: alias %q does not resolve to the expected "+
					"container in SCRIPT_LANGUAGES after ALTER SYSTEM", alias,
			)
		}

		return nil
	})

	return displaced, err
}

func deactivateCustomSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	alias, restoreURI string,
) error {
	return withLocalDatabase(ctx, deployment, func(database connecttypes.Databaser) error {
		entries, err := currentScriptLanguages(ctx, database)
		if err != nil {
			return err
		}

		var updated []customslc.AliasEntry
		if restoreURI != "" {
			updated = customslc.SetAlias(entries, alias, restoreURI)
		} else {
			updated = customslc.RemoveAlias(entries, alias)
		}
		if err := setScriptLanguages(ctx, database, updated); err != nil {
			return err
		}

		confirmed, err := currentScriptLanguages(ctx, database)
		if err != nil {
			return err
		}

		return confirmDeactivated(confirmed, alias, restoreURI)
	})
}

func confirmDeactivated(entries []customslc.AliasEntry, alias, restoreURI string) error {
	if restoreURI != "" {
		if !aliasResolvesTo(entries, alias, restoreURI) {
			return fmt.Errorf(
				"deactivation did not take effect: alias %q was not restored to the "+
					"built-in in SCRIPT_LANGUAGES after ALTER SYSTEM", alias,
			)
		}

		return nil
	}
	if _, ok := customslc.FindAlias(entries, alias); ok {
		return fmt.Errorf(
			"deactivation did not take effect: alias %q is still present in "+
				"SCRIPT_LANGUAGES after ALTER SYSTEM", alias,
		)
	}

	return nil
}

func readScriptLanguages(
	ctx context.Context,
	deployment config.DeploymentDir,
) ([]customslc.AliasEntry, error) {
	var entries []customslc.AliasEntry
	err := withLocalDatabase(ctx, deployment, func(database connecttypes.Databaser) error {
		parsed, err := currentScriptLanguages(ctx, database)
		entries = parsed

		return err
	})

	return entries, err
}

func currentScriptLanguages(
	ctx context.Context,
	database connecttypes.Databaser,
) ([]customslc.AliasEntry, error) {
	result, err := database.Exec(ctx, scriptLanguagesQuery, 0)
	if err != nil {
		return nil, err
	}
	rows := result.Rows()
	if len(rows) == 0 || len(rows[0]) == 0 {
		return nil, nil
	}

	return customslc.ParseScriptLanguages(rows[0][0]), nil
}

func setScriptLanguages(
	ctx context.Context,
	database connecttypes.Databaser,
	entries []customslc.AliasEntry,
) error {
	value := customslc.SerializeScriptLanguages(entries)
	if strings.Contains(value, "'") {
		return fmt.Errorf(
			"refusing to write SCRIPT_LANGUAGES: value contains a single quote (%q)",
			value,
		)
	}
	_, err := database.Exec(ctx, "ALTER SYSTEM SET SCRIPT_LANGUAGES='"+value+"'", 0)

	return err
}

func withLocalDatabase(
	ctx context.Context,
	deployment config.DeploymentDir,
	callback func(connecttypes.Databaser) error,
) error {
	connectionInfo, err := config.ResolveConnectionInfo(deployment)
	if err != nil {
		return err
	}

	database, err := connect.NewExasolConnection(
		deployment,
		connectionInfo,
		connectionInfo.Username,
		"",
		connectionInfo.InsecureSkipCertValidation,
	)
	if err != nil {
		return err
	}
	if err := database.Connect(ctx); err != nil {
		return err
	}
	defer database.Close()

	return callback(database)
}

// acquiredTarball's cleanup deletes a downloaded tarball but is a no-op for a user-supplied
// --file, which must never be deleted.
type acquiredTarball struct {
	path    string
	sha256  string
	cleanup func()
}

func acquireCustomTarball(ctx context.Context, opts CustomSLCInstallOpts) (acquiredTarball, error) {
	if filePath := strings.TrimSpace(opts.File); filePath != "" {
		slog.Info("reading the custom script language container", "file", filePath)
		sha, err := hashFile(filePath)
		if err != nil {
			return acquiredTarball{}, err
		}

		return acquiredTarball{
			path:   filePath,
			sha256: sha,
			cleanup: func() {
				// A user-supplied --file is not ours to delete.
			},
		}, nil
	}

	slog.Info("downloading the custom script language container (this may take a few minutes)")

	return downloadCustomTarball(ctx, strings.TrimSpace(opts.URL))
}

func downloadCustomTarball(ctx context.Context, url string) (acquiredTarball, error) {
	tmp, err := os.CreateTemp("", "custom-slc-*.tar.gz")
	if err != nil {
		return acquiredTarball{}, err
	}
	defer tmp.Close()

	remove := func() { _ = os.Remove(tmp.Name()) }

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		remove()

		return acquiredTarball{}, err
	}
	client := &http.Client{CheckRedirect: rejectNonHTTPSRedirect}
	resp, err := client.Do(req)
	if err != nil {
		remove()

		return acquiredTarball{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		remove()

		return acquiredTarball{}, fmt.Errorf(
			"failed to download custom SLC from %s: %s", url, resp.Status,
		)
	}

	hasher := sha256.New()
	if _, err := io.Copy(tmp, io.TeeReader(resp.Body, hasher)); err != nil {
		remove()

		return acquiredTarball{}, err
	}

	return acquiredTarball{
		path:    tmp.Name(),
		sha256:  hex.EncodeToString(hasher.Sum(nil)),
		cleanup: remove,
	}, nil
}

func rejectNonHTTPSRedirect(req *http.Request, _ []*http.Request) error {
	if !strings.EqualFold(req.URL.Scheme, "https") {
		return fmt.Errorf("refusing redirect to non-https URL %s", req.URL.String())
	}

	return nil
}

func hashFile(filePath string) (string, error) {
	file, err := os.Open(filePath) //nolint:gosec // path is user-supplied by design (--file)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func validateCustomAlias(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("a custom SLC requires an --alias")
	}
	if !customAliasPattern.MatchString(trimmed) {
		return "", fmt.Errorf(
			"invalid alias %q: must start with a letter and use only letters, digits, and "+
				"underscores (max 128 characters)", trimmed,
		)
	}

	return customslc.NormalizeAlias(trimmed), nil
}

func validateCustomSource(opts CustomSLCInstallOpts) (string, error) {
	hasFile := strings.TrimSpace(opts.File) != ""
	hasURL := strings.TrimSpace(opts.URL) != ""
	if hasFile == hasURL {
		return "", errors.New("provide exactly one of --file or --url")
	}
	if hasFile {
		return strings.TrimSpace(opts.File), nil
	}

	rawURL := strings.TrimSpace(opts.URL)
	parsed, err := neturl.ParseRequestURI(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid --url %q", rawURL)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", errors.New("--url must use https; use --file for local files")
	}

	return rawURL, nil
}

func confirmCustom(confirm CustomSLCConfirm, prompt string) error {
	if confirm == nil {
		return nil
	}
	ok, err := confirm(prompt)
	if err != nil {
		return err
	}
	if !ok {
		return ErrSLCOperationCancelled
	}

	return nil
}

func officialAliasNamespace(
	_ config.DeploymentDir,
	entries []customslc.AliasEntry,
) map[string]bool {
	names := make(map[string]bool)
	for _, alias := range customslc.BuiltinAliases(entries) {
		names[alias] = true
	}

	catalog, err := slc.Load(resources.SLCCatalogYAML)
	if err != nil {
		return names
	}
	catalogEntries, err := catalog.List(runtime.GOARCH)
	if err != nil {
		return names
	}
	for _, entry := range catalogEntries {
		for _, alias := range entry.Aliases {
			names[customslc.NormalizeAlias(alias)] = true
		}
	}

	return names
}

func officialOwner(installed []config.InstalledSLC, alias string) string {
	needle := customslc.NormalizeAlias(alias)
	for _, inst := range installed {
		for _, declared := range inst.Aliases {
			if customslc.NormalizeAlias(declared) == needle {
				return inst.Flavor
			}
		}
	}

	return ""
}

func findInstalledCustomSLC(installed []config.InstalledCustomSLC, alias string) int {
	needle := customslc.NormalizeAlias(alias)
	for idx, inst := range installed {
		if customslc.NormalizeAlias(inst.Alias) == needle {
			return idx
		}
	}

	return -1
}

func upsertInstalledCustomSLC(
	existing []config.InstalledCustomSLC,
	entry config.InstalledCustomSLC,
) []config.InstalledCustomSLC {
	updated := make([]config.InstalledCustomSLC, 0, len(existing)+1)
	for _, inst := range existing {
		if customslc.NormalizeAlias(inst.Alias) == customslc.NormalizeAlias(entry.Alias) {
			continue
		}
		updated = append(updated, inst)
	}
	updated = append(updated, entry)
	sort.Slice(updated, func(i, j int) bool {
		return updated[i].Alias < updated[j].Alias
	})

	return updated
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}
