package accessgeometry

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/lua/semantics/exactkey"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal derives Flow's normalized table/access geometry.  Source and Flow are
// already committed, so the returned Result retains only scalar identities and
// dense derived planes.  Candidate reads are emitted first, followed by
// candidate writes; no map, sort, recursive walk, or generic IR is involved.
func Seal(
	sourceView source.View,
	flow authored.View,
	candidateResult *candidates.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Result, error) {
	sourceID := sourceView.Identity().ContentID()
	flowID := flow.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return nil, errors.New("program/flow/accessgeometry: owner identity is unavailable")
	}
	if !candidates.Matches(candidateResult, sourceID, flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/accessgeometry: candidate provenance disagrees with Source, Flow, Static, or Module")
	}
	counts, err := validateDenominators(sourceView, flow)
	if err != nil {
		return nil, err
	}

	result := &Result{
		sourceID: sourceID,
		flowID:   flowID,
		staticID: staticID,
		moduleID: moduleID,
		tableFields: tableFieldProjection{
			keys: make([]keyspace.Key, counts.tableFields+1),
		},
		exactLenses: exactLensProjection{
			keys: make([]keyspace.Key, counts.exactLenses+1),
		},
		dynamicLenses: dynamicLensProjection{
			keys: make([]keyspace.Key, counts.dynamicLenses+1),
		},
		indexAccesses: indexProjection{
			reads:  make([]uint32, counts.reads+1),
			writes: make([]uint32, counts.writes+1),
		},
	}

	if err := deriveTableFields(sourceView, flow, result.tableFields.keys); err != nil {
		return nil, err
	}
	if err := deriveExactLenses(sourceView, flow, result.exactLenses.keys); err != nil {
		return nil, err
	}
	// DynamicLenses is intentionally a zero plane. The allocation above is the
	// complete authored denominator; every element is already the canonical
	// dynamic absence value.

	if err := deriveIndexAccesses(sourceView, flow, candidateResult, result, counts.writes, counts.assigns); err != nil {
		return nil, err
	}
	return result, nil
}

type denominators struct {
	tableFields   int
	exactLenses   int
	dynamicLenses int
	reads         int
	writes        int
	assigns       int
}

func validateDenominators(sourceView source.View, flow authored.View) (denominators, error) {
	var counts denominators
	sourceIdentity := sourceView.Identity()
	if sourceIdentity.Name() == "" || sourceIdentity.TermCount() == 0 || sourceIdentity.ContentID() == (identity.ContentID{}) {
		return counts, errors.New("program/flow/accessgeometry: Source view is unavailable")
	}
	counts.tableFields = flow.Fields().Count()
	counts.exactLenses = flow.Access().Exact().Count()
	counts.dynamicLenses = flow.Access().Dynamic().Count()
	counts.reads = flow.Storage().Reads().Count()
	counts.writes = flow.Storage().Writes().Count()
	counts.assigns = flow.Storage().Assigns().Count()
	if counts.tableFields != sourceIdentity.FamilyCount(keyspace.FamilyTableField) ||
		counts.exactLenses != sourceIdentity.FamilyCount(keyspace.FamilyLensExact) ||
		counts.dynamicLenses != sourceIdentity.FamilyCount(keyspace.FamilyLensKey) ||
		counts.reads != sourceIdentity.FamilyCount(keyspace.FamilyRead) ||
		counts.writes != sourceIdentity.FamilyCount(keyspace.FamilyWrite) ||
		counts.assigns != sourceIdentity.FamilyCount(keyspace.FamilyAssign) {
		return counts, errors.New("program/flow/accessgeometry: authored/source denominator mismatch")
	}
	if !keyspace.TermOrdinalFits(counts.tableFields) || !keyspace.TermOrdinalFits(counts.exactLenses) ||
		!keyspace.TermOrdinalFits(counts.dynamicLenses) || !keyspace.TermOrdinalFits(counts.reads) ||
		!keyspace.TermOrdinalFits(counts.writes) || !keyspace.TermOrdinalFits(counts.assigns) {
		return counts, errors.New("program/flow/accessgeometry: denominator is unrepresentable")
	}
	return counts, nil
}

