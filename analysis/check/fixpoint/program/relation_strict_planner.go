package program

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// prepareStagedStrictRelationCatalog is the internal-only phase-collapse
// planner. It keeps the existing zero-boundary leaf slice and adds only exact
// parameterized leaves whose complete context State and immutable stdlib
// global vector can be bound before any equation is omitted.
func prepareStagedStrictRelationCatalog(reg *axis.Registry, bindings *bind.Result, keys programKeys, prepared preparedBodies, rootFn *ast.FunctionExpr, stats *Stats) relationRunCatalog {
	if reg == nil || bindings == nil {
		return relationRunCatalog{}
	}
	owners := relationCatalogOwners(keys, prepared, rootFn)
	slices.SortFunc(owners, func(a, b relationCatalogOwner) int {
		if a.key.Less(b.key) {
			return -1
		}
		if b.key.Less(a.key) {
			return 1
		}
		return 0
	})
	type survivor struct {
		owner          relationCatalogOwner
		shape          transformer.Shape
		globalBoundary transformer.GlobalBoundary
		globalBindings []transformer.GlobalRootBinding
		contexts       []keyedFunction
	}
	var survivors []survivor
	seen := make(map[summary.SummaryKey]struct{}, len(owners))
	for _, owner := range owners {
		if stats != nil {
			stats.RelationPlannerOwnersScanned++
		}
		if owner.fn == nil || owner.key.Ref.IsZero() || owner.hasEntryState {
			continue
		}
		if _, duplicate := seen[owner.key]; duplicate {
			continue
		}
		seen[owner.key] = struct{}{}
		origin, ok := bindings.FunctionOrigin(owner.fn)
		if !ok || origin.Kind == bind.FunctionOriginMethod || owner.prepared == nil {
			continue
		}
		plan := owner.prepared.OperationPlan()
		if plan == nil || !plan.BoundaryParamsValid() || !plan.BoundaryCapturesValid() || !plan.BoundaryGlobalsValid() ||
			!relationBoundaryMatchesBindings(bindings, owner.fn, plan.BoundaryParams(), plan.BoundaryCaptures(), plan.BoundaryGlobals()) {
			continue
		}
		shape := transformer.Shape{Params: uint32(len(plan.BoundaryParams())), Captures: uint32(len(plan.BoundaryCaptures())), Globals: uint32(len(plan.BoundaryGlobals()))}
		candidate := survivor{owner: owner, shape: shape}
		switch {
		case shape == (transformer.Shape{}):
			if !owner.prepared.CompositionEligibility().Eligible() || !stagedStrictCallFree(owner.prepared.HasCallSites(), plan) {
				continue
			}
		case shape.Params != 0 && shape.Captures == 0 && shape.Globals != 0:
			if !stagedStrictAnnotatedParams(bindings, owner.fn, plan.BoundaryParams()) || !stagedStrictCallFree(owner.prepared.HasCallSites(), plan) {
				continue
			}
			var exact bool
			candidate.globalBoundary, candidate.globalBindings, exact = stagedStrictIntrinsicGlobals(bindings, owner, plan)
			if !exact {
				continue
			}
			candidate.contexts, exact = stagedStrictCertifiedContexts(keys, owner, plan)
			if !exact {
				continue
			}
		default:
			continue
		}
		if stats != nil {
			stats.RelationPlannerOwnersPrefiltered++
		}
		survivors = append(survivors, candidate)
	}
	out := relationRunCatalog{byKey: make(map[summary.SummaryKey]int, len(survivors)), consumers: relationConsumerPolicy{byKey: make(map[summary.SummaryKey]int, len(survivors))}}
	out.generation = &relationCatalogGeneration{}
	out.consumers.generation = out.generation
	out.contextGeneration, out.registry = keys.contexts.discoveryGeneration, reg
	compiler := transformer.NewPlanCompiler()
	for _, candidate := range survivors {
		owner, plan := candidate.owner, candidate.owner.prepared.OperationPlan()
		if stats != nil {
			stats.RelationPlannerOwnersCompiled++
		}
		compiled, err := compiler.Prepare(reg, owner.prepared.Graph(), plan, candidate.shape)
		if err != nil || !compiled.EffectFree() {
			continue
		}
		direct, err := transformer.NewDirectCallCatalog(plan.PointCount(), nil)
		if err != nil {
			continue
		}
		identity := relationCellIdentity{Cell: transformer.CellRef{Function: uint64(len(out.entries) + 1)}, Summary: owner.key, BodyDigest: owner.prepared.IdentityDigest(), Prepared: owner.prepared, Generation: out.generation}
		out.byKey[owner.key] = len(out.entries)
		out.entries = append(out.entries, relationCatalogEntry{identity: identity, function: owner.fn, compiler: compiled, direct: direct, globalBoundary: candidate.globalBoundary})
		consumerIdentity := relationConsumerIdentity{Summary: owner.key, BodyDigest: identity.BodyDigest, Prepared: owner.prepared, Generation: out.generation}
		out.consumers.byKey[owner.key] = len(out.consumers.entries)
		out.consumers.entries = append(out.consumers.entries, relationConsumerEntry{identity: consumerIdentity, direct: direct, active: candidate.shape == (transformer.Shape{})})
		for _, context := range candidate.contexts {
			certificate := context.relationContextEntry
			params := make([]product.Value, len(certificate.params))
			captures := make([]product.Value, len(certificate.captures))
			paths := make([]pathdom.Path, len(params)+len(captures))
			for i, param := range certificate.params {
				params[i], paths[i] = param.value, pathdom.NewPlaceholder(i)
			}
			for i, capture := range certificate.captures {
				captures[i], paths[len(params)+i] = capture.value, capture.path
			}
			out.contexts = append(out.contexts, relationContextCandidate{context: context.key, base: identity, prepared: owner.prepared, discoveryGeneration: certificate.discoveryGeneration, params: params, captures: captures, globalBoundary: candidate.globalBoundary, globalBindings: append([]transformer.GlobalRootBinding(nil), candidate.globalBindings...), paths: paths, certificate: certificate})
		}
		if stats != nil {
			stats.RelationPlannerOwnersActivated++
		}
	}
	return out
}

