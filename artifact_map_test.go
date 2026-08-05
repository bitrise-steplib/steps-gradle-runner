package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bitrise-io/go-android/v2/gradle/artifactmap"
)

func TestDeployPaths(t *testing.T) {
	files := []artifactmap.File{
		{DeployPath: "/deploy/a.apk", SourcePath: "/src/app/build/outputs/apk/demo/release/a.apk"},
		{DeployPath: "/deploy/b.apk", SourcePath: "/src/app/build/outputs/apk/paid/release/b.apk"},
	}
	want := []string{"/deploy/a.apk", "/deploy/b.apk"}
	if got := deployPaths(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("deployPaths = %v, want %v", got, want)
	}
	if got := deployPaths(nil); len(got) != 0 {
		t.Fatalf("deployPaths(nil) = %v, want empty", got)
	}
}

// TestArtifactMapPairing locks the step-level expectation: files collected the
// way main() collects them (deploy path + gradle-runner source path) group into
// a map that pairs each variant's app files with that variant's mapping file,
// even after a name-collision rename in the deploy dir.
func TestArtifactMapPairing(t *testing.T) {
	deployDir := "/bitrise/deploy"
	apks := []artifactmap.File{
		{DeployPath: filepath.Join(deployDir, "app-demo-release.apk"), SourcePath: "/bitrise/src/app/build/outputs/apk/demo/release/app-demo-release.apk"},
		{DeployPath: filepath.Join(deployDir, "app-paid-release.apk"), SourcePath: "/bitrise/src/app/build/outputs/apk/paid/release/app-paid-release.apk"},
	}
	aabs := []artifactmap.File{
		{DeployPath: filepath.Join(deployDir, "app-demo-release.aab"), SourcePath: "/bitrise/src/app/build/outputs/bundle/demoRelease/app-demo-release.aab"},
	}
	mappings := []artifactmap.File{
		{DeployPath: filepath.Join(deployDir, "mapping.txt"), SourcePath: "/bitrise/src/app/build/outputs/mapping/demoRelease/mapping.txt"},
		// second mapping.txt got the collision-avoidance timestamp suffix on copy
		{DeployPath: filepath.Join(deployDir, "mapping20260805121530.txt"), SourcePath: "/bitrise/src/app/build/outputs/mapping/paidRelease/mapping.txt"},
	}

	m, warnings := artifactmap.Build(apks, aabs, mappings)

	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	want := map[string]artifactmap.Entry{
		"demoRelease": {
			Module:  "app",
			Mapping: "mapping.txt",
			AAB:     []string{"app-demo-release.aab"},
			APK:     []string{"app-demo-release.apk"},
		},
		"paidRelease": {
			Module:  "app",
			Mapping: "mapping20260805121530.txt",
			AAB:     []string{},
			APK:     []string{"app-paid-release.apk"},
		},
	}
	if !reflect.DeepEqual(m.Variants, want) {
		t.Fatalf("Variants = %+v, want %+v", m.Variants, want)
	}
}
