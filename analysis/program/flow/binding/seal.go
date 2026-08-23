package binding

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal proves the early definition role and host of every authored Cell.
// Source contributes the canonical Cell order for Binds and Function formals;
// Body contributes only its already-sealed parent and activation relations.
// No containment or later lexical judgment is retained here.
func Seal(preimage source.Preimage, view authored.View, bodies *body.Result, entry keyspace.Term) (Result, error) {
	identity := preimage.Identity()
	sourceID := identity.ContentID()
	flowID := view.ContentID()
	if !sourceID.Available() || identity.Name() == "" || identity.TermCount() == 0 {
		return Result{}, errors.New("program/flow/binding: Source preimage expired")
	}
	if !flowID.Available() {
		return Result{}, errors.New("program/flow/binding: authored view expired")
	}
	if bodies == nil || !body.Matches(bodies, sourceID, flowID) {
		return Result{}, errors.New("program/flow/binding: Body provenance disagrees with Source or Flow")
	}

	counts, err := sourceCounts(identity)
	if err != nil {
		return Result{}, err
	}
	storage := view.Storage()
	cells := storage.Cells()
	reads := storage.Reads()
	varargs := storage.Varargs()
	binds := storage.Binds()
	functions := view.Functions()
	loops := view.Control().Loops()
	if counts[keyspace.FamilyBody] != bodies.BodyCount() || bodies.BodyCount() == 0 {
		return Result{}, errors.New("program/flow/binding: Body family cardinality mismatch")
	}
	if counts[keyspace.FamilyCell] != cells.Count() || counts[keyspace.FamilyRead] != reads.Count() ||
		counts[keyspace.FamilyVararg] != varargs.Count() || counts[keyspace.FamilyBind] != binds.Count() ||
		counts[keyspace.FamilyFunction] != functions.Count() || counts[keyspace.FamilyLoop] != loops.Count() {
		return Result{}, errors.New("program/flow/binding: authored family cardinality mismatch")
	}

	bodyCount := bodies.BodyCount()
	if err := validateEntry(bodies, entry, bodyCount); err != nil {
		return Result{}, err
	}
	if err := validateFunctionBodies(functions, bodies, bodyCount); err != nil {
		return Result{}, err
	}
	cellCount := cells.Count()
	plane := newRolePlane(cellCount)

	keys := preimage.Keys()
	if err := validateCells(cells, keys, plane, bodyCount); err != nil {
		return Result{}, err
	}

	chunk, err := validateVarargs(varargs, functions, cells, bodies, entry, bodyCount)
	if err != nil {
		return Result{}, err
	}
	if chunk != 0 {
		if err := plane.assignRole(cells, chunk, kind.CellChunkVararg, entry, entry, 0, bodyCount); err != nil {
			return Result{}, err
		}
	}

	if err := sealBinds(preimage, binds, cells, plane, bodyCount); err != nil {
		return Result{}, err
	}
	if err := sealLoops(loops, cells, bodies, plane, bodyCount); err != nil {
		return Result{}, err
	}
	if err := sealFunctions(preimage, functions, cells, bodies, plane, bodyCount); err != nil {
		return Result{}, err
	}
	functionCells, err := sealFunctionCells(preimage, view, binds, functions, cells, plane, bodyCount)
	if err != nil {
		return Result{}, err
	}

	for ordinal := 1; ordinal <= cellCount; ordinal++ {
		if !validRole(plane.roles[ordinal]) {
			return Result{}, errors.New("program/flow/binding: unclassified Cell")
		}
		if plane.roles[ordinal] == kind.CellGlobal {
			if plane.hosts[ordinal] != 0 {
				return Result{}, errors.New("program/flow/binding: global Cell has a host")
			}
		} else if plane.hosts[ordinal] == 0 {
			return Result{}, errors.New("program/flow/binding: lexical Cell has no host")
		}
	}
	return Result{
		sourceID:      sourceID,
		flowID:        flowID,
		roles:         plane.roles,
		hosts:         plane.hosts,
		slots:         plane.slots,
		chunk:         chunk,
		functionCells: functionCells,
	}, nil
}

