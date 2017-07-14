package main

import (
	"io/ioutil"
	"testing"
)

func loadFileContent(filePth string) (string, error) {
	fileBytes, err := ioutil.ReadFile(filePth)
	if err != nil {
		return "", err
	}
	return string(fileBytes), nil
}

func testIsFoundWith(t *testing.T, searchPattern, outputToSearchIn string, isShouldFind bool) {
	if isFound := isStringFoundInOutput(searchPattern, outputToSearchIn); isFound != isShouldFind {
		t.Logf("Search pattern was: %s", searchPattern)
		t.Logf("Input string to search in was: %s", outputToSearchIn)
		t.Fatalf("Expected (%v), actual (%v)", isShouldFind, isFound)
	}
}
func testIsFoundWithFailedToFindTargetWithHasString(t *testing.T, outputToSearchIn string, isShouldFind bool) {
	testIsFoundWith(t, failedToFindTargetWithHasString, outputToSearchIn, isShouldFind)
}

func testIsFoundWithFailedToFindBuildToolRevision(t *testing.T, outputToSearchIn string, isShouldFind bool) {
	testIsFoundWith(t, failedToFindBuildToolRevision, outputToSearchIn, isShouldFind)
}

func TestIsStringFoundInOutput_failedToFindTargetWithHasString(t *testing.T) {
	failedToFindTargetWithHasStringLog, err := loadFileContent("./_samples/failedToFindTargetWithHasString.txt")
	if err != nil {
		t.Fatalf("Failed to load error sample log : %s", err)
	}

	sampleOKBuildLog, err := loadFileContent("./_samples/successfullBuild.txt")
	if err != nil {
		t.Fatalf("Failed to load successfullBuild.txt : %s", err)
	}

	t.Log("Should find")
	{
		for _, anOutStr := range []string{
			"Caused by: java.lang.IllegalStateException: failed to find target with hash string 'android-22' in:",
			"> failed to find target with hash string 'android-22' in:",
			failedToFindTargetWithHasStringLog,
		} {
			testIsFoundWithFailedToFindTargetWithHasString(t, anOutStr, true)
		}
	}

	t.Log("Should NOT find")
	{
		for _, anOutStr := range []string{
			"",
			"target with hash string",
			"Caused by: java.lang.IllegalStateException:",
			sampleOKBuildLog,
		} {
			testIsFoundWithFailedToFindTargetWithHasString(t, anOutStr, false)
		}
	}
}

func TestIsStringFoundInOutput_failedToFindBuildToolRevision(t *testing.T) {
	failedToFindBuildToolRevisionLog, err := loadFileContent("./_samples/failedToFindBuildToolRevision.txt")
	if err != nil {
		t.Fatalf("Failed to load error sample log : %s", err)
	}

	sampleOKBuildLog, err := loadFileContent("./_samples/successfullBuild.txt")
	if err != nil {
		t.Fatalf("Failed to load successfullBuild.txt : %s", err)
	}

	t.Log("Should find")
	{
		for _, anOutStr := range []string{
			"Caused by: java.lang.IllegalStateException: failed to find Build Tools revision 22.0.1",
			"> failed to find Build Tools revision 22.0.1",
			failedToFindBuildToolRevisionLog,
		} {
			testIsFoundWithFailedToFindBuildToolRevision(t, anOutStr, true)
		}
	}

	t.Log("Should NOT find")
	{
		for _, anOutStr := range []string{
			"",
			"Build Tools revision 22.0.1",
			"Caused by: java.lang.IllegalStateException:",
			sampleOKBuildLog,
		} {
			testIsFoundWithFailedToFindBuildToolRevision(t, anOutStr, false)
		}
	}
}
