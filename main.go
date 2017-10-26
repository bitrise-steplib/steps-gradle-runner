package main

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/retry"
	"github.com/bitrise-tools/go-steputils/cache"
	"github.com/bitrise-tools/go-steputils/input"
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

// ConfigsModel ...
type ConfigsModel struct {
	// Gradle Inputs
	GradleFile               string
	GradleTasks              string
	GradlewPath              string
	GradleOptions            string
	ApkFileIncludeFilter     string
	ApkFileExcludeFilter     string
	TestApkFileIncludeFilter string
	TestApkFileExcludeFilter string
	MappingFileIncludeFilter string
	MappingFileExcludeFilter string

	// Other configs
	DeployDir string
	// Cache configs
	CacheLevel string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		GradleFile:               os.Getenv("gradle_file"),
		GradleTasks:              os.Getenv("gradle_task"),
		GradlewPath:              os.Getenv("gradlew_path"),
		GradleOptions:            os.Getenv("gradle_options"),
		ApkFileIncludeFilter:     os.Getenv("apk_file_include_filter"),
		ApkFileExcludeFilter:     os.Getenv("apk_file_exclude_filter"),
		TestApkFileIncludeFilter: os.Getenv("test_apk_file_include_filter"),
		TestApkFileExcludeFilter: os.Getenv("test_apk_file_exclude_filter"),
		MappingFileIncludeFilter: os.Getenv("mapping_file_include_filter"),
		MappingFileExcludeFilter: os.Getenv("mapping_file_exclude_filter"),
		//
		DeployDir: os.Getenv("BITRISE_DEPLOY_DIR"),
		//
		CacheLevel: os.Getenv("cache_level"),
	}
}

func (configs ConfigsModel) print() {

	log.Infof("Configs:")
	log.Printf("- GradlewPath: %s", configs.GradlewPath)
	log.Printf("- GradleFile: %s", configs.GradleFile)
	log.Printf("- GradleTasks: %s", configs.GradleTasks)
	log.Printf("- GradleOptions: %s", configs.GradleOptions)
	log.Printf("- ApkFileIncludeFilter: %s", configs.ApkFileIncludeFilter)
	log.Printf("- ApkFileExcludeFilter: %s", configs.ApkFileExcludeFilter)
	log.Printf("- TestApkFileIncludeFilter: %s", configs.TestApkFileIncludeFilter)
	log.Printf("- TestApkFileExcludeFilter: %s", configs.TestApkFileExcludeFilter)
	log.Printf("- MappingFileIncludeFilter: %s", configs.MappingFileIncludeFilter)
	log.Printf("- MappingFileExcludeFilter: %s", configs.MappingFileExcludeFilter)
	log.Printf("- DeployDir: %s", configs.DeployDir)
	log.Printf("- CacheLevel: %s", configs.CacheLevel)
}