func sourceCounts(identity source.Identity) ([keyspace.FamilyCount]int, error) {
	var counts [keyspace.FamilyCount]int
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return counts, errors.New("program/flow/binding: invalid Source family cardinality")
		}
		counts[family] = count
		total += uint64(count)
	}
	if total != uint64(identity.TermCount()) {
		return counts, errors.New("program/flow/binding: Source family cardinality mismatch")
	}
	return counts, nil
}

func validateEntry(result *body.Result, entry keyspace.Term, bodyCount int) error {
	if !validBody(entry, bodyCount) {
		return errors.New("program/flow/binding: invalid Entry Body")
	}
	if _, hasParent := result.Parent(entry); hasParent {
		return errors.New("program/flow/binding: Entry Body has a parent")
	}
	activation, ok := result.Activation(entry)
	if !ok || activation != 0 {
		return errors.New("program/flow/binding: Entry activation mismatch")
	}
	return nil
}

func validateCells(cells authored.Cells, keys source.Keys, plane rolePlane, bodyCount int) error {
	cellCount := len(plane.roles) - 1
	// Exact atoms can be much larger than the Cell family. Allocate duplicate
	// scratch only when a global Cell actually claims an atom; local-only and
	// zero-Cell programs must not scale validation memory with ExactCount.
	var seenKeys map[keyspace.Key]struct{}
	for index := 0; index < cellCount; index++ {
		cell, ok := cells.At(index)
		if !ok {
			return errors.New("program/flow/binding: Cell view is not live")
		}
		cellKind, cellBody, key, ok := cells.Get(cell)
		if !ok {
			return errors.New("program/flow/binding: Cell row is not live")
		}
		switch cellKind {
		case authored.CellGlobal:
			if cellBody != 0 || key == 0 {
				return errors.New("program/flow/binding: invalid or duplicate global key")
			}
			value, exact := keys.Exact(key)
			if !exact || value.Kind != keyspace.LiteralString || value.String == "" {
				return errors.New("program/flow/binding: global key is not a canonical nonempty String atom")
			}
			if seenKeys == nil {
				seenKeys = make(map[keyspace.Key]struct{})
			}
			if _, duplicate := seenKeys[key]; duplicate {
				return errors.New("program/flow/binding: invalid or duplicate global key")
			}
			seenKeys[key] = struct{}{}
			if err := plane.assignRole(cells, cell, kind.CellGlobal, 0, 0, 0, bodyCount); err != nil {
				return err
			}
		case authored.CellLocal:
			if key != 0 || !validBody(cellBody, bodyCount) {
				return errors.New("program/flow/binding: invalid local Cell")
			}
		default:
			return errors.New("program/flow/binding: invalid authored Cell kind")
		}
	}
	return nil
}

func validateFunctionBodies(functions authored.Functions, bodies *body.Result, bodyCount int) error {
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			return errors.New("program/flow/binding: Function view is not live")
		}
		owner, functionBody, _, ok := functions.Get(function)
		if !ok || !validBody(owner, bodyCount) || !validBody(functionBody, bodyCount) || owner == functionBody {
			return errors.New("program/flow/binding: invalid Function Body authority")
		}
		parent, hasParent := bodies.Parent(functionBody)
		if !hasParent || parent != owner {
			return errors.New("program/flow/binding: Function Body parent mismatch")
		}
		activation, hasActivation := bodies.Activation(functionBody)
		if !hasActivation || activation != function {
			return errors.New("program/flow/binding: Function Body activation mismatch")
		}
	}
	return nil
}

