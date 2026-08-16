package containment

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// emitStaticMarks computes the one static-containment membership projection
// consumed by the containment kernel.  It deliberately does not build a
// relation: marks are a dense family/ordinal bitset and the work stack is
// discarded before this function returns.
//
// There are two and only two roots for this projection:
//
//   - TypeOf and Annotation rows, followed through their authored expression
//     operands; and
//   - static type terms whose local Static owner is a Call, i.e. the complete
//     typed subtree of a call's type arguments.
//
// Static declarations and ordinary static type trees are not marks merely
// because they are static syntax.  Reusable references (Cells, TypeRefs,
// Goto targets, static scopes, and construct Bodies) are never traversed as
// expression edges.  Cells owned by a marked Bind/Function/Loop are still
// classified directly, matching the source identity part of the legacy
// projection; they do not become traversal edges or a second graph.
func emitStaticMarks(
	preimage source.Preimage,
	staticView static.View,
	view authored.View,
	roots []keyspace.Term,
	counts [keyspace.FamilyCount]uint32,
) ([]keyspace.Term, error) {
	local := staticView.LocalContainment()
	marks := newStaticMarkBits(counts)
	stack := make([]keyspace.Term, 0)

	for _, root := range roots {
		if !flowrole.ValueOccurrence(counts, root) && !staticMarkFamily(counts, root, keyspace.FamilyValues) {
			return nil, staticMarkError("invalid static reference root")
		}
		stack = append(stack, root)
	}
	// The fallback emitter already validated and deduplicated TypeOf operand
	// and Annotation Values endpoints.  Add only the two owning root terms;
	// re-reading and re-appending their endpoints here would create a second
	// root derivation.  Static edge emission validates the row payloads.
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyTypeOf]; ordinal++ {
		stack = append(stack, keyspace.MakeTerm(keyspace.FamilyTypeOf, ordinal))
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyAnnotation]; ordinal++ {
		stack = append(stack, keyspace.MakeTerm(keyspace.FamilyAnnotation, ordinal))
	}
	if err := validateStaticCallArguments(staticView, counts); err != nil {
		return nil, err
	}
	if err := markCallOwnedStaticTypes(local, counts, &marks); err != nil {
		return nil, err
	}

	for len(stack) != 0 {
		term := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !marks.mark(term) {
			continue
		}
		if err := visitStaticExpression(preimage, term, view, counts, &stack); err != nil {
			return nil, err
		}
	}
	if err := markStaticSourceMetadata(preimage, view, counts, &marks); err != nil {
		return nil, err
	}

	return marks.terms(), nil
}

type staticMarkBits struct {
	counts [keyspace.FamilyCount]uint32
	words  [keyspace.FamilyCount][]uint64
	marked []keyspace.Term
}

func newStaticMarkBits(counts [keyspace.FamilyCount]uint32) staticMarkBits {
	return staticMarkBits{counts: counts}
}

