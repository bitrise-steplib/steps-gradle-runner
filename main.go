package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bitrise-io/go-android/cache"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/retry"
	"github.com/kballard/go-shellquote"
)

const failedToFindTargetWithHashString = `Failed to find target with hash string `
const failedToFindBuildToolRevision = `Failed to find Build Tools revision `
const failedToFindPlatformSDKWithPath = `Failed to find Platform SDK with path: `
const couldNotHEAD = `Could not HEAD `
const connectionTimedOut = `Connection timed out`
const couldNotRead = `Could not read `
const couldNotGetResource = `Could not get resource `
const couldNotGET = `Could not GET `
const couldNotDownload = `Could not download `
const receivedStatusCode503 = `Received status code 503 from server: Service Temporarily Unavailable`
const causeErrorInOpeningZipFile = `Cause: error in opening zip file.`
const failedToDownloadResource = `Failed to download resource`
const failedToDownloadSHA1ForResource = `Failed to download SHA1 for resource`

var automaticRetryReasonPatterns = []string{
	failedToFindTargetWithHashString,
	failedToFindBuildToolRevision,
	failedToFindPlatformSDKWithPath,
	couldNotHEAD,
	connectionTimedOut,
	couldNotRead,
	couldNotGetResource,
	couldNotGET,
	couldNotDownload,
	receivedStatusCode503,
	causeErrorInOpeningZipFile,
	failedToDownloadResource,
	failedToDownloadSHA1ForResource,
}

func isCouldNotFindInOutput(outputToSearchIn string) (shouldRetry bool, retryReasonPattern string) {
	retryReasonPattern = `(?i)Could not find ([^ ]*)`
	r := regexp.MustCompile(retryReasonPattern)
	matches := r.FindStringSubmatch(outputToSearchIn)
	shouldRetry = len(matches) == 2 && matches[1] != "google-services.json"
	return
}

// Config ...
type Config struct {
	// Gradle Inputs
	GradleFile    string `env:"gradle_file"`
	GradleTasks   string `env:"gradle_task,required"`
	GradlewPath   string `env:"gradlew_path,file"`
	GradleOptions string `env:"gradle_options"`
	// Export config
	AppFileIncludeFilter     string `env:"app_file_include_filter,required"`
	AppFileExcludeFilter     string `env:"app_file_exclude_filter"`
	TestApkFileIncludeFilter string `env:"test_apk_file_include_filter"`
	TestApkFileExcludeFilter string `env:"test_apk_file_exclude_filter"`
	MappingFileIncludeFilter string `env:"mapping_file_include_filter"`
	MappingFileExcludeFilter string `env:"mapping_file_exclude_filter"`

	// Debug
	CacheLevel     string `env:"cache_level,opt['all','only_deps','none']"`
	RetryOnFailure bool   `env:"retry_on_failure,opt['yes','no]"`

	// Other configs
	DeployDir string `env:"BITRISE_DEPLOY_DIR"`

	// Deprecated
	ApkFileIncludeFilter string `env:"apk_file_include_filter"`
	ApkFileExcludeFilter string `env:"apk_file_exclude_filter"`
}

func isStringFoundInOutput(searchStr, outputToSearchIn string) bool {
	r := regexp.MustCompile("(?i)" + searchStr)
	return r.MatchString(outputToSearchIn)
}

func shouldRetry(outputToSearchIn string) (bool, string) {
	for _, retryReasonPattern := range automaticRetryReasonPatterns {
		if isStringFoundInOutput(retryReasonPattern, outputToSearchIn) {
			return true, retryReasonPattern
		}
	}
	return isCouldNotFindInOutput(outputToSearchIn)
}

