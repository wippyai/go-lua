package transformer

import (
	"context"
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// PreparedPlanCompiler owns the persistent Builder and static semantic
// certificate for one function. Evaluate allocates row scratch only; term DAGs,
// descriptors, and operation-plan catalogs remain stable across equation reads.
type PreparedPlanCompiler struct {
	compiler             *PlanCompiler
	registry             *axis.Registry
	graph                cfg.Graph
	plan                 *operationplan.Plan
	shape                Shape
	builder              *Builder
	base                 planCompileContext
	certificate          SemanticCertificate
	wtoTape              *symbolicWTOTape
	observationComplete  bool
	projectionPlan       *sparseProjectionPlan
	projectionPlanReason string
	// cyclic is retained as prepared topology metadata for compatibility and
	// diagnostics. Evaluation deliberately does not branch on it: DAGs are the
	// zero-component case of the same exact dense executor.
	cyclic bool
}

// Shape returns the immutable packed boundary schema owned by this prepared
// lexical compiler.
func (p *PreparedPlanCompiler) Shape() Shape {
	if p == nil {
		return Shape{}
	}
	return p.shape
}

// MatchesPreparation proves this compiler was built from the exact run-local
// registry, CFG, operation plan, and packed boundary shape. Content digests do
// not substitute for these preparation owners.
func (p *PreparedPlanCompiler) MatchesPreparation(reg *axis.Registry, graph cfg.Graph, plan *operationplan.Plan, shape Shape) bool {
	return p != nil && reg != nil && graph != nil && plan != nil &&
		p.registry == reg && p.plan == plan && p.graph != nil && p.graph.ID() == graph.ID() && p.shape == shape
}

// EffectFree reports whether this prepared compiler can emit any structured
// EffectTerm. It inspects the same preparation-owned allocation terms and
// point effect catalog used by lowering; compiler admission alone is not an
// effect-free proof because exact signature allocations and writes are valid
// relation programs.
func (p *PreparedPlanCompiler) EffectFree() bool {
	if p == nil || p.plan == nil {
		return false
	}
	for _, effect := range p.base.allocationEffects {
		if effect != 0 {
			return false
		}
	}
	catalog := DefaultEffectCatalog()
	for raw := 0; raw < p.plan.PointCount(); raw++ {
		cursor := p.plan.Cursor(cfg.Point(raw))
		active := make([]operationplan.Kind, 0, 2)
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			active = append(active, cell.Kind())
		}
		_, admitted, err := catalog.AdmitPoint(active)
		if err != nil || admitted {
			return false
		}
	}
	return true
}

// Compile preserves the historical fail-closed API over the prepared
// lifecycle.
func (c *PlanCompiler) Compile(reg *axis.Registry, graph cfg.Graph, plan *operationplan.Plan, shape Shape) Relation {
	return c.compilePrepared(reg, graph, plan, shape)
}

// Prepare validates all context-independent compiler inputs once. Direct
// lexical dependencies are deliberately a later Evaluate concern; this base
// preparation preserves today's exact no-dependency behavior.
func (c *PlanCompiler) Prepare(reg *axis.Registry, graph cfg.Graph, plan *operationplan.Plan, shape Shape) (*PreparedPlanCompiler, error) {
	return c.prepare(reg, graph, plan, shape, operationplan.DirectLexicalDeclarations{}, false)
}

// PrepareWithLexicalBoundaryRoots explicitly admits RootCapture and RootGlobal
// terms owned by the plan's exact boundary schema. The ordinary Prepare API
// retains its historical parameter-only admission policy.
func (c *PlanCompiler) PrepareWithLexicalBoundaryRoots(reg *axis.Registry, graph cfg.Graph, plan *operationplan.Plan, shape Shape) (*PreparedPlanCompiler, error) {
	return c.prepare(reg, graph, plan, shape, operationplan.DirectLexicalDeclarations{}, true)
}

// PrepareWithDirectLexicalDeclarations admits only a binder-proven complete,
// stable, non-escaping local-function declaration census sealed to plan.
func (c *PlanCompiler) PrepareWithDirectLexicalDeclarations(reg *axis.Registry, graph cfg.Graph, plan *operationplan.Plan, shape Shape, authority DirectLexicalDeclarationAuthority) (*PreparedPlanCompiler, error) {
	if !authority.matches(plan) {
		return nil, fmt.Errorf("compiler: direct lexical declaration authority does not match plan")
	}
	return c.prepare(reg, graph, plan, shape, authority.declarations, false)
}

