package cache

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitrise-io/go-steputils/cache"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
)

// Level defines the extent to which caching should be used.
// - LevelNone: no caching
// - LevelDeps: only dependencies will be cached
// - LevelAll: caching will include gradle and android build cache
type Level string

// Cache level
const (
	LevelNone = Level("none")
	LevelDeps = Level("only_deps")
	LevelAll  = Level("all")
)

// Collect walks the directory tree underneath projectRoot and registers matching
// paths for caching based on the value of cacheLevel. Returns an error if there
// was an underlying error that would lead to a corrupted cache file, otherwise
// the given path is skipped.
func Collect(projectRoot string, cacheLevel Level) error {
	if cacheLevel == LevelNone {
		return nil
	}

	gradleCache := cache.New()

	homeDir := pathutil.UserHomeDir()

	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("cache collection skipped: failed to determine project root path")
	}

	includePths, err := collectIncludePaths(homeDir, projectRoot, cacheLevel)
	if err != nil {
		return err
	}

	excludePths := collectExcludePaths(homeDir, projectRoot)

	gradleCache.IncludePath(strings.Join(includePths, "\n"))
	gradleCache.ExcludePath(strings.Join(excludePths, "\n"))

	if err := gradleCache.Commit(); err != nil {
		return fmt.Errorf("failed to commit cache paths: %s", err)
	}

	return nil
}

func collectIncludePaths(homeDir, projectDir string, cacheLevel Level) ([]string, error) {
	var includePths []string

	lockFilePath := filepath.Join(projectDir, "gradle.deps")

	lockfileContent := ""
	if err := filepath.Walk(projectDir, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk %s: %s", path, err)
		}

		if f.IsDir() || strings.Contains(path, "node_modules") {
			return nil
		}

		if !strings.HasSuffix(f.Name(), ".gradle") && !strings.HasSuffix(f.Name(), ".gradle.kts") && f.Name() != "gradlew-wrapper.properties" {
			return nil
		}

		md5Hash, err := computeMD5String(path)
		if err != nil {
			log.Warnf("Failed to compute MD5 hash of %s: %s", path, err)
			return nil
		}

		lockfileContent += md5Hash

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to create cache indicator file: %s", err)
	}
	if err := fileutil.WriteStringToFile(lockFilePath, lockfileContent); err != nil {
		return nil, fmt.Errorf("failed to write indicator file: %s", err)
	}

	includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".gradle"), lockFilePath))
	includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".kotlin"), lockFilePath))
	includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".m2"), lockFilePath))

	if cacheLevel == LevelAll {
		includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".android", "build-cache"), lockFilePath))

		if err := filepath.Walk(projectDir, func(path string, f os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("failed to walk %s: %s", path, err)
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
			return nil, fmt.Errorf("failed to collect build cache: %s", err)
		}
	}

	return includePths, nil
}

func computeMD5String(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Errorf("Failed to close %s: %s", filePath, err)
		}
	}()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func collectExcludePaths(homeDir, projectDir string) []string {
	excludePths := []string{
		"!~/.gradle/daemon/*/daemon-*.out.log", // excludes Gradle daemon logs, like: ~/.gradle/daemon/6.1.1/daemon-3122.out.log
		"~/.android/build-cache/**",
		"*.lock",
		"*.bin",
		"*/build/*.json",
		"*/build/*.html",
		"*/build/*.xml",
		"*/build/*.properties",
		"*/build/*/zip-cache/*",
		"*.log",
		"*.txt",
		"*.rawproto",
		"!*.ap_",
		"!*.apk",
	}

	ver, err := projectGradleVersion(projectDir)
	if err != nil {
		log.Warnf("Failed to get project gradle version: %s", err)
		return nil
	}

	{
		gradleUserHome := filepath.Join(homeDir, ".gradle")
		exist, err := pathutil.IsPathExists(gradleUserHome)
		if err != nil {
			log.Warnf("Failed to check if gradle user home dir (%s) exists: %s", gradleUserHome, err)
			return nil
		}
		if !exist {
			log.Warnf("Gradle user home dir (%s) does not exist", gradleUserHome)
			return nil
		}

		excludes, err := gradleUserHomeExcludePaths(gradleUserHome, ver)
		if err != nil {
			log.Warnf("Failed to collect gradle user home exclude paths: %s", err)
			return nil
		}

		excludePths = append(excludePths, excludes...)
	}

	{
		excludes, err := projectGradleExcludePaths(projectDir, ver)
		if err != nil {
			log.Warnf("Failed to collect project gradle exclude paths: %s", err)
			return nil
		}

		excludePths = append(excludePths, excludes...)

	}

	return excludePths
}
