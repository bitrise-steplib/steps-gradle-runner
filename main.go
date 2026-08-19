package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/bitrise-build-cache-cli/v2/pkg/reactnative/wrap"
	"github.com/bitrise-io/go-steputils/v2/export"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/fileutil"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/log/colorstring"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/kballard/go-shellquote"
)

const (
	bitriseGradleResultsTextEnvKey = "BITRISE_GRADLE_RAW_RESULT_TEXT_PATH"
	rawGradleResultFileName        = "raw-gradle-output.log"
)

// Config ...
type Config struct {
	// Gradle Inputs
	BuildRootDirectory string `env:"build_root_directory,required"`
	GradleTasks        string `env:"gradle_task,required"`
	GradlewPath        string `env:"gradlew_path"`
	GradleOptions      string `env:"gradle_options"`
	// Export config
	AppFileIncludeFilter     string `env:"app_file_include_filter,required"`
	AppFileExcludeFilter     string `env:"app_file_exclude_filter"`
	TestApkFileIncludeFilter string `env:"test_apk_file_include_filter"`
	TestApkFileExcludeFilter string `env:"test_apk_file_exclude_filter"`
	MappingFileIncludeFilter string `env:"mapping_file_include_filter"`
	MappingFileExcludeFilter string `env:"mapping_file_exclude_filter"`

	// Other configs
	DeployDir string `env:"BITRISE_DEPLOY_DIR"`
}

func runGradleTask(
	logger log.Logger,
	cmdFactory command.Factory,
	exporter export.Exporter,
	gradleTool, tasks, options, workDir, destDir string,
) error {
	optionSlice, err := shellquote.Split(options)
	if err != nil {
		return err
	}

	taskSlice, err := shellquote.Split(tasks)
	if err != nil {
		return err
	}

	cmdSlice := []string{gradleTool}
	cmdSlice = append(cmdSlice, taskSlice...)
	cmdSlice = append(cmdSlice, optionSlice...)

	gradleArgs := cmdSlice[1:]
	det := wrap.Detect(context.Background(), wrap.DetectParams{Logger: logger})
	if det.ReactNativeEnabled {
		logger.Infof("Bitrise Build Cache: React Native cache active — wrapping gradle with %s", det.CLIPath)
	}
	name, wrappedArgs := wrap.Wrap(det, gradleTool, gradleArgs)

	if shouldSaveOutputToLogFile(optionSlice) {
		// Do not write to stdout as debug log may contain sensitive information.
		var outBuffer bytes.Buffer
		cmd := cmdFactory.Create(name, wrappedArgs, &command.Opts{
			Stdout: &outBuffer,
			Stderr: &outBuffer,
			Dir:    workDir,
		})

		fmt.Println()
		logger.Donef("$ %s", cmd.PrintableCommandArgs())
		fmt.Println()

		rawOutputLogPath := filepath.Join(destDir, rawGradleResultFileName)
		cmdErr := cmd.Run()

		return runAndExportOutput(logger, exporter, outBuffer.String(), rawOutputLogPath, bitriseGradleResultsTextEnvKey, 20, cmdErr)
	}

	cmd := cmdFactory.Create(name, wrappedArgs, &command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Dir:    workDir,
	})

	fmt.Println()
	logger.Donef("$ %s", cmd.PrintableCommandArgs())
	fmt.Println()

	if err := cmd.Run(); err != nil {
		var exitErr *command.ExitStatusError
		if errors.As(err, &exitErr) {
			return err
		}

		return fmt.Errorf("could not run gradlew command: %v", err)
	}

	return nil
}

