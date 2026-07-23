package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	enginestate "github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/semantic/intrinsic"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// PlanCompiler lowers the certified, immutable operation plan into a symbolic
// relation. It is deliberately inactive: callers must explicitly compile and
// specialize the result, and production call routing does not consult it.
//
// Every operation-plan catalog entry owns a registration. A nil handler is an
// explicit contextual verdict, not an omitted case. This keeps FactsInput and
// extension growth fail-closed without a second semantic catalog.
type PlanCompiler struct {
	facts                  map[operationplan.Kind]planKindHandler
	extensions             map[operationplan.ExtensionKind]planExtensionHandler
	expressionValueFreeze  sourcevalue.ExpressionValueProvider
	expressionValueState   enginestate.State
	expressionValueResults map[frozenExpressionValueKey]frozenExpressionValueResult
}

type frozenExpressionValueKey struct {
	point  cfg.Point
	source factflow.ValueSource
}

type frozenExpressionValueResult struct {
	value product.Value
	exact bool
}

type planCompileContext struct {
	registry                  *axis.Registry
	graph                     cfg.Graph
	plan                      *operationplan.Plan
	facts                     factflow.Facts
	builder                   *Builder
	locals                    map[symbol.ID]ValueTerm
	resultRoots               map[ResultRoot]ValueTerm
	expressions               map[factflow.ExprRef][]ValueTerm
	allocationEffects         map[cfg.Point]EffectTerm
	externalResults           map[cfg.Point][]ValueTerm
	valueAccess               *valueAccessCollector
	rowSteps                  *[]rowStep
	rowOutput                 *summary.Summary
	structuralOutput          *structuralOutputContribution
	genericBindings           map[symbol.ID]symbolicGenericBinding
	directDeclarations        operationplan.DirectLexicalDeclarations
	allowLexicalBoundaryRoots bool
	allowConstantAdd          bool
	predicateExpressions      map[factflow.ExprRef]struct{}
	expressionRefinements     map[factflow.ExprRef]struct{}
	structuralPredicates      map[factflow.ExprRef]factflow.StructuralExpressionRegion
	structuralEnvironment     bool
	rootAssignment            *rootAssignmentTerm
	returnTransaction         *returnTransactionTerm
	expressionValueFreeze     sourcevalue.ExpressionValueProvider
	expressionValueState      enginestate.State
	expressionValueResults    map[frozenExpressionValueKey]frozenExpressionValueResult
	point                     cfg.Point
}

// valueAccessCollector records source-point ownership while the canonical
// ValueSource compiler is already traversing the source DAG. It neither walks
// ValueSource a second time nor introduces an alternate term vocabulary.
type valueAccessCollector struct {
	primary cfg.Point
	terms   map[valueAccessTerm]struct{}
}

func newValueAccessCollector(primary cfg.Point) *valueAccessCollector {
	return &valueAccessCollector{primary: primary, terms: make(map[valueAccessTerm]struct{})}
}

func (c *valueAccessCollector) record(source factflow.ValueSource, term ValueTerm) {
	if c == nil || term == 0 {
		return
	}
	// A direct call-result source is owned by its call point. Expression
	// sources may prefer a flow source point and fall back to their producing
	// call point only when that preferred wire is unreachable.
	if source.Kind == factflow.ValueSourceCall && source.HasCallPoint && source.CallPoint != 0 {
		c.terms[valueAccessTerm{term: term, point: source.CallPoint, hasPoint: true}] = struct{}{}
		return
	}
	preferred := c.primary
	if source.HasSourcePoint && source.SourcePoint != 0 {
		preferred = source.SourcePoint
	} else if source.HasCallPoint && source.CallPoint != 0 {
		preferred = source.CallPoint
	}
	c.terms[valueAccessTerm{term: term, point: preferred, hasPoint: true}] = struct{}{}
	if source.HasSourcePoint && source.SourcePoint != 0 && source.HasCallPoint && source.CallPoint != 0 && source.CallPoint != preferred {
		c.terms[valueAccessTerm{term: term, point: source.CallPoint, hasPoint: true, fallback: true}] = struct{}{}
	}
}

func (c *valueAccessCollector) frozen() []valueAccessTerm {
	if c == nil || len(c.terms) == 0 {
		return nil
	}
	out := make([]valueAccessTerm, 0, len(c.terms))
	for term := range c.terms {
		out = append(out, term)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].point != out[j].point {
			return out[i].point < out[j].point
		}
		if out[i].term != out[j].term {
			return out[i].term < out[j].term
		}
		return !out[i].fallback && out[j].fallback
	})
	return out
}

type symbolicGenericBinding struct {
	Transaction factapply.GenericForTransaction
	Iterator    iteration.Iterator
	Container   ValueTerm
	Projection  ValueTerm
	FirstTarget symbol.ID
	Identity    frozenGenericForIdentityPublication
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
	c.facts[operationplan.ExpressionOperation] = expressionOperationPlanHandler{}
	// ExpressionFunction is the identity sidecar for an ordinary exact
	// ExpressionValue. The assignment/store owner lowers that value; the
	// sidecar itself has no independent executable effect. A separately sealed
	// direct-declaration subset may omit only values proven unobservable.
	c.facts[operationplan.ExpressionFunction] = expressionFunctionPlanHandler{}
	// ExpressionRefinement is only a dependency carrier. Preparation admits it
	// iff every refinement is an exact runtime-cast member of one certified
	// returned predicate DAG; the point-local handler has no executable effect.
	c.facts[operationplan.ExpressionRefinement] = dynamicIndexDependencyPlanHandler{kind: operationplan.ExpressionRefinement}
	c.facts[operationplan.ExpressionCondition] = expressionConditionPlanHandler{}
	c.facts[operationplan.ExpressionPath] = dynamicIndexDependencyPlanHandler{kind: operationplan.ExpressionPath}
	c.facts[operationplan.DynamicIndexExpression] = dynamicIndexDependencyPlanHandler{kind: operationplan.DynamicIndexExpression}
	c.facts[operationplan.RootAssignment] = rootAssignmentPlanHandler{}
	for _, pathStoreKind := range []operationplan.Kind{operationplan.PathAssignment, operationplan.PathStaticMemberWrite} {
		c.facts[pathStoreKind] = pathStorePlanHandler{kind: pathStoreKind}
	}
	c.facts[operationplan.Return] = returnPlanHandler{}
	c.facts[operationplan.CallSite] = signatureCallPlanHandler{}
	for _, dynamicKind := range []operationplan.Kind{
		operationplan.PathDescendantInvalidation,
		operationplan.DynamicIndexWrite,
	} {
		c.facts[dynamicKind] = dynamicIndexPlanHandler{kind: dynamicKind}
	}
	// These edge families are consumed together by (*PreparedPlanCompiler).structuralBranch.
	// They are registered here so the operation-plan exhaustiveness gate recognizes
	// one semantic owner; their point-local handlers intentionally publish nothing.
	for _, branchKind := range []operationplan.Kind{
		operationplan.BranchEdgeReachability,
		operationplan.BranchConditionSource,
		operationplan.BranchRefinement,
		operationplan.BranchPresenceRelation,
		operationplan.BranchPathRelation,
		operationplan.BranchPathEvidence,
		operationplan.BranchSufficientLiteralCase,
	} {
		c.facts[branchKind] = branchEdgePlanHandler{kind: branchKind}
	}
	for _, resultKind := range []operationplan.Kind{
		operationplan.CallResultValue,
		operationplan.PostconditionRefinement,
		operationplan.PostconditionPathRelation,
		operationplan.ReturnPresenceRelation,
	} {
		c.facts[resultKind] = callResultPlanHandler{kind: resultKind}
	}
	c.facts[operationplan.NoNormalReturn] = noNormalReturnPlanHandler{}
	c.facts[operationplan.ObjectLiteral] = pointTransactionPlanHandler{kind: operationplan.ObjectLiteral}
	for _, kind := range []operationplan.Kind{
		operationplan.PathValuePresenceImplication,
		operationplan.ChannelSelect,
		operationplan.CovariantExposure,
	} {
		c.facts[kind] = pointTransactionPlanHandler{kind: kind}
	}
	c.extensions[operationplan.BodyGenericFor] = genericForPlanHandler{}
	return c
}

type branchEdgePlanHandler struct{ kind operationplan.Kind }

func (h branchEdgePlanHandler) Kind() operationplan.Kind                              { return h.kind }
func (branchEdgePlanHandler) Preflight(planCompileContext, cfg.Point) error           { return nil }
func (branchEdgePlanHandler) Lower(planCompileContext, cfg.Point, *[]Operation) error { return nil }

type callResultPlanHandler struct{ kind operationplan.Kind }

func (h callResultPlanHandler) Kind() operationplan.Kind                              { return h.kind }
func (callResultPlanHandler) Preflight(planCompileContext, cfg.Point) error           { return nil }
func (callResultPlanHandler) Lower(planCompileContext, cfg.Point, *[]Operation) error { return nil }

type noNormalReturnPlanHandler struct{}

func (noNormalReturnPlanHandler) Kind() operationplan.Kind                                { return operationplan.NoNormalReturn }
func (noNormalReturnPlanHandler) Preflight(planCompileContext, cfg.Point) error           { return nil }
func (noNormalReturnPlanHandler) Lower(planCompileContext, cfg.Point, *[]Operation) error { return nil }

// pointTransactionPlanHandler admits families whose complete typed payload is
// frozen by the structural point transaction in canonical phase order. It has
// no secondary lowering and therefore cannot create an alternative semantic
// path.
type pointTransactionPlanHandler struct{ kind operationplan.Kind }

func (h pointTransactionPlanHandler) Kind() operationplan.Kind                    { return h.kind }
func (pointTransactionPlanHandler) Preflight(planCompileContext, cfg.Point) error { return nil }
func (pointTransactionPlanHandler) Lower(planCompileContext, cfg.Point, *[]Operation) error {
	return nil
}

type signatureCallPlanHandler struct{}

func (signatureCallPlanHandler) Kind() operationplan.Kind { return operationplan.CallSite }
func (signatureCallPlanHandler) Preflight(ctx planCompileContext, point cfg.Point) error {
	op, ok := ctx.plan.SignatureCallOperation(point)
	if !ok {
		// Module-only require sites are external calls with their own frozen N0
		// producer.  They deliberately have no signature descriptor; treating
		// that absence as an unresolved call discards the exact manifest export
		// already owned by the operation plan.
		if _, module := ctx.plan.ModuleLoadOperation(point); module {
			return nil
		}
		if _, _, exact := boundaryMemberCallFromSite(ctx, point); exact {
			return nil
		}
		if surface, exact := ctx.plan.CallSurface(); exact {
			if classified, found := surface.Site(point); found && classified.Target.Kind() != operationplan.CallSurfaceTargetLexical {
				return nil
			}
		}
		return fmt.Errorf("signature call: resolved producer missing")
	}
	if intrinsic, exact := op.Intrinsic(); exact {
		if intrinsic != signature.IntrinsicLuaType {
			return fmt.Errorf("signature call: unsupported intrinsic identity %d", intrinsic)
		}
		site, exists := ctx.facts.CallSiteView(point)
		if !exists || site.ArgumentSourceCount() != 1 {
			return fmt.Errorf("signature call: intrinsic call-site payload missing or malformed")
		}
		return nil
	}
	sig := op.Signature()
	// A composite require site owns both descriptors.  The module transaction
	// is the authoritative result producer; the signature remains the owner of
	// any declared diagnostics/effects.  Result construction was frozen by
	// bindStaticSignatureTerms and therefore needs no contextual body solve.
	if _, module := ctx.plan.ModuleLoadOperation(point); module {
		return nil
	}
	if _, exact := ctx.plan.SignatureAllocationOperation(point); exact {
		if ctx.allocationEffects[point] == 0 {
			return fmt.Errorf("signature call: allocation term missing")
		}
		return nil
	}
	if _, ok := exactStaticSignatureReturnsAtPoint(ctx, point, op); ok {
		if _, exists := ctx.facts.CallSiteView(point); !exists {
			return fmt.Errorf("signature call: call-site payload missing")
		}
		return nil
	}
	iterator, ok := iteration.ActiveIterator(sig.Effect.Labels)
	if !ok || sig.Effect.Tail != nil || len(sig.Effect.Labels) != 1 || sig.OperationalEffects != nil {
		return nil
	}
	for p := 0; p < ctx.plan.PointCount(); p++ {
		generic, exists := ctx.plan.GenericForOperation(cfg.Point(p))
		if !exists {
			continue
		}
		source, _ := generic.ProtocolSource(0)
		got, hasIterator := generic.Iterator()
		if source.Kind == operationplan.GenericForSourceCall && source.HasCallPoint && source.CallPoint == point && hasIterator && got == iterator {
			return nil
		}
	}
	return fmt.Errorf("signature call: iterator result is not owned by generic-for")
}
func (signatureCallPlanHandler) Lower(ctx planCompileContext, point cfg.Point, _ *[]Operation) error {
	// A signature allocation is not an external-call producer. Its result and
	// complete heap/fresh effect are the two projections of one allocation term
	// frozen by bindStaticSignatureTerms. Keep that producer kind structurally
	// separate: consulting provider operands here would make an unrelated
	// callee/argument source a prerequisite for an already-complete allocation
	// transaction.
	if _, allocation := ctx.plan.SignatureAllocationOperation(point); allocation {
		return lowerSignatureAllocationTransaction(ctx, point)
	}
	return lowerExternalSignatureCall(ctx, point)
}

