package transformer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// PlanCompiler lowers the certified, immutable operation plan into a symbolic
// relation. It is deliberately inactive: callers must explicitly compile and
// specialize the result, and production call routing does not consult it.
//
// Every operation-plan catalog entry owns a registration. A nil handler is an
// explicit contextual verdict, not an omitted case. This keeps FactsInput and
// extension growth fail-closed without a second semantic catalog.
type PlanCompiler struct {
	facts      map[operationplan.Kind]planKindHandler
	extensions map[operationplan.ExtensionKind]planExtensionHandler
}

type planCompileContext struct {
	registry *axis.Registry
	graph    cfg.Graph
	plan     *operationplan.Plan
	facts    factflow.Facts
	builder  *Builder
}

type planKindHandler interface {
	Kind() operationplan.Kind
	Preflight(planCompileContext, cfg.Point) error
	Lower(planCompileContext, cfg.Point, *[]Operation) error
}

type planExtensionHandler interface {
	Kind() operationplan.ExtensionKind
	Preflight(planCompileContext, cfg.Point) error
	Lower(planCompileContext, cfg.Point, *[]Operation) error
}

// NewPlanCompiler returns the smallest exact production compiler slice. The
// Return handler accepts only scalar sources whose meaning is already fully
// represented by factflow's canonical literal or ExpressionValue payload.
func NewPlanCompiler() *PlanCompiler {
	c := &PlanCompiler{
		facts:      make(map[operationplan.Kind]planKindHandler, len(operationplan.Kinds())),
		extensions: make(map[operationplan.ExtensionKind]planExtensionHandler, len(operationplan.ExtensionKinds())),
	}
	for _, fact := range operationplan.Kinds() {
		c.facts[fact] = nil
	}
	for _, extension := range operationplan.ExtensionKinds() {
		c.extensions[extension] = nil
	}
	// ExpressionValue is a dependency consumed by returnHandler. It has no
	// executable lowering of its own; its registration makes lane certificate
	// and payload ownership explicit.
	c.facts[operationplan.ExpressionValue] = expressionValuePlanHandler{}
	c.facts[operationplan.Return] = returnPlanHandler{}
	return c
}

// Compile atomically returns either a complete relation or one contextual
// relation naming every unsupported active family. No partial rows escape.
func (c *PlanCompiler) Compile(reg *axis.Registry, graph cfg.Graph, plan *operationplan.Plan, shape Shape) Relation {
	arena := NewArena(reg)
	contextual := func(reason string) Relation {
		return Relation{shape: shape, arena: arena, contextual: reason, widened: true}
	}
	if c == nil || reg == nil || graph == nil || plan == nil {
		return contextual("compiler: registry, graph, plan, and compiler are required")
	}
	if graph.Size() != plan.PointCount() {
		return contextual(fmt.Sprintf("compiler: graph points %d != operation rows %d", graph.Size(), plan.PointCount()))
	}
	ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts()}
	unsupported := c.unsupportedActive(plan)
	if len(unsupported) != 0 {
		return contextual("compiler: contextual operations: " + strings.Join(unsupported, ", "))
	}
	if reason := directRelationTopologyReason(graph); reason != "" {
		return contextual(reason)
	}
	if reason := c.preflight(ctx); reason != "" {
		return contextual(reason)
	}

	semantic := DefaultSemanticCapabilityRegistry()
	for _, fact := range operationplan.Kinds() {
		if c.facts[fact] == nil || !planHasFact(plan, fact) {
			continue
		}
		for _, lane := range state.DefaultLanes() {
			_ = semantic.SetFact(fact, lane, CapabilityUnaffected)
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
		return contextual("compiler: certificate: " + err.Error())
	}
	builder := NewBuilder(reg, shape, DefaultOutputCapabilityRegistry(), plan)
	ctx.builder = builder
	var operations []Operation
	for point := 0; point < plan.PointCount(); point++ {
		cursor := plan.Cursor(cfg.Point(point))
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			if err := c.facts[cell.Kind()].Lower(ctx, cfg.Point(point), &operations); err != nil {
				return contextual(fmt.Sprintf("compiler: %s at point %d: %v", cell.Kind(), point, err))
			}
		}
		extensions := plan.ExtensionCursor(cfg.Point(point))
		for cell, ok := extensions.Next(); ok; cell, ok = extensions.Next() {
			if err := c.extensions[cell.Kind()].Lower(ctx, cfg.Point(point), &operations); err != nil {
				return contextual(fmt.Sprintf("compiler: extension %d at point %d: %v", cell.Kind(), point, err))
			}
		}
	}
	relation, err := builder.Build(certificate, []Row{{Guard: builder.Arena().True(), Ops: operations}})
	if err != nil {
		return contextual("compiler: relation admission: " + err.Error())
	}
	return relation
}

