package continuation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

type cellSeal struct {
	store     *scopeStore
	roots     [keyspace.FamilyCount][]uint32
	scopeAt   []uint32
	bodyStart []uint32
	bodyScope []uint32
	bodyReady []bool
	bodyQueue []keyspace.Term
	counts    [keyspace.FamilyCount]uint32
	entry     keyspace.Term
	source    source.View
	flow      authored.View
	bodies    *body.Result
	binding   binding.Result
	exec      *executable.Result
	cand      *candidates.Result
}

func newCellSeal(input inputProof) (*cellSeal, error) {
	seal := &cellSeal{
		store:     newScopeStore(),
		bodyStart: make([]uint32, input.counts[keyspace.FamilyBody]+1),
		bodyScope: make([]uint32, input.counts[keyspace.FamilyBody]+1),
		bodyReady: make([]bool, input.counts[keyspace.FamilyBody]+1),
		counts:    input.counts,
		entry:     input.entry,
		source:    input.source,
		flow:      input.flow,
		bodies:    input.bodies,
		binding:   input.binding,
		exec:      input.exec,
		cand:      input.cand,
	}
	for _, family := range continuationSubjects {
		seal.roots[family] = make([]uint32, input.counts[family]+1)
		for ordinal := range seal.roots[family] {
			seal.roots[family][ordinal] = absentRoot
		}
	}
	if err := seal.layout(); err != nil {
		return nil, err
	}
	return seal, nil
}

func (seal *cellSeal) layout() error {
	if seal == nil || seal.store == nil || seal.bodies == nil {
		return errors.New("program/flow/continuation: lexical Cell owner is unavailable")
	}
	var total uint64
	for ordinal := uint32(1); ordinal <= seal.counts[keyspace.FamilyBody]; ordinal++ {
		bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		count, ok := seal.bodies.RootCount(bodyTerm)
		if !ok || count < 0 {
			return errors.New("program/flow/continuation: Body roots are unavailable")
		}
		if total > uint64(^uint32(0)) || total+uint64(count)+1 > uint64(^uint32(0)) {
			return errors.New("program/flow/continuation: lexical position space is too large")
		}
		seal.bodyStart[ordinal] = uint32(total)
		total += uint64(count) + 1
	}
	seal.scopeAt = make([]uint32, int(total))
	for index := range seal.scopeAt {
		seal.scopeAt[index] = absentRoot
	}
	entryScope, err := seal.entryScope()
	if err != nil {
		return err
	}
	if err := seal.assignBody(seal.entry, entryScope); err != nil {
		return err
	}
	functions := seal.flow.Functions()
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			return errors.New("program/flow/continuation: Function view is unavailable")
		}
		_, functionBody, _, ok := functions.Get(function)
		if !ok {
			return errors.New("program/flow/continuation: Function row is unavailable")
		}
		functionScope, scopeOK, scopeErr := seal.functionScope(function)
		if scopeErr != nil {
			return scopeErr
		}
		if !scopeOK {
			functionScope = 0
		}
		if err := seal.assignBody(functionBody, functionScope); err != nil {
			return err
		}
	}
	if err := seal.assignBodyScopes(); err != nil {
		return err
	}
	return seal.assignSubjects()
}

func (seal *cellSeal) entryScope() (uint32, error) {
	chunk, ok := seal.binding.ChunkVararg()
	if !ok {
		return 0, nil
	}
	if err := seal.checkCell(chunk, kind.CellChunkVararg, seal.entry); err != nil {
		return 0, err
	}
	return seal.appendCells(0, seal.entry, []keyspace.Term{chunk})
}

