package keychain

import (
	"errors"
	"fmt"
	"strings"

	dbus "github.com/godbus/dbus/v5"
	keyring "github.com/zalando/go-keyring"
)

// ErrUnavailable means this machine has no usable OS keychain at all: a Linux box
// with no secret-service on the session bus (headless, containers, most CI), or a
// platform go-keyring has no backend for. Nothing failed — there is nowhere to
// read from or write to — so callers treat the backend as absent, not broken.
var ErrUnavailable = errors.New("no usable OS keychain on this machine")

// dbusSessionBusMissing is godbus's message when there is no session bus to talk
// to; unlike the errors below it arrives as a plain error, with no name to match.
const dbusSessionBusMissing = "couldn't determine address of session bus"

// classify converts a backend error into ErrUnavailable when it says the keychain
// service doesn't exist here, and leaves every other error alone.
func classify(err error) error {
	if !Unavailable(err) {
		return err
	}

	// %w twice rather than errors.Join, whose newline separator wraps badly in logs.
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}

// Unavailable reports whether err means there is no keychain service to talk to.
// Exported for callers holding a raw Backend, which doesn't pass through classify.
func Unavailable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, keyring.ErrUnsupportedPlatform) {
		return true
	}

	var dbusErr dbus.Error
	if errors.As(err, &dbusErr) {
		// ServiceUnknown: nothing implements org.freedesktop.secrets. Spawn.*: the
		// service is declared but couldn't be started. NoServer: no bus to ask.
		switch {
		case dbusErr.Name == "org.freedesktop.DBus.Error.ServiceUnknown",
			dbusErr.Name == "org.freedesktop.DBus.Error.NoServer",
			strings.HasPrefix(dbusErr.Name, "org.freedesktop.DBus.Error.Spawn."):
			return true
		}
	}

	return strings.Contains(err.Error(), dbusSessionBusMissing)
}
