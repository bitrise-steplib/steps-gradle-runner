package xcelerate

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/auth/store"
	configcommon "github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/config/common"
	multiplatformconfig "github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/config/multiplatform"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/consts"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/envexport"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/paths"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/utils"
)

const (
	ActivateXcodeSuccessful = "✅ Bitrise Build Cache for Xcode activated"
	AddXcelerateToPath      = "ℹ️ To start building, run `export PATH=~/.bitrise-xcelerate/bin:$PATH` or restart your terminal."
	ErrFmtCreateXcodeConfig = "failed to create Xcode config: %w"

	cliBasename                    = "bitrise-build-cache-cli"
	xcodebuildWrapperScriptContent = `#!/bin/bash
set -e

if [ "${1-}" = "-version" ]; then
  %s "$@"
else
  %s/bitrise-build-cache-cli xcelerate xcodebuild "$@"
fi
`
	xcrunWrapperScriptContent = `#!/bin/bash
set -e

if [ "${1-}" = "xcodebuild" ] && [ "${2-}" = "-version" ]; then
  %s "$@"
elif [ "${1-}" = "xcodebuild" ]; then
  shift
  %s/bitrise-build-cache-cli xcelerate xcodebuild "$@"
else
  %s "$@"
fi
`
)

// Activate creates the Xcode build cache configuration, copies the CLI binary,
// and sets up the xcodebuild wrapper script.
func Activate(
	ctx context.Context,
	logger log.Logger,
	osProxy utils.OsProxy,
	commandFunc utils.CommandFunc,
	encoderFactory utils.EncoderFactory,
	decoderFactory utils.DecoderFactory,
	activateXcodeParams Params,
	envs map[string]string,
) error {
	overrideActivateXcodeParamsFromExistingConfig(
		logger, osProxy, &activateXcodeParams, decoderFactory, envs)

	authConfig, _, err := configcommon.ResolveAuthConfig(envs)
	if err != nil {
		return fmt.Errorf("resolve auth config: %w", err)
	}

	benchmarkClient := configcommon.NewBenchmarkPhaseClient(consts.BitriseWebsiteBaseURL, authConfig, logger)

	config, err := NewConfig(
		ctx,
		logger,
		activateXcodeParams,
		envs,
		osProxy,
		commandFunc,
		envexport.New(envs, logger),
		benchmarkClient,
	)
	if err != nil {
		return fmt.Errorf("failed to create xcelerate config: %w", err)
	}

	if err := config.Save(logger, osProxy, encoderFactory); err != nil {
		return fmt.Errorf(ErrFmtCreateXcodeConfig, err)
	}

	mpCfg := multiplatformconfig.Config{DebugLogging: config.DebugLogging}
	store.PersistActivateCreds(logger, envs, config.AuthConfig, &mpCfg)
	if err := mpCfg.Save(osProxy, encoderFactory); err != nil {
		return fmt.Errorf("failed to save multiplatform analytics config: %w", err)
	}
	logger.Infof("Wrote multiplatform analytics config: %s", multiplatformconfig.FilePath(osProxy))

	if err := copyCLIToXcelerateBinDir(ctx, osProxy, logger); err != nil {
		return fmt.Errorf("failed to copy xcelerate cli to ~/.bitrise-xcelerate/bin: %w", err)
	}

	if err := addXcelerateCommandToPathWithScriptWrapper(config, osProxy, logger, envs); err != nil { //nolint:contextcheck // envman export inside is fire-and-forget; ctx propagation would cascade through TemplateInventory/ApplyBenchmarkPhase for no operational gain
		return fmt.Errorf("failed to add xcelerate command: %w", err)
	}

	exportDerivedDataPath(logger, config, envs) //nolint:contextcheck // envman export inside is fire-and-forget, matching the wrapper-script export above

	logger.TInfof(ActivateXcodeSuccessful)
	logger.TInfof(AddXcelerateToPath)

	return nil
}

// exportDerivedDataPath publishes where the wrapper relocates DerivedData to, so cache steps can
// target the SPM checkouts under it.
func exportDerivedDataPath(logger log.Logger, config Config, envs map[string]string) {
	if !config.BuildCacheEnabled || config.BuildCacheSkipFlags || config.DisablePrefixMapping {
		return
	}

	p, err := paths.Default()
	if err != nil {
		logger.Debugf("Skipping %s export: %v", EnvDerivedDataPath, err)

		return
	}

	envexport.New(envs, logger).Export(EnvDerivedDataPath, p.XcodeManagedDerivedDataRoot())
}

// ---------------------------------------------------------------------------
// Private — activation helpers
// ---------------------------------------------------------------------------

func overrideActivateXcodeParamsFromExistingConfig(
	logger log.Logger,
	osProxy utils.OsProxy,
	activateXcodeParams *Params,
	decoderFactory utils.DecoderFactory,
	envs map[string]string,
) {
	if existingConfig, err := ReadConfig(osProxy, decoderFactory); err == nil {
		if strings.Contains(existingConfig.OriginalXcodebuildPath, PathFor(osProxy, BinDir)) {
			logger.Warnf("Removing xcelerate wrapper as original xcodebuild path...")
			existingConfig.OriginalXcodebuildPath = ""
		}

		activateXcodeParams.XcodePathOverride = cmp.Or(
			activateXcodeParams.XcodePathOverride,
			existingConfig.OriginalXcodebuildPath,
		)

		if strings.Contains(existingConfig.OriginalXcrunPath, PathFor(osProxy, BinDir)) {
			logger.Warnf("Removing xcelerate wrapper as original xcrun path...")
			existingConfig.OriginalXcrunPath = ""
		}

		activateXcodeParams.XcrunPathOverride = cmp.Or(
			activateXcodeParams.XcrunPathOverride,
			existingConfig.OriginalXcrunPath,
		)
	} else if isXcelerateInPath(osProxy, envs) {
		logger.Warnf("It seems that the xcelerate config file is missing, but xcelerate is already in the PATH. \n" +
			"This will lead to unexpected behavior when determining the xcodebuild path. \n" +
			"Defaulting to /usr/bin/xcodebuild...")
		activateXcodeParams.XcodePathOverride = "/usr/bin/xcodebuild"
	}
}

