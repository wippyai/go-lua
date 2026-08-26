package relcompile

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Declaration is the small, resolved input accepted by the first relational
// lowering pass.  The owning declaration surfaces resolve their names to
// nominal relation/column/key identities before handing this value to the
// compiler; relcompile does not keep a second name registry or a physical
// address table.
//
// Relations, columns, keys, scopes, and semantic signatures are copied into
// the resulting plan. Rules contain only relational facts: a base relation,
// ordered equijoins, an optional scope, optional completion, one typed Apply,
// and one publication. Parent, predicate, correspondence, route, activation
// and transport are all represented by the same JoinSpec/column vectors.
type Declaration struct {
	SchemaID   model.SchemaID
	Relations  []model.RelationSchema
	Columns    []model.ColumnSchema
	Keys       []model.KeySchema
	Scopes     []model.ScopeSchema
	Signatures []signature.Signature
	Rules      []Rule
}

// Rule is one declaration-to-expression specimen. The compiler does not
// interpret a domain role or choose a runtime form; it lowers these generic
// relational facts into the closed algebra.
type Rule struct {
	ID         model.DependencyID
	Expression model.ExpressionID
	Candidate  model.RelationID
	Joins      []JoinSpec
	Scope      model.ScopeID
	Complete   *model.DenominatorRef
	// Operand is the typed row the semantic operation reads. A join yields a
	// row of two relations' columns and no relation of its own, so the row an
	// operation is applied to is projected onto the relation its signature
	// names before it is applied. Absent when the rule applies no operation.
	Operand *Operand
	Apply   signature.Identity
	Carry   *CarrySpec
	Publish *Publication
}

// CarrySpec is the alternative derivation a rule publishes for the rows its
// semantic operation did not produce: the destination relation observed at the
// carried input port, optionally transformed by one further typed operation.
// It is combined with the operation's own rows by the destination key's
// declared algebra, which is Merge and never a form of its own.
type CarrySpec struct {
	Relation  model.RelationID
	Scope     model.ScopeID
	Transform *signature.Identity
}

// JoinSpec is one oriented equijoin. The vectors are typed by the relation
// and column registries in Declaration. A parent relation, predicate/tag
// relation, correspondence, route source, activation branch, or transport
// axis is not a distinct engine mechanism: it is just a declared relation and
// a matching column vector.
type JoinSpec struct {
	Relation     model.RelationID
	LeftColumns  []model.ColumnID
	RightColumns []model.ColumnID
	// Scope is the decision scope the joined rows are observed at. It is the
	// relational statement of the input port a read was declared on: two reads
	// naming one port observe one scope.
	Scope model.ScopeID
	// Complete is the authenticated denominator this join closes over. It is
	// present exactly when the authored read materializes an absent coordinate
	// through a denominator, and absent when the read stays sparse.
	Complete *model.DenominatorRef
}

// Operand is the projection from a rule's joined row onto the relation its
// semantic operation reads. Every column of that relation is defined exactly
// once, by the read that produced it.
type Operand struct {
	Relation model.RelationID
	Key      model.KeyID
	Columns  []ColumnMapping
}

// ColumnMapping carries one column of the joined row into the operand row.
type ColumnMapping struct {
	Source model.ColumnID
	Target model.ColumnID
}

// Publication names the sole logical write destination. The engine later
// decides how to arrange or commit it; relcompile emits only Publish.
type Publication struct {
	Relation model.RelationID
	Key      model.KeyID
}
