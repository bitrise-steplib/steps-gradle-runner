package paths

import (
	"path/filepath"
	"strings"
)

const (
	GradleUserHomeEnvKey        = "GRADLE_USER_HOME"
	GradleHomeRelative          = ".gradle"
	GradleInitScriptName        = "bitrise-build-cache.init.gradle.kts"
	GradleMirrorsInitScriptName = "bitrise-gradle-mirrors.init.gradle.kts"
	gradleInitDir               = "init.d"
)

// GradleHome honors GRADLE_USER_HOME (verbatim), else <home>/.gradle. Gradle only
// auto-applies init scripts from $GRADLE_USER_HOME/init.d.
func (p Paths) GradleHome(gradleUserHomeEnv string) string {
	if v := strings.TrimSpace(gradleUserHomeEnv); v != "" {
		return v
	}

	return filepath.Join(p.Home, GradleHomeRelative)
}

func GradleInitDir(gradleHome string) string {
	return filepath.Join(gradleHome, gradleInitDir)
}

func GradleInitScript(gradleHome string) string {
	return filepath.Join(GradleInitDir(gradleHome), GradleInitScriptName)
}

func GradleMirrorsInitScript(gradleHome string) string {
	return filepath.Join(GradleInitDir(gradleHome), GradleMirrorsInitScriptName)
}
