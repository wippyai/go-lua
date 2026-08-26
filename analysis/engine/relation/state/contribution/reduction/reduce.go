package reduction

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/contribution"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Reduce derives one destination aggregate from the exact producer rows for
// target.  It performs no state mutation and no identity derivation.
//
// The rows are copied and canonically ordered by their already-issued
// contribution handles before folding.  Therefore callers may provide a
// permutation of the same authenticated rows without changing the result;
// mixed targets, mixed destination cells, foreign directories/fences,
// duplicate producer handles, malformed lineages, and unsupported presence
// contracts refuse.
//
// An empty row slice is refused by Reduce because the current Target carries
// only a RowID and no CellToken can be invented for that case.  Use ReduceAt
// when the caller has the target's exact owner-issued destination cell.
func Reduce(target contribution.Target, rows []contribution.Row, mounted witness.Mounted, spec output.ContributionSpec) (Aggregate, bool) {
	if len(rows) == 0 {
		return Aggregate{}, false
	}

	return reduceAt(target, rows, binding.CellToken{}, mounted, spec, false)
}

// ReduceAt is the exact empty-target entry point.  destination must be the
// owner-issued CellToken for target's relation/column/row and the mounted
// runtime.  It is mandatory even for a sparse result because a reducer may
// never reconstruct a destination from RowID or output-column identity.
func ReduceAt(target contribution.Target, rows []contribution.Row, destination binding.CellToken, mounted witness.Mounted, spec output.ContributionSpec) (Aggregate, bool) {
	return reduceAt(target, rows, destination, mounted, spec, true)
}

func reduceAt(target contribution.Target, rows []contribution.Row, destination binding.CellToken, mounted witness.Mounted, spec output.ContributionSpec, explicitDestination bool) (Aggregate, bool) {
	if !validContext(target, mounted, spec) || !contributionsReducer(spec) {
		return Aggregate{}, false
	}
	if !presentOrOpaque(spec.Presence()) {
		// ProduceOptional and ProduceAbsent are not accepted until a closed-
		// world producer denominator is sealed.  A mere output denominator
		// does not prove that every producer was observed.
		return Aggregate{}, false
	}
	if len(rows) == 0 {
		if !explicitDestination || !destination.ValidFor(mounted.RuntimeFence()) || destination.Column() != target.Port.Column || destination.Row() != target.Destination {
			return Aggregate{}, false
		}
		return Aggregate{target: target, destination: destination, removal: true, sealed: true}, true
	}

	ordered, cell, ok := validateRows(target, rows, mounted, spec, destination, explicitDestination)
	if !ok {
		return Aggregate{}, false
	}

	lineageAuthority, ok := mounted.Lineage()
	if !ok || lineageAuthority == nil {
		return Aggregate{}, false
	}
	lineage := ordered[0].Lineage
	if !lineageAuthority.Validate(lineage) {
		return Aggregate{}, false
	}
	for index := 1; index < len(ordered); index++ {
		joined, joinOK := lineageAuthority.Join(lineage, ordered[index].Lineage)
		if !joinOK || !joined.Available() || !lineageAuthority.Validate(joined) {
			return Aggregate{}, false
		}
		lineage = joined
	}

	var value binding.ValueToken
	switch spec.Presence() {
	case signature.ProducePresent:
		algebra, algebraOK := mounted.Algebra(spec.ValueType())
		capability, capabilityOK := mounted.TypeCapability(spec.ValueType())
		if !algebraOK || algebra == nil || !capabilityOK || !capability.Equal(spec.Algebra()) || !capability.Ascending() {
			return Aggregate{}, false
		}
		value = ordered[0].Value
		if !algebra.LessOrEqual(value, value) {
			return Aggregate{}, false
		}
		for index := 1; index < len(ordered); index++ {
			joined, joinOK := algebra.Join(value, ordered[index].Value)
			if !joinOK || !joined.ValidFor(mounted.RuntimeFence()) || joined.Type() != spec.ValueType() {
				return Aggregate{}, false
			}
			if !algebra.LessOrEqual(value, joined) || !algebra.LessOrEqual(ordered[index].Value, joined) {
				return Aggregate{}, false
			}
			value = joined
		}
	case signature.ProduceOpaque:
		value = ordered[0].Value
		for index := 1; index < len(ordered); index++ {
			if !value.Same(ordered[index].Value) {
				return Aggregate{}, false
			}
		}
	default:
		return Aggregate{}, false
	}

	presence, presenceOK := model.NewPresence(presenceFor(spec.Presence()))
	if !presenceOK || !spec.Presence().Allows(presence) {
		return Aggregate{}, false
	}
	result := Aggregate{target: target, destination: cell, value: value, presence: presence, lineage: lineage, sealed: true}
	if !result.Available() {
		return Aggregate{}, false
	}
	return result, true
}

