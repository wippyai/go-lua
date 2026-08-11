package directbinding

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	flowbinding "github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

// Seal builds one shared iterative exact selector parent-chain and its three
// typed projections. Source is supplied as a Preimage because exact-key
// normalization is a Source-owned construction-time capability; Static and
// Module are read only through their typed views while their owner finalizers
// are live. No owner is retained in the returned Result.
func Seal(
	preimage source.Preimage,
	flow authored.View,
	bodies *body.Result,
	bindings flowbinding.Result,
	staticView static.View,
	moduleView module.View,
) (*Result, error) {
	if err := validateInputs(preimage, flow, bodies, bindings, staticView, moduleView); err != nil {
		return nil, err
	}
	sourceID := preimage.Identity().ContentID()
	flowID := flow.Cold().ContentID()
	staticID := staticView.ContentID()
	moduleID := moduleView.ContentID()

	reads := flow.Storage().Reads()
	cells := flow.Storage().Cells()
	keys := preimage.Keys()
	cellCount := cells.Count()
	readCount := reads.Count()
	publicationCount := staticView.Publications().Count()

	aliases := make([]bool, cellCount+1)
	bodyCount := preimage.Identity().FamilyCount(keyspace.FamilyBody)
	if err := collectImportAliases(preimage, flow, bindings, moduleView, aliases, cells.Count(), bodyCount, flow.Calls().Count()); err != nil {
		return nil, err
	}

	builder := selectorBuilder{
		preimage:          preimage,
		keys:              keys,
		flow:              flow,
		bodies:            bodies,
		aliases:           aliases,
		bodyCount:         bodyCount,
		rows:              make([]selectionRow, 0, readCount+publicationCount),
		slots:             make([]uint32, readCount+1),
		state:             make([]uint8, readCount+1),
		trail:             make([]keyspace.Term, readCount),
		publicationOwners: make([]keyspace.Term, publicationCount+1),
	}

	// Materialize every syntactically admissible Read once. This gives the
	// projection its dense Read slot and also makes every acyclic exact chain
	// available to later publication and call checks. Dynamic/FieldKey and
	// non-string FieldExact Reads simply have no selector row.
	for index := 0; index < readCount; index++ {
		read, ok := reads.At(index)
		if !ok {
			return nil, errors.New("program/flow/directbinding: Read ordinal is unavailable")
		}
		if _, _, err := builder.ensureRead(read); err != nil {
			return nil, err
		}
	}

	publicationStart := uint32(len(builder.rows) + 1)
	publicationSlots := make([]uint32, publicationCount+1)
	if err := builder.buildPublications(staticView, publicationSlots); err != nil {
		return nil, err
	}

	callCount := flow.Calls().Count()
	directCalls := make([]directCallRow, callCount+1)
	if err := builder.buildCalls(directCalls); err != nil {
		return nil, err
	}

	return &Result{
		selections:        builder.rows,
		rowReads:          builder.rowReads,
		readSlots:         builder.slots,
		publication:       publicationSlots,
		publicationStart:  publicationStart,
		publicationOwners: builder.publicationOwners,
		directCalls:       directCalls,
		sourceID:          sourceID,
		flowID:            flowID,
		staticID:          staticID,
		moduleID:          moduleID,
	}, nil
}

type selectorBuilder struct {
	preimage  source.Preimage
	keys      source.Keys
	flow      authored.View
	bodies    *body.Result
	aliases   []bool
	bodyCount int

	rows              []selectionRow
	rowReads          []keyspace.Term
	slots             []uint32
	state             []uint8
	trail             []keyspace.Term
	publicationOwners []keyspace.Term
}

func (b *selectorBuilder) row(index uint32) (selectionRow, bool) {
	if b == nil || index == 0 || uint64(index) > uint64(len(b.rows)) {
		return selectionRow{}, false
	}
	return b.rows[index-1], true
}

func (b *selectorBuilder) appendRow(row selectionRow, read keyspace.Term) uint32 {
	b.rows = append(b.rows, row)
	b.rowReads = append(b.rowReads, read)
	return uint32(len(b.rows))
}

