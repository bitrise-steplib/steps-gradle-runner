package gradle

import (
	"path/filepath"
	"strings"
)

// ArtifactVariant identifies the build variant an artifact belongs to: the
// module together with the merged Gradle variant name (for example
// "demoRelease"). It is only compared between artifacts, so its exact textual
// form is not a stable contract; an app artifact and the mapping.txt built for
// the same variant just need to produce equal values. The module is included so
// artifacts from different modules that share a variant name (for example both
// producing "release") do not collide.
type ArtifactVariant struct {
	Module  string
	Variant string
}

// Variant reports the build variant of the artifact, derived from its path in
// the Gradle build output tree. ok is false when the path is not a recognised
// output or mapping path.
func (artifact Artifact) Variant() (variant ArtifactVariant, ok bool) {
	return VariantFromPath(artifact.Path)
}

// VariantFromPath reports the build variant encoded in a Gradle build-output
// path. It reconciles the two directory shapes the Android Gradle Plugin uses:
// APKs split the variant into flavor and build type
// (build/outputs/apk/demo/release/), while AABs and mapping files use a single
// merged directory (build/outputs/bundle/demoRelease/,
// build/outputs/mapping/demoRelease/). Joining the APK's segments yields the
// same merged name, so an APK and its mapping resolve to equal ArtifactVariants.
//
// It anchors on the "outputs" (or "intermediates") directory and reads the
// artifact kind that follows it, so a flavor directory that happens to be named
// "apk"/"bundle"/"mapping" is not mistaken for the kind marker. ok is false when
// the path is not a recognised output or mapping path.
func VariantFromPath(path string) (variant ArtifactVariant, ok bool) {
	segments := strings.Split(filepath.ToSlash(path), "/")

	for i := 0; i+1 < len(segments); i++ {
		if segments[i] != "outputs" && segments[i] != "intermediates" {
			continue
		}

		module := moduleFromSegments(segments[:i])
		switch segments[i+1] {
		case "apk", "bundle":
			// The variant is every directory between the kind and the file name.
			variantSegments := segments[i+2 : len(segments)-1]
			if len(variantSegments) == 0 {
				return ArtifactVariant{}, false
			}
			return ArtifactVariant{Module: module, Variant: mergeVariantSegments(variantSegments)}, true
		case "mapping":
			// The variant is the single directory right after "mapping",
			// ignoring any deeper "minify..." subdirectory.
			if i+2 <= len(segments)-2 {
				return ArtifactVariant{Module: module, Variant: segments[i+2]}, true
			}
			return ArtifactVariant{}, false
		}
		// Some other "outputs" child (e.g. logs), or a directory named "outputs"
		// higher up the path: keep scanning for the real marker.
	}

	return ArtifactVariant{}, false
}

// moduleFromSegments returns the module directory (the segment right before
// "build"), or "" when the path has no "build" segment.
func moduleFromSegments(segments []string) string {
	for i := len(segments) - 1; i >= 1; i-- {
		if segments[i] == "build" {
			return segments[i-1]
		}
	}
	return ""
}

// mergeVariantSegments joins variant directory segments into the single merged
// Gradle variant name: the first segment as-is, each following one capitalised
// on its first letter, so ["demo", "release"] and ["demoRelease"] both yield
// "demoRelease".
func mergeVariantSegments(segments []string) string {
	var builder strings.Builder
	for i, segment := range segments {
		if i == 0 {
			builder.WriteString(segment)
			continue
		}
		builder.WriteString(capitalizeFirst(segment))
	}
	return builder.String()
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