// PrepareWithDirectLexicalDeclarationsAndBoundaryRoots combines the two
// independently sealed capabilities used by total evaluated programs.
func (c *PlanCompiler) PrepareWithDirectLexicalDeclarationsAndBoundaryRoots(reg *axis.Registry, graph cfg.Graph, plan *operationplan.Plan, shape Shape, authority DirectLexicalDeclarationAuthority) (*PreparedPlanCompiler, error) {
	if !authority.matches(plan) {
		return nil, fmt.Errorf("compiler: direct lexical declaration authority does not match plan")
	}
	return c.prepare(reg, graph, plan, shape, authority.declarations, true)
}

func (c *PlanCompiler) prepare(reg *axis.Registry, graph cfg.Graph, plan *operationplan.Plan, shape Shape, declarations operationplan.DirectLexicalDeclarations, allowLexicalBoundaryRoots bool) (*PreparedPlanCompiler, error) {
	if c == nil || reg == nil || graph == nil || plan == nil {
		return nil, fmt.Errorf("compiler: registry, graph, plan, and compiler are required")
	}
	if graph.Size() != plan.PointCount() {
		return nil, fmt.Errorf("compiler: graph points %d != operation rows %d", graph.Size(), plan.PointCount())
	}
	ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts(), directDeclarations: declarations, allowLexicalBoundaryRoots: allowLexicalBoundaryRoots}
	outputCaps := DefaultOutputCapabilityRegistry()
	summaryKinds := []callboundary.BoundaryFactKind{
		"NormalReturnParams", "NormalReturnFacts", "ReturnFlows", "ReturnParamPathAliases",
		"ReturnConditionParamRefinements",
	}
	if planReturnArity(plan) > 1 {
		summaryKinds = append(summaryKinds, "ReturnConditionSlotRefinements", "ReturnPresenceRelations")
	}
	// MaySuspend is transitive through both signature and direct Relation calls.
	// Configure its authority once so repeated equation evaluation never mutates
	// the persistent Builder capability snapshot.
	summaryKinds = append(summaryKinds, "MaySuspend")
	for _, kind := range summaryKinds {
		for _, lane := range state.DefaultLanes() {
			_ = outputCaps.SetSummary(kind, lane, CapabilitySupported)
		}
	}
	descriptors, err := newCompilerDescriptorRegistry(plan.BoundaryReturns())
	if err != nil {
		return nil, fmt.Errorf("compiler: descriptors: %w", err)
	}
	builder := NewBuilderWithDescriptors(reg, shape, outputCaps, descriptors, plan)
	builder.inferReturnCorrelations = planReturnArity(plan) > 1
	ctx.builder = builder
	ctx.locals = make(map[symbol.ID]ValueTerm)
	ctx.expressions = make(map[factflow.ExprRef][]ValueTerm)
	ctx.allocationEffects = make(map[cfg.Point]EffectTerm)
	ctx.genericBindings = make(map[symbol.ID]symbolicGenericBinding)
	if err := bindBoundaryTerms(&ctx, shape); err != nil {
		return nil, fmt.Errorf("compiler: boundary: %w", err)
	}
	if err := bindStaticSignatureTerms(&ctx); err != nil {
		return nil, fmt.Errorf("compiler: signature calls: %w", err)
	}
	if err := preparePredicateExpressions(&ctx); err != nil {
		return nil, fmt.Errorf("compiler: predicate: %w", err)
	}
	if planHasFact(plan, operationplan.ExpressionFunction) && !declarations.Matches(plan) {
		return nil, fmt.Errorf("compiler: contextual operations: ExpressionFunctions (no complete direct lexical declaration proof)")
	}
	if unsupported := c.unsupportedActive(plan); len(unsupported) != 0 {
		return nil, fmt.Errorf("compiler: contextual operations: %s", strings.Join(unsupported, ", "))
	}
	semantic := DefaultSemanticCapabilityRegistry()
	for _, fact := range operationplan.Kinds() {
		if c.facts[fact] == nil || !planHasFact(plan, fact) {
			continue
		}
		for _, lane := range state.DefaultLanes() {
			capability := CapabilityUnaffected
			if dynamicCapability, handled := dynamicIndexEffectCapability(fact, lane); handled {
				capability = dynamicCapability
			} else if fact == operationplan.RootAssignment || isBranchEdgeOwnedKind(fact) {
				capability = CapabilitySupported
			}
			_ = semantic.SetFact(fact, lane, capability)
		}
		_ = semantic.SetFact(fact, state.LaneValues, CapabilitySupported)
	}
	for _, extension := range operationplan.ExtensionKinds() {
		if c.extensions[extension] == nil || !planHasExtension(plan, extension) {
			continue
		}
		for _, lane := range state.DefaultLanes() {
			_ = semantic.SetExtension(extension, lane, CapabilityUnaffected)
		}
		_ = semantic.SetExtension(extension, state.LaneValues, CapabilitySupported)
	}
	certificate, err := CertifyPlan(plan, semantic)
	if err != nil {
		return nil, fmt.Errorf("compiler: certificate: %w", err)
	}
	tape, err := compileSymbolicWTOTape(graph)
	if err != nil {
		return nil, fmt.Errorf("compiler: WTO topology: %w", err)
	}
	observationComplete := exactObservationCoverage(plan, shape, len(tape.components) != 0)
	var projectionPlan *sparseProjectionPlan
	var projectionPlanReason string
	if observationComplete {
		if requirements, sealed := plan.ObservationRequirements(); sealed {
			projectionPlan, err = compileSparseProjectionPlan(requirements)
			if err != nil {
				projectionPlanReason = err.Error()
			}
		} else {
			projectionPlanReason = "projection trace: observation requirements are not sealed"
		}
	}
	return &PreparedPlanCompiler{
		compiler: c, registry: reg, graph: graph, plan: plan, shape: shape,
		builder: builder, base: ctx, certificate: certificate, wtoTape: tape,
		observationComplete: observationComplete,
		projectionPlan:      projectionPlan, projectionPlanReason: projectionPlanReason,
		cyclic: len(tape.components) != 0,
	}, nil
}

