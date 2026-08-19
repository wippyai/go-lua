package causal

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func causalArcSourceFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyBody, keyspace.FamilyBind, keyspace.FamilyAssign,
		keyspace.FamilyCall, keyspace.FamilyBranch, keyspace.FamilyLoop,
		keyspace.FamilyBreak, keyspace.FamilyGoto:
		return true
	default:
		return false
	}
}

func (s *proofState) buildTypedIndexes() error {
	s.valueParent = make([]keyspace.Term, s.counts[keyspace.FamilyValues]+1)
	s.invalidExactKeys = make([]bool, s.counts[keyspace.FamilyUnary]+1)
	s.invalidExactFields = make([]bool, s.counts[keyspace.FamilyTableField]+1)
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyTableField]; ordinal++ {
		field := keyspace.MakeTerm(keyspace.FamilyTableField, ordinal)
		table, key, _, fieldKind, ok := s.flow.Fields().Get(field)
		if !ok {
			return errors.New("program/flow/causal: TableField key projection is unavailable")
		}
		tableOwner, tableOwnerOK := s.flow.Tables().Get(table)
		if !tableOwnerOK || keyspace.TermFamily(tableOwner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: TableField owner is unavailable")
		}
		eligibility, proofErr := s.graph.TableFieldThrowEligibility(s.source, s.flow, s.outs, field, tableOwner)
		if proofErr != nil {
			return proofErr
		}
		if fieldKind == kind.FieldKey && !eligibility.Available() {
			return errors.New("program/flow/causal: FieldKey Throw eligibility is unavailable")
		}
		invalidExact := fieldKind == kind.FieldExact && eligibility.Available()
		if invalidExact {
			s.invalidExactFields[ordinal] = true
		}
		s.tableFieldThrowProof[ordinal] = eligibility
		if invalidExact && keyspace.TermFamily(key) == keyspace.FamilyUnary {
			keyOrdinal := keyspace.TermOrdinal(key)
			if keyOrdinal == 0 || uint64(keyOrdinal) >= uint64(len(s.invalidExactKeys)) {
				return errors.New("program/flow/causal: invalid exact Unary key ordinal")
			}
			s.invalidExactKeys[keyOrdinal] = true
		}
	}
	setValueParent := func(values, parent keyspace.Term) error {
		ordinal := keyspace.TermOrdinal(values)
		if keyspace.TermFamily(values) != keyspace.FamilyValues || ordinal == 0 || uint64(ordinal) >= uint64(len(s.valueParent)) {
			return errors.New("program/flow/causal: Values parent has invalid Values term")
		}
		if s.valueParent[ordinal] != 0 && s.valueParent[ordinal] != parent {
			return errors.New("program/flow/causal: Values has multiple typed parents")
		}
		s.valueParent[ordinal] = parent
		return nil
	}
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyCall]; ordinal++ {
		call := keyspace.MakeTerm(keyspace.FamilyCall, ordinal)
		_, _, _, actuals, ok := s.flow.Calls().Get(call)
		if !ok || setValueParent(actuals, call) != nil {
			if !ok {
				return errors.New("program/flow/causal: Call actual Values parent is unavailable")
			}
			return errors.New("program/flow/causal: Call actual Values parent is invalid")
		}
	}
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyReturn]; ordinal++ {
		ret := keyspace.MakeTerm(keyspace.FamilyReturn, ordinal)
		_, values, ok := s.flow.Control().Returns().Get(ret)
		if !ok || setValueParent(values, ret) != nil {
			return errors.New("program/flow/causal: Return Values parent is unavailable")
		}
	}
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyBind]; ordinal++ {
		bind := keyspace.MakeTerm(keyspace.FamilyBind, ordinal)
		_, values, ok := s.flow.Storage().Binds().Get(bind)
		if !ok || setValueParent(values, bind) != nil {
			return errors.New("program/flow/causal: Bind Values parent is unavailable")
		}
	}
	assigns, writes := s.flow.Storage().Assigns(), s.flow.Storage().Writes()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyAssign]; ordinal++ {
		assign := keyspace.MakeTerm(keyspace.FamilyAssign, ordinal)
		_, values, ok := assigns.Get(assign)
		if !ok || setValueParent(values, assign) != nil {
			return errors.New("program/flow/causal: Assign Values parent is unavailable")
		}
		count, ok := assigns.WriteCount(assign)
		if !ok || count < 0 {
			return errors.New("program/flow/causal: Assign Write range is unavailable")
		}
		for index := 0; index < count; index++ {
			write, ok := assigns.WriteAt(assign, index)
			if !ok {
				return errors.New("program/flow/causal: Assign Write is unavailable")
			}
			parent, _, ok := writes.Get(write)
			if !ok || parent != assign {
				return errors.New("program/flow/causal: Write parent disagrees with Assign")
			}
		}
	}
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyTableField]; ordinal++ {
		field := keyspace.MakeTerm(keyspace.FamilyTableField, ordinal)
		_, _, values, _, ok := s.flow.Fields().Get(field)
		if !ok || setValueParent(values, field) != nil {
			return errors.New("program/flow/causal: TableField Values parent is unavailable")
		}
	}
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyLoop]; ordinal++ {
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, ordinal)
		_, _, _, control, ok := s.flow.Control().Loops().Get(loop)
		if !ok {
			return errors.New("program/flow/causal: Loop control is unavailable")
		}
		if keyspace.TermFamily(control) == keyspace.FamilyValues {
			if err := setValueParent(control, loop); err != nil {
				return err
			}
		}
	}

	bodyCount := s.counts[keyspace.FamilyBody]
	s.bodyParentRoot = make([]keyspace.Term, bodyCount+1)
	s.bodyParentCursor = make([]int, bodyCount+1)
	for index := range s.bodyParentCursor {
		s.bodyParentCursor[index] = -1
	}
	for ordinal := uint32(1); ordinal <= bodyCount; ordinal++ {
		bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		parent, hasParent := s.bodies.Parent(bodyTerm)
		if !hasParent {
			continue
		}
		root, rootOK := s.source.Index().Root(bodyTerm)
		// Position is a closure projection: a Body nested beneath an
		// authored direct Body root inherits that root's coordinate.  Only
		// Root(body) == body is the direct Source Body occurrence whose
		// parent must agree with the Body proof.  Typed construct Bodies
		// remain unpositioned unless their contents reach a direct root, and
		// must not be mistaken for a second direct Body parent authority.
		if !rootOK || root != bodyTerm {
			continue
		}
		positionBody, _, cursor, positionOK := s.source.Index().Position(bodyTerm)
		if !positionOK {
			continue
		}
		if positionBody != parent || cursor < 0 {
			return errors.New("program/flow/causal: direct Body parent position disagrees with Body proof")
		}
		s.bodyParentRoot[ordinal] = bodyTerm
		s.bodyParentCursor[ordinal] = cursor
	}
	branches, loops := s.flow.Control().Branches(), s.flow.Control().Loops()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyBranch]; ordinal++ {
		branch := keyspace.MakeTerm(keyspace.FamilyBranch, ordinal)
		owner, _, whenTrue, whenFalse, ok := branches.Get(branch)
		if !ok {
			return errors.New("program/flow/causal: Branch body relation is unavailable")
		}
		for _, arm := range [...]keyspace.Term{whenTrue, whenFalse} {
			armOrdinal := keyspace.TermOrdinal(arm)
			if armOrdinal == 0 || uint64(armOrdinal) >= uint64(len(s.bodyParentRoot)) || s.bodyParentRoot[armOrdinal] != 0 {
				return errors.New("program/flow/causal: Branch Body parent is duplicated")
			}
			s.bodyParentRoot[armOrdinal] = branch
			cursor, cursorOK := s.rootCursor(owner, branch)
			if !cursorOK {
				return errors.New("program/flow/causal: Branch root cursor is unavailable")
			}
			s.bodyParentCursor[armOrdinal] = cursor
		}
	}
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyLoop]; ordinal++ {
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, ordinal)
		owner, loopBody, _, _, ok := loops.Get(loop)
		if !ok {
			return errors.New("program/flow/causal: Loop body relation is unavailable")
		}
		bodyOrdinal := keyspace.TermOrdinal(loopBody)
		if bodyOrdinal == 0 || uint64(bodyOrdinal) >= uint64(len(s.bodyParentRoot)) || s.bodyParentRoot[bodyOrdinal] != 0 {
			return errors.New("program/flow/causal: Loop Body parent is duplicated")
		}
		s.bodyParentRoot[bodyOrdinal] = loop
		cursor, cursorOK := s.rootCursor(owner, loop)
		if !cursorOK {
			return errors.New("program/flow/causal: Loop root cursor is unavailable")
		}
		s.bodyParentCursor[bodyOrdinal] = cursor
	}
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyFunction]; ordinal++ {
		function := keyspace.MakeTerm(keyspace.FamilyFunction, ordinal)
		_, child, _, ok := s.flow.Functions().Get(function)
		if !ok {
			return errors.New("program/flow/causal: Function body relation is unavailable")
		}
		childOrdinal := keyspace.TermOrdinal(child)
		if childOrdinal == 0 || uint64(childOrdinal) >= uint64(len(s.bodyParentRoot)) {
			return errors.New("program/flow/causal: Function Body ordinal is invalid")
		}
		if s.bodyParentRoot[childOrdinal] == 0 {
			s.bodyParentRoot[childOrdinal] = function
		}
	}
	return nil
}

