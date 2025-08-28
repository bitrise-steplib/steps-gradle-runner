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

		// Convert absolute path to relative path for pattern matching.
		// According to the docs of `fs.WalkFunc`, the "relativeness" of `path` is determined by the `searchDir`.
		// That is, if we call `filepath.Walk` with an absolute path, the `path` will be absolute as well.
		relPath, err := filepath.Rel(searchDir, path)
		if err != nil {
			log.Warnf("failed to get relative path for %s: %s", path, err)
			return nil
		}

		includeMatch := false
		for _, includePattern := range patterns.include {
			if glob.Glob(includePattern, relPath) {
				includeMatch = true
				break
			}
		}
		if !includeMatch {
			return nil
		}

		for _, excludePattern := range patterns.exclude {
			if excludePattern != "" && glob.Glob(excludePattern, relPath) {
				return nil
			}
		}

		artifacts = append(artifacts, path)
		return nil
	})
}
