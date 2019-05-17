package main

import (
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"sort"
	"testing"
	"time"
)

func Test_findArtifacts(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "glob-test")
	if err != nil {
		t.Errorf("setup: failed to create temp dir, error: %s", err)
	}
	defer func() {

		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("failed to remove temp dir, error: %s", err)
		}
	}()

	type args struct {
		nameInclude []string
		nameExclude []string
	}
	tests := []struct {
		name      string
		args      args
		filePaths []string
		want      []string
		wantErr   bool
	}{
		{
			name: "Inc: 1 ext, Excl: none",
			args: args{
				nameInclude: []string{"*.apk"},
				nameExclude: []string{""},
			},
			filePaths: []string{"test.apk"},
			want:      []string{"test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 1",
			args: args{
				nameInclude: []string{"*.apk"},
				nameExclude: []string{"*.aab"},
			},
			filePaths: []string{"test.apk", "test.aab"},
			want:      []string{"test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 0, Nested",
			args: args{
				nameInclude: []string{"*.apk"},
				nameExclude: []string{""},
			},
			filePaths: []string{"a/test.apk"},
			want:      []string{"a/test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: no path match, Nested",
			args: args{
				nameInclude: []string{"*.apk"},
				nameExclude: []string{"unaligned*.apk"},
			},
			filePaths: []string{"a/test.apk", "a/unaligned-test.apk"},
			want:      []string{"a/test.apk", "a/unaligned-test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 1, Nested",
			args: args{
				nameInclude: []string{"*.apk"},
				nameExclude: []string{"*unaligned*.apk"},
			},
			filePaths: []string{"a/test.apk", "a/unaligned-test.apk"},
			want:      []string{"a/test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 2, Nested",
			args: args{
				nameInclude: []string{"*.apk"},
				nameExclude: []string{"*unaligned*.apk", "*Test*.apk"},
			},
			filePaths: []string{"a/test.apk", "a/unaligned-test.apk", "a/Test-app.apk"},
			want:      []string{"a/test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 2, Nested, path in include",
			args: args{
				nameInclude: []string{"*/b/*.apk"},
				nameExclude: []string{"*unaligned*.apk", "*Test*.apk"},
			},
			filePaths: []string{"a/b/test.apk", "a/b/unaligned-test.apk", "a/b/Test-app.apk"},
			want:      []string{"a/b/test.apk"},
			wantErr:   false,
		},
		{
			name: "Inc: 1 ext, Excl: 1, Nested, path in include, path in exclude",
			args: args{
				nameInclude: []string{"*/b/*.apk"},
				nameExclude: []string{"*/c/*"},
			},
			filePaths: []string{"a/b/test.apk", "a/c/unaligned-test.apk", "a/c/Test-app.apk"},
			want:      []string{"a/b/test.apk"},
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupFiles := func(tempDir string, filePaths []string) string {
				currentTestDir, err := ioutil.TempDir(tempDir, "")
				if err != nil {
					t.Errorf("setup: failed to create temp dir, error: %s", err)
				}
				for _, partialFilePath := range filePaths {
					dirPath := path.Join(currentTestDir, path.Dir(partialFilePath))
					if err := os.MkdirAll(dirPath, 0700); err != nil {
						t.Errorf("setup: failed to create directory (%s), error: %s", dirPath, err)
					}

					filePath := path.Join(currentTestDir, partialFilePath)
					if err := ioutil.WriteFile(filePath, nil, 0600); err != nil {
						t.Errorf("setup: failed to create file (%s), error: %s", filePath, err)
					}
				}
				return currentTestDir
			}
			startTime := time.Now()
			currentTestDir := setupFiles(tempDir, tt.filePaths)

			got, err := findArtifacts(currentTestDir, startTime, tt.args.nameInclude, tt.args.nameExclude)

			// got, err := find2(currentTestDir, tt.args.nameInclude, tt.args.nameExclude)
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
