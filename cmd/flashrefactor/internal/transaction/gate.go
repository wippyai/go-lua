package transaction

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

// ErrSafetyFailure marks a bounded test which exceeded the mandatory safety
// limit (the runner's exit status 125).  Callers must stop rather than retry
// or continue later gates.
var ErrSafetyFailure = errors.New("bounded test safety failure")

// GateError retains the structured request and output of a failed semantic
// gate.  It deliberately contains no shell command supplied by a caller.
type GateError struct {
	Spec   cutplan.Law
	Output string
	Err    error
}

func (e *GateError) Error() string {
	return fmt.Sprintf("bounded semantic law %s %s: %v", e.Spec.Package, e.Spec.Test, e.Err)
}

func (e *GateError) Unwrap() error { return e.Err }

// RunGates invokes only the repository bounded runner, with the one allowed
// argument shape.  It does not accept shell text, environment overrides,
// limits, extra test flags, or an alternate executable.
func RunGates(root string, specs []cutplan.Law) error {
	canonicalRoot, err := canonicalRoot(root)
	if err != nil {
		return err
	}
	runner := filepath.Join(canonicalRoot, "scripts", "bounded_test.sh")
	info, err := os.Lstat(runner)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repository bounded runner is not a regular file")
	}
	return runGates(canonicalRoot, specs)
}

func runGates(root string, specs []cutplan.Law) error {
	runner := filepath.Join(root, "scripts", "bounded_test.sh")
	info, err := os.Lstat(runner)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("bounded runner is not a regular file")
	}
	for _, spec := range specs {
		if err := validateLaw(spec); err != nil {
			return err
		}
		args := []string{"go", "test", "-json", spec.Package, "-run", "^" + spec.Test + "$", "-count=1"}
		command := exec.Command(runner, args...)
		command.Dir = root
		output, err := command.CombinedOutput()
		if err == nil {
			if parseErr := verifyLawJSON(spec, output); parseErr == nil {
				continue
			} else {
				return &GateError{Spec: spec, Output: string(output), Err: parseErr}
			}
		}
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 125 {
			return &GateError{Spec: spec, Output: string(output), Err: ErrSafetyFailure}
		}
		return &GateError{Spec: spec, Output: string(output), Err: err}
	}
	return nil
}

func validateLaw(spec cutplan.Law) error {
	if spec.ID == "" || !token.IsIdentifier(spec.ID) || !cutplan.ConcretePackage(spec.Package) || !token.IsIdentifier(spec.Test) || len(spec.Test) < 5 || spec.Test[:4] != "Test" {
		return fmt.Errorf("invalid bounded semantic test")
	}
	return nil
}

type goTestEvent struct{ Action, Package, Test string }

// verifyLawJSON makes a passed process insufficient: the bounded invocation
// must have run exactly the reviewed top-level name and its package must pass.
func verifyLawJSON(spec cutplan.Law, output []byte) error {
	var runs, passes int
	packagePass := false
	packageID := ""
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		var event goTestEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		} // runner accounting
		if event.Test == spec.Test {
			if event.Package == "" {
				return fmt.Errorf("law JSON omits package identity for %s", spec.Test)
			}
			if packageID == "" {
				packageID = event.Package
			} else if event.Package != packageID {
				return fmt.Errorf("law JSON changes package identity: %s then %s", packageID, event.Package)
			}
			switch event.Action {
			case "run":
				runs++
			case "pass":
				passes++
			}
			continue
		}
		if packageID != "" && event.Package == packageID && event.Test != "" && (event.Action == "run" || event.Action == "pass") && event.Test != spec.Test && strings.HasPrefix(event.Test, spec.Test) {
			return fmt.Errorf("law emitted non-top-level test name %q", event.Test)
		}
		if packageID != "" && event.Package == packageID && event.Test == "" && event.Action == "pass" {
			packagePass = true
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read go test JSON: %w", err)
	}
	if runs != 1 || passes != 1 || !packagePass {
		return fmt.Errorf("law JSON requires exactly one top-level run/pass and package pass: run=%d pass=%d package-pass=%t", runs, passes, packagePass)
	}
	return nil
}