// Evaluate executes the prepared no-dependency equation. It returns a
// contextual Relation on CFG/topology transfer failure and never publishes
// partial rows.
func (p *PreparedPlanCompiler) Evaluate() Relation {
	return p.evaluate(context.Background(), RelationView{}, nil)
}

// EvaluateDirect composes a frozen acyclic lexical dependency catalog. Empty,
// contextual, widened, foreign-shape, and undeclared dependency relations fail
// the complete caller relation closed.
func (p *PreparedPlanCompiler) EvaluateDirect(view RelationView, catalog DirectCallCatalog) Relation {
	if p == nil || catalog.PointCount() != p.plan.PointCount() {
		return Relation{contextual: "compiler: direct-call catalog width differs from plan", widened: true}
	}
	return p.evaluate(context.Background(), view, &catalog)
}

// DirectCompositionEligible reports whether this producer's row outputs can
// be substituted into a caller without a second semantic projection step.
// Declared return contracts are currently applied during specialization, so a
// caller-side symbolic projection is required before those producers can join
// the exact direct-composition slice.
func (p *PreparedPlanCompiler) DirectCompositionEligible() bool {
	if p == nil || p.builder == nil || p.builder.descriptors == nil {
		return false
	}
	handler, ok := p.builder.descriptors.handlers[DescriptorReturn].(returnHandler)
	return !ok || len(handler.declared) == 0
}