func lowerSignatureAllocationTransaction(ctx planCompileContext, point cfg.Point) error {
	effect := ctx.allocationEffects[point]
	if effect == 0 {
		return fmt.Errorf("signature call: allocation term missing")
	}
	if ctx.rowSteps == nil {
		return fmt.Errorf("signature call: allocation row sink missing")
	}
	if ctx.builder == nil || ctx.builder.EffectArena() == nil || int(effect) >= len(ctx.builder.EffectArena().nodes) {
		return fmt.Errorf("signature call: allocation effect is unowned")
	}
	effectNode := ctx.builder.EffectArena().nodes[effect]
	if effectNode.kind != EffectAllocationTemplate || !ctx.builder.Arena().validAllocation(effectNode.allocation) {
		return fmt.Errorf("signature call: allocation effect has no shared template")
	}
	op, exact := ctx.plan.SignatureAllocationOperation(point)
	if !exact || !allocationOperationEqual(ctx.builder.Arena().allocations[effectNode.allocation].op, op) {
		return fmt.Errorf("signature call: allocation effect diverges from its planned operation")
	}
	resultIndex := op.Template().ReturnIndex
	result := ctx.builder.Arena().AllocationResultValue(effectNode.allocation, resultIndex)
	if result == 0 {
		return fmt.Errorf("signature call: allocation result %d is missing", resultIndex)
	}
	site, exact := ctx.facts.CallSiteView(point)
	if !exact {
		return fmt.Errorf("signature call: allocation result targets are missing")
	}
	for index := 0; index < site.ResultTargetCount(); index++ {
		target, ok := site.ResultTargetAt(index)
		if !ok || target.Kind() != factflow.CallResultTargetLocalAssignment || target.ResultIndex() != resultIndex {
			continue
		}
		if target.TargetSymbol() == 0 || ctx.locals == nil {
			return fmt.Errorf("signature call: allocation local result has no structural destination")
		}
		ctx.locals[target.TargetSymbol()] = result
	}
	*ctx.rowSteps = append(*ctx.rowSteps, localEffectStep(effect))
	return nil
}

func lowerExternalSignatureCall(ctx planCompileContext, point cfg.Point) error {
	memberCall, _, hasMemberCall := boundaryMemberCallFromSite(ctx, point)
	if ctx.structuralOutput != nil {
		plan, err := externalCallAccessTerms(ctx, point)
		if err != nil {
			return err
		}
		ctx.structuralOutput.externalAccess = plan.access
		ctx.structuralOutput.externalOperands = plan.operands
		ctx.structuralOutput.externalSealed = true
		if hasMemberCall {
			// The external instruction owns both the CallOutcome and the
			// flow-sensitive member diagnostic for this execution. Derive the
			// diagnostic here, while freezing that instruction, rather than in a
			// point-wide postpass that cannot distinguish allocations or lexical doors.
			ctx.structuralOutput.memberCalls = append(ctx.structuralOutput.memberCalls, memberCall)
		}
	}
	if op, ok := ctx.plan.SignatureCallOperation(point); ok {
		operational := op.Signature().OperationalEffects
		if operational == nil || !operational.SuspensionKnown || operational.MaySuspend {
			if ctx.structuralOutput != nil {
				ctx.structuralOutput.maySuspend = true
			} else if ctx.rowOutput == nil {
				return fmt.Errorf("signature call: suspension row output sink missing")
			} else {
				ctx.rowOutput.MaySuspend = true
			}
		}
	} else if hasMemberCall {
		if ctx.structuralOutput == nil {
			return fmt.Errorf("signature call: contextual member diagnostics require structural output")
		}
		// A boundary-provided callback has no sealed operational summary. Preserve
		// the existing conservative suspension result while its typed diagnostic
		// event stays conditional on the stabilized provider value.
		ctx.structuralOutput.maySuspend = true
	} else {
		// A dynamically typed external producer is resolved by the canonical
		// external-call instruction. Without a sealed signature, suspension is
		// conservatively possible.
		if ctx.structuralOutput != nil {
			ctx.structuralOutput.maySuspend = true
		}
	}
	return nil
}

// externalCallAccessTerms lowers the provider-visible call operands through
// the same canonical ValueTerm compiler used by execution. The term DAG is
// therefore the only source-level dependency law; access planning never
// re-walks or reinterprets ValueSource syntax.
type externalCallAccessPlan struct {
	access   []valueAccessTerm
	operands callOutcomeOperandTerms
}

func externalCallAccessTerms(ctx planCompileContext, point cfg.Point) (externalCallAccessPlan, error) {
	site, ok := ctx.facts.CallSiteView(point)
	if !ok {
		return externalCallAccessPlan{}, fmt.Errorf("external call at point %d has no call-site payload", point)
	}
	collector := newValueAccessCollector(point)
	ctx.valueAccess = collector
	appendSource := func(label string, source factflow.ValueSource) (ValueTerm, error) {
		term, err := exactCompilerSourceTerm(ctx, source)
		if err != nil {
			return 0, fmt.Errorf("external call at point %d %s: %w", point, label, err)
		}
		if term == 0 {
			return 0, fmt.Errorf("external call at point %d %s has no canonical value term", point, label)
		}
		return term, nil
	}
	var operands callOutcomeOperandTerms
	if source, present := site.CalleeSource(); present {
		term, err := appendSource("callee", source)
		if err != nil {
			return externalCallAccessPlan{}, err
		}
		operands.callee, operands.hasCallee = term, true
	}
	if source, present := site.ReceiverSource(); present {
		term, err := appendSource("receiver", source)
		if err != nil {
			return externalCallAccessPlan{}, err
		}
		operands.receiver, operands.hasReceiver = term, true
	}
	var sourceErr error
	site.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
		term, err := appendSource(fmt.Sprintf("argument %d", index), source)
		sourceErr = err
		if err == nil {
			operands.arguments = append(operands.arguments, term)
		}
		return sourceErr == nil
	})
	if sourceErr != nil {
		return externalCallAccessPlan{}, sourceErr
	}
	return externalCallAccessPlan{access: collector.frozen(), operands: operands}, nil
}

// withExpressionValueFreeze binds the caller's opaque expression authority to
// this one compiler preparation. Every consulted source is memoized and
// lowered immediately to an immutable Constant term; neither the prepared
// relation nor its executor retains the callback.
func (c *PlanCompiler) withExpressionValueFreeze(provider sourcevalue.ExpressionValueProvider, in enginestate.State) *PlanCompiler {
	if c == nil || provider == nil {
		return c
	}
	c.expressionValueFreeze = provider
	c.expressionValueState = in
	c.expressionValueResults = make(map[frozenExpressionValueKey]frozenExpressionValueResult)
	return c
}

func frozenCustomExpressionTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, bool, error) {
	if ctx.expressionValueFreeze == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return 0, false, nil
	}
	key := frozenExpressionValueKey{point: ctx.point, source: source}
	result, cached := ctx.expressionValueResults[key]
	if !cached {
		result.value, result.exact = ctx.expressionValueFreeze(ctx.point, source.ExprRef, source, ctx.expressionValueState)
		ctx.expressionValueResults[key] = result
	}
	if !result.exact {
		return 0, false, nil
	}
	if !product.BelongsToRegistry(ctx.registry, result.value) {
		return 0, false, fmt.Errorf("custom expression %d returned a value from another registry", source.ExprRef)
	}
	term := ctx.builder.Arena().Constant(result.value)
	if term == 0 {
		return 0, false, fmt.Errorf("custom expression %d failed immutable freezing", source.ExprRef)
	}
	return term, true, nil
}

// bindChannelSelectResultTerms seals the scalar register written by each
// channel-select transaction. OpSelect uses the common call-result source
// vocabulary but has no CallSite; its select event is the producer authority.
func bindChannelSelectResultTerms(ctx *planCompileContext) error {
	if ctx == nil || ctx.plan == nil || ctx.builder == nil {
		return fmt.Errorf("channel select results have no term owner")
	}
	for rawPoint := 0; rawPoint < ctx.plan.PointCount(); rawPoint++ {
		point := cfg.Point(rawPoint)
		for _, event := range ctx.facts.ChannelSelects(point) {
			if event.Kind() != factflow.ChannelSelectSelect {
				continue
			}
			if event.Index() < 0 || ctx.builder.Arena().bindCallResult(point, event.Index()) == 0 {
				return fmt.Errorf("channel select at point %d has no exact result slot", point)
			}
		}
	}
	return nil
}

