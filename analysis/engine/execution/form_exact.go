// form_exact.go owns the E form: an exact product folded onto one exact write.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// classifyExactForm claims a descriptor whose joins are all exact and whose
// output is exact. A typed family owns products wider than the built-in
// one-read identity fold, but classification remains one engine decision.
func classifyExactForm(rule generated.CompiledRule) (FormRow, bool) {
	mode, modeOK := rule.OutputMode()
	if !modeOK || mode != ruleprogram.ModeExact || rule.ReadCount() == 0 {
		return FormRow{}, false
	}
	for index := 0; index < rule.ReadCount(); index++ {
		read, ok := rule.ReadAt(index)
		if !ok || read.Form != ruleprogram.Exact || read.Input >= uint32(rule.InputCount()) || read.Input > uint32(^uint16(0)) {
			return FormRow{}, false
		}
	}
	// A carry is part of the claim: identity hands the prior output fact on
	// unchanged, which this fold does by writing nothing else, while a
	// transformed carry owes a domain call this form cannot make.
	if carry, present := rule.CarryMode(); present && carry != ruleprogram.CarryIdentity {
		return FormRow{}, false
	}
	input := rule.ReadInput()
	if input < 0 || input >= rule.InputCount() || input > int(^uint16(0)) || rule.InputCount() <= 0 {
		return FormRow{}, false
	}
	return FormRow{Form: FormExact, Input: uint16(input)}, true
}

// buildExactForm seals one typed E family from this Factor's exact rows. Rows
// keep their discovery order, so a member's local ordinal is its position in
// the Factor's exact ladder.
func buildExactForm[K scalar.Key, V any](plane FormPlane[K, V], rows []FormRow) (Family, []FormAddress, bool) {
	if !plane.Valid() || len(rows) == 0 {
		return nil, nil, false
	}
	sealed := make([]ExactRow[K, V], 0, len(rows))
	addresses := make([]FormAddress, 0, len(rows))
	for _, row := range rows {
		if row.Rule.Available() && row.Rule.ReadCount() != 1 {
			return nil, nil, false
		}
		exact, ok := NewExactRow(plane.binding, row.Unit, row.Input, row.Target, 0)
		if !ok {
			return nil, nil, false
		}
		addresses = append(addresses, FormAddress{Member: row.Member, Local: uint32(len(sealed))})
		sealed = append(sealed, exact)
	}
	family, ok := NewExactFamily(sealed)
	if !ok {
		return nil, nil, false
	}
	return family, addresses, true
}

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
	family       *exactFamily[K, V]
	run          *Run
	readScratch  Scratch[K, V]
	writeScratch Scratch[K, V]
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
	outcome, valid := FoldExact(ticket, row.read, row.write, &worker.readScratch, &worker.writeScratch)
	if !valid || !ticket.Submit(outcome) {
		return Result{}, false
	}
	count := 0
	if outcome == structure.Concrete {
		count = 1
	}
	return NewResult(outcome, count)
}

// FoldExact consumes the complete exact cursor, staging every present row
// into one write transaction. Sparse rows are omitted; an entirely sparse or
// empty cursor is a valid NoCandidate. The boolean is structural validity:
// Refuse is never submitted as a semantic reduction outcome.
func FoldExact[K scalar.Key, V any](ticket Ticket, read ExactRead[K, V], write ExactWrite[K, V], readScratch, writeScratch *Scratch[K, V]) (structure.ReductionOutcome, bool) {
	within, withinOK := ticket.Within()
	if !ticket.Valid() || !ticket.Checkpoint() || !withinOK || !within.Valid() || !read.Valid() || !write.Valid() || readScratch == nil || writeScratch == nil {
		return structure.Refuse, false
	}
	// Empty support still authenticates the complete declared operation. It is
	// a valid sparse result only when this ticket owns both the read port and
	// output slot; otherwise returning NoCandidate would compensate for a
	// malformed family row that happened not to execute.
	_, _, _, readOK := read.context(ticket)
	_, _, _, writeOK := write.context(ticket)
	if !readOK || !writeOK || int(write.output) >= ticket.OutputCount() {
		return structure.Refuse, false
	}
	if support.Empty(within) {
		return structure.NoCandidate, true
	}
	observed := false
	staged := false
	for {
		if !ticket.Checkpoint() {
			_ = readScratch.Discard(ticket)
			_ = writeScratch.Discard(ticket)
			return structure.Refuse, false
		}
		switch read.Read(ticket, readScratch) {
		case ReadAvailable:
			observed = true
			region, regionOK := readScratch.Region()
			value, valueOK := readScratch.Value()
			if !regionOK || !valueOK {
				_ = readScratch.Discard(ticket)
				_ = writeScratch.Discard(ticket)
				return structure.Refuse, false
			}
			if readScratch.Present() {
				if !write.Stage(ticket, writeScratch, region, value) {
					_ = readScratch.Discard(ticket)
					_ = writeScratch.Discard(ticket)
					return structure.Refuse, false
				}
				staged = true
			}
		case ReadExhausted:
			if !read.Close(ticket, readScratch) {
				_ = readScratch.Discard(ticket)
				_ = writeScratch.Discard(ticket)
				return structure.Refuse, false
			}
			if !observed {
				_ = readScratch.Discard(ticket)
				_ = writeScratch.Discard(ticket)
				return structure.Refuse, false
			}
			if !staged {
				return structure.NoCandidate, true
			}
			if !write.Close(ticket, writeScratch) {
				_ = writeScratch.Discard(ticket)
				return structure.Refuse, false
			}
			return structure.Concrete, true
		default:
			_ = readScratch.Discard(ticket)
			_ = writeScratch.Discard(ticket)
			return structure.Refuse, false
		}
	}
}
