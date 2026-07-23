package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/bitrise-build-cache-cli/v2/pkg/reactnative/wrap"
	"github.com/bitrise-io/go-android/v2/gradle"
	"github.com/bitrise-io/go-android/v2/gradle/mappinglist"
	"github.com/bitrise-io/go-steputils/commandhelper"
	"github.com/bitrise-io/go-steputils/v2/export"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/retry"
	v2command "github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/fileutil"
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

	gradleArgs := cmdSlice[1:]
	det := wrap.Detect(context.Background(), wrap.DetectParams{})
	if det.ReactNativeEnabled {
		log.Infof("Bitrise Build Cache: React Native cache active — wrapping gradle with %s", det.CLIPath)
	}
	name, wrappedArgs := wrap.Wrap(det, gradleTool, gradleArgs)

	cmd := command.New(name, wrappedArgs...)
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

// copiedApp is an app artifact after it has been copied to the deploy dir,
// together with the build variant it belongs to.
type copiedApp struct {
	deployPath string
	variant    gradle.ArtifactVariant
	hasVariant bool
}

func deployPaths(apps []copiedApp) []string {
	paths := make([]string, len(apps))
	for i, a := range apps {
		paths[i] = a.deployPath
	}
	return paths
}

// alignedMappingList builds a mapping-file list that is index-aligned with one
// of the app lists so that mapping[i] is the mapping for app[i]. It mirrors the
// AAB list when at least one AAB variant has a mapping (matching
// google-play-deploy's AAB-over-APK preference), otherwise the APK list. Each
// entry holds the mapping path for that app's variant, or an empty string when
// the variant produced no mapping (so positions never shift). It returns
// ok=false when neither app list has a variant that matched a mapping file.
//
// The list mirrors exactly one app list, so a consumer must pair it with that
// same list (e.g. BITRISE_AAB_PATH_LIST when AABs were built): the APK and AAB
// lists can differ in length and order (e.g. ABI/density-split APKs).
func alignedMappingList(aabs, apks []copiedApp, mappingByVariant map[gradle.ArtifactVariant]string) ([]string, bool) {
	if len(mappingByVariant) == 0 {
		return nil, false
	}
	// Prefer the AAB list, but fall back to the APK list when no AAB variant
	// matched a mapping (e.g. AABs whose variant could not be resolved while the
	// APKs' could).
	if list, matched := buildMappingList(aabs, mappingByVariant); matched > 0 {
		return list, true
	}
	if list, matched := buildMappingList(apks, mappingByVariant); matched > 0 {
		return list, true
	}
	return nil, false
}

// buildMappingList returns a slice aligned with apps where each entry is the
// mapping path for that app's variant (empty when none), plus the number of
// entries that matched a mapping.
func buildMappingList(apps []copiedApp, mappingByVariant map[gradle.ArtifactVariant]string) ([]string, int) {
	if len(apps) == 0 {
		return nil, 0
	}
	list := make([]string, len(apps))
	matched := 0
	for i, app := range apps {
		if !app.hasVariant {
			continue
		}
		if mappingPath, ok := mappingByVariant[app.variant]; ok {
			list[i] = mappingPath
			matched++
		}
	}
	return list, matched
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

func failf(message string, args ...interface{}) {
	log.Errorf(message, args...)
	os.Exit(1)
}

func main() {
	var configs Config
	envRepo := env.NewRepository()
	parser := stepconf.NewInputParser(envRepo)
	if err := parser.Parse(&configs); err != nil {
		failf("Issue with input: %s", err)
	}
	stepconf.Print(configs)
	fmt.Println()

	cmdFactory := v2command.NewFactory(envRepo)
	exporter := export.NewExporter(cmdFactory, fileutil.NewFileManager())

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

	var copiedApkFiles []copiedApp
	var copiedAabFiles []copiedApp
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

		// Remember the source variant so mapping files can be paired with this
		// artifact later, even though the deploy path is flat.
		variantKey, hasVariant := gradle.VariantFromPath(appFile)
		copied := copiedApp{deployPath: deployPth, variant: variantKey, hasVariant: hasVariant}

		switch strings.ToLower(ext) {
		case ".apk":
			copiedApkFiles = append(copiedApkFiles, copied)
		case ".aab":
			copiedAabFiles = append(copiedAabFiles, copied)
		default:
		}
	}

	for appEnv, appFiles := range map[string][]copiedApp{
		"BITRISE_APK_PATH": copiedApkFiles,
		"BITRISE_AAB_PATH": copiedAabFiles} {
		if len(appFiles) != 0 {
			lastCopiedFile := appFiles[len(appFiles)-1].deployPath
			if err := exporter.ExportOutput(appEnv, lastCopiedFile); err != nil {
				failf("Failed to export environment (%s): %s", appEnv, err)
			}
			log.Donef("The app path is now available in the Environment Variable: $%s (value: %s)", appEnv, lastCopiedFile)
		}
	}
	for appListEnv, appFiles := range map[string][]copiedApp{
		"BITRISE_APK_PATH_LIST": copiedApkFiles,
		"BITRISE_AAB_PATH_LIST": copiedAabFiles} {
		if len(appFiles) != 0 {
			appList := strings.Join(deployPaths(appFiles), "|")
			if err := exporter.ExportOutput(appListEnv, appList); err != nil {
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
		if err := exporter.ExportOutput("BITRISE_TEST_APK_PATH", lastCopiedTestApkFile); err != nil {
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
	mappingByVariant := map[gradle.ArtifactVariant]string{}
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

		// Index by variant so the mapping list can be aligned with the app list.
		// First match per variant wins (matches the deterministic walk order).
		if variantKey, ok := gradle.VariantFromPath(mappingFile); ok {
			if _, exists := mappingByVariant[variantKey]; !exists {
				mappingByVariant[variantKey] = deployPth
			}
		}
	}

	if lastCopiedMappingFile != "" {
		if err := exporter.ExportOutput("BITRISE_MAPPING_PATH", lastCopiedMappingFile); err != nil {
			failf("Failed to export environment (BITRISE_MAPPING_PATH): %s", err)
		}
		log.Donef("The mapping path is now available in the Environment Variable: $BITRISE_MAPPING_PATH (value: %s)", lastCopiedMappingFile)
	}

	// Export a mapping list aligned index-by-index with the app list, so a
	// downstream step (e.g. google-play-deploy) can pair each artifact with its
	// mapping file by position.
	if mappingList, ok := alignedMappingList(copiedAabFiles, copiedApkFiles, mappingByVariant); ok {
		value := mappinglist.Encode(mappingList)
		if err := exporter.ExportOutput("BITRISE_MAPPING_PATH_LIST", value); err != nil {
			failf("Failed to export environment (BITRISE_MAPPING_PATH_LIST): %s", err)
		}
		log.Donef("The mapping paths list is now available in the Environment Variable: $BITRISE_MAPPING_PATH_LIST (value: %s)", value)
	}
}