func (p *PreparedPlanCompiler) evaluate(evalCtx context.Context, view RelationView, direct *DirectCallCatalog) Relation {
	if p == nil || p.compiler == nil || p.builder == nil {
		return Relation{contextual: "compiler: nil prepared plan", widened: true}
	}
	contextual := func(reason string) Relation {
		return Relation{shape: p.shape, arena: p.builder.Arena(), contextual: reason, widened: true}
	}
	ctx := p.base
	ctx.directCalls = direct
	initial := SymbolicCFGRow{
		Guard: p.builder.Arena().True(), Values: ctx.locals, genericBindings: ctx.genericBindings,
		paramPreserved: newBoundaryPreservationLedger(p.shape.Params, p.shape.Captures),
	}
	if p.shape.Params != 0 {
		initial.Output.NormalReturnParams = make([]product.Value, p.shape.Params)
		for i := range initial.Output.NormalReturnParams {
			initial.Output.NormalReturnParams[i] = product.Top()
		}
	}
	annotations := relationAnnotations{}
	transfer := func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
		return p.lowerPreparedPointWithAnnotations(ctx, view, direct, point, row, &annotations)
	}
	branch := func(point cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
		return compileBranchEdge(ctx, point, row, cond)
	}
	var traceBuilder *sparseProjectionTraceBuilder
	traceReason := p.projectionPlanReason
	if p.observationComplete {
		if p.projectionPlan != nil {
			traceBuilder = p.projectionPlan.newBuilder(p.builder.Arena(), p.plan)
		}
	}
	exitRows, err := solveExactWTOCFGExpandedExitRowsWithTrace(evalCtx, p.graph, p.wtoTape, p.builder.Arena(), initial,
		transfer, branch, SymbolicExactWTOOptions{SymbolicCFGOptions: SymbolicCFGOptions{Shape: p.shape}}, traceBuilder)
	if err != nil {
		return contextual("compiler: " + err.Error())
	}
	rows := make([]Row, len(exitRows))
	for i, row := range exitRows {
		rows[i] = Row{
			Guard: row.Guard, Output: row.Output, Ops: row.Operations, Effects: row.Effects, Proofs: row.Proofs,
			Observations: row.Observations, observationObligations: row.observationObligations,
			PathRefinements: row.paramPreserved.certifiedRefinements(p.builder.Arena(), p.builder.EffectArena(), p.shape, row, p.plan.BoundaryParams(), p.plan.BoundaryCaptures()),
		}
	}
	relation, err := p.builder.Build(p.certificate, rows)
	if err != nil {
		return contextual("compiler: relation admission: " + err.Error())
	}
	aliases, exact := projectedReturnParamAliases(p.builder.Arena(), rows)
	if !exact || len(aliases) != 0 && !relation.authority.allowsSummary("ReturnParamPathAliases") {
		return contextual("compiler: relation admission: return parameter alias projection unsupported")
	}
	relation.projection = normalizeRelationProjection(p.registry, aliases)
	relation.annotations = unionRelationAnnotations(p.builder.Arena(), annotations)
	relation.projectionTraceReason = traceReason
	if traceBuilder != nil {
		traceBuilder.mergeRelationAnnotations(annotations)
		trace, traceErr := traceBuilder.freeze()
		if traceErr != nil {
			relation.projectionTraceReason = traceErr.Error()
		} else {
			relation.projectionTrace = trace
			relation.observationComplete, err = sparseProjectionTraceCoversObservations(evalCtx, p.builder.Arena(), trace, relation.annotations)
			if err != nil {
				return contextual("compiler: observation coverage canceled")
			}
		}
	}
	if direct != nil {
		for _, cell := range direct.Cells() {
			dependency, ok := view.Lookup(cell)
			// Bottom has no feasible callee row in this SCC round, so it
			// contributes no uncovered observation. Treat it as the neutral
			// element of this must-property until the relation grows.
			if !ok || !dependency.IsBottom() && !dependency.ObservationCoverageComplete() {
				relation.observationComplete = false
				break
			}
		}
	}
	if relation.projectionTrace != nil {
		if !relation.observationComplete {
			relation.projectionTraceReason = "projection trace: required observation evidence is incomplete"
		} else {
			relation.projectionTraceReason = ""
		}
	}
	return relation
}

// projectedReturnParamAliases preserves the concrete projector's syntactic
// may-alias contract across all return alternatives. This metadata is not a
// row effect: an unreachable return still contributes a conservative alias,
// while it must not manufacture an unconditional ReturnFlow.
func projectedReturnParamAliases(arena *Arena, rows []Row) ([]summary.ReturnParamPathAlias, bool) {
	var out []summary.ReturnParamPathAlias
	for _, row := range rows {
		for _, operation := range row.Ops {
			if operation.Descriptor != DescriptorReturn {
				continue
			}
			param, exact := arena.directParamRoot(operation.Value)
			if !exact {
				param, exact = arena.refinedParamRoot(operation.Value)
			}
			if !exact {
				continue
			}
			placeholder, ok := pathaddr.PlaceholderKeyFromPath(pathdom.NewPlaceholder(param))
			if !ok {
				return nil, false
			}
			out = append(out, summary.ReturnParamPathAlias{ReturnIndex: int(operation.Slot), Source: placeholder})
		}
	}
	return out, true
}

