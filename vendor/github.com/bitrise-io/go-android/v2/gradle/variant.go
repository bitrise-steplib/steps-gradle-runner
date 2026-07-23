package gradle

import (
	"slices"
	"strings"
)

// Variants ...
type Variants map[string][]string

// Filter ...
func (v Variants) Filter(module, filter string) Variants {
	cleanedFilters := cleanStringSlice(strings.Split(filter, "\n"))

	cleanModules := Variants{}
	if module != "" {
		for m, variants := range v {
			if m == module {
				cleanModules[m] = variants
				break
			}
		}
	} else {
		cleanModules = v
	}

	if len(cleanedFilters) == 0 {
		return cleanModules
	}

	cleanedVariants := Variants{}
	for m, variants := range cleanModules {
		for _, variant := range variants {
			for _, filter := range cleanedFilters {
				if strings.Contains(strings.ToLower(variant), strings.ToLower(filter)) &&
					!slices.Contains(cleanedVariants[m], variant) {
					cleanedVariants[m] = append(cleanedVariants[m], variant)
				}
			}
		}
	}
	return cleanedVariants
}
