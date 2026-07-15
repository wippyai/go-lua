package program

import (
	"context"
	"fmt"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// runEvaluatedBoundChunk is the production-shaped, non-default evaluated
// transaction. It prepares the bound program exactly once and then takes only
// the total catalog -> relation fixpoint -> evaluated-root path. It never
// discovers call contexts, invokes query.Run, solves a body, or materializes a
// body.Result. Unsupported input fails the whole transaction with no artifact.
func runEvaluatedBoundChunk(ctx context.Context, stmts []ast.Stmt, bindings *bind.Result, config Config) (evaluatedProgram, error) {
	if ctx == nil {
		return evaluatedProgram{}, fmt.Errorf("evaluated bound program: context is required")
	}
	if err := ctx.Err(); err != nil {
		return evaluatedProgram{}, err
	}
	config.Context = ctx
	config = configWithStats(config)
	if config.Check.Registry == nil || bindings == nil {
		return evaluatedProgram{}, fmt.Errorf("evaluated bound program: registry and lexical bindings are required")
	}
	keys := collectKeys(bindings, rootKey(config.RootKey), config.Check.Registry, config.Check.ModuleTypes, config.Check.ModuleExports, stmts)
	config.Check = configWithMetatableMethodSignatureArguments(config.Check, keys.metatableProof)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, config.Check, keys)
	if err != nil {
		return evaluatedProgram{}, err
	}
	if err := ctx.Err(); err != nil {
		return evaluatedProgram{}, err
	}
	catalog, err := prepareTotalEvaluatedRelationCatalog(config.Check.Registry, bindings, keys, prepared, config.Stats)
	if err != nil {
		return evaluatedProgram{}, err
	}
	// Observer construction is a single structural projection of the sealed
	// catalog. It neither evaluates relations nor discovers invocation paths.
	forest, err := buildLexicalObserverForest(ctx, catalog)
	if err != nil {
		return evaluatedProgram{}, err
	}
	boundary, err := evaluatedCatalogBoundaryBindings(catalog)
	if err != nil {
		return evaluatedProgram{}, err
	}
	return solveEvaluatedObserverProgram(ctx, catalog, boundary, forest, config.Stats)
}