func deriveTableFields(sourceView source.View, flow authored.View, keys []keyspace.Key) error {
	fields := flow.Fields()
	tables := flow.Tables()
	for ordinal := 1; ordinal < len(keys); ordinal++ {
		field := keyspace.MakeTerm(keyspace.FamilyTableField, uint32(ordinal))
		at, ok := fields.At(ordinal - 1)
		if !ok || at != field {
			return errors.New("program/flow/accessgeometry: TableField ordinal is not canonical")
		}
		table, sourceTerm, _, fieldKind, ok := fields.Get(field)
		if !ok {
			return errors.New("program/flow/accessgeometry: TableField row is unavailable")
		}
		tableOwner, ok := tables.Get(table)
		if !ok || !sourceBodyTerm(sourceView, tableOwner) {
			return errors.New("program/flow/accessgeometry: TableField table owner is unavailable")
		}
		key, valid, err := normalizedFieldKey(sourceView, flow, sourceTerm, fieldKind, tableOwner)
		if err != nil {
			return err
		}
		if !valid {
			return errors.New("program/flow/accessgeometry: TableField key source is malformed")
		}
		keys[ordinal] = key
	}
	return nil
}

func deriveExactLenses(sourceView source.View, flow authored.View, keys []keyspace.Key) error {
	exact := flow.Access().Exact()
	for ordinal := 1; ordinal < len(keys); ordinal++ {
		lens := keyspace.MakeTerm(keyspace.FamilyLensExact, uint32(ordinal))
		at, ok := exact.At(ordinal - 1)
		if !ok || at != lens {
			return errors.New("program/flow/accessgeometry: ExactLens ordinal is not canonical")
		}
		owner, _, sourceTerm, fieldKind, ok := exact.Get(lens)
		if !ok || !sourceBodyTerm(sourceView, owner) {
			return errors.New("program/flow/accessgeometry: ExactLens owner is unavailable")
		}
		key, valid, err := normalizedFieldKey(sourceView, flow, sourceTerm, fieldKind, owner)
		if err != nil {
			return err
		}
		if !valid {
			return errors.New("program/flow/accessgeometry: ExactLens key source is malformed")
		}
		keys[ordinal] = key
	}
	return nil
}

