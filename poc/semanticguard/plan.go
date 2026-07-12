// Package semanticguard is an isolated proof that branch correlation and
// call-boundary path rebasing can be represented without capturing a caller
// State or resolver. It is not imported by production analysis code.
package semanticguard

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

// Relation is the immutable, caller-independent part of a branch relation.
// Paths retain boundary roots ($N and ret[N]) until Instantiate.
type Relation struct {
	kind          factflow.BranchPathRelationKind
	left          pathdom.Path
	right         pathdom.Path
	activeOnTrue  bool
	activeOnFalse bool
}

// Plan is one correlated set of branch facts. Instantiation is transactional:
// either every relation selected by an edge is rebound, or none are returned.
type Plan struct {
	relations []Relation
}

// BoundRelation is a relation whose two paths belong to the caller namespace.
// It is deliberately not executable against State: the production relation
// interpreter also needs point-local visibility, keyspace and type caches.
type BoundRelation struct {
	kind  factflow.BranchPathRelationKind
	left  pathdom.Path
	right pathdom.Path
}

func (r BoundRelation) Kind() factflow.BranchPathRelationKind { return r.kind }
func (r BoundRelation) LeftPath() pathdom.Path                { return r.left.Clone() }
func (r BoundRelation) RightPath() pathdom.Path               { return r.right.Clone() }

// Compile copies the complete relation set and rejects unknown operations.
// Rejecting at this boundary makes future relation kinds contextual by default.
func Compile(relations []factflow.BranchPathRelation) (Plan, error) {
	out := make([]Relation, len(relations))
	for i, relation := range relations {
		if !supportedKind(relation.Kind()) {
			return Plan{}, fmt.Errorf("semanticguard: relation %d has unsupported kind %d", i, relation.Kind())
		}
		out[i] = Relation{
			kind:          relation.Kind(),
			left:          relation.LeftPath(),
			right:         relation.RightPath(),
			activeOnTrue:  relation.ActiveOnEdge(true),
			activeOnFalse: relation.ActiveOnEdge(false),
		}
	}
	return Plan{relations: out}, nil
}

func supportedKind(kind factflow.BranchPathRelationKind) bool {
	switch kind {
	case factflow.BranchPathRelationEqual,
		factflow.BranchPathRelationNotEqual,
		factflow.BranchPathRelationTypeMatch,
		factflow.BranchPathRelationTypeUnmatch:
		return true
	default:
		return false
	}
}

// Instantiate selects one branch edge and rebinds all of its active relations.
// A missing argument/return binding aborts the whole correlated row. Inactive
// relations are not rebound and therefore cannot spuriously require bindings.
func (p Plan) Instantiate(edgeValue bool, bindings callboundary.PathBindings) ([]BoundRelation, bool) {
	count := 0
	for _, relation := range p.relations {
		if relation.active(edgeValue) {
			count++
		}
	}
	bound := make([]BoundRelation, 0, count)
	for _, relation := range p.relations {
		if !relation.active(edgeValue) {
			continue
		}
		left, ok := bindings.Substitute(relation.left)
		if !ok {
			return nil, false
		}
		right, ok := bindings.Substitute(relation.right)
		if !ok {
			return nil, false
		}
		bound = append(bound, BoundRelation{kind: relation.kind, left: left, right: right})
	}
	return bound, true
}

func (r Relation) active(edgeValue bool) bool {
	if edgeValue {
		return r.activeOnTrue
	}
	return r.activeOnFalse
}

// Executable is false until the shared semantic operation plan owns the exact
// point-local State interpreter. Structural completeness alone must not allow
// publication of analyzer results.
func (Plan) Executable() bool { return false }
