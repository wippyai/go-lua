package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
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
	ctx := planCompileContext{
		registry: reg, graph: graph, plan: plan, facts: plan.Facts(),
		builder: NewBuilder(reg, shape, DefaultOutputCapabilityRegistry(), plan),
		locals:  make(map[symbol.ID]ValueTerm), genericBindings: make(map[symbol.ID]symbolicGenericBinding),
	}
	if err := bindBoundaryParamTerms(&ctx, shape); err != nil {
		return []PlanEligibilityEntry{{Family: "compiler", Reason: "boundary: " + err.Error()}}
	}
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
		cursor := plan.Cursor(point)
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			handler := c.facts[cell.Kind()]
			entry := PlanEligibilityEntry{Point: point, Family: cell.Kind().String()}
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
