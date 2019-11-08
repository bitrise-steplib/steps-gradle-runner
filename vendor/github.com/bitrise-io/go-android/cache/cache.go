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
	if cacheLevel != LevelNone {
		gradleCache := cache.New()
		homeDir := pathutil.UserHomeDir()

		projectRoot, err := filepath.Abs(projectRoot)
		if err != nil {
			return fmt.Errorf("cache collection skipped: failed to determine project root path")
		}

		lockFilePath := filepath.Join(projectRoot, "gradle.deps")

		excludePths := []string{
			"~/.gradle/**",
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

		var includePths []string

		if cacheLevel == LevelAll || cacheLevel == LevelDeps {
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
				return fmt.Errorf("dependency map generation skipped: failed to collect dependencies")
			}
			if err := fileutil.WriteStringToFile(lockFilePath, lockfileContent); err != nil {
				return fmt.Errorf("dependency map generation skipped: failed to write lockfile, error: %s", err)
			}

			includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".gradle"), lockFilePath))
			includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".kotlin"), lockFilePath))
			includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".m2"), lockFilePath))
		}

		if cacheLevel == LevelAll {
			includePths = append(includePths, fmt.Sprintf("%s -> %s", filepath.Join(homeDir, ".android", "build-cache"), lockFilePath))

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
				return fmt.Errorf("cache collection skipped: failed to determine cache paths")
			}
		}
		gradleCache.IncludePath(strings.Join(includePths, "\n"))
		gradleCache.ExcludePath(strings.Join(excludePths, "\n"))

		if err := gradleCache.Commit(); err != nil {
			return fmt.Errorf("cache collection skipped: failed to commit cache paths")
		}
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