func (seal *cellSeal) functionScope(function keyspace.Term) (uint32, bool, error) {
	functions := seal.flow.Functions()
	owner, functionBody, functionVararg, ok := functions.Get(function)
	if !ok || !keyspace.ValidTerm(owner, keyspace.FamilyBody, int(seal.counts[keyspace.FamilyBody])) ||
		!keyspace.ValidTerm(functionBody, keyspace.FamilyBody, int(seal.counts[keyspace.FamilyBody])) || owner == functionBody {
		return 0, false, errors.New("program/flow/continuation: malformed Function Body relation")
	}
	if parent, parentOK := seal.bodies.Parent(functionBody); !parentOK || parent != owner {
		return 0, false, errors.New("program/flow/continuation: Function Body parent mismatch")
	}
	if activation, activationOK := seal.bodies.Activation(functionBody); !activationOK || activation != function {
		return 0, false, errors.New("program/flow/continuation: Function activation mismatch")
	}
	cells := make([]keyspace.Term, 0)
	formalOrder := seal.source.Formals()
	formalCount, ok := formalOrder.Len(function)
	if !ok || formalCount < 0 {
		return 0, false, errors.New("program/flow/continuation: Function formal order is unavailable")
	}
	for index := 0; index < formalCount; index++ {
		cell, cellOK := formalOrder.At(function, index)
		if !cellOK {
			return 0, false, errors.New("program/flow/continuation: Function formal Cell is unavailable")
		}
		if err := seal.checkCell(cell, kind.CellFormal, function); err != nil {
			return 0, false, err
		}
		cells = append(cells, cell)
	}
	if functionVararg != 0 {
		if err := seal.checkCell(functionVararg, kind.CellFunctionVararg, function); err != nil {
			return 0, false, err
		}
		cells = append(cells, functionVararg)
	}
	captureCount, ok := functions.CaptureCount(function)
	if !ok || captureCount < 0 {
		return 0, false, errors.New("program/flow/continuation: Function capture range is unavailable")
	}
	for index := 0; index < captureCount; index++ {
		inner, outer, captureOK := functions.CaptureAt(function, index)
		if !captureOK || inner == 0 || outer == 0 || inner == outer {
			return 0, false, errors.New("program/flow/continuation: malformed Function capture")
		}
		if err := seal.checkCell(inner, kind.CellCapture, function); err != nil {
			return 0, false, err
		}
		// Binding owns the captured outer relation.  It is intentionally not
		// installed in the new Function lexical root.
		outerRole, outerRoleOK := seal.binding.Role(outer)
		outerHost, outerHostOK := seal.binding.Host(outer)
		if !outerRoleOK || !outerHostOK || outerRole == kind.CellGlobal || outerHost == 0 {
			return 0, false, errors.New("program/flow/continuation: malformed Capture outer Cell")
		}
		cells = append(cells, inner)
	}
	if len(cells) == 0 {
		return 0, false, nil
	}
	scope, err := seal.appendCells(0, function, cells)
	return scope, true, err
}

func (seal *cellSeal) assignBodyScopes() error {
	for cursor := 0; cursor < len(seal.bodyQueue); cursor++ {
		bodyTerm := seal.bodyQueue[cursor]
		bodyOrdinal := keyspace.TermOrdinal(bodyTerm)
		base := seal.bodyScope[bodyOrdinal]
		rootCount, ok := seal.bodies.RootCount(bodyTerm)
		if !ok || rootCount < 0 {
			return errors.New("program/flow/continuation: Body root range is unavailable")
		}
		env := base
		for rootIndex := 0; rootIndex < rootCount; rootIndex++ {
			coordinate := seal.bodyStart[bodyOrdinal] + uint32(rootIndex)
			seal.scopeAt[coordinate] = env
			root, rootOK := seal.bodies.RootAt(bodyTerm, rootIndex)
			if !rootOK {
				return errors.New("program/flow/continuation: Body root is unavailable")
			}
			rootFamily, rootOrdinal := keyspace.TermFamily(root), keyspace.TermOrdinal(root)
			if rootFamily <= keyspace.FamilyInvalid || rootFamily >= keyspace.FamilyCount || rootOrdinal == 0 || rootOrdinal > seal.counts[rootFamily] || !body.RootFamily(rootFamily) {
				return errors.New("program/flow/continuation: Body root leaves canonical denominator")
			}
			switch keyspace.TermFamily(root) {
			case keyspace.FamilyBind:
				var err error
				env, err = seal.bindScope(env, root)
				if err != nil {
					return err
				}
			case keyspace.FamilyBody:
				if err := seal.assignBody(root, env); err != nil {
					return err
				}
			case keyspace.FamilyBranch:
				if err := seal.branchBodies(env, root); err != nil {
					return err
				}
			case keyspace.FamilyLoop:
				if err := seal.loopBody(env, root); err != nil {
					return err
				}
			}
		}
		seal.scopeAt[seal.bodyStart[bodyOrdinal]+uint32(rootCount)] = env
	}
	for ordinal := uint32(1); ordinal <= seal.counts[keyspace.FamilyBody]; ordinal++ {
		if !seal.bodyReady[ordinal] {
			return errors.New("program/flow/continuation: Body has no lexical scope")
		}
	}
	return nil
}

