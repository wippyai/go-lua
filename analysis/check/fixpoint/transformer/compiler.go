package transformer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
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
	registry        *axis.Registry
	graph           cfg.Graph
	plan            *operationplan.Plan
	facts           factflow.Facts
	builder         *Builder
	locals          map[symbol.ID]ValueTerm
	genericBindings map[symbol.ID]symbolicGenericBinding
}

type symbolicGenericBinding struct {
	Transaction factapply.GenericForTransaction
	Iterator    iteration.Iterator
	Container   ValueTerm
	Projection  ValueTerm
	FirstTarget symbol.ID
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
	c.facts[operationplan.RootAssignment] = rootAssignmentPlanHandler{}
	c.facts[operationplan.Return] = returnPlanHandler{}
	c.facts[operationplan.CallSite] = signatureCallPlanHandler{}
	c.extensions[operationplan.BodyGenericFor] = genericForPlanHandler{}
	return c
}

type signatureCallPlanHandler struct{}

func (signatureCallPlanHandler) Kind() operationplan.Kind { return operationplan.CallSite }
func (signatureCallPlanHandler) Preflight(ctx planCompileContext, point cfg.Point) error {
	op, ok := ctx.plan.SignatureCallOperation(point)
	if !ok {
		return fmt.Errorf("signature call: resolved producer missing")
	}
	sig := op.Signature()
	iterator, ok := iteration.ActiveIterator(sig.Effect.Labels)
	if !ok || sig.Effect.Tail != nil || len(sig.Effect.Labels) != 1 || sig.OperationalEffects != nil {
		return fmt.Errorf("signature call: non-iterator effects require contextual composition")
	}
	for p := 0; p < ctx.plan.PointCount(); p++ {
		generic, exists := ctx.plan.GenericForOperation(cfg.Point(p))
		if !exists {
			continue
		}
		source := generic.Source()
		got, hasIterator := generic.Iterator()
		if source.Kind == operationplan.GenericForSourceCall && source.HasCallPoint && source.CallPoint == point && hasIterator && got == iterator {
			return nil
		}
	}
	return fmt.Errorf("signature call: iterator result is not owned by generic-for")
}
func (signatureCallPlanHandler) Lower(planCompileContext, cfg.Point, *[]Operation) error { return nil }

type genericForPlanHandler struct{}

func (genericForPlanHandler) Kind() operationplan.ExtensionKind { return operationplan.BodyGenericFor }
func (genericForPlanHandler) Preflight(ctx planCompileContext, point cfg.Point) error {
	_, err := lowerGenericForBinding(ctx, point, false)
	return err
}
func (genericForPlanHandler) Lower(ctx planCompileContext, point cfg.Point, _ *[]Operation) error {
	_, err := lowerGenericForBinding(ctx, point, true)
	return err
}

