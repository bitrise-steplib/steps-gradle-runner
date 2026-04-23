// Package commandhelper runs a command, captures its combined output to a
// file, exports the file path as an env var, and optionally logs the last
// N lines.
package commandhelper

import (
	"fmt"

	"github.com/bitrise-io/go-steputils/v2/export"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
)

// RunAndExportOutputWithReturningLastNLines runs cmd, captures its
// combined stdout/stderr, writes it to destinationPath, exports the path
// under envKey, and returns the last `lines` lines of the output.
//
// The returned values are, in order: the last-N-lines string, the error
// returned by cmd.Run (if any), and any error from exporting the output.
func RunAndExportOutputWithReturningLastNLines(cmd command.Command, exporter export.Exporter, destinationPath, envKey string, lines int) (string, error, error) {
	rawOutput, cmdErr := cmd.RunAndReturnTrimmedCombinedOutput()

	lastLines, exportErr := exporter.ExportStringToFileOutputAndReturnLastNLines(envKey, rawOutput, destinationPath, lines)
	if exportErr != nil {
		return "", cmdErr, exportErr
	}

	return lastLines, cmdErr, nil
}

// RunAndExportOutput runs cmd and writes the combined output to
// destinationPath (exported as envKey). The last `lines` lines are logged
// via logger. Export errors are surfaced as warnings; the run error is
// returned.
func RunAndExportOutput(cmd command.Command, exporter export.Exporter, logger log.Logger, destinationPath, envKey string, lines int) error {
	outputLines, cmdErr, exportErr := RunAndExportOutputWithReturningLastNLines(cmd, exporter, destinationPath, envKey, lines)

	if exportErr != nil {
		logger.Warnf("Failed to export %s, error: %s", envKey, exportErr)
	}

	if lines > 0 && len(outputLines) > 0 {
		header := "You can find the last couple of lines of output below.:"
		if cmdErr != nil {
			logger.Errorf(header)
		} else {
			logger.Infof(header)
		}

		logger.Printf(outputLines)

		if cmdErr != nil {
			logger.Warnf("If you can't find the reason of the error in the log, please check the %s.", destinationPath)
		}
	}

	logger.Infof(fmt.Sprintf("The log file is stored in %s, and its full path is available in the $%s environment variable.", destinationPath, envKey))

	return cmdErr
}
