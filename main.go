package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-android/cache"
	utilscache "github.com/bitrise-io/go-steputils/cache"
	"github.com/bitrise-io/go-steputils/commandhelper"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/retry"
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

	// Debug
	CacheLevel string `env:"cache_level,opt['all','only_deps','none']"`

	// Other configs
	DeployDir string `env:"BITRISE_DEPLOY_DIR"`
}

func runGradleTask(gradleTool, tasks, options, workDir, destDir string) error {
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

	fmt.Println()
	log.Donef("$ %s", command.PrintableCommandArgs(false, cmdSlice))
	fmt.Println()

	cmd := command.New(cmdSlice[0], cmdSlice[1:]...)
	cmd.SetDir(workDir)

	if shouldSaveOutputToLogFile(optionSlice) { // Do not write to stdout as debug log may contain sensitive information
		rawOutputLogPath := filepath.Join(destDir, rawGradleResultFileName)
		return commandhelper.RunAndExportOutput(*cmd, rawOutputLogPath, bitriseGradleResultsTextEnvKey, 20)
	}

	cmd.SetStdout(os.Stdout)
	cmd.SetStderr(os.Stderr)
	if err := cmd.Run(); err != nil {
		if errorutil.IsExitStatusError(err) {
			return err
		}

		return fmt.Errorf("could not run gradlew command: %v", err)
	}

	return nil
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

func createDeployPth(deployDir, apkName string) (string, error) {
	deployPth := filepath.Join(deployDir, apkName)

	if exist, err := pathutil.IsPathExists(deployPth); err != nil {
		return "", err
	} else if exist {
		return "", fmt.Errorf("file already exists at: %s", deployPth)
	}

	return deployPth, nil
}

func findDeployPth(deployDir, baseName, ext string) (string, error) {
	deployPth := ""
	retryApkName := baseName + ext

	err := retry.Times(10).Wait(1 * time.Second).Try(func(attempt uint) error {
		requestedPath := filepath.Join(deployDir, retryApkName)
		if attempt > 0 {
			log.Warnf("Trying %s instead", requestedPath)
		}

		pth, pathErr := createDeployPth(deployDir, retryApkName)
		if pathErr != nil {
			log.Warnf("Couldn't open %s for writing: %s", requestedPath, pathErr.Error())
		}

		t := time.Now()
		retryApkName = baseName + t.Format("20060102150405") + ext
		deployPth = pth

		return pathErr
	})

	return deployPth, err
}

func exportEnvironmentWithEnvman(keyStr, valueStr string) error {
	cmd := command.New("envman", "add", "--key", keyStr)
	cmd.SetStdin(strings.NewReader(valueStr))
	return cmd.Run()
}

func failf(message string, args ...interface{}) {
	log.Errorf(message, args...)
	os.Exit(1)
}

func main() {
	var configs Config
	if err := stepconf.Parse(&configs); err != nil {
		failf("Issue with input: %s", err)
	}
	stepconf.Print(configs)
	fmt.Println()

	gradlewPath, err := resolveGradlewPath(configs.BuildRootDirectory, configs.GradlewPath)
	if err != nil {
		failf("Failed to resolve gradlew path: %s", err)
	}

	buildRootAbs, err := filepath.Abs(configs.BuildRootDirectory)
	if err != nil {
		failf("Can't get absolute path for build_root_directory (%s): %s", configs.BuildRootDirectory, err)
	}

	if err := os.Chmod(gradlewPath, 0770); err != nil {
		failf("Failed to add executable permission on gradlew file (%s): %s", gradlewPath, err)
	}

	gradleStarted := time.Now()

	log.Infof("Running gradle task...")
	if err := runGradleTask(gradlewPath, configs.GradleTasks, configs.GradleOptions, buildRootAbs, configs.DeployDir); err != nil {
		failf("Gradle task failed: %s", err)
	}

	// Collecting caches
	fmt.Println()
	log.Infof("Collecting cache:")
	if warning := cache.Collect(buildRootAbs, utilscache.Level(configs.CacheLevel)); warning != nil {
		log.Warnf("%s", warning)
	}

	// Move apk and aab files
	fmt.Println()
	log.Infof("Move APK and AAB files...")
	appFiles, err := findArtifacts(buildRootAbs,
		filePatterns{
			include: filterEmpty(strings.Split(configs.AppFileIncludeFilter, "\n")),
			exclude: filterEmpty(strings.Split(configs.AppFileExcludeFilter, "\n")),
		})
	if err != nil {
		failf("Failed to find APK or AAB files: %s", err)
	}
	if len(appFiles) == 0 {
		log.Warnf("No file name matched app filters")
	}

	var copiedApkFiles []string
	var copiedAabFiles []string
	for _, appFile := range appFiles {
		fi, err := os.Lstat(appFile)
		if err != nil {
			failf("Failed to get file info: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			log.Warnf("skipping: %s, modified before the gradle task has started", appFile)
			continue
		}

		ext := filepath.Ext(appFile)
		baseName := filepath.Base(appFile)
		baseName = strings.TrimSuffix(baseName, ext)
		fileName := baseName + ext

		log.Printf("Copying %s --> %s", appFile, filepath.Join(configs.DeployDir, fileName))

		deployPth, err := findDeployPth(configs.DeployDir, baseName, ext)
		if err != nil {
			failf("Failed to create deploy path for %s: %s", fileName, err)
		}

		if err := command.CopyFile(appFile, deployPth); err != nil {
			failf("Failed to copy %s: %s", fileName, err)
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
			if err := exportEnvironmentWithEnvman(appEnv, lastCopiedFile); err != nil {
				failf("Failed to export environment (%s): %s", appEnv, err)
			}
			log.Donef("The app path is now available in the Environment Variable: $%s (value: %s)", appEnv, lastCopiedFile)
		}
	}
	for appListEnv, appFiles := range map[string][]string{
		"BITRISE_APK_PATH_LIST": copiedApkFiles,
		"BITRISE_AAB_PATH_LIST": copiedAabFiles} {
		if len(appFiles) != 0 {
			appList := strings.Join(appFiles, "|")
			if err := exportEnvironmentWithEnvman(appListEnv, appList); err != nil {
				failf("Failed to export environment (%s): %s", appListEnv, err)
			}
			log.Donef("The app paths list is now available in the Environment Variable: $%s (value: %s)", appListEnv, appList)
		}
	}

	testApkFiles, err := findArtifacts(buildRootAbs,
		filePatterns{
			include: filterEmpty(strings.Split(configs.TestApkFileIncludeFilter, "\n")),
			exclude: filterEmpty(strings.Split(configs.TestApkFileExcludeFilter, "\n")),
		})
	if err != nil {
		failf("Failed to find test apk files: %s", err)
	}

	if len(testApkFiles) == 0 {
		log.Warnf("No file name matched test apk filters")
	}

	lastCopiedTestApkFile := ""
	for _, apkFile := range testApkFiles {
		fi, err := os.Lstat(apkFile)
		if err != nil {
			failf("Failed to get file info: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			log.Warnf("skipping: %s, modified before the gradle task has started", apkFile)
			continue
		}

		ext := filepath.Ext(apkFile)
		baseName := filepath.Base(apkFile)
		baseName = strings.TrimSuffix(baseName, ext)
		fileName := baseName + ext

		log.Printf("Copying %s --> %s", apkFile, filepath.Join(configs.DeployDir, fileName))

		deployPth, err := findDeployPth(configs.DeployDir, baseName, ext)
		if err != nil {
			failf("Failed to create deploy path for %s: %s", fileName, err)
		}

		if err := command.CopyFile(apkFile, deployPth); err != nil {
			failf("Failed to copy %s: %s", fileName, err)
		}

		lastCopiedTestApkFile = deployPth
	}
	if lastCopiedTestApkFile != "" {
		if err := exportEnvironmentWithEnvman("BITRISE_TEST_APK_PATH", lastCopiedTestApkFile); err != nil {
			failf("Failed to export environment (BITRISE_TEST_APK_PATH): %s", err)
		}
		log.Donef("The apk path is now available in the Environment Variable: $BITRISE_TEST_APK_PATH (value: %s)", lastCopiedTestApkFile)
	}

	// Move mapping files
	log.Infof("Move mapping files...")
	mappingFiles, err := findArtifacts(buildRootAbs,
		filePatterns{
			include: filterEmpty(strings.Split(configs.MappingFileIncludeFilter, "\n")),
			exclude: filterEmpty(strings.Split(configs.MappingFileExcludeFilter, "\n")),
		})
	if err != nil {
		failf("Failed to find mapping files: %s", err)
	}

	if len(mappingFiles) == 0 {
		log.Printf("No mapping file matched the filters")
	}

	lastCopiedMappingFile := ""
	for _, mappingFile := range mappingFiles {
		fi, err := os.Lstat(mappingFile)
		if err != nil {
			failf("Failed to get file info: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			log.Warnf("skipping: %s, modified before the gradle task has started", mappingFile)
			continue
		}

		ext := filepath.Ext(mappingFile)
		baseName := filepath.Base(mappingFile)
		baseName = strings.TrimSuffix(baseName, ext)
		fileName := baseName + ext

		log.Printf("Copying %s --> %s", mappingFile, filepath.Join(configs.DeployDir, fileName))

		deployPth, err := findDeployPth(configs.DeployDir, baseName, ext)
		if err != nil {
			failf("Failed to create deploy path for %s: %s", fileName, err)
		}

		if err := command.CopyFile(mappingFile, deployPth); err != nil {
			failf("Failed to copy %s: %s", fileName, err)
		}

		lastCopiedMappingFile = deployPth
	}

	if lastCopiedMappingFile != "" {
		if err := exportEnvironmentWithEnvman("BITRISE_MAPPING_PATH", lastCopiedMappingFile); err != nil {
			failf("Failed to export environment (BITRISE_MAPPING_PATH): %s", err)
		}
		log.Donef("The mapping path is now available in the Environment Variable: $BITRISE_MAPPING_PATH (value: %s)", lastCopiedMappingFile)
	}
}