func validateVarargs(varargs authored.Varargs, functions authored.Functions, cells authored.Cells, bodies *body.Result, entry keyspace.Term, bodyCount int) (keyspace.Term, error) {
	var chunk keyspace.Term
	for index := 0; index < varargs.Count(); index++ {
		rowTerm, ok := varargs.At(index)
		if !ok {
			return 0, errors.New("program/flow/binding: Vararg view is not live")
		}
		owner, cell, ok := varargs.Get(rowTerm)
		if !ok || !validBody(owner, bodyCount) || !validCell(cell, cells.Count()) {
			return 0, errors.New("program/flow/binding: invalid Vararg row")
		}
		cellKind, cellBody, key, ok := cells.Get(cell)
		if !ok || cellKind != authored.CellLocal || key != 0 || !validBody(cellBody, bodyCount) {
			return 0, errors.New("program/flow/binding: Vararg does not name a local Cell")
		}
		activation, ok := bodies.Activation(owner)
		if !ok {
			return 0, errors.New("program/flow/binding: Vararg owner activation unavailable")
		}
		if activation == 0 {
			if cellBody != entry {
				return 0, errors.New("program/flow/binding: chunk Vararg Cell is not entry-local")
			}
			if chunk != 0 && chunk != cell {
				return 0, errors.New("program/flow/binding: conflicting chunk Vararg Cells")
			}
			chunk = cell
			continue
		}
		if !validFunction(activation, functions.Count()) {
			return 0, errors.New("program/flow/binding: Function Vararg provider mismatch")
		}
		_, functionBody, functionVararg, ok := functions.Get(activation)
		if !ok || functionVararg == 0 || functionVararg != cell || functionBody != cellBody {
			return 0, errors.New("program/flow/binding: Vararg does not match Function provider")
		}
	}
	return chunk, nil
}

func sealBinds(preimage source.Preimage, binds authored.Binds, cells authored.Cells, plane rolePlane, bodyCount int) error {
	order := preimage.Binds()
	for index := 0; index < binds.Count(); index++ {
		bind, ok := binds.At(index)
		if !ok {
			return errors.New("program/flow/binding: Bind view is not live")
		}
		owner, _, ok := binds.Get(bind)
		if !ok || !validBody(owner, bodyCount) {
			return errors.New("program/flow/binding: invalid Bind owner")
		}
		length, ok := order.Len(bind)
		if !ok || length <= 0 {
			return errors.New("program/flow/binding: Bind requires nonempty Source Cell order")
		}
		for at := 0; at < length; at++ {
			cell, ok := order.At(bind, at)
			if !ok {
				return errors.New("program/flow/binding: Bind order is not live")
			}
			if err := plane.assignRole(cells, cell, kind.CellLocal, owner, bind, uint32(at+1), bodyCount); err != nil {
				return err
			}
		}
	}
	return nil
}

func sealLoops(loops authored.Loops, cells authored.Cells, bodies *body.Result, plane rolePlane, bodyCount int) error {
	for index := 0; index < loops.Count(); index++ {
		loop, ok := loops.At(index)
		if !ok {
			return errors.New("program/flow/binding: Loop view is not live")
		}
		owner, loopBody, loopKind, _, ok := loops.Get(loop)
		if !ok || !validBody(owner, bodyCount) || !validBody(loopBody, bodyCount) ||
			loopKind < kind.LoopWhile || loopKind > kind.LoopGenericFor {
			return errors.New("program/flow/binding: invalid Loop row")
		}
		parent, hasParent := bodies.Parent(loopBody)
		if !hasParent || parent != owner {
			return errors.New("program/flow/binding: Loop Body parent mismatch")
		}
		length, ok := loops.CellCount(loop)
		if !ok || length < 0 {
			return errors.New("program/flow/binding: Loop Cell order is not live")
		}
		for at := 0; at < length; at++ {
			cell, ok := loops.CellAt(loop, at)
			if !ok {
				return errors.New("program/flow/binding: Loop Cell order is not live")
			}
			if err := plane.assignRole(cells, cell, kind.CellLoop, loopBody, loop, uint32(at+1), bodyCount); err != nil {
				return err
			}
		}
	}
	return nil
}

