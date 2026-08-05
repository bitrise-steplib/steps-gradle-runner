package ccache

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/bitrise-io/go-utils/v2/log"

	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/config/common"
	multiplatformconfig "github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/config/multiplatform"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/paths"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/toolconfig"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/utils"
)

const (
	ccacheToolName   = "ccache"
	ccacheConfigFile = "config.json"

	defaultLogFile            = "ccache-%s.log"
	defaultErrLogFile         = "ccache-err.log"
	defaultIdleTimeout        = "15m"
	defaultCRSHDataTimeout    = "5s"
	defaultCRSHRequestTimeout = "20s"

	ErrFmtOpenConfigFile   = "open ccache config file (%s): %w"
	ErrFmtDecodeConfigFile = "decode ccache config file (%s): %w"
	ErrFmtCreateConfigFile = "failed to create ccache config file: %w"
	ErrFmtEncodeConfigFile = "failed to encode ccache config file: %w"
	ErrFmtCreateFolder     = "failed to create .bitrise/cache/ccache folder (%s): %w"
	ErrNoAuthConfig        = "resolve auth config: %w"
)

// Params holds the parameters for creating a ccache activate config.
type Params struct {
	BuildCacheEndpoint    string
	PushEnabled           bool
	IPCSocketPathOverride string
	BaseDirOverride       string
}

type Config struct {
	ConfigVersion      string        `json:"configVersion,omitempty"`
	WrittenAt          time.Time     `json:"writtenAt,omitzero"`
	LogFile            string        `json:"logFile,omitempty"`
	ErrLogFile         string        `json:"errLogFile,omitempty"`
	IPCEndpoint        string        `json:"ipcEndpoint,omitempty"`
	IdleTimeout        time.Duration `json:"idleTimeout,omitempty"`
	PushEnabled        bool          `json:"pushEnabled"`
	Enabled            bool          `json:"enabled"`
	DebugLogging       bool          `json:"debugLogging,omitempty"`
	BuildCacheEndpoint string        `json:"buildCacheEndpoint,omitempty"`

	// AuthConfig is populated at runtime from the multiplatform analytics
	// config (single canonical source for auth credentials on disk). Not
	// persisted in the ccache config JSON.
	AuthConfig common.CacheAuthConfig `json:"-"`
}

// relCcacheDir is the per-tool ccache cache dir relative to a root directory.
func relCcacheDir() string {
	return filepath.Join(paths.BitriseRootRelative, "cache", ccacheToolName)
}

func DirPath(osProxy utils.OsProxy) string {
	if home, err := osProxy.UserHomeDir(); err == nil {
		return paths.FromHome(home).BitriseCacheDir(ccacheToolName)
	}

	if wd, err := osProxy.Getwd(); err == nil {
		return filepath.Join(wd, relCcacheDir())
	}

	if exe, err := osProxy.Executable(); err == nil {
		if dir := filepath.Dir(exe); dir != "" {
			return filepath.Join(dir, relCcacheDir())
		}
	}

	return filepath.Join(".", relCcacheDir())
}

func PathFor(osProxy utils.OsProxy, subpath string) string {
	return filepath.Join(DirPath(osProxy), subpath)
}

// ConfigFile returns the absolute path of the ccache config.json.
func ConfigFile(osProxy utils.OsProxy) string {
	return PathFor(osProxy, ccacheConfigFile)
}

// EnvIPCSocketPath overrides the default IPC socket location when set.
const EnvIPCSocketPath = "BITRISE_CCACHE_IPC_SOCKET_PATH"

// ResolveIPCSocketPath returns the storage helper's IPC socket path in the same
// order the activator uses: an explicit override → BITRISE_CCACHE_IPC_SOCKET_PATH
// env var → <temp-dir>/ccache-ipc.sock.
func ResolveIPCSocketPath(override string, envs map[string]string, osProxy utils.OsProxy) string {
	if override != "" {
		return override
	}
	if env := envs[EnvIPCSocketPath]; env != "" {
		return env
	}

	return paths.FromHome("").CcacheSocketPath(osProxy.TempDir())
}

