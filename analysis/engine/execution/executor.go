// executor.go declares the one dynamic dispatch seam every execution form is
// reached through. Form-specific rows, families, and folds live in their own
// form_<name>.go child file.

package execution

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// Executor is the one dynamic dispatch seam. Implementations are sealed typed
// families, never one closure or interface object per member. The opaque
// Frame/Ticket boundary keeps carrier state and domain values private.
type Executor interface {
	Execute(Frame, Ticket) (Result, bool)
}

// Family is the sealed static half of one typed rule family. It owns immutable
// descriptors only; each epoch asks it for one worker Executor with private
// scratch. This keeps schema/program compilation outside the solve loop.
type Family interface {
	NewExecutor(*Run) Executor
	InputCapacity() int
	OutputCapacity() int
}

type Frame struct {
	run    *Run
	serial uint64
}

func NewFrame(ticket Ticket) (Frame, bool) {
	if !ticket.Valid() {
		return Frame{}, false
	}
	return Frame{run: ticket.issuer, serial: ticket.serial}, true
}

func (frame Frame) Valid(ticket Ticket) bool {
	return frame.run != nil && frame.serial != 0 && ticket.Valid() && ticket.issuer == frame.run && ticket.serial == frame.serial
}

// Result is completion metadata only. Patches remain owned by Run and reach
// the solver only through Consume.
type Result struct {
	outcome structure.ReductionOutcome
	count   uint16
}

func NewResult(outcome structure.ReductionOutcome, count int) (Result, bool) {
	if !outcome.Available() || count < 0 || count > int(^uint16(0)) || outcome != structure.Concrete && count != 0 {
		return Result{}, false
	}
	return Result{outcome: outcome, count: uint16(count)}, true
}
func (result Result) Valid() bool {
	return result.outcome.Available() && (result.outcome == structure.Concrete || result.count == 0)
}
func (result Result) Outcome() structure.ReductionOutcome { return result.outcome }
func (result Result) Count() int                          { return int(result.count) }
