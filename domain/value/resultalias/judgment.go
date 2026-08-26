// Package resultalias owns Value's selected Target ResultAlias judgment and
// the family its declaration is emitted into. The rule reads the Call fact of
// the mounted occurrence its candidate was sealed for, observes the actuals
// the operations that fact selects alias the first result to, and publishes
// the joined image at the call-result coordinate Value already issued.
package resultalias

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/domain/value/resultalias/aliasroute"
)

// Judgment is the sealed semantic state of the result-alias rule: the three
// cold schemas its answer rests on.
//
// It is the family's state, not a rule payload. All three are cold and
// immutable for the life of the binding they were issued by, so the state is
// sealed once when the family is installed and every invocation reads it.
// None of them is ever a parameter of the fold: the fold takes the result slot
// it is indexed by, the one Call fact it read, and the actuals its selection
// observed, and nothing else.
type Judgment struct {
	values *valuedomain.Schema
	calls  *calldomain.Algebra
	packs  *packdomain.Schema
}

// Derive seals the judgment against the three schemas the declaration names.
// They must belong to one Link: a mounted call joins a Value result slot, a
// Call row and a Pack actual list, and owners of different Links have no such
// row in common.
func Derive(values *valuedomain.Schema, calls *calldomain.Algebra, packs *packdomain.Schema) (Judgment, bool) {
	judgment := Judgment{values: values, calls: calls, packs: packs}
	if !judgment.Valid() {
		return Judgment{}, false
	}
	return judgment, true
}

// Valid reports whether this state was sealed by Derive.
func (judgment Judgment) Valid() bool {
	return judgment.values != nil && judgment.values.Valid() &&
		judgment.calls != nil && judgment.calls.Valid() && judgment.packs != nil &&
		judgment.values.LinkOwner().Matches(judgment.calls.LinkOwner()) &&
		judgment.packs.LinkOwner().Matches(judgment.calls.LinkOwner())
}

// Result is the one irreducible judgment of the result-alias rule: the Value
// fact one mounted call's first result publishes, given the targets that call
// dispatches to and the actuals its selected operations alias that result to.
//
// Evidence beyond enumeration answers Top. A site whose selected operations
// declare no result-zero alias carries no evidence of this kind and settles as
// an absent candidate. Otherwise every observed actual is authenticated
// against the formal its tag names and against the coordinate it was read at,
// and their join is the aliased result. An answer that reduces to Bottom is no
// candidate rather than a published empty fact.
func (judgment Judgment) Result(
	candidate valuedomain.MountedCallResultSlot,
	dispatched calldomain.Value,
	cells []operand.SelectedCell[valuedomain.Value],
) (valuedomain.Value, structure.ReductionOutcome) {
	if !judgment.Valid() || !judgment.values.OwnsMountedCallResultSlot(candidate) {
		return valuedomain.Value{}, structure.Refuse
	}
	selection, actual, selectionOK := aliasroute.Select(judgment.values, judgment.calls, judgment.packs, candidate, dispatched)
	if !selectionOK {
		return valuedomain.Value{}, structure.Refuse
	}
	if selection.Top() {
		return judgment.values.Top(), structure.Concrete
	}
	if !selection.Aliased() {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	sources := selection.Sources()
	if len(cells) != len(sources) {
		return valuedomain.Value{}, structure.Refuse
	}
	combined := judgment.values.Bottom()
	presentAny := false
	seen := make([]bool, len(sources))
	for _, cell := range cells {
		if cell.Tag == 0 || cell.Tag-1 > uint64(^uint32(0)) {
			return valuedomain.Value{}, structure.Refuse
		}
		source := uint32(cell.Tag - 1)
		index := sourceOrdinalIndex(sources, source)
		if index < 0 || seen[index] {
			return valuedomain.Value{}, structure.Refuse
		}
		// Selection order is the order the engine canonicalizes members by,
		// not authored ordinal order. Track the canonical tag set directly
		// instead of replaying prior members.
		seen[index] = true
		if !cell.Present {
			continue
		}
		semantic, semanticOK := actual.ActualAt(int(source))
		coordinate, coordinateOK := judgment.values.CoordinateForMountedSemantic(semantic.Module(), semantic.ID())
		if !semanticOK || !coordinateOK || !judgment.values.AdmitsCoordinate(coordinate, cell.Value) {
			return valuedomain.Value{}, structure.Refuse
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

// sourceOrdinalIndex locates one authored formal ordinal in the strictly
// ascending set the selection named.
func sourceOrdinalIndex(sources []uint32, source uint32) int {
	index := sort.Search(len(sources), func(index int) bool { return sources[index] >= source })
	if index >= len(sources) || sources[index] != source {
		return -1
	}
	return index
}