// ensureRead resolves one Read iteratively. state=1 is the active trail,
// state=2 is a completed (possibly absent) result. Exact Lens -> Read edges
// are followed only when the exact suffix is an admissible normalized string.
func (b *selectorBuilder) ensureRead(read keyspace.Term) (uint32, bool, error) {
	if b == nil || keyspace.TermFamily(read) != keyspace.FamilyRead {
		return 0, false, nil
	}
	ordinal := keyspace.TermOrdinal(read)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(b.slots)) {
		return 0, false, nil
	}
	if b.slots[ordinal] != 0 {
		return b.slots[ordinal], true, nil
	}
	if b.state[ordinal] == 2 {
		return 0, false, nil
	}

	trailLen := 0
	current := read
	parent := uint32(0)
	for {
		currentOrdinal := keyspace.TermOrdinal(current)
		if keyspace.TermFamily(current) != keyspace.FamilyRead || currentOrdinal == 0 ||
			uint64(currentOrdinal) >= uint64(len(b.slots)) {
			b.markTrail(trailLen)
			return 0, false, nil
		}
		if b.slots[currentOrdinal] != 0 {
			parent = b.slots[currentOrdinal]
			break
		}
		if b.state[currentOrdinal] == 1 {
			return 0, false, errors.New("program/flow/directbinding: cyclic exact Read chain")
		}
		if b.state[currentOrdinal] == 2 {
			b.markTrail(trailLen)
			return 0, false, nil
		}
		if trailLen >= len(b.trail) {
			return 0, false, errors.New("program/flow/directbinding: exact Read trail overflow")
		}
		b.state[currentOrdinal] = 1
		b.trail[trailLen] = current
		trailLen++

		owner, sourceTerm, _, ok := b.flow.Storage().Reads().Get(current)
		if !ok {
			return 0, false, errors.New("program/flow/directbinding: Read row is unavailable")
		}
		if !validFlowBody(owner, b.bodyCount) {
			return 0, false, errors.New("program/flow/directbinding: Read owner is outside Source Body universe")
		}
		switch keyspace.TermFamily(sourceTerm) {
		case keyspace.FamilyCell:
			cellKind, cellBody, exact, cellOK := b.flow.Storage().Cells().Get(sourceTerm)
			if !cellOK {
				return 0, false, errors.New("program/flow/directbinding: Read Cell row is unavailable")
			}
			if cellKind == authored.CellGlobal {
				if cellBody != 0 || exact == 0 {
					return 0, false, errors.New("program/flow/directbinding: global Cell has a Body owner")
				}
			} else if cellKind == authored.CellLocal {
				if !validFlowBody(cellBody, b.bodyCount) || exact != 0 || b.bodies == nil || !b.bodies.Contains(cellBody, owner) {
					return 0, false, errors.New("program/flow/directbinding: local Cell owner disagrees with Read")
				}
			} else {
				return 0, false, errors.New("program/flow/directbinding: Read Cell kind is unavailable")
			}
			row, ok := b.rootRow(owner, sourceTerm)
			if !ok {
				return 0, false, errors.New("program/flow/directbinding: Read Cell is not an exact root")
			}
			parent = b.appendRow(row, current)
			b.slots[currentOrdinal] = parent
			b.state[currentOrdinal] = 2
			trailLen--
			for trailLen > 0 {
				trailLen--
				if _, ok := b.expandRead(b.trail[trailLen], parent); !ok {
					b.markTrail(trailLen)
					return 0, false, nil
				}
				parent = b.slots[keyspace.TermOrdinal(b.trail[trailLen])]
			}
			return b.slots[ordinal], b.slots[ordinal] != 0, nil
		case keyspace.FamilyLensExact:
			lensOwner, base, _, _, lensOK := b.flow.Access().Exact().Get(sourceTerm)
			if !lensOK || lensOwner != owner {
				return 0, false, errors.New("program/flow/directbinding: exact Lens relation is malformed")
			}
			// An exact Lens over a scalar literal or other non-Read value is
			// a valid evaluated access but cannot extend a lexical selector
			// chain. It contributes no DirectBinding row; only a same-owner
			// Read base is traversable here.
			if keyspace.TermFamily(base) != keyspace.FamilyRead || keyspace.TermOrdinal(base) == 0 {
				b.markTrail(trailLen)
				return 0, false, nil
			}
			baseOwner, _, _, baseOK := b.flow.Storage().Reads().Get(base)
			if !baseOK || !validFlowBody(baseOwner, b.bodyCount) || baseOwner != owner {
				return 0, false, errors.New("program/flow/directbinding: exact Lens Read owner disagrees")
			}
			base, valid := b.exactBase(owner, sourceTerm)
			if !valid {
				b.markTrail(trailLen)
				return 0, false, nil
			}
			current = base
		default:
			// Dynamic and unsupported access kinds are deliberately fences.
			b.markTrail(trailLen)
			return 0, false, nil
		}
	}

	// The loop reaches this point only through an already-completed parent.
	trailLen--
	for trailLen >= 0 {
		if _, ok := b.expandRead(b.trail[trailLen], parent); !ok {
			b.markTrail(trailLen + 1)
			return 0, false, nil
		}
		parent = b.slots[keyspace.TermOrdinal(b.trail[trailLen])]
		trailLen--
	}
	return b.slots[ordinal], b.slots[ordinal] != 0, nil
}

