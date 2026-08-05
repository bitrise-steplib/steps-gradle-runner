package common

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os/user"
	"strings"

	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/auth/keychain"
)

const (
	EnvAuthToken   = "BITRISE_BUILD_CACHE_AUTH_TOKEN"   //nolint:gosec // env-var key, not a credential
	EnvWorkspaceID = "BITRISE_BUILD_CACHE_WORKSPACE_ID" //nolint:gosec // env-var key, not a credential
	EnvJWT         = "BITRISEIO_BITRISE_SERVICES_ACCESS_TOKEN"
	EnvUsername    = "BITRISE_BUILD_CACHE_USERNAME"
)

var (
	ErrAuthTokenNotProvided   = errors.New(EnvAuthToken + " or " + EnvJWT + " environment variable not set")
	ErrWorkspaceIDNotProvided = errors.New(EnvWorkspaceID + " environment variable not set")
)

type CacheAuthConfig struct {
	AuthToken   string
	WorkspaceID string
	IsJWT       bool
}

// TokenInGradleFormat returns the auth token in gradle format.
// For JWT tokens, the token is sent as-is (the workspace ID is embedded in the JWT).
// For PAT tokens, the format is "workspaceID:token".
func (cac CacheAuthConfig) TokenInGradleFormat() string {
	if cac.IsJWT || cac.WorkspaceID == "" {
		return cac.AuthToken
	}

	return cac.WorkspaceID + ":" + cac.AuthToken
}

type AuthLoader interface {
	Load() (keychain.Credentials, error)
}

// AuthSource identifies where a credential resolved from.
type AuthSource int

const (
	AuthSourceNone AuthSource = iota
	AuthSourceEnvVars
	AuthSourceJWT
	AuthSourceKeychain
	AuthSourceFile
	AuthSourceMultiplatform
)

// GetKeychainCredentials returns the credentials stored in the OS keychain.
// Bool is true only when both AuthToken and WorkspaceID are populated.
func GetKeychainCredentials() (CacheAuthConfig, bool) {
	return GetKeychainCredentialsWith(keychain.New())
}

func GetKeychainCredentialsWith(loader AuthLoader) (CacheAuthConfig, bool) {
	creds, err := loader.Load()
	if err != nil || creds.AuthToken == "" || creds.WorkspaceID == "" {
		return CacheAuthConfig{}, false
	}

	return CacheAuthConfig{
		AuthToken:   creds.AuthToken,
		WorkspaceID: creds.WorkspaceID,
	}, true
}

type UsernameSource int

const (
	UsernameSourceNone UsernameSource = iota
	UsernameSourceEnv
	UsernameSourceKeychain
	UsernameSourceFile
	UsernameSourceOS
)

func (s UsernameSource) String() string {
	switch s {
	case UsernameSourceEnv:
		return "env"
	case UsernameSourceKeychain:
		return "keychain"
	case UsernameSourceFile:
		return "file"
	case UsernameSourceOS:
		return "os"
	case UsernameSourceNone:
		return sourceNameNone
	}

	return "unknown"
}

func ResolveUsername(envs map[string]string) (string, UsernameSource) {
	return resolveUsername(envs, keychain.New(), fileCredentialsReader, osUsername)
}

func resolveUsername(envs map[string]string, loader AuthLoader, readFile func() (keychain.Credentials, bool), osResolver func() string) (string, UsernameSource) {
	if v := strings.TrimSpace(envs[EnvUsername]); v != "" {
		return v, UsernameSourceEnv
	}
	if creds, err := loader.Load(); err == nil {
		if v := strings.TrimSpace(creds.Username); v != "" {
			return v, UsernameSourceKeychain
		}
	}
	if readFile != nil {
		if creds, ok := readFile(); ok {
			if v := strings.TrimSpace(creds.Username); v != "" {
				return v, UsernameSourceFile
			}
		}
	}
	if v := strings.TrimSpace(osResolver()); v != "" {
		return v, UsernameSourceOS
	}

	return "", UsernameSourceNone
}

func osUsername() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}

	return u.Username
}

// Wired via RegisterMultiplatformReader to avoid a multiplatform→common import cycle.
//
//nolint:gochecknoglobals
var multiplatformConfigReader func() (CacheAuthConfig, error)

func RegisterMultiplatformReader(fn func() (CacheAuthConfig, error)) {
	multiplatformConfigReader = fn
}

// Wired from multiplatform to avoid the import cycle.
//
//nolint:gochecknoglobals
var fileCredentialsReader func() (keychain.Credentials, bool)

