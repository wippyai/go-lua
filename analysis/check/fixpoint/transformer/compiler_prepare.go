package transformer

import (
	"context"
	"fmt"
	"strings"

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
	compiler            *PlanCompiler
	registry            *axis.Registry
	graph               cfg.Graph
	plan                *operationplan.Plan
	shape               Shape
	builder             *Builder
	base                planCompileContext
	certificate         SemanticCertificate
	wtoTape             *symbolicWTOTape
	observationComplete bool
	// cyclic is retained as prepared topology metadata for compatibility and
	// diagnostics. Evaluation deliberately does not branch on it: DAGs are the
	// zero-component case of the same exact dense executor.
	cyclic bool
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
	if c == nil || reg == nil || graph == nil || plan == nil {
		return nil, fmt.Errorf("compiler: registry, graph, plan, and compiler are required")
	}
	if graph.Size() != plan.PointCount() {
		return nil, fmt.Errorf("compiler: graph points %d != operation rows %d", graph.Size(), plan.PointCount())
	}
	ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts()}
	outputCaps := DefaultOutputCapabilityRegistry()
	summaryKinds := []callboundary.BoundaryFactKind{
		"NormalReturnParams", "NormalReturnFacts", "ReturnFlows", "ReturnParamPathAliases",
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
	descriptors, err := NewDescriptorRegistry(returnHandler{declared: plan.BoundaryReturns()}, obligationHandler{})
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
	if err := bindBoundaryParamTerms(&ctx, shape); err != nil {
		return nil, fmt.Errorf("compiler: boundary: %w", err)
	}
	if err := bindStaticSignatureTerms(&ctx); err != nil {
		return nil, fmt.Errorf("compiler: signature calls: %w", err)
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
	return &PreparedPlanCompiler{
		compiler: c, registry: reg, graph: graph, plan: plan, shape: shape,
		builder: builder, base: ctx, certificate: certificate, wtoTape: tape,
		observationComplete: exactObservationCoverage(plan, shape),
		cyclic:              len(tape.components) != 0,
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
	transfer := func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
		return p.lowerPreparedPoint(ctx, view, direct, point, row)
	}
	branch := func(point cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
		return compileBranchEdge(ctx, point, row, cond)
	}
	exitRows, err := solveExactWTOCFGExpandedExitRowsWithTape(evalCtx, p.graph, p.wtoTape, p.builder.Arena(), initial,
		transfer, branch, SymbolicExactWTOOptions{SymbolicCFGOptions: SymbolicCFGOptions{Shape: p.shape}})
	if err != nil {
		return contextual("compiler: " + err.Error())
	}
	rows := make([]Row, len(exitRows))
	for i, row := range exitRows {
		rows[i] = Row{
			Guard: row.Guard, Output: row.Output, Ops: row.Operations, Effects: row.Effects, Proofs: row.Proofs,
			Observations:    row.Observations,
			PathRefinements: row.paramPreserved.certifiedRefinements(p.builder.Arena(), p.builder.EffectArena(), p.shape, row, p.plan.BoundaryParams(), p.plan.BoundaryCaptures()),
		}
	}
	relation, err := p.builder.Build(p.certificate, rows)
	if err != nil {
		return contextual("compiler: relation admission: " + err.Error())
	}
	relation.observationComplete = p.observationComplete
	if direct != nil {
		for _, cell := range direct.Cells() {
			dependency, ok := view.Lookup(cell)
			if !ok || !dependency.ObservationCoverageComplete() {
				relation.observationComplete = false
				break
			}
		}
	}
	return relation
}

func (p *PreparedPlanCompiler) lowerPreparedPoint(base planCompileContext, view RelationView, direct *DirectCallCatalog, point cfg.Point, initial SymbolicCFGRow) ([]SymbolicCFGRow, error) {
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
				if !found || callee.ContextualReason() != "" || callee.Widened() || callee.Rows() == 0 || callee.Shape() != directTarget.Shape {
					return nil, fmt.Errorf("direct call: dependency %v is unresolved, widened, contextual, or foreign-shaped", directTarget.Cell)
				}
				site, _ := rowCtx.facts.CallSiteView(point)
				bindings, err := exactDirectCallBindings(rowCtx, directTarget.Shape, site)
				if err != nil {
					return nil, fmt.Errorf("direct call: %w", err)
				}
				composed, err := ComposeDirectCallRows(p.builder, p.shape, row, callee, bindings, site, 256)
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
		if assignment, ok := p.plan.Facts().RootAssignment(point); ok && !p.cyclic {
			value, present := rows[index].Values[assignment.TargetSymbol()]
			if !present {
				return nil, fmt.Errorf("observation: assignment target %d has no symbolic value", assignment.TargetSymbol())
			}
			anchor := factflow.ExprRef(0)
			if source := assignment.Source(); source.HasExpr {
				anchor = source.ExprRef
			}
			rows[index].Observations = recordObservationTerm(rows[index].Observations, ObservationTerm{Kind: ObservationAssignment, Point: point, Anchor: anchor, Guard: rows[index].Guard, Symbol: assignment.TargetSymbol(), Actual: value})
		}
		if site, ok := p.plan.Facts().CallSiteView(point); ok && !p.cyclic {
			for targetIndex := 0; targetIndex < site.ResultTargetCount(); targetIndex++ {
				target, found := site.ResultTargetAt(targetIndex)
				if !found || target.Kind() != factflow.CallResultTargetLocalAssignment || target.TargetSymbol() == 0 {
					continue
				}
				value, present := rows[index].Values[target.TargetSymbol()]
				if !present {
					return nil, fmt.Errorf("observation: call result target %d has no symbolic value", target.TargetSymbol())
				}
				anchor, _ := site.Expr()
				rows[index].Observations = recordObservationTerm(rows[index].Observations, ObservationTerm{Kind: ObservationCallResult, Point: point, Anchor: anchor, Guard: rows[index].Guard, Symbol: target.TargetSymbol(), Actual: value})
			}
		}
	}
	return rows, nil
}