// bindStaticSignatureTerms materializes context-independent call results once
// into the symbolic expression table. It consumes only the Plan's resolved,
// immutable signature sidecar; no runtime provider or callee-name dispatch is
// consulted during relation compilation.
func bindStaticSignatureTerms(ctx *planCompileContext) error {
	for rawPoint := 0; rawPoint < ctx.plan.PointCount(); rawPoint++ {
		point := cfg.Point(rawPoint)
		if transaction, exact := factapply.PlanModuleLoadTransaction(ctx.registry, ctx.plan, point); exact {
			argument, err := exactReturnSourceValue(ctx.registry, ctx.facts, transaction.Argument())
			if err != nil {
				return fmt.Errorf("module-load call at point %d argument: %w", point, err)
			}
			resolved, ok := transaction.Resolve(ctx.registry, argument)
			if !ok {
				return fmt.Errorf("module-load call at point %d failed exact export resolution", point)
			}
			result := resolved.ResultTransaction()
			step, ok := result.Step(0)
			if !ok {
				return fmt.Errorf("module-load call at point %d has no result transaction", point)
			}
			value, ok := step.ResultValue()
			if !ok || value.Index() != operationplan.ModuleLoadResultIndex {
				return fmt.Errorf("module-load call at point %d has malformed result transaction", point)
			}
			site, ok := ctx.facts.CallSiteView(point)
			if !ok {
				return fmt.Errorf("module-load call at point %d has no call-site payload", point)
			}
			if ref, hasExpr := site.Expr(); hasExpr {
				if _, duplicate := ctx.expressions[ref]; duplicate {
					return fmt.Errorf("module-load expression %d has multiple producers", ref)
				}
				ctx.expressions[ref] = []ValueTerm{ctx.builder.Arena().Constant(value.Value())}
			}
			// A composite site must not install the weaker declared signature
			// result over the exact manifest export below.
			continue
		}
		if allocationOp, exact := ctx.plan.SignatureAllocationOperation(point); exact {
			site, ok := ctx.facts.CallSiteView(point)
			if !ok {
				return fmt.Errorf("allocation signature call at point %d has no call-site payload", point)
			}
			ref, hasExpr := site.Expr()
			if !hasExpr {
				return fmt.Errorf("allocation signature call at point %d has no expression identity", point)
			}
			allocation := ctx.builder.Arena().AllocationTemplate(allocationOp)
			resultIndex := allocationOp.Template().ReturnIndex
			result := ctx.builder.Arena().AllocationResultValue(allocation, resultIndex)
			effect, err := ctx.builder.EffectArena().AllocationTemplate(allocation)
			if allocation == 0 || result == 0 || err != nil {
				return fmt.Errorf("allocation signature call at point %d failed symbolic construction", point)
			}
			if _, exists := ctx.expressions[ref]; exists {
				return fmt.Errorf("signature call expression %d has multiple producers", ref)
			}
			terms := make([]ValueTerm, resultIndex+1)
			terms[resultIndex] = result
			ctx.expressions[ref] = terms
			ctx.allocationEffects[point] = effect
			continue
		}
		op, ok := ctx.plan.SignatureCallOperation(point)
		if !ok {
			continue
		}
		if intrinsic, exact := op.Intrinsic(); exact {
			site, exists := ctx.facts.CallSiteView(point)
			if !exists {
				return fmt.Errorf("intrinsic signature call at point %d has no call-site payload", point)
			}
			ref, hasExpr := site.Expr()
			if !hasExpr || site.ArgumentSourceCount() != 1 {
				return fmt.Errorf("intrinsic signature call at point %d has no scalar expression/argument", point)
			}
			arg, present := site.ArgumentSourceAt(0)
			if !present {
				return fmt.Errorf("intrinsic signature call at point %d has no first argument", point)
			}
			argTerm, err := exactCompilerSourceTerm(*ctx, arg)
			if err != nil {
				return fmt.Errorf("intrinsic signature call at point %d argument: %w", point, err)
			}
			var result ValueTerm
			switch intrinsic {
			case signature.IntrinsicLuaType:
				result = ctx.builder.Arena().LuaTypeNameValue(argTerm)
			default:
				return fmt.Errorf("intrinsic signature call at point %d has unsupported identity %d", point, intrinsic)
			}
			if result == 0 {
				return fmt.Errorf("intrinsic signature call at point %d failed symbolic construction", point)
			}
			if _, duplicate := ctx.expressions[ref]; duplicate {
				return fmt.Errorf("signature call expression %d has multiple producers", ref)
			}
			ctx.expressions[ref] = []ValueTerm{result}
			continue
		}
		values, ok := exactStaticSignatureReturnsAtPoint(*ctx, point, op)
		if !ok {
			if err := bindExternalCallSlotTerms(ctx, point, op.Signature().Type); err != nil {
				return err
			}
			site, present := ctx.facts.CallSiteView(point)
			if !present {
				return fmt.Errorf("signature call at point %d has no call-site payload", point)
			}
			if ref, hasExpr := site.Expr(); hasExpr {
				terms := append([]ValueTerm(nil), ctx.expressions[ref]...)
				for resultIndex := range terms {
					term, dependent, err := exactDependentCallResultTerm(*ctx, point, resultIndex)
					if err != nil {
						return err
					}
					if dependent {
						terms[resultIndex] = term
					}
				}
				ctx.expressions[ref] = terms
			}
			continue
		}
		site, ok := ctx.facts.CallSiteView(point)
		if !ok {
			return fmt.Errorf("signature call at point %d has no call-site payload", point)
		}
		ref, hasExpr := site.Expr()
		if !hasExpr {
			if site.ResultTargetCount() != 0 {
				return fmt.Errorf("signature call at point %d has result targets but no expression identity", point)
			}
			continue
		}
		if _, exists := ctx.expressions[ref]; exists {
			return fmt.Errorf("signature call expression %d has multiple producers", ref)
		}
		terms := make([]ValueTerm, len(values))
		for i, value := range values {
			terms[i] = ctx.builder.Arena().Constant(value)
		}
		ctx.expressions[ref] = terms
	}
	// Rejected-but-complete external call-surface sites (for example a method
	// whose callable type is known only from stabilized State) have no signature
	// descriptor. They still consume the same point-owned call-result vocabulary; the
	// prepared external producer resolves the value in the equation step.
	if surface, exact := ctx.plan.CallSurface(); exact {
		for _, classified := range surface.Sites() {
			if classified.Target.Kind() == operationplan.CallSurfaceTargetLexical {
				continue
			}
			if err := bindExternalCallSlotTerms(ctx, classified.Point, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func bindExternalCallSlotTerms(ctx *planCompileContext, point cfg.Point, declared *typ.Function) error {
	if ctx == nil || ctx.builder == nil {
		return fmt.Errorf("external call at point %d has no term builder", point)
	}
	site, ok := ctx.facts.CallSiteView(point)
	if !ok {
		return fmt.Errorf("external call at point %d has no call-site payload", point)
	}
	ref, hasExpr := site.Expr()
	_, expressionAlreadyBound := ctx.expressions[ref]
	width := 0
	if declared != nil {
		width = len(declared.Returns)
	}
	// Condition calls consume exactly their adjusted head result even though
	// they have no assignment/return target. Seal that registered coordinate so
	// the branch source and external-call instruction share one CallResult
	// authority.
	if site.Context() == factflow.CallSiteContextCondition && width < 1 {
		width = 1
	}
	// A direct open tail has one syntactic target but consumes every remaining
	// coordinate of the enclosing function's declared return tuple.  Seal that
	// complete tuple here so later N5 normalization can select each slot without
	// inventing a second call-result authority.
	if site.Context() == factflow.CallSiteContextReturnSource && site.OpenTail() {
		if required := planReturnArity(ctx.plan); required > width {
			width = required
		}
	}
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		if next := target.ResultIndex() + 1; next > width {
			width = next
		}
		return true
	})
	if width == 0 {
		return nil
	}
	terms := make([]ValueTerm, width)
	for index := range terms {
		terms[index] = ctx.builder.Arena().bindCallResult(point, index)
		if terms[index] == 0 {
			return fmt.Errorf("external call at point %d could not seal result slot %d", point, index)
		}
	}
	if ctx.externalResults == nil {
		ctx.externalResults = make(map[cfg.Point][]ValueTerm)
	}
	ctx.externalResults[point] = append([]ValueTerm(nil), terms...)
	if hasExpr && !expressionAlreadyBound {
		ctx.expressions[ref] = terms
	}
	return nil
}

func exactStaticSignatureReturnsAtPoint(ctx planCompileContext, point cfg.Point, op operationplan.SignatureCallOperation) ([]product.Value, bool) {
	site, ok := ctx.facts.CallSiteView(point)
	if !ok {
		return nil, false
	}
	if site.MethodName() != "" {
		return effectlowering.StaticScalarStringMethodReturns(ctx.registry, nil, op.Signature(), site)
	}
	return effectlowering.StaticScalarSignatureReturns(ctx.registry, nil, op.Signature())
}

func exactDependentSignatureReturnTerm(ctx planCompileContext, point cfg.Point, op operationplan.SignatureCallOperation, resultIndex int) (ValueTerm, bool, error) {
	site, ok := ctx.facts.CallSiteView(point)
	if !ok {
		return 0, false, fmt.Errorf("signature call at point %d has no call-site payload", point)
	}
	argumentIndex, dependent := effectlowering.ExactSameAsReturnArgument(op.Signature(), resultIndex, site.ArgumentSourceCount())
	if !dependent {
		return 0, false, nil
	}
	argument, present := site.ArgumentSourceAt(argumentIndex)
	if !present {
		return 0, false, fmt.Errorf("signature call at point %d result %d has no dependent argument %d", point, resultIndex, argumentIndex)
	}
	term, err := exactCompilerSourceTerm(ctx, argument)
	if err != nil {
		return 0, false, fmt.Errorf("signature call at point %d result %d dependent argument %d: %w", point, resultIndex, argumentIndex, err)
	}
	if term == 0 {
		return 0, false, fmt.Errorf("signature call at point %d result %d dependent argument %d failed symbolic construction", point, resultIndex, argumentIndex)
	}
	return term, true, nil
}

// exactDependentCallResultTerm compiles call-result equations whose value is
// determined by a call operand. Typed operation descriptors take precedence:
// they are binding-resolved semantic identities even when a generic/effectful
// signature is intentionally absent from the static signature catalog.
func exactDependentCallResultTerm(ctx planCompileContext, point cfg.Point, resultIndex int) (ValueTerm, bool, error) {
	if attach, exact := ctx.plan.AttachMetatableOperation(point); exact && resultIndex == 0 {
		term, err := exactCompilerSourceTerm(ctx, attach.Table())
		if err != nil {
			return 0, false, fmt.Errorf("attach-metatable call at point %d table result: %w", point, err)
		}
		if term == 0 {
			return 0, false, fmt.Errorf("attach-metatable call at point %d table result failed symbolic construction", point)
		}
		return term, true, nil
	}
	if op, exact := ctx.plan.SignatureCallOperation(point); exact {
		return exactDependentSignatureReturnTerm(ctx, point, op, resultIndex)
	}
	return 0, false, nil
}

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
	source, _ := op.ProtocolSource(0)
	switch source.Kind {
	case operationplan.GenericForSourceCall:
		if !source.HasCallPoint {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator call point missing")
		}
		site, ok := ctx.facts.CallSiteView(source.CallPoint)
		if !ok {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator call-site payload missing")
		}
		iterator, collectionIterator := op.Iterator()
		if !collectionIterator {
			// A sealed callable signature, when the operation was admitted with
			// one, remains an exact consistency obligation.  An unclassified
			// generic-for call has no separate transfer: it is represented by the
			// canonical protocol-result term below, whose evaluator either projects
			// the function result from the existing value carrier or takes its
			// explicit no-write fallback.
			if op.CallableIterator() {
				call, sealed := ctx.plan.SignatureCallOperation(source.CallPoint)
				if !sealed {
					return symbolicGenericBinding{}, fmt.Errorf("generic-for: callable iterator signature missing")
				}
				if _, callable := effectlowering.CallableIteratorSignature(call.Signature()); !callable {
					return symbolicGenericBinding{}, fmt.Errorf("generic-for: callable iterator signature drifted")
				}
			}
			ref, hasExpr := site.Expr()
			terms := ctx.expressions[ref]
			fallback := ctx.locals[op.Target()]
			if fallback == 0 {
				fallback = ctx.builder.Arena().Constant(product.Bottom(ctx.builder.Arena().reg))
			}
			iteratorTerm := fallback
			if hasExpr && len(terms) != 0 && terms[0] != 0 {
				iteratorTerm = terms[0]
			}
			nilTerm := ctx.builder.Arena().Constant(typevalue.Nil(ctx.builder.Arena().reg))
			projection := ctx.builder.Arena().genericForResultValue(op.VariableIndex(), iteratorTerm, nilTerm, nilTerm, fallback)
			if projection == 0 {
				return symbolicGenericBinding{}, fmt.Errorf("generic-for: callable iterator projection unsupported")
			}
			identityPublication, err := sealGenericForIdentityPublication(ctx.builder.Arena(), statekey.SymbolValue(op.Target()), projection)
			if err != nil {
				return symbolicGenericBinding{}, err
			}
			binding := symbolicGenericBinding{Transaction: transaction, Container: iteratorTerm, Projection: projection, FirstTarget: op.FirstTarget(), Identity: identityPublication}
			if publish {
				ctx.locals[op.Target()] = projection
				ctx.genericBindings[op.Target()] = binding
			}
			return binding, nil
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
		asserted, hasAsserted := op.SourceContract(sourceIndex)
		projection := ctx.builder.Arena().iteratorProjectionValueWithFallback(iterator, op.VariableIndex(), term, ctx.locals[op.Target()], asserted, hasAsserted)
		if projection == 0 {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator projection unsupported")
		}
		identityPublication, err := sealGenericForIdentityPublication(ctx.builder.Arena(), statekey.SymbolValue(op.Target()), projection)
		if err != nil {
			return symbolicGenericBinding{}, err
		}
		binding := symbolicGenericBinding{Transaction: transaction, Iterator: iterator, Container: term, Projection: projection, FirstTarget: op.FirstTarget(), Identity: identityPublication}
		if publish {
			ctx.locals[op.Target()] = projection
			ctx.genericBindings[op.Target()] = binding
		}
		return binding, nil
	case operationplan.GenericForSourceExpression:
		if !source.HasRootPath || source.RootPath.Symbol == 0 || len(source.RootPath.Segments) != 0 {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator expression is not an exact root binding")
		}
		iterator, ok := ctx.locals[source.RootPath.Symbol]
		if !ok {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator source symbol %d has no exact binding", source.RootPath.Symbol)
		}
		protocol := [3]ValueTerm{iterator, ctx.builder.Arena().Constant(typevalue.Nil(ctx.builder.Arena().reg)), ctx.builder.Arena().Constant(typevalue.Nil(ctx.builder.Arena().reg))}
		for index := 1; index < len(protocol) && index < op.ProtocolSourceCount(); index++ {
			protocolSource, _ := op.ProtocolSource(index)
			if protocolSource.Kind != operationplan.GenericForSourceExpression || !protocolSource.HasRootPath ||
				protocolSource.RootPath.Symbol == 0 || len(protocolSource.RootPath.Segments) != 0 {
				return symbolicGenericBinding{}, fmt.Errorf("generic-for: protocol source %d is not an exact root binding", index)
			}
			term, exists := ctx.locals[protocolSource.RootPath.Symbol]
			if !exists {
				return symbolicGenericBinding{}, fmt.Errorf("generic-for: protocol source %d symbol %d has no exact binding", index, protocolSource.RootPath.Symbol)
			}
			protocol[index] = term
		}
		fallback := ctx.locals[op.Target()]
		if fallback == 0 {
			fallback = ctx.builder.Arena().Constant(product.Bottom(ctx.builder.Arena().reg))
		}
		projection := ctx.builder.Arena().genericForResultValue(op.VariableIndex(), protocol[0], protocol[1], protocol[2], fallback)
		if projection == 0 {
			return symbolicGenericBinding{}, fmt.Errorf("generic-for: protocol result projection unsupported")
		}
		identityPublication, err := sealGenericForIdentityPublication(ctx.builder.Arena(), statekey.SymbolValue(op.Target()), projection)
		if err != nil {
			return symbolicGenericBinding{}, err
		}
		binding := symbolicGenericBinding{Transaction: transaction, Container: protocol[0], Projection: projection, FirstTarget: op.FirstTarget(), Identity: identityPublication}
		if publish {
			ctx.locals[op.Target()] = projection
			ctx.genericBindings[op.Target()] = binding
		}
		return binding, nil
	default:
		return symbolicGenericBinding{}, fmt.Errorf("generic-for: iterator source unknown")
	}
}

func planReturnArity(plan *operationplan.Plan) int {
	if plan == nil {
		return 0
	}
	arity := len(plan.BoundaryReturns())
	facts := plan.Facts()
	for rawPoint := 0; rawPoint < plan.PointCount(); rawPoint++ {
		ret, ok := facts.Return(cfg.Point(rawPoint))
		if ok && len(ret.Sources()) > arity {
			arity = len(ret.Sources())
		}
	}
	return arity
}

func numericForBranchHead(ctx planCompileContext, point cfg.Point) bool {
	fact, ok := ctx.facts.RootAssignment(point)
	if !ok {
		return false
	}
	// Control-flow ownership is structural and survives an invalid iterator
	// annotation/bound. Diagnostics may reject those values, but the numeric-for
	// header still branches on the canonical loop-continuation cell rather than
	// requiring an unrelated source expression.
	return numericForHeaderBinding(ctx, point, fact.TargetPathRef())
}

func appendDynamicConditionBranchProof(arena *Arena, branch factapply.BranchAlgebra, edge bool, condition ValueTerm, out *[]BranchProofTerm) error {
	if arena == nil || out == nil || condition == 0 || int(condition) >= len(arena.values) {
		return fmt.Errorf("branch: invalid dynamic condition proof sink")
	}
	descriptor, ok := branch.Condition()
	if !ok || !descriptor.TruthyOnEdge(edge) {
		return nil
	}
	node := arena.values[condition]
	if node.op != valueDynamicRead {
		return nil
	}
	if node.path == 0 || len(node.args) != 2 || node.args[1] == 0 {
		return fmt.Errorf("branch: contextual-dynamic-condition-evidence")
	}
	*out = append(*out, BranchProofTerm{
		Kind: pathevidence.BranchProofPathPresence, Table: node.path, Key: node.args[1], Presence: presence.Present(),
	})
	return nil
}

func genericForBranchHead(graph cfg.Graph, plan *operationplan.Plan, point cfg.Point) bool {
	if graph == nil || plan == nil || !graph.IsBranch(point) {
		return false
	}
	successors := cfg.SuccessorsReadOnly(graph, point)
	conditions := cfg.SuccessorConditionsReadOnly(graph, point)
	if len(successors) != 2 || len(conditions) != len(successors) {
		return false
	}
	trueGeneric, falseGeneric := false, false
	for index, successor := range successors {
		cond := conditions[index]
		_, generic := plan.GenericForOperation(successor)
		if cond {
			trueGeneric = generic
		} else {
			falseGeneric = generic
		}
	}
	return trueGeneric && !falseGeneric
}

func appendRepresentedBranchEvidenceOutput(ctx planCompileContext, branch factapply.BranchAlgebra, cond bool, out *summary.Summary) {
	if out == nil {
		return
	}
	branch.ForEachPathEvidence(func(proof factflow.BranchPathEvidence) bool {
		if proof.Kind() != factflow.BranchPathEvidencePresence || !proof.ActiveOnEdge(cond) {
			return true
		}
		value, ok := proof.Presence()
		if !ok {
			return true
		}
		path := proof.PathRef()
		for index, param := range ctx.plan.BoundaryParams() {
			if path.Symbol != param || path.Version != 0 || len(path.Segments) != 0 {
				continue
			}
			candidate := callboundary.BranchProof{
				Kind: pathevidence.BranchProofPathPresence, Path: pathdom.NewPlaceholder(index), Presence: value,
			}
			if !containsCompilerBranchProof(out.NormalReturnFacts.BranchProofs, candidate) {
				out.NormalReturnFacts.BranchProofs = append(out.NormalReturnFacts.BranchProofs, candidate)
			}
			break
		}
		return true
	})
}

func containsCompilerBranchProof(proofs []callboundary.BranchProof, candidate callboundary.BranchProof) bool {
	for _, proof := range proofs {
		if proof.Kind == candidate.Kind && proof.Path.Equal(candidate.Path) && proof.Other.Equal(candidate.Other) &&
			presence.Equal(proof.Presence, candidate.Presence) {
			return true
		}
	}
	return false
}

func lowerCompilerBranchRefinements(arena *Arena, branch factapply.BranchAlgebra, cond bool, ctx planCompileContext, condition ValueTerm) ([]SymbolicBranchRefinement, error) {
	var out []SymbolicBranchRefinement
	for _, active := range branch.ActiveRefinements(cond) {
		target := active.TargetPathRef()
		value, ok := latestSymbolicPathValue(out, target)
		if !ok {
			value, ok = exactCompilerPathTerm(ctx, target)
		}
		if !ok {
			return nil, fmt.Errorf("branch: contextual-refinement-path")
		}
		refinement := active.Refinement()
		if refinement.NegatedLiteral() {
			// Literal exclusion is relational at every path depth, not a product
			// meet.  The canonical branch transaction carries and applies that
			// relation (including descendant-origin narrowing); the symbolic
			// environment only needs positive product refinements.
			continue
		}
		baseValue := value
		value, ok = arena.RefineValue(value, refinement)
		if !ok {
			_, hasConstraint := refinement.Constraint()
			return nil, fmt.Errorf("branch: contextual-refinement-kind negated=%t falsy-absent=%t constraint=%t value=%d condition=%d", refinement.NegatedLiteral(), refinement.FalsyAbsent(), hasConstraint, baseValue, condition)
		}
		out = append(out, SymbolicBranchRefinement{target: target, value: value})
	}
	return out, nil
}

func validateRepresentedBranchEvidence(ctx planCompileContext, branch factapply.BranchAlgebra, condition ValueTerm) error {
	var validationErr error
	branchCondition, hasCondition := branch.Condition()
	if !hasCondition {
		return fmt.Errorf("branch:missing-condition-source")
	}
	proofCondition := condition
	conditionSource := branchCondition.Source()
	if conditionSource.Kind == factflow.ValueSourceExpression && conditionSource.HasExpr {
		if _, certified := ctx.predicateExpressions[conditionSource.ExprRef]; certified {
			var exact bool
			proofCondition, exact = exactPredicateProofTerm(ctx, conditionSource)
			if !exact {
				return fmt.Errorf("branch: contextual-path-evidence-certificate")
			}
		}
	}
	branch.ForEachPathEvidence(func(proof factflow.BranchPathEvidence) bool {
		activeEdge, exactEdge := singleActiveEvidenceEdge(proof.ActiveOnEdge(true), proof.ActiveOnEdge(false))
		if !exactEdge {
			validationErr = fmt.Errorf("branch: contextual-path-evidence-polarity")
			return false
		}
		predicateTruthy := branchCondition.TruthyOnEdge(activeEdge)
		switch validateBranchPathEvidenceSource(ctx, conditionSource, proofCondition, proof, predicateTruthy) {
		case branchPathEvidenceSourceEntailed:
			return true
		case branchPathEvidenceSourcePathMismatch:
			validationErr = fmt.Errorf("branch: contextual-path-evidence-path")
			return false
		case branchPathEvidenceSourcePolarityMismatch:
			validationErr = fmt.Errorf("branch: contextual-path-evidence-polarity")
			return false
		}
		_, ok := exactCompilerPathTerm(ctx, proof.PathRef())
		if !ok {
			validationErr = fmt.Errorf("branch: contextual-path-evidence-path")
			return false
		}
		terms := exactPredicatePathTerms(ctx, proofCondition, proof.PathRef())
		var directCondition, typePredicatePath, scalarPredicatePath, notPredicatePath, truthyEvidence bool
		var typePredicate exactTypePredicate
		var scalarPredicate exactScalarPredicate
		var typePredicateTruthy, scalarPredicateTruthy bool
		for _, candidate := range terms {
			candidateDirect := candidate == proofCondition
			candidateType, candidateTypeTruthy, candidateTypePath := exactTypePredicateEntailed(ctx, proofCondition, candidate, predicateTruthy)
			candidateScalar, candidateScalarTruthy, candidateScalarPath := exactScalarPredicateEntailed(ctx, proofCondition, candidate, predicateTruthy)
			candidateNot := exactNotPredicateEvidence(ctx, proofCondition, candidate)
			candidateTruthy := exactTruthyEvidenceEntailed(ctx, proofCondition, candidate, predicateTruthy)
			if !candidateDirect && !candidateTypePath && !candidateScalarPath && !candidateNot && !candidateTruthy {
				continue
			}
			directCondition = candidateDirect
			typePredicate, typePredicatePath = candidateType, candidateTypePath
			scalarPredicate, scalarPredicatePath = candidateScalar, candidateScalarPath
			typePredicateTruthy, scalarPredicateTruthy = candidateTypeTruthy, candidateScalarTruthy
			notPredicatePath, truthyEvidence = candidateNot, candidateTruthy
			break
		}
		otherPath, hasOtherPath := proof.OtherPathRef()
		var pathRelation exactPathRelation
		var pathRelationPath bool
		if hasOtherPath {
			if _, exactOtherTerm := exactCompilerPathTerm(ctx, otherPath); exactOtherTerm {
				otherTerms := exactPredicatePathTerms(ctx, proofCondition, otherPath)
				for _, left := range terms {
					for _, right := range otherTerms {
						if relation, exact := exactPathRelationEvidence(ctx, proofCondition, left, right, predicateTruthy); exact {
							pathRelation, pathRelationPath = relation, true
							break
						}
					}
					if pathRelationPath {
						break
					}
				}
			}
		}
		if !directCondition && !typePredicatePath && !scalarPredicatePath && !notPredicatePath && !truthyEvidence && !pathRelationPath {
			validationErr = fmt.Errorf("branch: contextual-path-evidence-path")
			return false
		}
		switch proof.Kind() {
		case factflow.BranchPathEvidenceEqual, factflow.BranchPathEvidenceNotEqual:
			if !pathRelationPath {
				validationErr = fmt.Errorf("branch: contextual-path-evidence-kind %d", proof.Kind())
				return false
			}
			wantEqual := proof.Kind() == factflow.BranchPathEvidenceEqual
			if pathRelation.equalityHolds != wantEqual {
				validationErr = fmt.Errorf("branch: contextual-path-evidence-polarity")
				return false
			}
			return true
		case factflow.BranchPathEvidenceTruthy:
			if !directCondition && !truthyEvidence {
				validationErr = fmt.Errorf("branch: contextual-path-evidence-kind %d", proof.Kind())
				return false
			}
			if truthyEvidence {
				return true
			}
			// The truthy row guard is the exact durable representation.
		case factflow.BranchPathEvidencePresence:
			value, hasPresence := proof.Presence()
			if !hasPresence {
				validationErr = fmt.Errorf("branch: contextual-path-evidence-presence")
				return false
			}
			if scalarPredicatePath {
				expected, entailed := scalarPredicate.entailedPresence(scalarPredicateTruthy)
				if !entailed || !presence.Equal(value, expected) {
					validationErr = fmt.Errorf("branch: contextual-path-evidence-presence")
					return false
				}
				// Exact scalar equality owns both sides of the nil partition.
				return true
			}
			if !presence.Equal(value, presence.Present()) {
				validationErr = fmt.Errorf("branch: contextual-path-evidence-presence")
				return false
			}
		default:
			validationErr = fmt.Errorf("branch: contextual-path-evidence-kind %d", proof.Kind())
			return false
		}
		if truthyEvidence {
			return true
		}
		if directCondition {
			// A direct condition establishes truthiness (and therefore presence)
			// only when the canonical source is truthy on this CFG edge.
			if !predicateTruthy {
				validationErr = fmt.Errorf("branch: contextual-path-evidence-polarity")
				return false
			}
			return true
		}
		// A type predicate can establish root presence on either predicate
		// outcome. The exact implication is derived from the canonical term;
		// syntax and mutable concrete state are never consulted here.
		if proof.Kind() != factflow.BranchPathEvidencePresence ||
			(typePredicatePath && !typePredicate.impliesPresence(typePredicateTruthy)) ||
			(notPredicatePath && predicateTruthy) {
			validationErr = fmt.Errorf("branch: contextual-path-evidence-polarity")
			return false
		}
		return true
	})
	return validationErr
}

// exactPredicateProofTerm reconstructs the immutable scalar DAG certified by
// prepareCertifiedScalarExpressions. It is deliberately separate from the branch's
// executable condition term: a structural and/or executes through its owned
// ExpressionValue cell, while this term is consulted only to prove which path
// evidence that Boolean result entails. Conditional operands are therefore
// certificate atoms here; this DAG is never installed as a WorldProgram read.
func exactPredicateProofTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, bool) {
	return exactPredicateProofTermActive(ctx, source, make(map[factflow.ExprRef]bool))
}

func exactPredicateProofTermActive(ctx planCompileContext, source factflow.ValueSource, active map[factflow.ExprRef]bool) (ValueTerm, bool) {
	if !source.Valid() || ctx.builder == nil {
		return 0, false
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		_, certified := ctx.predicateExpressions[source.ExprRef]
		operation, hasOperation := ctx.facts.ExpressionOperation(source.ExprRef)
		if certified && hasOperation {
			if active[source.ExprRef] {
				return 0, false
			}
			if operation.Kind() == factflow.ExpressionOperationBinary && (operation.Op() == "and" || operation.Op() == "or") {
				if _, exactRegion := ctx.plan.StructuralExpressionRegion(source.ExprRef); !exactRegion {
					return 0, false
				}
			}
			active[source.ExprRef] = true
			left, leftExact := exactPredicateProofTermActive(ctx, operation.Left(), active)
			if !leftExact {
				delete(active, source.ExprRef)
				return 0, false
			}
			arena := ctx.builder.Arena()
			var term ValueTerm
			var exact bool
			switch operation.Kind() {
			case factflow.ExpressionOperationUnary:
				identity, intrinsicOperation := operation.Intrinsic()
				if intrinsicOperation {
					if identity != intrinsic.LuaType {
						delete(active, source.ExprRef)
						return 0, false
					}
					term, exact = arena.LuaTypeNameValue(left), true
				} else {
					term, exact = arena.ScalarUnaryValue(operation.Op(), left)
				}
			case factflow.ExpressionOperationBinary:
				right, rightExact := exactPredicateProofTermActive(ctx, operation.Right(), active)
				if !rightExact {
					delete(active, source.ExprRef)
					return 0, false
				}
				term, exact = arena.ScalarBinaryValue(operation.Op(), left, right)
			default:
				delete(active, source.ExprRef)
				return 0, false
			}
			delete(active, source.ExprRef)
			return term, exact && term != 0
		}
	}
	term, err := exactCompilerSourceTermActive(ctx, source, active)
	return term, err == nil && term != 0
}

type exactPathRelation struct {
	equalityHolds bool
}

// exactPredicatePathTerms returns the source-authored terms for one path that
// actually occur in the current predicate DAG. A point-sensitive path read
// rebuilt at the branch head is the value to refine, but it need not have the
// same term identity as the earlier read whose truth value selected the edge.
func exactPredicatePathTerms(ctx planCompileContext, condition ValueTerm, target pathdom.Path) []ValueTerm {
	if ctx.builder == nil || condition == 0 || target.Symbol == 0 {
		return nil
	}
	arena := ctx.builder.Arena()
	set := make(map[ValueTerm]struct{})
	add := func(term ValueTerm) {
		if term != 0 && valueTermContains(arena, condition, term, make(map[ValueTerm]bool)) {
			set[term] = struct{}{}
		}
	}
	if term, exact := exactCompilerPathTerm(ctx, target); exact {
		add(term)
		collectEquivalentPredicatePathTerms(arena, condition, term, set, make(map[ValueTerm]bool))
	}
	ctx.facts.ForEachExpressionPath(func(ref factflow.ExprRef, candidate pathdom.Path) bool {
		if !candidate.Equal(target) {
			return true
		}
		for _, term := range ctx.expressions[ref] {
			add(term)
		}
		return true
	})
	out := make([]ValueTerm, 0, len(set))
	for term := range set {
		out = append(out, term)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func collectEquivalentPredicatePathTerms(arena *Arena, root, reference ValueTerm, out map[ValueTerm]struct{}, active map[ValueTerm]bool) {
	if arena == nil || root == 0 || reference == 0 || int(root) >= len(arena.values) || int(reference) >= len(arena.values) || active[root] {
		return
	}
	active[root] = true
	defer delete(active, root)
	if samePredicatePathRead(arena, root, reference) {
		out[root] = struct{}{}
	}
	for _, child := range arena.values[root].args {
		collectEquivalentPredicatePathTerms(arena, child, reference, out, active)
	}
}

// samePredicatePathRead compares the immutable path identity of two reads but
// deliberately ignores their evaluation point. Branch evidence is emitted for
// the source read and replayed at the branch head; those terms have different
// points while naming the same owner/path/key transaction.
func samePredicatePathRead(arena *Arena, left, right ValueTerm) bool {
	if arena == nil || left == 0 || right == 0 || int(left) >= len(arena.values) || int(right) >= len(arena.values) {
		return false
	}
	l, r := arena.values[left], arena.values[right]
	if (l.op != valueDynamicRead && l.op != valueDynamicTableRead) ||
		(r.op != valueDynamicRead && r.op != valueDynamicTableRead) ||
		l.path == 0 || l.path != r.path || len(l.args) < 2 || len(r.args) < 2 {
		return false
	}
	return l.args[1] == r.args[1] && l.keyPath == r.keyPath
}

func valueTermContains(arena *Arena, root, target ValueTerm, active map[ValueTerm]bool) bool {
	if root == target {
		return true
	}
	if arena == nil || root == 0 || int(root) >= len(arena.values) || active[root] {
		return false
	}
	active[root] = true
	defer delete(active, root)
	for _, child := range arena.values[root].args {
		if valueTermContains(arena, child, target, active) {
			return true
		}
	}
	return false
}

// exactPathRelationEvidence recognizes equality between the two canonical path
// terms carried by a branch evidence fact. It proves only the relation selected
// by the branch's own scalar comparison; no type, syntax, or mutable-state
// inference is involved.
func exactPathRelationEvidence(ctx planCompileContext, condition, left, right ValueTerm, predicateTruthy bool) (exactPathRelation, bool) {
	if ctx.builder == nil || condition == 0 || left == 0 || right == 0 {
		return exactPathRelation{}, false
	}
	arena := ctx.builder.Arena()
	var prove func(ValueTerm, bool, map[ValueTerm]bool) (exactPathRelation, bool)
	prove = func(term ValueTerm, truthy bool, active map[ValueTerm]bool) (exactPathRelation, bool) {
		if term == 0 || int(term) >= len(arena.values) || active[term] {
			return exactPathRelation{}, false
		}
		active[term] = true
		defer delete(active, term)
		node := arena.values[term]
		if node.op == valueBinaryOperation && (node.operator == "==" || node.operator == "~=") && len(node.args) == 2 &&
			((node.args[0] == left && node.args[1] == right) || (node.args[0] == right && node.args[1] == left)) {
			return exactPathRelation{equalityHolds: (node.operator == "==") == truthy}, true
		}
		switch node.op {
		case valueUnaryOperation:
			if node.operator == "not" && len(node.args) == 1 {
				return prove(node.args[0], !truthy, active)
			}
		case valueBinaryOperation:
			if len(node.args) != 2 {
				return exactPathRelation{}, false
			}
			if node.operator == "and" && truthy || node.operator == "or" && !truthy {
				if relation, ok := prove(node.args[0], truthy, active); ok {
					return relation, true
				}
				return prove(node.args[1], truthy, active)
			}
		}
		return exactPathRelation{}, false
	}
	return prove(condition, predicateTruthy, make(map[ValueTerm]bool))
}

// exactTruthyEvidenceEntailed proves only conjunction-shaped truthiness facts
// that hold on every concrete execution of the selected result. For `or ==
// false` both operands are false; for `and == true` both are true. The other
// outcomes are disjunctive and deliberately remain unproven here.
func exactTruthyEvidenceEntailed(ctx planCompileContext, condition, evidence ValueTerm, predicateTruthy bool) bool {
	if ctx.builder == nil || condition == 0 || evidence == 0 {
		return false
	}
	arena := ctx.builder.Arena()
	var prove func(ValueTerm, bool, map[ValueTerm]bool) bool
	prove = func(term ValueTerm, truthy bool, active map[ValueTerm]bool) bool {
		if term == evidence {
			return truthy
		}
		if term == 0 || int(term) >= len(arena.values) || active[term] {
			return false
		}
		active[term] = true
		defer delete(active, term)
		node := arena.values[term]
		switch node.op {
		case valueUnaryOperation:
			return node.operator == "not" && len(node.args) == 1 && prove(node.args[0], !truthy, active)
		case valueBinaryOperation:
			switch node.operator {
			case "or":
				return !truthy && len(node.args) == 2 &&
					(prove(node.args[0], false, active) || prove(node.args[1], false, active))
			case "and":
				return truthy && len(node.args) == 2 &&
					(prove(node.args[0], true, active) || prove(node.args[1], true, active))
			}
			return false
		default:
			return false
		}
	}
	return prove(condition, predicateTruthy, make(map[ValueTerm]bool))
}

func exactNotPredicateEvidence(ctx planCompileContext, condition, evidence ValueTerm) bool {
	if ctx.builder == nil || condition == 0 || evidence == 0 {
		return false
	}
	arena := ctx.builder.Arena()
	if int(condition) >= len(arena.values) {
		return false
	}
	node := arena.values[condition]
	return node.op == valueUnaryOperation && node.operator == "not" && len(node.args) == 1 && node.args[0] == evidence
}

// exactScalarPredicate records which truth value of an exact scalar equality
// proves the compared path present.  Equality with a non-nil literal proves
// presence when equality holds; equality with nil proves it when equality does
// not hold.  No type or syntax inference is involved: both operands are terms
// from the canonical branch condition.
type exactScalarPredicate struct {
	equal          bool
	literalPresent bool
}

func (p exactScalarPredicate) entailedPresence(predicateTruthy bool) (presence.Value, bool) {
	equalityHolds := p.equal == predicateTruthy
	if equalityHolds {
		if p.literalPresent {
			return presence.Present(), true
		}
		return presence.Absent(), true
	}
	if !p.literalPresent {
		return presence.Present(), true
	}
	// A value unequal to one non-nil literal can still be nil or any other
	// non-nil value, so this outcome has no exact presence consequence.
	return presence.Bottom(), false
}

func exactScalarPredicateEvidence(ctx planCompileContext, condition, evidence ValueTerm) (exactScalarPredicate, bool) {
	if ctx.builder == nil || condition == 0 || evidence == 0 {
		return exactScalarPredicate{}, false
	}
	arena := ctx.builder.Arena()
	if int(condition) >= len(arena.values) {
		return exactScalarPredicate{}, false
	}
	node := arena.values[condition]
	if node.op != valueBinaryOperation || (node.operator != "==" && node.operator != "~=") || len(node.args) != 2 {
		return exactScalarPredicate{}, false
	}
	for index, arg := range node.args {
		if arg != evidence {
			continue
		}
		literalTerm := node.args[1-index]
		if literalTerm == 0 || int(literalTerm) >= len(arena.values) {
			return exactScalarPredicate{}, false
		}
		literalNode := arena.values[literalTerm]
		if literalNode.op != valueConstant {
			return exactScalarPredicate{}, false
		}
		literalPresence := product.PresenceOf(literalNode.value)
		switch {
		case presence.Equal(literalPresence, presence.Present()):
			return exactScalarPredicate{equal: node.operator == "==", literalPresent: true}, true
		case presence.Equal(literalPresence, presence.Absent()):
			return exactScalarPredicate{equal: node.operator == "==", literalPresent: false}, true
		default:
			return exactScalarPredicate{}, false
		}
	}
	return exactScalarPredicate{}, false
}

func exactScalarPredicateEntailed(ctx planCompileContext, condition, evidence ValueTerm, predicateTruthy bool) (exactScalarPredicate, bool, bool) {
	if ctx.builder == nil || condition == 0 || evidence == 0 {
		return exactScalarPredicate{}, false, false
	}
	arena := ctx.builder.Arena()
	var prove func(ValueTerm, bool, map[ValueTerm]bool) (exactScalarPredicate, bool, bool)
	prove = func(term ValueTerm, truthy bool, active map[ValueTerm]bool) (exactScalarPredicate, bool, bool) {
		if term == 0 || int(term) >= len(arena.values) || active[term] {
			return exactScalarPredicate{}, false, false
		}
		if predicate, exact := exactScalarPredicateEvidence(ctx, term, evidence); exact {
			if _, entailed := predicate.entailedPresence(truthy); entailed {
				return predicate, truthy, true
			}
		}
		active[term] = true
		defer delete(active, term)
		node := arena.values[term]
		switch node.op {
		case valueUnaryOperation:
			if node.operator == "not" && len(node.args) == 1 {
				return prove(node.args[0], !truthy, active)
			}
		case valueBinaryOperation:
			if len(node.args) == 2 && (node.operator == "and" && truthy || node.operator == "or" && !truthy) {
				if predicate, selected, exact := prove(node.args[0], truthy, active); exact {
					return predicate, selected, true
				}
				return prove(node.args[1], truthy, active)
			}
		}
		return exactScalarPredicate{}, false, false
	}
	return prove(condition, predicateTruthy, make(map[ValueTerm]bool))
}

func singleActiveEvidenceEdge(activeTrue, activeFalse bool) (bool, bool) {
	if activeTrue == activeFalse {
		return false, false
	}
	return activeTrue, true
}

type exactTypePredicate struct {
	equal bool
	tag   runtimekind.Tag
}

func (p exactTypePredicate) impliesPresence(predicateTruthy bool) bool {
	equalityHolds := p.equal == predicateTruthy
	if equalityHolds {
		return p.tag != runtimekind.Nil
	}
	return p.tag == runtimekind.Nil
}

// exactTypePredicateEvidence recognizes an exact certified Lua type comparison
// over evidence. The condition term already came from the branch's canonical
// source and immutable predicate allow-set, so structural term identity—not
// AST/CFG spelling—is the authority shared by statement and expression forms.
// The returned predicate describes both outcomes; callers decide whether the
// active predicate truth value entails the requested evidence.
func exactTypePredicateEvidence(ctx planCompileContext, condition, evidence ValueTerm) (exactTypePredicate, bool) {
	if ctx.builder == nil || condition == 0 || evidence == 0 {
		return exactTypePredicate{}, false
	}
	arena := ctx.builder.Arena()
	if int(condition) >= len(arena.values) {
		return exactTypePredicate{}, false
	}
	node := arena.values[condition]
	if node.op != valueBinaryOperation || (node.operator != "==" && node.operator != "~=") || len(node.args) != 2 {
		return exactTypePredicate{}, false
	}
	for index, arg := range node.args {
		if arg == 0 || int(arg) >= len(arena.values) {
			continue
		}
		typeNode := arena.values[arg]
		if typeNode.op != valueLuaTypeName || len(typeNode.args) != 1 || typeNode.args[0] != evidence {
			continue
		}
		literalTerm := node.args[1-index]
		if literalTerm == 0 || int(literalTerm) >= len(arena.values) {
			return exactTypePredicate{}, false
		}
		literalNode := arena.values[literalTerm]
		if literalNode.op != valueConstant {
			return exactTypePredicate{}, false
		}
		name, exact := typevalue.StringLiteralOf(ctx.registry, literalNode.value)
		tag, validTag := runtimekind.ParseTag(name)
		if !exact || !validTag {
			return exactTypePredicate{}, false
		}
		return exactTypePredicate{equal: node.operator == "==", tag: tag}, true
	}
	return exactTypePredicate{}, false
}

func exactTypePredicateEntailed(ctx planCompileContext, condition, evidence ValueTerm, predicateTruthy bool) (exactTypePredicate, bool, bool) {
	if ctx.builder == nil || condition == 0 || evidence == 0 {
		return exactTypePredicate{}, false, false
	}
	arena := ctx.builder.Arena()
	var prove func(ValueTerm, bool, map[ValueTerm]bool) (exactTypePredicate, bool, bool)
	prove = func(term ValueTerm, truthy bool, active map[ValueTerm]bool) (exactTypePredicate, bool, bool) {
		if term == 0 || int(term) >= len(arena.values) || active[term] {
			return exactTypePredicate{}, false, false
		}
		if predicate, exact := exactTypePredicateEvidence(ctx, term, evidence); exact {
			return predicate, truthy, true
		}
		active[term] = true
		defer delete(active, term)
		node := arena.values[term]
		switch node.op {
		case valueUnaryOperation:
			if node.operator == "not" && len(node.args) == 1 {
				return prove(node.args[0], !truthy, active)
			}
		case valueBinaryOperation:
			if len(node.args) == 2 && (node.operator == "and" && truthy || node.operator == "or" && !truthy) {
				if predicate, selected, exact := prove(node.args[0], truthy, active); exact {
					return predicate, selected, true
				}
				return prove(node.args[1], truthy, active)
			}
		}
		return exactTypePredicate{}, false, false
	}
	return prove(condition, predicateTruthy, make(map[ValueTerm]bool))
}

func exactCompilerPathTerm(ctx planCompileContext, path pathdom.Path) (ValueTerm, bool) {
	term, err := exactCompilerStaticPathTerm(ctx, path)
	return term, err == nil && term != 0
}

func bindBoundaryTerms(ctx *planCompileContext, shape Shape) error {
	if !ctx.plan.BoundaryParamsValid() {
		return fmt.Errorf("parameter boundary is malformed")
	}
	if !ctx.plan.BoundaryCapturesValid() {
		return fmt.Errorf("capture boundary is malformed")
	}
	if !ctx.plan.BoundaryGlobalsValid() {
		return fmt.Errorf("global boundary is malformed")
	}
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
	captures := ctx.plan.BoundaryCaptures()
	if len(captures) != int(shape.Captures) {
		return fmt.Errorf("capture symbols %d != shape captures %d", len(captures), shape.Captures)
	}
	for index, capture := range captures {
		if _, exists := ctx.locals[capture]; exists {
			return fmt.Errorf("duplicate capture symbol %d", capture)
		}
		ctx.locals[capture] = ctx.builder.Arena().Root(Root{Kind: RootCapture, Index: uint32(index)})
	}
	globals := ctx.plan.BoundaryGlobals()
	if len(globals) != int(shape.Globals) {
		return fmt.Errorf("global symbols %d != shape globals %d", len(globals), shape.Globals)
	}
	for index, global := range globals {
		if _, exists := ctx.locals[global]; exists {
			return fmt.Errorf("duplicate global symbol %d", global)
		}
		ctx.locals[global] = ctx.builder.Arena().Root(Root{Kind: RootGlobal, Index: uint32(index)})
	}
	return nil
}

// bindBoundaryParamTerms is retained for focused package tests and the
// eligibility census; boundary binding is now one atomic three-namespace
// transaction despite the historical name.
func bindBoundaryParamTerms(ctx *planCompileContext, shape Shape) error {
	return bindBoundaryTerms(ctx, shape)
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

type expressionOperationPlanHandler struct{}

func (expressionOperationPlanHandler) Kind() operationplan.Kind {
	return operationplan.ExpressionOperation
}

type expressionConditionPlanHandler struct{}

func (expressionConditionPlanHandler) Kind() operationplan.Kind {
	return operationplan.ExpressionCondition
}
func (expressionConditionPlanHandler) Preflight(planCompileContext, cfg.Point) error { return nil }
func (expressionConditionPlanHandler) Lower(planCompileContext, cfg.Point, *[]Operation) error {
	return nil
}
func (expressionOperationPlanHandler) Preflight(planCompileContext, cfg.Point) error { return nil }
func (expressionOperationPlanHandler) Lower(planCompileContext, cfg.Point, *[]Operation) error {
	return nil
}

func (returnPlanHandler) Preflight(ctx planCompileContext, point cfg.Point) error {
	if _, ok := ctx.facts.Return(point); !ok {
		// ExpressionValue is registered to this handler as a dependency and has
		// no point-local preflight.
		return nil
	}
	_, err := compileReturnTransactionTerm(ctx, point)
	return err
}

func (returnPlanHandler) Lower(ctx planCompileContext, point cfg.Point, operations *[]Operation) error {
	fact, ok := ctx.facts.Return(point)
	if !ok {
		return nil
	}
	if ctx.returnTransaction == nil {
		return fmt.Errorf("point %d return has no N5 transaction sink", point)
	}
	transaction, err := compileReturnTransactionTerm(ctx, point)
	if err != nil {
		return err
	}
	if ctx.returnTransaction.transaction.Valid() {
		return fmt.Errorf("point %d has multiple return transactions", point)
	}
	*ctx.returnTransaction = transaction
	for i, source := range fact.Sources() {
		if exactBoundaryMemberZeroResultReturn(ctx, source) {
			continue
		}
		appendReturnedParamRefinements(&ctx, source, i)
	}
	return nil
}

// appendReturnedParamRefinements mirrors the concrete summary projector for
// an exact returned source. Only certified parameter symbols become boundary
// placeholders; locals, captures, globals, and malformed roots cannot leak.
func appendReturnedParamRefinements(ctx *planCompileContext, source factflow.ValueSource, returnIndex int) {
	if ctx == nil || ctx.rowOutput == nil && ctx.structuralOutput == nil || !source.HasExpr ||
		(source.Kind != factflow.ValueSourceExpression && !(source.Kind == factflow.ValueSourceCall && source.ResultIndex == 0)) {
		return
	}
	condition, ok := ctx.facts.ExpressionCondition(source.ExprRef)
	if !ok {
		return
	}
	params := ctx.plan.BoundaryParams()
	for _, selected := range []bool{true, false} {
		for _, refinement := range condition.FactsForValue(selected).Refinements() {
			target := refinement.TargetPath()
			value, exact := refinement.Value().Constraint()
			if !exact || target.Symbol == 0 {
				continue
			}
			for index, param := range params {
				if param == 0 || target.Symbol != param {
					continue
				}
				if ctx.structuralOutput != nil {
					ctx.structuralOutput.returnConditions = append(ctx.structuralOutput.returnConditions, returnConditionParamRefinementTerm{
						ReturnIndex: returnIndex, ReturnValue: selected,
						Target: pathdom.NewPlaceholder(index).AppendSegments(target.Segments), Value: value,
					})
				} else {
					ctx.rowOutput.ReturnConditionParamRefinements = append(ctx.rowOutput.ReturnConditionParamRefinements, summary.ReturnConditionParamRefinement{
						ReturnIndex: returnIndex, ReturnValue: selected,
						Target: pathdom.NewPlaceholder(index).AppendSegments(target.Segments), Value: value,
					})
				}
				break
			}
		}
	}
}

func exactReturnSourceTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, error) {
	if source.Kind == factflow.ValueSourceCall && source.HasCallPoint && source.ResultIndex >= 0 {
		if term, dependent, err := exactDependentCallResultTerm(ctx, source.CallPoint, source.ResultIndex); err != nil {
			return 0, err
		} else if dependent {
			return term, nil
		}
	}
	if term, ok := exactSignatureExpressionTerm(ctx, source); ok {
		return term, nil
	}
	if source.Kind == factflow.ValueSourceCall && source.HasCallPoint {
		if calls, sealed := ctx.plan.CallSurface(); sealed {
			if surface, present := calls.Site(source.CallPoint); present {
				if _, lexical := surface.Target.LexicalBody(); lexical {
					term, err := exactReturnCallResultTerm(ctx, source)
					if err != nil {
						return 0, err
					}
					return term, nil
				}
				if source.ResultIndex >= 0 {
					if term, exact, err := exactExternalCallResultTerm(ctx, source); err != nil {
						return 0, err
					} else if exact {
						return term, nil
					}
				}
			}
		}
	}
	return exactCompilerSourceTerm(ctx, source)
}

// exactReturnCallResultTerm consumes the frame-result root minted by the
// direct-call composition step. Open-tail and expanded returns are admitted
// only one explicitly enumerated Return target at a time; adjusted calls are
// scalar and therefore can only consume result slot zero. The factflow call
// surface, not source syntax, owns these value-list rules.
func exactReturnCallResultTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, error) {
	if !source.Valid() || !source.HasCallPoint || source.ResultIndex < 0 {
		return 0, fmt.Errorf("non-scalar return source")
	}
	site, ok := ctx.facts.CallSiteView(source.CallPoint)
	if !ok {
		return 0, fmt.Errorf("call return source has no exact value-list authority")
	}
	directReturn := site.Context() == factflow.CallSiteContextReturnSource &&
		source.Final == site.Final() && source.Expanded == site.Expanded() &&
		source.Adjusted == site.Adjusted() && source.OpenTail == site.OpenTail()
	expressionResult := site.Context() == factflow.CallSiteContextExpressionProducer
	if !directReturn && !expressionResult {
		return 0, fmt.Errorf("call return source has no exact value-list authority")
	}
	if source.Adjusted && source.ResultIndex != 0 {
		return 0, fmt.Errorf("adjusted call return source selects result %d", source.ResultIndex)
	}
	targeted := false
	for index := 0; index < site.ResultTargetCount(); index++ {
		target, present := site.ResultTargetAt(index)
		if !present || target.ResultIndex() != source.ResultIndex ||
			directReturn && target.Kind() != factflow.CallResultTargetReturn ||
			expressionResult && target.Kind() != factflow.CallResultTargetExpression {
			continue
		}
		if directReturn && source.TargetIndex >= 0 && target.Index() != source.TargetIndex {
			continue
		}
		targeted = true
		break
	}
	// An open tail call implicitly targets every remaining declared return
	// coordinate.  Those coordinates are frozen by deferStructuralCall even
	// though factflow has only one explicit AST result target.
	implicitOpenTailTarget := directReturn && site.OpenTail() && source.TargetIndex >= 0 && source.TargetIndex < planReturnArity(ctx.plan)
	if !targeted && !implicitOpenTailTarget {
		return 0, fmt.Errorf("call result %d has no exact return target", source.ResultIndex)
	}
	term, bound := ctx.resultRoots[ResultRoot{Point: source.CallPoint, Slot: uint32(source.ResultIndex)}]
	if !bound || term == 0 {
		return 0, fmt.Errorf("call result %d has no frozen frame root", source.ResultIndex)
	}
	return term, nil
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
	if fact.TargetSymbol() == 0 {
		return fmt.Errorf("assignment target has no symbol")
	}
	target := fact.TargetPathRef()
	if target.Symbol != fact.TargetSymbol() || target.Version != 0 || len(target.Segments) != 0 {
		return fmt.Errorf("assignment target is not its canonical root symbol")
	}
	if !exactDirectLexicalDeclaration(ctx, fact) {
		if _, err := compileRootAssignmentTerm(ctx, point); err != nil {
			return err
		}
	}
	switch fact.Kind() {
	case factflow.RootAssignmentLocalDeclaration:
		if exactDirectLexicalDeclaration(ctx, fact) {
			return nil
		}
		return nil
	case factflow.RootAssignmentOrdinaryRootWrite:
		return nil
	default:
		return fmt.Errorf("root assignment kind %d has no exact symbolic semantics", fact.Kind())
	}
}
func (rootAssignmentPlanHandler) Lower(ctx planCompileContext, point cfg.Point, _ *[]Operation) error {
	fact, ok := ctx.facts.RootAssignment(point)
	if !ok {
		return fmt.Errorf("missing root-assignment payload")
	}
	if exactDirectLexicalDeclaration(ctx, fact) {
		// The function value has no observable value use: its complete use set
		// is represented by the sealed direct-call surface. Do not retain a
		// synthetic closure value in the symbolic local environment.
		return nil
	}
	if ctx.rootAssignment == nil {
		return fmt.Errorf("point %d root assignment has no N4 transaction sink", point)
	}
	transaction, err := compileRootAssignmentTerm(ctx, point)
	if err != nil {
		return err
	}
	if ctx.rootAssignment.transaction.Valid() {
		return fmt.Errorf("point %d has multiple root-assignment transactions", point)
	}
	*ctx.rootAssignment = transaction
	term, exact := ctx.builder.Arena().environmentValue(fact.TargetSymbol())
	if !exact || term == 0 {
		return fmt.Errorf("root target %d has no sealed post-N4 environment term", fact.TargetSymbol())
	}
	if ctx.structuralOutput != nil {
		ctx.structuralOutput.paramObligations = append(
			ctx.structuralOutput.paramObligations,
			boundaryConcatParamObligations(ctx, transaction.sources[0])...,
		)
	}
	if fact.Kind() == factflow.RootAssignmentOrdinaryRootWrite {
		if _, exists := ctx.locals[fact.TargetSymbol()]; !exists {
			return fmt.Errorf("symbol %d ordinary write precedes its declaration", fact.TargetSymbol())
		}
		ctx.locals[fact.TargetSymbol()] = term
		return nil
	}
	if prior, exists := ctx.locals[fact.TargetSymbol()]; exists {
		if ctx.structuralEnvironment {
			node := ctx.builder.Arena().values[prior]
			if node.op == valueEnvironment && node.slot == statekey.SymbolValue(fact.TargetSymbol()) {
				ctx.locals[fact.TargetSymbol()] = term
				return nil
			}
		}
		// A lexical declaration is revisited by cyclic row closure. Replaying
		// the identical interned binding is an idempotent transfer, while a
		// different binding remains an unsupported multi-write.
		if prior == term {
			return nil
		}
		return fmt.Errorf("symbol %d has multiple writes", fact.TargetSymbol())
	}
	ctx.locals[fact.TargetSymbol()] = term
	return nil
}

func exactDirectLexicalDeclaration(ctx planCompileContext, fact factflow.RootAssignment) bool {
	if ctx.plan == nil || fact.Kind() != factflow.RootAssignmentLocalDeclaration || fact.TargetSymbol() == 0 {
		return false
	}
	source := fact.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr || source.ExprRef == 0 || source.ResultIndex != 0 ||
		source.Expanded || source.OpenTail {
		return false
	}
	function, ok := ctx.facts.ExpressionFunction(source.ExprRef)
	return ok && ctx.directDeclarations.Contains(ctx.plan, source.ExprRef, function, fact.TargetSymbol())
}

type expressionFunctionPlanHandler struct{}

func (expressionFunctionPlanHandler) Kind() operationplan.Kind {
	return operationplan.ExpressionFunction
}
func (expressionFunctionPlanHandler) Preflight(planCompileContext, cfg.Point) error { return nil }
func (expressionFunctionPlanHandler) Lower(planCompileContext, cfg.Point, *[]Operation) error {
	return nil
}

func compilerConstantValue(arena *Arena, term ValueTerm) bool {
	return arena != nil && term != 0 && int(term) < len(arena.values) && arena.values[term].op == valueConstant
}

func exactNumericForIteratorBinding(ctx planCompileContext, point cfg.Point, fact factflow.RootAssignment) (ValueTerm, bool) {
	if fact.Kind() != factflow.RootAssignmentLocalDeclaration || fact.Source().Kind != factflow.ValueSourceUnknown ||
		!fact.DeclaredValueContracts() || fact.DeclaredValueOverlays() {
		return 0, false
	}
	declared, ok := fact.DeclaredValue()
	if !ok {
		return 0, false
	}
	annotation, hasAnnotation := fact.DeclaredAnnotationValue()
	want := typevalue.WithWitness(ctx.registry, typevalue.FromType(ctx.registry, typ.Integer), typ.Integer)
	if !product.Equal(ctx.registry, declared, want) || !hasAnnotation || !product.Equal(ctx.registry, annotation, declared) ||
		!numericForBindingPoint(ctx, point, fact.TargetPathRef()) {
		return 0, false
	}
	return ctx.builder.Arena().Constant(declared), true
}

func numericForBindingPoint(ctx planCompileContext, point cfg.Point, target pathdom.Path) bool {
	if numericForHeaderBinding(ctx, point, target) {
		return true
	}
	successors := cfg.SuccessorsReadOnly(ctx.graph, point)
	return len(successors) == 1 && numericForHeaderBinding(ctx, successors[0], target)
}

func numericForHeaderBinding(ctx planCompileContext, point cfg.Point, target pathdom.Path) bool {
	if !ctx.graph.IsBranch(point) {
		return false
	}
	header, ok := ctx.facts.RootAssignment(point)
	if !ok || header.Kind() != factflow.RootAssignmentLocalDeclaration || header.TargetSymbol() != target.Symbol ||
		!header.TargetPathRef().Equal(target) || header.Source().Kind != factflow.ValueSourceUnknown {
		return false
	}
	// The numeric OpIterate lowering owns this exact structural marker: an
	// iterator variable is declared from the unknown continuation cell at a
	// branch point. The inferred/declared value can reflect an invalid bound and
	// numeric floors or array-range evidence are optional refinements; none of
	// them may decide whether the loop header itself exists.
	return true
}

func singleCertifiedAccumulatorWrite(ctx planCompileContext, point cfg.Point, fact factflow.RootAssignment) bool {
	source := fact.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	op, ok := ctx.facts.ExpressionOperation(source.ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationBinary || op.Op() != "+" {
		return false
	}
	left := op.Left()
	leftSymbol, ok := compilerRootSourceSymbol(ctx, left)
	if !ok || leftSymbol != fact.TargetSymbol() {
		return false
	}
	right := op.Right()
	rightSymbol, ok := compilerRootSourceSymbol(ctx, right)
	if !ok || !certifiedNumericForIteratorSymbol(ctx, rightSymbol) {
		return false
	}
	writes := 0
	for raw := 0; raw < ctx.plan.PointCount(); raw++ {
		candidate, exists := ctx.facts.RootAssignment(cfg.Point(raw))
		if !exists || candidate.Kind() != factflow.RootAssignmentOrdinaryRootWrite || candidate.TargetSymbol() != fact.TargetSymbol() {
			continue
		}
		writes++
		if cfg.Point(raw) != point {
			return false
		}
	}
	return writes == 1
}

func compilerRootSourceSymbol(ctx planCompileContext, source factflow.ValueSource) (symbol.ID, bool) {
	switch source.Kind {
	case factflow.ValueSourcePath:
		sym, version, suffix, ok := pathaddr.ParseResolverPath(source.PathKey)
		return sym, ok && sym != 0 && version == 0 && suffix == ""
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			return 0, false
		}
		path, ok := ctx.facts.ExpressionPathRef(source.ExprRef)
		return path.Symbol, ok && path.Symbol != 0 && path.Version == 0 && len(path.Segments) == 0
	default:
		return 0, false
	}
}

func certifiedNumericForIteratorSymbol(ctx planCompileContext, target symbol.ID) bool {
	for raw := 0; raw < ctx.plan.PointCount(); raw++ {
		point := cfg.Point(raw)
		fact, ok := ctx.facts.RootAssignment(point)
		if !ok || fact.TargetSymbol() != target {
			continue
		}
		if _, exact := exactNumericForIteratorBinding(ctx, point, fact); exact {
			return true
		}
	}
	return false
}

func exactCompilerSourceTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, error) {
	return exactCompilerSourceTermActive(ctx, source, nil)
}

func exactCompilerSourceTermActive(ctx planCompileContext, source factflow.ValueSource, active map[factflow.ExprRef]bool) (ValueTerm, error) {
	term, err := exactCompilerSourceTermRaw(ctx, source, active)
	if err == nil || !source.Valid() || !source.Adjusted || source.Expanded || source.OpenTail || source.ResultIndex != 0 {
		if err == nil {
			ctx.valueAccess.record(source, term)
		}
		return term, err
	}
	// An adjusted, closed slot zero is Lua's exact one-value projection. The
	// symbolic producer remains unchanged; normalization only removes the
	// value-list presentation flag after the language has selected that slot.
	normalized := source
	normalized.Adjusted = false
	term, err = exactCompilerSourceTermRaw(ctx, normalized, active)
	if err == nil {
		ctx.valueAccess.record(source, term)
	}
	return term, err
}

func exactCompilerExpressionRefinementTerm(
	ctx planCompileContext,
	ref factflow.ExprRef,
	refinement factflow.ExpressionRefinement,
	active map[factflow.ExprRef]bool,
) (ValueTerm, error) {
	if _, certified := ctx.expressionRefinements[ref]; !certified {
		return 0, fmt.Errorf("expression refinement %d is not certified", ref)
	}
	if active[ref] {
		return 0, fmt.Errorf("cyclic expression source %d", ref)
	}
	if active == nil {
		active = make(map[factflow.ExprRef]bool, 1)
	}
	active[ref] = true
	inner, innerErr := exactCompilerSourceTermActive(ctx, refinement.Source(), active)
	delete(active, ref)
	if innerErr != nil {
		if refinement.Mode() != factflow.ExpressionRefinementRuntimeValidation || !runtimeValidationOwnsUnresolvedScalarSource(refinement.Source()) {
			return 0, fmt.Errorf("expression refinement %d source: %w", ref, innerErr)
		}
		// RuntimeValidation is the exact successful-path authority. Match
		// engine/sourcevalue: when the pre-check producer cannot be resolved,
		// merge the validated contract over lattice Bottom. The resulting term
		// has no invented caller/environment/resource/heap dependency.
		inner = ctx.builder.Arena().Constant(product.Bottom(ctx.registry))
		if inner == 0 {
			return 0, fmt.Errorf("expression refinement %d failed unresolved-source construction", ref)
		}
	}
	term := ctx.builder.Arena().expressionRefinementValue(inner, refinement)
	if term == 0 {
		return 0, fmt.Errorf("expression refinement %d failed symbolic construction", ref)
	}
	return term, nil
}

func exactCompilerSourceTermRaw(ctx planCompileContext, source factflow.ValueSource, active map[factflow.ExprRef]bool) (ValueTerm, error) {
	// A refinement wrapper is the canonical producer for its expression
	// identity. Its ResultPath is only the post-validation address; call-result,
	// local-path, and signature terms below are inputs to the wrapper and must
	// never shadow it merely because they share the outer ExprRef.
	if source.HasExpr && source.ExprRef != 0 {
		if refinement, ok := ctx.facts.ExpressionRefinement(source.ExprRef); ok {
			return exactCompilerExpressionRefinementTerm(ctx, source.ExprRef, refinement, active)
		}
	}
	if source.Kind == factflow.ValueSourceCall && source.HasCallPoint && source.ResultIndex >= 0 {
		if term, bound := ctx.resultRoots[ResultRoot{Point: source.CallPoint, Slot: uint32(source.ResultIndex)}]; bound {
			return term, nil
		}
		if term, exact := exactChannelSelectResultTerm(ctx, source); exact {
			return term, nil
		}
		if term, exact, err := exactExternalCallResultTerm(ctx, source); err != nil {
			return 0, err
		} else if exact {
			return term, nil
		}
	}
	if source.Kind == factflow.ValueSourceCall && source.HasCallPoint && !source.OpenTail && !source.Adjusted {
		if site, ok := ctx.facts.CallSiteView(source.CallPoint); ok {
			for index := 0; index < site.ResultTargetCount(); index++ {
				target, found := site.ResultTargetAt(index)
				if !found || target.ResultIndex() != source.ResultIndex || target.Kind() != factflow.CallResultTargetLocalAssignment ||
					target.TargetSymbol() == 0 || target.TargetPathEmpty() || target.TargetPathSegmentCount() != 0 || target.TargetPath().Symbol != target.TargetSymbol() {
					continue
				}
				if term, bound := ctx.locals[target.TargetSymbol()]; bound {
					return term, nil
				}
			}
		}
	}
	// Deferred call lowering binds each exact frame-result target into row-local
	// symbols before the following declaration fact is replayed. Call sources
	// therefore resolve through the same canonical expression path as ordinary
	// path-backed expressions; no concrete call value is consulted.
	if (source.Kind == factflow.ValueSourceExpression || source.Kind == factflow.ValueSourceCall) && source.HasExpr {
		if p, ok := ctx.facts.ExpressionPathRef(source.ExprRef); ok && p.Symbol != 0 && p.Version == 0 && len(p.Segments) == 0 {
			if term, bound := ctx.locals[p.Symbol]; bound {
				return term, nil
			}
		}
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if literal, object := ctx.facts.ObjectLiteralView(source.ExprRef); object {
			if source.ResultIndex != 0 || source.Expanded || source.OpenTail {
				return 0, fmt.Errorf("object literal %d is not a scalar value", source.ExprRef)
			}
			if _, identified := literal.Identity(); !identified {
				return 0, fmt.Errorf("object literal %d has invalid allocation identity provenance", source.ExprRef)
			}
			if active == nil {
				active = make(map[factflow.ExprRef]bool)
			}
			if active[source.ExprRef] {
				return 0, fmt.Errorf("cyclic object-literal source %d", source.ExprRef)
			}
			active[source.ExprRef] = true
			defer delete(active, source.ExprRef)
			plan, planned := luasourcevalue.CompileObjectLiteralPlanCached(ctx.registry, nil, literal)
			if !planned {
				return 0, fmt.Errorf("object literal %d has no canonical constructor plan", source.ExprRef)
			}
			args := make([]ValueTerm, plan.ValueSourceCount())
			for index := range args {
				raw, present := plan.ValueSourceAt(index)
				if !present {
					return 0, fmt.Errorf("object literal %d constructor source %d is missing", source.ExprRef, index)
				}
				arg, compileErr := exactCompilerSourceTermActive(ctx, raw, active)
				if compileErr != nil {
					return 0, fmt.Errorf("object literal %d constructor source %d: %w", source.ExprRef, index, compileErr)
				}
				args[index] = arg
			}
			term := ctx.builder.Arena().ObjectLiteralValue(plan, args...)
			if term == 0 {
				return 0, fmt.Errorf("object literal %d failed derived-term construction", source.ExprRef)
			}
			return term, nil
		}
		// ObjectLiteralView is the sole authority for a constructor expression.
		// Only after ruling it out may a generic prebound expression term answer
		// this source; otherwise injected expression maps can shadow the derived
		// plan and resurrect the old context-dependent heap witness path.
		if term, ok := exactSignatureExpressionTerm(ctx, source); ok {
			return term, nil
		}
		if function, ok := ctx.facts.ExpressionFunction(source.ExprRef); ok {
			if source.ResultIndex != 0 || source.Expanded || source.OpenTail {
				return 0, fmt.Errorf("function expression %d is not a scalar value", source.ExprRef)
			}
			value, valued := ctx.facts.ExpressionValue(source.ExprRef)
			id, identified := product.Get(ctx.registry, value, identity.Key).ID()
			kind := product.Get(ctx.registry, value, runtimekind.Key)
			if !valued || !identified || id != identity.LuaFunction(uint64(function)) ||
				!runtimekind.Equal(kind, runtimekind.Singleton(runtimekind.Function)) {
				return 0, fmt.Errorf("function expression %d has no exact function value", source.ExprRef)
			}
			return ctx.builder.Arena().Constant(value), nil
		}
		if active[source.ExprRef] {
			return 0, fmt.Errorf("cyclic expression source %d", source.ExprRef)
		}
		if term, exact, err := frozenCustomExpressionTerm(ctx, source); err != nil {
			return 0, err
		} else if exact {
			return term, nil
		}
		if operation, ok := ctx.facts.ExpressionOperation(source.ExprRef); ok {
			if operation.Kind() == factflow.ExpressionOperationBinary && (operation.Op() == "and" || operation.Op() == "or") {
				if _, structural := ctx.plan.StructuralExpressionRegion(source.ExprRef); structural {
					term, bound := ctx.builder.Arena().expressionValue(source.ExprRef)
					if !bound {
						return 0, fmt.Errorf("logical expression %d has no bound result cell", source.ExprRef)
					}
					return term, nil
				}
			}
			identity, hasIdentity := operation.Intrinsic()
			if _, certified := ctx.predicateExpressions[source.ExprRef]; certified && hasIdentity && identity == intrinsic.LuaType {
				if active == nil {
					active = make(map[factflow.ExprRef]bool, 1)
				}
				active[source.ExprRef] = true
				arg, argErr := exactCompilerSourceTermActive(ctx, operation.Left(), active)
				delete(active, source.ExprRef)
				if argErr != nil {
					return 0, fmt.Errorf("lua type expression %d argument: %w", source.ExprRef, argErr)
				}
				term := ctx.builder.Arena().LuaTypeNameValue(arg)
				if term == 0 {
					return 0, fmt.Errorf("lua type expression %d failed symbolic construction", source.ExprRef)
				}
				return term, nil
			}
			_, certifiedPredicate := ctx.predicateExpressions[source.ExprRef]
			if operation.Kind() == factflow.ExpressionOperationUnary && isPureUnaryOperator(operation.Op()) {
				if active == nil {
					active = make(map[factflow.ExprRef]bool, 1)
				}
				active[source.ExprRef] = true
				operand, operandErr := exactCompilerSourceTermActive(ctx, operation.Left(), active)
				delete(active, source.ExprRef)
				if operandErr != nil {
					return 0, fmt.Errorf("unary expression %d operand: %w", source.ExprRef, operandErr)
				}
				term, exact := ctx.builder.Arena().ScalarUnaryValue(operation.Op(), operand)
				if !exact || term == 0 {
					return 0, fmt.Errorf("unary expression %d failed symbolic construction", source.ExprRef)
				}
				return term, nil
			}
			if operation.Kind() == factflow.ExpressionOperationBinary && isPureBinaryOperator(operation.Op()) {
				if active == nil {
					active = make(map[factflow.ExprRef]bool, 1)
				}
				active[source.ExprRef] = true
				left, leftErr := exactCompilerSourceTermActive(ctx, operation.Left(), active)
				right, rightErr := exactCompilerSourceTermActive(ctx, operation.Right(), active)
				delete(active, source.ExprRef)
				if leftErr != nil {
					return 0, fmt.Errorf("binary expression %d left operand: %w", source.ExprRef, leftErr)
				}
				if rightErr != nil {
					return 0, fmt.Errorf("binary expression %d right operand: %w", source.ExprRef, rightErr)
				}
				exactScalarComparison := isExactScalarPredicateOperator(operation.Op())
				safeScalarComparison := compilerConstantScalarValue(ctx.builder.Arena(), left) ||
					compilerConstantScalarValue(ctx.builder.Arena(), right)
				if exactScalarComparison && !certifiedPredicate && !safeScalarComparison {
					return 0, fmt.Errorf("scalar comparison %d has non-scalar or metamethod-capable operands", source.ExprRef)
				}
				term, exact := ctx.builder.Arena().ScalarBinaryValue(operation.Op(), left, right)
				if !exact || term == 0 {
					return 0, fmt.Errorf("binary expression %d failed symbolic construction", source.ExprRef)
				}
				return term, nil
			}
			if operation.Kind() != factflow.ExpressionOperationBinary || operation.Op() != ".." {
				return 0, fmt.Errorf("expression operation %d is outside the pure symbolic vocabulary", source.ExprRef)
			}
			if active == nil {
				active = make(map[factflow.ExprRef]bool, 1)
			}
			active[source.ExprRef] = true
			left, leftErr := exactStringConcatSourceTerm(ctx, operation.Left(), active)
			right, rightErr := exactStringConcatSourceTerm(ctx, operation.Right(), active)
			delete(active, source.ExprRef)
			if leftErr != nil {
				return 0, fmt.Errorf("expression concat %d left operand: %w", source.ExprRef, leftErr)
			}
			if rightErr != nil {
				return 0, fmt.Errorf("expression concat %d right operand: %w", source.ExprRef, rightErr)
			}
			term := ctx.builder.Arena().StringConcatValue(left, right)
			if term == 0 {
				return 0, fmt.Errorf("expression concat %d failed symbolic construction", source.ExprRef)
			}
			return term, nil
		}
		if dynamic, ok := ctx.facts.DynamicIndexExpression(source.ExprRef); ok {
			if active == nil {
				active = make(map[factflow.ExprRef]bool, 1)
			}
			active[source.ExprRef] = true
			term, err := exactCompilerDynamicReadTerm(ctx, dynamic, active)
			delete(active, source.ExprRef)
			if err != nil {
				return 0, fmt.Errorf("dynamic expression %d: %w", source.ExprRef, err)
			}
			return term, nil
		}
		if p, ok := ctx.facts.ExpressionPathRef(source.ExprRef); ok {
			term, err := exactCompilerStaticPathTerm(ctx, p)
			if err != nil {
				return 0, fmt.Errorf("source expression path: %w", err)
			}
			return term, nil
		}
	}
	if term, ok := exactSignatureExpressionTerm(ctx, source); ok {
		return term, nil
	}
	if source.Kind == factflow.ValueSourcePath {
		p, ok := compilerResolverPath(source.PathKey)
		if !ok || p.Version != 0 {
			return 0, fmt.Errorf("source path is not a canonical lexical path")
		}
		term, err := exactCompilerStaticPathTerm(ctx, p)
		if err != nil {
			return 0, fmt.Errorf("source path: %w", err)
		}
		return term, nil
	}
	value, err := exactReturnSourceValue(ctx.registry, ctx.facts, source)
	if err != nil {
		return 0, fmt.Errorf("assignment source is not a context-independent scalar")
	}
	return ctx.builder.Arena().Constant(value), nil
}

// exactChannelSelectResultTerm consumes the point-owned result cell written by the
// channel-select N3 transaction. WIR represents this producer with the common
// call-result ValueSource vocabulary, but it deliberately has no CallSite:
// the select event is its exact producer/slot authority.
func exactChannelSelectResultTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, bool) {
	if ctx.builder == nil || !source.Valid() || source.Kind != factflow.ValueSourceCall || !source.HasCallPoint ||
		source.ResultIndex < 0 || !source.Final || source.Expanded || source.Adjusted || source.OpenTail {
		return 0, false
	}
	for _, event := range ctx.facts.ChannelSelects(source.CallPoint) {
		if event.Kind() != factflow.ChannelSelectSelect || event.Index() != source.ResultIndex {
			continue
		}
		term, exact := ctx.builder.Arena().callResultValue(source.CallPoint, source.ResultIndex)
		return term, exact && term != 0
	}
	return 0, false
}

