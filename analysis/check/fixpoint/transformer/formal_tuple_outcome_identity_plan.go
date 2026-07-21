package transformer

import (
	"fmt"
	"os"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// formalOutcomeSourceIdentityPlan is the identity-support certificate for one
// N5 source ordinal.  supports is intentionally keyed by source syntax rather
// than coordinate inventory: Select traversal below preserves its exact leaf
// guard while every irreducible source term retains the already-solved support.
type formalOutcomeSourceIdentityPlan struct {
	owner    relationVar
	arena    *Arena
	source   ValueTerm
	supports map[ValueTerm]formalIdentitySupport
	sealed   bool
}

func (p formalOutcomeSourceIdentityPlan) validFor(owner relationVar, arena *Arena, source ValueTerm) bool {
	if !p.sealed || p.owner != owner || p.arena != arena || p.source != source || arena == nil || !arena.Sealed() ||
		source == 0 || int(source) >= len(arena.values) || len(p.supports) == 0 {
		return false
	}
	for term := range p.supports {
		if term == 0 || int(term) >= len(arena.values) {
			return false
		}
	}
	return true
}

// sourceSupport returns the source's feasible identity set in this exact
// executable leaf.  Only Select is split here: its condition is already a
// demanded guard in the Outcome lift, so a leaf cannot collapse its two arms.
// Other forwarding/join nodes retain the solved support for their own syntax;
// the closure owns dynamic-read and frame-result support construction.
func (p formalOutcomeSourceIdentityPlan) sourceSupport(
	evaluator formalTupleLeafEvaluator,
	term ValueTerm,
	visiting map[ValueTerm]bool,
) (formalIdentitySupport, error) {
	if !p.validFor(evaluator.variable, evaluator.authority.terms, p.source) || term == 0 || int(term) >= len(p.arena.values) {
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
			return nil, fmt.Errorf("transformer: Outcome source identity support is incomplete")
		}
		return append(formalIdentitySupport(nil), support...), nil
	}
	if len(node.args) != 2 || node.guard == 0 {
		return nil, fmt.Errorf("transformer: Outcome source identity Select is malformed")
	}
	canTrue, canFalse, exact := evaluator.exactGuardPossibilities(p.owner, p.arena, 0, node.guard)
	if !exact || !canTrue && !canFalse {
		return nil, fmt.Errorf("transformer: Outcome source identity Select has no executable guard")
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

// apply annotates an evaluated source only when the frozen plan proves one
// non-formal identity in this leaf. Allocation templates are the identity of
// an object-literal value actually assigned to the whole return root and must
// be retained. Formal input roots deliberately do not synthesize an identity:
// an exact caller value is retained, while an inexact caller argument remains
// inexact. Coordinate admission never participates.
func (p formalOutcomeSourceIdentityPlan) apply(
	evaluator formalTupleLeafEvaluator,
	value product.Value,
) (product.Value, error) {
	support, err := p.sourceSupport(evaluator, p.source, make(map[ValueTerm]bool))
	if err != nil {
		return product.Value{}, err
	}
	if os.Getenv("GOLUA_TRACE_IDENTITY_SUPPORT") != "" {
		fmt.Fprintf(os.Stderr, "IDENTITY_OUTCOME_SOURCE owner=%d source=%s support=%v exact=%t\n",
			p.owner, p.arena.canonicalValue(p.source), support, len(support) == 1)
	}
	if len(support) != 1 {
		return value, nil
	}
	term := support[0]
	if _, formal := term.Formal(); formal {
		return value, nil
	}
	if allocation, allocated := term.Allocation(); allocated {
		if evaluator.body == nil || evaluator.body.rootAllocations == nil {
			return product.Value{}, fmt.Errorf("transformer: Outcome allocation identity has no root boundary authority")
		}
		actual, exact := evaluator.body.rootAllocations.RebaseAllocation(allocation)
		if !exact {
			return product.Value{}, fmt.Errorf("transformer: Outcome allocation identity is outside root boundary authority")
		}
		term = identity.ConcreteTerm(actual)
		if os.Getenv("GOLUA_TRACE_IDENTITY_SUPPORT") != "" {
			fmt.Fprintf(os.Stderr, "IDENTITY_OUTCOME_REBASE owner=%d identity=%v valid=%t\n", p.owner, actual, term.Valid())
		}
	}
	current, exact := product.Get(evaluator.authority.product.Registry(), value, identity.Key).Term()
	if exact && current.Valid() {
		if current != term {
			// A concrete source has already crossed a boundary with its exact
			// owner. The static support is an admission certificate, not an
			// authority to replace that concrete identity.
			return value, nil
		}
		return value, nil
	}
	// A Values carrier can be Bottom when all of a returned object's ordinary
	// facts live in registered heap coordinates. Bottom cannot carry an axis,
	// so create only the otherwise-unknown whole value before restoring the
	// exact root identity; member facts remain owned by those coordinates.
	base := value
	if product.PresenceOf(base).IsBottom() {
		base = product.Top()
	}
	annotated := product.Set(evaluator.authority.product.Registry(), base, identity.Key, identity.SingletonTerm(term))
	if os.Getenv("GOLUA_TRACE_IDENTITY_SUPPORT") != "" {
		annotatedTerm, exact := product.Get(evaluator.authority.product.Registry(), annotated, identity.Key).Term()
		fmt.Fprintf(os.Stderr, "IDENTITY_OUTCOME_ANNOTATED owner=%d identity=%v exact=%t\n", p.owner, annotatedTerm, exact)
	}
	return annotated, nil
}

func (c *formalCoordinateDependencyClosure) freezeOutcomeSourceIdentityPlans() (map[formalRelationCell][]formalOutcomeSourceIdentityPlan, error) {
	if c == nil || c.program == nil {
		return nil, fmt.Errorf("transformer: Outcome source identity plans are unowned")
	}
	plans := make(map[formalRelationCell][]formalOutcomeSourceIdentityPlan)
	for cellIndex, cell := range c.cells {
		if cell.Kind != formalRelationCellOutcome {
			continue
		}
		bodyIndex := int(cell.Variable - 1)
		body := &c.program.bodies[bodyIndex]
		outcome := body.relation.code.outcomes[cell.Outcome]
		perSource := make([]formalOutcomeSourceIdentityPlan, len(outcome.returnTransaction.sources))
		for sourceIndex, source := range outcome.returnTransaction.sources {
			supports := make(map[ValueTerm]formalIdentitySupport)
			seen := make(map[ValueTerm]bool)
			var collect func(ValueTerm) error
			collect = func(term ValueTerm) error {
				if term == 0 || int(term) >= len(body.relation.arena.values) {
					return fmt.Errorf("transformer: Outcome source identity has foreign syntax")
				}
				if seen[term] {
					return nil
				}
				seen[term] = true
				support, err := c.identityValueSupport(bodyIndex, c.cellIdentity[cellIndex], term, make(map[ValueTerm]bool))
				if err != nil {
					return err
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
				return nil, fmt.Errorf("transformer: Outcome source %d identity plan: %w", sourceIndex, err)
			}
			perSource[sourceIndex] = formalOutcomeSourceIdentityPlan{
				owner: cell.Variable, arena: body.relation.arena, source: source, supports: supports, sealed: true,
			}
		}
		plans[cell] = perSource
	}
	return plans, nil
}
