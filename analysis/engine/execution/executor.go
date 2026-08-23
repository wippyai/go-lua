package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
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

// ExactRow is static typed row data for the E form. It contains only sealed
// binding/unit/target/port descriptors and no live Run or callback.
type ExactRow[K scalar.Key, V any] struct {
	read  ExactRead[K, V]
	write ExactWrite[K, V]
}

func NewExactRow[K scalar.Key, V any](binding *factbinding.Binding[K, V], unit carrier.Unit, input uint16, target carrier.Target, output uint16) (ExactRow[K, V], bool) {
	read, readOK := NewExactRead(binding, unit, input)
	write, writeOK := NewExactWrite(binding, target, output)
	if !readOK || !writeOK {
		return ExactRow[K, V]{}, false
	}
	return ExactRow[K, V]{read: read, write: write}, true
}

type exactFamily[K scalar.Key, V any] struct{ rows []ExactRow[K, V] }
type exactWorker[K scalar.Key, V any] struct {
	family  *exactFamily[K, V]
	run     *Run
	scratch Scratch[K, V]
}

// NewExactFamily builds one reusable executor for one typed rule family.
func NewExactFamily[K scalar.Key, V any](rows []ExactRow[K, V]) (Family, bool) {
	if len(rows) == 0 {
		return nil, false
	}
	sealed := append([]ExactRow[K, V](nil), rows...)
	for _, row := range sealed {
		if !row.read.Valid() || !row.write.Valid() {
			return nil, false
		}
	}
	return &exactFamily[K, V]{rows: sealed}, true
}

func (family *exactFamily[K, V]) NewExecutor(run *Run) Executor {
	if family == nil || run == nil {
		return nil
	}
	return &exactWorker[K, V]{family: family, run: run}
}
func (*exactFamily[K, V]) InputCapacity() int  { return 1 }
func (*exactFamily[K, V]) OutputCapacity() int { return 1 }

func (worker *exactWorker[K, V]) Execute(frame Frame, ticket Ticket) (Result, bool) {
	if worker == nil || worker.family == nil || worker.run == nil || !frame.Valid(ticket) || ticket.issuer != worker.run {
		return Result{}, false
	}
	_, local, localOK := ticket.familyLocal()
	if !localOK || uint64(local) >= uint64(len(worker.family.rows)) {
		return Result{}, false
	}
	row := worker.family.rows[local]
	outcome := FoldExact(ticket, row.read, row.write, &worker.scratch)
	if !ticket.Submit(outcome) {
		return Result{}, false
	}
	count := 0
	if outcome == structure.Concrete {
		count = 1
	}
	return NewResult(outcome, count)
}

// SourceRow is static typed row data for the Z form. The owner materialized
// column is copied at seal; the runtime indexes it only by the ticket's issued
// candidate ordinal.
type SourceRow[K scalar.Key, V any] struct {
	write  ExactWrite[K, V]
	column memberrelation.SourceColumn[V]
}

func NewSourceRow[K scalar.Key, V any](binding *factbinding.Binding[K, V], target carrier.Target, output uint16, column memberrelation.SourceColumn[V]) (SourceRow[K, V], bool) {
	write, writeOK := NewExactWrite(binding, target, output)
	if !writeOK || !column.Valid() {
		return SourceRow[K, V]{}, false
	}
	// The column has already been copied from the cold owner into the sealed
	// bound Factor. A family borrows that immutable Program value; cloning here
	// once per candidate would turn source lowering into quadratic retention.
	return SourceRow[K, V]{write: write, column: column}, true
}

type sourceFamily[K scalar.Key, V any] struct{ rows []SourceRow[K, V] }
type sourceWorker[K scalar.Key, V any] struct {
	family  *sourceFamily[K, V]
	run     *Run
	scratch Scratch[K, V]
}

func NewSourceFamily[K scalar.Key, V any](rows []SourceRow[K, V]) (Family, bool) {
	if len(rows) == 0 {
		return nil, false
	}
	sealed := append([]SourceRow[K, V](nil), rows...)
	for index := range sealed {
		if !sealed[index].write.Valid() || !sealed[index].column.Valid() {
			return nil, false
		}
	}
	return &sourceFamily[K, V]{rows: sealed}, true
}

func (family *sourceFamily[K, V]) NewExecutor(run *Run) Executor {
	if family == nil || run == nil {
		return nil
	}
	return &sourceWorker[K, V]{family: family, run: run}
}
func (*sourceFamily[K, V]) InputCapacity() int  { return 0 }
func (*sourceFamily[K, V]) OutputCapacity() int { return 1 }

func (worker *sourceWorker[K, V]) Execute(frame Frame, ticket Ticket) (Result, bool) {
	if worker == nil || worker.family == nil || worker.run == nil || !frame.Valid(ticket) || ticket.issuer != worker.run || ticket.InputCount() != 0 {
		return Result{}, false
	}
	_, local, localOK := ticket.familyLocal()
	candidate, candidateOK := ticket.CandidateOrdinal()
	if !localOK || !candidateOK || uint64(local) >= uint64(len(worker.family.rows)) {
		return Result{}, false
	}
	row := worker.family.rows[local]
	value, valueOK := row.column.At(candidate)
	if !valueOK {
		return Result{}, false
	}
	_, _, within, contextOK := row.write.context(ticket)
	if !contextOK || !row.write.Stage(ticket, &worker.scratch, within, value) || !row.write.Close(ticket, &worker.scratch) || !ticket.Submit(structure.Concrete) {
		return Result{}, false
	}
	return NewResult(structure.Concrete, 1)
}