func (b *selectorBuilder) markTrail(length int) {
	for index := 0; index < length; index++ {
		ordinal := keyspace.TermOrdinal(b.trail[index])
		if ordinal != 0 && uint64(ordinal) < uint64(len(b.state)) {
			b.state[ordinal] = 2
		}
	}
}

func (b *selectorBuilder) expandRead(read keyspace.Term, parent uint32) (selectionRow, bool) {
	if b == nil || keyspace.TermFamily(read) != keyspace.FamilyRead || parent == 0 {
		return selectionRow{}, false
	}
	ordinal := keyspace.TermOrdinal(read)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(b.slots)) {
		return selectionRow{}, false
	}
	owner, sourceTerm, _, ok := b.flow.Storage().Reads().Get(read)
	if !ok || keyspace.TermFamily(sourceTerm) != keyspace.FamilyLensExact {
		return selectionRow{}, false
	}
	lensOwner, base, _, fieldKind, ok := b.flow.Access().Exact().Get(sourceTerm)
	if !ok || lensOwner != owner || base == 0 || keyspace.TermFamily(base) != keyspace.FamilyRead ||
		keyspace.TermOrdinal(base) == 0 || keyspace.TermOrdinal(base) >= uint32(len(b.slots)) ||
		b.slots[keyspace.TermOrdinal(base)] != parent {
		return selectionRow{}, false
	}
	suffix, typeSegment := b.exactSuffix(sourceTerm, lensOwner, fieldKind)
	if suffix == 0 {
		return selectionRow{}, false
	}
	baseOwner, _, _, baseReadOK := b.flow.Storage().Reads().Get(base)
	baseRow, ok := b.row(parent)
	if !ok || !baseReadOK || baseOwner != owner || baseRow.depth == ^uint32(0) {
		return selectionRow{}, false
	}
	row := selectionRow{
		root:     baseRow.root,
		parent:   parent,
		suffix:   suffix,
		depth:    baseRow.depth + 1,
		external: baseRow.external,
		plane:    selectionPlaneRead,
		typePath: baseRow.typePath && typeSegment,
	}
	index := b.appendRow(row, read)
	b.slots[ordinal] = index
	b.state[ordinal] = 2
	return row, true
}

func (b *selectorBuilder) rootRow(owner, sourceTerm keyspace.Term) (selectionRow, bool) {
	kindValue, body, exact, ok := b.flow.Storage().Cells().Get(sourceTerm)
	if !ok || sourceTerm == 0 || keyspace.TermFamily(sourceTerm) != keyspace.FamilyCell || keyspace.TermOrdinal(sourceTerm) == 0 {
		return selectionRow{}, false
	}
	switch kindValue {
	case authored.CellGlobal:
		atom, exactOK := b.keys.Exact(exact)
		if !exactOK || atom.Kind != keyspace.LiteralString || exact == 0 {
			return selectionRow{}, false
		}
		return selectionRow{
			root: sourceTerm, external: true, plane: selectionPlaneRead, typePath: true,
		}, true
	case authored.CellLocal:
		if !validFlowBody(body, b.bodyCount) || b.bodies == nil || !b.bodies.Contains(body, owner) {
			return selectionRow{}, false
		}
		ordinal := keyspace.TermOrdinal(sourceTerm)
		external := uint64(ordinal) < uint64(len(b.aliases)) && b.aliases[ordinal]
		return selectionRow{
			root: sourceTerm, external: external, plane: selectionPlaneRead, typePath: true,
		}, true
	default:
		return selectionRow{}, false
	}
}

func (b *selectorBuilder) exactBase(owner, sourceTerm keyspace.Term) (keyspace.Term, bool) {
	lensOwner, base, _, fieldKind, ok := b.flow.Access().Exact().Get(sourceTerm)
	if !ok || lensOwner != owner || keyspace.TermFamily(base) != keyspace.FamilyRead || keyspace.TermOrdinal(base) == 0 {
		return 0, false
	}
	baseOwner, _, _, readOK := b.flow.Storage().Reads().Get(base)
	if !readOK || baseOwner != owner {
		return 0, false
	}
	suffix, _ := b.exactSuffix(sourceTerm, lensOwner, fieldKind)
	if suffix == 0 {
		return 0, false
	}
	return base, true
}

