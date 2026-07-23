package equation

import (
	"errors"
	"fmt"
)

// ErrPartialExecution means a kernel returned a partial Execution without an
// error.  That violates the execution boundary: callers may only observe a
// complete result or an explicit failure.
var ErrPartialExecution = errors.New("equation: partial execution without error")

// AccessRecord is the dynamic audit payload recorded by an existing bound
// kernel.  The compiler never inspects it to influence evaluation.
type AccessRecord struct {
	Reads, Writes, Advances, Outcomes, Diagnostics, Dependencies []string
	// Payload retains the source owner's exact audit record.  It is opaque to
	// the equation package so audit plumbing cannot reinterpret a contract or
	// grow an alternate access vocabulary.
	Payload any
}

// Execution is the result recorded around a canonical kernel call.  Complete
// distinguishes a legitimate complete bottom/result from a failed partial
// transaction.  A partial result is never publishable.
type Execution struct {
	Complete  bool
	Published bool
	Access    AccessRecord
}

// RunAndVerify executes a pre-existing bound kernel and then verifies its
// audit record.  Audit verification runs after execution and cannot select a
// branch, alter a result, or provide a semantic fallback.
func RunAndVerify(run func() (Execution, error), verify func(AccessRecord) error) error {
	if run == nil || verify == nil {
		return fmt.Errorf("equation: audit harness is incomplete")
	}
	execution, err := run()
	if err != nil {
		return err
	}
	if !execution.Complete {
		return ErrPartialExecution
	}
	if err := verify(execution.Access); err != nil {
		return fmt.Errorf("equation: access audit: %w", err)
	}
	return nil
}
