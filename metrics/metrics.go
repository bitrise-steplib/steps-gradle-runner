package metrics

import (
	"fmt"
	"os"
	"path"

	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
)

type MetricsCollector struct {
	cmdFactory  command.Factory
	pathProvider pathutil.PathProvider
	logger log.Logger
	gradlewPath string
}

func NewMetricsCollector(cmdFactory command.Factory, pathProvider pathutil.PathProvider, gradlewPath string) MetricsCollector {
	return MetricsCollector{
		cmdFactory:  cmdFactory,
		pathProvider: pathProvider,
		gradlewPath: gradlewPath,
	}
}

func (c MetricsCollector) CollectMetrics() error {
	initScriptPath, err := c.createInitScript()
	if err != nil {
		return err
	}

	if err := c.runGradleTask(initScriptPath); err != nil {
		return err
	}

	return nil
}

func (c MetricsCollector) runGradleTask(initScriptPath string) error {
	args := []string{
		"producer",
		"--init-script",
		initScriptPath,
	}
	cmd := c.cmdFactory.Create(c.gradlewPath, args, nil)
	return cmd.Run()
}

func (c MetricsCollector) createInitScript() (string, error) {
	// TODO: finalize this
	inventory := templateInventory{}
	scriptContent, err := renderTemplate(inventory)
	if err != nil {
		return "", fmt.Errorf("failed to create init script contents: %w", err)
	}

	dir, err := c.pathProvider.CreateTempDir("gradle-runner")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir for init script: %w", err)
	}
	initPath := path.Join(dir, "init.gradle")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for init script: %w", err)
	}

	err = os.WriteFile(initPath, []byte(scriptContent), 0755)
	if err != nil {
		return "", err
	}

	return "", nil
}