func DefaultParams() Params {
	return Params{
		PushEnabled: true,
	}
}

func NewConfig(envs map[string]string, osProxy utils.OsProxy, params Params) (Config, error) {
	authConfig, _, err := common.ResolveAuthConfig(envs)
	if err != nil {
		return Config{}, fmt.Errorf(ErrNoAuthConfig, err)
	}

	ipcEndpoint := ResolveIPCSocketPath(params.IPCSocketPathOverride, envs, osProxy)

	buildCacheEndpoint := common.SelectCacheEndpointURL(params.BuildCacheEndpoint, envs)
	idleTimeout, _ := time.ParseDuration(defaultIdleTimeout)

	return Config{
		AuthConfig:         authConfig,
		ConfigVersion:      toolconfig.CcacheConfigVersion,
		WrittenAt:          time.Now().UTC(),
		IPCEndpoint:        ipcEndpoint,
		LogFile:            defaultLogFile,
		ErrLogFile:         defaultErrLogFile,
		IdleTimeout:        idleTimeout,
		PushEnabled:        params.PushEnabled,
		Enabled:            true,
		BuildCacheEndpoint: buildCacheEndpoint,
	}, nil
}

func (config Config) CRSHRemoteStorageURL() string {
	return fmt.Sprintf("crsh:%s data-timeout=%s request-timeout=%s",
		config.IPCEndpoint, defaultCRSHDataTimeout, defaultCRSHRequestTimeout)
}

// BuildEnv serves both delivery paths — envman on CI, the wrapped build's own
// environment off it — so the two cannot drift.
func (config Config) BuildEnv(baseDir string) map[string]string {
	return map[string]string{
		"CCACHE_BASEDIR":              baseDir,
		"CCACHE_NOHASHDIR":            "true",
		"CCACHE_REMOTE_ONLY":          "true",
		"CCACHE_REMOTE_STORAGE":       config.CRSHRemoteStorageURL(),
		"CMAKE_CXX_COMPILER_LAUNCHER": "ccache",
		"CMAKE_C_COMPILER_LAUNCHER":   "ccache",
	}
}

func (config Config) Save(logger log.Logger, osProxy utils.OsProxy, encoderFactory utils.EncoderFactory) error {
	ccacheDir := DirPath(osProxy)
	if err := osProxy.MkdirAll(ccacheDir, 0o755); err != nil {
		return fmt.Errorf(ErrFmtCreateFolder, ccacheDir, err)
	}

	configFilePath := PathFor(osProxy, ccacheConfigFile)
	f, err := osProxy.Create(configFilePath)
	if err != nil {
		return fmt.Errorf(ErrFmtCreateConfigFile, err)
	}
	defer f.Close()

	enc := encoderFactory.Encoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(config); err != nil {
		return fmt.Errorf(ErrFmtEncodeConfigFile, err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync ccache config file: %w", err)
	}

	logger.TInfof("Config saved to: %s", configFilePath)

	return nil
}

func ReadConfig(osProxy utils.OsProxy, decoderFactory utils.DecoderFactory) (Config, error) {
	configFilePath := PathFor(osProxy, ccacheConfigFile)

	f, err := osProxy.OpenFile(configFilePath, 0, 0)
	if err != nil {
		return Config{}, fmt.Errorf(ErrFmtOpenConfigFile, configFilePath, err)
	}
	defer f.Close()

	dec := decoderFactory.Decoder(f)
	var config Config
	if err := dec.Decode(&config); err != nil {
		return Config{}, fmt.Errorf(ErrFmtDecodeConfigFile, configFilePath, err)
	}

	if kcCfg, ok := common.GetKeychainCredentials(); ok {
		config.AuthConfig = kcCfg
	} else if mpCfg, mpErr := multiplatformconfig.ReadConfig(osProxy, decoderFactory); mpErr == nil && mpCfg.AuthConfig.AuthToken != "" {
		config.AuthConfig = mpCfg.AuthConfig
	}

	return config, nil
}