func deriveIndexAccesses(
	sourceView source.View,
	flow authored.View,
	candidateResult *candidates.Result,
	result *Result,
	writeCount int,
	assignCount int,
) error {
	if result == nil || candidateResult == nil {
		return errors.New("program/flow/accessgeometry: candidate result is unavailable")
	}
	get := candidateResult.IndexGet()
	set := candidateResult.IndexSet()
	result.indexAccesses.accesses = make([]indexAccess, 0, get.Count()+set.Count())
	result.indexAccesses.readCount = get.Count()
	result.indexAccesses.writeCount = set.Count()
	// Candidate IndexGet rows are emitted first, exactly in their existing
	// canonical authored order.
	for index := 0; index < get.Count(); index++ {
		read, ok := get.At(index)
		if !ok || keyspace.TermFamily(read) != keyspace.FamilyRead || keyspace.TermOrdinal(read) == 0 || !get.Contains(read) {
			return errors.New("program/flow/accessgeometry: candidate IndexGet Read is malformed")
		}
		owner, lens, _, ok := flow.Storage().Reads().Get(read)
		if !ok || !sourceBodyTerm(sourceView, owner) {
			return errors.New("program/flow/accessgeometry: candidate Read row is unavailable")
		}
		base, key, lensOK := lensGeometry(flow, result, lens, owner)
		if !lensOK || base == 0 {
			return errors.New("program/flow/accessgeometry: candidate Read Lens geometry is malformed")
		}
		// Dense slots use one-based access ordinals; zero is the explicit
		// non-candidate sentinel in every authored Read plane.
		accessIndex := uint32(len(result.indexAccesses.accesses))
		result.indexAccesses.accesses = append(result.indexAccesses.accesses, indexAccess{Read: read, Base: base, KeyTerm: key, Position: -1, Lens: lens})
		ordinal := keyspace.TermOrdinal(read)
		if ordinal == 0 || uint64(ordinal) >= uint64(len(result.indexAccesses.reads)) || result.indexAccesses.reads[ordinal] != 0 {
			return errors.New("program/flow/accessgeometry: candidate Read slot is malformed")
		}
		result.indexAccesses.reads[ordinal] = accessIndex + 1
	}
	// Scan the authored Assign ranges once. This supplies each candidate Set
	// access route's local Write position without retaining an all-Write position plane
	// and without restarting a Write×Assign search. Candidate Set rows are
	// already canonical; compare the encountered candidate ordinal to At so the
	// retained access order remains exactly IndexSet order.
	writes := flow.Storage().Writes()
	assigns := flow.Storage().Assigns()
	candidateOrdinal := 0
	seenWrites := 0
	for assignOrdinal := 1; assignOrdinal <= assignCount; assignOrdinal++ {
		assign := keyspace.MakeTerm(keyspace.FamilyAssign, uint32(assignOrdinal))
		if at, ok := assigns.At(assignOrdinal - 1); !ok || at != assign {
			return errors.New("program/flow/accessgeometry: Assign ordinal is not canonical")
		}
		writeCountAt, ok := assigns.WriteCount(assign)
		if !ok || writeCountAt <= 0 {
			return errors.New("program/flow/accessgeometry: Assign has no authored Writes")
		}
		owner, values, ok := assigns.Get(assign)
		if !ok || !sourceBodyTerm(sourceView, owner) || values == 0 {
			return errors.New("program/flow/accessgeometry: Assign Values are unavailable")
		}
		for position := 0; position < writeCountAt; position++ {
			write, writeOK := assigns.WriteAt(assign, position)
			if !writeOK || keyspace.TermFamily(write) != keyspace.FamilyWrite || keyspace.TermOrdinal(write) == 0 || int(keyspace.TermOrdinal(write)) > writeCount {
				return errors.New("program/flow/accessgeometry: Assign Write range is malformed")
			}
			seenWrites++
			if !set.Contains(write) {
				continue
			}
			want, wantOK := set.At(candidateOrdinal)
			if !wantOK || want != write {
				return errors.New("program/flow/accessgeometry: candidate IndexSet order disagrees with authored Write order")
			}
			_, lens, rowOK := writes.Get(write)
			if !rowOK {
				return errors.New("program/flow/accessgeometry: candidate Write row is unavailable")
			}
			base, key, lensOK := lensGeometry(flow, result, lens, owner)
			if !lensOK || base == 0 {
				return errors.New("program/flow/accessgeometry: candidate Write Lens geometry is malformed")
			}
			accessIndex := uint32(len(result.indexAccesses.accesses))
			result.indexAccesses.accesses = append(result.indexAccesses.accesses, indexAccess{Write: write, Base: base, KeyTerm: key, Values: values, Position: position, Lens: lens})
			writeOrdinal := keyspace.TermOrdinal(write)
			if uint64(writeOrdinal) >= uint64(len(result.indexAccesses.writes)) || result.indexAccesses.writes[writeOrdinal] != 0 {
				return errors.New("program/flow/accessgeometry: candidate Write slot is malformed")
			}
			result.indexAccesses.writes[writeOrdinal] = accessIndex + 1
			candidateOrdinal++
		}
	}
	if seenWrites != writeCount || candidateOrdinal != set.Count() {
		return errors.New("program/flow/accessgeometry: authored/candidate Write denominator is not covered")
	}
	return nil
}

func lensGeometry(flow authored.View, result *Result, lens, owner keyspace.Term) (base keyspace.Term, keyTerm keyspace.Term, ok bool) {
	if result == nil || owner == 0 {
		return 0, 0, false
	}
	switch keyspace.TermFamily(lens) {
	case keyspace.FamilyLensExact:
		lensOwner, lensBase, sourceTerm, _, rowOK := flow.Access().Exact().Get(lens)
		if !rowOK || lensOwner != owner {
			return 0, 0, false
		}
		_, rowOK = result.ExactLenses().Get(lens)
		return lensBase, sourceTerm, rowOK
	case keyspace.FamilyLensKey:
		lensOwner, lensBase, keyTerm, rowOK := flow.Access().Dynamic().Get(lens)
		if !rowOK || lensOwner != owner {
			return 0, 0, false
		}
		_, rowOK = result.DynamicLenses().Get(lens)
		return lensBase, keyTerm, rowOK
	default:
		return 0, 0, false
	}
}

