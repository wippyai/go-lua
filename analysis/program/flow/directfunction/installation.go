package directfunction

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func terminalCells(captureOuter []keyspace.Term) ([]keyspace.Term, error) {
	if len(captureOuter) == 0 {
		return nil, errors.New("program/flow/directfunction: empty Cell denominator")
	}
	terminal := make([]keyspace.Term, len(captureOuter))
	state := make([]uint8, len(captureOuter))
	path := make([]uint32, 0, len(captureOuter))
	for start := uint32(1); uint64(start) < uint64(len(captureOuter)); start++ {
		if state[start] == 2 {
			continue
		}
		path = path[:0]
		node := start
		for state[node] == 0 {
			state[node] = 1
			path = append(path, node)
			next := captureOuter[node]
			if next == 0 {
				break
			}
			if keyspace.TermFamily(next) != keyspace.FamilyCell || keyspace.TermOrdinal(next) == 0 ||
				uint64(keyspace.TermOrdinal(next)) >= uint64(len(captureOuter)) {
				return nil, errors.New("program/flow/directfunction: Capture relation leaves Cell universe")
			}
			node = keyspace.TermOrdinal(next)
		}
		if state[node] == 1 && captureOuter[node] != 0 {
			return nil, errors.New("program/flow/directfunction: cyclic Capture relation")
		}
		base := terminal[node]
		if base == 0 && captureOuter[node] == 0 {
			base = keyspace.MakeTerm(keyspace.FamilyCell, node)
		}
		if keyspace.TermFamily(base) != keyspace.FamilyCell || keyspace.TermOrdinal(base) == 0 {
			return nil, errors.New("program/flow/directfunction: Capture terminal is unavailable")
		}
		for len(path) != 0 {
			at := path[len(path)-1]
			path = path[:len(path)-1]
			terminal[at] = base
			state[at] = 2
		}
	}
	return terminal, nil
}

