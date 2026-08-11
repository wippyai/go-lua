package causal

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
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
		_, key, _, fieldKind, ok := s.flow.Fields().Get(field)
		if !ok {
			return errors.New("program/flow/causal: TableField key projection is unavailable")
		}
		invalidExact := fieldKind == kind.FieldExact && !s.exactFieldAvailable(key)
		if invalidExact {
			s.invalidExactFields[ordinal] = true
		}
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
	return validPreTerm(term, s.counts) && s.exec.Executable(term)
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
		owner, ok := s.flow.Control().Breaks().Get(term)
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

func (s *proofState) exactLiteral(term keyspace.Term) (keyspace.LiteralValue, bool) {
	// Walk nested exact unary negations iteratively.  Exact-key admission is a
	// seal-time query, but authored input can be arbitrarily deep; recursion
	// here would reintroduce a depth-dependent causal seal failure.
	negated := false
	for keyspace.TermFamily(term) == keyspace.FamilyUnary {
		_, op, operand, ok := s.flow.Operators().Unaries().Get(term)
		if !ok || op != kind.UnaryNeg {
			return keyspace.LiteralValue{}, false
		}
		negated = !negated
		term = operand
	}
	var literal keyspace.LiteralValue
	var ok bool
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBool:
		_, _, value, found := s.source.Literals().Bools().At(int(keyspace.TermOrdinal(term) - 1))
		if found {
			literal, ok = keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}, true
		}
	case keyspace.FamilyInteger:
		_, _, value, found := s.source.Literals().Integers().At(int(keyspace.TermOrdinal(term) - 1))
		if found {
			literal, ok = keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}, true
		}
	case keyspace.FamilyFloat:
		_, _, bits, found := s.source.Literals().Floats().At(int(keyspace.TermOrdinal(term) - 1))
		if found {
			literal, ok = keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: bits}, true
		}
	case keyspace.FamilyString:
		_, _, value, found := s.source.Literals().Strings().At(int(keyspace.TermOrdinal(term) - 1))
		if found {
			literal, ok = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}, true
		}
	}
	if !ok || !negated {
		return literal, ok
	}
	switch literal.Kind {
	case keyspace.LiteralInteger:
		if literal.Integer == -1<<63 {
			return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(-float64(literal.Integer))}, true
		}
		literal.Integer = -literal.Integer
		return literal, true
	case keyspace.LiteralFloat:
		literal.FloatBits = math.Float64bits(-math.Float64frombits(literal.FloatBits))
		return literal, true
	default:
		return keyspace.LiteralValue{}, false
	}
}