func sealFunctions(preimage source.Preimage, functions authored.Functions, cells authored.Cells, bodies *body.Result, plane rolePlane, bodyCount int) error {
	formalOrder := preimage.Formals()
	seenOuter := make([]uint32, cells.Count()+1)
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			return errors.New("program/flow/binding: Function view is not live")
		}
		owner, functionBody, functionVararg, ok := functions.Get(function)
		if !ok || !validBody(owner, bodyCount) || !validBody(functionBody, bodyCount) {
			return errors.New("program/flow/binding: invalid Function row")
		}
		formalCount, ok := formalOrder.Len(function)
		if !ok || formalCount < 0 {
			return errors.New("program/flow/binding: Function formal order is not live")
		}
		for at := 0; at < formalCount; at++ {
			cell, ok := formalOrder.At(function, at)
			if !ok {
				return errors.New("program/flow/binding: Function formal order is not live")
			}
			if err := plane.assignRole(cells, cell, kind.CellFormal, functionBody, function, uint32(at+1), bodyCount); err != nil {
				return err
			}
		}
		if functionVararg != 0 {
			if err := plane.assignRole(cells, functionVararg, kind.CellFunctionVararg, functionBody, function, 0, bodyCount); err != nil {
				return err
			}
		}
		captureCount, ok := functions.CaptureCount(function)
		if !ok || captureCount < 0 {
			return errors.New("program/flow/binding: Function capture range is not live")
		}
		marker := uint32(index + 1)
		for at := 0; at < captureCount; at++ {
			inner, outer, ok := functions.CaptureAt(function, at)
			if !ok || !validCell(inner, cells.Count()) || !validCell(outer, cells.Count()) || inner == outer {
				return errors.New("program/flow/binding: invalid Capture")
			}
			innerKind, innerBody, innerKey, ok := cells.Get(inner)
			if !ok || innerKind != authored.CellLocal || innerKey != 0 || innerBody != functionBody {
				return errors.New("program/flow/binding: Capture Inner mismatch")
			}
			outerKind, outerBody, outerKey, ok := cells.Get(outer)
			if !ok || outerKind != authored.CellLocal || outerKey != 0 || !validBody(outerBody, bodyCount) {
				return errors.New("program/flow/binding: Capture Outer mismatch")
			}
			if !bodies.AncestorOrSelf(outerBody, owner) {
				return errors.New("program/flow/binding: Capture Outer ancestry mismatch")
			}
			ownerActivation, ownerActivationOK := bodies.Activation(owner)
			outerActivation, outerActivationOK := bodies.Activation(outerBody)
			if !ownerActivationOK || !outerActivationOK || ownerActivation != outerActivation {
				return errors.New("program/flow/binding: Capture Outer activation mismatch")
			}
			outerOrdinal := keyspace.TermOrdinal(outer)
			if seenOuter[outerOrdinal] == marker {
				return errors.New("program/flow/binding: duplicate Capture Outer")
			}
			seenOuter[outerOrdinal] = marker
			if err := plane.assignRole(cells, inner, kind.CellCapture, functionBody, function, uint32(at+1), bodyCount); err != nil {
				return err
			}
		}
	}
	return nil
}

// sealFunctionCells derives the sole narrow Function identity projection.
// Source's Bind order supplies the Cell at each positional slot, while the
// authored Values row supplies the Function term installed at that slot. Only
// a local Cell owned by the same Bind Body and a Function authored by that
// Body can establish self identity. The projection is one-way and dense by
// Function ordinal; claimedCells is Seal-local duplicate scratch and is not
// published.
func sealFunctionCells(
	preimage source.Preimage,
	view authored.View,
	binds authored.Binds,
	functions authored.Functions,
	cells authored.Cells,
	plane rolePlane,
	bodyCount int,
) ([]keyspace.Term, error) {
	functionCells := make([]keyspace.Term, functions.Count()+1)
	claimedCells := make([]bool, cells.Count()+1)
	order := preimage.Binds()
	values := view.Values()
	for index := 0; index < binds.Count(); index++ {
		bind, ok := binds.At(index)
		if !ok {
			return nil, errors.New("program/flow/binding: Bind view is not live")
		}
		bindOwner, valuesTerm, ok := binds.Get(bind)
		if !ok || !validBody(bindOwner, bodyCount) {
			return nil, errors.New("program/flow/binding: invalid Bind owner")
		}
		valuesOwner, _, ok := values.Get(valuesTerm)
		if !ok {
			return nil, errors.New("program/flow/binding: Bind Values row is not live")
		}
		// A Values row belongs to the Body that evaluates it. A mismatched row
		// cannot prove that a Function is this Bind's self identity; leave it
		// absent rather than manufacturing a cross-Body relation.
		if valuesOwner != bindOwner {
			continue
		}
		bindLength, ok := order.Len(bind)
		if !ok || bindLength < 0 {
			return nil, errors.New("program/flow/binding: Bind order is not live")
		}
		valueLength, ok := values.Len(valuesTerm)
		if !ok || valueLength < 0 {
			return nil, errors.New("program/flow/binding: Bind Values order is not live")
		}
		if valueLength > bindLength {
			valueLength = bindLength
		}
		for at := 0; at < valueLength; at++ {
			cell, ok := order.At(bind, at)
			if !ok || !validCell(cell, cells.Count()) {
				return nil, errors.New("program/flow/binding: Bind Cell order is not live")
			}
			value, ok := values.Member(valuesTerm, at)
			if !ok || keyspace.TermFamily(value) != keyspace.FamilyFunction {
				continue
			}
			if !validFunction(value, functions.Count()) {
				return nil, errors.New("program/flow/binding: Bind Function value is not live")
			}
			functionOwner, _, _, ok := functions.Get(value)
			if !ok {
				return nil, errors.New("program/flow/binding: Function view is not live")
			}
			// A Function authored by another Body is not this Bind's self
			// identity, even if malformed Values data places it in this slot.
			if functionOwner != bindOwner {
				continue
			}
			functionOrdinal := keyspace.TermOrdinal(value)
			cellOrdinal := keyspace.TermOrdinal(cell)
			if functionCells[functionOrdinal] != 0 || claimedCells[cellOrdinal] {
				return nil, errors.New("program/flow/binding: ambiguous Function Cell binding")
			}
			if cellOrdinal == 0 || int(cellOrdinal) >= len(plane.roles) || len(plane.hosts) != len(plane.roles) ||
				plane.roles[cellOrdinal] != kind.CellLocal || plane.hosts[cellOrdinal] != bind {
				// Binding roles already reject nonlocal/global Cells in Source
				// Bind order. Keep this projection fail-closed if a foreign or
				// malformed Result is ever assembled internally.
				continue
			}
			functionCells[functionOrdinal] = cell
			claimedCells[cellOrdinal] = true
		}
	}
	return functionCells, nil
}

