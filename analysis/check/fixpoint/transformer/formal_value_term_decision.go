package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
)

// compileFormalValueTermDecisions lifts sealed ValueTerm syntax pointwise over
// the tuple's shared decision DAG. Each node correlates only its immediate
// children and registered direct factor dependencies; memoization by ValueTerm
// retains every reconvergent subterm. Product semantics remain exclusively in
// resolveValueNodeProduct.
func (a *formalTupleAlgebra) compileFormalValueTermDecisions(
	tuple formalRelationTuple,
	arena *Arena,
	scope loopMuTerm,
	terms ...ValueTerm,
) ([]decisionRef, error) {
	if a == nil || arena == nil || !arena.Sealed() || tuple.bottom() {
		return nil, errFormalComponentForeignOwner
	}
	if err := a.validateTuple(tuple); err != nil {
		return nil, err
	}
	span, directory, authority, ok := a.span(tuple.variable)
	if !ok || tuple.root.owner != directory || authority.terms != arena {
		return nil, errFormalComponentForeignOwner
	}
	body := &a.program.bodies[tuple.variable-1]
	spanGroups := span.groupDescriptors()
	values := formalValuesFiberGroup{descriptor: authority.body.factors.values}
	if !values.valid() || a.program.formalSlots == nil {
		return nil, errFormalComponentMalformed
	}
	care, err := a.care(tuple)
	if err != nil {
		return nil, err
	}
	bottom := product.Bottom(authority.product.Registry())
	encode := func(value product.Value) (decisionLeaf, error) {
		if !product.BelongsToRegistry(authority.product.Registry(), value) {
			return 0, errFormalComponentForeignOwner
		}
		if product.Equal(authority.product.Registry(), value, bottom) {
			return 0, nil
		}
		return authority.internGroundValue(value)
	}
	decode := func(leaf decisionLeaf) (product.Value, error) {
		if leaf == 0 {
			return bottom, nil
		}
		terminal, terminalErr := authority.terminal(leaf)
		if terminalErr != nil || terminal.kind != formalComponentGroundValue ||
			!product.BelongsToRegistry(authority.product.Registry(), terminal.ground) {
			if terminalErr != nil {
				return product.Value{}, terminalErr
			}
			return product.Value{}, errFormalComponentMalformed
		}
		return terminal.ground, nil
	}
	valueSlotRoot := func(slot FormalSlot) (decisionRef, error) {
		member, present := values.slot(slot)
		top, topPresent := values.top()
		if !present || !topPresent {
			return 0, fmt.Errorf("transformer: formal value decision source is outside Values")
		}
		memberValue, memberErr := directory.valueAt(tuple.root, member.ordinal)
		if memberErr != nil {
			return 0, memberErr
		}
		topValue, topErr := directory.valueAt(tuple.root, top.ordinal)
		if topErr != nil {
			return 0, topErr
		}
		topLeaf, topErr := encode(product.Top())
		if topErr != nil {
			return 0, topErr
		}
		return a.decisions.condition(a.ctx, decisionRef(topValue), a.decisions.terminal(topLeaf), decisionRef(memberValue))
	}
	directGroups := func(term ValueTerm) (state.TransferInputAccess, []formalFiberGroupDescriptor, error) {
		access, accessErr := body.valueTermNodeFactorAccessMode(term, false)
		if accessErr != nil {
			return state.TransferInputAccess{}, nil, accessErr
		}
		groups := make([]formalFiberGroupDescriptor, 0, access.Lanes.Len())
		for _, lane := range body.productDomain.NonValuesLaneInventory() {
			if !access.Lanes.Has(lane.ID()) {
				continue
			}
			found := false
			for _, group := range spanGroups {
				if group.kind != formalFiberGroupValues && group.lane == lane {
					groups = append(groups, group)
					found = true
					break
				}
			}
			if !found {
				return state.TransferInputAccess{}, nil, fmt.Errorf("transformer: formal value decision lane %q is outside formal fibers", lane.ID())
			}
		}
		if len(groups) != access.Lanes.Len() {
			return state.TransferInputAccess{}, nil, errFormalComponentMalformed
		}
		return access, groups, nil
	}

	memo := make(map[ValueTerm]decisionRef)
	visiting := make(map[ValueTerm]bool)
	type observedObjectSource struct {
		observation luasourcevalue.ObjectLiteralSourceObservation
		leaf        decisionLeaf
	}
	observedObjectSources := make(map[uint64][]observedObjectSource)
	quotientObjectSource := func(root decisionRef) (decisionRef, error) {
		return a.decisions.mapLeavesTransient(a.ctx, root, func(leaf decisionLeaf) (decisionLeaf, error) {
			value, valueErr := decode(leaf)
			if valueErr != nil {
				return 0, valueErr
			}
			observation, exact := luasourcevalue.ObserveObjectLiteralSourceCached(authority.product.Registry(), arena.typeValues, value, true)
			if !exact {
				return 0, errFormalComponentMalformed
			}
			fingerprint := observation.Fingerprint()
			for _, prior := range observedObjectSources[fingerprint] {
				if observation.Equal(prior.observation) {
					return prior.leaf, nil
				}
			}
			representative, internErr := encode(value)
			if internErr != nil {
				return 0, internErr
			}
			observedObjectSources[fingerprint] = append(observedObjectSources[fingerprint], observedObjectSource{
				observation: observation.Clone(), leaf: representative,
			})
			return representative, nil
		})
	}
	var compile func(ValueTerm) (decisionRef, error)
	compile = func(term ValueTerm) (decisionRef, error) {
		if root, present := memo[term]; present {
			return root, nil
		}
		if term == 0 || int(term) >= len(arena.values) || visiting[term] {
			return 0, fmt.Errorf("transformer: formal value decision contains a foreign or cyclic term")
		}
		visiting[term] = true
		defer delete(visiting, term)
		node := arena.values[term]
		if node.op == valueRoot {
			slot, exact := a.program.formalSlots.Slot(body.body, node.root)
			if !exact {
				return 0, fmt.Errorf("transformer: formal value decision root has no typed slot")
			}
			root, rootErr := valueSlotRoot(slot)
			if rootErr == nil {
				memo[term] = root
			}
			return root, rootErr
		}
		if node.op == valueEnvironment {
			slot, exact := formalMiddleSlotForStateKey(a.program, body, node.slot)
			if !exact {
				return 0, fmt.Errorf("transformer: formal value decision environment has no Middle slot")
			}
			root, rootErr := valueSlotRoot(slot)
			if rootErr == nil {
				memo[term] = root
			}
			return root, rootErr
		}
		if node.op == valueSelect {
			if len(node.args) != 2 || node.guard == 0 {
				return 0, errFormalComponentMalformed
			}
			high, highErr := compile(node.args[0])
			if highErr != nil {
				return 0, highErr
			}
			low, lowErr := compile(node.args[1])
			if lowErr != nil {
				return 0, lowErr
			}
			guard, guardErr := a.decisionForGuard(tuple.variable, scope, arena, node.guard)
			if guardErr != nil {
				return 0, guardErr
			}
			root, conditionErr := a.decisions.condition(a.ctx, guard, high, low)
			if conditionErr == nil {
				memo[term] = root
			}
			return root, conditionErr
		}

		roots := make([]decisionRef, 0, len(node.args)+1)
		for _, child := range node.args {
			root, childErr := compile(child)
			if childErr != nil {
				return 0, childErr
			}
			if node.op == valueObjectLiteral {
				root, childErr = quotientObjectSource(root)
				if childErr != nil {
					return 0, childErr
				}
			}
			roots = append(roots, root)
		}
		_, groups, accessErr := directGroups(term)
		if accessErr != nil {
			return 0, accessErr
		}
		for _, group := range groups {
			for _, ordinal := range group.members {
				value, valueErr := directory.valueAt(tuple.root, ordinal)
				if valueErr != nil {
					return 0, valueErr
				}
				roots = append(roots, decisionRef(value))
			}
		}
		proofPosition := -1
		if node.integerProof != 0 {
			proof, proofErr := compile(node.integerProof)
			if proofErr != nil {
				return 0, proofErr
			}
			proofPosition = len(roots)
			roots = append(roots, proof)
		}
		resolve := func(leaves []decisionLeaf) (decisionLeaf, error) {
			if len(leaves) != len(roots) {
				return 0, errDecisionMalformed
			}
			args := make([]product.Value, len(node.args))
			for index := range args {
				value, valueErr := decode(leaves[index])
				if valueErr != nil {
					return 0, valueErr
				}
				args[index] = value
			}
			factorOffset := len(node.args)
			factors := make([]state.LaneFactor, len(groups))
			for index, group := range groups {
				end := factorOffset + len(group.members)
				if end > len(leaves) {
					return 0, errDecisionMalformed
				}
				factor, factorErr := a.formalFactorSpelling(authority, group, leaves[factorOffset:end])
				if factorErr != nil {
					return 0, factorErr
				}
				factors[index] = factor
				factorOffset = end
			}
			resolver := valueNodeLeafResolver{
				dynamicRead: func(candidate valueNode, values []product.Value) (product.Value, bool) {
					return resolveFormalDynamicValue(body, span, candidate, values, factors, func(child ValueTerm) (product.Value, bool) {
						if child != node.integerProof || proofPosition < 0 || proofPosition >= len(leaves) {
							return product.Value{}, false
						}
						value, valueErr := decode(leaves[proofPosition])
						return value, valueErr == nil
					})
				},
				allocationResult: func(candidate valueNode) (product.Value, bool) {
					return arena.allocationResult(candidate.allocation, candidate.resultIndex)
				},
			}
			value, exact := resolveValueNodeProduct(authority.product.Registry(), arena.typeValues, node, args, resolver)
			if !exact {
				return 0, fmt.Errorf("transformer: formal value decision cannot resolve term %d (op=%d)", term, node.op)
			}
			return encode(value)
		}
		if len(roots) == 0 {
			leaf, resolveErr := resolve(nil)
			if resolveErr != nil {
				return 0, resolveErr
			}
			root := a.decisions.terminal(leaf)
			memo[term] = root
			return root, nil
		}
		transformed, transformErr := a.decisions.applyNaryUnderCare(a.ctx, care, roots, resolve)
		if transformErr != nil {
			return 0, transformErr
		}
		memo[term] = transformed
		return transformed, nil
	}

	out := make([]decisionRef, len(terms))
	for index, term := range terms {
		out[index], err = compile(term)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func formalGroundValueDecisionLeaf(authority *formalComponentTerminalAuthority, leaf decisionLeaf) (product.Value, error) {
	if authority == nil {
		return product.Value{}, errFormalComponentForeignOwner
	}
	if leaf == 0 {
		return product.Bottom(authority.product.Registry()), nil
	}
	terminal, err := authority.terminal(leaf)
	if err != nil || terminal.kind != formalComponentGroundValue || !product.BelongsToRegistry(authority.product.Registry(), terminal.ground) {
		if err != nil {
			return product.Value{}, err
		}
		return product.Value{}, errFormalComponentMalformed
	}
	return terminal.ground, nil
}
