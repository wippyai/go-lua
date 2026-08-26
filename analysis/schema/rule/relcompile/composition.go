package relcompile

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/relinput"
)

// Composition is the placement authority for one rule catalog.
//
// A decision scope names a relation-schema conjunction, and the pass that
// resolves a rule's reads is the pass that knows which conjunction the rule's
// candidate rows are decided at and which one each declared input port
// observes. That pass is Resolve, so the composition is the table Resolve
// fills as it lowers, addressed by the dense rule ordinal the rule catalog
// numbers its rules with.
//
// The composition publishes what it resolved and derives nothing. Region
// evidence is the identity the scope's own owner issued when it installed the
// scope; the composition references that issuance through the registry and
// mints no identity of its own.
type Composition struct {
	registry *Registry
	placed   map[int]relinput.Placement
}

// Compose opens the placement authority over one identity registry. Every
// scope the composition ever states is a scope that registry resolved, so a
// composition and the rules it placed answer under one issuing owner.
func Compose(registry *Registry) *Composition {
	if registry == nil {
		return nil
	}
	return &Composition{registry: registry, placed: map[int]relinput.Placement{}}
}

// Available reports whether the composition holds the registry its answers
// resolve through.
func (composition *Composition) Available() bool {
	return composition != nil && composition.registry != nil
}

// Resolve lowers one authored rule declaration at the dense rule ordinal the
// catalog numbers it with, publishes the placement it resolved, and answers
// the resolution itself so a caller reads the lowering's own answer rather
// than reading it back through the table this composition publishes.
//
// Lowering and placing are one act: a rule this composition compiled is a
// rule this composition has placed, which is what makes the published table
// total over everything it compiled. A rule whose candidate or declared port
// names a scope no owner installed refuses here, so an unresolvable scope is
// a refusal and never an ordinal quietly left unplaced.
//
// One ordinal is one rule. A second rule offered at an ordinal already placed
// refuses rather than replacing the first.
func (composition *Composition) Resolve(ordinal int, spec rule.Spec, placement Placement) (Resolution, error) {
	if !composition.Available() {
		return Resolution{}, refuse(Site{Path: "composition"}, Name{}, KindOwner, ReasonUnavailable)
	}
	name := EntryName(schema.SurfaceKindRule, spec.Key)
	if ordinal < 0 {
		return Resolution{}, refuse(Site{Rule: spec.Key, Path: "composition.ordinal"}, name, KindDependency, ReasonUnavailable)
	}
	if _, taken := composition.placed[ordinal]; taken {
		return Resolution{}, refuse(Site{Rule: spec.Key, Path: "composition.ordinal"}, name, KindDependency, ReasonDuplicateName)
	}
	resolution, err := Resolve(composition.registry, spec, placement)
	if err != nil {
		return Resolution{}, err
	}
	composition.placed[ordinal] = resolution.Placed
	return resolution, nil
}

// Count is the number of rule ordinals this composition has placed.
func (composition *Composition) Count() int {
	if !composition.Available() {
		return 0
	}
	return len(composition.placed)
}

// Placement states one lowered rule's decision scopes by its dense rule
// ordinal, satisfying the boundary the relation input bundle seals against.
// An ordinal this composition never lowered is unplaced, and the seal refuses
// with it named rather than reading a default.
func (composition *Composition) Placement(ordinal int) (relinput.Placement, bool) {
	if !composition.Available() {
		return relinput.Placement{}, false
	}
	placed, held := composition.placed[ordinal]
	if !held {
		return relinput.Placement{}, false
	}
	return relinput.Placement{Candidate: placed.Candidate, Ports: append([]model.ScopeID(nil), placed.Ports...)}, true
}

// ScopeRegion states the owner-issued region evidence one named scope stands
// on: the identity that scope's own owner issued when it installed it. The
// composition holds no region and issues no identity; it answers with the
// issuance the registry recorded and nothing else.
func (composition *Composition) ScopeRegion(scope model.ScopeID) (identity.ContentID, bool) {
	if !composition.Available() {
		return identity.ContentID{}, false
	}
	return composition.registry.ScopeEvidence(scope)
}
