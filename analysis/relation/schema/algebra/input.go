package algebra

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Input names one sealed logical relation. It contains only the stable
// relation reference; physical scans, lookups, and arrangements are mount
// concerns.
type Input struct {
	relation model.RelationID
}

// NewInput constructs an input expression. Relation availability and
// declaration membership are checker responsibilities.
func NewInput(relation model.RelationID) Input { return Input{relation: relation} }

// Relation returns the stable relation reference.
func (input Input) Relation() model.RelationID { return input.relation }

// Kind implements Expression.
func (input Input) Kind() Kind { return KindInput }

// Digest returns the deterministic structural identity.
func (input Input) Digest() identity.ContentID {
	return derive("analysis/relation/schema/algebra/input/v1", appendRelation(nil, input.relation))
}

func (input Input) expression() {}
