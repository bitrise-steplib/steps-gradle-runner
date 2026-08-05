package store

import (
	"fmt"
	"strings"

	"github.com/bitrise-io/go-utils/v2/log"

	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/auth/keychain"
	configcommon "github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/config/common"
	multiplatformconfig "github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/config/multiplatform"
)

// PersistActivateCreds routes non-JWT activation creds to keychain (local) or the multiplatform Credentials field (CI); JWT keeps the legacy AuthConfig write for downstream reactnative/invocation compat.
func PersistActivateCreds(logger log.Logger, envs map[string]string, auth configcommon.CacheAuthConfig, mpCfg *multiplatformconfig.Config) {
	if auth.IsJWT {
		mpCfg.AuthConfig = auth

		return
	}
	persistActivateCredsTo(logger, SelectAuto(envs), auth, mpCfg)
}

// persistActivateCredsTo takes the store so tests can refuse a write without a
// real keychain.
func persistActivateCredsTo(logger log.Logger, target Store, auth configcommon.CacheAuthConfig, mpCfg *multiplatformconfig.Config) {
	if target.Kind() == KindFile {
		persistToFile(auth, mpCfg)
		logger.Infof("Saved auth credentials to the multiplatform config file (CI-safe — fastlane setup_ci swaps the keychain)")

		return
	}
	if err := target.Save(mergeActivateCreds(target, auth)); err != nil {
		// AuthConfig has no room for the refresh token; the config file does.
		logger.Warnf("Keychain save failed (%v); saving to the multiplatform config file instead", err)
		persistToFile(auth, mpCfg)

		return
	}
	logger.Infof("Saved auth credentials to the OS keychain")
}

// Merges against the file store, not the caller's: merging against an unreadable
// keychain is what yields a bare token.
func persistToFile(auth configcommon.CacheAuthConfig, mpCfg *multiplatformconfig.Config) {
	c := mergeActivateCreds(NewFile(), auth)
	mpCfg.Credentials = &c
	mpCfg.AuthConfig = auth
}

// The tokens are deliberately not compared: a login's PAT is short-lived and gets
// refreshed, so a mismatch is normal, and treating it as a different credential
// discarded the refresh token.
func mergeActivateCreds(target Store, auth configcommon.CacheAuthConfig) keychain.Credentials {
	existing, err := target.Load()
	if err != nil {
		return keychain.Credentials{AuthToken: auth.AuthToken, WorkspaceID: auth.WorkspaceID}
	}

	existing.AuthToken = auth.AuthToken
	existing.WorkspaceID = auth.WorkspaceID

	return existing
}

// SetUsername writes name into the store that already holds credentials so a
// username-only edit can't strand an empty-token entry in the wrong backend.
// Empty name clears the override. Returns the store written to.
func SetUsername(envs map[string]string, name string) (Kind, error) {
	target, existing := storeHoldingCreds(envs)
	existing.Username = strings.TrimSpace(name)
	if err := target.Save(existing); err != nil {
		return target.Kind(), fmt.Errorf("save display name to %s: %w", target.Kind(), err)
	}

	return target.Kind(), nil
}

func storeHoldingCreds(envs map[string]string) (Store, keychain.Credentials) {
	for _, s := range []Store{NewKeychain(), NewFile()} {
		creds, err := s.Load()
		if err == nil && (strings.TrimSpace(creds.AuthToken) != "" || strings.TrimSpace(creds.WorkspaceID) != "") {
			return s, creds
		}
	}

	target := SelectAuto(envs)
	creds, _ := target.Load()

	return target, creds
}
