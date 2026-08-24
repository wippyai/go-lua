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
	Sources  []SourceRef
	Relation member.RelationRef
	Key      member.ProjectionRef
	// Predicate is the selection/tag projection a Selected or Summary read
	// resolves through. It is optional on both forms for the same underlying
	// reason a Parent is declared: a row addressed by something other than an
	// owner-issued tag needs no tag to declare.
	Predicate member.ProjectionRef
	// Parent restates Relation's own declared Parent (MemberParent in the
	// axis definition, member.Relation.Parent in the sealed catalog): the
	// relation whose candidate row Relation's rows nest under as a bounded,
	// ordinal-addressed member set. It is declared only when Relation is such
	// a self-provided nested member set, and never invented to describe a
	// relation that is not - seal/plan authenticate it against the resolved
	// relation's real Parent/Ordinal, the same way they authenticate Key and
	// Predicate against the relation they name. This declaration states the
	// fact; it does not decide it.
	Parent member.RelationRef
	Read   ReadDecl
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
	if join.Parent.Declared() {
		references = append(references, join.Parent.EntryReference())
	}
	return append(references, join.Read.References()...)
}

// normalForm checks the five inventory-derived combinations. It deliberately
// returns only a boolean: the five forms are seal facts, not a runtime enum.
//
// A join's normal form is a property of the join. Nothing about the enclosing
// Program participates: a form that held only when some output happened to
// name the join would not be a form, it would be a permission.
func (join JoinDecl) normalForm(position int) bool {
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
	if join.Parent.Declared() && !join.Parent.Available() {
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
		// S2 is a dependent keyed relation read; when Sources includes an
		// earlier result it is the S4 variant. The predicate is the owner's
		// tag/role metadata and is optional: a nested member set is already
		// addressed by (parent, ordinal), and a routed member already carries
		// its paired tag beside its destination, so a selection with nothing
		// to tag declares nothing. A predicate that IS declared must resolve,
		// which the clause above already holds it to.
		return true
	case Summary:
		// S3 is the selected relation summary; the denominator is mandatory
		// above. Predicate is the sealed selection/tag projection that
		// correlates each returned cell with its row, and is required for
		// every relation except one: a self-provided nested member set is
		// already addressed by (parent, ordinal) - its ordinal position IS
		// the correlation - so a Predicate declared over it would be a
		// second, duplicate tagging authority for the same row, the same
		// defect Selected's own untagged form exists to avoid. This
		// declaration states that fact as Parent, restating the relation's
		// own MemberParent/MemberOrdinal; it never infers nestedness from
		// this join's own shape, and seal/plan authenticate the restatement
		// against the resolved relation. No production rule declares
		// ruleprogram.Summary yet; this restatement is what admits the
		// first one.
		return join.Predicate.Available() || join.Parent.Available()
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