// mark returns true only for the first mark of a valid concrete Term.  A
// malformed term is not silently treated as already visited; callers must
// validate all foreign keys before asking to mark them.
func (marks *staticMarkBits) mark(term keyspace.Term) bool {
	if marks == nil {
		return false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount ||
		ordinal == 0 || ordinal > marks.counts[family] {
		return false
	}
	words := marks.words[family]
	if words == nil {
		words = make([]uint64, (uint64(marks.counts[family])+63)/64)
		marks.words[family] = words
	}
	word := uint64(ordinal-1) >> 6
	if word >= uint64(len(words)) {
		return false
	}
	mask := uint64(1) << ((ordinal - 1) & 63)
	if words[word]&mask != 0 {
		return false
	}
	words[word] |= mask
	marks.marked = append(marks.marked, term)
	return true
}

func (marks staticMarkBits) has(term keyspace.Term) bool {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount ||
		ordinal == 0 || ordinal > marks.counts[family] {
		return false
	}
	words := marks.words[family]
	word := uint64(ordinal-1) >> 6
	return word < uint64(len(words)) && words[word]&(uint64(1)<<((ordinal-1)&63)) != 0
}

// terms returns the dense-bitset's first-seen projection.  KernelStatic only
// consumes membership bits, so retaining this discovery order avoids an
// O(all Program terms) rescan and does not introduce an ordering authority.
func (marks staticMarkBits) terms() []keyspace.Term {
	return marks.marked
}

// validateStaticCallArguments validates the Static contract's Call -> type
// argument relation. Ownership of the complete subtree is proved below from
// LocalContainment; this pass does not copy the argument relation into a
// second graph.
func validateStaticCallArguments(staticView static.View, counts [keyspace.FamilyCount]uint32) error {
	calls := staticView.Contracts().Calls()
	if calls.Count() != int(counts[keyspace.FamilyCall]) {
		return staticMarkError("static Call cardinality mismatch")
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyCall]; ordinal++ {
		call := keyspace.MakeTerm(keyspace.FamilyCall, ordinal)
		argumentCount, ok := calls.TypeArgumentCount(call)
		if !ok || argumentCount < 0 {
			return staticMarkError("invalid static Call type-argument range")
		}
		for index := 0; index < argumentCount; index++ {
			argument, ok := calls.TypeArgumentAt(call, index)
			if !ok || !staticMarkStaticType(counts, argument) {
				return staticMarkError("invalid static Call type argument")
			}
		}
	}
	return nil
}

// markCallOwnedStaticTypes classifies the complete local typed subtree whose
// resolved static owner is a Call.  LocalContainment exposes only parent rows
// for static type families and FieldOwner for TypeField; walking those typed
// projections is sufficient and avoids retaining a copied owner table.
func markCallOwnedStaticTypes(
	local static.LocalContainment,
	counts [keyspace.FamilyCount]uint32,
	marks *staticMarkBits,
) error {
	scratch := staticMarkOwnerScratch{}
	localCount := local.Count()
	for index := 0; index < localCount; index++ {
		term, ok := local.At(index)
		if !ok || !staticMarkStaticType(counts, term) {
			return staticMarkError("invalid Static local containment term")
		}
		ownedByCall, err := staticMarkCallOwner(term, local, counts, &scratch, marks)
		if err != nil {
			return err
		}
		if ownedByCall {
			marks.mark(term)
		}
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyTypeField]; ordinal++ {
		field := keyspace.MakeTerm(keyspace.FamilyTypeField, ordinal)
		ownedByCall, err := staticMarkCallOwner(field, local, counts, &scratch, marks)
		if err != nil {
			return err
		}
		if ownedByCall {
			marks.mark(field)
		}
	}
	return nil
}

type staticMarkOwnerScratch struct {
	state [keyspace.FamilyCount][]uint8
	path  []keyspace.Term
}

func staticMarkCallOwner(
	start keyspace.Term,
	local static.LocalContainment,
	counts [keyspace.FamilyCount]uint32,
	scratch *staticMarkOwnerScratch,
	marks *staticMarkBits,
) (bool, error) {
	if scratch == nil {
		return false, staticMarkError("nil Static owner scratch")
	}
	if !staticMarkStaticTypeOrField(counts, start) {
		return false, staticMarkError("invalid Static owner term")
	}
	path := scratch.path[:0]
	current := start
	ownedByCall := false
	for current != 0 {
		family, ordinal := keyspace.TermFamily(current), keyspace.TermOrdinal(current)
		if family == keyspace.FamilyCall {
			ownedByCall = true
			break
		}
		if !staticMarkStaticTypeOrField(counts, current) {
			// A non-static parent is the opaque owner frontier. It is valid,
			// but only Call frontiers classify this subtree as static.
			break
		}
		if scratch.state[family] == nil {
			if counts[family] == 0 {
				return false, staticMarkError("Static owner family is empty")
			}
			scratch.state[family] = make([]uint8, counts[family])
		}
		if ordinal == 0 || uint64(ordinal) > uint64(len(scratch.state[family])) {
			return false, staticMarkError("Static owner ordinal out of range")
		}
		slot := &scratch.state[family][ordinal-1]
		switch *slot {
		case 2:
			current = 0
			continue
		case 3:
			ownedByCall = true
			current = 0
			continue
		case 1:
			return false, staticMarkError("cycle in Static local containment")
		}
		*slot = 1
		path = append(path, current)

		if family == keyspace.FamilyTypeField {
			parent, ok := local.FieldOwner(current)
			if !ok || !staticMarkValid(counts, parent) {
				return false, staticMarkError("invalid TypeField owner")
			}
			current = parent
			continue
		}
		parent, ok := local.Parent(current)
		if !ok {
			// Parent's false result with a zero parent is a valid local root;
			// only a Call frontier classifies this path as static-owned.
			current = 0
			continue
		}
		if !staticMarkValid(counts, parent) {
			return false, staticMarkError("invalid Static parent")
		}
		current = parent
	}
	for _, term := range path {
		family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
		if ownedByCall {
			scratch.state[family][ordinal-1] = 3
			marks.mark(term)
		} else {
			scratch.state[family][ordinal-1] = 2
		}
	}
	scratch.path = path[:0]
	return ownedByCall, nil
}

func visitStaticExpression(
	preimage source.Preimage,
	term keyspace.Term,
	view authored.View,
	counts [keyspace.FamilyCount]uint32,
	stack *[]keyspace.Term,
) error {
	push := func(child keyspace.Term) error {
		if !staticMarkValid(counts, child) {
			return staticMarkError("invalid static expression child")
		}
		*stack = append(*stack, child)
		return nil
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyTypeOf, keyspace.FamilyAnnotation:
		// These are static roots. Their endpoint relations were validated by
		// the fallback emitter; Scope and Annotation.Target are reusable static
		// references, not expression children.
		return nil
	case keyspace.FamilyBody:
		length, ok := preimage.Order().BodyLen(term)
		if !ok || length < 0 {
			return staticMarkError("invalid static Body source order")
		}
		for index := 0; index < length; index++ {
			child, ok := preimage.Order().BodyAt(term, index)
			if !ok || !staticMarkValid(counts, child) {
				return staticMarkError("invalid static Body source term")
			}
			if staticMarkStatementRoot(keyspace.TermFamily(child)) {
				if err := push(child); err != nil {
					return err
				}
			}
		}
		return nil
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyKey,
		keyspace.FamilyBreak, keyspace.FamilyLabel, keyspace.FamilyGoto,
		keyspace.FamilyControlFault:
		// Atoms and control identities have no structural expression child.
		// Goto's Label target is a reusable reference and is intentionally not
		// followed here.
		return nil
	case keyspace.FamilyValues:
		values := view.Values()
		owner, tail, ok := values.Get(term)
		if !ok || !staticMarkBody(counts, owner) {
			return staticMarkError("invalid static Values owner")
		}
		length, ok := values.Len(term)
		if !ok || length < 0 {
			return staticMarkError("invalid static Values range")
		}
		for index := 0; index < length; index++ {
			member, ok := values.Member(term, index)
			if !ok || !flowrole.ValueOccurrence(counts, member) {
				return staticMarkError("invalid static Values member")
			}
			if err := push(member); err != nil {
				return err
			}
		}
		if tail != 0 {
			if !flowrole.OpenOccurrence(counts, tail) {
				return staticMarkError("invalid static Values tail")
			}
			if err := push(tail); err != nil {
				return err
			}
		}
	case keyspace.FamilyRead:
		owner, source, _, ok := view.Storage().Reads().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !staticMarkValid(counts, source) {
			return staticMarkError("invalid static Read")
		}
		switch keyspace.TermFamily(source) {
		case keyspace.FamilyCell:
			// Cell identity is a reusable storage reference, not an
			// expression child.
		case keyspace.FamilyLensExact, keyspace.FamilyLensKey:
			return push(source)
		default:
			return staticMarkError("invalid static Read source")
		}
	case keyspace.FamilyVararg:
		owner, cell, ok := view.Storage().Varargs().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !staticMarkFamily(counts, cell, keyspace.FamilyCell) {
			return staticMarkError("invalid static Vararg")
		}
		// Vararg's Cell is a reusable storage identity and is not followed.
	case keyspace.FamilyLensExact:
		owner, base, source, _, ok := view.Access().Exact().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !flowrole.ValueOccurrence(counts, base) ||
			!staticMarkValid(counts, source) {
			return staticMarkError("invalid static exact Lens")
		}
		if err := push(base); err != nil {
			return err
		}
		return push(source)
	case keyspace.FamilyLensKey:
		owner, base, key, ok := view.Access().Dynamic().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !flowrole.ValueOccurrence(counts, base) || !flowrole.ValueOccurrence(counts, key) {
			return staticMarkError("invalid static dynamic Lens")
		}
		if err := push(base); err != nil {
			return err
		}
		return push(key)
	case keyspace.FamilyUnary:
		owner, _, operand, ok := view.Operators().Unaries().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !flowrole.ValueOccurrence(counts, operand) {
			return staticMarkError("invalid static Unary")
		}
		return push(operand)
	case keyspace.FamilyBinary:
		owner, _, left, right, ok := view.Operators().Binaries().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !flowrole.ValueOccurrence(counts, left) || !flowrole.ValueOccurrence(counts, right) {
			return staticMarkError("invalid static Binary")
		}
		if err := push(left); err != nil {
			return err
		}
		return push(right)
	case keyspace.FamilySelect:
		owner, _, left, right, ok := view.Operators().Selects().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !flowrole.ValueOccurrence(counts, left) || !flowrole.ValueOccurrence(counts, right) {
			return staticMarkError("invalid static Select")
		}
		if err := push(left); err != nil {
			return err
		}
		return push(right)
	case keyspace.FamilyValueClaim:
		owner, operand, _, ok := view.Claims().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !flowrole.ValueOccurrence(counts, operand) {
			return staticMarkError("invalid static ValueClaim")
		}
		// The optional static target belongs to the Static type forest and
		// is not an expression child of the runtime operand.
		return push(operand)
	case keyspace.FamilyTypeValue:
		owner, ok := view.TypeValues().Get(term)
		if !ok || !staticMarkBody(counts, owner) {
			return staticMarkError("invalid static TypeValue")
		}
	case keyspace.FamilyCall:
		owner, callee, receiver, actuals, ok := view.Calls().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !flowrole.ValueOccurrence(counts, callee) ||
			(receiver != 0 && !flowrole.ValueOccurrence(counts, receiver)) ||
			!staticMarkFamily(counts, actuals, keyspace.FamilyValues) {
			return staticMarkError("invalid static Call")
		}
		if err := push(callee); err != nil {
			return err
		}
		// Receiver is a reusable call-site reference.  Legacy static closure
		// follows callee and actuals only; validating it above keeps the foreign
		// key boundary closed without turning receiver identity into an edge.
		return push(actuals)
	case keyspace.FamilyReturn:
		owner, values, ok := view.Control().Returns().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !staticMarkFamily(counts, values, keyspace.FamilyValues) {
			return staticMarkError("invalid static Return")
		}
		return push(values)
	case keyspace.FamilyBind:
		owner, values, ok := view.Storage().Binds().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !staticMarkFamily(counts, values, keyspace.FamilyValues) {
			return staticMarkError("invalid static Bind")
		}
		// Bind Cell identities are reusable storage references; Values is the
		// only structural expression child.
		return push(values)
	case keyspace.FamilyAssign:
		owner, values, ok := view.Storage().Assigns().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !staticMarkFamily(counts, values, keyspace.FamilyValues) {
			return staticMarkError("invalid static Assign")
		}
		assigns := view.Storage().Assigns()
		writes := view.Storage().Writes()
		writeCount, ok := assigns.WriteCount(term)
		if !ok || writeCount <= 0 {
			return staticMarkError("invalid static Assign writes")
		}
		for index := 0; index < writeCount; index++ {
			write, ok := assigns.WriteAt(term, index)
			if !ok || !staticMarkFamily(counts, write, keyspace.FamilyWrite) {
				return staticMarkError("invalid static Assign Write")
			}
			writeAssign, target, ok := writes.Get(write)
			if !ok || writeAssign != term || !flowrole.Addressable(counts, target) {
				return staticMarkError("invalid static Write target")
			}
			if keyspace.TermFamily(target) == keyspace.FamilyLensExact || keyspace.TermFamily(target) == keyspace.FamilyLensKey {
				if err := push(target); err != nil {
					return err
				}
			}
		}
		return push(values)
	case keyspace.FamilyFunction:
		owner, body, vararg, ok := view.Functions().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !staticMarkFamily(counts, body, keyspace.FamilyBody) ||
			(vararg != 0 && !staticMarkFamily(counts, vararg, keyspace.FamilyCell)) {
			return staticMarkError("invalid static Function")
		}
		captureCount, ok := view.Functions().CaptureCount(term)
		if !ok || captureCount < 0 {
			return staticMarkError("invalid static Function captures")
		}
		for index := 0; index < captureCount; index++ {
			inner, outer, ok := view.Functions().CaptureAt(term, index)
			if !ok || !staticMarkFamily(counts, inner, keyspace.FamilyCell) || !staticMarkFamily(counts, outer, keyspace.FamilyCell) {
				return staticMarkError("invalid static Function capture")
			}
		}
		// Formal, vararg, and capture Cells are reusable identities. The
		// Function Body is the one structural syntax subtree whose authored
		// source roots must be classified when a nested Function itself is a
		// static expression.
		return push(body)
	case keyspace.FamilyBranch:
		owner, condition, whenTrue, whenFalse, ok := view.Control().Branches().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !flowrole.ValueOccurrence(counts, condition) ||
			!staticMarkFamily(counts, whenTrue, keyspace.FamilyBody) || !staticMarkFamily(counts, whenFalse, keyspace.FamilyBody) {
			return staticMarkError("invalid static Branch")
		}
		if err := push(condition); err != nil {
			return err
		}
		if err := push(whenTrue); err != nil {
			return err
		}
		return push(whenFalse)
	case keyspace.FamilyLoop:
		owner, body, _, control, ok := view.Control().Loops().Get(term)
		if !ok || !staticMarkBody(counts, owner) || !staticMarkFamily(counts, body, keyspace.FamilyBody) ||
			!staticMarkFamily(counts, control, keyspace.FamilyValues) {
			return staticMarkError("invalid static Loop")
		}
		cells := view.Control().Loops()
		cellCount, ok := cells.CellCount(term)
		if !ok || cellCount < 0 {
			return staticMarkError("invalid static Loop cells")
		}
		for index := 0; index < cellCount; index++ {
			cell, ok := cells.CellAt(term, index)
			if !ok || !staticMarkFamily(counts, cell, keyspace.FamilyCell) {
				return staticMarkError("invalid static Loop Cell")
			}
		}
		if err := push(control); err != nil {
			return err
		}
		return push(body)
	case keyspace.FamilyTable:
		owner, ok := view.Tables().Get(term)
		if !ok || !staticMarkBody(counts, owner) {
			return staticMarkError("invalid static Table")
		}
		tables, fields := view.Tables(), view.Fields()
		fieldCount, ok := tables.FieldCount(term)
		if !ok || fieldCount < 0 {
			return staticMarkError("invalid static Table fields")
		}
		for index := 0; index < fieldCount; index++ {
			field, ok := tables.FieldAt(term, index)
			if !ok || !staticMarkFamily(counts, field, keyspace.FamilyTableField) {
				return staticMarkError("invalid static TableField")
			}
			fieldTable, key, values, fieldKind, ok := fields.Get(field)
			if !ok || fieldTable != term || !staticMarkFieldKey(view, counts, key, fieldKind) ||
				!staticMarkFamily(counts, values, keyspace.FamilyValues) {
				return staticMarkError("invalid static TableField foreign key")
			}
			if err := push(key); err != nil {
				return err
			}
			if err := push(values); err != nil {
				return err
			}
		}
		return nil
	case keyspace.FamilyTableField:
		// Table normally consumes fields directly. This case keeps the closed
		// expression vocabulary fail-closed if a future owner pushes one.
		table, key, values, fieldKind, ok := view.Fields().Get(term)
		if !ok || !staticMarkFamily(counts, table, keyspace.FamilyTable) ||
			!staticMarkFieldKey(view, counts, key, fieldKind) || !staticMarkFamily(counts, values, keyspace.FamilyValues) {
			return staticMarkError("invalid static TableField")
		}
		if err := push(key); err != nil {
			return err
		}
		return push(values)
	case keyspace.FamilyWrite:
		assign, target, ok := view.Storage().Writes().Get(term)
		if !ok || !staticMarkFamily(counts, assign, keyspace.FamilyAssign) || !flowrole.Addressable(counts, target) {
			return staticMarkError("invalid static Write")
		}
		return nil
	default:
		return staticMarkError("unsupported static expression family")
	}
	return nil
}

func staticMarkStatementRoot(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyBody, keyspace.FamilyBind, keyspace.FamilyAssign,
		keyspace.FamilyCall, keyspace.FamilyBranch, keyspace.FamilyLoop,
		keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyGoto:
		return true
	default:
		return false
	}
}