// exactExternalCallResultTerm resolves one call-produced scalar through the
// call site's frozen value-list authority. Static signatures contribute their
// exact abstract result directly; contextual external calls consume the
// point-owned result cell sealed for the N0 external-call instruction. Lexical calls
// are excluded because their sole authority is the frame-result term above.
func exactExternalCallResultTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, bool, error) {
	if ctx.plan == nil || ctx.builder == nil || !source.Valid() || source.Kind != factflow.ValueSourceCall ||
		!source.HasCallPoint || source.ResultIndex < 0 {
		return 0, false, nil
	}
	site, ok := ctx.facts.CallSiteView(source.CallPoint)
	if !ok {
		return 0, false, nil
	}
	producerShape := source.Final == site.Final() && source.Expanded == site.Expanded() &&
		source.Adjusted == site.Adjusted() && source.OpenTail == site.OpenTail()
	// Branch lowering presents an adjusted condition call as its closed scalar
	// head: the producer is adjusted at the call site, while the branch-owned
	// ValueSourceCall has already consumed that presentation flag. This is the
	// one registered condition normalization; point and result zero still name
	// the exact external producer.
	conditionShape := site.Context() == factflow.CallSiteContextCondition && source.ResultIndex == 0 &&
		site.Final() && !site.Expanded() && site.Adjusted() && !site.OpenTail() &&
		source.Final && !source.Expanded && !source.Adjusted && !source.OpenTail
	// Expression producers may be presented with the enclosing consumer's
	// value-list shape (for example a final call in a table constructor).  The
	// call point/result target still identifies the exact producer; direct
	// assignment/return sources must retain the producer shape byte-for-byte.
	if !producerShape && !conditionShape && site.Context() != factflow.CallSiteContextExpressionProducer {
		return 0, false, nil
	}
	if surface, sealed := ctx.plan.CallSurface(); sealed {
		if classified, present := surface.Site(source.CallPoint); present && classified.Target.Kind() == operationplan.CallSurfaceTargetLexical {
			return 0, false, nil
		}
	}
	owned := false
	if source.HasExpr {
		ref, exact := site.Expr()
		owned = exact && ref == source.ExprRef
	}
	for index := 0; index < site.ResultTargetCount(); index++ {
		target, exact := site.ResultTargetAt(index)
		if !exact || target.ResultIndex() != source.ResultIndex {
			continue
		}
		// Expression-producer coordinates are owned by the producing call and
		// result slot. ValueSource.TargetIndex belongs to the enclosing consumer's
		// tuple and is therefore deliberately unrelated: the same inner result can
		// occupy argument one, argument two, or any other outer position. Direct
		// assignment and return sites retain their target-index identity below.
		if site.Context() == factflow.CallSiteContextExpressionProducer {
			if target.Kind() != factflow.CallResultTargetExpression {
				continue
			}
		} else if source.TargetIndex >= 0 && target.Index() != source.TargetIndex {
			continue
		}
		owned = true
		break
	}
	// A condition-context call has no assignment/return target by construction:
	// its scalar result is consumed directly by the following branch. The
	// registered call-site context plus exact producer shape owns slot zero, just
	// as a result target owns a materialized coordinate in other contexts.
	if !owned && (producerShape || conditionShape) && site.Context() == factflow.CallSiteContextCondition && source.ResultIndex == 0 {
		owned = true
	}
	// A direct open tail has one syntactic target but denotes every remaining
	// declared return coordinate. expandOpenTailReturnSources has already made
	// the selected coordinate explicit in ResultIndex/TargetIndex.
	if !owned && producerShape && site.Context() == factflow.CallSiteContextReturnSource &&
		site.OpenTail() && source.TargetIndex >= 0 && source.TargetIndex < planReturnArity(ctx.plan) {
		owned = true
	}
	if !owned {
		return 0, false, nil
	}
	// An exact producer expression (allocation, module load, intrinsic, static
	// scalar, or dependent result) is the value authority for its result slot.
	// The point-owned CallResult register remains the execution/publication
	// footprint for contextual producers, but must not shadow a term already
	// derived from the immutable operation plan.
	if term, exact := exactSignatureExpressionTerm(ctx, source); exact {
		return term, true, nil
	}
	if term, dependent, err := exactDependentCallResultTerm(ctx, source.CallPoint, source.ResultIndex); err != nil {
		return 0, false, err
	} else if dependent {
		return term, true, nil
	}
	if op, exact := ctx.plan.SignatureCallOperation(source.CallPoint); exact {
		if values, static := exactStaticSignatureReturnsAtPoint(ctx, source.CallPoint, op); static {
			if source.ResultIndex < len(values) {
				term := ctx.builder.Arena().Constant(values[source.ResultIndex])
				if term == 0 {
					return 0, false, fmt.Errorf("external call result %d failed static construction", source.ResultIndex)
				}
				return term, true, nil
			}
		}
	}
	term, exact := ctx.builder.Arena().callResultValue(source.CallPoint, source.ResultIndex)
	if !exact || term == 0 {
		return 0, false, nil
	}
	return term, true, nil
}