func (s *proofState) exactFieldAvailable(term keyspace.Term) bool {
	literal, ok := s.exactLiteral(term)
	if !ok {
		return false
	}
	_, ok = s.source.Keys().Find(literal)
	return ok
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

// entryEndpoint follows only typed evaluation children until it reaches a
// live pre-Outcome endpoint. Static operands are skipped at this final
// boundary; no static term is emitted as a causal endpoint.
func (s *proofState) entryEndpoint(term keyspace.Term) (keyspace.Term, bool) {
	if isOutcome(term) {
		return term, true
	}
	if !validPreTerm(term, s.counts) {
		return 0, false
	}
	candidate := term
	for {
		if s.live(candidate) {
			entry, ok := s.ports.Entry(candidate)
			if ok {
				if isOutcome(entry) || s.live(entry) {
					return entry, true
				}
			}
			// A live authored operation can have a static first operand. Follow
			// its typed child sequence to the first runtime occurrence; if all
			// operands are static, the operation itself is the runtime anchor.
			if child, childOK := s.firstChild(candidate); childOK {
				candidate = child
				continue
			}
			return candidate, true
		}
		child, childOK := s.firstChild(candidate)
		if !childOK {
			return 0, false
		}
		candidate = child
	}
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

func (s *proofState) firstChild(term keyspace.Term) (keyspace.Term, bool) {
	family := keyspace.TermFamily(term)
	switch family {
	case keyspace.FamilyRead:
		_, child, _, ok := s.flow.Storage().Reads().Get(term)
		if !ok || keyspace.TermFamily(child) == keyspace.FamilyCell || !s.live(child) {
			return 0, false
		}
		return child, true
	case keyspace.FamilyLensExact:
		_, child, sourceTerm, fieldKind, ok := s.flow.Access().Exact().Get(term)
		if !ok {
			return 0, false
		}
		if s.live(child) {
			return child, true
		}
		if fieldKind == kind.FieldExact && sourceTerm != 0 && s.live(sourceTerm) {
			return sourceTerm, true
		}
		return 0, false
	case keyspace.FamilyLensKey:
		_, base, key, ok := s.flow.Access().Dynamic().Get(term)
		if !ok {
			return 0, false
		}
		if s.live(base) {
			return base, true
		}
		return key, key != 0 && s.live(key)
	case keyspace.FamilyUnary:
		_, _, child, ok := s.flow.Operators().Unaries().Get(term)
		return child, ok && s.live(child)
	case keyspace.FamilyBinary:
		_, _, left, right, ok := s.flow.Operators().Binaries().Get(term)
		if !ok {
			return 0, false
		}
		if s.live(left) {
			return left, true
		}
		return right, s.live(right)
	case keyspace.FamilySelect:
		_, _, left, right, ok := s.flow.Operators().Selects().Get(term)
		if !ok {
			return 0, false
		}
		if s.live(left) {
			return left, true
		}
		return right, s.live(right)
	case keyspace.FamilyValues:
		values := s.flow.Values()
		length, ok := values.Len(term)
		if !ok {
			return 0, false
		}
		for index := 0; index < length; index++ {
			member, memberOK := values.Member(term, index)
			if memberOK && s.live(member) {
				return member, true
			}
		}
		_, tail, rowOK := values.Get(term)
		return tail, rowOK && tail != 0 && s.live(tail)
	case keyspace.FamilyValueClaim:
		_, child, _, ok := s.flow.Claims().Get(term)
		return child, ok && s.live(child)
	case keyspace.FamilyBind:
		_, child, ok := s.flow.Storage().Binds().Get(term)
		return child, ok && s.live(child)
	case keyspace.FamilyAssign:
		assigns := s.flow.Storage().Assigns()
		count, ok := assigns.WriteCount(term)
		if !ok {
			return 0, false
		}
		for index := 0; index < count; index++ {
			write, writeOK := assigns.WriteAt(term, index)
			if !writeOK {
				return 0, false
			}
			_, target, targetOK := s.flow.Storage().Writes().Get(write)
			if targetOK && keyspace.TermFamily(target) != keyspace.FamilyCell && s.live(target) {
				return target, true
			}
		}
		_, child, rowOK := assigns.Get(term)
		return child, rowOK && s.live(child)
	case keyspace.FamilyCall:
		_, callee, _, actuals, ok := s.flow.Calls().Get(term)
		if !ok {
			return 0, false
		}
		if s.live(callee) {
			return callee, true
		}
		return actuals, actuals != 0 && s.live(actuals)
	case keyspace.FamilyTableField:
		_, key, values, fieldKind, ok := s.flow.Fields().Get(term)
		if !ok {
			return 0, false
		}
		if (fieldKind == kind.FieldKey || fieldKind == kind.FieldExact) && s.live(key) {
			return key, true
		}
		return values, values != 0 && s.live(values)
	case keyspace.FamilyReturn:
		_, child, ok := s.flow.Control().Returns().Get(term)
		return child, ok && s.live(child)
	case keyspace.FamilyBranch:
		_, child, _, _, ok := s.flow.Control().Branches().Get(term)
		return child, ok && s.live(child)
	case keyspace.FamilyLoop:
		_, bodyTerm, loopKind, controlTerm, ok := s.flow.Control().Loops().Get(term)
		if !ok {
			return 0, false
		}
		if loopKind == kind.LoopRepeat {
			return bodyTerm, s.live(bodyTerm)
		}
		return controlTerm, s.live(controlTerm)
	default:
		return 0, false
	}
}