func (b *selectorBuilder) exactSuffix(lens keyspace.Term, owner keyspace.Term, fieldKind kind.FieldKind) (keyspace.Key, bool) {
	lensOwner, _, sourceTerm, storedKind, ok := b.flow.Access().Exact().Get(lens)
	if !ok || lensOwner != owner || storedKind != fieldKind {
		return 0, false
	}
	switch fieldKind {
	case kind.FieldName:
		keyOwner, _, exact, keyOK := b.keys.Name(sourceTerm)
		if !keyOK || keyOwner != owner || exact == 0 {
			return 0, false
		}
		atom, atomOK := b.keys.Exact(exact)
		return exact, atomOK && atom.Kind == keyspace.LiteralString
	case kind.FieldExact:
		literal, literalOwner, literalOK := b.exactLiteral(sourceTerm)
		if !literalOK || literalOwner != owner || literal.Kind != keyspace.LiteralString {
			return 0, false
		}
		// Source.Keys.Find is the owner-provided binary search; retaining no
		// second key index keeps the seal at O(N log K) in normalized key count.
		exact, exactOK := b.keys.Find(literal)
		if !exactOK || exact == 0 {
			return 0, false
		}
		atom, atomOK := b.keys.Exact(exact)
		if !atomOK || atom.Kind != keyspace.LiteralString {
			return 0, false
		}
		// A normalized string bracket is a valid direct selector, but it is
		// not an authored FieldName segment and therefore cannot participate
		// in a Static type-publication path.
		return exact, false
	default:
		return 0, false
	}
}

func (b *selectorBuilder) exactLiteral(term keyspace.Term) (keyspace.LiteralValue, keyspace.Term, bool) {
	if b == nil {
		return keyspace.LiteralValue{}, 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return keyspace.LiteralValue{}, 0, false
	}
	index := int(ordinal - 1)
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBool:
		_, owner, value, ok := b.preimage.Literals().Bools().At(index)
		if ok {
			return keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}, owner, true
		}
	case keyspace.FamilyInteger:
		_, owner, value, ok := b.preimage.Literals().Integers().At(index)
		if ok {
			return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}, owner, true
		}
	case keyspace.FamilyFloat:
		_, owner, bits, ok := b.preimage.Literals().Floats().At(index)
		if ok {
			return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: bits}, owner, true
		}
	case keyspace.FamilyString:
		_, owner, value, ok := b.preimage.Literals().Strings().At(index)
		if ok {
			return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}, owner, true
		}
	case keyspace.FamilyUnary:
		owner, op, operand, ok := b.flow.Operators().Unaries().Get(term)
		if !ok || op != kind.UnaryNeg {
			return keyspace.LiteralValue{}, 0, false
		}
		// The authored static-key vocabulary admits UnaryNeg only over a
		// source Integer or Float. Read that one source literal directly: a
		// second Unary is not an authored static-key candidate, and avoiding a
		// recursive walk is part of this seal's deep-chain contract.
		var value keyspace.LiteralValue
		var valueOK bool
		switch keyspace.TermFamily(operand) {
		case keyspace.FamilyInteger:
			_, operandOwner, integer, literalOK := b.preimage.Literals().Integers().At(int(keyspace.TermOrdinal(operand) - 1))
			if literalOK && operandOwner == owner {
				value = keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: integer}
				valueOK = true
			}
		case keyspace.FamilyFloat:
			_, operandOwner, bits, literalOK := b.preimage.Literals().Floats().At(int(keyspace.TermOrdinal(operand) - 1))
			if literalOK && operandOwner == owner {
				value = keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: bits}
				valueOK = true
			}
		}
		if !valueOK {
			return keyspace.LiteralValue{}, 0, false
		}
		switch value.Kind {
		case keyspace.LiteralInteger:
			if value.Integer == math.MinInt64 {
				return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(-float64(value.Integer))}, owner, true
			}
			value.Integer = -value.Integer
			return value, owner, true
		case keyspace.LiteralFloat:
			value.FloatBits = math.Float64bits(-math.Float64frombits(value.FloatBits))
			return value, owner, true
		default:
			return keyspace.LiteralValue{}, 0, false
		}
	}
	return keyspace.LiteralValue{}, 0, false
}

