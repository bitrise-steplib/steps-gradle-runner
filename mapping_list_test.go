package main

import (
	"reflect"
	"testing"

	"github.com/bitrise-io/go-android/v2/gradle"
)

func TestAlignedMappingList(t *testing.T) {
	demoRelease := gradle.ArtifactVariant{Module: "app", Variant: "demoRelease"}
	prodRelease := gradle.ArtifactVariant{Module: "app", Variant: "prodRelease"}
	debug := gradle.ArtifactVariant{Module: "app", Variant: "debug"}

	app := func(path string, v gradle.ArtifactVariant) copiedApp {
		return copiedApp{deployPath: path, variant: v, hasVariant: true}
	}

	tests := []struct {
		name     string
		aabs     []copiedApp
		apks     []copiedApp
		mappings map[gradle.ArtifactVariant]string
		wantList []string
		wantOK   bool
	}{
		{
			name:     "aligns to AAB list when AABs present",
			aabs:     []copiedApp{app("/d/demo.aab", demoRelease), app("/d/prod.aab", prodRelease)},
			apks:     []copiedApp{app("/d/demo.apk", demoRelease)},
			mappings: map[gradle.ArtifactVariant]string{demoRelease: "/d/demo-mapping.txt", prodRelease: "/d/prod-mapping.txt"},
			wantList: []string{"/d/demo-mapping.txt", "/d/prod-mapping.txt"},
			wantOK:   true,
		},
		{
			name:     "falls back to APK list when no AABs",
			apks:     []copiedApp{app("/d/demo.apk", demoRelease), app("/d/prod.apk", prodRelease)},
			mappings: map[gradle.ArtifactVariant]string{demoRelease: "/d/demo-mapping.txt", prodRelease: "/d/prod-mapping.txt"},
			wantList: []string{"/d/demo-mapping.txt", "/d/prod-mapping.txt"},
			wantOK:   true,
		},
		{
			name:     "falls back to APK list when no AAB variant matched a mapping",
			aabs:     []copiedApp{app("/d/debug.aab", debug)},
			apks:     []copiedApp{app("/d/demo.apk", demoRelease)},
			mappings: map[gradle.ArtifactVariant]string{demoRelease: "/d/demo-mapping.txt"},
			wantList: []string{"/d/demo-mapping.txt"},
			wantOK:   true,
		},
		{
			name:     "empty placeholder keeps positions when a variant has no mapping",
			aabs:     []copiedApp{app("/d/demo.aab", demoRelease), app("/d/debug.aab", debug), app("/d/prod.aab", prodRelease)},
			mappings: map[gradle.ArtifactVariant]string{demoRelease: "/d/demo-mapping.txt", prodRelease: "/d/prod-mapping.txt"},
			wantList: []string{"/d/demo-mapping.txt", "", "/d/prod-mapping.txt"},
			wantOK:   true,
		},
		{
			name:     "no apps to align to",
			mappings: map[gradle.ArtifactVariant]string{demoRelease: "/d/demo-mapping.txt"},
			wantOK:   false,
		},
		{
			name:   "no mappings at all",
			aabs:   []copiedApp{app("/d/demo.aab", demoRelease)},
			wantOK: false,
		},
		{
			name:     "no variant matched any mapping",
			aabs:     []copiedApp{app("/d/demo.aab", demoRelease)},
			mappings: map[gradle.ArtifactVariant]string{prodRelease: "/d/prod-mapping.txt"},
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, ok := alignedMappingList(tt.aabs, tt.apks, tt.mappings)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if !reflect.DeepEqual(list, tt.wantList) {
				t.Fatalf("list = %v, want %v", list, tt.wantList)
			}
		})
	}
}