func buildInstallations(
	preimage source.View,
	flow authored.View,
	bodies *body.Result,
	bindings binding.Result,
	counts [keyspace.FamilyCount]uint32,
	terminal []keyspace.Term,
	forest *containment.Result,
	executableResult *executable.Result,
) ([]keyspace.Term, []keyspace.Term, []keyspace.Term, []keyspace.Term, error) {
	cellCount := int(counts[keyspace.FamilyCell])
	functionCount := int(counts[keyspace.FamilyFunction])
	if cellCount < 0 || functionCount < 0 || len(terminal) != cellCount+1 {
		return nil, nil, nil, nil, errors.New("program/flow/directfunction: installation denominator is unavailable")
	}
	mutations := make([]uint32, cellCount+1)
	assigned := make([]keyspace.Term, cellCount+1)
	assignedOrigin := make([]keyspace.Term, cellCount+1)
	bindFunction := make([]keyspace.Term, cellCount+1)
	bindOrigin := make([]keyspace.Term, cellCount+1)

	storage := flow.Storage()
	cells := storage.Cells()
	assigns := storage.Assigns()
	writes := storage.Writes()
	values := flow.Values()
	for index := 0; index < assigns.Count(); index++ {
		assign, ok := assigns.At(index)
		if !ok {
			return nil, nil, nil, nil, errors.New("program/flow/directfunction: Assign ordinal is unavailable")
		}
		owner, valueTerm, rowOK := assigns.Get(assign)
		if !rowOK || !validBody(owner, counts) || !validFamilyTerm(valueTerm, counts, keyspace.FamilyValues) {
			return nil, nil, nil, nil, errors.New("program/flow/directfunction: malformed Assign row")
		}
		valuesOwner, _, valuesOK := values.Get(valueTerm)
		if !valuesOK || !validBody(valuesOwner, counts) {
			return nil, nil, nil, nil, errors.New("program/flow/directfunction: Assign Values owner is unavailable")
		}
		writeCount, countOK := assigns.WriteCount(assign)
		if !countOK || writeCount <= 0 {
			return nil, nil, nil, nil, errors.New("program/flow/directfunction: Assign has no authored Writes")
		}
		// A dead or static assignment is not a runtime installation.  It must
		// not invalidate a live Bind or manufacture a Function↔Cell claim.
		if !activeOrigin(forest, executableResult, assign) {
			continue
		}
		for writeIndex := 0; writeIndex < writeCount; writeIndex++ {
			write, writeOK := assigns.WriteAt(assign, writeIndex)
			writeAssign, target, targetOK := writes.Get(write)
			if !writeOK || !targetOK || writeAssign != assign {
				return nil, nil, nil, nil, errors.New("program/flow/directfunction: Assign Write parent disagrees")
			}
			position, positionOK := values.Position(valueTerm, writeIndex)
			if !positionOK {
				return nil, nil, nil, nil, errors.New("program/flow/directfunction: Assign Values position is unavailable")
			}
			switch keyspace.TermFamily(target) {
			case keyspace.FamilyCell:
				cellKind, _, _, cellOK := cells.Get(target)
				if !cellOK {
					return nil, nil, nil, nil, errors.New("program/flow/directfunction: Assign Cell target is unavailable")
				}
				if cellKind == authored.CellGlobal {
					continue
				}
				if !validCell(target, counts) || !visibleCell(flow, bodies, owner, target, counts) {
					return nil, nil, nil, nil, errors.New("program/flow/directfunction: Assign Cell target is not visible")
				}
				base := terminal[keyspace.TermOrdinal(target)]
				if !validCell(base, counts) {
					return nil, nil, nil, nil, errors.New("program/flow/directfunction: Assign Cell has no terminal")
				}
				baseOrdinal := keyspace.TermOrdinal(base)
				mutations[baseOrdinal]++
				if position.Fixed != 0 && keyspace.TermFamily(position.Fixed) == keyspace.FamilyFunction {
					if keyspace.TermOrdinal(position.Fixed) > counts[keyspace.FamilyFunction] {
						return nil, nil, nil, nil, errors.New("program/flow/directfunction: Assign Function is outside universe")
					}
					functionOwner, _, _, functionOK := flow.Functions().Get(position.Fixed)
					if functionOK && valuesOwner == owner && functionOwner == owner {
						assigned[baseOrdinal] = position.Fixed
						assignedOrigin[baseOrdinal] = assign
					}
				}
			case keyspace.FamilyLensExact:
				lensOwner, _, _, _, lensOK := flow.Access().Exact().Get(target)
				if !lensOK || lensOwner != owner {
					return nil, nil, nil, nil, errors.New("program/flow/directfunction: Exact Lens Write owner disagrees")
				}
			case keyspace.FamilyLensKey:
				lensOwner, _, _, lensOK := flow.Access().Dynamic().Get(target)
				if !lensOK || lensOwner != owner {
					return nil, nil, nil, nil, errors.New("program/flow/directfunction: Dynamic Lens Write owner disagrees")
				}
			default:
				return nil, nil, nil, nil, errors.New("program/flow/directfunction: Assign Write target is not writable")
			}
		}
	}

	binds := storage.Binds()
	bindOrder := preimage.Binds()
	for index := 0; index < binds.Count(); index++ {
		bind, ok := binds.At(index)
		if !ok {
			return nil, nil, nil, nil, errors.New("program/flow/directfunction: Bind ordinal is unavailable")
		}
		owner, valueTerm, rowOK := binds.Get(bind)
		if !rowOK || !validBody(owner, counts) || !validFamilyTerm(valueTerm, counts, keyspace.FamilyValues) {
			return nil, nil, nil, nil, errors.New("program/flow/directfunction: malformed Bind row")
		}
		valuesOwner, _, valuesOK := values.Get(valueTerm)
		if !valuesOK || !validBody(valuesOwner, counts) {
			return nil, nil, nil, nil, errors.New("program/flow/directfunction: Bind Values owner is unavailable")
		}
		length, lengthOK := bindOrder.Len(bind)
		if !lengthOK || length < 0 {
			return nil, nil, nil, nil, errors.New("program/flow/directfunction: Bind Cell order is unavailable")
		}
		if valuesOwner != owner {
			return nil, nil, nil, nil, errors.New("program/flow/directfunction: Bind Values shape disagrees with Source Cell order")
		}
		for offset := 0; offset < length; offset++ {
			cell, cellOK := bindOrder.At(bind, offset)
			if !cellOK || !validCell(cell, counts) || !bindCell(flow, bindings, owner, bind, cell) {
				return nil, nil, nil, nil, errors.New("program/flow/directfunction: Bind Cell order disagrees")
			}
			position, positionOK := values.Position(valueTerm, offset)
			if !positionOK {
				return nil, nil, nil, nil, errors.New("program/flow/directfunction: Bind Values position is unavailable")
			}
			// Only a fixed Function position identifies a direct Function
			// binding. Tail positions and nil-fill are valid Bind inputs but
			// carry no occurrence-specific Function identity; fixed values
			// beyond the Source Cell order are intentionally ignored.
			if position.Fixed == 0 || keyspace.TermFamily(position.Fixed) != keyspace.FamilyFunction {
				continue
			}
			if keyspace.TermOrdinal(position.Fixed) > counts[keyspace.FamilyFunction] {
				return nil, nil, nil, nil, errors.New("program/flow/directfunction: Bind Function is outside universe")
			}
			functionOwner, _, _, functionOK := flow.Functions().Get(position.Fixed)
			if !functionOK || functionOwner != owner {
				continue
			}
			base := terminal[keyspace.TermOrdinal(cell)]
			if !validCell(base, counts) {
				return nil, nil, nil, nil, errors.New("program/flow/directfunction: Bind Cell has no terminal")
			}
			baseOrdinal := keyspace.TermOrdinal(base)
			if !activeOrigin(forest, executableResult, bind) {
				continue
			}
			if bindFunction[baseOrdinal] != 0 {
				return nil, nil, nil, nil, errors.New("program/flow/directfunction: Function binding is not one-to-one")
			}
			bindFunction[baseOrdinal] = position.Fixed
			bindOrigin[baseOrdinal] = bind
		}
	}

	functionForCell := make([]keyspace.Term, cellCount+1)
	functionOrigin := make([]keyspace.Term, cellCount+1)
	cellForFunction := make([]keyspace.Term, functionCount+1)
	for ordinal := uint32(1); ordinal <= uint32(cellCount); ordinal++ {
		if bindFunction[ordinal] != 0 {
			if mutations[ordinal] == 0 {
				function := bindFunction[ordinal]
				functionForCell[ordinal] = function
				functionOrigin[ordinal] = bindOrigin[ordinal]
				cellForFunction[keyspace.TermOrdinal(function)] = keyspace.MakeTerm(keyspace.FamilyCell, ordinal)
				continue
			}
			// A later active Assign invalidates the authored Bind claim. Fall
			// through so a sole Assign can establish the replacement mapping.
		}
		if mutations[ordinal] != 1 || assigned[ordinal] == 0 {
			continue
		}
		function := assigned[ordinal]
		functionOrdinal := keyspace.TermOrdinal(function)
		if cellForFunction[functionOrdinal] != 0 {
			return nil, nil, nil, nil, errors.New("program/flow/directfunction: Function binding is not one-to-one")
		}
		functionForCell[ordinal] = function
		functionOrigin[ordinal] = assignedOrigin[ordinal]
		cellForFunction[functionOrdinal] = keyspace.MakeTerm(keyspace.FamilyCell, ordinal)
	}

	recursiveSelf := make([]keyspace.Term, functionCount+1)
	functions := flow.Functions()
	for index := 0; index < functions.Count(); index++ {
		function := keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		base := cellForFunction[keyspace.TermOrdinal(function)]
		if !validCell(base, counts) || functionForCell[keyspace.TermOrdinal(base)] != function {
			continue
		}
		captureCount, ok := functions.CaptureCount(function)
		if !ok {
			return nil, nil, nil, nil, errors.New("program/flow/directfunction: recursive Function capture range is unavailable")
		}
		for at := 0; at < captureCount; at++ {
			_, outer, captureOK := functions.CaptureAt(function, at)
			if !captureOK || !validCell(outer, counts) {
				return nil, nil, nil, nil, errors.New("program/flow/directfunction: recursive Capture is unavailable")
			}
			outerBase := terminal[keyspace.TermOrdinal(outer)]
			if outerBase != base {
				continue
			}
			if recursiveSelf[keyspace.TermOrdinal(function)] != 0 {
				return nil, nil, nil, nil, errors.New("program/flow/directfunction: recursive Function witness is ambiguous")
			}
			recursiveSelf[keyspace.TermOrdinal(function)] = base
		}
	}
	return functionForCell, functionOrigin, cellForFunction, recursiveSelf, nil
}

// activeOrigin is deliberately narrower than source-control reachability:
// only a non-static, executable authored installation can establish a
// Function↔Cell mapping.  Foreign or malformed proofs fail closed through
// their own query fences.
func activeOrigin(forest *containment.Result, executableResult *executable.Result, origin keyspace.Term) bool {
	return forest != nil && executableResult != nil && !forest.Static(origin) && executableResult.Contains(origin)
}

func bindCell(flow authored.View, bindings binding.Result, owner, bind, cell keyspace.Term) bool {
	cellKind, cellBody, key, cellOK := flow.Storage().Cells().Get(cell)
	role, roleOK := bindings.Role(cell)
	host, hostOK := bindings.Host(cell)
	return cellOK && cellKind == authored.CellLocal && key == 0 && cellBody == owner &&
		roleOK && role == kind.CellLocal && hostOK && host == bind
}
