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
	// KeyVector names the directory whose rows publish the ordered dense key
	// vector this read is taken over: the coordinates a candidate row was
	// constructed from, which the axis they belong to issued one at a time and
	// groups nowhere. It is the third way a whole-vector read can be
	// addressed, declared only when Relation's own candidate provider is such
	// a directory, and authenticated against the sealed catalog exactly as
	// Parent is.
	//
	// A read declares one addressing, never two: a predicate names a tagged
	// selection, a parent names a nested member set, and this names a
	// candidate-published span.
	KeyVector member.RelationRef
	// AddressIdentity names the occurrence the corresponded foreign directory
	// this read is addressed by is enumerated under, projected from THIS rule's
	// own candidate row in the member.Identity role.
	//
	// A correspondence says two directories enumerate the same subjects, never
	// that they enumerate them in the same positions, so a corresponded read is
	// resolved through the occurrence both directories are addressed by. That
	// occurrence is the candidate's own whenever the candidate row IS the
	// subject. It is not when the row NAMES one: a directory may hold rows
	// drawn from several occurrence families, where one family's row is an
	// interpretation of a subject sealed elsewhere, and asking the foreign
	// directory under such a row's own occurrence asks for a subject it never
	// enumerated.
	//
	// It is declared only for that case, and only beside a read whose relation
	// is a corresponded foreign directory. The identity is owner-issued because
	// no dense coordinate carries it - the same reason an activation's branch
	// vocabulary is declared rather than derived.
	AddressIdentity member.ProjectionRef
	// Selection names the owner-issued operation that publishes the rows this
	// read returns.
	//
	// A read whose rows are PRODUCED rather than enumerated has no coordinate
	// to be paired against until the reads before it have delivered their
	// cells, so what it reads is an operation's output and not a column
	// vector. The operation publishes into Relation and stamps each row with
	// Predicate, which is why the two are declared together: a read that names
	// a tag names the operation that stamps it, and a read that consumes an
	// earlier result names the operation that consumed it.
	Selection member.SelectionRef
	Read      ReadDecl
}

func (join JoinDecl) Available() bool {
	if len(join.Sources) == 0 || !join.Relation.Available() || !join.Key.Available() || !join.Read.Available() {
		return false
	}
	if join.Selection.Declared() && !join.Selection.Available() {
		return false
	}
	// A selection belongs to a produced read. The converse - that every
	// produced read names one - is the same law from the other side and is
	// stated here once every declaration names its operation.
	return !join.Selection.Available() || join.Produced()
}

// Produced reports whether this read's rows are published by an operation
// rather than enumerated from a declared directory. A read that correlates its
// rows by a tag was selected, and a read computed from an earlier result was
// selected by the values that result delivered; either way there is no
// coordinate to pair against until the operation has run.
func (join JoinDecl) Produced() bool {
	if join.Predicate.Declared() {
		return true
	}
	for _, source := range join.Sources {
		if !source.Candidate {
			return true
		}
	}
	return false
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
	if join.AddressIdentity.Declared() {
		references = append(references, join.AddressIdentity.EntryReference())
	}
	if join.Selection.Declared() {
		references = append(references, join.Selection.EntryReference())
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
	memberAddressed := join.Parent.Declared() || join.KeyVector.Declared()
	if RequiresFactorDenominator(join.Read.Form, join.Read.Contract.Sparse, memberAddressed) && !join.Read.Contract.DenominatorRef.Available() {
		return false
	}
	if !RequiresFactorDenominator(join.Read.Form, join.Read.Contract.Sparse, memberAddressed) && join.Read.Contract.DenominatorRef.Declared() && !join.Read.Contract.DenominatorRef.Available() {
		return false
	}

	return ReadFormAddressing(join.Read.Form, join.Predicate.Declared(), join.Parent.Declared(), join.KeyVector.Declared())
}

// ReadFormAddressing is the one statement of which addressing facts each read
// form's normal form requires. It is a function of the form and of what the
// declaration declares, so a downstream surface that has to know whether a
// sealed read is admissible asks this rather than spelling the five cases
// again.
//
//   - S1 is the exact keyed lookup. If a key consumes an earlier result, the
//     same declaration is S4's computed-coordinate normal form.
//   - S2 is a dependent keyed relation read; when Sources includes an earlier
//     result it is the S4 variant. The predicate is the owner's tag/role
//     metadata and is optional: a nested member set is already addressed by
//     (parent, ordinal), and a routed member already carries its paired tag
//     beside its destination, so a selection with nothing to tag declares
//     nothing.
//   - S3 is the selected relation summary. Predicate is the sealed
//     selection/tag projection that correlates each returned cell with its
//     row, and is required for every relation except one: a self-provided
//     nested member set is already addressed by (parent, ordinal) - its
//     ordinal position IS the correlation - so a Predicate declared over it
//     would be a second, duplicate tagging authority for the same row, the
//     same defect Selected's own untagged form exists to avoid. The parent
//     restatement states that fact; it never infers nestedness from the
//     join's own shape, and seal/plan authenticate the restatement against
//     the resolved relation.
//   - S5 is the closed-denominator whole-vector read.
func ReadFormAddressing(form ReadForm, predicateDeclared, parentDeclared, keyVectorDeclared bool) bool {
	switch form {
	case Exact, Complete:
		return !predicateDeclared
	case Selected:
		return true
	case Summary:
		// A whole-vector read is addressed exactly one way. The three are a
		// tagged selection, a nested member set, and a span the candidate row
		// publishes; a read declaring two states two denominators for one
		// vector, and a read declaring none states no width at all.
		declared := 0
		for _, addressing := range []bool{predicateDeclared, parentDeclared, keyVectorDeclared} {
			if addressing {
				declared++
			}
		}
		return declared == 1
	default:
		return false
	}
}

func cloneJoin(join JoinDecl) JoinDecl {
	join.Sources = append([]SourceRef(nil), join.Sources...)
	return join
}

// ReadFormCandidateAddressed states which read forms are INDEXED BY the rule
// candidate's ordinal, and therefore name the directory that ordinal has to
// be resolved in.
//
// An exact keyed lookup and a whole-vector read both take one row of a
// directory and read from it: the exact read projects that row, the vector
// read enumerates the member set hanging off it. A selected read takes none -
// its coordinates are the members of a relation that exists only per
// invocation, resolved by the reading family - and a closed-denominator read
// spans the whole denominator rather than one candidate's rows. Neither is
// addressed by the candidate at all, so neither has a directory to name.
//
// It is a statement about the form, kept beside ReadFormAddressing because
// both are answers to "what does this form's normal form require", and asked
// by the plan compiler and the sealing engine rather than spelled again in
// either.
func ReadFormCandidateAddressed(form ReadForm) bool {
	return form == Exact || form == Summary
}
