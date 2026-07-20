package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// PlanEligibilityEntry is one deterministic compiler-admission verdict.
// Dependencies are family markers because operationplan deliberately retains
// their payloads in Facts rather than duplicating expression refs into rows.
type PlanEligibilityEntry struct {
	Point      cfg.Point
	Family     string
	Dependency bool
	Extension  bool
	Exact      bool
	Reason     string
}

// EligibilityCensus evaluates every active operation instance through the
// actual compiler handler. It carries exact local bindings in CFG order, but
// never builds or publishes a Relation and has no production routing effect.
func (c *PlanCompiler) EligibilityCensus(reg *axis.Registry, graph cfg.Graph, plan *operationplan.Plan, shape Shape) []PlanEligibilityEntry {
	if c == nil || reg == nil || graph == nil || plan == nil || graph.Size() != plan.PointCount() {
		return []PlanEligibilityEntry{{Family: "compiler", Reason: "registry, graph, plan, and matching rows are required"}}
	}
	builder := NewBuilder(reg, shape, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{
		registry: reg, graph: graph, plan: plan, facts: plan.Facts(),
		builder: builder,
		locals:  make(map[symbol.ID]ValueTerm), expressions: make(map[factflow.ExprRef][]ValueTerm), allocationEffects: make(map[cfg.Point]EffectTerm), genericBindings: make(map[symbol.ID]symbolicGenericBinding),
	}
	var rowSteps []rowStep
	var rowOutput summary.Summary
	ctx.rowSteps = &rowSteps
	ctx.rowOutput = &rowOutput
	if err := bindBoundaryParamTerms(&ctx, shape); err != nil {
		return []PlanEligibilityEntry{{Family: "compiler", Reason: "boundary: " + err.Error()}}
	}
	if err := bindChannelSelectResultTerms(&ctx); err != nil {
		return []PlanEligibilityEntry{{Family: "compiler", Reason: "channel select results: " + err.Error()}}
	}
	if err := bindStaticSignatureTerms(&ctx); err != nil {
		return []PlanEligibilityEntry{{Family: "compiler", Reason: "signature calls: " + err.Error()}}
	}
	// Production lowering executes against the one sealed State-backed
	// environment. The census must exercise the same term producers rather than
	// the retired row-local binding model, or exact N4/N5 instances are reported
	// contextual even though the structural freezer admits them.
	ctx.locals = make(map[symbol.ID]ValueTerm)
	for _, id := range sealedEnvironmentSymbols(plan) {
		term := builder.Arena().bindEnvironmentSymbol(id)
		if term == 0 {
			return []PlanEligibilityEntry{{Family: "compiler", Reason: fmt.Sprintf("environment symbol %d could not be sealed", id)}}
		}
		ctx.locals[id] = term
	}
	if err := bindStructuralExpressionTerms(builder, plan); err != nil {
		return []PlanEligibilityEntry{{Family: "compiler", Reason: "structural expressions: " + err.Error()}}
	}
	ctx.structuralEnvironment = true
	var out []PlanEligibilityEntry
	dependencies := plan.DependencyCursor()
	for family, ok := dependencies.Next(); ok; family, ok = dependencies.Next() {
		handler := c.facts[family]
		entry := PlanEligibilityEntry{Family: family.String(), Dependency: true, Exact: handler != nil}
		if handler == nil {
			entry.Reason = "unsupported dependency family"
		}
		out = append(out, entry)
	}
	var operations []Operation
	visited := make([]bool, plan.PointCount())
	order := append([]cfg.Point(nil), graph.RPO()...)
	for _, point := range order {
		if int(point) < len(visited) {
			visited[point] = true
		}
	}
	for point := 0; point < plan.PointCount(); point++ {
		if !visited[point] {
			order = append(order, cfg.Point(point))
		}
	}
	for _, point := range order {
		var rootAssignment rootAssignmentTerm
		var returnTransaction returnTransactionTerm
		var structuralOutput structuralOutputContribution
		ctx.point = point
		ctx.rootAssignment = &rootAssignment
		ctx.returnTransaction = &returnTransaction
		ctx.structuralOutput = &structuralOutput
		cursor := plan.Cursor(point)
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			handler := c.facts[cell.Kind()]
			entry := PlanEligibilityEntry{Point: point, Family: cell.Kind().String()}
			if isBranchEdgeOwnedKind(cell.Kind()) {
				entry.Reason = "exactness is owned by whole symbolic CFG edge compilation"
				out = append(out, entry)
				continue
			}
			var preflight error
			if handler != nil {
				preflight = handler.Preflight(ctx, point)
			}
			switch {
			case handler == nil:
				entry.Reason = "unsupported operation family"
			case preflight != nil:
				entry.Reason = preflight.Error()
			default:
				if err := handler.Lower(ctx, point, &operations); err != nil {
					entry.Reason = err.Error()
				} else {
					entry.Exact = true
				}
			}
			out = append(out, entry)
		}
		extensions := plan.ExtensionCursor(point)
		for cell, ok := extensions.Next(); ok; cell, ok = extensions.Next() {
			handler := c.extensions[cell.Kind()]
			entry := PlanEligibilityEntry{Point: point, Family: fmt.Sprintf("extension:%d", cell.Kind()), Extension: true}
			var preflight error
			if handler != nil {
				preflight = handler.Preflight(ctx, point)
			}
			switch {
			case handler == nil:
				entry.Reason = "unsupported extension family"
			case preflight != nil:
				entry.Reason = preflight.Error()
			default:
				if err := handler.Lower(ctx, point, &operations); err != nil {
					entry.Reason = err.Error()
				} else {
					entry.Exact = true
				}
			}
			out = append(out, entry)
		}
	}
	return out
}

func isBranchEdgeOwnedKind(kind operationplan.Kind) bool {
	switch kind {
	case operationplan.BranchEdgeReachability, operationplan.BranchConditionSource, operationplan.BranchRefinement, operationplan.BranchPathEvidence:
		return true
	default:
		return false
	}
}
