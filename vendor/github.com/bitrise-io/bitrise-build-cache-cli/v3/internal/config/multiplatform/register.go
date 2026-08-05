package multiplatform

import (
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/auth/keychain"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/config/common"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/utils"
)

func init() { //nolint:gochecknoinits
	common.RegisterMultiplatformReader(readMultiplatformAuthConfig)
	common.RegisterFileCredentialsReader(readMultiplatformCredentials)
}

func readMultiplatformAuthConfig() (common.CacheAuthConfig, error) {
	cfg, err := ReadConfig(utils.DefaultOsProxy{}, utils.DefaultDecoderFactory{})
	if err != nil {
		return common.CacheAuthConfig{}, err //nolint:wrapcheck // surfaced only as a fallback signal
	}

	return cfg.AuthConfig, nil
}

func readMultiplatformCredentials() (keychain.Credentials, bool) {
	return ReadCredentials(utils.DefaultOsProxy{}, utils.DefaultDecoderFactory{})
}
