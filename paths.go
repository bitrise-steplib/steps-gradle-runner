package main

import (
	"fmt"
	"path/filepath"

	"github.com/bitrise-io/go-utils/v2/pathutil"
)

func resolveGradlewPath(buildRootDir, gradlewPath string) (string, error) {
	buildRootAbs, err := filepath.Abs(buildRootDir)
	if err != nil {
		return "", fmt.Errorf("can't get absolute path for build_root_directory (%s): %w", buildRootDir, err)
	}

	pathChecker := pathutil.NewPathChecker()
	if exist, err := pathChecker.IsPathExists(buildRootAbs); err != nil {
		return "", fmt.Errorf("failed to check if build_root_directory exists at: %s: %w", buildRootAbs, err)
	} else if !exist {
		return "", fmt.Errorf("build_root_directory does not exist at: %s", buildRootAbs)
	}

	var resolvedGradlewPath string
	if filepath.IsAbs(gradlewPath) {
		resolvedGradlewPath = gradlewPath
	} else {
		resolvedGradlewPath = filepath.Clean(filepath.Join(buildRootAbs, gradlewPath))
	}

	if exist, err := pathChecker.IsPathExists(resolvedGradlewPath); err != nil {
		return "", fmt.Errorf("failed to check if gradlew exists at: %s: %w", resolvedGradlewPath, err)
	} else if !exist {
		return "", fmt.Errorf("gradlew does not exist at: %s", resolvedGradlewPath)
	}

	return resolvedGradlewPath, nil
}