// rolePlane is the dense definition column under construction: the role, the
// definition host, and the host-order slot of every Cell ordinal. Index zero
// is the reserved invalid Term.
type rolePlane struct {
	roles []kind.CellRole
	hosts []keyspace.Term
	slots []uint32
}

func newRolePlane(cellCount int) rolePlane {
	return rolePlane{
		roles: make([]kind.CellRole, cellCount+1),
		hosts: make([]keyspace.Term, cellCount+1),
		slots: make([]uint32, cellCount+1),
	}
}

// assignRole claims one Cell for exactly one definition role. slot is the
// Cell's one-based position in the ordered group its host introduces, and is
// zero when the host introduces the Cell outright: a Function or chunk
// Vararg, and a global Cell's Program scope.
func (plane rolePlane) assignRole(cells authored.Cells, cell keyspace.Term, role kind.CellRole, body, host keyspace.Term, slot uint32, bodyCount int) error {
	if !validCell(cell, len(plane.roles)-1) || !validRole(role) {
		return errors.New("program/flow/binding: invalid Cell role input")
	}
	ordinal := keyspace.TermOrdinal(cell)
	if plane.roles[ordinal] != 0 {
		return errors.New("program/flow/binding: Cell has multiple definition roles")
	}
	cellKind, cellBody, key, ok := cells.Get(cell)
	if !ok {
		return errors.New("program/flow/binding: Cell view is not live")
	}
	if role == kind.CellGlobal {
		if cellKind != authored.CellGlobal || cellBody != 0 || key == 0 || body != 0 || host != 0 || slot != 0 {
			return errors.New("program/flow/binding: invalid Global role")
		}
	} else if cellKind != authored.CellLocal || key != 0 || !validBody(body, bodyCount) || cellBody != body || host == 0 {
		return errors.New("program/flow/binding: invalid lexical role")
	}
	plane.roles[ordinal] = role
	plane.hosts[ordinal] = host
	plane.slots[ordinal] = slot
	return nil
}

func validBody(term keyspace.Term, count int) bool {
	return keyspace.ValidTerm(term, keyspace.FamilyBody, count)
}

func validCell(term keyspace.Term, count int) bool {
	return keyspace.ValidTerm(term, keyspace.FamilyCell, count)
}

func validFunction(term keyspace.Term, count int) bool {
	return keyspace.ValidTerm(term, keyspace.FamilyFunction, count)
}
