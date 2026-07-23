package gradle

import (
	"strings"
)

// If we parse tasks that starts with lint, we will have tasks that starts
// with lintVital also. So list here each conflicting tasks. (only overlapping ones)
var conflicts = map[string][]string{
	"lint": {
		"lintVital",
		"lintFix",
	},
}

func cleanStringSlice(in []string) (out []string) {
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return
}