// prepareTotalEvaluatedRelationCatalog independently builds every equation,
// including the uniquely identified chunk root. It does not reuse the partial
// inactive activation catalog: absence of any owner or route rejects the whole
// evaluated transaction.
func prepareTotalEvaluatedRelationCatalog(reg *axis.Registry, bindings *bind.Result, keys programKeys, prepared preparedBodies, stats *Stats) (relationRunCatalog, error) {
	if reg == nil || bindings == nil || prepared.root == nil {
		return relationRunCatalog{}, fmt.Errorf("evaluated catalog: registry, bindings, and root preparation are required")
	}
	owners := relationCatalogOwners(keys, prepared, nil)
	slices.SortFunc(owners, func(left, right relationCatalogOwner) int {
		if left.key.Less(right.key) {
			return -1
		}
		if right.key.Less(left.key) {
			return 1
		}
		return 0
	})
	candidates := make(map[summary.SummaryKey]*relationCatalogCandidate, len(owners))
	for _, owner := range owners {
		if owner.key.Ref.IsZero() || owner.prepared == nil {
			return relationRunCatalog{}, fmt.Errorf("evaluated catalog: incomplete prepared owner")
		}
		if _, duplicate := candidates[owner.key]; duplicate {
			return relationRunCatalog{}, fmt.Errorf("evaluated catalog: duplicate owner %v", owner.key)
		}
		plan := owner.prepared.OperationPlan()
		if plan == nil || !plan.BoundaryParamsValid() || !plan.BoundaryCapturesValid() || !plan.BoundaryGlobalsValid() {
			return relationRunCatalog{}, fmt.Errorf("evaluated catalog: owner %v has no exact boundary", owner.key)
		}
		shape := transformer.Shape{Params: uint32(len(plan.BoundaryParams())), Captures: uint32(len(plan.BoundaryCaptures())), Globals: uint32(len(plan.BoundaryGlobals()))}
		if shape.Captures != 0 || shape.Globals != 0 {
			return relationRunCatalog{}, fmt.Errorf("evaluated catalog: owner %v is outside the params-only slice", owner.key)
		}
		if owner.fn == nil {
			if owner.key != keys.rootKey || owner.prepared != prepared.root || shape != (transformer.Shape{}) {
				return relationRunCatalog{}, fmt.Errorf("evaluated catalog: nil function owner is not the exact zero-boundary chunk root")
			}
		} else {
			origin, ok := bindings.FunctionOrigin(owner.fn)
			if !ok || origin.Kind == bind.FunctionOriginMethod || !owner.prepared.CompositionEligibility().Eligible() ||
				!relationBoundaryMatchesBindings(bindings, owner.fn, plan.BoundaryParams(), plan.BoundaryCaptures(), plan.BoundaryGlobals()) {
				return relationRunCatalog{}, fmt.Errorf("evaluated catalog: lexical owner %v is outside the exact params-only slice", owner.key)
			}
		}
		if stats != nil {
			stats.EvaluatedRelationCompilerPrepares++
		}
		planCompiler := transformer.NewPlanCompiler()
		compiler, err := prepareTotalEvaluatedCompiler(planCompiler, reg, owner, bindings, plan, shape)
		if err != nil {
			return relationRunCatalog{}, fmt.Errorf("evaluated catalog: owner %v transformer preparation: %w", owner.key, err)
		}
		if !compiler.EffectFree() {
			return relationRunCatalog{}, fmt.Errorf("evaluated catalog: owner %v has symbolic effects", owner.key)
		}
		candidates[owner.key] = &relationCatalogCandidate{
			key: owner.key, fn: owner.fn, prepared: owner.prepared, compiler: compiler, shape: shape,
			direct: exactRelationDirectTargets(bindings, keys, owner.key, owner.prepared), hasEntryState: owner.hasEntryState,
		}
	}
	if len(candidates) != len(owners) {
		return relationRunCatalog{}, fmt.Errorf("evaluated catalog: candidate set is partial")
	}
	if recursive := recursiveRelationCandidates(candidates); len(recursive) != 0 {
		return relationRunCatalog{}, fmt.Errorf("evaluated catalog: recursive relation family requires widening")
	}

	catalog := relationRunCatalog{
		entries: make([]relationCatalogEntry, 0, len(owners)), byKey: make(map[summary.SummaryKey]int, len(owners)),
		registry: reg, generation: &relationCatalogGeneration{},
	}
	identities := make(map[summary.SummaryKey]relationCellIdentity, len(owners))
	for index, owner := range owners {
		candidate := candidates[owner.key]
		identities[owner.key] = relationCellIdentity{
			Cell: transformer.CellRef{Function: uint64(index + 1)}, Summary: owner.key,
			BodyDigest: candidate.prepared.IdentityDigest(), Prepared: candidate.prepared, Generation: catalog.generation,
		}
	}
	for _, owner := range owners {
		candidate := candidates[owner.key]
		routes := make(map[cfg.Point]transformer.DirectCallTarget, len(candidate.direct))
		for point, targetKey := range candidate.direct {
			target, admitted := candidates[targetKey]
			targetIdentity, identified := identities[targetKey]
			if !admitted || !identified {
				return relationRunCatalog{}, fmt.Errorf("evaluated catalog: owner %v point %d targets an unowned relation", owner.key, point)
			}
			routes[point] = transformer.DirectCallTarget{Cell: targetIdentity.Cell, Shape: target.shape}
		}
		direct, err := transformer.NewDirectCallCatalog(candidate.prepared.OperationPlan().PointCount(), routes)
		if err != nil {
			return relationRunCatalog{}, fmt.Errorf("evaluated catalog: owner %v direct-call surface: %w", owner.key, err)
		}
		if len(direct.Cells()) != 0 && !candidate.compiler.DirectCompositionEligible() {
			return relationRunCatalog{}, fmt.Errorf("evaluated catalog: owner %v cannot compose direct calls", owner.key)
		}
		entry := sealRelationCatalogEntry(relationCatalogEntry{
			identity: identities[owner.key], function: owner.fn, compiler: candidate.compiler, direct: direct, hasEntryState: owner.hasEntryState,
		})
		catalog.byKey[owner.key] = len(catalog.entries)
		catalog.entries = append(catalog.entries, entry)
	}

	for _, entry := range catalog.entries {
		if err := validateTotalEvaluatedDirectSurface(entry, catalog, bindings, keys); err != nil {
			return relationRunCatalog{}, err
		}
	}
	return catalog, nil
}

func prepareTotalEvaluatedCompiler(compiler *transformer.PlanCompiler, reg *axis.Registry, owner relationCatalogOwner, bindings *bind.Result, plan *operationplan.Plan, shape transformer.Shape) (*transformer.PreparedPlanCompiler, error) {
	if compiler == nil || bindings == nil || plan == nil {
		return nil, fmt.Errorf("evaluated catalog: compiler, bindings, and plan are required")
	}
	hasFunctions := false
	plan.Facts().ForEachExpressionFunction(func(factflow.ExprRef, symbol.ID) bool {
		hasFunctions = true
		return false
	})
	if !hasFunctions {
		return compiler.Prepare(reg, owner.prepared.Graph(), plan, shape)
	}

	authority, err := transformer.SealDirectLexicalDeclarationAuthority(plan, bindings, owner.fn)
	if err != nil {
		return nil, fmt.Errorf("compiler: contextual operations: ExpressionFunctions (%v)", err)
	}
	return compiler.PrepareWithDirectLexicalDeclarations(reg, owner.prepared.Graph(), plan, shape, authority)
}

