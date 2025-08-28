package main

import (
	"os"
	"path"
	"reflect"
	"sort"
	"testing"
)

func Test_findArtifacts(t *testing.T) {
	tests := []struct {
		name      string
		patterns  filePatterns
		filePaths []string
		want      []string
		wantErr   bool
	}{
		{
			name: "Inc: 1 ext, Excl: none",
			patterns: filePatterns{
				include: []string{"*.apk"},
				exclude: []string{""},
			},
			filePaths: []string{"test.apk"},
			want:      []string{"test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 1",
			patterns: filePatterns{
				include: []string{"*.apk"},
				exclude: []string{"*.aab"},
			},
			filePaths: []string{"test.apk", "test.aab"},
			want:      []string{"test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 0, Nested",
			patterns: filePatterns{
				include: []string{"*.apk"},
				exclude: []string{""},
			},
			filePaths: []string{"a/test.apk"},
			want:      []string{"a/test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: no path match, Nested",
			patterns: filePatterns{
				include: []string{"*.apk"},
				exclude: []string{"unaligned*.apk"},
			},
			filePaths: []string{"a/test.apk", "a/unaligned-test.apk"},
			want:      []string{"a/test.apk", "a/unaligned-test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 1, Nested",
			patterns: filePatterns{
				include: []string{"*.apk"},
				exclude: []string{"*unaligned*.apk"},
			},
			filePaths: []string{"a/test.apk", "a/unaligned-test.apk"},
			want:      []string{"a/test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 2, Nested",
			patterns: filePatterns{
				include: []string{"*.apk"},
				exclude: []string{"*unaligned*.apk", "*Test*.apk"},
			},
			filePaths: []string{"a/test.apk", "a/unaligned-test.apk", "a/Test-app.apk"},
			want:      []string{"a/test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 2, Nested, path in include",
			patterns: filePatterns{
				include: []string{"*/b/*.apk"},
				exclude: []string{"*unaligned*.apk", "*Test*.apk"},
			},
			filePaths: []string{"a/b/test.apk", "a/b/unaligned-test.apk", "a/b/Test-app.apk"},
			want:      []string{"a/b/test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 1, Nested, path in include, path in exclude",
			patterns: filePatterns{
				include: []string{"*/b/*.apk"},
				exclude: []string{"*/c/*"},
			},
			filePaths: []string{"a/b/test.apk", "a/c/unaligned-test.apk", "a/c/Test-app.apk"},
			want:      []string{"a/b/test.apk"},
			wantErr:   false,
		},
		{
			name: "Incl: 2, Nested, path in include",
			patterns: filePatterns{
				include: []string{"a/*.apk", "b/*.aab"},
				exclude: []string{},
			},
			filePaths: []string{"a/test.apk", "a.test.aab", "b/test.apk", "b/test.aab"},
			want:      []string{"a/test.apk", "b/test.aab"},
			wantErr:   false,
		},
		{
			name: "Empty include patterns",
			patterns: filePatterns{
				include: []string{},
				exclude: []string{},
			},
			filePaths: []string{"test.apk", "test.aab"},
			want:      nil,
			wantErr:   false,
		},
		{
			name: "No files match include pattern",
			patterns: filePatterns{
				include: []string{"*.jar"},
				exclude: []string{},
			},
			filePaths: []string{"test.apk", "test.aab"},
			want:      nil,
			wantErr:   false,
		},
		{
			name: "All files excluded",
			patterns: filePatterns{
				include: []string{"*.apk"},
				exclude: []string{"*.apk"},
			},
			filePaths: []string{"test.apk", "other.apk"},
			want:      nil,
			wantErr:   false,
		},
		{
			name: "Files with spaces and special characters",
			patterns: filePatterns{
				include: []string{"*.apk"},
				exclude: []string{},
			},
			filePaths: []string{"app release-v1.0.apk", "test (debug).apk", "café-release.apk"},
			want:      []string{"app release-v1.0.apk", "test (debug).apk", "café-release.apk"},
			wantErr:   false,
		},
		{
			name: "Deep nested directories",
			patterns: filePatterns{
				include: []string{"*.apk", "**/*.apk"},
				exclude: []string{},
			},
			filePaths: []string{"a/b/c/d/e/f/deep.apk", "shallow.apk"},
			want:      []string{"a/b/c/d/e/f/deep.apk", "shallow.apk"},
			wantErr:   false,
		},
		{
			name: "Mixed file types with multiple patterns",
			patterns: filePatterns{
				include: []string{"*.apk", "*.aab", "*.jar"},
				exclude: []string{"*test*"},
			},
			filePaths: []string{"app.apk", "app.aab", "lib.jar", "test.apk", "test.aab", "readme.txt"},
			want:      []string{"app.apk", "app.aab", "lib.jar"},
			wantErr:   false,
		},
		{
			name: "Overlapping include and exclude patterns",
			patterns: filePatterns{
				include: []string{"*release*", "*debug*"},
				exclude: []string{"*unaligned*"},
			},
			filePaths: []string{"app-release.apk", "app-debug.apk", "app-release-unaligned.apk", "app-debug-unaligned.apk", "other.apk"},
			want:      []string{"app-release.apk", "app-debug.apk"},
			wantErr:   false,
		},
		{
			name: "Complex directory patterns",
			patterns: filePatterns{
				include: []string{"build/outputs/**/*.apk", "app/build/**/*.aab"},
				exclude: []string{"**/intermediates/**/*", "**/tmp/**/*"},
			},
			filePaths: []string{
				"build/outputs/apk/debug/app.apk",
				"build/outputs/apk/release/app.apk", 
				"app/build/outputs/bundle/release/app.aab",
				"build/intermediates/apk/debug/temp.apk",
				"build/tmp/cache/temp.apk",
				"other/app.apk",
			},
			want: []string{"build/outputs/apk/debug/app.apk", "build/outputs/apk/release/app.apk", "app/build/outputs/bundle/release/app.aab"},
			wantErr: false,
		},
		{
			name: "Empty exclude patterns with empty strings",
			patterns: filePatterns{
				include: []string{"*.apk"},
				exclude: []string{"", "", ""},
			},
			filePaths: []string{"test.apk", "other.apk"},
			want:      []string{"test.apk", "other.apk"},
			wantErr:   false,
		},
		{
			name: "Default APK/AAB include patterns from step.yml",
			patterns: filePatterns{
				include: []string{"*.apk", "*.aab"},
				exclude: []string{},
			},
			filePaths: []string{"app-release.apk", "app-debug.apk", "app.aab", "lib.jar", "README.md"},
			want:      []string{"app-release.apk", "app-debug.apk", "app.aab"},
			wantErr:   false,
		},
		{
			name: "Default APK/AAB exclude patterns from step.yml",
			patterns: filePatterns{
				include: []string{"*.apk", "*.aab"},
				exclude: []string{"*unaligned.apk", "*Test*.apk", "*/intermediates/*"},
			},
			filePaths: []string{
				"app-release.apk",
				"app-debug-unaligned.apk",
				"app-androidTest.apk",
				"build/intermediates/apk/debug/temp.apk",
				"app.aab",
			},
			want: []string{"app-release.apk", "app.aab"},
			wantErr: false,
		},
		{
			name: "Default test APK include pattern from step.yml",
			patterns: filePatterns{
				include: []string{"*Test*.apk"},
				exclude: []string{},
			},
			filePaths: []string{
				"app-release.apk",
				"app-androidTest.apk", 
				"app-debugAndroidTest.apk",
				"Test-runner.apk",
				"app.aab",
			},
			want: []string{"app-androidTest.apk", "app-debugAndroidTest.apk", "Test-runner.apk"},
			wantErr: false,
		},
		{
			name: "Default mapping file include pattern from step.yml",
			patterns: filePatterns{
				include: []string{"*/mapping.txt"},
				exclude: []string{},
			},
			filePaths: []string{
				"app/build/outputs/mapping/release/mapping.txt",
				"build/outputs/mapping/debug/mapping.txt",
				"mapping.txt",
				"other.txt",
				"app.apk",
			},
			want: []string{"app/build/outputs/mapping/release/mapping.txt", "build/outputs/mapping/debug/mapping.txt"},
			wantErr: false,
		},
		{
			name: "Default mapping file exclude pattern from step.yml", 
			patterns: filePatterns{
				include: []string{"*/mapping.txt"},
				exclude: []string{"*/tmp/*"},
			},
			filePaths: []string{
				"app/build/outputs/mapping/release/mapping.txt", 
				"build/tmp/mapping/mapping.txt",
				"some/tmp/cache/mapping.txt",
			},
			want: []string{"app/build/outputs/mapping/release/mapping.txt"},
			wantErr: false,
		},
		{
			name: "Real-world Android build structure with defaults",
			patterns: filePatterns{
				include: []string{"*.apk", "*.aab"},
				exclude: []string{"*unaligned.apk", "*Test*.apk", "*/intermediates/*"},
			},
			filePaths: []string{
				"app/build/outputs/apk/debug/app-debug.apk",
				"app/build/outputs/apk/release/app-release.apk", 
				"app/build/outputs/apk/release/app-release-unaligned.apk",
				"app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk",
				"app/build/outputs/bundle/release/app-release.aab",
				"app/build/intermediates/apk_ide_redirect/debug/redirect.apk",
				"build/intermediates/merged_manifests/debug/temp.apk",
			},
			want: []string{
				"app/build/outputs/apk/debug/app-debug.apk",
				"app/build/outputs/apk/release/app-release.apk",
				"app/build/outputs/bundle/release/app-release.aab",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupFiles := func(filePaths []string) string {
				currentTestDir := t.TempDir()
				for _, partialFilePath := range filePaths {
					dirPath := path.Join(currentTestDir, path.Dir(partialFilePath))
					if err := os.MkdirAll(dirPath, 0700); err != nil {
						t.Errorf("setup: failed to create directory (%s), error: %s", dirPath, err)
					}

					filePath := path.Join(currentTestDir, partialFilePath)
					if err := os.WriteFile(filePath, nil, 0600); err != nil {
						t.Errorf("setup: failed to create file (%s), error: %s", filePath, err)
					}
				}
				return currentTestDir
			}
			currentTestDir := setupFiles(tt.filePaths)

			got, err := findArtifacts(currentTestDir, tt.patterns)
			if (err != nil) != tt.wantErr {
				t.Errorf("findArtifacts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for i := range tt.want {
				tt.want[i] = path.Join(currentTestDir, tt.want[i])
			}
			sort.Strings(got)
			sort.Strings(tt.want)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("findArtifacts() = %v, want %v", got, tt.want)
			}
		})
	}
}