func (b *selectorBuilder) buildPublications(view static.View, slots []uint32) error {
	publications := view.Publications()
	assigns := b.flow.Storage().Assigns()
	writes := b.flow.Storage().Writes()
	exact := b.flow.Access().Exact()
	for index := 0; index < publications.Count(); index++ {
		publication := keyspace.MakeTerm(keyspace.FamilyTypePublication, uint32(index+1))
		assign, pair, _, ok := publications.Get(publication)
		if !ok || assign == 0 || keyspace.TermFamily(assign) != keyspace.FamilyAssign || keyspace.TermOrdinal(assign) == 0 {
			return errors.New("program/flow/directbinding: malformed Static publication")
		}
		owner, _, assignOK := assigns.Get(assign)
		if !assignOK || !validFlowBody(owner, b.bodyCount) {
			return errors.New("program/flow/directbinding: publication Assign owner is unavailable")
		}
		writeCount, countOK := assigns.WriteCount(assign)
		// Static Publication.Pair is the zero-based authored WriteAt index.
		if !countOK || uint64(pair) >= uint64(writeCount) {
			return errors.New("program/flow/directbinding: publication Pair has no Write")
		}
		write, writeOK := assigns.WriteAt(assign, int(pair))
		writeAssign, target, targetOK := writes.Get(write)
		if !writeOK || !targetOK || writeAssign != assign {
			return errors.New("program/flow/directbinding: publication Write parent disagrees")
		}
		if keyspace.TermFamily(target) != keyspace.FamilyLensExact || keyspace.TermOrdinal(target) == 0 {
			return errors.New("program/flow/directbinding: publication target is not exact")
		}
		lensOwner, base, _, fieldKind, lensOK := exact.Get(target)
		if !lensOK || lensOwner != owner || fieldKind != kind.FieldName || keyspace.TermFamily(base) != keyspace.FamilyRead {
			return errors.New("program/flow/directbinding: publication target is not a same-owner name lens")
		}
		parent, selected, err := b.ensureRead(base)
		if err != nil {
			return err
		}
		if !selected || parent == 0 {
			return errors.New("program/flow/directbinding: publication has no exact selection parent")
		}
		baseOwner, _, _, baseReadOK := b.flow.Storage().Reads().Get(base)
		baseRow, baseOK := b.row(parent)
		if !baseOK || !baseReadOK || !validFlowBody(baseOwner, b.bodyCount) || baseOwner != owner || !baseRow.typePath || baseRow.depth == ^uint32(0) {
			return errors.New("program/flow/directbinding: publication path is not a same-owner dotted path")
		}
		suffix, typeSegment := b.exactSuffix(target, owner, fieldKind)
		if suffix == 0 || !typeSegment {
			return errors.New("program/flow/directbinding: publication target has no normalized name key")
		}
		b.appendRow(selectionRow{
			root: baseRow.root, parent: parent, suffix: suffix,
			depth: baseRow.depth + 1, external: baseRow.external, plane: selectionPlanePublication, typePath: true,
		}, 0)
		slots[index+1] = uint32(len(b.rows))
		b.publicationOwners[index+1] = owner
	}
	return nil
}

func (b *selectorBuilder) buildCalls(slots []directCallRow) error {
	calls := b.flow.Calls()
	reads := b.flow.Storage().Reads()
	for index := 0; index < calls.Count(); index++ {
		call := keyspace.MakeTerm(keyspace.FamilyCall, uint32(index+1))
		owner, callee, receiver, _, ok := calls.Get(call)
		if !ok || !validFlowBody(owner, b.bodyCount) {
			return errors.New("program/flow/directbinding: malformed Call row")
		}
		if keyspace.TermFamily(callee) != keyspace.FamilyRead || keyspace.TermOrdinal(callee) == 0 {
			continue
		}
		readOwner, _, _, readOK := reads.Get(callee)
		if !readOK || readOwner != owner {
			continue
		}
		ordinal := keyspace.TermOrdinal(callee)
		if uint64(ordinal) >= uint64(len(b.slots)) {
			continue
		}
		selection := b.slots[ordinal]
		row, rowOK := b.row(selection)
		if !rowOK || !row.external {
			continue
		}
		form := CallFormPlain
		if receiver != 0 {
			form = CallFormMethod
		}
		slots[index+1] = directCallRow{read: callee, form: form}
	}
	return nil
}
