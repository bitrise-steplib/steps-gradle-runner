package gradle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/ryanuber/go-glob"
)

// Project ...
type Project struct {
	location   string
	monoRepo   bool
	cmdFactory command.Factory
	logger     log.Logger
}

// NewProject ...
func NewProject(location string, cmdFactory command.Factory, logger log.Logger) (Project, error) {
	var err error
	location, err = filepath.Abs(location)
	if err != nil {
		return Project{}, err
	}

	buildGradleFound, err := pathutil.IsPathExists(filepath.Join(location, "build.gradle"))
	if err != nil {
		return Project{}, err
	}

	buildGradleKtsFound, err := pathutil.IsPathExists(filepath.Join(location, "build.gradle.kts"))
	if err != nil {
		return Project{}, err
	}

	if !buildGradleFound && !buildGradleKtsFound {
		return Project{}, fmt.Errorf("no build.gradle or build.gradle.kts file found in (%s)", location)
	}

	if location == "/" {
		return Project{location: location, monoRepo: false, cmdFactory: cmdFactory}, nil
	}

	root := filepath.Join(location, "..")

	entries, err := os.ReadDir(root)
	if err != nil {
		return Project{}, fmt.Errorf("failed to read entries of %s: %w", root, err)
	}

	projectsCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			if buildGradleExists, err := pathutil.IsPathExists(filepath.Join(root, entry.Name(), "build.gradle")); err != nil {
				return Project{}, err
			} else if buildGradleKtsExists, err := pathutil.IsPathExists(filepath.Join(root, entry.Name(), "build.gradle.kts")); err != nil {
				return Project{}, err
			} else if buildGradleExists || buildGradleKtsExists {
				projectsCount++
			}
		}
	}

	return Project{
		location:   location,
		monoRepo:   projectsCount > 1,
		cmdFactory: cmdFactory,
		logger:     logger,
	}, nil
}

// GetTask ...
func (proj Project) GetTask(name string) *Task {
	return NewTask(name, proj, proj.logger)
}

// FindArtifacts ...
func (proj Project) FindArtifacts(generatedAfter time.Time, pattern string, includeModuleInName bool) ([]Artifact, error) {
	var a []Artifact
	return a, filepath.Walk(proj.location, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			proj.logger.Warnf("failed to walk path: %s", err)
			return nil
		}

		if info.IsDir() || !glob.Glob(pattern, path) {
			return nil
		}

		if info.ModTime().Before(generatedAfter) {
			proj.logger.Warnf("Ignoring %s because it was created by a previous step based on the file modification time", info.Name())
			return nil
		}

		name, err := proj.extractArtifactName(path, includeModuleInName)
		if err != nil {
			return err
		}

		a = append(a, Artifact{Name: name, Path: path})
		return nil
	})
}

// FindDirs ...
func (proj Project) FindDirs(generatedAfter time.Time, pattern string, includeModuleInName bool) ([]Artifact, error) {
	var a []Artifact
	return a, filepath.Walk(proj.location, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			proj.logger.Warnf("failed to walk path: %s", err)
			return nil
		}

		if info.ModTime().Before(generatedAfter) || !info.IsDir() || !glob.Glob(pattern, path) {
			return nil
		}

		name, err := proj.extractArtifactName(path, includeModuleInName)
		if err != nil {
			return err
		}

		a = append(a, Artifact{Name: name, Path: path})
		return nil
	})
}

func (proj Project) extractArtifactName(path string, includeModuleInName bool) (string, error) {
	relPath, err := filepath.Rel(proj.location, path)
	if err != nil {
		return "", err
	}

	fileName := filepath.Base(relPath)

	if includeModuleInName {
		fileName = strings.Split(relPath, "/")[0] + "-" + fileName
	}

	if proj.monoRepo {
		split := strings.Split(proj.location, "/")
		prefix := split[len(split)-1]
		if prefix != "" {
			fileName = prefix + "-" + fileName
		}
	}

	return fileName, nil
}