// runAndExportOutput mirrors the v1 commandhelper.RunAndExportOutput behavior:
// write the captured output to a file, export the path via envman, log the last
// N lines (colored by cmdErr), and return the command error.
func runAndExportOutput(
	logger log.Logger,
	exporter export.Exporter,
	rawOutput, destinationPath, envKey string,
	lines int,
	cmdErr error,
) error {
	lastLines, exportErr := exporter.ExportStringToFileOutputAndReturnLastNLines(envKey, rawOutput, destinationPath, lines)
	if exportErr != nil {
		logger.Warnf("Failed to export %s, error: %s", envKey, exportErr)
	}

	if lines > 0 && len(lastLines) > 0 {
		banner := "You can find the last couple of lines of output below.:"
		if cmdErr != nil {
			logger.Errorf(banner)
		} else {
			logger.Infof(banner)
		}

		logger.Printf(lastLines)

		if cmdErr != nil {
			logger.Warnf("If you can't find the reason of the error in the log, please check the %s.", destinationPath)
		}
	}

	logger.Infof(colorstring.Magenta(fmt.Sprintf(`The log file is stored in %s, and its full path is available in the $%s environment variable.`, destinationPath, envKey)))

	return cmdErr
}

func shouldSaveOutputToLogFile(options []string) bool {
	for _, option := range options {
		if option == "--debug" || option == "-d" {
			return true
		}
	}

	return false
}

func filterEmpty(in []string) (out []string) {
	for _, item := range in {
		if strings.TrimSpace(item) != "" {
			out = append(out, item)
		}
	}
	return
}

func createDeployPth(pathChecker pathutil.PathChecker, deployDir, apkName string) (string, error) {
	deployPth := filepath.Join(deployDir, apkName)

	if exist, err := pathChecker.IsPathExists(deployPth); err != nil {
		return "", err
	} else if exist {
		return "", fmt.Errorf("file already exists at: %s", deployPth)
	}

	return deployPth, nil
}

func findDeployPth(logger log.Logger, pathChecker pathutil.PathChecker, deployDir, baseName, ext string) (string, error) {
	deployPth := ""
	retryApkName := baseName + ext

	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		requestedPath := filepath.Join(deployDir, retryApkName)
		if attempt > 0 {
			logger.Warnf("Trying %s instead", requestedPath)
		}

		pth, pathErr := createDeployPth(pathChecker, deployDir, retryApkName)
		if pathErr != nil {
			logger.Warnf("Couldn't open %s for writing: %s", requestedPath, pathErr.Error())
		}

		t := time.Now()
		retryApkName = baseName + t.Format("20060102150405") + ext
		deployPth = pth
		lastErr = pathErr

		if pathErr == nil {
			return deployPth, nil
		}

		if attempt < 9 {
			time.Sleep(1 * time.Second)
		}
	}

	return deployPth, lastErr
}

func failf(logger log.Logger, message string, args ...interface{}) {
	logger.Errorf(message, args...)
	os.Exit(1)
}

