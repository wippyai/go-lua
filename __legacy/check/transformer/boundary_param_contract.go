package transformer

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	valuerefinement "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// boundaryParamContractMode distinguishes the two semantic entry routes. A
// concrete Apply transports caller values, whereas definition analysis starts
// from the function's jointly instantiated formal tuple.
type boundaryParamContractMode uint8

const (
	boundaryParamContractConcrete boundaryParamContractMode = iota
	boundaryParamContractDefinition
)

// applyBoundaryParamContracts is the single call-entry contract transaction.
// Concrete Apply refines transported actuals and preserves Bottom exactly.
// Definition analysis instead installs the formal tuple authoritatively: it is
// the generic function world being solved, not one caller specialization.
func applyBoundaryParamContracts(reg *axis.Registry, body *relationProgramBody, entry state.State, mode boundaryParamContractMode) state.State {
	if reg == nil || body == nil || body.plan == nil {
		return entry
	}
	params := body.plan.BoundaryParams()
	contracts, _ := instantiateBoundaryContracts(reg, body, entry, nil)
	if len(params) == 0 || len(contracts) != len(params) {
		return entry
	}
	edit := entry.EditValues(reg)
	pathRefinements := make([]product.Value, len(params))
	pathPresent := make([]bool, len(params))
	for index, param := range params {
		slot := key.SymbolValue(param)
		actual := entry.ReadValue(reg, slot)
		value := contracts[index]
		if mode == boundaryParamContractConcrete {
			if product.Equal(reg, actual, product.Bottom(reg)) {
				continue
			}
			value = meetBoundaryParamContract(reg, actual, contracts[index])
		}
		edit.Write(slot, value)
		if index < len(body.roots.roots) {
			root := body.roots.roots[index]
			if root.root == (Root{Kind: RootParam, Index: uint32(index)}) {
				pathRefinement := value
				if mode == boundaryParamContractConcrete {
					pathValue := entry.ReadLocalPathKey(reg, root.path)
					if product.Equal(reg, pathValue, product.Bottom(reg)) {
						continue
					}
					pathRefinement = meetBoundaryParamContract(reg, pathValue, contracts[index])
				}
				pathRefinements[index] = pathRefinement
				pathPresent[index] = true
			}
		}
	}
	out := edit.Done()
	for index, present := range pathPresent {
		if present {
			out = out.WriteLocalPathKey(reg, body.roots.roots[index].path, pathRefinements[index])
		}
	}
	return out
}

func meetBoundaryParamContract(reg *axis.Registry, actual, contract product.Value) product.Value {
	// Gradual-top is provenance for an unannotated formal, not a semantic
	// restriction on a concrete invocation.  Remove only that provenance from
	// the contract before meeting so the remaining registered axes (notably the
	// present-self constraint) still apply, while the caller's exact evidence,
	// type witness, identity and shape survive.  Explicit any/unknown is not
	// erased here: it remains an authored contract with its existing semantics.
	if product.Get(reg, contract, evidence.Key).IsGradualTop() {
		contract = product.Set(reg, contract, evidence.Key, evidence.Top())
	}
	if boundaryArgumentSatisfiesContract(reg, actual, contract) {
		// Assignability proves actual ⊆ contract in the language's semantic
		// type order, so their intersection is exactly actual. Preserve that
		// canonical value directly: the reduced product's structural witness
		// meet can spell assignable record/table arms as Bottom before the
		// cross-axis quotient has a chance to restore presence and identity.
		return actual
	}
	return product.Meet(reg, actual, contract)
}

func boundaryArgumentSatisfiesContract(reg *axis.Registry, actual, contract product.Value) bool {
	actualType, actualOK := typevalue.TypeOf(reg, actual)
	contractType, contractOK := typevalue.TypeOf(reg, contract)
	if !actualOK || !contractOK || actualType == nil || contractType == nil {
		return false
	}
	return typecall.InstantiatedArgumentAssignable(actualType, contractType)
}

