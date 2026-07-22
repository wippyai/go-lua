package equation

import "fmt"

// AccessRecord is the dynamic audit payload recorded by an existing bound
// kernel.  The compiler never inspects it to influence evaluation.
type AccessRecord struct {
	Reads, Writes, Advances, Outcomes, Diagnostics, Dependencies []string
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
	if !execution.Complete && execution.Published {
		return fmt.Errorf("equation: partial transaction published")
	}
	if err := verify(execution.Access); err != nil {
		return fmt.Errorf("equation: access audit: %w", err)
	}
	return nil
}