func normalizedFieldKey(
	sourceView source.View,
	flow authored.View,
	sourceTerm keyspace.Term,
	fieldKind kind.FieldKind,
	wantOwner keyspace.Term,
) (keyspace.Key, bool, error) {
	keys := sourceView.Keys()
	switch fieldKind {
	case kind.FieldList:
		owner, _, key, ok := keys.List(sourceTerm)
		if !ok || owner != wantOwner {
			return 0, false, errors.New("program/flow/accessgeometry: FieldList source is not a Source ListKey")
		}
		return key, true, nil
	case kind.FieldName:
		owner, _, key, ok := keys.Name(sourceTerm)
		if !ok || owner != wantOwner {
			return 0, false, errors.New("program/flow/accessgeometry: FieldName source is not a Source NameKey")
		}
		return key, true, nil
	case kind.FieldKey:
		// Dynamic fields have no exact equality identity. Keep the row in the
		// dense plane but leave its key at the canonical zero value.
		if !sourceTermInDenominator(sourceView, sourceTerm) || !flowrole.ValueOccurrenceFamily(keyspace.TermFamily(sourceTerm)) {
			return 0, false, errors.New("program/flow/accessgeometry: FieldKey source is outside the Source denominator")
		}
		return 0, true, nil
	case kind.FieldExact:
		literal, literalOK := exactLiteral(sourceView, flow, sourceTerm, wantOwner)
		if !literalOK {
			return 0, false, errors.New("program/flow/accessgeometry: FieldExact source is malformed")
		}
		// Nil and NaN have no storable key. NormalizeExactKey is the Source
		// authority for every other exact literal; this pass never interns or
		// compares literals itself.
		if keyspace.TermFamily(sourceTerm) == keyspace.FamilyNil {
			return 0, true, nil
		}
		normalized, normalOK := exactkey.Normalize(literal)
		if !normalOK {
			return 0, true, nil
		}
		key, found := keys.Find(normalized)
		if !found {
			return 0, false, errors.New("program/flow/accessgeometry: normalized exact key is absent from Source denominator")
		}
		return key, true, nil
	default:
		return 0, false, errors.New("program/flow/accessgeometry: unsupported field kind")
	}
}

func sourceTermInDenominator(view source.View, term keyspace.Term) bool {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal != 0 &&
		uint64(ordinal) <= uint64(view.Identity().FamilyCount(family))
}

func sourceBodyTerm(view source.View, term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyBody && sourceTermInDenominator(view, term)
}

// exactLiteral resolves only Source literal rows and one authored UnaryNeg
// over an integer/float literal.  It is deliberately nonrecursive: authored
// Flow's static exact-key vocabulary admits no nested Unary operand.
func exactLiteral(sourceView source.View, flow authored.View, term, wantOwner keyspace.Term) (keyspace.LiteralValue, bool) {
	if keyspace.TermFamily(term) == keyspace.FamilyNil {
		ordinal := keyspace.TermOrdinal(term)
		if ordinal == 0 || int(ordinal) > sourceView.Identity().FamilyCount(keyspace.FamilyNil) {
			return keyspace.LiteralValue{}, false
		}
		_, owner, ok := sourceView.Literals().Nils().At(int(ordinal - 1))
		return keyspace.LiteralValue{}, ok && owner == wantOwner
	}
	if keyspace.TermOrdinal(term) == 0 {
		return keyspace.LiteralValue{}, false
	}
	if keyspace.TermFamily(term) == keyspace.FamilyUnary {
		owner, op, operand, ok := flow.Operators().Unaries().Get(term)
		if !ok || owner != wantOwner || op != kind.UnaryNeg || keyspace.TermOrdinal(operand) == 0 {
			return keyspace.LiteralValue{}, false
		}
		value, valueOwner, ok := sourceLiteral(sourceView, operand)
		if !ok || valueOwner != wantOwner || (keyspace.TermFamily(operand) != keyspace.FamilyInteger && keyspace.TermFamily(operand) != keyspace.FamilyFloat) {
			return keyspace.LiteralValue{}, false
		}
		if value.Kind == keyspace.LiteralInteger {
			if value.Integer == math.MinInt64 {
				return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(-float64(value.Integer))}, true
			}
			value.Integer = -value.Integer
			return value, true
		}
		value.FloatBits = math.Float64bits(-math.Float64frombits(value.FloatBits))
		return value, true
	}
	value, owner, ok := sourceLiteral(sourceView, term)
	return value, ok && owner == wantOwner
}

func sourceLiteral(view source.View, term keyspace.Term) (keyspace.LiteralValue, keyspace.Term, bool) {
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return keyspace.LiteralValue{}, 0, false
	}
	index := int(ordinal - 1)
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBool:
		_, owner, value, ok := view.Literals().Bools().At(index)
		return keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}, owner, ok
	case keyspace.FamilyInteger:
		_, owner, value, ok := view.Literals().Integers().At(index)
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}, owner, ok
	case keyspace.FamilyFloat:
		_, owner, bits, ok := view.Literals().Floats().At(index)
		return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: bits}, owner, ok
	case keyspace.FamilyString:
		_, owner, value, ok := view.Literals().Strings().At(index)
		return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}, owner, ok
	default:
		return keyspace.LiteralValue{}, 0, false
	}
}
