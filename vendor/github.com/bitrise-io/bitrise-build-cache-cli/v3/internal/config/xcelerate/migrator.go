package xcelerate

import (
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/bitrise-io/go-utils/v2/log"

	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/toolconfig"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/utils"
)

// ConfigMigrator implements toolconfig.Migrator for ~/.bitrise-xcelerate/config.json.
type ConfigMigrator struct {
	Logger log.Logger
}

func (ConfigMigrator) Tool() toolconfig.Tool { return toolconfig.Xcelerate }

func (m ConfigMigrator) Migrate(_ string) error {
	osProxy := utils.DefaultOsProxy{}

	cfg, err := ReadConfig(osProxy, utils.DefaultDecoderFactory{})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("read xcelerate config: %w", err)
	}

	cfg.ConfigVersion = toolconfig.XcelerateConfigVersion
	cfg.WrittenAt = time.Now().UTC()

	if err := cfg.Save(m.Logger, osProxy, utils.DefaultEncoderFactory{}); err != nil {
		return fmt.Errorf("rewrite xcelerate config: %w", err)
	}

	return nil
}