func stagedStrictAnnotatedParams(bindings *bind.Result, fn *ast.FunctionExpr, boundary []symbol.ID) bool {
	if bindings == nil || fn == nil {
		return false
	}
	slots := bindings.ParamSlots(fn)
	if len(slots) == 0 || len(slots) != len(boundary) {
		return false
	}
	for i, slot := range slots {
		if slot.Symbol == 0 || slot.Symbol != boundary[i] || slot.Vararg || slot.ImplicitSelf || slot.Type == nil {
			return false
		}
	}
	return true
}

// stagedStrictIntrinsicGlobals accepts only the canonical immutable Lua type
// global. The parameterized slice is independently call-free; normalized
// structural type predicates have no call site and are certified by
// PlanCompiler.
func stagedStrictIntrinsicGlobals(bindings *bind.Result, owner relationCatalogOwner, plan *operationplan.Plan) (transformer.GlobalBoundary, []transformer.GlobalRootBinding, bool) {
	if bindings == nil || owner.prepared == nil || plan == nil {
		return transformer.GlobalBoundary{}, nil, false
	}
	environment := owner.prepared.BoundaryEnvironmentDigest()
	descriptors := make([]transformer.GlobalRootDescriptor, 0, len(plan.BoundaryGlobals()))
	bound := make([]transformer.GlobalRootBinding, 0, len(plan.BoundaryGlobals()))
	for _, global := range plan.BoundaryGlobals() {
		name := bindings.Name(global)
		if name != "type" {
			return transformer.GlobalBoundary{}, nil, false
		}
		content, err := transformer.DeriveGlobalContentID(environment, transformer.GlobalRootImmutableStdlib, name)
		if err != nil {
			return transformer.GlobalBoundary{}, nil, false
		}
		descriptors = append(descriptors, transformer.GlobalRootDescriptor{Symbol: global, Class: transformer.GlobalRootImmutableStdlib, StableName: name, ContentID: content})
		bound = append(bound, transformer.GlobalRootBinding{Symbol: global, ContentID: content, Value: product.Top()})
	}
	boundary, err := transformer.SealGlobalBoundary(transformer.GlobalBoundaryComplete, descriptors)
	if err != nil {
		return transformer.GlobalBoundary{}, nil, false
	}
	if _, err := transformer.InstantiateGlobalBoundary(boundary, bound); err != nil {
		return transformer.GlobalBoundary{}, nil, false
	}
	return boundary, bound, true
}

func stagedStrictCertifiedContexts(keys programKeys, owner relationCatalogOwner, plan *operationplan.Plan) ([]keyedFunction, bool) {
	if plan == nil || keys.contexts.discoveryGeneration == 0 {
		return nil, false
	}
	var out []keyedFunction
	for _, context := range keys.contexts.entries {
		if context.funcExpr != owner.fn {
			continue
		}
		certificate := context.relationContextEntry
		if certificate == nil || certificate.context != context.key || certificate.base != owner.key || certificate.discoveryGeneration != keys.contexts.discoveryGeneration || certificate.preparedBodyDigest != owner.prepared.IdentityDigest() || !certificate.matchesBoundary(plan.BoundaryParams(), plan.BoundaryCaptures()) {
			return nil, false
		}
		out = append(out, context)
	}
	slices.SortFunc(out, func(a, b keyedFunction) int {
		if a.key.Less(b.key) {
			return -1
		}
		if b.key.Less(a.key) {
			return 1
		}
		return 0
	})
	return out, len(out) != 0
}

func stagedStrictCallFree(metadataHasCalls bool, plan *operationplan.Plan) bool {
	if metadataHasCalls || plan == nil {
		return false
	}
	facts := plan.Facts()
	if facts.HasCallSites() {
		return false
	}
	for raw := 0; raw < plan.PointCount(); raw++ {
		if _, call := facts.CallSiteView(cfg.Point(raw)); call {
			return false
		}
	}
	return true
}
