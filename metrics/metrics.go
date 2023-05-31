package metrics

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
)

const defaultEndpoint = "gradle-analytics.services.bitrise.io"
const defaultPort = 443

// Sync the major version of this step and the plugin.
// Use the latest 1.x version of the plugin, so we don't have to update this definition after every plugin release.
// But don't forget to update this to `2.+` if the library reaches version 2.0!
const defaultPluginVersion = "main-SNAPSHOT" // TODO: change to 1.+

type Collector struct {
	envRepo       env.Repository
	cmdFactory    command.Factory
	pathProvider  pathutil.PathProvider
	logger        log.Logger
	gradlewPath   string
	buildFilePath string

	initScriptPath string
}

func NewCollector(
	envRepo env.Repository,
	cmdFactory command.Factory,
	pathProvider pathutil.PathProvider,
	logger log.Logger,
	gradlewPath string,
	buildFilePath string,
) Collector {
	return Collector{
		envRepo:       envRepo,
		cmdFactory:    cmdFactory,
		pathProvider:  pathProvider,
		logger:        logger,
		gradlewPath:   gradlewPath,
		buildFilePath: buildFilePath,
	}
}

func (c *Collector) CanBeEnabled(gradleFlags string) bool {
	initScriptDefined := strings.Contains(gradleFlags, "--init-script")
	if initScriptDefined {
		c.logger.Warnf("An init script is already defined via the additional Gradle flags step input. Metrics collection will be disabled.")
	}
	return !initScriptDefined
}

func (c *Collector) SetupMetricsCollection() error {
	authToken := c.envRepo.Get("BITRISEIO_BITRISE_SERVICES_ACCESS_TOKEN")
	if authToken == "" {
		return fmt.Errorf("$BITRISEIO_BITRISE_SERVICES_ACCESS_TOKEN is empty. This step is only supposed to run in Bitrise CI builds")
	}

	return c.createInitScript(authToken)
}

func (c *Collector) UpdateGradleFlagsWithInitScript(gradleFlags string) string {
	if c.initScriptPath == "" {
		return gradleFlags
	}

	return fmt.Sprintf("%s --init-script %s", gradleFlags, c.initScriptPath)
}

func (c *Collector) SendMetrics() error {
	if c.initScriptPath == "" {
		return fmt.Errorf("init script path is empty, this can't run without an existing init script")
	}

	if err := c.runGradleTask(c.initScriptPath); err != nil {
		return err
	}

	return nil
}

func (c *Collector) runGradleTask(initScriptPath string) error {
	args := []string{
		"producer",
		"--build-file", c.buildFilePath,
		"--init-script", initScriptPath,
	}
	opts := command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	cmd := c.cmdFactory.Create(c.gradlewPath, args, &opts)
	err := cmd.Run()
	if err != nil {
		c.logger.Warnf("Failed to run Gradle task: %s", err)
	}
	return nil
}

func (c *Collector) createInitScript(authToken string) error {
	inventory := templateInventory{
		Endpoint:  defaultEndpoint,
		Port:      defaultPort,
		Version:   defaultPluginVersion,
		AuthToken: authToken,
	}
	scriptContent, err := renderTemplate(inventory)
	if err != nil {
		return fmt.Errorf("failed to create init script contents: %w", err)
	}

	dir, err := c.pathProvider.CreateTempDir("gradle-runner")
	if err != nil {
		return fmt.Errorf("failed to create temp dir for init script: %w", err)
	}
	initPath := path.Join(dir, "init-analytics.gradle")
	if err != nil {
		return fmt.Errorf("failed to create temp file for init script: %w", err)
	}

	err = os.WriteFile(initPath, []byte(scriptContent), 0755)
	if err != nil {
		return err
	}

	c.initScriptPath = initPath
	return nil
}
