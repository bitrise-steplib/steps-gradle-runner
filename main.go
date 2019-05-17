package main

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bitrise-io/go-steputils/cache"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/retry"
	"github.com/kballard/go-shellquote"
)

const failedToFindTargetWithHashString = `Failed to find target with hash string `
const failedToFindBuildToolRevision = `Failed to find Build Tools revision `
const failedToFindPlatformSDKWithPath = `Failed to find Platform SDK with path: `
const couldNotFind = `Could not find `
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
	couldNotFind,
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

// Config ...
type Config struct {
	// Gradle Inputs
	GradleFile               string `env:"gradle_file"`
	GradleTasks              string `env:"gradle_task,required"`
	GradlewPath              string `env:"gradlew_path,file"`
	GradleOptions            string `env:"gradle_options"`
	AppFileIncludeFilter     string `env:"app_file_include_filter"`
	AppFileExcludeFilter     string `env:"app_file_exclude_filter"`
	TestApkFileIncludeFilter string `env:"test_apk_file_include_filter"`
	TestApkFileExcludeFilter string `env:"test_apk_file_exclude_filter"`
	MappingFileIncludeFilter string `env:"mapping_file_include_filter"`
	MappingFileExcludeFilter string `env:"mapping_file_exclude_filter"`

	// Other configs
	DeployDir string `env:"BITRISE_DEPLOY_DIR"`
	// Cache configs
	CacheLevel string `env:"cache_level,opt['all','only_deps','none']"`

	// Deprecated
	ApkFileIncludeFilter string `env:"apk_file_include_filter"`
	ApkFileExcludeFilter string `env:"apk_file_exclude_filter"`
}

func isStringFoundInOutput(searchStr, outputToSearchIn string) bool {
	r, err := regexp.Compile("(?i)" + searchStr)
	if err != nil {
		log.Warnf("Failed to compile regexp: %s", err)
		return false
	}
	return r.MatchString(outputToSearchIn)
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
			for _, retryReasonPattern := range automaticRetryReasonPatterns {
				if isStringFoundInOutput(retryReasonPattern, outBuffer.String()) {
					log.Warnf("Automatic retry reason found in log: %s - retrying...", retryReasonPattern)
					return runGradleTask(gradleTool, buildFile, tasks, options, false)
				}
			}
		}
		return err
	}
	return nil
}

