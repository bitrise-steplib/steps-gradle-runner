package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/cmdex"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/retry"
	log "github.com/bitrise-io/steps-gradle-runner/logger"
	"github.com/kballard/go-shellquote"
)

// ConfigsModel ...
type ConfigsModel struct {
	// Gradle Inputs
	GradleFile               string
	GradleTasks              string
	GradlewPath              string
	GradleOptions            string
	ApkFileIncludeFilter     string
	ApkFileExcludeFilter     string
	MappingFileIncludeFilter string
	MappingFileExcludeFilter string
	// Other configs
	DeployDir string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		GradleFile:               os.Getenv("gradle_file"),
		GradleTasks:              os.Getenv("gradle_task"),
		GradlewPath:              os.Getenv("gradlew_path"),
		GradleOptions:            os.Getenv("gradle_options"),
		ApkFileIncludeFilter:     os.Getenv("apk_file_include_filter"),
		ApkFileExcludeFilter:     os.Getenv("apk_file_exclude_filter"),
		MappingFileIncludeFilter: os.Getenv("mapping_file_include_filter"),
		MappingFileExcludeFilter: os.Getenv("mapping_file_exclude_filter"),
		//
		DeployDir: os.Getenv("BITRISE_DEPLOY_DIR"),
	}
}

func (configs ConfigsModel) print() {

	log.Info("Configs:")
	log.Detail("- GradlewPath: %s", configs.GradlewPath)
	log.Detail("- GradleFile: %s", configs.GradleFile)
	log.Detail("- GradleTasks: %s", configs.GradleTasks)
	log.Detail("- GradleOptions: %s", configs.GradleOptions)
	log.Detail("- ApkFileIncludeFilter: %s", configs.ApkFileIncludeFilter)
	log.Detail("- ApkFileExcludeFilter: %s", configs.ApkFileExcludeFilter)
	log.Detail("- MappingFileIncludeFilter: %s", configs.MappingFileIncludeFilter)
	log.Detail("- MappingFileExcludeFilter: %s", configs.MappingFileExcludeFilter)
	log.Detail("- DeployDir: %s", configs.DeployDir)
}

func (configs ConfigsModel) validate() (string, error) {
	// required
	if configs.GradleFile == "" {
		return "", errors.New("No GradleFile parameter specified!")
	}
	if exist, err := pathutil.IsPathExists(configs.GradleFile); err != nil {
		return "", fmt.Errorf("Failed to check if GradleFile exist at: %s, error: %s", configs.GradleFile, err)
	} else if !exist {
		return "", fmt.Errorf("GradleFile not exist at: %s", configs.GradleFile)
	}

	if configs.GradleTasks == "" {
		return "", errors.New("No GradleTask parameter specified!")
	}

	if configs.GradlewPath == "" {
		explanation := `
Using a Gradle Wrapper (gradlew) is required, as the wrapper is what makes sure
that the right Gradle version is installed and used for the build.

You can find more information about the Gradle Wrapper (gradlew),
and about how you can generate one (if you would not have one already
in the official guide at: https://docs.gradle.org/current/userguide/gradle_wrapper.html`

		return explanation, errors.New("No GradlewPath parameter specified!")
	}
	if exist, err := pathutil.IsPathExists(configs.GradlewPath); err != nil {
		return "", fmt.Errorf("Failed to check if GradlewPath exist at: %s, error: %s", configs.GradlewPath, err)
	} else if !exist {
		return "", fmt.Errorf("GradlewPath not exist at: %s", configs.GradlewPath)
	}

	return "", nil
}

func runGradleTask(gradleTool, buildFile, tasks, options string) error {
	optionSlice, err := shellquote.Split(options)
	if err != nil {
		return err
	}

	taskSlice, err := shellquote.Split(tasks)
	if err != nil {
		return err
	}

	cmdSlice := []string{gradleTool, "--build-file", buildFile}
	cmdSlice = append(cmdSlice, taskSlice...)
	cmdSlice = append(cmdSlice, optionSlice...)

	log.Detail(cmdex.PrintableCommandArgs(false, cmdSlice))
	fmt.Println()

	cmd := cmdex.NewCommand(cmdSlice[0], cmdSlice[1:]...)
	cmd.SetStdout(os.Stdout)
	cmd.SetStderr(os.Stderr)
	return cmd.Run()
}