// instantiateBoundaryContracts substitutes one generic binder environment
// across both sides of a function boundary. Expected results are optional
// contextual evidence (for a direct return-forwarding call); they participate
// in the same inference transaction as ordinary actual parameters.
func instantiateBoundaryContracts(reg *axis.Registry, body *relationProgramBody, carrier state.State, expectedResults []typ.Type) ([]product.Value, []product.Value) {
	return instantiateBoundaryContractsFromValues(reg, body, func(index int) product.Value {
		params := body.plan.BoundaryParams()
		if index < 0 || index >= len(params) {
			return product.Bottom(reg)
		}
		return carrier.ReadValue(reg, key.SymbolValue(params[index]))
	}, expectedResults)
}

// instantiateBoundaryContractsFromValues is the canonical contract
// instantiation primitive. Callers provide the already-selected parameter
// coordinates; concrete State and guarded DD execution are merely adapters.
func instantiateBoundaryContractsFromValues(reg *axis.Registry, body *relationProgramBody, actualAt func(int) product.Value, expectedResults []typ.Type) ([]product.Value, []product.Value) {
	if reg == nil || body == nil || body.plan == nil {
		return nil, nil
	}
	params := body.plan.BoundaryParams()
	paramContracts := body.plan.BoundaryParamContracts()
	returnContracts := body.plan.BoundaryReturns()
	if len(paramContracts) != len(params) {
		return nil, nil
	}
	allContracts := make([]product.Value, 0, len(paramContracts)+len(returnContracts))
	allContracts = append(allContracts, paramContracts...)
	allContracts = append(allContracts, returnContracts...)
	formalTypes := make([]typ.Type, len(allContracts))
	actualTypes := make([]typ.Type, len(allContracts))
	var binders []*typ.TypeParam
	seenBinders := make(map[*typ.TypeParam]bool)
	cache := typevalue.NewCache()
	for index, contract := range allContracts {
		formal, ok := typevalue.TypeOf(reg, contract)
		if ok {
			formalTypes[index] = formal
			collectFreeBoundaryTypeParams(formal, nil, seenBinders, &binders, make(map[typ.Type]bool))
		}
		if index >= len(params) {
			result := index - len(params)
			if result < len(expectedResults) {
				actualTypes[index] = expectedResults[result]
			}
			continue
		}
		actual := product.Bottom(reg)
		if actualAt != nil {
			actual = actualAt(index)
		}
		if product.Equal(reg, actual, product.Bottom(reg)) {
			continue
		}
		if t, ok := typevalue.TypeOf(reg, actual); ok {
			actualTypes[index] = t
		} else if t, ok := typevalue.StructuralTypeOf(reg, cache, actual, typevalue.StructuralTypeOptions{ApplyPresence: true}); ok {
			actualTypes[index] = t
		}
	}

	instantiated := formalTypes
	if len(binders) != 0 {
		canonical, from, to := canonicalBoundaryTypeParams(binders)
		if len(from) != 0 {
			formalTypes = append([]typ.Type(nil), formalTypes...)
			for index, formal := range formalTypes {
				formalTypes[index] = subst.Params(formal, from, to)
			}
		}
		binders = canonical
		builder := typ.Func().ReserveParams(len(formalTypes))
		for _, binder := range binders {
			builder.TypeParamRef(binder)
		}
		for _, formal := range formalTypes {
			builder.Param("", formal)
		}
		synthetic := builder.Build()
		if fn, _, _ := typecall.InstantiateGenericCallWithBindings(synthetic, actualTypes); fn != nil && len(fn.Params) == len(formalTypes) {
			instantiated = make([]typ.Type, len(fn.Params))
			for index := range fn.Params {
				instantiated[index] = fn.Params[index].Type
			}
		}
	}

	materialized := append([]product.Value(nil), allContracts...)
	for index := range allContracts {
		contract := allContracts[index]
		if formalTypes[index] != nil && instantiated[index] != nil && !typ.TypeEquals(formalTypes[index], instantiated[index]) {
			contract = cache.FromTypeWithWitness(reg, instantiated[index])
		}
		materialized[index] = contract
	}
	return materialized[:len(paramContracts)], materialized[len(paramContracts):]
}

