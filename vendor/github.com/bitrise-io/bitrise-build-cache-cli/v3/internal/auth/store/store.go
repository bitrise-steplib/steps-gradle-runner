// Package store picks the credential backend: CI→file (fastlane setup_ci swaps the keychain), local→keychain.
package store

import (
	"errors"
	"fmt"

	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/auth/keychain"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/config/common"
	multiplatformconfig "github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/config/multiplatform"
	"github.com/bitrise-io/bitrise-build-cache-cli/v3/internal/utils"
)

// Sentinel independent of backend; callers can errors.Is either this or keychain.ErrNotFound.
var ErrNotFound = errors.New("no Bitrise Build Cache credentials in store")

type Kind int

const (
	KindKeychain Kind = iota
	KindFile
)

func (k Kind) String() string {
	switch k {
	case KindKeychain:
		return "keychain"
	case KindFile:
		return "file"
	}

	return "unknown"
}

// Load returns ErrNotFound when nothing is stored.
type Store interface {
	Kind() Kind
	Load() (keychain.Credentials, error)
	Save(creds keychain.Credentials) error
	Clear() error
}

// CI→file, local→keychain. Total function; no error path.
func SelectAuto(envs map[string]string) Store {
	if common.DetectCIProvider(envs) != "" {
		return NewFile()
	}

	return NewKeychain()
}

// override: "keychain" | "file" | "" | "auto"; empty/auto delegates to SelectAuto.
func Select(envs map[string]string, override string) (Store, error) {
	switch override {
	case "", "auto":
		return SelectAuto(envs), nil
	case "keychain":
		return NewKeychain(), nil
	case "file":
		return NewFile(), nil
	}

	return nil, fmt.Errorf("unknown storage backend %q (want keychain|file|auto)", override)
}

// Saves to target, then best-effort clears every other backend to prevent split-brain.
func SaveExclusive(target Store, creds keychain.Credentials) error {
	if err := target.Save(creds); err != nil {
		return err //nolint:wrapcheck
	}
	for _, other := range []Store{NewKeychain(), NewFile()} {
		if other.Kind() == target.Kind() {
			continue
		}
		_ = other.Clear()
	}

	return nil
}

// SaveOutcome reports which backend a save ended up in. KeychainErr is set only
// when the keychain was tried and refused, so callers can explain the fallback.
type SaveOutcome struct {
	Kind        Kind
	FellBack    bool
	KeychainErr error
}

// SaveExclusiveWithFallback saves to target, dropping to the file store when the
// keychain refuses the write: a host with no secret-service (headless Linux,
// containers) would otherwise have no way to store credentials at all, short of
// knowing to pass --storage=file.
//
// allowFallback is false when the caller picked the backend explicitly — they
// asked for that one, so silently using another would be wrong.
func SaveExclusiveWithFallback(target Store, creds keychain.Credentials, allowFallback bool) (SaveOutcome, error) {
	err := SaveExclusive(target, creds)
	if err == nil {
		return SaveOutcome{Kind: target.Kind()}, nil
	}

	if target.Kind() != KindKeychain || !allowFallback {
		return SaveOutcome{Kind: target.Kind()}, err
	}

	fallback := NewFile()
	if fbErr := SaveExclusive(fallback, creds); fbErr != nil {
		return SaveOutcome{Kind: fallback.Kind()}, fmt.Errorf("save to the keychain (%w) and to the config file (%w)", err, fbErr)
	}

	return SaveOutcome{Kind: fallback.Kind(), FellBack: true, KeychainErr: err}, nil
}

func NewKeychain() Store {
	return keychainStore{kc: keychain.New()}
}

func NewFile() Store {
	return fileStore{
		osProxy:        utils.DefaultOsProxy{},
		encoderFactory: utils.DefaultEncoderFactory{},
		decoderFactory: utils.DefaultDecoderFactory{},
	}
}

type keychainStore struct {
	kc *keychain.Keychain
}

func (s keychainStore) Kind() Kind { return KindKeychain }

// A machine with no keychain reads as empty rather than failing, so every
// credential lookup falls through to the next backend. `auth status` and the
// doctor call the keychain directly and keep the distinction.
func (s keychainStore) Load() (keychain.Credentials, error) {
	creds, err := s.kc.Load()
	if errors.Is(err, keychain.ErrNotFound) || errors.Is(err, keychain.ErrUnavailable) {
		return keychain.Credentials{}, ErrNotFound
	}

	return creds, err //nolint:wrapcheck // keychain.Keychain already wraps
}

func (s keychainStore) Save(c keychain.Credentials) error {
	return s.kc.Save(c) //nolint:wrapcheck
}

func (s keychainStore) Clear() error {
	return s.kc.Clear() //nolint:wrapcheck
}

type fileStore struct {
	osProxy        utils.OsProxy
	encoderFactory utils.EncoderFactory
	decoderFactory utils.DecoderFactory
}

func (s fileStore) Kind() Kind { return KindFile }

func (s fileStore) Load() (keychain.Credentials, error) {
	creds, ok := multiplatformconfig.ReadCredentials(s.osProxy, s.decoderFactory)
	if !ok {
		return keychain.Credentials{}, ErrNotFound
	}

	return creds, nil
}

func (s fileStore) Save(c keychain.Credentials) error {
	if err := multiplatformconfig.SaveCredentials(s.osProxy, s.encoderFactory, s.decoderFactory, c); err != nil {
		return fmt.Errorf("save credentials to multiplatform config: %w", err)
	}

	return nil
}

func (s fileStore) Clear() error {
	if err := multiplatformconfig.ClearCredentials(s.osProxy, s.encoderFactory, s.decoderFactory); err != nil {
		return fmt.Errorf("clear credentials from multiplatform config: %w", err)
	}

	return nil
}