func (p *PreparedPlanCompiler) lowerPreparedPoint(base planCompileContext, view RelationView, direct *DirectCallCatalog, point cfg.Point, initial SymbolicCFGRow) ([]SymbolicCFGRow, error) {
	return p.lowerPreparedPointWithAnnotations(base, view, direct, point, initial, nil)
}

func (p *PreparedPlanCompiler) lowerPreparedPointWithAnnotations(base planCompileContext, view RelationView, direct *DirectCallCatalog, point cfg.Point, initial SymbolicCFGRow, annotations *relationAnnotations) ([]SymbolicCFGRow, error) {
	if site, ok := p.plan.Facts().CallSiteView(point); ok && !p.cyclic {
		for slot := 0; slot < site.ArgumentSourceCount(); slot++ {
			anchor, durable := p.plan.CallArgumentObservationAnchor(point, uint32(slot))
			if durable {
				initial.observationObligations = recordobservationObligation(initial.observationObligations, observationObligation{BodyOwner: p.plan.ObservationBody(), Anchor: anchor, Guard: initial.Guard})
			}
		}
	}
	rows := []SymbolicCFGRow{initial}
	cursor := p.plan.Cursor(point)
	for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
		handler := p.compiler.facts[cell.Kind()]
		next := make([]SymbolicCFGRow, 0, len(rows))
		for _, row := range rows {
			rowCtx := base
			rowCtx.locals = row.Values
			rowCtx.genericBindings = row.genericBindings
			rowCtx.rowEffects = &row.Effects
			rowCtx.rowOutput = &row.Output
			directTarget, isDirect := DirectCallTarget{}, false
			if cell.Kind() == operationplan.CallSite && direct != nil {
				directTarget, isDirect = direct.Lookup(point)
			}
			if !isDirect {
				row.paramPreserved.observeFact(rowCtx, point, cell.Kind())
			}
			if err := handler.Preflight(rowCtx, point); err != nil {
				return nil, fmt.Errorf("%s: %w", cell.Kind(), err)
			}
			if isDirect {
				callee, found := view.Lookup(directTarget.Cell)
				if !found || callee.ContextualReason() != "" || callee.Widened() || callee.Shape() != directTarget.Shape {
					return nil, fmt.Errorf("direct call: dependency %v is unresolved, widened, contextual, or foreign-shaped", directTarget.Cell)
				}
				site, found := rowCtx.facts.CallSiteView(point)
				if !found {
					return nil, fmt.Errorf("direct call: source point %d has no call-site fact", point)
				}
				sitePoint, hasPoint := site.Point()
				if !hasPoint || sitePoint != point {
					return nil, fmt.Errorf("direct call: call-site identity %d/%v differs from CFG point %d", sitePoint, hasPoint, point)
				}
				boundary, boundaryOwned := direct.Boundary(point)
				if !boundaryOwned {
					return nil, fmt.Errorf("direct call: dependency %v has no exact boundary order", directTarget.Cell)
				}
				bindings, err := exactDirectCallBindings(rowCtx, directTarget.Shape, boundary, site)
				if err != nil {
					return nil, fmt.Errorf("direct call: %w", err)
				}
				composed, err := composeDirectCallRowsTargeted(p.builder, p.shape, row, callee, bindings, site, directTarget, annotations)
				if err != nil {
					return nil, err
				}
				next = append(next, composed...)
				continue
			}
			if err := handler.Lower(rowCtx, point, &row.Operations); err != nil {
				return nil, fmt.Errorf("%s: %w", cell.Kind(), err)
			}
			row.Values = rowCtx.locals
			row.genericBindings = rowCtx.genericBindings
			next = append(next, row)
		}
		rows = next
	}
	// Extension cursors must be replayed independently for every alternative.
	for index := range rows {
		rowCtx := base
		rowCtx.locals = rows[index].Values
		rowCtx.genericBindings = rows[index].genericBindings
		rowCtx.rowEffects = &rows[index].Effects
		rowCtx.rowOutput = &rows[index].Output
		extensions := p.plan.ExtensionCursor(point)
		for cell, ok := extensions.Next(); ok; cell, ok = extensions.Next() {
			handler := p.compiler.extensions[cell.Kind()]
			rows[index].paramPreserved.observeExtension(cell.Kind())
			if err := handler.Preflight(rowCtx, point); err != nil {
				return nil, fmt.Errorf("extension %d: %w", cell.Kind(), err)
			}
			if err := handler.Lower(rowCtx, point, &rows[index].Operations); err != nil {
				return nil, fmt.Errorf("extension %d: %w", cell.Kind(), err)
			}
		}
		rows[index].Values = rowCtx.locals
		rows[index].genericBindings = rowCtx.genericBindings
		if assignment, ok := p.plan.Facts().RootAssignment(point); ok && !p.cyclic && !exactDirectLexicalDeclaration(rowCtx, assignment) {
			anchor, durable := p.plan.AssignmentObservationAnchor(point)
			if durable {
				rows[index].observationObligations = recordobservationObligation(rows[index].observationObligations, observationObligation{BodyOwner: p.plan.ObservationBody(), Anchor: anchor, Guard: rows[index].Guard})
			}
			value, present := rows[index].Values[assignment.TargetSymbol()]
			if !present {
				return nil, fmt.Errorf("observation: assignment target %d has no symbolic value", assignment.TargetSymbol())
			}
			if durable {
				// Facts exposes at most one RootAssignment per CFG point, and the
				// durable occurrence identifies that point, so no run-local symbol
				// is needed to distinguish assignment annotations.
				rows[index].Observations = recordObservationTerm(rows[index].Observations, ObservationTerm{BodyOwner: p.plan.ObservationBody(), Kind: ObservationAssignment, Anchor: anchor, Guard: rows[index].Guard, Actual: value})
			}
		}
		if site, ok := p.plan.Facts().CallSiteView(point); ok && !p.cyclic {
			for targetIndex := 0; targetIndex < site.ResultTargetCount(); targetIndex++ {
				target, found := site.ResultTargetAt(targetIndex)
				if !found || target.Kind() != factflow.CallResultTargetLocalAssignment || target.TargetSymbol() == 0 {
					continue
				}
				anchor, durable := p.plan.CallResultObservationAnchor(point, uint32(targetIndex))
				if durable {
					rows[index].observationObligations = recordobservationObligation(rows[index].observationObligations, observationObligation{BodyOwner: p.plan.ObservationBody(), Anchor: anchor, Guard: rows[index].Guard})
				}
				value, present := rows[index].Values[target.TargetSymbol()]
				if !present {
					// Evidence coverage is a separate, fail-closed authority. A
					// missing symbolic observation must not invalidate an otherwise
					// exact dormant relation; whole-owner admission remains false.
					continue
				}
				if durable {
					rows[index].Observations = recordObservationTerm(rows[index].Observations, ObservationTerm{BodyOwner: p.plan.ObservationBody(), Kind: ObservationCallResult, Anchor: anchor, Guard: rows[index].Guard, Slot: uint32(targetIndex), Actual: value})
				}
			}
		}
	}
	return rows, nil
}