// markStaticSourceMetadata retains the zero-width source identities which
// belong to a statically classified Body. They are not expression children,
// so the Body walk above intentionally skips them and this deterministic
// postpass installs exactly the owner-derived membership.
func markStaticSourceMetadata(
	preimage source.Preimage,
	view authored.View,
	counts [keyspace.FamilyCount]uint32,
	marks *staticMarkBits,
) error {
	if err := markStaticCells(preimage, view, counts, marks); err != nil {
		return err
	}
	labels := view.Control().Labels()
	if labels.Count() != int(counts[keyspace.FamilyLabel]) {
		return staticMarkError("Label cardinality mismatch")
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyLabel]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyLabel, ordinal)
		owner, ok := labels.Get(term)
		if !ok || !staticMarkBody(counts, owner) {
			return staticMarkError("invalid Label owner")
		}
		if marks.has(owner) {
			marks.mark(term)
		}
	}
	faults := preimage.Faults()
	if faults.Count() != int(counts[keyspace.FamilyControlFault]) {
		return staticMarkError("ControlFault cardinality mismatch")
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyControlFault]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyControlFault, ordinal)
		row, ok := faults.At(term)
		if !ok || !staticMarkBody(counts, row.Owner) {
			return staticMarkError("invalid ControlFault owner")
		}
		if marks.has(row.Owner) {
			marks.mark(term)
		}
	}
	return nil
}

