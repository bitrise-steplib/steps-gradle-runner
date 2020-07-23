package commandhelper

import (
	"bytes"
	"fmt"

	"github.com/bitrise-io/go-steputils/output"
	"github.com/bitrise-io/go-utils/colorstring"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
)

// RunAndExportOutputWithReturningLastNLines runs a command and captures it's output to a file.
// The genereated output file will be exported to the envKey environment variable.
// It returns the last N lines of the output, the error of the command and if any error happened during
// exporting the output file.
func RunAndExportOutputWithReturningLastNLines(cmd command.Model, destinationPath, envKey string, lines int) (string, error, error) {
	var outBuffer bytes.Buffer
	cmd.SetStdout(&outBuffer)
	cmd.SetStderr(&outBuffer)

	cmdError := cmd.Run()
	rawOutput := outBuffer.String()

	lastLines, err := output.ExportOutputFileContentAndReturnLastNLines(rawOutput, destinationPath, envKey, lines)
	if err != nil {
		return "", cmdError, err
	}

	return lastLines, cmdError, nil
}

// RunAndExportOutput runs a command and captures it's output to a file.
// The genereated output file will be exported to the envKey environment variable.
// The last N lines of the output if loged with some description.
func RunAndExportOutput(cmd command.Model, destinationPath, envKey string, lines int) error {
	outputLines, cmdErr, exportErr := RunAndExportOutputWithReturningLastNLines(cmd, destinationPath, envKey, lines)

	if exportErr != nil {
		log.Warnf("Failed to export %s, error: %s", envKey, exportErr)
	}

	if lines > 0 && len(outputLines) > 0 {
		lastLines := "You can find the last couple of lines of output below.:"
		if cmdErr != nil {
			log.Errorf(lastLines)
		} else {
			log.Infof(lastLines)
		}

		log.Printf(outputLines)

		if cmdErr != nil {
			log.Warnf("If you can't find the reason of the error in the log, please check the %s.", destinationPath)
		}
	}

	log.Infof(colorstring.Magenta(fmt.Sprintf(`The log file is stored in %s, and its full path is available in the $%s environment variable.`, destinationPath, envKey)))

	return cmdErr
}