func exactObservationCoverage(plan *operationplan.Plan, shape Shape, cyclic bool) bool {
	if plan == nil || cyclic {
		return false
	}
	if len(plan.BoundaryParamContracts()) != int(shape.Params) {
		return false
	}
	requirements, sealed := plan.ObservationRequirements()
	if !sealed {
		return false
	}
	callArguments := make(map[cfg.Point]struct{})
	routes := make(map[cfg.Point]struct{})
	cursor := requirements.Cursor(false)
	for requirement, ok := cursor.Next(); ok; requirement, ok = cursor.Next() {
		if requirement.Stage() == operationplan.RequirementRoute {
			if requirement.Projection() != operationplan.ProjectionObservationCallInvocation {
				return false
			}
			routes[requirement.Point()] = struct{}{}
			continue
		}
		if requirement.Stage() != operationplan.RequirementObservation {
			continue
		}
		switch requirement.Projection() {
		case operationplan.ProjectionObservationAssignment, operationplan.ProjectionObservationCallResult:
		case operationplan.ProjectionObservationCallArgument:
			callArguments[requirement.Point()] = struct{}{}
		default:
			return false
		}
	}
	for point := range callArguments {
		if _, ok := routes[point]; !ok {
			return false
		}
	}
	return true
}

func exactDirectCallBindings(ctx planCompileContext, shape Shape, boundary DirectCallBoundary, site factflow.CallSiteView) (DirectCallBindings, error) {
	if shape.Results != 0 || shape.HeapTemplates != 0 {
		return DirectCallBindings{}, fmt.Errorf("callee result or heap-template boundary is contextual")
	}
	if len(boundary.Captures) != int(shape.Captures) || len(boundary.Globals) != int(shape.Globals) {
		return DirectCallBindings{}, fmt.Errorf("callee capture/global boundary order differs from shape")
	}
	out := DirectCallBindings{Values: make([]ValueTerm, 0, shape.ValueCount()), Paths: make([]PathTerm, 0, shape.ValueCount())}
	if receiver, ok := site.ReceiverSource(); ok {
		value, path, err := exactDirectCallSourceBinding(ctx, receiver)
		if err != nil {
			return DirectCallBindings{}, fmt.Errorf("receiver: %w", err)
		}
		out.Values, out.Paths = append(out.Values, value), append(out.Paths, path)
	} else if receiverPath, ok := site.ReceiverPath(); ok {
		binding, err := exactBoundaryPathBinding(ctx, receiverPath)
		if err != nil {
			return DirectCallBindings{}, fmt.Errorf("receiver path: %w", err)
		}
		value, path, err := ctx.builder.Arena().LowerBoundaryPathValue(receiverPath, binding)
		if err != nil {
			return DirectCallBindings{}, fmt.Errorf("receiver path: %w", err)
		}
		out.Values, out.Paths = append(out.Values, value), append(out.Paths, path)
	}
	sources := make([]factflow.ValueSource, 0, site.ArgumentSourceCount())
	site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
		sources = append(sources, source)
		return true
	})
	if len(out.Values)+len(sources) != int(shape.Params) {
		return DirectCallBindings{}, fmt.Errorf("argument width %d differs from callee params %d", len(out.Values)+len(sources), shape.Params)
	}
	for i, source := range sources {
		value, path, err := exactDirectCallSourceBinding(ctx, source)
		if err != nil {
			return DirectCallBindings{}, fmt.Errorf("argument %d: %w", i, err)
		}
		out.Values, out.Paths = append(out.Values, value), append(out.Paths, path)
	}
	for _, capture := range boundary.Captures {
		value, path, err := exactDirectCallLexicalBinding(ctx, capture)
		if err != nil {
			return DirectCallBindings{}, fmt.Errorf("capture %d: %w", capture, err)
		}
		out.Values, out.Paths = append(out.Values, value), append(out.Paths, path)
	}
	for _, global := range boundary.Globals {
		value, path, err := exactDirectCallLexicalBinding(ctx, global)
		if err != nil {
			return DirectCallBindings{}, fmt.Errorf("global %d: %w", global, err)
		}
		out.Values, out.Paths = append(out.Values, value), append(out.Paths, path)
	}
	return out, nil
}