func compilerResolverPath(key pathdom.PathKey) (pathdom.Path, bool) {
	sym, version, suffix, ok := pathaddr.ParseResolverPath(key)
	if !ok || sym == 0 {
		return pathdom.Path{}, false
	}
	segments, ok := segment.InternFormattedSegments(suffix)
	if !ok {
		return pathdom.Path{}, false
	}
	return pathdom.Path{Symbol: sym, Version: version, Segments: segments}, true
}

// exactCompilerStaticPathTerm keeps the value producer and its path
// provenance together. Boundary descendants retain a DynamicRead term so
// specialization consults the caller's exact path/heap evidence. Descendants
// of row-local values use the pure static-index relation; evaluation still
// fails the whole transaction when the abstract value cannot answer exactly.
func exactCompilerStaticPathTerm(ctx planCompileContext, p pathdom.Path) (ValueTerm, error) {
	if p.Symbol == 0 || p.Version != 0 {
		return 0, fmt.Errorf("path is not canonical")
	}
	// The sealed structural environment is an exact lexical binding even while
	// predicate expressions are compiled before their row-local map is filled.
	// Resolve that shared authority first instead of imposing a second locals
	// admission rule on the same path.
	if binding, err := exactBoundaryPathBinding(ctx, p); err == nil {
		if len(p.Segments) == 0 {
			return binding.Owner, nil
		}
		term, _, lowerErr := ctx.builder.Arena().LowerBoundaryPathValue(p, binding)
		return term, lowerErr
	}
	owner, ok := ctx.locals[p.Symbol]
	if !ok || owner == 0 {
		return 0, fmt.Errorf("symbol %d has no exact local binding", p.Symbol)
	}
	if len(p.Segments) == 0 {
		return owner, nil
	}
	if ctx.structuralEnvironment && ctx.point != 0 {
		node := ctx.builder.Arena().values[owner]
		if node.op == valueEnvironment && node.slot == statekey.SymbolValue(p.Symbol) {
			rootOwner := owner
			for index, member := range p.Segments {
				key, exact := sourcevalue.StaticPathSegmentValue(ctx.registry, member)
				if !exact {
					return 0, fmt.Errorf("descendant has a non-scalar static key")
				}
				tablePath := ctx.builder.Arena().EnvironmentPath(p.Symbol, p.Segments[:index]...)
				if index == 0 {
					// The environment owner already is the value of the root-only
					// table path. Keep the path as flow-evidence metadata, but do
					// not project it from State a second time: value-only loop and
					// returned bindings may intentionally have no duplicate path
					// spelling at this coordinate.
					owner = ctx.builder.Arena().DynamicReadTableValueAt(ctx.point, rootOwner, tablePath, ctx.builder.Arena().Constant(key))
				} else {
					owner = ctx.builder.Arena().DynamicReadValueAt(ctx.point, rootOwner, tablePath, ctx.builder.Arena().Constant(key))
				}
				if owner == 0 {
					return 0, fmt.Errorf("descendant environment read failed symbolic construction")
				}
			}
			return owner, nil
		}
	}
	for _, member := range p.Segments {
		owner = ctx.builder.Arena().StaticIndexValue(owner, member)
		if owner == 0 {
			return 0, fmt.Errorf("descendant has a non-scalar static key")
		}
	}
	return owner, nil
}

