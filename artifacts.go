package main

import (
	"os"
	"path/filepath"

	"github.com/bitrise-io/go-utils/log"
	"github.com/ryanuber/go-glob"
)

type filePatterns struct {
	include []string
	exclude []string
}

func findArtifacts(searchDir string, patterns filePatterns) ([]string, error) {
	var artifacts []string
	return artifacts, filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Warnf("failed to walk path: %s", err)
			return err
		}

		if info.IsDir() {
			return nil
		}

		includeMatch := false
		for _, includePattern := range patterns.include {
			if glob.Glob(includePattern, path) {
				includeMatch = true
				break
			}
		}
		if !includeMatch {
			return nil
		}

		for _, excludePattern := range patterns.exclude {
			if glob.Glob(excludePattern, path) {
				return nil
			}
		}

		artifacts = append(artifacts, path)
		return nil
	})
}
