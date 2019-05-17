package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/bitrise-io/go-utils/log"
	"github.com/ryanuber/go-glob"
)

func findArtifacts(searchDir string, generatedAfter time.Time, includePatterns []string, excludePatterns []string) ([]string, error) {
	var artifacts []string
	return artifacts, filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Warnf("failed to walk path: %s", err)
			return err
		}

		if info.IsDir() || info.ModTime().Before(generatedAfter) {
			return nil
		}

		includeMatch := false
		for _, includePattern := range includePatterns {
			if glob.Glob(includePattern, path) {
				includeMatch = true
				break
			}
		}
		if !includeMatch {
			return nil
		}

		for _, excludePattern := range excludePatterns {
			if glob.Glob(excludePattern, path) {
				return nil
			}
		}

		artifacts = append(artifacts, path)
		return nil
	})
}