func (s *proofState) rootCursor(owner, root keyspace.Term) (int, bool) {
	body, _, cursor, ok := s.source.Index().Position(root)
	if !ok || body != owner || cursor < 0 {
		return 0, false
	}
	rootAt, rootOK := s.bodies.RootAt(owner, cursor)
	return cursor, rootOK && rootAt == root
}

func validPreTerm(term keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && family != keyspace.FamilyOutcome &&
		ordinal != 0 && ordinal <= counts[family]
}

func (s *proofState) live(term keyspace.Term) bool {
	if term == 0 {
		return false
	}
	if isOutcome(term) {
		return true
	}
	return validPreTerm(term, s.counts) && s.exec.Contains(term)
}

func (s *proofState) static(term keyspace.Term) bool {
	return term != 0 && validPreTerm(term, s.counts) && s.forest.Static(term)
}

func (s *proofState) bodyOf(term keyspace.Term) (keyspace.Term, bool) {
	if keyspace.TermFamily(term) == keyspace.FamilyBody {
		return term, true
	}
	// Authored rows carry their owning Body; this helper is used only for
	// semantic endpoint validation, so the typed lookup is exhaustive.
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyValues:
		owner, _, ok := s.flow.Values().Get(term)
		return owner, ok
	case keyspace.FamilyLensExact:
		owner, _, _, _, ok := s.flow.Access().Exact().Get(term)
		return owner, ok
	case keyspace.FamilyLensKey:
		owner, _, _, ok := s.flow.Access().Dynamic().Get(term)
		return owner, ok
	case keyspace.FamilyRead:
		owner, _, _, ok := s.flow.Storage().Reads().Get(term)
		return owner, ok
	case keyspace.FamilyVararg:
		owner, _, ok := s.flow.Storage().Varargs().Get(term)
		return owner, ok
	case keyspace.FamilyBind:
		owner, _, ok := s.flow.Storage().Binds().Get(term)
		return owner, ok
	case keyspace.FamilyAssign:
		owner, _, ok := s.flow.Storage().Assigns().Get(term)
		return owner, ok
	case keyspace.FamilyWrite:
		assign, _, ok := s.flow.Storage().Writes().Get(term)
		if !ok {
			return 0, false
		}
		owner, _, assignOK := s.flow.Storage().Assigns().Get(assign)
		return owner, assignOK
	case keyspace.FamilyTable:
		owner, ok := s.flow.Tables().Get(term)
		return owner, ok
	case keyspace.FamilyTableField:
		table, _, _, _, ok := s.flow.Fields().Get(term)
		if !ok {
			return 0, false
		}
		owner, tableOK := s.flow.Tables().Get(table)
		return owner, tableOK
	case keyspace.FamilyUnary:
		owner, _, _, ok := s.flow.Operators().Unaries().Get(term)
		return owner, ok
	case keyspace.FamilyBinary:
		owner, _, _, _, ok := s.flow.Operators().Binaries().Get(term)
		return owner, ok
	case keyspace.FamilySelect:
		owner, _, _, _, ok := s.flow.Operators().Selects().Get(term)
		return owner, ok
	case keyspace.FamilyFunction:
		owner, _, _, ok := s.flow.Functions().Get(term)
		return owner, ok
	case keyspace.FamilyCall:
		owner, _, _, _, ok := s.flow.Calls().Get(term)
		return owner, ok
	case keyspace.FamilyReturn:
		owner, _, ok := s.flow.Control().Returns().Get(term)
		return owner, ok
	case keyspace.FamilyBreak:
		owner, _, ok := s.flow.Control().Breaks().Get(term)
		return owner, ok
	case keyspace.FamilyGoto:
		owner, _, ok := s.flow.Control().Gotos().Get(term)
		return owner, ok
	case keyspace.FamilyBranch:
		owner, _, _, _, ok := s.flow.Control().Branches().Get(term)
		return owner, ok
	case keyspace.FamilyLoop:
		owner, _, _, _, ok := s.flow.Control().Loops().Get(term)
		return owner, ok
	case keyspace.FamilyValueClaim:
		owner, _, _, ok := s.flow.Claims().Get(term)
		return owner, ok
	case keyspace.FamilyTypeValue:
		owner, ok := s.flow.TypeValues().Get(term)
		return owner, ok
	default:
		return 0, false
	}
}