func main() {
	logger := log.NewLogger()

	var configs Config
	envRepo := env.NewRepository()
	parser := stepconf.NewInputParser(envRepo)
	if err := parser.Parse(&configs); err != nil {
		failf(logger, "Issue with input: %s", err)
	}
	stepconf.Print(configs)
	fmt.Println()

	cmdFactory := command.NewFactory(envRepo)
	fm := fileutil.NewFileManager()
	exporter := export.NewExporter(cmdFactory, fm)
	pathChecker := pathutil.NewPathChecker()

	gradlewPath, err := resolveGradlewPath(configs.BuildRootDirectory, configs.GradlewPath)
	if err != nil {
		failf(logger, "Failed to resolve gradlew path: %s", err)
	}

	buildRootAbs, err := filepath.Abs(configs.BuildRootDirectory)
	if err != nil {
		failf(logger, "Can't get absolute path for build_root_directory (%s): %s", configs.BuildRootDirectory, err)
	}

	if err := os.Chmod(gradlewPath, 0770); err != nil {
		failf(logger, "Failed to add executable permission on gradlew file (%s): %s", gradlewPath, err)
	}

	gradleStarted := time.Now()

	logger.Infof("Running gradle task...")
	if err := runGradleTask(logger, cmdFactory, exporter, gradlewPath, configs.GradleTasks, configs.GradleOptions, buildRootAbs, configs.DeployDir); err != nil {
		failf(logger, "Gradle task failed: %s", err)
	}

	// Move apk and aab files
	fmt.Println()
	logger.Infof("Move APK and AAB files...")
	appFiles, err := findArtifacts(logger, buildRootAbs,
		filePatterns{
			include: filterEmpty(strings.Split(configs.AppFileIncludeFilter, "\n")),
			exclude: filterEmpty(strings.Split(configs.AppFileExcludeFilter, "\n")),
		})
	if err != nil {
		failf(logger, "Failed to find APK or AAB files: %s", err)
	}
	if len(appFiles) == 0 {
		logger.Warnf("No file name matched app filters")
	}

	var copiedApkFiles []string
	var copiedAabFiles []string
	for _, appFile := range appFiles {
		fi, err := os.Lstat(appFile)
		if err != nil {
			failf(logger, "Failed to get file info: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			logger.Warnf("skipping: %s, modified before the gradle task has started", appFile)
			continue
		}

		ext := filepath.Ext(appFile)
		baseName := filepath.Base(appFile)
		baseName = strings.TrimSuffix(baseName, ext)
		fileName := baseName + ext

		logger.Printf("Copying %s --> %s", appFile, filepath.Join(configs.DeployDir, fileName))

		deployPth, err := findDeployPth(logger, pathChecker, configs.DeployDir, baseName, ext)
		if err != nil {
			failf(logger, "Failed to create deploy path for %s: %s", fileName, err)
		}

		if err := fm.CopyFile(appFile, deployPth, &fileutil.CopyOptions{Overwrite: true}); err != nil {
			failf(logger, "Failed to copy %s: %s", fileName, err)
		}

		switch strings.ToLower(ext) {
		case ".apk":
			copiedApkFiles = append(copiedApkFiles, deployPth)
		case ".aab":
			copiedAabFiles = append(copiedAabFiles, deployPth)
		default:
		}
	}

	for appEnv, appFiles := range map[string][]string{
		"BITRISE_APK_PATH": copiedApkFiles,
		"BITRISE_AAB_PATH": copiedAabFiles} {
		if len(appFiles) != 0 {
			lastCopiedFile := appFiles[len(appFiles)-1]
			if err := exporter.ExportOutput(appEnv, lastCopiedFile); err != nil {
				failf(logger, "Failed to export environment (%s): %s", appEnv, err)
			}
			logger.Donef("The app path is now available in the Environment Variable: $%s (value: %s)", appEnv, lastCopiedFile)
		}
	}
	for appListEnv, appFiles := range map[string][]string{
		"BITRISE_APK_PATH_LIST": copiedApkFiles,
		"BITRISE_AAB_PATH_LIST": copiedAabFiles} {
		if len(appFiles) != 0 {
			appList := strings.Join(appFiles, "|")
			if err := exporter.ExportOutput(appListEnv, appList); err != nil {
				failf(logger, "Failed to export environment (%s): %s", appListEnv, err)
			}
			logger.Donef("The app paths list is now available in the Environment Variable: $%s (value: %s)", appListEnv, appList)
		}
	}

	testApkFiles, err := findArtifacts(logger, buildRootAbs,
		filePatterns{
			include: filterEmpty(strings.Split(configs.TestApkFileIncludeFilter, "\n")),
			exclude: filterEmpty(strings.Split(configs.TestApkFileExcludeFilter, "\n")),
		})
	if err != nil {
		failf(logger, "Failed to find test apk files: %s", err)
	}

	if len(testApkFiles) == 0 {
		logger.Warnf("No file name matched test apk filters")
	}

	lastCopiedTestApkFile := ""
	for _, apkFile := range testApkFiles {
		fi, err := os.Lstat(apkFile)
		if err != nil {
			failf(logger, "Failed to get file info: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			logger.Warnf("skipping: %s, modified before the gradle task has started", apkFile)
			continue
		}

		ext := filepath.Ext(apkFile)
		baseName := filepath.Base(apkFile)
		baseName = strings.TrimSuffix(baseName, ext)
		fileName := baseName + ext

		logger.Printf("Copying %s --> %s", apkFile, filepath.Join(configs.DeployDir, fileName))

		deployPth, err := findDeployPth(logger, pathChecker, configs.DeployDir, baseName, ext)
		if err != nil {
			failf(logger, "Failed to create deploy path for %s: %s", fileName, err)
		}

		if err := fm.CopyFile(apkFile, deployPth, &fileutil.CopyOptions{Overwrite: true}); err != nil {
			failf(logger, "Failed to copy %s: %s", fileName, err)
		}

		lastCopiedTestApkFile = deployPth
	}
	if lastCopiedTestApkFile != "" {
		if err := exporter.ExportOutput("BITRISE_TEST_APK_PATH", lastCopiedTestApkFile); err != nil {
			failf(logger, "Failed to export environment (BITRISE_TEST_APK_PATH): %s", err)
		}
		logger.Donef("The apk path is now available in the Environment Variable: $BITRISE_TEST_APK_PATH (value: %s)", lastCopiedTestApkFile)
	}

	// Move mapping files
	logger.Infof("Move mapping files...")
	mappingFiles, err := findArtifacts(logger, buildRootAbs,
		filePatterns{
			include: filterEmpty(strings.Split(configs.MappingFileIncludeFilter, "\n")),
			exclude: filterEmpty(strings.Split(configs.MappingFileExcludeFilter, "\n")),
		})
	if err != nil {
		failf(logger, "Failed to find mapping files: %s", err)
	}

	if len(mappingFiles) == 0 {
		logger.Printf("No mapping file matched the filters")
	}

	var copiedMappingFiles []string
	for _, mappingFile := range mappingFiles {
		fi, err := os.Lstat(mappingFile)
		if err != nil {
			failf(logger, "Failed to get file info: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			logger.Warnf("skipping: %s, modified before the gradle task has started", mappingFile)
			continue
		}

		ext := filepath.Ext(mappingFile)
		baseName := filepath.Base(mappingFile)
		baseName = strings.TrimSuffix(baseName, ext)
		fileName := baseName + ext

		logger.Printf("Copying %s --> %s", mappingFile, filepath.Join(configs.DeployDir, fileName))

		deployPth, err := findDeployPth(logger, pathChecker, configs.DeployDir, baseName, ext)
		if err != nil {
			failf(logger, "Failed to create deploy path for %s: %s", fileName, err)
		}

		if err := fm.CopyFile(mappingFile, deployPth, &fileutil.CopyOptions{Overwrite: true}); err != nil {
			failf(logger, "Failed to copy %s: %s", fileName, err)
		}

		copiedMappingFiles = append(copiedMappingFiles, deployPth)
	}

	if len(copiedMappingFiles) != 0 {
		lastCopiedMappingFile := copiedMappingFiles[len(copiedMappingFiles)-1]
		if err := exporter.ExportOutput("BITRISE_MAPPING_PATH", lastCopiedMappingFile); err != nil {
			failf(logger, "Failed to export environment (BITRISE_MAPPING_PATH): %s", err)
		}
		logger.Donef("The mapping path is now available in the Environment Variable: $BITRISE_MAPPING_PATH (value: %s)", lastCopiedMappingFile)

		mappingList := strings.Join(copiedMappingFiles, "|")
		if err := exporter.ExportOutput("BITRISE_MAPPING_PATH_LIST", mappingList); err != nil {
			failf(logger, "Failed to export environment (BITRISE_MAPPING_PATH_LIST): %s", err)
		}
		logger.Donef("The mapping paths list is now available in the Environment Variable: $BITRISE_MAPPING_PATH_LIST (value: %s)", mappingList)
	}
}
