package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalPathStoreOwnerIdentityPlan is the frozen owner certificate for one
// descendant PathStore static write.  Its support comes exclusively from the
// solved identity environment entering the occurrence; execution may only
// restrict that support by guards already demanded for the same occurrence.
type formalPathStoreOwnerIdentityPlan struct {
	owner    relationVar
	arena    *Arena
	source   ValueTerm
	supports map[ValueTerm]formalIdentitySupport
	suffix   []segment.Segment
	plans    map[identity.Term]state.HeapStaticMemberWritePlan
	sealed   bool
}

func (p formalPathStoreOwnerIdentityPlan) validFor(owner relationVar, arena *Arena) bool {
	if !p.sealed || p.owner != owner || p.arena != arena || arena == nil || !arena.Sealed() || p.source == 0 ||
		int(p.source) >= len(arena.values) || len(p.suffix) == 0 || len(p.supports) == 0 {
		return false
	}
	for term, support := range p.supports {
		if term == 0 || int(term) >= len(arena.values) || support == nil {
			return false
		}
	}
	for term, plan := range p.plans {
		if !term.Valid() || !plan.Valid() {
			return false
		}
	}
	return true
}

func formalPathStoreOwnerSource(arena *Arena, target PathTerm) (ValueTerm, []segment.Segment, error) {
	if arena == nil || target == 0 || int(target) >= len(arena.paths) {
		return 0, nil, fmt.Errorf("PathStore owner has foreign target syntax")
	}
	path := arena.paths[target]
	if len(path.segments) == 0 {
		return 0, nil, nil
	}
	if path.environment != 0 {
		source, ok := arena.environmentValue(path.environment)
		if !ok {
			return 0, nil, fmt.Errorf("PathStore owner environment is undeclared")
		}
		return source, append([]segment.Segment(nil), path.segments...), nil
	}
	source := arena.Root(path.root)
	if source == 0 {
		return 0, nil, fmt.Errorf("PathStore owner root is undeclared")
	}
	return source, append([]segment.Segment(nil), path.segments...), nil
}

func (p formalPathStoreOwnerIdentityPlan) sourceSupport(
	evaluator formalTupleLeafEvaluator,
	term ValueTerm,
	visiting map[ValueTerm]bool,
) (formalIdentitySupport, error) {
	if !p.validFor(evaluator.variable, evaluator.authority.terms) || term == 0 || int(term) >= len(p.arena.values) {
		return nil, errFormalComponentForeignOwner
	}
	if visiting[term] {
		return nil, nil
	}
	visiting[term] = true
	defer delete(visiting, term)
	node := p.arena.values[term]
	if node.op != valueSelect {
		support, ok := p.supports[term]
		if !ok {
			return nil, fmt.Errorf("transformer: PathStore owner identity support is incomplete")
		}
		return append(formalIdentitySupport(nil), support...), nil
	}
	if len(node.args) != 2 || node.guard == 0 {
		return nil, fmt.Errorf("transformer: PathStore owner identity Select is malformed")
	}
	canTrue, canFalse, exact := evaluator.exactGuardPossibilities(p.owner, p.arena, 0, node.guard)
	if !exact || !canTrue && !canFalse {
		return nil, fmt.Errorf("transformer: PathStore owner identity Select has no executable guard")
	}
	var supports []formalIdentitySupport
	if canTrue {
		support, err := p.sourceSupport(evaluator, node.args[0], visiting)
		if err != nil {
			return nil, err
		}
		supports = append(supports, support)
	}
	if canFalse {
		support, err := p.sourceSupport(evaluator, node.args[1], visiting)
		if err != nil {
			return nil, err
		}
		supports = append(supports, support)
	}
	return unionFormalIdentitySupport(supports...), nil
}

// bind returns a member transaction only for an exact owner.  In particular,
// a non-singleton owner support never becomes one write per candidate and
// never emits a Bottom member.
func (p formalPathStoreOwnerIdentityPlan) bind(
	evaluator formalTupleLeafEvaluator,
	parent product.Value,
	value product.Value,
) (state.HeapStaticMemberWritePlan, bool, error) {
	support, err := p.sourceSupport(evaluator, p.source, make(map[ValueTerm]bool))
	if err != nil {
		return state.HeapStaticMemberWritePlan{}, false, err
	}
	if len(support) != 1 {
		return state.HeapStaticMemberWritePlan{}, false, nil
	}
	term := support[0]
	if actual, exact := product.Get(evaluator.authority.product.Registry(), parent, identity.Key).Term(); exact && actual.Valid() && actual != term {
		return state.HeapStaticMemberWritePlan{}, false, fmt.Errorf("transformer: PathStore owner exact identity disagrees with frozen support")
	}
	plan, ok := p.plans[term]
	if !ok {
		return state.HeapStaticMemberWritePlan{}, false, fmt.Errorf("transformer: PathStore owner has no registered heap member transaction")
	}
	bound, err := evaluator.authority.product.BindHeapStaticMemberWriteValue(plan, value)
	if err != nil {
		return state.HeapStaticMemberWritePlan{}, false, err
	}
	return bound, true, nil
}

