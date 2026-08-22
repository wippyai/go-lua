package directfunction

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// populateSelectedBodyPaths seals the exact closure consumed by diagnostic
// conformance. Non-callable bodies are roots; each retained direct Function
// Call adds its target body, and the process repeats to a fixed point. The
// owner stores only issued BodyPath identities, sorted for allocation-free
// membership queries.
func populateSelectedBodyPaths(result *Result, flow authored.View, bodies *body.Result, bodyPaths *semanticpath.VertexCatalogPaths, counts [keyspace.FamilyCount]uint32) error {
	if result == nil || bodyPaths == nil || bodies == nil || bodyPaths.BodyCount() != bodies.BodyCount() || bodyPaths.BodyCount() != int(counts[keyspace.FamilyBody]) {
		return errors.New("program/flow/directfunction: BodyPath denominator is unavailable")
	}
	functions := flow.Functions()
	callable := make([]bool, bodies.BodyCount()+1)
	for index := 0; index < functions.Count(); index++ {
		function, functionOK := functions.At(index)
		owner, functionBody, _, rowOK := functions.Get(function)
		ownerOrdinal := keyspace.TermOrdinal(owner)
		bodyOrdinal := keyspace.TermOrdinal(functionBody)
		if !functionOK || !rowOK || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermFamily(functionBody) != keyspace.FamilyBody ||
			ownerOrdinal == 0 || bodyOrdinal == 0 || uint64(ownerOrdinal) > uint64(bodies.BodyCount()) || uint64(bodyOrdinal) > uint64(bodies.BodyCount()) {
			return errors.New("program/flow/directfunction: Function Body path owner is unavailable")
		}
		callable[bodyOrdinal] = true
	}
	selected := make([]bool, len(callable))
	selectedCount := 0
	for ordinal := 1; ordinal < len(callable); ordinal++ {
		if callable[ordinal] {
			continue
		}
		selected[ordinal] = true
		selectedCount++
	}
	if selectedCount == 0 {
		return errors.New("program/flow/directfunction: direct-call closure has no root Body")
	}
	calls := flow.Calls()
	for changed := true; changed; {
		changed = false
		for index := 0; index < calls.Count(); index++ {
			call, callOK := calls.At(index)
			owner, _, _, _, rowOK := calls.Get(call)
			ownerOrdinal := keyspace.TermOrdinal(owner)
			if !callOK || !rowOK || keyspace.TermFamily(owner) != keyspace.FamilyBody || ownerOrdinal == 0 || uint64(ownerOrdinal) >= uint64(len(selected)) {
				return errors.New("program/flow/directfunction: Call Body path owner is unavailable")
			}
			if !selected[ownerOrdinal] {
				continue
			}
			function, directOK := result.Call(call)
			if !directOK {
				continue
			}
			_, targetBody, _, functionOK := functions.Get(function)
			targetOrdinal := keyspace.TermOrdinal(targetBody)
			if !functionOK || keyspace.TermFamily(targetBody) != keyspace.FamilyBody || targetOrdinal == 0 || uint64(targetOrdinal) >= uint64(len(selected)) || !callable[targetOrdinal] {
				return errors.New("program/flow/directfunction: direct Call target Body path is unavailable")
			}
			if selected[targetOrdinal] {
				continue
			}
			selected[targetOrdinal] = true
			selectedCount++
			changed = true
		}
	}
	result.selectedBodyPaths = make([]identity.ContentID, 0, selectedCount)
	for ordinal := uint32(1); ordinal < uint32(len(selected)); ordinal++ {
		path, pathOK := bodyPaths.BodyAt(ordinal)
		if !pathOK || !path.Available() {
			return errors.New("program/flow/directfunction: selected BodyPath is unavailable")
		}
		if selected[ordinal] {
			result.selectedBodyPaths = append(result.selectedBodyPaths, path)
		}
	}
	identity.SortContentIDs(result.selectedBodyPaths)
	return nil
}