func validContext(target contribution.Target, mounted witness.Mounted, spec output.ContributionSpec) bool {
	if !target.Available() || !mounted.Available() || !spec.Available() || spec.Port() != target.Port || spec.Column() != target.Port.Column || !spec.ValueType().Available() || !spec.Algebra().Available() {
		return false
	}
	capability, ok := mounted.TypeCapability(spec.ValueType())
	return ok && capability.Equal(spec.Algebra())
}

func contributionsReducer(spec output.ContributionSpec) bool {
	return spec.Reducer() == output.Contributions
}

func presentOrOpaque(contract signature.PresenceContract) bool {
	return contract == signature.ProducePresent || contract == signature.ProduceOpaque
}

func presenceFor(contract signature.PresenceContract) model.PresenceKind {
	if contract == signature.ProduceOpaque {
		return model.AuthenticatedOpaque
	}
	return model.Present
}

func validateRows(target contribution.Target, rows []contribution.Row, mounted witness.Mounted, spec output.ContributionSpec, explicitCell binding.CellToken, explicitDestination bool) ([]contribution.Row, binding.CellToken, bool) {
	ordered := append([]contribution.Row(nil), rows...)
	var cell binding.CellToken
	if explicitDestination {
		if !explicitCell.ValidFor(mounted.RuntimeFence()) || explicitCell.Column() != target.Port.Column || explicitCell.Row() != target.Destination {
			return nil, binding.CellToken{}, false
		}
		cell = explicitCell
	}
	var directory contribution.Handle
	haveDirectory := false
	for _, row := range ordered {
		if !row.ValidFor(mounted.RuntimeFence()) || !row.Target().Same(target) || row.Key.Port != spec.Port() || row.Key.Destination != target.Destination || row.Destination.Column() != target.Port.Column || row.Destination.Row() != target.Destination {
			return nil, binding.CellToken{}, false
		}
		if !spec.Presence().Allows(row.Presence) {
			return nil, binding.CellToken{}, false
		}
		if row.Presence.Is(model.Present) || row.Presence.Is(model.AuthenticatedOpaque) {
			if !row.Value.ValidFor(mounted.RuntimeFence()) || row.Value.Type() != spec.ValueType() {
				return nil, binding.CellToken{}, false
			}
		}
		if !haveDirectory {
			directory = row.Key.Invocation
			haveDirectory = true
		} else if !directory.SameDirectory(row.Key.Invocation) {
			return nil, binding.CellToken{}, false
		}
		if !cell.Available() {
			cell = row.Destination
		} else if !cell.Same(row.Destination) {
			return nil, binding.CellToken{}, false
		}
	}

	sort.Slice(ordered, func(left, right int) bool {
		result, ok := contribution.CompareHandles(ordered[left].Key.Invocation, ordered[right].Key.Invocation)
		return ok && result < 0
	})
	for index := 1; index < len(ordered); index++ {
		result, ok := contribution.CompareHandles(ordered[index-1].Key.Invocation, ordered[index].Key.Invocation)
		if !ok || result >= 0 {
			// Equal handles are duplicate producers.  They cannot be folded as
			// two contributions, even if their payloads happen to match.
			return nil, binding.CellToken{}, false
		}
	}
	return ordered, cell, cell.Available()
}