func find(dir, nameInclude, nameExclude string) ([]string, error) {
	cmdSlice := []string{"find", dir}

	if nameInclude != "" {
		cmdSlice = append(cmdSlice, "-name", nameInclude)
	}

	if nameExclude != "" {
		cmdSlice = append(cmdSlice, "!", "-name", nameExclude)
	}

	log.Detail(cmdex.PrintableCommandArgs(false, cmdSlice))

	out, err := cmdex.NewCommand(cmdSlice[0], cmdSlice[1:]...).RunAndReturnTrimmedOutput()
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
			log.Warn("  Retrying...")
		}

		pth, pathErr := createDeployPth(deployDir, retryApkName)
		if pathErr != nil {
			log.Warn("  %d attempt failed:", attempt+1)
			log.Detail(pathErr.Error())
		}

		t := time.Now()
		retryApkName = baseName + t.Format("20060102150405") + ext
		deployPth = pth

		return pathErr
	})

	return deployPth, err
}

func exportEnvironmentWithEnvman(keyStr, valueStr string) error {
	cmd := cmdex.NewCommand("envman", "add", "--key", keyStr)
	cmd.SetStdin(strings.NewReader(valueStr))
	return cmd.Run()
}

func main() {
	configs := createConfigsModelFromEnvs()
	configs.print()
	if explanation, err := configs.validate(); err != nil {
		fmt.Println()
		log.Error("Issue with input: %s", err)
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
		log.Fail("Failed to add executable permission on gradlew file (%s), error: %s", configs.GradlewPath, err)
	}

	log.Info("Running gradle task...")
	if err := runGradleTask(configs.GradlewPath, configs.GradleFile, configs.GradleTasks, configs.GradleOptions); err != nil {
		log.Fail("Gradle task failed, error: %s", err)
	}

	// Move apk files
	log.Info("Move apk files...")
	apkFiles, err := find(".", configs.ApkFileIncludeFilter, configs.ApkFileExcludeFilter)
	if err != nil {
		log.Fail("Failed to find apk files, error: %s", err)
	}

	if len(apkFiles) == 0 {
		log.Warn("No apk matched the filters")
	}

	lastCopiedApkFile := ""
	for _, apkFile := range apkFiles {
		ext := filepath.Ext(apkFile)
		baseName := filepath.Base(apkFile)
		baseName = strings.TrimSuffix(baseName, ext)

		deployPth, err := findDeployPth(configs.DeployDir, baseName, ext)
		if err != nil {
			log.Fail("Failed to create apk deploy path, error: %s", err)
		}

		log.Detail("copy %s to %s", apkFile, deployPth)
		cmdex.CopyFile(apkFile, deployPth)

		lastCopiedApkFile = deployPth
	}

	if lastCopiedApkFile != "" {
		if err := exportEnvironmentWithEnvman("BITRISE_APK_PATH", lastCopiedApkFile); err != nil {
			log.Fail("Failed to export enviroment (BITRISE_APK_PATH), error: %s", err)
		}
		log.Done("The apk path is now available in the Environment Variable: $BITRISE_APK_PATH (value: %s)", lastCopiedApkFile)
	}

	// Move mapping files
	log.Info("Move mapping files...")
	mappingFiles, err := find(".", configs.MappingFileIncludeFilter, configs.MappingFileExcludeFilter)
	if err != nil {
		log.Fail("Failed to find mapping files, error: %s", err)
	}

	if len(mappingFiles) == 0 {
		log.Detail("No mapping file matched the filters")
	}

	lastCopiedMappingFile := ""
	for _, mappingFile := range mappingFiles {
		ext := filepath.Ext(mappingFile)
		baseName := filepath.Base(mappingFile)
		baseName = strings.TrimSuffix(baseName, ext)

		deployPth, err := findDeployPth(configs.DeployDir, baseName, ext)
		if err != nil {
			log.Fail("Failed to create mapping deploy path, error: %s", err)
		}

		log.Detail("copy %s to %s", mappingFile, deployPth)
		cmdex.CopyFile(mappingFile, deployPth)

		lastCopiedMappingFile = deployPth
	}

	if lastCopiedMappingFile != "" {
		if err := exportEnvironmentWithEnvman("BITRISE_MAPPING_PATH", lastCopiedMappingFile); err != nil {
			log.Fail("Failed to export enviroment (BITRISE_MAPPING_PATH), error: %s", err)
		}
		log.Done("The mapping path is now available in the Environment Variable: $BITRISE_MAPPING_PATH (value: %s)", lastCopiedMappingFile)
	}
}