func RegisterFileCredentialsReader(fn func() (keychain.Credentials, bool)) {
	fileCredentialsReader = fn
}

// Precedence: env → keychain → multiplatform Credentials (CI-safe file) → legacy authConfig → not-set error.
func ResolveAuthConfig(envs map[string]string) (CacheAuthConfig, AuthSource, error) {
	return resolveAuthConfig(envs, keychain.New(), fileCredentialsReader, multiplatformConfigReader)
}

func resolveAuthConfig(envs map[string]string, loader AuthLoader, readFile func() (keychain.Credentials, bool), readMultiplatform func() (CacheAuthConfig, error)) (CacheAuthConfig, AuthSource, error) {
	if hasAuthEnvVars(envs) {
		return readAuthConfigFromEnvironments(envs)
	}

	if cfg, ok := GetKeychainCredentialsWith(loader); ok {
		return cfg, AuthSourceKeychain, nil
	}

	if readFile != nil {
		if creds, ok := readFile(); ok && creds.AuthToken != "" && creds.WorkspaceID != "" {
			return CacheAuthConfig{AuthToken: creds.AuthToken, WorkspaceID: creds.WorkspaceID}, AuthSourceFile, nil
		}
	}

	if readMultiplatform != nil {
		if mpCfg, mpErr := readMultiplatform(); mpErr == nil && mpCfg.AuthToken != "" && mpCfg.WorkspaceID != "" {
			return mpCfg, AuthSourceMultiplatform, nil
		}
	}

	return readAuthConfigFromEnvironments(envs)
}

func hasAuthEnvVars(envs map[string]string) bool {
	if envs[EnvJWT] != "" {
		return true
	}

	return envs[EnvAuthToken] != "" && envs[EnvWorkspaceID] != ""
}

func readAuthConfigFromEnvironments(envs map[string]string) (CacheAuthConfig, AuthSource, error) {
	authTokenEnv := envs[EnvAuthToken]
	workspaceIDEnv := envs[EnvWorkspaceID]

	if len(authTokenEnv) > 0 && len(workspaceIDEnv) > 0 {
		return CacheAuthConfig{
			AuthToken:   authTokenEnv,
			WorkspaceID: workspaceIDEnv,
		}, AuthSourceEnvVars, nil
	}

	// Try to fall back to JWT which is always available on Bitrise.
	// It's a JWT token which already includes the workspace ID.
	if serviceToken := envs[EnvJWT]; len(serviceToken) > 0 {
		workspaceID, err := extractWorkspaceIDFromJWT(serviceToken)
		if err != nil {
			return CacheAuthConfig{}, AuthSourceNone, fmt.Errorf("extract workspace ID from JWT: %w", err)
		}

		return CacheAuthConfig{
			AuthToken:   serviceToken,
			WorkspaceID: workspaceID,
			IsJWT:       true,
		}, AuthSourceJWT, nil
	}

	if len(authTokenEnv) < 1 {
		return CacheAuthConfig{}, AuthSourceNone, ErrAuthTokenNotProvided
	}

	return CacheAuthConfig{}, AuthSourceNone, ErrWorkspaceIDNotProvided
}

type jwtPermissionClaims struct {
	OrgID []string `json:"org_id"`
}

type jwtPermission struct {
	Rsname string              `json:"rsname"`
	Claims jwtPermissionClaims `json:"claims"`
}

type jwtAuthorization struct {
	Permissions []jwtPermission `json:"permissions"`
}

type jwtPayload struct {
	Authorization jwtAuthorization `json:"authorization"`
}

// Extracts org_id from a Bitrise UMA-style JWT without verifying the signature
// (we trust the issuer — Bitrise mints these per build).
func extractWorkspaceIDFromJWT(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 { //nolint:mnd
		return "", errors.New("invalid JWT format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode JWT payload: %w", err)
	}

	var claims jwtPayload
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse JWT payload: %w", err)
	}

	for _, perm := range claims.Authorization.Permissions {
		if perm.Rsname != "default" {
			continue
		}

		if len(perm.Claims.OrgID) == 0 {
			return "", errors.New("org_id claim is missing from JWT")
		}

		workspaceID := perm.Claims.OrgID[0]
		if workspaceID == "" {
			return "", errors.New("org_id claim is empty in JWT")
		}

		return workspaceID, nil
	}

	return "", errors.New("'default' permission not found in JWT")
}