func (c *PlanCompiler) unsupportedActive(plan *operationplan.Plan) []string {
	var out []string
	dependencies := plan.DependencyCursor()
	for fact, ok := dependencies.Next(); ok; fact, ok = dependencies.Next() {
		if c.facts[fact] == nil {
			out = append(out, fact.String())
		}
	}
	seen := make(map[operationplan.Kind]bool)
	seenExtension := make(map[operationplan.ExtensionKind]bool)
	for point := 0; point < plan.PointCount(); point++ {
		cursor := plan.Cursor(cfg.Point(point))
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			if c.facts[cell.Kind()] == nil && !seen[cell.Kind()] {
				seen[cell.Kind()] = true
				out = append(out, cell.Kind().String())
			}
		}
		extensions := plan.ExtensionCursor(cfg.Point(point))
		for cell, ok := extensions.Next(); ok; cell, ok = extensions.Next() {
			if c.extensions[cell.Kind()] == nil && !seenExtension[cell.Kind()] {
				seenExtension[cell.Kind()] = true
				out = append(out, fmt.Sprintf("extension:%d", cell.Kind()))
			}
		}
	}
	sort.Strings(out)
	return out
}

func (c *PlanCompiler) preflight(ctx planCompileContext) string {
	for point := 0; point < ctx.plan.PointCount(); point++ {
		cursor := ctx.plan.Cursor(cfg.Point(point))
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			if err := c.facts[cell.Kind()].Preflight(ctx, cfg.Point(point)); err != nil {
				return fmt.Sprintf("compiler: %s at point %d: %v", cell.Kind(), point, err)
			}
		}
	}
	return ""
}

func directRelationTopologyReason(graph cfg.Graph) string {
	rpo := graph.RPO()
	if len(rpo) != graph.Size() {
		return "compiler: unreachable CFG points require contextual solving"
	}
	seen := make(map[cfg.Point]bool, len(rpo))
	point := graph.Entry()
	for {
		if seen[point] {
			return "compiler: cyclic CFG requires relational SCC lowering"
		}
		seen[point] = true
		if point == graph.Exit() {
			break
		}
		successors := cfg.SuccessorsReadOnly(graph, point)
		if len(successors) != 1 {
			return "compiler: branching CFG requires guard lowering"
		}
		point = successors[0]
	}
	if len(seen) != graph.Size() {
		return "compiler: non-linear CFG requires contextual solving"
	}
	return ""
}

func planHasFact(plan *operationplan.Plan, target operationplan.Kind) bool {
	dependencies := plan.DependencyCursor()
	for fact, ok := dependencies.Next(); ok; fact, ok = dependencies.Next() {
		if fact == target {
			return true
		}
	}
	for point := 0; point < plan.PointCount(); point++ {
		cursor := plan.Cursor(cfg.Point(point))
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			if cell.Kind() == target {
				return true
			}
		}
	}
	return false
}