func compilerConstantScalarValue(arena *Arena, term ValueTerm) bool {
	if arena == nil || term == 0 || int(term) >= len(arena.values) {
		return false
	}
	node := arena.values[term]
	return node.op == valueConstant && scalarValue(arena.reg, node.value)
}

func exactStringConcatSourceTerm(ctx planCompileContext, source factflow.ValueSource, active map[factflow.ExprRef]bool) (ValueTerm, error) {
	if !source.Valid() || source.Expanded || source.OpenTail || source.Adjusted && source.ResultIndex != 0 {
		return 0, fmt.Errorf("non-scalar or malformed operand %#v", source)
	}
	// Preserve the producer's exact value-list shape while resolving it.  In
	// particular, an adjusted call result must still match its CallSite
	// authority; exactCompilerSourceTermActive applies the scalar projection
	// only after producer-aware resolution has had the first opportunity.
	term, err := exactCompilerSourceTermActive(ctx, source, active)
	if err != nil {
		return 0, err
	}
	return term, nil
}

func exactSignatureExpressionTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, bool) {
	if source.Kind == factflow.ValueSourceCall && source.HasCallPoint {
		op, sealed := ctx.plan.SignatureCallOperation(source.CallPoint)
		if sealed {
			_, intrinsic := op.Intrinsic()
			site, ok := ctx.facts.CallSiteView(source.CallPoint)
			// A no-expr return source represents a value-list forwarding
			// boundary. Only an adjusted call is scalar there; admitting an
			// expanded/open-tail call would silently discard later returns.
			if ok && (intrinsic || source.HasExpr || exactAdjustedSignatureCallSource(source, site)) {
				ref, hasExpr := site.Expr()
				if hasExpr {
					terms := ctx.expressions[ref]
					index := source.ResultIndex
					if index < 0 {
						index = 0
					}
					if index < len(terms) {
						return terms[index], true
					}
				}
			}
		}
	}
	if (source.Kind != factflow.ValueSourceExpression && source.Kind != factflow.ValueSourceCall) || !source.HasExpr {
		return 0, false
	}
	terms, ok := ctx.expressions[source.ExprRef]
	if !ok {
		return 0, false
	}
	index := source.ResultIndex
	if index < 0 {
		index = 0
	}
	if index >= len(terms) {
		return 0, false
	}
	return terms[index], true
}

func exactAdjustedSignatureCallSource(source factflow.ValueSource, site factflow.CallSiteView) bool {
	return source.Valid() && source.ResultIndex == 0 && source.Final == site.Final() &&
		source.Adjusted && site.Adjusted() &&
		!source.Expanded && !site.Expanded() &&
		!source.OpenTail && !site.OpenTail()
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
	return false
}

func scalarValue(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String:
		return true
	case kind.Literal:
		literal, exact := t.(*typ.Literal)
		if !exact {
			return false
		}
		switch literal.Base {
		case kind.Boolean, kind.Number, kind.Integer, kind.String:
			return true
		default:
			return false
		}
	default:
		return false
	}
}
