package common

import (
	"fmt"
	"time"

	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/auth/keychain"
)

const sourceNameNone = "none"

// AuthDescription is the shared description of a resolved credential, consumed
// by status, doctor, and `auth status` so the source taxonomy and the
// OAuth-login/expiry knowledge live in one place.
type AuthDescription struct {
	Source       AuthSource
	WorkspaceID  string
	IsOAuthLogin bool      // keychain credential created by `bitrise-build-cache auth login`
	PATExpiry    time.Time // zero when unknown / not applicable
}

// SourceLabel is the canonical human name for a credential source.
func SourceLabel(source AuthSource, isOAuthLogin bool) string {
	switch source {
	case AuthSourceEnvVars:
		return "environment variables"
	case AuthSourceJWT:
		return "CI JWT (" + EnvJWT + ")"
	case AuthSourceKeychain:
		if isOAuthLogin {
			return "OAuth login (keychain)"
		}

		return "OS keychain"
	case AuthSourceFile:
		return "config file (CI-safe)"
	case AuthSourceMultiplatform:
		return "multiplatform config"
	case AuthSourceNone:
		return sourceNameNone
	}

	return sourceNameNone
}

// Label is the canonical source name for this description.
func (d AuthDescription) Label() string {
	return SourceLabel(d.Source, d.IsOAuthLogin)
}

// Expired reports whether a known PAT expiry is in the past.
func (d AuthDescription) Expired() bool {
	return !d.PATExpiry.IsZero() && time.Now().After(d.PATExpiry)
}

// Detail is the canonical one-line human description (label + workspace + expiry).
func (d AuthDescription) Detail() string {
	out := d.Label()
	if d.WorkspaceID != "" {
		out += fmt.Sprintf(" (workspace %s)", d.WorkspaceID)
	}
	switch {
	case d.PATExpiry.IsZero():
	case d.Expired():
		out += ", token expired — refreshes on next use"
	default:
		out += ", token valid until " + d.PATExpiry.Format(time.RFC3339)
	}

	return out
}

// DescribeResolved describes a resolved credential, reading the OS keychain for
// the OAuth-login + expiry distinction when the source is the keychain.
func DescribeResolved(cfg CacheAuthConfig, source AuthSource) AuthDescription {
	return DescribeResolvedWith(cfg, source, keychain.New())
}

// DescribeResolvedWith is DescribeResolved with an injectable keychain loader
// (for the doctor's injected backend and for tests).
func DescribeResolvedWith(cfg CacheAuthConfig, source AuthSource, loader AuthLoader) AuthDescription {
	if source == AuthSourceKeychain && loader != nil {
		if creds, err := loader.Load(); err == nil {
			return DescribeKeychainCredentials(creds)
		}
	}

	d := AuthDescription{Source: source}
	// JWT embeds the workspace in the token; callers don't surface it.
	if source != AuthSourceJWT {
		d.WorkspaceID = cfg.WorkspaceID
	}

	return d
}

// DescribeKeychainCredentials describes an already-loaded keychain credential,
// distinguishing an OAuth login (with PAT expiry) from a manual `auth set`.
func DescribeKeychainCredentials(creds keychain.Credentials) AuthDescription {
	d := AuthDescription{Source: AuthSourceKeychain, WorkspaceID: creds.WorkspaceID}
	if creds.IsOAuthManaged() {
		d.IsOAuthLogin = true
		d.PATExpiry = creds.PATExpiry
	}

	return d
}
