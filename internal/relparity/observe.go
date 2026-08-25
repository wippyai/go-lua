package relparity

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Exit statuses the harness assigns to a run that produced no status of its
// own, kept distinct so a killed process and an unstartable one never read as
// the same outcome.
const (
	exitCodeExhausted = -1
	exitCodeUnstarted = -2
)

// Observation is one side's whole answer for one (fixture, verb): how the
// process ended and what it published.
type Observation struct {
	Side     string `json:"side"`
	Fixture  string `json:"fixture"`
	Verb     string `json:"verb"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
	Stdout   string `json:"-"`
	Stderr   string `json:"stderr"`
}

// Observe runs one side's binary for one fixture and verb, in its own process,
// bounded by the probe's timeout.
//
// Both sides run with the same working directory, so the two runtimes read one
// fixture tree and any path either of them prints is the same path.
func Observe(ctx context.Context, side Side, probe Probe, workingDirectory, fixture, verb string) Observation {
	bounded, cancel := context.WithTimeout(ctx, probe.Timeout)
	defer cancel()

	command := exec.CommandContext(bounded, side.Binary, fixture, verb)
	command.Dir = workingDirectory
	command.Env = append(command.Environ(), side.Env...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()

	observation := Observation{
		Side:    side.Name,
		Fixture: fixture,
		Verb:    verb,
		Stdout:  stdout.String(),
		Stderr:  strings.TrimRight(stderr.String(), "\n"),
	}
	if errors.Is(bounded.Err(), context.DeadlineExceeded) {
		observation.TimedOut = true
	}
	var exit *exec.ExitError
	switch {
	case observation.TimedOut:
		// The bound killed the process. Its exit status is the kill, not an
		// answer, so it is recorded as the fixed exhausted-bound status and
		// the timed-out divergence carries the finding.
		observation.ExitCode = exitCodeExhausted
	case err == nil:
		observation.ExitCode = 0
	case errors.As(err, &exit):
		observation.ExitCode = exit.ExitCode()
	default:
		observation.ExitCode = exitCodeUnstarted
		if observation.Stderr != "" {
			observation.Stderr += "\n"
		}
		observation.Stderr += err.Error()
	}
	return observation
}
