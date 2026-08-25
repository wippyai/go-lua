package plan

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// RelationRef carries a stable model identity; it is not a registry address.
type RelationRef struct{ id model.RelationID }

func NewRelationRef(id model.RelationID) (RelationRef, bool) {
	if !id.Available() {
		return RelationRef{}, false
	}
	return RelationRef{id: id}, true
}

func (ref RelationRef) Available() bool      { return ref.id.Available() }
func (ref RelationRef) ID() model.RelationID { return ref.id }

// ExpressionRef is an immutable expression-registry entry. The nominal
// expression ID is the logical key; Digest is structural evidence from the
// sealed algebra node retained for checking and artifact identity.
type ExpressionRef struct {
	id         model.ExpressionID
	expression algebra.Expression
	digest     identity.ContentID
}

func DefineExpressionRef(id model.ExpressionID, expression algebra.Expression) ExpressionRef {
	var digest identity.ContentID
	if expression != nil {
		digest = expression.Digest()
	}
	return ExpressionRef{id: id, expression: expression, digest: digest}
}

func (ref ExpressionRef) Available() bool                { return ref.id.Available() && ref.digest.Available() }
func (ref ExpressionRef) ID() model.ExpressionID         { return ref.id }
func (ref ExpressionRef) Expression() algebra.Expression { return ref.expression }
func (ref ExpressionRef) Digest() identity.ContentID     { return ref.digest }
