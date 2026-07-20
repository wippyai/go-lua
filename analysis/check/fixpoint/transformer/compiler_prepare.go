package transformer

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// PreparedPlanCompiler owns the persistent Builder, static semantic
// certificate, and one frozen WorldProgram for a lexical function.
type PreparedPlanCompiler struct {
	compiler       *PlanCompiler
	registry       *axis.Registry
	graph          cfg.Graph
	plan           *operationplan.Plan
	shape          Shape
	builder        *Builder
	base           planCompileContext
	certificate    SemanticCertificate
	wtoTape        *symbolicWTOTape
	worldBase      WorldProgram
	codeBase       *relationCode
	rootBase       relationRootRef
	freezeCount    uint32
	reductionCount uint32
	freezeMu       sync.Mutex
	frozen         bool
	frozenDirect   bool
	freezeErr      error
	effectFree     bool
	// cyclic is retained as prepared topology metadata for compatibility and
	// diagnostics. Evaluation deliberately does not branch on it: DAGs are the
	// zero-component case of the same exact dense executor.
	cyclic             bool
	environmentSymbols []symbol.ID
	ambientRoots       []AmbientRoot
}

// sealAmbientEnvironment extends the prepared callable schema with the exact
// closure-conversion roots computed for this lexical body.  Ambient roots are
// a first-class boundary namespace: they are never smuggled through captures,
// and Symbol/Mutable cannot become misaligned because they cross this API as
// one canonical value.
func (p *PreparedPlanCompiler) sealAmbientEnvironment(roots []AmbientRoot) error {
	if p == nil || p.builder == nil || p.builder.Arena() == nil || p.builder.Arena().Sealed() {
		return fmt.Errorf("compiler: ambient environment has no open term owner")
	}
	if !validAmbientRoots(roots) {
		return fmt.Errorf("compiler: ambient root inventory is not canonical")
	}
	if p.shape.Ambients != uint32(len(roots)) {
		return fmt.Errorf("compiler: ambient root width %d differs from prepared shape %d", len(roots), p.shape.Ambients)
	}
	owned := make(map[symbol.ID]struct{}, len(p.environmentSymbols)+len(roots))
	for _, id := range p.environmentSymbols {
		owned[id] = struct{}{}
	}
	for _, root := range roots {
		if _, exists := owned[root.Symbol]; !exists {
			if p.builder.Arena().bindEnvironmentSymbol(root.Symbol) == 0 {
				return fmt.Errorf("compiler: ambient environment symbol %d could not be sealed", root.Symbol)
			}
			owned[root.Symbol] = struct{}{}
			p.environmentSymbols = append(p.environmentSymbols, root.Symbol)
		}
	}
	sort.Slice(p.environmentSymbols, func(i, j int) bool { return p.environmentSymbols[i] < p.environmentSymbols[j] })
	p.ambientRoots = append([]AmbientRoot(nil), roots...)
	return nil
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
	return p != nil && p.effectFree
}

