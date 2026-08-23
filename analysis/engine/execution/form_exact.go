// form_exact.go owns the E form: one exact read folded onto one exact write.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// classifyExactForm claims a descriptor with exactly one sealed exact join and
// an exact output: the read port is the form's only coordinate. A single join
// declaring any other read form belongs to the form that executes that read,
// so the read form is part of the claim and not an assumption.
func classifyExactForm(rule generated.CompiledRule) (FormRow, bool) {
	mode, modeOK := rule.OutputMode()
	if !modeOK || mode != ruleprogram.ModeExact || rule.ReadCount() != 1 {
		return FormRow{}, false
	}
	if form, ok := rule.ReadFormAt(0); !ok || form != ruleprogram.Exact {
		return FormRow{}, false
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

// FoldExact is the one-read identity fold: the first exact cell is staged to
// the write axis, sparse absence and an empty cursor are NoCandidate, and any
// other cursor failure is Refuse. Ticket remains open for Submit.
func FoldExact[K scalar.Key, V any](ticket Ticket, read ExactRead[K, V], write ExactWrite[K, V], scratch *Scratch[K, V]) structure.ReductionOutcome {
	if scratch == nil || !read.Valid() || !write.Valid() {
		return structure.Refuse
	}
	switch read.Read(ticket, scratch) {
	case ReadAvailable:
		region, regionOK := scratch.Region()
		value, valueOK := scratch.Value()
		present := scratch.Present()
		if !read.Close(ticket, scratch) {
			_ = scratch.Discard(ticket)
			return structure.Refuse
		}
		if !present {
			return structure.NoCandidate
		}
		if regionOK && valueOK && write.Stage(ticket, scratch, region, value) && write.Close(ticket, scratch) {
			return structure.Concrete
		}
		_ = scratch.Discard(ticket)
		return structure.Refuse
	case ReadExhausted:
		if read.Close(ticket, scratch) {
			return structure.NoCandidate
		}
		return structure.Refuse
	default:
		_ = scratch.Discard(ticket)
		return structure.Refuse
	}
}