// invalidExactFieldKey reports the one typed use that must not enter the
// evaluation plane.  A FieldExact whose normalized key is absent (or cannot
// be normalized) reaches its Field -> Throw outcome directly; emitting the
// UnaryNeg operand route would leave a dead Unary endpoint before that Field.
func (s *proofState) invalidExactFieldKey(term keyspace.Term) bool {
	if keyspace.TermFamily(term) != keyspace.FamilyUnary {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	return ordinal != 0 && uint64(ordinal) < uint64(len(s.invalidExactKeys)) && s.invalidExactKeys[ordinal]
}

func (s *proofState) invalidExactField(field keyspace.Term) bool {
	if keyspace.TermFamily(field) != keyspace.FamilyTableField {
		return false
	}
	ordinal := keyspace.TermOrdinal(field)
	return ordinal != 0 && uint64(ordinal) < uint64(len(s.invalidExactFields)) && s.invalidExactFields[ordinal]
}

// finishEndpoint applies the sealed Finish port and fails closed for a dead
// or static endpoint. A live operation whose final typed port is static keeps
// the operation itself as its commit anchor.
func (s *proofState) finishEndpoint(term keyspace.Term) (keyspace.Term, bool) {
	if isOutcome(term) {
		return term, true
	}
	if !validPreTerm(term, s.counts) || !s.live(term) {
		return 0, false
	}
	finish, ok := s.ports.Finish(term)
	if ok && (isOutcome(finish) || s.live(finish)) {
		return finish, true
	}
	return term, true
}