func (configs ConfigsModel) validate() (string, error) {
	if configs.GradleFile != "" {
		if exist, err := pathutil.IsPathExists(configs.GradleFile); err != nil {
			return "", fmt.Errorf("Failed to check if GradleFile exists at: %s, error: %s", configs.GradleFile, err)
		} else if !exist {
			return "", fmt.Errorf("GradleFile does not exist at: %s", configs.GradleFile)
		}
	}

	if configs.GradleTasks == "" {
		return "", errors.New("no GradleTask parameter specified")
	}

	if configs.GradlewPath == "" {
		explanation := `
Using a Gradle Wrapper (gradlew) is required, as the wrapper is what makes sure
that the right Gradle version is installed and used for the build.

You can find more information about the Gradle Wrapper (gradlew),
and about how you can generate one (if you would not have one already
in the official guide at: https://docs.gradle.org/current/userguide/gradle_wrapper.html`

		return explanation, errors.New("no GradlewPath parameter specified")
	}
	if exist, err := pathutil.IsPathExists(configs.GradlewPath); err != nil {
		return "", fmt.Errorf("Failed to check if GradlewPath exist at: %s, error: %s", configs.GradlewPath, err)
	} else if !exist {
		return "", fmt.Errorf("GradlewPath not exist at: %s", configs.GradlewPath)
	}

	if err := input.ValidateIfNotEmpty(configs.CacheLevel); err != nil {
		return "", fmt.Errorf("CacheLevel, error: %s", err)
	}

	if err := input.ValidateWithOptions(configs.CacheLevel, "all", "only_deps", "none"); err != nil {
		return "", fmt.Errorf("CacheLevel, error: %s", err)
	}

	return "", nil
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
		if exclude != "" {
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
	configs := createConfigsModelFromEnvs()
	configs.print()
	if explanation, err := configs.validate(); err != nil {
		fmt.Println()
		log.Errorf("Issue with input: %s", err)
		fmt.Println()

		if explanation != "" {
			fmt.Println(explanation)
			fmt.Println()
		}

		os.Exit(1)
	}

	if configs.ApkFileIncludeFilter == "" {
		configs.ApkFileIncludeFilter = "*.apk"
	}

	err := os.Chmod(configs.GradlewPath, 0770)
	if err != nil {
		failf("Failed to add executable permission on gradlew file (%s), error: %s", configs.GradlewPath, err)
	}

	log.Infof("Running gradle task...")
	if err := runGradleTask(configs.GradlewPath, configs.GradleFile, configs.GradleTasks, configs.GradleOptions, true); err != nil {
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
			log.Warnf("Cache collection skipped: failed to determine project root path.")
			collectCaches = false
		}

		lockFilePath := filepath.Join(projectRoot, "gradle.deps")

		if configs.CacheLevel == "all" || configs.CacheLevel == "only_deps" {

			// create dependencies lockfile
			log.Printf(" Generate dependencies map...")
			lockfileContent := ""
			if err := filepath.Walk(projectRoot, func(path string, f os.FileInfo, err error) error {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".gradle") && !strings.Contains(path, "node_modules") {
					if md5Hash, err := computeMD5String(path); err != nil {
						log.Warnf("Failed to compute MD5 hash of file(%s), error: %s", path, err)
					} else {
						lockfileContent += md5Hash
					}
				}
				return nil
			}); err != nil {
				log.Warnf("Dependency map generation skipped: failed to collect dependencies.")
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
				log.Warnf("Cache collection skipped: failed to determine cache paths.")
				collectCaches = false
			}
		}
		if collectCaches {
			gradleCache.IncludePath(strings.Join(includePths, "\n"))
			gradleCache.ExcludePath(strings.Join(excludePths, "\n"))

			if err := gradleCache.Commit(); err != nil {
				log.Warnf("Cache collection skipped: failed to commit cache paths.")
			}
		}
		log.Donef("Done")
	}

	// Move apk files
	fmt.Println()
	log.Infof("Move apk files...")
	apkFiles, err := find(".", configs.ApkFileIncludeFilter, configs.ApkFileExcludeFilter)
	if err != nil {
		failf("Failed to find apk files, error: %s", err)
	}

	if len(apkFiles) == 0 {
		log.Warnf("No file name matched apk filters")
	}

	lastCopiedApkFile := ""
	copiedApkFiles := []string{}
	for _, apkFile := range apkFiles {
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

		lastCopiedApkFile = deployPth
		copiedApkFiles = append(copiedApkFiles, deployPth)
	}

	if lastCopiedApkFile != "" {
		if err := exportEnvironmentWithEnvman("BITRISE_APK_PATH", lastCopiedApkFile); err != nil {
			failf("Failed to export enviroment (BITRISE_APK_PATH), error: %s", err)
		}
		log.Donef("The apk path is now available in the Environment Variable: $BITRISE_APK_PATH (value: %s)", lastCopiedApkFile)
	}
	if len(copiedApkFiles) > 0 {
		apkList := strings.Join(copiedApkFiles, "|")
		if err := exportEnvironmentWithEnvman("BITRISE_APK_PATH_LIST", apkList); err != nil {
			failf("Failed to export enviroment (BITRISE_APK_PATH_LIST), error: %s", err)
		}
		log.Donef("The apk paths list is now available in the Environment Variable: $BITRISE_APK_PATH_LIST (value: %s)", apkList)
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