func isXcelerateInPath(osProxy utils.OsProxy, envs map[string]string) bool {
	path := envs["PATH"]
	for _, p := range strings.Split(path, ":") {
		if strings.Contains(p, PathFor(osProxy, BinDir)) {
			return true
		}
	}

	return false
}

func copyCLIToXcelerateBinDir(ctx context.Context, osProxy utils.OsProxy, logger log.Logger) error {
	src, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}

	reader, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source executable: %w", err)
	}
	defer reader.Close()

	binPath := PathFor(osProxy, BinDir)
	if err := osProxy.MkdirAll(binPath, 0o755); err != nil {
		return fmt.Errorf("failed to create bin dir: %w", err)
	}

	target := filepath.Join(binPath, cliBasename)

	// Already the pinned copy: there is nothing to copy, and the rename this
	// guards is what made terminating the running CLI necessary in the first place.
	if src == target {
		logger.TDonef("CLI already in place at %s", target)

		return nil
	}

	if err := makeSureCLIIsNotRunning(ctx, target, logger); err != nil {
		return fmt.Errorf("failed to ensure cli is not running: %w", err)
	}

	if err := writeExecutableAtomically(binPath, target, reader); err != nil {
		return err
	}

	logger.TInfof("Copied CLI to %s", target)

	return nil
}

// writeExecutableAtomically renames a temp copy over target, so a failed write or a still-running old CLI can't leave a corrupted binary in place.
func writeExecutableAtomically(dir, target string, src io.Reader) error {
	tmp, err := os.CreateTemp(dir, cliBasename+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp executable: %w", err)
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()

	if _, err = io.Copy(tmp, src); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("failed to copy executable: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp executable: %w", err)
	}

	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return fmt.Errorf("failed to chmod temp executable: %w", err)
	}

	if err := os.Rename(tmp.Name(), target); err != nil {
		return fmt.Errorf("failed to move executable into place: %w", err)
	}

	return nil
}

func makeSureCLIIsNotRunning(ctx context.Context, target string, logger log.Logger) error {
	processes, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to list processes: %w", err)
	}

	for _, p := range processes {
		// Since activate pins the bin dir onto PATH, target is what
		// `bitrise-build-cache-cli` resolves to on any machine that has activated
		// before — so without this the second activation terminates itself.
		if int(p.Pid) == os.Getpid() {
			continue
		}

		exe, err := p.ExeWithContext(ctx)
		if err != nil {
			continue
		}

		if exe != target {
			continue
		}

		logger.TWarnf("Terminating already running CLI (pid: %d)", p.Pid)

		if err := p.TerminateWithContext(ctx); err != nil {
			logger.TWarnf("Failed to terminate already running CLI, attempting to kill it")

			if err := p.KillWithContext(ctx); err != nil {
				return fmt.Errorf("failed to kill already running CLI (pid: %d): %w", p.Pid, err)
			}
		}

		waitForProcessExit(ctx, p, logger)
	}

	return nil
}

func waitForProcessExit(ctx context.Context, p *process.Process, logger log.Logger) {
	for range 50 {
		if running, err := p.IsRunningWithContext(ctx); err == nil && !running {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}

	logger.TWarnf("Already running CLI (pid: %d) did not exit in time", p.Pid)
}

func addXcelerateCommandToPathWithScriptWrapper(
	config Config,
	osProxy utils.OsProxy,
	logger log.Logger,
	envs map[string]string,
) error {
	binPath := PathFor(osProxy, BinDir)
	if err := osProxy.MkdirAll(binPath, 0o755); err != nil {
		return fmt.Errorf("failed to create bin dir: %w", err)
	}

	scriptPath := filepath.Join(binPath, "xcodebuild")

	if err := osProxy.WriteFile(scriptPath,
		[]byte(fmt.Sprintf(xcodebuildWrapperScriptContent,
			config.OriginalXcodebuildPath,
			binPath)), 0o755); err != nil {
		return fmt.Errorf("failed to create xcodebuild wrapper script: %w", err)
	}
	logger.Infof("Wrote xcodebuild wrapper script: %s", scriptPath)

	scriptPath = filepath.Join(binPath, "xcrun")

	if err := osProxy.WriteFile(scriptPath,
		[]byte(fmt.Sprintf(xcrunWrapperScriptContent,
			config.OriginalXcrunPath,
			binPath,
			config.OriginalXcrunPath)), 0o755); err != nil {
		return fmt.Errorf("failed to create xcrun wrapper script: %w", err)
	}
	logger.Infof("Wrote xcrun wrapper script: %s", scriptPath)

	path := strings.ReplaceAll(envs["PATH"], binPath+":", "")
	path = strings.Join([]string{binPath, path}, ":")

	exporter := envexport.New(envs, logger)
	exporter.Export("PATH", path)
	exporter.ExportToShellRC("Bitrise Xcelerate", fmt.Sprintf("export PATH=%s:$PATH", binPath))

	return nil
}