func runGradleTask(gradleTool, buildFile, tasks, options string, isAutomaticRetryOnReason bool) error {
	optionSlice, err := shellquote.Split(options)
	if err != nil {
		return err
	}

	taskSlice, err := shellquote.Split(tasks)
	if err != nil {
		return err
	}

	cmdSlice := []string{gradleTool}
	if buildFile != "" {
		cmdSlice = append(cmdSlice, "--build-file", buildFile)
	}
	cmdSlice = append(cmdSlice, taskSlice...)
	cmdSlice = append(cmdSlice, optionSlice...)

	log.Printf(command.PrintableCommandArgs(false, cmdSlice))
	fmt.Println()

	var outBuffer bytes.Buffer
	outWriter := io.MultiWriter(os.Stdout, &outBuffer)

	cmd := command.New(cmdSlice[0], cmdSlice[1:]...)
	cmd.SetStdout(outWriter)
	cmd.SetStderr(outWriter)
	if err := cmd.Run(); err != nil {
		if isAutomaticRetryOnReason {
			if isRetry, retryReasonPattern := shouldRetry(outBuffer.String()); isRetry {
				log.Warnf("Automatic retry reason found in log: %s - retrying...", retryReasonPattern)
				return runGradleTask(gradleTool, buildFile, tasks, options, false)
			}
		}
		return err
	}
	return nil
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
		if attempt > 0 {
			log.Warnf("  Retrying...")
		}

		pth, pathErr := createDeployPth(deployDir, retryApkName)
		if pathErr != nil {
			log.Warnf("  %d attempt failed:", attempt+1)
			log.Printf(pathErr.Error())
		}

		t := time.Now()
		retryApkName = baseName + t.Format("20060102150405") + ext
		deployPth = pth

		return pathErr
	})

	return deployPth, err
}

