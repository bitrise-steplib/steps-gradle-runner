package keychain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	keyring "github.com/zalando/go-keyring"
)

const (
	serviceName = "bitrise-build-cache"
	accountName = "default"
)

var ErrNotFound = errors.New("no Bitrise Build Cache credentials in keychain")

// Credentials is the single keychain item. AuthToken + WorkspaceID are always
// present (a manual `auth set` writes only those). The remaining fields are set
// only for an OAuth login (`bitrise-build-cache auth login`), where AuthToken is the
// minted PAT and the refresh token + expiries drive transparent refresh.
type Credentials struct {
	AuthToken          string    `json:"auth_token"`
	WorkspaceID        string    `json:"workspace_id"`
	Username           string    `json:"username,omitempty"`
	PATExpiry          time.Time `json:"pat_expiry,omitempty"`
	JWT                string    `json:"jwt,omitempty"`
	JWTExpiry          time.Time `json:"jwt_expiry,omitempty"`
	RefreshToken       string    `json:"refresh_token,omitempty"`
	RefreshTokenExpiry time.Time `json:"refresh_token_expiry,omitempty"`
}

// IsOAuthManaged reports whether the credential came from an OAuth login (it
// carries a refresh token), as opposed to a manual `auth set`.
func (c Credentials) IsOAuthManaged() bool {
	return c.RefreshToken != ""
}

type Backend interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

type defaultBackend struct{}

func (defaultBackend) Get(service, account string) (string, error) {
	return keyring.Get(service, account) //nolint:wrapcheck // wrapped in Keychain methods
}

func (defaultBackend) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret) //nolint:wrapcheck
}

func (defaultBackend) Delete(service, account string) error {
	return keyring.Delete(service, account) //nolint:wrapcheck
}

type Keychain struct {
	Backend Backend
}

func New() *Keychain {
	return &Keychain{Backend: defaultBackend{}}
}

// NewBackend returns the OS keychain backend used by Keychain — exposed for
// callers that need raw Set/Get/Delete against a non-default service/account
// (e.g. the doctor smoke-test).
func NewBackend() Backend {
	return defaultBackend{}
}

func (k *Keychain) Load() (Credentials, error) {
	raw, err := k.Backend.Get(serviceName, accountName)
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		return Credentials{}, ErrNotFound
	case err != nil:
		return Credentials{}, fmt.Errorf("keychain read: %w", classify(err))
	}

	var c Credentials
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return Credentials{}, fmt.Errorf("keychain decode: %w", err)
	}

	return c, nil
}

func (k *Keychain) Save(c Credentials) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("keychain encode: %w", err)
	}

	if err := k.Backend.Set(serviceName, accountName, string(raw)); err != nil {
		return fmt.Errorf("keychain write: %w", classify(err))
	}

	return nil
}

// SaveIfChanged writes c only when it differs from the stored value; returns whether a write happened.
func (k *Keychain) SaveIfChanged(c Credentials) (bool, error) {
	existing, err := k.Load()
	if err == nil && existing == c {
		return false, nil
	}

	if err := k.Save(c); err != nil {
		return false, err
	}

	return true, nil
}

func (k *Keychain) Clear() error {
	switch err := k.Backend.Delete(serviceName, accountName); {
	case err == nil, errors.Is(err, keyring.ErrNotFound):
		return nil
	default:
		return fmt.Errorf("keychain delete: %w", classify(err))
	}
}