// markStaticCells preserves the legacy distinction between reusable storage
// identities and expression edges: Bind cells, Function formals/vararg and
// capture-inner cells, and Loop cells are marked when their owner expression
// is marked, but none is pushed onto the expression worklist. Capture.outer,
// Write, and TableField remain deliberately unclassified here.
func markStaticCells(
	preimage source.Preimage,
	view authored.View,
	counts [keyspace.FamilyCount]uint32,
	marks *staticMarkBits,
) error {
	binds := preimage.Binds()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyBind]; ordinal++ {
		bind := keyspace.MakeTerm(keyspace.FamilyBind, ordinal)
		length, ok := binds.Len(bind)
		if !ok || length < 0 {
			return staticMarkError("invalid static Bind cell range")
		}
		for index := 0; index < length; index++ {
			cell, ok := binds.At(bind, index)
			if !ok || !staticMarkFamily(counts, cell, keyspace.FamilyCell) {
				return staticMarkError("invalid static Bind cell")
			}
			if marks.has(bind) {
				marks.mark(cell)
			}
		}
	}

	functions := view.Functions()
	formals := preimage.Formals()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyFunction]; ordinal++ {
		function := keyspace.MakeTerm(keyspace.FamilyFunction, ordinal)
		if !marks.has(function) {
			continue
		}
		_, body, vararg, ok := functions.Get(function)
		if !ok || !staticMarkFamily(counts, body, keyspace.FamilyBody) ||
			(vararg != 0 && !staticMarkFamily(counts, vararg, keyspace.FamilyCell)) {
			return staticMarkError("invalid static Function storage")
		}
		if vararg != 0 {
			marks.mark(vararg)
		}
		formalCount, ok := formals.Len(function)
		if !ok || formalCount < 0 {
			return staticMarkError("invalid static Function formal range")
		}
		for index := 0; index < formalCount; index++ {
			formal, ok := formals.At(function, index)
			if !ok || !staticMarkFamily(counts, formal, keyspace.FamilyCell) {
				return staticMarkError("invalid static Function formal")
			}
			marks.mark(formal)
		}
		captureCount, ok := functions.CaptureCount(function)
		if !ok || captureCount < 0 {
			return staticMarkError("invalid static Function capture range")
		}
		for index := 0; index < captureCount; index++ {
			inner, outer, ok := functions.CaptureAt(function, index)
			if !ok || !staticMarkFamily(counts, inner, keyspace.FamilyCell) ||
				!staticMarkFamily(counts, outer, keyspace.FamilyCell) {
				return staticMarkError("invalid static Function capture")
			}
			marks.mark(inner)
		}
	}

	loops := view.Control().Loops()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyLoop]; ordinal++ {
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, ordinal)
		if !marks.has(loop) {
			continue
		}
		cellCount, ok := loops.CellCount(loop)
		if !ok || cellCount < 0 {
			return staticMarkError("invalid static Loop cell range")
		}
		for index := 0; index < cellCount; index++ {
			cell, ok := loops.CellAt(loop, index)
			if !ok || !staticMarkFamily(counts, cell, keyspace.FamilyCell) {
				return staticMarkError("invalid static Loop cell")
			}
			marks.mark(cell)
		}
	}
	return nil
}