func (p *PreparedPlanCompiler) computeEffectFree() bool {
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

// Prepare validates all context-independent compiler inputs once.
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
	ctx := planCompileContext{
		registry: reg, graph: graph, plan: plan, facts: plan.Facts(), directDeclarations: declarations,
		allowLexicalBoundaryRoots: allowLexicalBoundaryRoots,
		expressionValueFreeze:     c.expressionValueFreeze, expressionValueState: c.expressionValueState,
		expressionValueResults: c.expressionValueResults,
	}
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
	ctx.externalResults = make(map[cfg.Point][]ValueTerm)
	ctx.genericBindings = make(map[symbol.ID]symbolicGenericBinding)
	if err := bindBoundaryTerms(&ctx, shape); err != nil {
		return nil, fmt.Errorf("compiler: boundary: %w", err)
	}
	environmentSymbols := sealedEnvironmentSymbols(plan)
	for _, id := range environmentSymbols {
		if builder.Arena().bindEnvironmentSymbol(id) == 0 {
			return nil, fmt.Errorf("compiler: environment symbol %d could not be sealed", id)
		}
	}
	if err := bindStructuralExpressionTerms(builder, plan); err != nil {
		return nil, fmt.Errorf("compiler: structural expressions: %w", err)
	}
	if err := bindChannelSelectResultTerms(&ctx); err != nil {
		return nil, fmt.Errorf("compiler: channel select results: %w", err)
	}
	if err := bindStaticSignatureTerms(&ctx); err != nil {
		return nil, fmt.Errorf("compiler: signature calls: %w", err)
	}
	if err := prepareCertifiedScalarExpressions(&ctx); err != nil {
		return nil, fmt.Errorf("compiler: predicate: %w", err)
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
			} else if pathStoreCapability, handled := pathStoreEffectCapability(fact, lane); handled {
				capability = pathStoreCapability
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
	prepared := &PreparedPlanCompiler{
		compiler: c, registry: reg, graph: graph, plan: plan, shape: shape,
		builder: builder, base: ctx, certificate: certificate, wtoTape: tape,
		cyclic:             len(tape.components) != 0,
		environmentSymbols: environmentSymbols,
	}
	prepared.effectFree = prepared.computeEffectFree()
	return prepared, nil
}

// bindStructuralExpressionTerms installs the environment cells written at the
// joins of short-circuit logical regions. Every compiler-facing preparation
// path uses this one binder so admission/census and production lowering see
// the same symbolic environment vocabulary.
func bindStructuralExpressionTerms(builder *Builder, plan *operationplan.Plan) error {
	if builder == nil || plan == nil {
		return fmt.Errorf("builder and plan are required")
	}
	var bindErr error
	plan.ForEachStructuralExpressionRegion(func(ref factflow.ExprRef, _ factflow.StructuralExpressionRegion) bool {
		operation, exact := plan.Facts().ExpressionOperation(ref)
		if !exact || operation.Kind() != factflow.ExpressionOperationBinary || operation.Op() != "and" && operation.Op() != "or" {
			bindErr = fmt.Errorf("logical expression %d has no exact binary operation", ref)
			return false
		}
		if builder.Arena().bindExpressionValue(ref) == 0 {
			bindErr = fmt.Errorf("logical expression %d could not bind its result cell", ref)
			return false
		}
		return true
	})
	return bindErr
}

func sealedEnvironmentSymbols(plan *operationplan.Plan) []symbol.ID {
	if plan == nil {
		return nil
	}
	set := make(map[symbol.ID]struct{})
	add := func(id symbol.ID) {
		if id != 0 {
			set[id] = struct{}{}
		}
	}
	for _, ids := range [][]symbol.ID{plan.BoundaryParams(), plan.BoundaryCaptures(), plan.BoundaryGlobals()} {
		for _, id := range ids {
			add(id)
		}
	}
	facts := plan.Facts()
	for raw := 0; raw < plan.PointCount(); raw++ {
		point := cfg.Point(raw)
		if generic, ok := plan.GenericForOperation(point); ok {
			add(generic.Target())
		}
		if assignment, ok := facts.RootAssignment(point); ok {
			add(assignment.TargetSymbol())
		}
		if site, ok := facts.CallSiteView(point); ok {
			// A finite lexical call is selected from the callee's current identity.
			// Seal that lexical root even when it is read-only (and therefore has no
			// assignment/result lane of its own) so guarded dispatch reads the same
			// canonical environment namespace as ordinary path semantics.
			add(site.CalleePathRef().Symbol)
			for index := 0; index < site.ResultTargetCount(); index++ {
				if target, ok := site.ResultTargetAt(index); ok {
					add(target.TargetSymbol())
				}
			}
		}
	}
	out := make([]symbol.ID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func arenaTrue(arena *Arena) Guard {
	if arena == nil {
		return 0
	}
	return arena.True()
}

// frozenRelation returns the canonical per-body relation produced by the sole
// RelationProgram freeze. It performs no preparation, solve, or publication.
func (p *PreparedPlanCompiler) frozenRelation() Relation {
	if p == nil || p.builder == nil {
		return Relation{contextual: "compiler: nil prepared plan", widened: true}
	}
	relation := p.builder.bottomRelation()
	if p.codeBase == nil || p.rootBase == 0 || p.reductionCount != 1 || !p.codeBase.valid(p.rootBase) {
		return Relation{shape: p.shape, arena: p.builder.Arena(), contextual: "compiler: relation code was not sealed exactly once", widened: true}
	}
	relation.code = p.codeBase
	relation.root = p.rootBase
	return relation
}

func exactDirectCallBindings(ctx planCompileContext, shape Shape, boundary DirectCallBoundary, site factflow.CallSiteView) (DirectCallBindings, error) {
	if len(boundary.Captures) != int(shape.Captures) || len(boundary.Globals) != int(shape.Globals) ||
		len(boundary.Ambients) != int(shape.Ambients) || !validAmbientRoots(boundary.Ambients) {
		return DirectCallBindings{}, fmt.Errorf("callee capture/global/ambient boundary order differs from shape")
	}
	out := DirectCallBindings{Values: make([]ValueTerm, 0, shape.InputCount()), Paths: make([]PathTerm, 0, shape.InputCount())}
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
	for i, source := range sources {
		if len(out.Values) >= int(shape.Params) {
			break
		}
		value, path, err := exactDirectCallSourceBinding(ctx, source)
		if err != nil {
			return DirectCallBindings{}, fmt.Errorf("argument %d: %w", i, err)
		}
		out.Values, out.Paths = append(out.Values, value), append(out.Paths, path)
	}
	// Lua binds every missing fixed parameter to nil and ignores excess values
	// after evaluating their expressions. The call-site diagnostic transaction
	// still reports the arity mismatch; the lexical body relation must remain a
	// total call equation so that diagnostic publication cannot abort freezing.
	for len(out.Values) < int(shape.Params) {
		out.Values = append(out.Values, ctx.builder.Arena().Constant(typevalue.Nil(ctx.registry)))
		out.Paths = append(out.Paths, 0)
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
	for _, ambient := range boundary.Ambients {
		value, path, err := exactDirectCallLexicalBinding(ctx, ambient.Symbol)
		if err != nil {
			return DirectCallBindings{}, fmt.Errorf("ambient %d: %w", ambient.Symbol, err)
		}
		out.Values, out.Paths = append(out.Values, value), append(out.Paths, path)
	}
	return out, nil
}

func exactDirectCallLexicalBinding(ctx planCompileContext, target symbol.ID) (ValueTerm, PathTerm, error) {
	if expression, function, direct := ctx.directDeclarations.FunctionForTarget(ctx.plan, target); direct {
		value, exact := ctx.facts.ExpressionValue(expression)
		identified, hasFunction := ctx.facts.ExpressionFunction(expression)
		if !exact || !hasFunction || identified != function {
			return 0, 0, fmt.Errorf("direct lexical function %d has no exact value", target)
		}
		term := ctx.builder.Arena().Constant(value)
		if term == 0 {
			return 0, 0, fmt.Errorf("direct lexical function %d has a foreign value", target)
		}
		return term, 0, nil
	}
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
	return value, binding.Base, nil
}

func exactDirectCallSourceBinding(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, PathTerm, error) {
	value, err := exactCompilerScalarSourceTerm(ctx, source)
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

// exactCompilerScalarSourceTerm is the single scalar admission boundary for a
// direct lexical call. A canonical ValueTerm is already a one-value symbolic
// producer; this function proves only the surrounding Lua list shape is closed
// and, when adjusted, selects slot zero exactly.
func exactCompilerScalarSourceTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, error) {
	if !source.Valid() {
		return 0, fmt.Errorf("source shape is malformed")
	}
	// ValueSource is already one slot of the surrounding argument tuple. An
	// expanded call therefore remains an exact scalar source when factflow has
	// selected both its result slot and its destination slot; the open tail is
	// list metadata after that selected coordinate, not uncertainty in this
	// coordinate. Other expanded producers retain no such result identity and
	// remain inadmissible here.
	if source.Expanded || source.OpenTail {
		if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.ResultIndex < 0 || source.TargetIndex < 0 {
			return 0, fmt.Errorf("source has an unselected expanded or open-tail shape")
		}
	}
	if source.Adjusted && source.ResultIndex != 0 {
		return 0, fmt.Errorf("adjusted source does not select slot zero")
	}
	term, err := exactCompilerSourceTerm(ctx, source)
	if err != nil {
		return 0, err
	}
	if term == 0 {
		return 0, fmt.Errorf("source has no exact scalar term")
	}
	return term, nil
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
			if binding.Root == (Root{}) {
				return 0, fmt.Errorf("expression path is body-local and has no boundary lens")
			}
			_, term, err := ctx.builder.Arena().LowerBoundaryPathValue(p, binding)
			return term, err
		}
	}
	_, path, err := boundaryLexicalSourceTerms(ctx, source)
	if err == nil && path != 0 {
		node := ctx.builder.Arena().paths[path]
		if node.environment != 0 {
			return 0, fmt.Errorf("source path is body-local and has no boundary lens")
		}
	}
	return path, err
}