// pathStoreOwnerSupport resolves a pre-mutation source through the registered
// input identity images of every call occurrence targeting this body.  A body
// input's raw support is its formal variable; the image is the already-solved
// concrete/allocation support that owns the heap coordinate in that occurrence.
func (c *formalCoordinateDependencyClosure) pathStoreOwnerSupport(bodyIndex int, environment formalIdentityEnvironment, source ValueTerm) (formalIdentitySupport, error) {
	support, err := c.identityValueSupport(bodyIndex, environment, source, make(map[ValueTerm]bool))
	if err != nil || len(support) == 0 {
		return support, err
	}
	var resolved []formalIdentitySupport
	for _, term := range support {
		if _, formalTerm := term.Formal(); !formalTerm {
			resolved = append(resolved, formalIdentitySupport{term})
			continue
		}
		var images []formalIdentitySupport
		for frameIndex := range c.frames {
			frame := &c.frames[frameIndex]
			if frame.target != bodyIndex || frame.identityImage == nil {
				continue
			}
			if image, ok := frame.identityImage.Image(term); ok && len(image) != 0 {
				images = append(images, formalIdentitySupport(image))
			}
		}
		if len(images) == 0 {
			resolved = append(resolved, formalIdentitySupport{term})
		} else {
			resolved = append(resolved, images...)
		}
	}
	return unionFormalIdentitySupport(resolved...), nil
}

func (c *formalCoordinateDependencyClosure) freezePathStoreOwnerIdentityPlans() (map[formalRelationCell]formalPathStoreOwnerIdentityPlan, error) {
	plans := make(map[formalRelationCell]formalPathStoreOwnerIdentityPlan)
	for cellIndex, cell := range c.cells {
		if cell.Kind != formalRelationCellStep {
			continue
		}
		bodyIndex := int(cell.Variable - 1)
		body := &c.program.bodies[bodyIndex]
		step := body.relation.code.nodes[cell.Root].steps[cell.Step-1]
		if step.kind != boundaryStepEffect || step.effect == 0 {
			continue
		}
		effect := body.relation.effects.nodes[step.effect]
		if effect.kind != EffectPathStore || !effect.pathStoreHasStatic {
			continue
		}
		source, suffix, err := formalPathStoreOwnerSource(body.relation.arena, effect.pathStoreStatic.Target)
		if err != nil {
			return nil, err
		}
		if source == 0 {
			continue
		}
		supports := make(map[ValueTerm]formalIdentitySupport)
		seen := make(map[ValueTerm]bool)
		var collect func(ValueTerm) error
		collect = func(term ValueTerm) error {
			if term == 0 || int(term) >= len(body.relation.arena.values) || seen[term] {
				return nil
			}
			seen[term] = true
			var support formalIdentitySupport
			var supportErr error
			if term == source {
				support, supportErr = c.pathStoreOwnerSupport(bodyIndex, c.cellInputIdentity[cellIndex], term)
			} else {
				support, supportErr = c.identityValueSupport(bodyIndex, c.cellInputIdentity[cellIndex], term, make(map[ValueTerm]bool))
			}
			if supportErr != nil {
				return supportErr
			}
			supports[term] = append(formalIdentitySupport(nil), support...)
			for _, child := range body.relation.arena.values[term].args {
				if err := collect(child); err != nil {
					return err
				}
			}
			return nil
		}
		if err := collect(source); err != nil {
			return nil, err
		}
		memberPlans := make(map[identity.Term]state.HeapStaticMemberWritePlan)
		for _, term := range supports[source] {
			plan, planErr := body.productDomain.PrepareHeapStaticMemberWritePlan(c.keys[bodyIndex], term, suffix, product.Bottom(body.productDomain.Registry()))
			if planErr != nil {
				return nil, planErr
			}
			memberPlans[term] = plan
		}
		plans[cell] = formalPathStoreOwnerIdentityPlan{
			owner: cell.Variable, arena: body.relation.arena, source: source, supports: supports,
			suffix: suffix, plans: memberPlans, sealed: true,
		}
	}
	return plans, nil
}