// canonicalBoundaryTypeParams collapses resolver copies of one lexical binder
// before substitution. Type annotations are resolved independently, so the T
// in Result<T> and the T in (T)->U can be distinct nodes even though they are
// the same function type parameter. Leaving both in a synthetic signature
// makes name-shadowing defeat substitution and keeps an otherwise concrete
// call generic.
func canonicalBoundaryTypeParams(params []*typ.TypeParam) (canonical, from []*typ.TypeParam, to []typ.Type) {
	for _, param := range params {
		if param == nil {
			continue
		}
		var representative *typ.TypeParam
		for _, known := range canonical {
			if known == param || known.Equals(param) {
				representative = known
				break
			}
		}
		if representative == nil {
			canonical = append(canonical, param)
			continue
		}
		if representative != param {
			from = append(from, param)
			to = append(to, representative)
		}
	}
	return canonical, from, to
}

// boundaryReturnContractPlan is the immutable result-coordinate policy for one
// call boundary. Generic inference and caller-context binding happen once when
// the plan is prepared; each result can then be normalized independently.
// This is the canonical return-contract primitive for both concrete State and
// guarded scalar execution.
type boundaryReturnContractPlan struct {
	reg     *axis.Registry
	results []boundaryReturnContract
}

type boundaryReturnContract struct {
	contract   product.Value
	active     bool
	structural bool
}

// prepareBoundaryReturnContractPlan closes generic result witnesses before the
// callee tuple crosses back into the caller. Direct return forwarding supplies
// the caller's instantiated declared result as inference evidence, covering
// return-only binders such as err<T>(string): Result<T> without inventing a
// second call-specialization path.
func prepareBoundaryReturnContractPlan(reg *axis.Registry, caller, target *relationProgramBody, frame *linkedRelationFrame, callerState, calleeState state.State) boundaryReturnContractPlan {
	return prepareBoundaryReturnContractPlanFromValues(reg, caller, target, frame,
		func(index int) product.Value {
			params := caller.plan.BoundaryParams()
			if index < 0 || index >= len(params) {
				return product.Bottom(reg)
			}
			return callerState.ReadValue(reg, key.SymbolValue(params[index]))
		},
		func(index int) product.Value {
			params := target.plan.BoundaryParams()
			if index < 0 || index >= len(params) {
				return product.Bottom(reg)
			}
			return calleeState.ReadValue(reg, key.SymbolValue(params[index]))
		})
}

// prepareBoundaryReturnContractPlanFromValues closes the same contract
// transaction directly over parameter coordinates. It is the only semantic
// implementation; the State entry point above supplies a lookup adapter.
func prepareBoundaryReturnContractPlanFromValues(reg *axis.Registry, caller, target *relationProgramBody, frame *linkedRelationFrame, callerActual, calleeActual func(int) product.Value) boundaryReturnContractPlan {
	if reg == nil || caller == nil || target == nil || target.plan == nil || frame == nil {
		return boundaryReturnContractPlan{}
	}
	_, callerReturns := instantiateBoundaryContractsFromValues(reg, caller, callerActual, nil)
	expected := make([]typ.Type, len(target.plan.BoundaryReturns()))
	for _, result := range frame.resultSelectors {
		if int(result.slot) >= len(expected) {
			continue
		}
		for _, destination := range result.targets {
			if destination.kind != factflow.CallResultTargetReturn {
				continue
			}
			index, ok := key.ParseReturnSlot(destination.slot)
			if !ok || index < 0 || index >= len(callerReturns) {
				continue
			}
			expected[result.slot], _ = typevalue.TypeOf(reg, callerReturns[index])
		}
	}
	_, returns := instantiateBoundaryContractsFromValues(reg, target, calleeActual, expected)
	plan := boundaryReturnContractPlan{reg: reg, results: make([]boundaryReturnContract, len(returns))}
	for index, contract := range returns {
		result := boundaryReturnContract{contract: contract}
		contractType, typed := typevalue.TypeOf(reg, contract)
		if typed && contractType != nil && !refinement.ContainsFreeTypeParam(contractType) {
			result.active = true
			_, result.structural = unwrap.Optional(contractType).(*typ.Record)
		}
		plan.results[index] = result
	}
	return plan
}