func validateAndMigrateConfig(config *Config) error {
	if config.GradleFile != "" {
		if exist, err := pathutil.IsPathExists(config.GradleFile); err != nil {
			return fmt.Errorf("failed to check if GradleFile exists at: %s, error: %s", config.GradleFile, err)
		} else if !exist {
			return fmt.Errorf("GradleFile does not exist at: %s", config.GradleFile)
		}
	}

	if strings.TrimSpace(config.ApkFileIncludeFilter) != "" {
		log.Warnf(`Step input 'APK file include filter' (apk_file_include_filter) is deprecated and will be removed soon,
use 'APK and AAB file include filter' (app_file_include_filter) instead.`)
		fmt.Println()
		log.Infof(`'APK file include filter' (apk_file_include_filter) is used, 'APK and AAB file include filter' (app_file_include_filter) is ignored.
Use 'APK and AAB file include filter' and set 'APK file include filter' to empty value.`)
		fmt.Println()
		config.AppFileIncludeFilter = config.ApkFileIncludeFilter
	}
	if strings.TrimSpace(config.ApkFileExcludeFilter) != "" {
		log.Warnf(`Step input 'APK file exclude filter' (apk_file_exclude_filter) is deprecated and will be removed soon,
use 'APK and AAB file exclude filter' (app_file_exclude_filter) instead.`)
		fmt.Println()
		log.Infof(`'APK file exclude filter' (apk_file_exclude_filter) is used, 'APK and AAB file exclude filter' (app_file_exclude_filter) is ignored.
Use 'APK and AAB file exclude filter' and set 'APK file exclude filter' to empty value.`)
		fmt.Println()
		config.AppFileExcludeFilter = config.ApkFileExcludeFilter
	}
	return nil
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
	if err := validateAndMigrateConfig(&configs); err != nil {
		failf("Issue with input: %s", err)
	}
	fmt.Println()

	gradlewPath, err := filepath.Abs(configs.GradlewPath)
	if err != nil {
		failf("Can't get absolute path for gradlew file (%s), error: %s", configs.GradlewPath, err)
	}

	if err := os.Chmod(gradlewPath, 0770); err != nil {
		failf("Failed to add executable permission on gradlew file (%s), error: %s", gradlewPath, err)
	}

	gradleStarted := time.Now()

	log.Infof("Running gradle task...")
	if err := runGradleTask(gradlewPath, configs.GradleFile, configs.GradleTasks, configs.GradleOptions, configs.RetryOnFailure); err != nil {
		failf("Gradle task failed, error: %s", err)
	}

	// Collecting caches
	log.Infof("Collecting cache:")
	const defaultProjectRoot = "."
	if warning := cache.Collect(defaultProjectRoot, cache.Level(configs.CacheLevel)); warning != nil {
		log.Warnf("%s", warning)
	}

	// Move apk and aab files
	fmt.Println()
	log.Infof("Move APK and AAB files...")
	appFiles, err := findArtifacts(".",
		filePatterns{
			include: filterEmpty(strings.Split(configs.AppFileIncludeFilter, "\n")),
			exclude: filterEmpty(strings.Split(configs.AppFileExcludeFilter, "\n")),
		})
	if err != nil {
		failf("Failed to find APK or AAB files, error: %s", err)
	}
	if len(appFiles) == 0 {
		log.Warnf("No file name matched app filters")
	}

	var copiedApkFiles []string
	var copiedAabFiles []string
	for _, appFile := range appFiles {
		fi, err := os.Lstat(appFile)
		if err != nil {
			failf("Failed to get file info, error: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			log.Warnf("skipping: %s, modified before the gradle task has started", appFile)
			continue
		}

		ext := filepath.Ext(appFile)
		baseName := filepath.Base(appFile)
		baseName = strings.TrimSuffix(baseName, ext)

		deployPth, err := findDeployPth(configs.DeployDir, baseName, ext)
		if err != nil {
			failf("Failed to create apk deploy path, error: %s", err)
		}

		log.Printf("copy %s to %s", appFile, deployPth)
		if err := command.CopyFile(appFile, deployPth); err != nil {
			failf("Failed to copy apk, error: %s", err)
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
				failf("Failed to export environment (%s), error: %s", appEnv, err)
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
				failf("Failed to export environment (%s), error: %s", appListEnv, err)
			}
			log.Donef("The app paths list is now available in the Environment Variable: $%s (value: %s)", appListEnv, appList)
		}
	}

	testApkFiles, err := findArtifacts(".",
		filePatterns{
			include: filterEmpty(strings.Split(configs.TestApkFileIncludeFilter, "\n")),
			exclude: filterEmpty(strings.Split(configs.TestApkFileExcludeFilter, "\n")),
		})
	if err != nil {
		failf("Failed to find test apk files, error: %s", err)
	}

	if len(testApkFiles) == 0 {
		log.Warnf("No file name matched test apk filters")
	}

	lastCopiedTestApkFile := ""
	for _, apkFile := range testApkFiles {
		fi, err := os.Lstat(apkFile)
		if err != nil {
			failf("Failed to get file info, error: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			log.Warnf("skipping: %s, modified before the gradle task has started", apkFile)
			continue
		}

		ext := filepath.Ext(apkFile)
		baseName := filepath.Base(apkFile)
		baseName = strings.TrimSuffix(baseName, ext)

		deployPth, err := findDeployPth(configs.DeployDir, baseName, ext)
		if err != nil {
			failf("Failed to create apk deploy path, error: %s", err)
		}

		log.Printf("copy %s to %s", apkFile, deployPth)
		if err := command.CopyFile(apkFile, deployPth); err != nil {
			failf("Failed to copy apk, error: %s", err)
		}

		lastCopiedTestApkFile = deployPth
	}
	if lastCopiedTestApkFile != "" {
		if err := exportEnvironmentWithEnvman("BITRISE_TEST_APK_PATH", lastCopiedTestApkFile); err != nil {
			failf("Failed to export environment (BITRISE_TEST_APK_PATH), error: %s", err)
		}
		log.Donef("The apk path is now available in the Environment Variable: $BITRISE_TEST_APK_PATH (value: %s)", lastCopiedTestApkFile)
	}

	// Move mapping files
	log.Infof("Move mapping files...")
	mappingFiles, err := findArtifacts(".",
		filePatterns{
			include: filterEmpty(strings.Split(configs.MappingFileIncludeFilter, "\n")),
			exclude: filterEmpty(strings.Split(configs.MappingFileExcludeFilter, "\n")),
		})
	if err != nil {
		failf("Failed to find mapping files, error: %s", err)
	}

	if len(mappingFiles) == 0 {
		log.Printf("No mapping file matched the filters")
	}

	lastCopiedMappingFile := ""
	for _, mappingFile := range mappingFiles {
		fi, err := os.Lstat(mappingFile)
		if err != nil {
			failf("Failed to get file info, error: %s", err)
		}

		if fi.ModTime().Before(gradleStarted) {
			log.Warnf("skipping: %s, modified before the gradle task has started", mappingFile)
			continue
		}

		ext := filepath.Ext(mappingFile)
		baseName := filepath.Base(mappingFile)
		baseName = strings.TrimSuffix(baseName, ext)

		deployPth, err := findDeployPth(configs.DeployDir, baseName, ext)
		if err != nil {
			failf("Failed to create mapping deploy path, error: %s", err)
		}

		log.Printf("copy %s to %s", mappingFile, deployPth)
		if err := command.CopyFile(mappingFile, deployPth); err != nil {
			failf("Failed to copy mapping file, error: %s", err)
		}

		lastCopiedMappingFile = deployPth
	}

	if lastCopiedMappingFile != "" {
		if err := exportEnvironmentWithEnvman("BITRISE_MAPPING_PATH", lastCopiedMappingFile); err != nil {
			failf("Failed to export environment (BITRISE_MAPPING_PATH), error: %s", err)
		}
		log.Donef("The mapping path is now available in the Environment Variable: $BITRISE_MAPPING_PATH (value: %s)", lastCopiedMappingFile)
	}
}
