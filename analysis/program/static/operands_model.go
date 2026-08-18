package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// ClaimTarget is the optional authored type operand of one Flow ValueClaim.
// Its sparse denominator is the nonzero target relation, not ValueClaim
// cardinality: a postfix non-nil claim has no static type operand.
type ClaimTarget struct {
	Claim  keyspace.Term
	Target keyspace.Term
}

// TypeValueTarget is the authored runtime-loadable target of one Flow
// TypeValue. The relation is dense by the TypeValue canonical ordinal.
type TypeValueTarget struct{ Target keyspace.Term }

// Annotation is authored metadata attached to one static type occurrence or
// field. Scope and Values are cross-owner references; neither is a concrete
// static type child.
type Annotation struct {
	Scope  keyspace.Term
	Target keyspace.Term
	Name   keyspace.Key
	Values keyspace.Term
}

// OperandsInput contains the three exact Static operand sidecars. Claim is
// sparse; TypeValue and Annotation are dense by their canonical families.
type OperandsInput struct {
	Claim      []ClaimTarget
	TypeValue  []TypeValueTarget
	Annotation []Annotation
}

type operandsStore struct {
	// claims is the sparse ClaimTarget semantic relation in canonical Flow
	// ValueClaim order. claimTargets is its dense ordinal lookup derivative:
	// zero means the sparse relation has no row for that Flow claim.
	claims       []claimTargetRow
	claimTargets []keyspace.Term
	typeValues   []keyspace.Term
	annotations  []Annotation

	// annotationTargets/ranges/terms form a query-only CSR. They are derived
	// from complete annotation rows and never constitute another semantic
	// authority or future artifact denominator.
	annotationTargets []keyspace.Term
	annotationRanges  []poolRange
	annotationTerms   []keyspace.Term
}

type claimTargetRow struct {
	claim  keyspace.Term
	target keyspace.Term
}

// Operands partitions the three typed operand relations without a generic
// sidecar or operand IR.
type Operands struct {
	component *Component
	state     *draftState
}
type ClaimTargets struct {
	component *Component
	state     *draftState
}
type TypeValueTargets struct {
	component *Component
	state     *draftState
}
type Annotations struct {
	component *Component
	state     *draftState
}

// annotationTargetPresent binds the authored Annotation target role to the
// sealed census column, so the query boundary admits exactly the targets Build
// admitted rather than recounting a store to decide the same question.
func annotationTargetPresent(component *Component, target keyspace.Term) bool {
	return component != nil && staticrole.AnnotationTarget(component.census, target)
}
