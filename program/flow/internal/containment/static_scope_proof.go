package containment

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
	staticrole "github.com/wippyai/go-lua/program/static/role"
)

// ScopeObservationKind is the closed observation vocabulary used by the
// assembly-local Static scope proof.  It is intentionally defined in this
// internal package: it is not a Program term family, an artifact row, or an
// analysis/domain fact.
type ScopeObservationKind uint8

const (
	ScopeObservationInvalid ScopeObservationKind = iota
	ScopeObservationSourceOccurrence
	ScopeObservationCellIntroduction
	ScopeObservationFunctionGeneric
	ScopeObservationFunctionHeader
)

// scopeObservation is the private immutable descriptor retained only by the
// assembly-local proof.  Its term is always the exact source occurrence,
// Cell, or Function named by kind; no owner pointer, scope view, or copied
// relation is retained.
type scopeObservation struct {
	kind ScopeObservationKind
	term keyspace.Term
}

// scopeRecord keeps the exact lexical Body and its one observation together.
// Records are dense only in the closed static-scope families, so queries are
// O(1) and do not allocate.  The proof never embeds this storage in Result.
type scopeRecord struct {
	body        keyspace.Term
	observation scopeObservation
}

// StaticScopeProof is the transient, assembly-only proof of static-scope
// resolution.  It is returned alongside the retained containment Result and
// is deliberately not part of Result, Flow content, or artifact bytes.
//
// The proof carries only the exact Source, authored Flow, Static, and Module
// identities needed to reject equal-cardinality foreign owners.  Its dense
// rows contain no source spans, owner view, resolver, or construction
// capability.
type StaticScopeProof struct {
	sourceID keyspace.ContentID
	flowID   keyspace.ContentID
	staticID keyspace.ContentID
	moduleID keyspace.ContentID
	rows     [keyspace.FamilyCount][]scopeRecord
}

// Matches reports whether proof was sealed from exactly the supplied Source,
// authored Flow, Static, and Module identities.  The argument order is the
// canonical post-containment provenance order.
func (proof *StaticScopeProof) Matches(sourceID, flowID, staticID, moduleID keyspace.ContentID) bool {
	return proof != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		proof.sourceID == sourceID && proof.flowID == flowID && proof.staticID == staticID && proof.moduleID == moduleID
}

func (proof *StaticScopeProof) available() bool {
	return proof != nil && proof.sourceID.Available() && proof.flowID.Available() && proof.staticID.Available() && proof.moduleID.Available()
}

// Body returns the exact lexical Body for one closed static-scope handle.
// Malformed, unreferenced, rejected, and foreign terms fail closed.
func (proof *StaticScopeProof) Body(scope keyspace.Term) (keyspace.Term, bool) {
	record, ok := proof.record(scope)
	if !ok || record.body == 0 {
		return 0, false
	}
	return record.body, true
}

// Observation returns the exact terminal observation for one resolved scope.
// The returned term is respectively a source occurrence, Cell introduction,
// Function generic, or Function header according to kind.
func (proof *StaticScopeProof) Observation(scope keyspace.Term) (ScopeObservationKind, keyspace.Term, bool) {
	record, ok := proof.record(scope)
	if !ok || record.body == 0 || record.observation.kind == ScopeObservationInvalid || record.observation.term == 0 {
		return ScopeObservationInvalid, 0, false
	}
	return record.observation.kind, record.observation.term, true
}

func (proof *StaticScopeProof) record(scope keyspace.Term) (scopeRecord, bool) {
	if !proof.available() {
		return scopeRecord{}, false
	}
	family, ordinal := keyspace.TermFamily(scope), keyspace.TermOrdinal(scope)
	if !staticrole.ScopeHandleFamily(family) || ordinal == 0 || uint64(ordinal) > uint64(len(proof.rows[family])) {
		return scopeRecord{}, false
	}
	record := proof.rows[family][ordinal-1]
	if record.body == 0 || keyspace.TermFamily(record.body) != keyspace.FamilyBody {
		return scopeRecord{}, false
	}
	return record, true
}

// sealStaticScopeProof materializes the one proof projection needed by the
// later StaticCheck pass.  It deliberately reuses resolver's memoized walk;
// there is no second resolver, graph, or forwarding representation.
func sealStaticScopeProof(
	preimage source.Preimage,
	staticView static.View,
	view authored.View,
	staticID keyspace.ContentID,
	moduleID keyspace.ContentID,
	resolver *staticScopeResolver,
	counts [keyspace.FamilyCount]uint32,
) (*StaticScopeProof, error) {
	if resolver == nil || !staticView.Available() || !view.Cold().ContentID().Available() {
		return nil, errors.New("program/flow/containment: static scope proof owner view expired")
	}
	sourceID := preimage.Identity().ContentID()
	flowID := view.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return nil, errors.New("program/flow/containment: static scope proof identity unavailable")
	}
	proof := &StaticScopeProof{sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID}

	// These three typed rows are the complete static-scope input boundary.
	// TypeFunction Scope, TypeOf Scope, and Annotation Scope are the only
	// authored references that can request a lexical static frontier.  A
	// referenced TypeParam/Call/ValueClaim/Function is reached through the
	// same resolver and therefore gets the same exact memo row.
	accept := func(scope keyspace.Term) error {
		if !staticrole.ScopeHandle(counts, scope) {
			return errors.New("program/flow/containment: invalid static scope proof handle")
		}
		body, observation, ok := resolver.resolveObservation(scope)
		if !ok || keyspace.TermFamily(body) != keyspace.FamilyBody || observation.kind == ScopeObservationInvalid || observation.term == 0 {
			return errors.New("program/flow/containment: static scope does not resolve to an exact Body")
		}
		family, ordinal := keyspace.TermFamily(scope), keyspace.TermOrdinal(scope)
		if proof.rows[family] == nil {
			proof.rows[family] = make([]scopeRecord, int(counts[family]))
		}
		rows := proof.rows[family]
		if ordinal == 0 || uint64(ordinal) > uint64(len(rows)) {
			return errors.New("program/flow/containment: static scope proof handle escapes denominator")
		}
		record := scopeRecord{body: body, observation: observation}
		if prior := rows[ordinal-1]; prior.body != 0 && prior != record {
			return errors.New("program/flow/containment: static scope proof is not deterministic")
		}
		rows[ordinal-1] = record
		return nil
	}

	typeFunctions := staticView.Signatures().TypeFunctions()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyTypeFunction]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyTypeFunction, ordinal)
		scope, _, _, _, ok := typeFunctions.Get(term)
		if !ok || !staticrole.ScopeHandle(counts, scope) {
			return nil, errors.New("program/flow/containment: invalid TypeFunction scope proof row")
		}
		if err := accept(scope); err != nil {
			return nil, err
		}
	}

	typeOfs := staticView.Operators().TypeOfs()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyTypeOf]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyTypeOf, ordinal)
		scope, _, ok := typeOfs.Get(term)
		if !ok || !staticrole.ScopeHandle(counts, scope) {
			return nil, errors.New("program/flow/containment: invalid TypeOf scope proof row")
		}
		if err := accept(scope); err != nil {
			return nil, err
		}
	}

	annotations := staticView.Operands().Annotations()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyAnnotation]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyAnnotation, ordinal)
		row, ok := annotations.Get(term)
		if !ok || !staticrole.ScopeHandle(counts, row.Scope) {
			return nil, errors.New("program/flow/containment: invalid Annotation scope proof row")
		}
		if err := accept(row.Scope); err != nil {
			return nil, err
		}
	}
	return proof, nil
}
