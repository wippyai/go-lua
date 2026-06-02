package flow

import (
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// PathObservationCandidate is one producer-specific observation candidate for a
// normalized path read. Producers own how candidates are computed; this package
// owns how candidates are selected.
type PathObservationCandidate struct {
	Type   typ.Type
	Source PathObservationSource
	OK     bool
}

// PathObservationSelection contains the normalized inputs for the path
// observation selection law. It is intentionally AST-free and solver-neutral.
type PathObservationSelection struct {
	Query    PathObservationQuery
	Declared typ.Type
	Direct   PathObservationCandidate
	Solved   PathObservationCandidate
	Proof    typ.Type
	// AdmitSelected applies the value-domain observation admission law to
	// selected non-declared observations when PreserveProof is false. Some legacy
	// assignment-source reads still finalize outside this policy, so producers own
	// the admission boundary while sharing the selection order.
	AdmitSelected bool
}

// AssignmentSourceObservationSelection contains the two canonical same-expression
// candidates available when observing an assignment RHS.
type AssignmentSourceObservationSelection struct {
	// Stored is the source-owned assignment evidence materialized by transfer.
	// It may be broad because it is computed before later path facts are folded.
	Stored typ.Type
	// Path is the normalized path observation for the same source expression.
	Path typ.Type
}

// SelectAssignmentSourceObservation chooses the best assignment-source
// observation without knowing AST syntax or solver internals. Stored evidence is
// the default because it owns non-path sources such as call returns and iterator
// projections. A normalized path proof can replace it only when it is strictly
// more precise under the gradual precision relation.
func SelectAssignmentSourceObservation(s AssignmentSourceObservationSelection) typ.Type {
	switch {
	case typ.IsAbsentOrUnknown(s.Stored):
		return s.Path
	case typ.IsAbsentOrUnknown(s.Path):
		return s.Stored
	case typ.IsAny(s.Stored) && !typ.IsAny(s.Path):
		return s.Path
	case typ.MorePrecise(s.Path, s.Stored):
		return s.Path
	default:
		return s.Stored
	}
}

// SelectPathObservationResult applies the producer-neutral path observation
// policy: direct path facts first, strict-pre fallback, authoritative bottom,
// local condition bottom, solved/proof selection, then declared fallback.
func SelectPathObservationResult(s PathObservationSelection) PathObservation {
	q := s.Query
	if q.Path.IsEmpty() {
		return PathObservation{}
	}
	if s.Direct.OK {
		selected := reconcilePathObservationType(s.Direct.Type, s.Declared)
		if directShouldYieldToProof(q, selected, s.Proof) {
			return selectedPathObservation(q, s.Declared, s.Proof, selected, s.Proof, PathObservationConditionProof, s.AdmitSelected)
		}
		return selectedPathObservation(q, s.Declared, selected, s.Direct.Type, nil, s.Direct.Source, s.AdmitSelected)
	}
	if q.StrictPhase && q.Phase == PathReadPre && !s.Solved.OK {
		return declaredPathObservation(s.Declared)
	}
	if typ.IsNever(s.Solved.Type) {
		return PathObservation{
			Type:     s.Solved.Type,
			State:    StateResolved,
			Source:   pathObservationSourceOr(s.Solved.Source, PathObservationFactProjection),
			Declared: s.Declared,
			Solved:   s.Solved.Type,
			Proof:    s.Proof,
		}
	}
	if q.AllowConditionProof && q.LocalCondition != nil && typ.IsNever(s.Proof) {
		return PathObservation{
			Type:     s.Proof,
			State:    StateResolved,
			Source:   PathObservationConditionProof,
			Declared: s.Declared,
			Solved:   s.Solved.Type,
			Proof:    s.Proof,
		}
	}
	if selected, ok := value.SelectPathObservation(s.Solved.Type, s.Proof, s.Declared); ok {
		source := pathObservationSourceOr(s.Solved.Source, PathObservationFactProjection)
		if typ.IsAbsentOrUnknown(s.Solved.Type) && !typ.IsAbsentOrUnknown(s.Proof) {
			source = PathObservationConditionProof
		}
		return selectedPathObservation(q, s.Declared, selected, s.Solved.Type, s.Proof, source, s.AdmitSelected)
	}
	return declaredPathObservation(s.Declared)
}

func selectedPathObservation(q PathObservationQuery, declared, selected, solved, proof typ.Type, source PathObservationSource, admit bool) PathObservation {
	if typ.IsAbsentOrUnknown(selected) {
		return PathObservation{}
	}
	if !typ.IsNever(selected) && admit && !q.PreserveProof {
		selected = value.AdmitObservation(selected)
	}
	return PathObservation{
		Type:     selected,
		State:    StateResolved,
		Source:   pathObservationSourceOr(source, PathObservationFactProjection),
		Declared: declared,
		Solved:   solved,
		Proof:    proof,
	}
}

func declaredPathObservation(declared typ.Type) PathObservation {
	if declared == nil {
		return PathObservation{}
	}
	return PathObservation{
		Type:     declared,
		State:    StateResolved,
		Source:   PathObservationDeclared,
		Declared: declared,
	}
}

func pathObservationSourceOr(source, fallback PathObservationSource) PathObservationSource {
	if source != PathObservationUnknown {
		return source
	}
	return fallback
}

func reconcilePathObservationType(observed, declared typ.Type) typ.Type {
	if t, ok := value.ReconcilePathFactWithDeclaredRead(observed, declared); ok {
		return t
	}
	return observed
}

func directShouldYieldToProof(q PathObservationQuery, direct, proof typ.Type) bool {
	if !q.AllowConditionProof || typ.IsAbsentOrUnknown(direct) || typ.IsAbsentOrUnknown(proof) || typ.TypeEquals(direct, proof) {
		return false
	}
	if typ.MorePrecise(proof, direct) {
		return true
	}
	proofSubDirect := subtype.IsSubtype(proof, direct)
	directSubProof := subtype.IsSubtype(direct, proof)
	return proofSubDirect && !directSubProof
}