func (seal *cellSeal) assignBody(bodyTerm keyspace.Term, scope uint32) error {
	if !keyspace.ValidTerm(bodyTerm, keyspace.FamilyBody, int(seal.counts[keyspace.FamilyBody])) ||
		uint64(scope) >= uint64(len(seal.store.nodes)) {
		return errors.New("program/flow/continuation: invalid Body lexical scope")
	}
	ordinal := keyspace.TermOrdinal(bodyTerm)
	if seal.bodyReady[ordinal] {
		if seal.bodyScope[ordinal] != scope {
			return errors.New("program/flow/continuation: conflicting Body lexical scopes")
		}
		return nil
	}
	seal.bodyReady[ordinal] = true
	seal.bodyScope[ordinal] = scope
	seal.bodyQueue = append(seal.bodyQueue, bodyTerm)
	return nil
}

func (seal *cellSeal) bindScope(parent uint32, bind keyspace.Term) (uint32, error) {
	if !keyspace.ValidTerm(bind, keyspace.FamilyBind, int(seal.counts[keyspace.FamilyBind])) {
		return 0, errors.New("program/flow/continuation: invalid Bind scope host")
	}
	length, ok := seal.source.Binds().Len(bind)
	if !ok || length <= 0 {
		return 0, errors.New("program/flow/continuation: Bind Cell order is unavailable")
	}
	cells := make([]keyspace.Term, length)
	order := seal.source.Binds()
	for index := range cells {
		cell, cellOK := order.At(bind, index)
		if !cellOK {
			return 0, errors.New("program/flow/continuation: Bind Cell order is unavailable")
		}
		if err := seal.checkCell(cell, kind.CellLocal, bind); err != nil {
			return 0, err
		}
		cells[index] = cell
	}
	return seal.appendCells(parent, bind, cells)
}

func (seal *cellSeal) branchBodies(parent uint32, branch keyspace.Term) error {
	owner, _, whenTrue, whenFalse, ok := seal.flow.Control().Branches().Get(branch)
	if !ok || owner == 0 || whenTrue == 0 || whenFalse == 0 || whenTrue == whenFalse ||
		!seal.bodies.Contains(owner, owner) {
		return errors.New("program/flow/continuation: malformed Branch Body relation")
	}
	if bodyParent, bodyOK := seal.bodies.Parent(whenTrue); !bodyOK || bodyParent != owner {
		return errors.New("program/flow/continuation: Branch true Body parent mismatch")
	}
	if bodyParent, bodyOK := seal.bodies.Parent(whenFalse); !bodyOK || bodyParent != owner {
		return errors.New("program/flow/continuation: Branch false Body parent mismatch")
	}
	if err := seal.assignBody(whenTrue, parent); err != nil {
		return err
	}
	return seal.assignBody(whenFalse, parent)
}