func validateTotalEvaluatedDirectSurface(entry relationCatalogEntry, catalog relationRunCatalog, bindings *bind.Result, keys programKeys) error {
	if entry.identity.Prepared == nil || entry.compiler == nil {
		return fmt.Errorf("evaluated catalog: cell %v has no prepared compiler", entry.identity.Cell)
	}
	plan := entry.identity.Prepared.OperationPlan()
	surface, ok := plan.CallSurface()
	if !ok || !surface.Complete() || surface.Owner() != entry.identity.Prepared.StableLexicalBodyID() || entry.direct.PointCount() != plan.PointCount() {
		return fmt.Errorf("evaluated catalog: cell %v has an incomplete call surface", entry.identity.Cell)
	}
	expected := exactRelationDirectTargets(bindings, keys, entry.identity.Summary, entry.identity.Prepared)
	if len(surface.Sites()) != len(expected) {
		return fmt.Errorf("evaluated catalog: cell %v call surface has unsupported residue", entry.identity.Cell)
	}
	for _, site := range surface.Sites() {
		targetKey, resolved := expected[site.Point]
		targetIndex, admitted := catalog.byKey[targetKey]
		if site.Target.Kind() != operationplan.CallSurfaceTargetLexical || !resolved || !admitted || targetIndex < 0 || targetIndex >= len(catalog.entries) {
			return fmt.Errorf("evaluated catalog: cell %v point %d is not an admitted lexical route", entry.identity.Cell, site.Point)
		}
		targetEntry := catalog.entries[targetIndex]
		targetBody, lexical := site.Target.LexicalBody()
		route, routed := entry.direct.Lookup(site.Point)
		if !lexical || targetBody != targetEntry.identity.Prepared.StableLexicalBodyID() || !routed ||
			route.Cell != targetEntry.identity.Cell || route.Shape != targetEntry.compiler.Shape() {
			return fmt.Errorf("evaluated catalog: cell %v point %d route identity differs from the sealed surface: lexical=%v body=%x/%x routed=%v cell=%v/%v shape=%v/%v",
				entry.identity.Cell, site.Point, lexical, targetBody, targetEntry.identity.Prepared.StableLexicalBodyID(), routed,
				route.Cell, targetEntry.identity.Cell, route.Shape, targetEntry.compiler.Shape())
		}
	}
	return nil
}

func evaluatedCatalogBoundaryBindings(catalog relationRunCatalog) (map[lexicalidentity.StableLexicalBodyID]evaluatedProgramBindings, error) {
	out := make(map[lexicalidentity.StableLexicalBodyID]evaluatedProgramBindings, len(catalog.entries))
	for _, entry := range catalog.entries {
		if entry.identity.Prepared == nil || entry.compiler == nil {
			return nil, fmt.Errorf("evaluated catalog: incomplete boundary owner %v", entry.identity.Cell)
		}
		plan := entry.identity.Prepared.OperationPlan()
		shape := entry.compiler.Shape()
		if plan == nil || shape.Captures != 0 || shape.Globals != 0 || shape.Results != 0 || shape.HeapTemplates != 0 ||
			shape.Params != uint32(len(plan.BoundaryParams())) {
			return nil, fmt.Errorf("evaluated catalog: cell %v boundary is not params-only", entry.identity.Cell)
		}
		contracts := plan.BoundaryParamContracts()
		if len(contracts) != int(shape.Params) {
			return nil, fmt.Errorf("evaluated catalog: cell %v parameter contracts are incomplete", entry.identity.Cell)
		}
		values := append([]product.Value(nil), contracts...)
		paths := make([]pathdom.Path, len(values))
		for index := range paths {
			paths[index] = pathdom.NewPlaceholder(index)
		}
		bodyID := entry.identity.Prepared.StableLexicalBodyID()
		if bodyID == (lexicalidentity.StableLexicalBodyID{}) {
			return nil, fmt.Errorf("evaluated catalog: cell %v has no lexical body identity", entry.identity.Cell)
		}
		if _, duplicate := out[bodyID]; duplicate {
			return nil, fmt.Errorf("evaluated catalog: duplicate boundary body %x", bodyID)
		}
		out[bodyID] = evaluatedProgramBindings{values: values, paths: paths, order: plan.BoundaryParams()}
	}
	return out, nil
}