func computeMD5String(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Errorf("Failed to close file(%s), error: %s", filePath, err)
		}
	}()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func find(dir, nameInclude, nameExclude string) ([]string, error) {
	cmdSlice := []string{"find", dir}
	cmdSlice = append(cmdSlice, "-path", nameInclude)

	for _, exclude := range strings.Split(nameExclude, "\n") {
		if strings.TrimSpace(exclude) != "" {
			cmdSlice = append(cmdSlice, "!", "-path", exclude)
		}
	}

	log.Printf(command.PrintableCommandArgs(false, cmdSlice))

	out, err := command.New(cmdSlice[0], cmdSlice[1:]...).RunAndReturnTrimmedOutput()
	if err != nil {
		return []string{}, err
	}

	split := strings.Split(out, "\n")
	files := []string{}
	for _, item := range split {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			files = append(files, trimmed)
		}
	}

	return files, nil
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

	if config.ApkFileIncludeFilter != "" {
		log.Warnf("Please use *APK and AAB file include filter* instead of *APK file include filter*.")
		fmt.Println()
	}
	if config.ApkFileExcludeFilter != "" {
		log.Warnf("Please use *APK and AAB file exclude filter* instead of *APK file exclude filter*.")
		fmt.Println()
	}
	if config.AppFileIncludeFilter != "" && config.ApkFileIncludeFilter != "" {
		log.Infof("*APK file include filter* is used, *APK and AAB file include filter* is ignored.")
		fmt.Println()
		config.AppFileIncludeFilter = config.ApkFileIncludeFilter
	}
	if config.AppFileExcludeFilter != "" && config.ApkFileExcludeFilter != "" {
		log.Infof("*APK file exclude filter* is used, *APK and AAB file exclude filter* is ignored.")
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
	if err := runGradleTask(gradlewPath, configs.GradleFile, configs.GradleTasks, configs.GradleOptions, true); err != nil {
		failf("Gradle task failed, error: %s", err)
	}

	// Collecting caches
	if configs.CacheLevel != "none" {
		fmt.Println()
		log.Infof("Collecting gradle caches...")

		gradleCache := cache.New()
		homeDir := pathutil.UserHomeDir()
		collectCaches := true
		includePths := []string{}

		projectRoot, err := filepath.Abs(".")
		if err != nil {
			log.Warnf("Cache collection skipped: failed to determine project root path, error: %s", err)
			collectCaches = false
		}

		lockFilePath := filepath.Join(projectRoot, "gradle.deps")

		if configs.CacheLevel == "all" || configs.CacheLevel == "only_deps" {

			// create dependencies lockfile
			log.Printf(" Generate dependencies map...")
			lockfileContent := ""
			if err := filepath.Walk(projectRoot, func(path string, f os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".gradle") && !strings.Contains(path, "node_modules") {
					if md5Hash, err := computeMD5String(path); err != nil {
						log.Warnf("Failed to compute MD5 hash of file(%s), error: %s", path, err)
					} else {
						lockfileContent += md5Hash
					}
				}
				return nil
			}); err != nil {
				log.Warnf("Dependency map generation skipped: failed to collect dependencies, error: ", err)
				collectCaches = false
			} else {
				err := fileutil.WriteStringToFile(lockFilePath, lockfileContent)
				if err != nil {
					log.Warnf("Dependency map generation skipped: failed to write lockfile, error: %s", err)
					collectCaches = false
				}
			}

			includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".gradle"), lockFilePath))
			includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".kotlin"), lockFilePath))
			includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".m2"), lockFilePath))
		}

		if configs.CacheLevel == "all" {
			includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".android", "build-cache"), lockFilePath))
		}

		excludePths := []string{
			"~/.gradle/**",
			"~/.android/build-cache/**",
			"*.lock",
			"*.bin",
			"/**/build/**.json",
			"/**/build/**.html",
			"/**/build/**.xml",
			"/**/build/**.properties",
			"/**/build/**/zip-cache/**",
			"*.log",
			"*.txt",
			"*.rawproto",
			"!*.ap_",
			"!*.apk",
		}

		if configs.CacheLevel == "all" {
			if err := filepath.Walk(projectRoot, func(path string, f os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if f.IsDir() {
					if f.Name() == "build" {
						includePths = append(includePths, path)
					}
					if f.Name() == ".gradle" {
						includePths = append(includePths, path)
					}
				}
				return nil
			}); err != nil {
				log.Warnf("Cache collection skipped: failed to determine cache paths, error: ", err)
				collectCaches = false
			}
		}
		if collectCaches {
			gradleCache.IncludePath(strings.Join(includePths, "\n"))
			gradleCache.ExcludePath(strings.Join(excludePths, "\n"))

			if err := gradleCache.Commit(); err != nil {
				log.Warnf("Cache collection skipped: failed to commit cache paths, error: %s", err)
			}
		}
		log.Donef("Done")
	}

	// Move apk and aab files
	fmt.Println()
	log.Infof("Move APK and AAB files...")
	appFiles, err := findArtifacts(".",
		gradleStarted,
		filterEmpty(strings.Split(configs.AppFileIncludeFilter, "\n")),
		filterEmpty(strings.Split(configs.AppFileExcludeFilter, "\n")))
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
				failf("Failed to export enviroment (%s), error: %s", appEnv, err)
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
				failf("Failed to export enviroment (%s), error: %s", appListEnv, err)
			}
			log.Donef("The app paths list is now available in the Environment Variable: $%s (value: %s)", appListEnv, appList)
		}
	}

	testApkFiles, err := find(".", configs.TestApkFileIncludeFilter, configs.TestApkFileExcludeFilter)
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
			failf("Failed to export enviroment (BITRISE_TEST_APK_PATH), error: %s", err)
		}
		log.Donef("The apk path is now available in the Environment Variable: $BITRISE_TEST_APK_PATH (value: %s)", lastCopiedTestApkFile)
	}

	// Move mapping files
	log.Infof("Move mapping files...")
	mappingFiles, err := find(".", configs.MappingFileIncludeFilter, configs.MappingFileExcludeFilter)
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
			failf("Failed to export enviroment (BITRISE_MAPPING_PATH), error: %s", err)
		}
		log.Donef("The mapping path is now available in the Environment Variable: $BITRISE_MAPPING_PATH (value: %s)", lastCopiedMappingFile)
	}
}
