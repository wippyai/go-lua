package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// SourceRef names either the owner-issued Candidate relation or an earlier
// JoinDecl result. Position is a zero-based join list position; Candidate is
// the only valid source with Candidate=true and position zero.
type SourceRef struct {
	Candidate bool
	Position  uint64
}

func CandidateSource() SourceRef { return SourceRef{Candidate: true} }

func PriorSource(position uint64) SourceRef { return SourceRef{Position: position} }

func (source SourceRef) AvailableBefore(joinPosition int) bool {
	if source.Candidate {
		return source.Position == 0
	}
	return source.Position < uint64(joinPosition)
}

// JoinRef is a zero-based result position in Program.Joins. Zero is a valid
// first result, so validity is checked against the enclosing join count.
type JoinRef uint64

// JoinDecl is the one relation declaration used by every census shape. Its
// normal form is sealed from Read.Form, projections, and ordered Sources; no
// public or private shape representation exists.
type JoinDecl struct {
	Sources   []SourceRef
	Relation  member.RelationRef
	Key       member.ProjectionRef
	Predicate member.ProjectionRef
	Read      ReadDecl
}

func (join JoinDecl) Available() bool {
	return len(join.Sources) != 0 && join.Relation.Available() && join.Key.Available() && join.Read.Available()
}

func (join JoinDecl) References() schema.EntryReferences {
	var references schema.EntryReferences
	if join.Relation.Declared() {
		references = append(references, join.Relation.EntryReference())
	}
	if join.Key.Declared() {
		references = append(references, join.Key.EntryReference())
	}
	if join.Predicate.Declared() {
		references = append(references, join.Predicate.EntryReference())
	}
	return append(references, join.Read.References()...)
}

// normalForm checks the five inventory-derived combinations. It deliberately
// returns only a boolean: the five forms are seal facts, not a runtime enum.
// normalForm checks one ordinary join in its local census normal form.
func (join JoinDecl) normalForm(position int) bool {
	return join.normalFormFor(position, false)
}

// normalFormFor checks one join in its local census normal form. A selected
// route may deliberately omit a tag projection: the route directory still
// carries its key and bounded read contract, while the tag is optional data.
// The enclosing Program supplies that one route-specific allowance so an
// ordinary selected read cannot silently become untagged.
func (join JoinDecl) normalFormFor(position int, optionalSelectedPredicate bool) bool {
	if !join.Available() {
		return false
	}
	seenSources := make(map[SourceRef]struct{}, len(join.Sources))
	for _, source := range join.Sources {
		if !source.AvailableBefore(position) {
			return false
		}
		if _, duplicate := seenSources[source]; duplicate {
			return false
		}
		seenSources[source] = struct{}{}
	}
	if join.Predicate.Declared() && !join.Predicate.Available() {
		return false
	}
	if join.Read.Contract.RequiresDenominator(join.Read.Form) && !join.Read.Contract.DenominatorRef.Available() {
		return false
	}
	if !join.Read.Contract.RequiresDenominator(join.Read.Form) && join.Read.Contract.DenominatorRef.Declared() && !join.Read.Contract.DenominatorRef.Available() {
		return false
	}

	switch join.Read.Form {
	case Exact:
		// S1 is the exact keyed lookup. If a key consumes an earlier result,
		// the same declaration is S4's computed-coordinate normal form.
		return !join.Predicate.Declared()
	case Selected:
		// S2 is a keyed lookup with an owner-issued tag/role predicate; when
		// Sources includes an earlier result it is the S4 variant. A routed
		// selected row may omit that predicate: the route's tag is optional,
		// not an implicit fallback or an untyped runtime callback.
		return join.Predicate.Available() || (optionalSelectedPredicate && !join.Predicate.Declared())
	case Summary:
		// S3 is the selected relation summary. Predicate is the sealed
		// selection/tag projection and the denominator is mandatory above.
		return join.Predicate.Available()
	case Complete:
		// S5 is the closed-denominator whole-vector read.
		return !join.Predicate.Declared()
	default:
		return false
	}
}

func cloneJoin(join JoinDecl) JoinDecl {
	join.Sources = append([]SourceRef(nil), join.Sources...)
	return join
}