// NormalizeResult applies the prepared declaration policy to one result
// coordinate. It is pure: it neither reads nor writes State, and an absent,
// unresolved, or out-of-range contract is the identity operation. Bottom is
// preserved exactly.
func (p boundaryReturnContractPlan) NormalizeResult(index int, value product.Value) product.Value {
	if p.reg == nil || index < 0 || index >= len(p.results) {
		return value
	}
	result := p.results[index]
	if !result.active || product.Equal(p.reg, value, product.Bottom(p.reg)) {
		return value
	}
	if boundaryArgumentSatisfiesContract(p.reg, value, result.contract) {
		// A declared result is a constraint, not permission to erase a
		// stronger computed witness. In particular, returning the literal
		// "exact" from a function declared as string must remain literal at
		// the caller. The same meet transaction used for concrete arguments
		// preserves every non-witness axis and restores the proven actual
		// witness when the cheap witness lattice cannot express containment.
		projected := meetBoundaryParamContract(p.reg, value, result.contract)
		if result.structural {
			// Function-valued record members can have two lattice-equal
			// spellings (an inferred variadic closure and its declared
			// receiver signature). Canonicalize that structural witness at
			// the declaration boundary, while leaving scalar/literal results
			// exactly as computed.
			projected = product.WithPresence(p.reg,
				valuerefinement.MergeDeclaredContract(p.reg, projected, result.contract),
				product.PresenceOf(value))
		}
		return projected
	}
	// The computed value does not yet satisfy the instantiated declaration.
	// The declaration is the caller-visible contract, including its nilability:
	// a body diagnostic does not turn a declared `number` result into `number?`
	// for every caller. Control reachability is carried by the enclosing State,
	// not by weakening this returned value's Presence axis. Preserve Bottom
	// above, then merge the declared facts with the declaration's own presence.
	return product.WithPresence(p.reg,
		valuerefinement.MergeDeclaredContract(p.reg, value, result.contract),
		product.PresenceOf(result.contract))
}

// applyBoundaryReturnContracts is the whole-State adapter over the same unary
// primitive consumed by guarded execution. It owns no contract semantics.
func applyBoundaryReturnContracts(reg *axis.Registry, caller, target *relationProgramBody, frame *linkedRelationFrame, callerState, calleeState state.State) state.State {
	plan := prepareBoundaryReturnContractPlan(reg, caller, target, frame, callerState, calleeState)
	out := calleeState
	for index := range plan.results {
		value := calleeState.ReadReturnSlot(reg, index)
		normalized := plan.NormalizeResult(index, value)
		if !product.Equal(reg, value, normalized) {
			out = out.WriteReturnSlot(reg, index, normalized)
		}
	}
	return out
}

// collectFreeBoundaryTypeParams finds only binders free at the lexical body
// boundary. Binders owned by nested function/generic types remain scoped, and
// instantiated generic declarations contribute only their argument terms.
func collectFreeBoundaryTypeParams(t typ.Type, owned map[*typ.TypeParam]int, seen map[*typ.TypeParam]bool, out *[]*typ.TypeParam, active map[typ.Type]bool) {
	if t == nil {
		return
	}
	t = unwrap.Annotated(t)
	if t == nil {
		return
	}
	if param, ok := t.(*typ.TypeParam); ok {
		if owned[param] == 0 && !seen[param] {
			seen[param] = true
			*out = append(*out, param)
		}
		return
	}
	if active[t] {
		return
	}
	active[t] = true
	defer delete(active, t)

	switch value := t.(type) {
	case *typ.Instantiated:
		for _, arg := range value.TypeArgs {
			collectFreeBoundaryTypeParams(arg, owned, seen, out, active)
		}
		return
	case *typ.Function:
		next := owned
		if len(value.TypeParams) != 0 {
			next = make(map[*typ.TypeParam]int, len(owned)+len(value.TypeParams))
			for param, count := range owned {
				next[param] = count
			}
			for _, param := range value.TypeParams {
				if param != nil {
					next[param]++
				}
			}
		}
		for _, param := range value.Params {
			collectFreeBoundaryTypeParams(param.Type, next, seen, out, active)
		}
		collectFreeBoundaryTypeParams(value.Variadic, next, seen, out, active)
		for _, result := range value.Returns {
			collectFreeBoundaryTypeParams(result, next, seen, out, active)
		}
		return
	case *typ.Generic, *typ.Interface:
		return
	}
	if t.Kind() == kind.Ref || refinement.ContainsFreeTypeParam(t) {
		typ.WalkChildren(t, func(child typ.Type) bool {
			collectFreeBoundaryTypeParams(child, owned, seen, out, active)
			return false
		})
	}
}