func exactObservationCoverage(plan *operationplan.Plan, shape Shape) bool {
	if plan == nil {
		return false
	}
	if len(plan.BoundaryParamContracts()) != int(shape.Params) {
		return false
	}
	facts := plan.Facts()
	for raw := 0; raw < plan.PointCount(); raw++ {
		site, ok := facts.CallSiteView(cfg.Point(raw))
		if !ok {
			continue
		}
		for i := 0; i < site.ResultTargetCount(); i++ {
			target, found := site.ResultTargetAt(i)
			if !found || target.Kind() != factflow.CallResultTargetLocalAssignment || target.TargetSymbol() == 0 || target.TargetPathEmpty() || target.TargetPathSegmentCount() != 0 {
				return false
			}
		}
	}
	// The term vocabulary currently covers assignment/call occurrences only.
	// Until operationplan publishes a closed diagnostic-family requirement
	// certificate (including abnormal terminals), whole-owner completeness must
	// remain false even when every represented occurrence is exact.
	return false
}

func exactDirectCallBindings(ctx planCompileContext, shape Shape, site factflow.CallSiteView) (DirectCallBindings, error) {
	if shape.Captures != 0 || shape.Globals != 0 || shape.Results != 0 || shape.HeapTemplates != 0 {
		return DirectCallBindings{}, fmt.Errorf("non-parameter callee boundary")
	}
	out := DirectCallBindings{Values: make([]ValueTerm, 0, shape.Params), Paths: make([]PathTerm, 0, shape.Params)}
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
	return out, nil
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
		return p.evaluate(ctx, RelationView{}, nil).withObservationOwner(ref), nil
	})
}

// DirectEquation publishes an acyclic direct-call equation. A self edge is
// rejected at preparation; longer cycles encounter unresolved Bottom
// dependencies during synchronous SCC evaluation and become contextual Top.
func (p *PreparedPlanCompiler) DirectEquation(ref CellRef, catalog DirectCallCatalog) (*PreparedEquation, error) {
	if p == nil || catalog.PointCount() != p.plan.PointCount() {
		return nil, fmt.Errorf("compiler: direct-call catalog width differs from plan")
	}
	dependencies := catalog.Cells()
	for _, dependency := range dependencies {
		if dependency == ref {
			return nil, fmt.Errorf("compiler: recursive direct relation cell %v requires contextual solver", ref)
		}
	}
	return NewPreparedEquation(ref, p.builder, dependencies, func(ctx context.Context, view RelationView, _ *Builder) (Relation, error) {
		if err := ctx.Err(); err != nil {
			return Relation{}, err
		}
		return p.evaluate(ctx, view, &catalog).withObservationOwner(ref), nil
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
