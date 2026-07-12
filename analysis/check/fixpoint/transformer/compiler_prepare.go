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
	compiler    *PlanCompiler
	registry    *axis.Registry
	graph       cfg.Graph
	plan        *operationplan.Plan
	shape       Shape
	builder     *Builder
	base        planCompileContext
	certificate SemanticCertificate
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
	summaryKinds := []callboundary.BoundaryFactKind{"NormalReturnParams", "NormalReturnFacts"}
	if planReturnArity(plan) > 1 {
		summaryKinds = append(summaryKinds, "ReturnConditionSlotRefinements", "ReturnPresenceRelations")
	}
	if planHasIteratorCall(plan) {
		summaryKinds = append(summaryKinds, "MaySuspend")
	}
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
	return &PreparedPlanCompiler{
		compiler: c, registry: reg, graph: graph, plan: plan, shape: shape,
		builder: builder, base: ctx, certificate: certificate,
	}, nil
}

// Evaluate executes the prepared no-dependency equation. It returns a
// contextual Relation on CFG/topology transfer failure and never publishes
// partial rows.
func (p *PreparedPlanCompiler) Evaluate() Relation {
	if p == nil || p.compiler == nil || p.builder == nil {
		return Relation{contextual: "compiler: nil prepared plan", widened: true}
	}
	contextual := func(reason string) Relation {
		return Relation{shape: p.shape, arena: p.builder.Arena(), contextual: reason, widened: true}
	}
	ctx := p.base
	initial := SymbolicCFGRow{
		Guard: p.builder.Arena().True(), Values: ctx.locals, genericBindings: ctx.genericBindings,
	}
	if p.shape.Params != 0 {
		initial.Output.NormalReturnParams = make([]product.Value, p.shape.Params)
		for i := range initial.Output.NormalReturnParams {
			initial.Output.NormalReturnParams[i] = product.Top()
		}
	}
	rowsByPoint, err := SolveAcyclicCFGRows(p.graph, p.builder.Arena(), initial,
		func(point cfg.Point, row SymbolicCFGRow) (SymbolicCFGRow, error) {
			rowCtx := ctx
			rowCtx.locals = row.Values
			rowCtx.genericBindings = row.genericBindings
			rowCtx.rowEffects = &row.Effects
			rowCtx.rowOutput = &row.Output
			cursor := p.plan.Cursor(point)
			for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
				handler := p.compiler.facts[cell.Kind()]
				if err := handler.Preflight(rowCtx, point); err != nil {
					return SymbolicCFGRow{}, fmt.Errorf("%s: %w", cell.Kind(), err)
				}
				if err := handler.Lower(rowCtx, point, &row.Operations); err != nil {
					return SymbolicCFGRow{}, fmt.Errorf("%s: %w", cell.Kind(), err)
				}
			}
			extensions := p.plan.ExtensionCursor(point)
			for cell, ok := extensions.Next(); ok; cell, ok = extensions.Next() {
				handler := p.compiler.extensions[cell.Kind()]
				if err := handler.Preflight(rowCtx, point); err != nil {
					return SymbolicCFGRow{}, fmt.Errorf("extension %d: %w", cell.Kind(), err)
				}
				if err := handler.Lower(rowCtx, point, &row.Operations); err != nil {
					return SymbolicCFGRow{}, fmt.Errorf("extension %d: %w", cell.Kind(), err)
				}
			}
			row.Values = rowCtx.locals
			row.genericBindings = rowCtx.genericBindings
			return row, nil
		},
		func(point cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
			return compileBranchEdge(ctx, point, row, cond)
		},
		SymbolicCFGOptions{Shape: p.shape},
	)
	if err != nil {
		return contextual("compiler: " + err.Error())
	}
	exitRows := rowsByPoint[p.graph.Exit()]
	rows := make([]Row, len(exitRows))
	for i, row := range exitRows {
		rows[i] = Row{Guard: row.Guard, Output: row.Output, Ops: row.Operations, Effects: row.Effects, Proofs: row.Proofs}
	}
	relation, err := p.builder.Build(p.certificate, rows)
	if err != nil {
		return contextual("compiler: relation admission: " + err.Error())
	}
	return relation
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
		return p.Evaluate(), nil
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
