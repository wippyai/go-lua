package program

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// prepareStagedStrictRelationCatalog is the internal-only phase-collapse
// planner. It keeps the existing zero-boundary leaf slice and adds only exact
// parameterized leaves whose complete context State and immutable stdlib
// global vector can be bound before any equation is omitted.
type stagedStrictContext struct {
	origin      keyedFunction
	certificate *relationContextEntryCertificate
	frame       *strictValidatedFrameCandidate
}

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
		contexts       []stagedStrictContext
		compiler       *transformer.PreparedPlanCompiler
		identity       relationCellIdentity
		direct         transformer.DirectCallCatalog
	}
	var survivors []survivor
	seen := make(map[summary.SummaryKey]struct{}, len(owners))
	for _, owner := range owners {
		if stats != nil {
			stats.RelationPlannerOwnersScanned++
		}
		if owner.fn == nil || owner.key.Ref.IsZero() {
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
		if _, exact := plan.CallSurface(); !exact {
			continue
		}
		switch {
		case shape == (transformer.Shape{}):
			// A zero-boundary relation pins the lexical base summary itself, so
			// it must not erase a seeded base entry. Parameterized owners below
			// retain a separately solved concrete base before omission, preserving
			// that seeded state and its exact dependency lineage.
			if owner.hasEntryState || !owner.prepared.CompositionEligibility().Eligible() {
				continue
			}
		case shape.Params != 0 && shape.Captures == 0:
			if !stagedStrictAnnotatedParams(bindings, owner.fn, plan.BoundaryParams()) {
				continue
			}
			if shape.Globals != 0 {
				var exact bool
				candidate.globalBoundary, candidate.globalBindings, exact = stagedStrictIntrinsicGlobals(bindings, owner, plan)
				if !exact {
					continue
				}
			}
			var exact bool
			candidate.contexts, exact = stagedStrictCertifiedContexts(reg, bindings, keys, owner, plan, shape, candidate.globalBoundary)
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
	compiled := make([]survivor, 0, len(survivors))
	for _, candidate := range survivors {
		owner, plan := candidate.owner, candidate.owner.prepared.OperationPlan()
		if stats != nil {
			stats.RelationPlannerOwnersCompiled++
		}
		preparedCompiler, err := compiler.Prepare(reg, owner.prepared.Graph(), plan, candidate.shape)
		if err != nil || !preparedCompiler.EffectFree() {
			continue
		}
		candidate.compiler = preparedCompiler
		candidate.identity = relationCellIdentity{Cell: transformer.CellRef{Function: uint64(len(compiled) + 1)}, Summary: owner.key, BodyDigest: owner.prepared.IdentityDigest(), Prepared: owner.prepared, Generation: out.generation}
		compiled = append(compiled, candidate)
	}

	// Resolve only the independently sealed lexical surface. A rejected,
	// external, missing, ambiguous, or cross-generation target rejects the
	// whole producer. Candidate identities are assigned before routing so every
	// dependency edge belongs to this exact transaction generation.
	byBody := make(map[lexicalidentity.StableLexicalBodyID]int, len(compiled))
	for index, candidate := range compiled {
		bodyID := candidate.owner.prepared.StableLexicalBodyID()
		if bodyID == (lexicalidentity.StableLexicalBodyID{}) {
			continue
		}
		if _, duplicate := byBody[bodyID]; duplicate {
			byBody[bodyID] = -1
		} else {
			byBody[bodyID] = index
		}
	}
	routed := make(map[transformer.CellRef]survivor, len(compiled))
	for _, candidate := range compiled {
		plan := candidate.owner.prepared.OperationPlan()
		surface, exact := plan.CallSurface()
		routes := make(map[cfg.Point]transformer.DirectCallTarget)
		for _, site := range surface.Sites() {
			call, represented := plan.Facts().CallSiteView(site.Point)
			if !represented {
				exact = false
				break
			}
			switch site.Target.Kind() {
			case operationplan.CallSurfaceTargetExternal:
				operation, sealed := plan.SignatureCallOperation(site.Point)
				if !sealed || !strictExternalCallSurfaceExact(reg, call, site.Target, operation) {
					exact = false
				}
				continue
			case operationplan.CallSurfaceTargetLexical:
				if !relationDirectSiteExact(call) {
					exact = false
				}
			default:
				exact = false
			}
			if !exact {
				break
			}
			targetBody, _ := site.Target.LexicalBody()
			targetIndex, found := byBody[targetBody]
			if !found || targetIndex < 0 {
				exact = false
				break
			}
			target := compiled[targetIndex]
			if !paramsOnlyShape(target.shape) || !target.compiler.DirectCompositionEligible() {
				exact = false
				break
			}
			routes[site.Point] = transformer.DirectCallTarget{Cell: target.identity.Cell, Shape: target.shape}
		}
		if !exact {
			continue
		}
		direct, err := transformer.NewDirectCallCatalog(plan.PointCount(), routes)
		if err != nil {
			continue
		}
		candidate.direct = direct
		routed[candidate.identity.Cell] = candidate
	}

	// Admit dependency-closed acyclic regions bottom-up. A cycle never reaches
	// the allowed set; widening an unresolved Bottom dependency is forbidden for
	// this first direct-call slice.
	allowed := make(map[transformer.CellRef]struct{}, len(routed))
	for changed := true; changed; {
		changed = false
		for cell, candidate := range routed {
			if _, admitted := allowed[cell]; admitted {
				continue
			}
			closed := true
			for _, dependency := range candidate.direct.Cells() {
				if _, admitted := allowed[dependency]; !admitted {
					closed = false
					break
				}
			}
			if closed {
				allowed[cell] = struct{}{}
				changed = true
			}
		}
	}

	for _, candidate := range compiled {
		if _, admitted := allowed[candidate.identity.Cell]; !admitted {
			continue
		}
		candidate = routed[candidate.identity.Cell]
		owner, identity, direct := candidate.owner, candidate.identity, candidate.direct
		out.byKey[owner.key] = len(out.entries)
		out.entries = append(out.entries, relationCatalogEntry{identity: identity, function: owner.fn, compiler: candidate.compiler, direct: direct, globalBoundary: candidate.globalBoundary})
		consumerIdentity := relationConsumerIdentity{Summary: owner.key, BodyDigest: identity.BodyDigest, Prepared: owner.prepared, Generation: out.generation}
		out.consumers.byKey[owner.key] = len(out.consumers.entries)
		out.consumers.entries = append(out.consumers.entries, relationConsumerEntry{identity: consumerIdentity, direct: direct, active: candidate.shape == (transformer.Shape{})})
		for _, context := range candidate.contexts {
			certificate := context.certificate
			params := make([]product.Value, len(certificate.params))
			captures := make([]product.Value, len(certificate.captures))
			paths := make([]pathdom.Path, len(params)+len(captures))
			for i, param := range certificate.params {
				params[i], paths[i] = param.value, pathdom.NewPlaceholder(i)
			}
			for i, capture := range certificate.captures {
				captures[i], paths[len(params)+i] = capture.value, capture.path
			}
			out.contexts = append(out.contexts, relationContextCandidate{context: context.origin.key, base: identity, prepared: owner.prepared, discoveryGeneration: certificate.discoveryGeneration, params: params, captures: captures, globalBoundary: candidate.globalBoundary, globalBindings: append([]transformer.GlobalRootBinding(nil), candidate.globalBindings...), paths: paths, certificate: certificate, validatedFrame: context.frame})
		}
		if stats != nil {
			stats.RelationPlannerOwnersActivated++
		}
	}
	return out
}

func strictExternalCallSurfaceExact(reg *axis.Registry, call factflow.CallSiteView, target operationplan.CallSurfaceTarget, operation operationplan.SignatureCallOperation) bool {
	if reg == nil || !target.MatchesExternalOperation(operation) {
		return false
	}
	if call.MethodName() == "" {
		return true
	}
	_, exact := effectlowering.StaticScalarSignatureReturns(reg, nil, operation.Signature())
	return exact
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
// global. Normalized structural type predicates have no call site and are
// certified by PlanCompiler; any actual call is owned separately by the sealed
// direct-call catalog.
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

func stagedStrictCertifiedContexts(reg *axis.Registry, bindings *bind.Result, keys programKeys, owner relationCatalogOwner, plan *operationplan.Plan, shape transformer.Shape, globalBoundary transformer.GlobalBoundary) ([]stagedStrictContext, bool) {
	if reg == nil || bindings == nil || plan == nil || keys.contexts.discoveryGeneration == 0 {
		return nil, false
	}
	var out []stagedStrictContext
	for _, context := range keys.contexts.entries {
		if context.funcExpr != owner.fn {
			continue
		}
		certificate := context.relationContextEntry
		var frame *strictValidatedFrameCandidate
		if certificate == nil {
			var exact bool
			certificate, frame, exact = strictValidatedFrameContextCertificate(reg, bindings, owner.fn, plan, shape, globalBoundary, context, owner.key, owner.prepared.IdentityDigest(), keys.contexts.discoveryGeneration)
			if !exact {
				return nil, false
			}
		}
		if certificate.context != context.key || certificate.base != owner.key || certificate.discoveryGeneration != keys.contexts.discoveryGeneration || certificate.preparedBodyDigest != owner.prepared.IdentityDigest() || !certificate.matchesBoundary(plan.BoundaryParams(), plan.BoundaryCaptures()) {
			return nil, false
		}
		out = append(out, stagedStrictContext{origin: context, certificate: certificate, frame: frame})
	}
	slices.SortFunc(out, func(a, b stagedStrictContext) int {
		if a.origin.key.Less(b.origin.key) {
			return -1
		}
		if b.origin.key.Less(a.origin.key) {
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