func exactDirectCallLexicalBinding(ctx planCompileContext, target symbol.ID) (ValueTerm, PathTerm, error) {
	value, ok := ctx.locals[target]
	if !ok || value == 0 {
		return 0, 0, fmt.Errorf("caller has no exact lexical value term")
	}
	binding, err := exactBoundaryPathBinding(ctx, pathdom.NewPath(target, ""))
	if err != nil {
		// A caller-local immutable capture has an exact value term without being a
		// caller boundary root. Value-only child use remains compositional; a child
		// path term will reject transactionally during DAG rebasing.
		return value, 0, nil
	}
	return value, ctx.builder.Arena().Path(binding.Root), nil
}

func exactDirectCallSourceBinding(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, PathTerm, error) {
	if source.OpenTail || source.Expanded {
		return 0, 0, fmt.Errorf("source has open or expanded shape")
	}
	if source.Adjusted && !exactAdjustedStaticMemberSource(ctx, source) {
		return 0, 0, fmt.Errorf("adjusted source is not a proven scalar member projection")
	}
	value, err := exactCompilerSourceTerm(ctx, source)
	if err != nil {
		return 0, 0, fmt.Errorf("value: %w", err)
	}
	path, err := exactDirectCallSourcePath(ctx, source)
	if err != nil {
		// Path bindings are optional. Value-only callee terms can compose without
		// importing caller path identity; RebaseTermDAGs fails closed if the
		// callee actually references a missing PathTerm.
		path = 0
	}
	return value, path, nil
}