func lowerGenericForBinding(ctx planCompileContext, point cfg.Point, publish bool) (symbolicGenericBinding, error) {
	op, ok := ctx.plan.GenericForOperation(point)
	if !ok {
		return symbolicGenericBinding{}, fmt.Errorf("generic-for: typed operation payload missing")
	}
	transaction, ok := factapply.PlanGenericForTransaction(op)
	if !ok {
		return symbolicGenericBinding{}, fmt.Errorf("generic-for: invalid binding transaction")
	}
	source := op.Source()
	switch source.Kind {
	case operationplan.GenericForSourceCall:
		if !source.HasCallPoint {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator call point missing")
		}
		iterator, ok := op.Iterator()
		if !ok {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: canonical signature iterator missing")
		}
		site, ok := ctx.facts.CallSiteView(source.CallPoint)
		if !ok {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator call-site payload missing")
		}
		sourceIndex, ok := effect.ResolveParamIndex(iterator.Source, site.ArgumentSourceCount())
		if !ok {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator source parameter unresolved")
		}
		container, ok := site.ArgumentSourceAt(sourceIndex)
		if !ok {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator source argument missing")
		}
		term, err := exactCompilerSourceTerm(ctx, container)
		if err != nil {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator source: %w", err)
		}
		projection := ctx.builder.Arena().IteratorProjectionValue(iterator, op.VariableIndex(), term)
		if projection == 0 {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator projection unsupported")
		}
		binding := symbolicGenericBinding{Transaction: transaction, Iterator: iterator, Container: term, Projection: projection, FirstTarget: op.FirstTarget()}
		if publish {
			ctx.locals[op.Target()] = projection
			ctx.genericBindings[op.Target()] = binding
		}
		return binding, nil
	case operationplan.GenericForSourceExpression:
		if !source.HasRootPath || source.RootPath.Symbol == 0 || len(source.RootPath.Segments) != 0 {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator expression is not an exact root binding")
		}
		if _, ok := ctx.locals[source.RootPath.Symbol]; !ok {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator source symbol %d has no exact binding", source.RootPath.Symbol)
		}
		return symbolicGenericBinding{}, fmt.Errorf("generic-for: custom iterator expression requires signature effect proof")
	default:
		return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator source unknown")
	}
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
	builder := NewBuilder(reg, shape, DefaultOutputCapabilityRegistry(), plan)
	ctx.builder = builder
	ctx.locals = make(map[symbol.ID]ValueTerm)
	ctx.genericBindings = make(map[symbol.ID]symbolicGenericBinding)
	if err := bindBoundaryParamTerms(&ctx, shape); err != nil {
		return contextual("compiler: boundary: " + err.Error())
	}
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
			capability := CapabilityUnaffected
			if fact == operationplan.RootAssignment {
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
		return contextual("compiler: certificate: " + err.Error())
	}
	var operations []Operation
	for _, point := range graph.RPO() {
		cursor := plan.Cursor(point)
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			if err := c.facts[cell.Kind()].Lower(ctx, point, &operations); err != nil {
				return contextual(fmt.Sprintf("compiler: %s at point %d: %v", cell.Kind(), point, err))
			}
		}
		extensions := plan.ExtensionCursor(point)
		for cell, ok := extensions.Next(); ok; cell, ok = extensions.Next() {
			if err := c.extensions[cell.Kind()].Lower(ctx, point, &operations); err != nil {
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

func bindBoundaryParamTerms(ctx *planCompileContext, shape Shape) error {
	params := ctx.plan.BoundaryParams()
	if len(params) != int(shape.Params) {
		return fmt.Errorf("parameter symbols %d != shape params %d", len(params), shape.Params)
	}
	for index, param := range params {
		if _, exists := ctx.locals[param]; exists {
			return fmt.Errorf("duplicate parameter symbol %d", param)
		}
		ctx.locals[param] = ctx.builder.Arena().Root(Root{Kind: RootParam, Index: uint32(index)})
	}
	return nil
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
	for _, point := range ctx.graph.RPO() {
		cursor := ctx.plan.Cursor(point)
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			if err := c.facts[cell.Kind()].Preflight(ctx, point); err != nil {
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
		if source.Kind == factflow.ValueSourcePath {
			if _, version, suffix, ok := pathaddr.ParseResolverPath(source.PathKey); !ok || version != 0 || suffix != "" {
				return fmt.Errorf("non-root or non-canonical return path")
			}
			continue
		}
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
		term, err := exactReturnSourceTerm(ctx, source)
		if err != nil {
			return err
		}
		slot := source.TargetIndex
		if slot < 0 {
			slot = i
		}
		*operations = append(*operations, Operation{
			Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: uint32(slot),
			Value: term,
		})
	}
	return nil
}

func exactReturnSourceTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, error) {
	if source.Kind == factflow.ValueSourcePath {
		sym, version, suffix, ok := pathaddr.ParseResolverPath(source.PathKey)
		if !ok || version != 0 || suffix != "" || sym == 0 {
			return 0, fmt.Errorf("non-root or non-canonical return path")
		}
		term, ok := ctx.locals[sym]
		if !ok {
			return 0, fmt.Errorf("return path symbol %d has no exact local binding", sym)
		}
		return term, nil
	}
	value, err := exactReturnSourceValue(ctx.registry, ctx.facts, source)
	if err != nil {
		return 0, err
	}
	return ctx.builder.Arena().Constant(value), nil
}

func exactReturnSourceValue(reg *axis.Registry, facts factflow.Facts, source factflow.ValueSource) (product.Value, error) {
	if !source.Valid() || source.Expanded || source.Adjusted || source.OpenTail {
		return product.Value{}, fmt.Errorf("non-scalar return source")
	}
	if value, ok := sourcevalue.StaticScalarValue(reg, source); ok {
		return value, nil
	}
	switch source.Kind {
	case factflow.ValueSourceUnknown:
		return product.Top(), nil
	case factflow.ValueSourceExpression:
		value, ok := facts.ExpressionValue(source.ExprRef)
		if !ok || expressionHasContextualSidecar(facts, source.ExprRef) || !scalarValue(reg, value) {
			break
		}
		return value, nil
	}
	return product.Value{}, fmt.Errorf("return source kind %d requires contextual source resolution", source.Kind)
}

type rootAssignmentPlanHandler struct{}

func (rootAssignmentPlanHandler) Kind() operationplan.Kind { return operationplan.RootAssignment }
func (rootAssignmentPlanHandler) Preflight(ctx planCompileContext, point cfg.Point) error {
	fact, ok := ctx.facts.RootAssignment(point)
	if !ok {
		return fmt.Errorf("missing root-assignment payload")
	}
	if fact.Kind() != factflow.RootAssignmentLocalDeclaration || fact.TargetSymbol() == 0 {
		return fmt.Errorf("only local declarations have exact symbolic binding semantics")
	}
	target := fact.TargetPathRef()
	if target.Symbol != fact.TargetSymbol() || target.Version != 0 || len(target.Segments) != 0 {
		return fmt.Errorf("assignment target is not its canonical root symbol")
	}
	if _, ok := fact.DeclaredValue(); ok || fact.DeclaredValueContracts() || fact.DeclaredValueOverlays() {
		return fmt.Errorf("declared contracts and overlays require contextual root semantics")
	}
	if _, ok := fact.DeclaredAnnotationValue(); ok {
		return fmt.Errorf("annotated roots require contextual declaration semantics")
	}
	if fact.Source().Kind == factflow.ValueSourcePath {
		if _, version, suffix, ok := pathaddr.ParseResolverPath(fact.Source().PathKey); !ok || version != 0 || suffix != "" {
			return fmt.Errorf("assignment source path is not a canonical root symbol")
		}
		return nil
	}
	if _, err := exactReturnSourceValue(ctx.registry, ctx.facts, fact.Source()); err != nil {
		return fmt.Errorf("assignment source is not a context-independent scalar")
	}
	return nil
}
func (rootAssignmentPlanHandler) Lower(ctx planCompileContext, point cfg.Point, _ *[]Operation) error {
	fact, ok := ctx.facts.RootAssignment(point)
	if !ok {
		return fmt.Errorf("missing root-assignment payload")
	}
	if _, exists := ctx.locals[fact.TargetSymbol()]; exists {
		return fmt.Errorf("symbol %d has multiple writes", fact.TargetSymbol())
	}
	term, err := exactCompilerSourceTerm(ctx, fact.Source())
	if err != nil {
		return err
	}
	ctx.locals[fact.TargetSymbol()] = term
	return nil
}

func exactCompilerSourceTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, error) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if p, ok := ctx.facts.ExpressionPathRef(source.ExprRef); ok {
			if p.Symbol == 0 || p.Version != 0 || len(p.Segments) != 0 {
				return 0, fmt.Errorf("source expression path is not a canonical root symbol")
			}
			term, ok := ctx.locals[p.Symbol]
			if !ok {
				return 0, fmt.Errorf("source expression symbol %d has no exact binding", p.Symbol)
			}
			return term, nil
		}
	}
	if source.Kind == factflow.ValueSourcePath {
		sym, version, suffix, ok := pathaddr.ParseResolverPath(source.PathKey)
		if !ok || sym == 0 || version != 0 || suffix != "" {
			return 0, fmt.Errorf("source path is not a canonical root symbol")
		}
		term, ok := ctx.locals[sym]
		if !ok {
			return 0, fmt.Errorf("source path symbol %d has no exact local binding", sym)
		}
		return term, nil
	}
	value, err := exactReturnSourceValue(ctx.registry, ctx.facts, source)
	if err != nil {
		return 0, fmt.Errorf("assignment source is not a context-independent scalar")
	}
	return ctx.builder.Arena().Constant(value), nil
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
