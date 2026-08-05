package multiplatform

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/auth/keychain"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/config/common"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/utils"
)

const (
	configPath = ".bitrise/analytics/multiplatform"
	configFile = "config.json"

	ErrFmtOpenConfigFile   = "open multiplatform analytics config file (%s): %w"
	ErrFmtDecodeConfigFile = "decode multiplatform analytics config file (%s): %w"
	ErrFmtCreateConfigFile = "failed to create multiplatform analytics config file: %w"
	ErrFmtEncodeConfigFile = "failed to encode multiplatform analytics config file: %w"
	ErrFmtCreateFolder     = "failed to create %s folder: %w"
)

// Credentials is the CI-safe file backend for auth set/login; AuthConfig stays for backward compatibility with older analytics readers.
type Config struct {
	AuthConfig   common.CacheAuthConfig `json:"authConfig"`
	Credentials  *keychain.Credentials  `json:"credentials,omitempty"`
	DebugLogging bool                   `json:"debugLogging,omitempty"`
}

func dirPath(osProxy utils.OsProxy) string {
	if home, err := osProxy.UserHomeDir(); err == nil {
		return filepath.Join(home, configPath)
	}

	if wd, err := osProxy.Getwd(); err == nil {
		return filepath.Join(wd, configPath)
	}

	return filepath.Join(".", configPath)
}

// FilePath returns the absolute path of the multiplatform analytics config file.
func FilePath(osProxy utils.OsProxy) string {
	return filepath.Join(dirPath(osProxy), configFile)
}

// Atomic write with 0600 perms — file holds PATs and OAuth refresh tokens.
func (c Config) Save(osProxy utils.OsProxy, encoderFactory utils.EncoderFactory) error {
	dir := dirPath(osProxy)
	if err := osProxy.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf(ErrFmtCreateFolder, dir, err)
	}

	var buf bytes.Buffer
	enc := encoderFactory.Encoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(c); err != nil {
		return fmt.Errorf(ErrFmtEncodeConfigFile, err)
	}

	path := FilePath(osProxy)
	tmp := path + ".tmp"
	if err := osProxy.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf(ErrFmtCreateConfigFile, err)
	}
	if err := osProxy.Rename(tmp, path); err != nil {
		_ = osProxy.Remove(tmp)

		return fmt.Errorf("rename multiplatform config file: %w", err)
	}

	return nil
}

// Mirrors creds into legacy AuthConfig so downstream reactnative/invocation readers keep working.
func SaveCredentials(osProxy utils.OsProxy, encoderFactory utils.EncoderFactory, decoderFactory utils.DecoderFactory, creds keychain.Credentials) error {
	cfg, err := ReadConfig(osProxy, decoderFactory)
	if err != nil && !isNotExist(err) {
		return err
	}

	c := creds
	cfg.Credentials = &c
	cfg.AuthConfig = common.CacheAuthConfig{AuthToken: creds.AuthToken, WorkspaceID: creds.WorkspaceID}

	return cfg.Save(osProxy, encoderFactory)
}

func ReadCredentials(osProxy utils.OsProxy, decoderFactory utils.DecoderFactory) (keychain.Credentials, bool) {
	cfg, err := ReadConfig(osProxy, decoderFactory)
	if err != nil || cfg.Credentials == nil {
		return keychain.Credentials{}, false
	}

	return *cfg.Credentials, true
}

func ClearCredentials(osProxy utils.OsProxy, encoderFactory utils.EncoderFactory, decoderFactory utils.DecoderFactory) error {
	cfg, err := ReadConfig(osProxy, decoderFactory)
	if err != nil {
		if isNotExist(err) {
			return nil
		}

		return err
	}
	if cfg.Credentials == nil && cfg.AuthConfig.AuthToken == "" {
		return nil
	}
	cfg.Credentials = nil
	cfg.AuthConfig = common.CacheAuthConfig{}

	return cfg.Save(osProxy, encoderFactory)
}

func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// ReadConfig loads the config from disk.
func ReadConfig(osProxy utils.OsProxy, decoderFactory utils.DecoderFactory) (Config, error) {
	path := FilePath(osProxy)

	f, err := osProxy.OpenFile(path, 0, 0)
	if err != nil {
		return Config{}, fmt.Errorf(ErrFmtOpenConfigFile, path, err)
	}
	defer f.Close()

	dec := decoderFactory.Decoder(f)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf(ErrFmtDecodeConfigFile, path, err)
	}

	return cfg, nil
}