func (seal *cellSeal) loopBody(parent uint32, loop keyspace.Term) error {
	owner, loopBody, loopKind, _, ok := seal.flow.Control().Loops().Get(loop)
	if !ok || owner == 0 || !keyspace.ValidTerm(loopBody, keyspace.FamilyBody, int(seal.counts[keyspace.FamilyBody])) ||
		loopKind < kind.LoopWhile || loopKind > kind.LoopGenericFor {
		return errors.New("program/flow/continuation: malformed Loop Body relation")
	}
	if bodyParent, bodyOK := seal.bodies.Parent(loopBody); !bodyOK || bodyParent != owner {
		return errors.New("program/flow/continuation: Loop Body parent mismatch")
	}
	loopScope := parent
	count, countOK := seal.flow.Control().Loops().CellCount(loop)
	if !countOK || count < 0 {
		return errors.New("program/flow/continuation: Loop Cell order is unavailable")
	}
	if count != 0 {
		cells := make([]keyspace.Term, count)
		loops := seal.flow.Control().Loops()
		for index := range cells {
			cell, cellOK := loops.CellAt(loop, index)
			if !cellOK {
				return errors.New("program/flow/continuation: Loop Cell order is unavailable")
			}
			if err := seal.checkCell(cell, kind.CellLoop, loop); err != nil {
				return err
			}
			cells[index] = cell
		}
		var err error
		loopScope, err = seal.appendCells(parent, loop, cells)
		if err != nil {
			return err
		}
	}
	return seal.assignBody(loopBody, loopScope)
}

func (seal *cellSeal) appendCells(parent uint32, host keyspace.Term, cells []keyspace.Term) (uint32, error) {
	if uint64(parent) >= uint64(len(seal.store.nodes)) {
		return 0, errors.New("program/flow/continuation: invalid lexical scope parent")
	}
	if host == 0 {
		return 0, errors.New("program/flow/continuation: invalid lexical scope host")
	}
	return seal.store.appendGroup(parent, cells)
}

func (seal *cellSeal) checkCell(cell keyspace.Term, role kind.CellRole, host keyspace.Term) error {
	if !keyspace.ValidTerm(cell, keyspace.FamilyCell, int(seal.counts[keyspace.FamilyCell])) {
		return errors.New("program/flow/continuation: Cell leaves canonical denominator")
	}
	gotRole, roleOK := seal.binding.Role(cell)
	gotHost, hostOK := seal.binding.Host(cell)
	if !roleOK || !hostOK || gotRole != role || gotHost != host {
		return errors.New("program/flow/continuation: Binding role or host mismatch")
	}
	return nil
}

func (seal *cellSeal) assignSubjects() error {
	index := seal.source.Index()
	for _, family := range continuationSubjects {
		for ordinal := uint32(1); ordinal <= seal.counts[family]; ordinal++ {
			term := keyspace.MakeTerm(family, ordinal)
			if !subjectFrom(seal.exec, seal.cand, term) {
				continue
			}
			bodyTerm, cursor, ok := index.Frontier(term)
			if !ok || !keyspace.ValidTerm(bodyTerm, keyspace.FamilyBody, int(seal.counts[keyspace.FamilyBody])) || cursor < 0 {
				return errors.New("program/flow/continuation: live subject lacks Source Frontier")
			}
			bodyOrdinal := keyspace.TermOrdinal(bodyTerm)
			rootIndex := uint64(seal.bodyStart[bodyOrdinal]) + uint64(cursor)
			if rootIndex >= uint64(len(seal.scopeAt)) || seal.scopeAt[rootIndex] == absentRoot {
				return errors.New("program/flow/continuation: Source Frontier lacks lexical scope")
			}
			seal.roots[family][ordinal] = seal.scopeAt[rootIndex]
		}
	}
	return compactScopeRoots(seal.store, seal.roots)
}
