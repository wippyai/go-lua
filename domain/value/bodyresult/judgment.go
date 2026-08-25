// Package bodyresult owns Value's executable-body call-result judgment and the
// family its declaration is emitted into. The rule reads the Call fact of the
// mounted occurrence its candidate was sealed for, observes the canonical first
// return member of every executable body that fact dispatches to, and publishes
// their join at the call-result coordinate Value already issued.
package bodyresult

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/domain/value/bodyresult/returnroute"
)

// Judgment is the sealed semantic state of the body-result rule: the two cold
// schemas its answer rests on.
//
// It is the family's state, not a rule payload. Both are cold and immutable
// for the life of the binding they were issued by, so the state is sealed once
// when the family is installed and every invocation reads it. Neither is ever a
// parameter of the fold: the fold takes the result slot it is indexed by, the
// one Call fact it read, and the return members its selection observed, and
// nothing else.
type Judgment struct {
	values *valuedomain.Schema
	calls  *calldomain.Algebra
}

// Derive seals the judgment against the two schemas the declaration names.
// They must belong to one Link: a mounted call joins a Value result slot to a
// Call row, and two owners of different Links have no such row in common.
func Derive(values *valuedomain.Schema, calls *calldomain.Algebra) (Judgment, bool) {
	judgment := Judgment{values: values, calls: calls}
	if !judgment.Valid() {
		return Judgment{}, false
	}
	return judgment, true
}

// Valid reports whether this state was sealed by Derive.
func (judgment Judgment) Valid() bool {
	return judgment.values != nil && judgment.values.Valid() &&
		judgment.calls != nil && judgment.calls.Valid() &&
		judgment.values.LinkOwner().Matches(judgment.calls.LinkOwner())
}

// Result is the one irreducible judgment of the body-result rule: the Value
// fact one mounted call's first result publishes, given the bodies that call
// dispatches to and the first return member each of them publishes.
//
// Evidence beyond enumeration answers Top. A site that reaches no executable
// body carries no evidence of this kind and settles as an absent candidate. A
// reached return that publishes no value contributes nil, every observed member
// is authenticated against the tag it was correlated by and the coordinate it
// was read at, and their join is the returned result. An answer that reduces to
// Bottom is no candidate rather than a published empty fact.
func (judgment Judgment) Result(
	candidate valuedomain.MountedCallResultSlot,
	dispatched calldomain.Value,
	cells []execution.SelectedCell[valuedomain.Value],
) (valuedomain.Value, structure.ReductionOutcome) {
	if !judgment.Valid() {
		return valuedomain.Value{}, structure.Refuse
	}
	selection, selectionOK := returnroute.Select(judgment.values, judgment.calls, candidate, dispatched)
	if !selectionOK {
		return valuedomain.Value{}, structure.Refuse
	}
	if selection.Top() {
		return judgment.values.Top(), structure.Concrete
	}
	if !selection.HasBody() {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	if len(cells) != len(selection.Tags()) {
		return valuedomain.Value{}, structure.Refuse
	}
	combined := judgment.values.Bottom()
	presentAny := false
	if selection.NilCase() {
		nilOK := false
		combined, nilOK = judgment.values.Nil()
		if !nilOK {
			return valuedomain.Value{}, structure.Refuse
		}
		presentAny = true
	}
	seen := make(map[uint64]struct{}, len(cells))
	for _, cell := range cells {
		if !selection.Contains(cell.Tag) {
			return valuedomain.Value{}, structure.Refuse
		}
		if _, duplicate := seen[cell.Tag]; duplicate {
			return valuedomain.Value{}, structure.Refuse
		}
		seen[cell.Tag] = struct{}{}
		coordinate, coordinateOK := judgment.values.CoordinateAt(int(cell.Tag - 1))
		if !coordinateOK || cell.Present && !judgment.values.AdmitsCoordinate(coordinate, cell.Value) {
			return valuedomain.Value{}, structure.Refuse
		}
		if !cell.Present {
			continue
		}
		if !presentAny {
			combined, presentAny = cell.Value, true
			continue
		}
		joined := false
		combined, joined = judgment.values.Join(combined, cell.Value)
		if !joined {
			return valuedomain.Value{}, structure.Refuse
		}
	}
	if !presentAny || combined.IsBottom() {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	return combined, structure.Concrete
}
