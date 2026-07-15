package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/evaluated"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	engineobservation "github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// ObserverCallInstance is one transient, correlated call application. Values
// and Paths use Target.Shape's packed boundary order; an empty path at a slot
// means that exact value has no addressable caller path.
type ObserverCallInstance struct {
	Worlds     evaluated.WorldSet
	Owner      lexicalidentity.StableLexicalBodyID
	Occurrence engineobservation.Occurrence
	Point      cfg.Point
	Target     DirectCallTarget
	Values     []product.Value
	Paths      []pathdom.Path
}

// ObserverCallProjection owns one shared finite-world proof and all correlated
// call applications specialized under the same entry transaction.
type ObserverCallProjection struct {
	proof        evaluated.WorldProof
	items        []ObserverCallInstance
	applications uint64
}

func (p ObserverCallProjection) Proof() evaluated.WorldProof { return cloneObserverWorldProof(p.proof) }
func (p ObserverCallProjection) Items() []ObserverCallInstance {
	out := make([]ObserverCallInstance, len(p.items))
	for i, item := range p.items {
		out[i] = item
		out[i].Values = append([]product.Value(nil), item.Values...)
		out[i].Paths = cloneObserverPaths(item.Paths)
	}
	return out
}
func (p ObserverCallProjection) TermApplications() uint64 { return p.applications }

// SpecializeObserverCalls evaluates owner-local call templates as one atomic
// transaction. Guard uncertainty remains a WorldSet in the shared ROBDD;
// DecisionFalse calls are unreachable and omitted. Any failure or cancellation
// returns the zero projection.
func (r Relation) SpecializeObserverCalls(ctx context.Context, cursor BindingCursor, specialization SpecializationContext) (ObserverCallProjection, error) {
	if ctx == nil {
		return ObserverCallProjection{}, fmt.Errorf("transformer: observer calls require a context")
	}
	if err := ctx.Err(); err != nil {
		return ObserverCallProjection{}, err
	}
	if r.arena == nil || r.contextual != "" || cursor.shape != r.shape {
		return ObserverCallProjection{}, fmt.Errorf("transformer: observer call relation or entry shape is invalid")
	}
	if !emptySpecializationContext(specialization) {
		return ObserverCallProjection{}, fmt.Errorf("transformer: observer call slice requires callback-free terms")
	}
	for _, template := range r.annotations.calls {
		if !template.valid(r.arena, r.shape) {
			return ObserverCallProjection{}, fmt.Errorf("transformer: malformed observer call template")
		}
	}
	evaluator, err := newEvaluatedTermEvaluator(ctx, r.arena, cursor)
	if err != nil {
		return ObserverCallProjection{}, err
	}
	guards := make([]Guard, 0, len(r.annotations.calls))
	seen := make(map[Guard]struct{}, len(r.annotations.calls))
	for _, template := range r.annotations.calls {
		if _, exists := seen[template.guard]; !exists {
			seen[template.guard] = struct{}{}
			guards = append(guards, template.guard)
		}
	}
	sortGuardsCanonical(r.arena, guards)
	proof, worlds, err := r.evaluateGuardWorldProofFor(ctx, evaluator, guards)
	if err != nil {
		return ObserverCallProjection{}, err
	}
	out := ObserverCallProjection{proof: proof}
	for index, template := range r.annotations.calls {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return ObserverCallProjection{}, err
			}
		}
		out.applications++
		world := worlds[template.guard]
		if world.Root == evaluated.DecisionFalse {
			continue
		}
		item := ObserverCallInstance{
			Worlds: world, Owner: template.owner, Occurrence: template.occurrence, Point: template.point, Target: template.target,
			Values: make([]product.Value, len(template.values)), Paths: make([]pathdom.Path, len(template.paths)),
		}
		for slot, term := range template.values {
			out.applications++
			value, err := evaluator.value(term)
			if err != nil {
				return ObserverCallProjection{}, err
			}
			item.Values[slot] = value
			if template.paths[slot] != 0 {
				out.applications++
				path, ok := r.arena.specializeOptionalObserverPath(template.paths[slot], cursor)
				if !ok {
					return ObserverCallProjection{}, fmt.Errorf("transformer: observer call path %d cannot be specialized", slot)
				}
				item.Paths[slot] = path.Clone()
			}
		}
		out.items = append(out.items, item)
	}
	if err := ctx.Err(); err != nil {
		return ObserverCallProjection{}, err
	}
	return out, nil
}

func (a *Arena) specializeOptionalObserverPath(term PathTerm, cursor BindingCursor) (pathdom.Path, bool) {
	if a == nil || term == 0 || int(term) >= len(a.paths) {
		return pathdom.Path{}, false
	}
	node := a.paths[term]
	if !cursor.shape.validate(node.root) {
		return pathdom.Path{}, false
	}
	// Paths are optional BindingCursor metadata. A nil path vector certifies
	// that this specialization has no addressable caller roots; preserve the
	// value binding and publish an empty optional path rather than rejecting an
	// otherwise exact summary application.
	if cursor.paths == nil {
		return pathdom.Path{}, true
	}
	root, ok := cursor.Path(node.root)
	if !ok {
		return pathdom.Path{}, false
	}
	if root.IsEmpty() {
		return pathdom.Path{}, true
	}
	out := root.Clone()
	out.Segments = append(out.Segments, node.segments...)
	return out, true
}

func cloneObserverPaths(in []pathdom.Path) []pathdom.Path {
	out := make([]pathdom.Path, len(in))
	for i := range in {
		out[i] = in[i].Clone()
	}
	return out
}

func cloneObserverWorldProof(in evaluated.WorldProof) evaluated.WorldProof {
	out := evaluated.WorldProof{
		Expressions: make([]evaluated.Expression, len(in.Expressions)),
		Predicates:  append([]evaluated.Predicate(nil), in.Predicates...), Decisions: append([]evaluated.Decision(nil), in.Decisions...),
	}
	for i, expression := range in.Expressions {
		out.Expressions[i] = expression
		out.Expressions[i].Args = append([]evaluated.ExpressionID(nil), expression.Args...)
	}
	return out
}