func staticMarkError(detail string) error {
	return errors.New("program/flow/containment: " + detail)
}

func staticMarkValid(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return validTerm(term, counts)
}

func staticMarkFamily(counts [keyspace.FamilyCount]uint32, term keyspace.Term, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 && keyspace.TermOrdinal(term) <= counts[family]
}

func staticMarkBody(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return staticMarkFamily(counts, term, keyspace.FamilyBody)
}

func staticMarkStaticType(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return staticMarkStaticTypeFamily(keyspace.TermFamily(term)) && staticMarkValid(counts, term)
}

func staticMarkStaticTypeOrField(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyTypeField && staticMarkValid(counts, term) || staticMarkStaticType(counts, term)
}

func staticMarkStaticTypeFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyTypeAlias, keyspace.FamilyTypeInterface, keyspace.FamilyTypeParam,
		keyspace.FamilyTypePrimitive, keyspace.FamilyTypeLiteral, keyspace.FamilyTypeOptional,
		keyspace.FamilyTypeUnion, keyspace.FamilyTypeIntersection, keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric, keyspace.FamilyTypeArray, keyspace.FamilyTypeMap,
		keyspace.FamilyTypeRecord, keyspace.FamilyTypeFunction, keyspace.FamilyTypeAsserts,
		keyspace.FamilyTypeOf, keyspace.FamilyTypeKeyOf, keyspace.FamilyTypeIndexAccess,
		keyspace.FamilyTypeConditional:
		return true
	default:
		return false
	}
}

func staticMarkFieldKey(
	view authored.View,
	counts [keyspace.FamilyCount]uint32,
	term keyspace.Term,
	fieldKind kind.FieldKind,
) bool {
	switch fieldKind {
	case kind.FieldList, kind.FieldName:
		return staticMarkFamily(counts, term, keyspace.FamilyKey)
	case kind.FieldExact:
		if !staticMarkValid(counts, term) {
			return false
		}
		switch keyspace.TermFamily(term) {
		case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
			keyspace.FamilyFloat, keyspace.FamilyString:
			return true
		case keyspace.FamilyUnary:
			_, op, operand, ok := view.Operators().Unaries().Get(term)
			return ok && op == kind.UnaryNeg &&
				(staticMarkFamily(counts, operand, keyspace.FamilyInteger) ||
					staticMarkFamily(counts, operand, keyspace.FamilyFloat))
		default:
			return false
		}
	case kind.FieldKey:
		return flowrole.ValueOccurrence(counts, term)
	default:
		return false
	}
}
