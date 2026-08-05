// Package exec is the shared command-runner abstraction for the CLI.
// Wraps os/exec with deterministic stdout/stderr/exit-code handling and an
// opt-in LC_ALL=C / LANG=C pin for callers that match on supervisor error
// strings (the daemon backends do).
package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
)

// Runner runs a binary with arguments and returns stdout, stderr, exit code, and err.
// ExitCode is -1 when the command never started; for non-zero exits err is nil and
// ExitCode carries the value so callers can branch on supervisor-specific codes.
// Callers that want "any non-zero is a failure" should use RunCheck instead.
type Runner interface {
	Run(ctx context.Context, bin string, args ...string) (stdout string, stderr string, exitCode int, err error)
}

// ExecRunner is the production Runner.
type ExecRunner struct {
	// Env extras appended to os.Environ() for the child process.
	Env []string
	// PinLocale forces LC_ALL=C / LANG=C so external command error strings stay English.
	// Off by default; opt in when callers substring-match supervisor output.
	PinLocale bool
}

// RunCheck wraps Run and folds any non-zero exit into err.
func (r ExecRunner) RunCheck(ctx context.Context, bin string, args ...string) (string, string, error) {
	stdout, stderr, code, err := r.Run(ctx, bin, args...)
	if err != nil {
		return stdout, stderr, err
	}

	if code != 0 {
		return stdout, stderr, fmt.Errorf("%s exited %d: %s", bin, code, strings.TrimSpace(stderr))
	}

	return stdout, stderr, nil
}

// Run executes bin with args. See Runner contract for return semantics.
func (r ExecRunner) Run(ctx context.Context, bin string, args ...string) (string, string, int, error) {
	cmd := osexec.CommandContext(ctx, bin, args...)

	if len(r.Env) > 0 || r.PinLocale {
		env := append(os.Environ(), r.Env...)
		if r.PinLocale {
			env = append(env, "LC_ALL=C", "LANG=C")
		}
		cmd.Env = env
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.String(), stderr.String(), exitErr.ExitCode(), nil
		}

		return stdout.String(), stderr.String(), -1, fmt.Errorf("run %s: %w", bin, err)
	}

	return stdout.String(), stderr.String(), 0, nil
}