func exactAdjustedStaticMemberSource(ctx planCompileContext, source factflow.ValueSource) bool {
	if !source.Valid() || source.Kind != factflow.ValueSourceExpression || !source.HasExpr || source.OpenTail || source.Expanded || source.ResultIndex != 0 {
		return false
	}
	p, ok := ctx.facts.ExpressionPathRef(source.ExprRef)
	if !ok || p.Symbol == 0 || p.Version != 0 || len(p.Segments) == 0 {
		return false
	}
	owner, ok := ctx.locals[p.Symbol]
	if !ok || !iteratorProjectionDerived(ctx.builder.Arena(), owner) {
		return false
	}
	for _, member := range p.Segments {
		next := ctx.builder.Arena().StaticIndexValue(owner, member)
		if next == 0 {
			return false
		}
		owner = next
	}
	return true
}

func iteratorProjectionDerived(arena *Arena, term ValueTerm) bool {
	if arena == nil || term == 0 || int(term) >= len(arena.values) {
		return false
	}
	return arena.values[term].op == valueIteratorProjection
}

func exactDirectCallSourcePath(ctx planCompileContext, source factflow.ValueSource) (PathTerm, error) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		p, ok := ctx.facts.ExpressionPathRef(source.ExprRef)
		if !ok {
			return 0, fmt.Errorf("expression has no canonical path")
		}
		if len(p.Segments) != 0 {
			binding, err := exactBoundaryPathBinding(ctx, p)
			if err != nil {
				return 0, err
			}
			_, term, err := ctx.builder.Arena().LowerBoundaryPathValue(p, binding)
			return term, err
		}
	}
	_, _, path, err := boundaryParamSourceTerms(ctx, source)
	return path, err
}

// Equation publishes this prepared compiler through the persistent lexical
// RelationCell lifecycle. The base tranche has no direct dependencies.
func (p *PreparedPlanCompiler) Equation(ref CellRef) (*PreparedEquation, error) {
	if p == nil {
		return nil, fmt.Errorf("compiler: nil prepared plan")
	}
	return NewPreparedEquation(ref, p.builder, nil, func(ctx context.Context, _ RelationView, _ *Builder) (Relation, error) {
		if err := ctx.Err(); err != nil {
			return Relation{}, err
		}
		return p.evaluate(ctx, RelationView{}, nil), nil
	})
}

// DirectEquation publishes a direct-call equation. Recursive dependencies read
// owner-shaped Bottom during the first synchronous SCC round; composition then
// contributes zero successor rows until a base-case relation grows.
func (p *PreparedPlanCompiler) DirectEquation(ref CellRef, catalog DirectCallCatalog) (*PreparedEquation, error) {
	if p == nil || catalog.PointCount() != p.plan.PointCount() {
		return nil, fmt.Errorf("compiler: direct-call catalog width differs from plan")
	}
	dependencies := catalog.Cells()
	return NewPreparedEquation(ref, p.builder, dependencies, func(ctx context.Context, view RelationView, _ *Builder) (Relation, error) {
		if err := ctx.Err(); err != nil {
			return Relation{}, err
		}
		return p.evaluate(ctx, view, &catalog), nil
	})
}

// compilePrepared preserves the legacy fail-closed Relation result rather than
// exposing preparation errors through Compile's historical API.
func (c *PlanCompiler) compilePrepared(reg *axis.Registry, graph cfg.Graph, plan *operationplan.Plan, shape Shape) Relation {
	prepared, err := c.Prepare(reg, graph, plan, shape)
	if err != nil {
		return Relation{shape: shape, arena: NewArena(reg), contextual: err.Error(), widened: true}
	}
	return prepared.Evaluate()
}