func planHasExtension(plan *operationplan.Plan, target operationplan.ExtensionKind) bool {
	for point := 0; point < plan.PointCount(); point++ {
		cursor := plan.ExtensionCursor(cfg.Point(point))
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			if cell.Kind() == target {
				return true
			}
		}
	}
	return false
}

type returnPlanHandler struct{}

func (returnPlanHandler) Kind() operationplan.Kind { return operationplan.Return }

type expressionValuePlanHandler struct{}

func (expressionValuePlanHandler) Kind() operationplan.Kind                      { return operationplan.ExpressionValue }
func (expressionValuePlanHandler) Preflight(planCompileContext, cfg.Point) error { return nil }
func (expressionValuePlanHandler) Lower(planCompileContext, cfg.Point, *[]Operation) error {
	return nil
}

func (returnPlanHandler) Preflight(ctx planCompileContext, point cfg.Point) error {
	if _, ok := ctx.facts.Return(point); !ok {
		// ExpressionValue is registered to this handler as a dependency and has
		// no point-local preflight.
		return nil
	}
	fact, _ := ctx.facts.Return(point)
	for _, source := range fact.Sources() {
		if _, err := exactReturnSourceValue(ctx.registry, ctx.facts, source); err != nil {
			return err
		}
	}
	return nil
}

func (returnPlanHandler) Lower(ctx planCompileContext, point cfg.Point, operations *[]Operation) error {
	fact, ok := ctx.facts.Return(point)
	if !ok {
		return nil
	}
	for i, source := range fact.Sources() {
		value, err := exactReturnSourceValue(ctx.registry, ctx.facts, source)
		if err != nil {
			return err
		}
		slot := source.TargetIndex
		if slot < 0 {
			slot = i
		}
		*operations = append(*operations, Operation{
			Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: uint32(slot),
			Value: ctx.builder.Arena().Constant(value),
		})
	}
	return nil
}

func exactReturnSourceValue(reg *axis.Registry, facts factflow.Facts, source factflow.ValueSource) (product.Value, error) {
	if !source.Valid() || source.Expanded || source.Adjusted || source.OpenTail {
		return product.Value{}, fmt.Errorf("non-scalar return source")
	}
	switch source.Kind {
	case factflow.ValueSourceUnknown:
		return product.Top(), nil
	case factflow.ValueSourceNil:
		return typevalue.Nil(reg), nil
	case factflow.ValueSourceLiteral:
		switch source.LiteralKind {
		case factflow.ValueSourceLiteralBool:
			return typevalue.LiteralBool(reg, source.Bool), nil
		case factflow.ValueSourceLiteralInteger:
			return typevalue.LiteralInt(reg, source.Int), nil
		case factflow.ValueSourceLiteralNumber:
			return typevalue.LiteralNumber(reg, source.Float), nil
		case factflow.ValueSourceLiteralString:
			return typevalue.LiteralString(reg, source.String), nil
		}
	case factflow.ValueSourceExpression:
		value, ok := facts.ExpressionValue(source.ExprRef)
		if !ok || expressionHasContextualSidecar(facts, source.ExprRef) || !scalarValue(reg, value) {
			break
		}
		return value, nil
	}
	return product.Value{}, fmt.Errorf("return source kind %d requires contextual source resolution", source.Kind)
}

func expressionHasContextualSidecar(facts factflow.Facts, ref factflow.ExprRef) bool {
	if _, ok := facts.ObjectLiteralView(ref); ok {
		return true
	}
	if _, ok := facts.ExpressionOperation(ref); ok {
		return true
	}
	if _, ok := facts.ExpressionFunction(ref); ok {
		return true
	}
	if _, ok := facts.ExpressionRefinement(ref); ok {
		return true
	}
	if _, ok := facts.ExpressionPathRef(ref); ok {
		return true
	}
	if _, ok := facts.DynamicIndexExpression(ref); ok {
		return true
	}
	if _, ok := facts.ExpressionCondition(ref); ok {
		return true
	}
	return false
}

func scalarValue(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Literal:
		return true
	default:
		return false
	}
}
