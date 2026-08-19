package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNameSignal covers what the bare wrapper error hides: a gradle process
// killed by a signal exits with code -1, which reads like a gradle failure. The
// naming applies in both output modes, so it lives in one helper.
func TestNameSignal(t *testing.T) {
	killErr := exec.Command("sh", "-c", "kill -9 $$").Run()
	var execErr *exec.ExitError
	require.True(t, errors.As(killErr, &execErr), "setup: expected an *exec.ExitError, got %v", killErr)
	require.Equal(t, -1, execErr.ExitCode(), "setup: a signalled process should report exit code -1")

	named := nameSignal(killErr)
	require.Contains(t, named.Error(), "gradle terminated abnormally")
	require.Contains(t, named.Error(), "killed")
	require.ErrorIs(t, named, killErr, "the original error must stay unwrappable")

	require.NoError(t, nameSignal(nil))

	plain := exec.Command("sh", "-c", "exit 3").Run()
	require.Equal(t, plain, nameSignal(plain), "an ordinary non-zero exit passes through unchanged")
}

func TestResolveGradlewPath(t *testing.T) {
	tests := []struct {
		name         string
		buildRootDir string
		gradlewPath  string
		createFiles  []string
		expectedPath string
		expectedErr  bool
	}{
		{
			name:         "relative gradlew path",
			buildRootDir: "testdir",
			gradlewPath:  "gradlew",
			createFiles:  []string{"testdir/gradlew"},
			expectedPath: "testdir/gradlew",
			expectedErr:  false,
		},
		{
			name:         "relative path with dots",
			buildRootDir: "testdir/nested",
			gradlewPath:  "../gradlew",
			createFiles:  []string{"testdir/nested/", "testdir/gradlew"},
			expectedPath: "testdir/gradlew",
			expectedErr:  false,
		},
		{
			name:         "absolute gradlew path",
			buildRootDir: "testdir",
			gradlewPath:  "", // Will be set dynamically
			createFiles:  []string{"testdir/", "gradlew"},
			expectedPath: "gradlew",
			expectedErr:  false,
		},
		{
			name:         "build root directory does not exist",
			buildRootDir: "nonexistent",
			gradlewPath:  "gradlew",
			createFiles:  []string{},
			expectedPath: "",
			expectedErr:  true,
		},
		{
			name:         "gradlew file does not exist",
			buildRootDir: "testdir",
			gradlewPath:  "nonexistent-gradlew",
			createFiles:  []string{"testdir/"},
			expectedPath: "",
			expectedErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origDir, err := os.Getwd()
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.Chdir(origDir))
			}()

			workDir := t.TempDir()
			require.NoError(t, os.Chdir(workDir))

			for _, path := range tt.createFiles {
				fullPath := filepath.Join(workDir, path)
				if strings.HasSuffix(path, "/") {
					require.NoError(t, os.MkdirAll(fullPath, 0755))
				} else {
					require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
					require.NoError(t, os.WriteFile(fullPath, []byte("#!/bin/bash"), 0755))
				}
			}

			gradlewPath := tt.gradlewPath
			if gradlewPath == "" && tt.expectedPath != "" {
				gradlewPath = filepath.Join(workDir, tt.expectedPath)
			}

			result, err := resolveGradlewPath(tt.buildRootDir, gradlewPath)

			if tt.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.True(t, filepath.IsAbs(result), "result should be absolute path: %s", result)

			if tt.expectedPath != "" {
				expectedAbs := filepath.Join(workDir, tt.expectedPath)
				expectedResolved, err := filepath.EvalSymlinks(expectedAbs)
				require.NoError(t, err)
				resultResolved, err := filepath.EvalSymlinks(result)
				require.NoError(t, err)
				require.Equal(t, expectedResolved, resultResolved)
			}
		})
	}
}
