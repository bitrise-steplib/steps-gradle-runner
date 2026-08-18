package artifactmap

import (
	"path/filepath"
	"strings"
)

// ArtifactVariant identifies the build variant an artifact belongs to: the
// module plus the merged Gradle variant name ("demoRelease"). Values are not a
// stable contract — an artifact and its mapping just need to compare equal;
// the module keeps same-named variants of different modules apart.
type ArtifactVariant struct {
	Module  string
	Variant string
}

// VariantFromPath reports the build variant encoded in a Gradle build-output
// path. It reconciles AGP's split (outputs/apk/demo/release/) and merged
// (outputs/{bundle,mapping}/demoRelease/) layouts, so an artifact and its
// mapping resolve to equal ArtifactVariants.
//
// Only official build/outputs/ paths are recognised — anything else
// (intermediates/ task workdirs, Compose mappings, custom destinations)
// reports ok false and stays unmatched. The scan anchors on "outputs"
// followed by the artifact kind, right-to-left so the marker closest to the
// file wins: a checkout directory named "outputs", or a flavor named
// "apk"/"bundle"/"mapping", cannot hijack parsing.
func VariantFromPath(path string) (variant ArtifactVariant, ok bool) {
	segments := strings.Split(filepath.ToSlash(path), "/")

	for i := len(segments) - 2; i >= 0; i-- {
		if segments[i] != "outputs" {
			continue
		}
		switch segments[i+1] {
		case "apk", "bundle", "mapping":
		default:
			// some other outputs child (e.g. logs): keep scanning
			continue
		}

		// the directories between the kind marker and the file name encode
		// the variant; none means the marker is not real (a file named
		// "apk"), keep scanning left
		variantSegments := segments[i+2 : max(i+2, len(segments)-1)]
		if len(variantSegments) == 0 {
			continue
		}

		module := moduleFromSegments(segments[:i])
		return ArtifactVariant{Module: module, Variant: mergeVariantSegments(variantSegments)}, true
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

// mergeVariantSegments joins variant directory segments into the merged Gradle
// variant name: ["demo", "release"] and ["demoRelease"] both yield
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
