package common

import "strings"

// SanitizeCacheKeyComponent replaces '/' with '_' so the value stays within a
// single segment of the "kv/<key>" cache resource name. The backend keys on one
// segment, so a slash (e.g. a branch like "renovate/x") would drop the trailing
// per-OS suffix and collide the linux/darwin entries.
func SanitizeCacheKeyComponent(s string) string {
	return strings.ReplaceAll(s, "/", "_")
}
