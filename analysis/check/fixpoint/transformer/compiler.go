package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/module/signature"
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
	facts      map[operationplan.Kind]planKindHandler
	extensions map[operationplan.ExtensionKind]planExtensionHandler
}

type planCompileContext struct {
	registry          *axis.Registry
	graph             cfg.Graph
	plan              *operationplan.Plan
	facts             factflow.Facts
	builder           *Builder
	locals            map[symbol.ID]ValueTerm
	expressions       map[factflow.ExprRef][]ValueTerm
	allocationEffects map[cfg.Point]EffectTerm
	rowEffects        *[]EffectTerm
	rowOutput         *summary.Summary
	genericBindings   map[symbol.ID]symbolicGenericBinding
	directCalls       *DirectCallCatalog
	allowConstantAdd  bool
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
	c.facts[operationplan.ExpressionOperation] = expressionOperationPlanHandler{}
	c.facts[operationplan.ExpressionPath] = dynamicIndexDependencyPlanHandler{kind: operationplan.ExpressionPath}
	c.facts[operationplan.DynamicIndexExpression] = dynamicIndexDependencyPlanHandler{kind: operationplan.DynamicIndexExpression}
	c.facts[operationplan.RootAssignment] = rootAssignmentPlanHandler{}
	c.facts[operationplan.Return] = returnPlanHandler{}
	c.facts[operationplan.CallSite] = signatureCallPlanHandler{}
	for _, dynamicKind := range []operationplan.Kind{
		operationplan.PathDescendantInvalidation,
		operationplan.DynamicIndexWrite,
	} {
		c.facts[dynamicKind] = dynamicIndexPlanHandler{kind: dynamicKind}
	}
	// These edge families are consumed together by compileBranchEdge. They are
	// registered here so the operation-plan exhaustiveness gate recognizes one
	// semantic owner; their point-local handlers intentionally publish nothing.
	for _, branchKind := range []operationplan.Kind{
		operationplan.BranchEdgeReachability,
		operationplan.BranchConditionSource,
		operationplan.BranchRefinement,
		operationplan.BranchPathEvidence,
	} {
		c.facts[branchKind] = branchEdgePlanHandler{kind: branchKind}
	}
	c.extensions[operationplan.BodyGenericFor] = genericForPlanHandler{}
	return c
}

type branchEdgePlanHandler struct{ kind operationplan.Kind }

func (h branchEdgePlanHandler) Kind() operationplan.Kind                              { return h.kind }
func (branchEdgePlanHandler) Preflight(planCompileContext, cfg.Point) error           { return nil }
func (branchEdgePlanHandler) Lower(planCompileContext, cfg.Point, *[]Operation) error { return nil }

type signatureCallPlanHandler struct{}

func (signatureCallPlanHandler) Kind() operationplan.Kind { return operationplan.CallSite }
func (signatureCallPlanHandler) Preflight(ctx planCompileContext, point cfg.Point) error {
	if ctx.directCalls != nil {
		if _, direct := ctx.directCalls.Lookup(point); direct {
			if _, ok := ctx.facts.CallSiteView(point); !ok {
				return fmt.Errorf("direct call: call-site payload missing")
			}
			return nil
		}
	}
	op, ok := ctx.plan.SignatureCallOperation(point)
	if !ok {
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
	if _, exact := ctx.plan.SignatureAllocationOperation(point); exact {
		if ctx.allocationEffects[point] == 0 {
			return fmt.Errorf("signature call: allocation term missing")
		}
		return nil
	}
	if _, ok := effectlowering.StaticScalarSignatureReturns(ctx.registry, nil, sig); ok {
		if _, exists := ctx.facts.CallSiteView(point); !exists {
			return fmt.Errorf("signature call: call-site payload missing")
		}
		return nil
	}
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
func (signatureCallPlanHandler) Lower(ctx planCompileContext, point cfg.Point, _ *[]Operation) error {
	if ctx.directCalls != nil {
		if _, direct := ctx.directCalls.Lookup(point); direct {
			return nil
		}
	}
	if op, ok := ctx.plan.SignatureCallOperation(point); ok {
		operational := op.Signature().OperationalEffects
		if operational == nil || !operational.SuspensionKnown || operational.MaySuspend {
			if ctx.rowOutput == nil {
				return fmt.Errorf("signature call: suspension row output sink missing")
			}
			ctx.rowOutput.MaySuspend = true
		}
	}
	if effect := ctx.allocationEffects[point]; effect != 0 {
		if ctx.rowEffects == nil {
			return fmt.Errorf("signature call: allocation row sink missing")
		}
		*ctx.rowEffects = append(*ctx.rowEffects, effect)
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
		values, ok := effectlowering.StaticScalarSignatureReturns(ctx.registry, nil, op.Signature())
		if !ok {
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
	return nil
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
		asserted, hasAsserted := op.SourceContract(sourceIndex)
		projection := ctx.builder.Arena().IteratorProjectionValueWithContract(iterator, op.VariableIndex(), term, asserted, hasAsserted)
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

func compileBranchEdge(base planCompileContext, point cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
	arena := base.builder.Arena()
	if base.facts.BranchEdgeUnreachable(point, cond) {
		return row, arena.False(), nil
	}
	if genericForBranchHead(base.graph, base.plan, point) || numericForBranchHead(base, point) {
		return row, arena.True(), nil
	}
	ctx := base
	ctx.locals = row.Values
	ctx.genericBindings = row.genericBindings
	branch := factapply.NewBranchAlgebra(base.facts, point)
	conditionSource, ok := branch.ConditionSource()
	if !ok {
		return SymbolicCFGRow{}, 0, fmt.Errorf("branch:missing-condition-source")
	}
	conditionTerm, conditionErr := exactCompilerSourceTerm(ctx, conditionSource)
	if conditionErr != nil {
		return SymbolicCFGRow{}, 0, fmt.Errorf("branch: contextual-condition-source")
	}
	if err := validateRepresentedBranchEvidence(ctx, branch, conditionTerm); err != nil {
		return SymbolicCFGRow{}, 0, err
	}
	truthy, falsy, err := lowerBranchConditionGuards(arena, branch, func(source factflow.ValueSource) (ValueTerm, bool) {
		term, resolveErr := exactCompilerSourceTerm(ctx, source)
		return term, resolveErr == nil
	}, true)
	if err != nil {
		return SymbolicCFGRow{}, 0, err
	}
	updates, err := lowerCompilerBranchRefinements(arena, branch, cond, ctx, conditionTerm)
	if err != nil {
		return SymbolicCFGRow{}, 0, err
	}
	for _, update := range updates {
		path := update.TargetPathRef()
		if path.Symbol == 0 || path.Version != 0 || len(path.Segments) != 0 {
			return SymbolicCFGRow{}, 0, fmt.Errorf("branch: contextual-refinement-path")
		}
		row.Values[path.Symbol] = update.Value()
	}
	appendRepresentedBranchEvidenceOutput(ctx, branch, cond, &row.Output)
	if err := appendDynamicConditionBranchProof(arena, branch, cond, conditionTerm, &row.Proofs); err != nil {
		return SymbolicCFGRow{}, 0, err
	}
	if cond {
		return row, truthy, nil
	}
	return row, falsy, nil
}

func numericForBranchHead(ctx planCompileContext, point cfg.Point) bool {
	fact, ok := ctx.facts.RootAssignment(point)
	if !ok {
		return false
	}
	_, exact := exactNumericForIteratorBinding(ctx, point, fact)
	return exact
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
	trueGeneric, falseGeneric := false, false
	for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
		cond, ok := graph.EdgeCond(point, successor)
		if !ok {
			return false
		}
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
		targetBase, _ := exactCompilerPathTerm(ctx, target)
		if refinement.FalsyAbsent() && targetBase == condition {
			// The conditional absent optimization is exactly subsumed by the
			// falsy condition guard. Retaining the original term preserves false
			// versus nil while row feasibility carries the refinement.
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
	truthyEdge := branchCondition.TruthyOnTrueEdge()
	branch.ForEachPathEvidence(func(proof factflow.BranchPathEvidence) bool {
		term, ok := exactCompilerPathTerm(ctx, proof.PathRef())
		if !ok || term != condition {
			validationErr = fmt.Errorf("branch: contextual-path-evidence-path")
			return false
		}
		if !proof.ActiveOnEdge(truthyEdge) || proof.ActiveOnEdge(!truthyEdge) {
			validationErr = fmt.Errorf("branch: contextual-path-evidence-polarity")
			return false
		}
		switch proof.Kind() {
		case factflow.BranchPathEvidenceTruthy:
			// The truthy row guard is the exact durable representation.
		case factflow.BranchPathEvidencePresence:
			value, hasPresence := proof.Presence()
			if !hasPresence || value != presence.Present() {
				validationErr = fmt.Errorf("branch: contextual-path-evidence-presence")
				return false
			}
			// Truthiness of this same root implies presence, so the row guard
			// represents this fact without a parallel mutable evidence lane.
		default:
			validationErr = fmt.Errorf("branch: contextual-path-evidence-kind %d", proof.Kind())
			return false
		}
		return true
	})
	return validationErr
}

func exactCompilerPathTerm(ctx planCompileContext, path pathdom.Path) (ValueTerm, bool) {
	if path.Symbol == 0 || path.Version != 0 || len(path.Segments) != 0 {
		return 0, false
	}
	term, ok := ctx.locals[path.Symbol]
	return term, ok
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
	fact, _ := ctx.facts.Return(point)
	for _, source := range fact.Sources() {
		if source.Kind == factflow.ValueSourcePath {
			if _, version, suffix, ok := pathaddr.ParseResolverPath(source.PathKey); !ok || version != 0 || suffix != "" {
				return fmt.Errorf("non-root or non-canonical return path")
			}
			continue
		}
		if _, err := exactReturnSourceTerm(ctx, source); err != nil {
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
	if term, ok := exactSignatureExpressionTerm(ctx, source); ok {
		return term, nil
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if _, pathBacked := ctx.facts.ExpressionPathRef(source.ExprRef); pathBacked {
			return exactCompilerSourceTerm(ctx, source)
		}
		if _, dynamic := ctx.facts.DynamicIndexExpression(source.ExprRef); dynamic {
			return exactCompilerSourceTerm(ctx, source)
		}
		if _, operation := ctx.facts.ExpressionOperation(source.ExprRef); operation {
			return exactCompilerSourceTerm(ctx, source)
		}
	}
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
	if fact.TargetSymbol() == 0 {
		return fmt.Errorf("assignment target has no symbol")
	}
	target := fact.TargetPathRef()
	if target.Symbol != fact.TargetSymbol() || target.Version != 0 || len(target.Segments) != 0 {
		return fmt.Errorf("assignment target is not its canonical root symbol")
	}
	switch fact.Kind() {
	case factflow.RootAssignmentLocalDeclaration:
		if _, exact := exactNumericForIteratorBinding(ctx, point, fact); exact {
			return nil
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
		if _, err := exactCompilerSourceTerm(ctx, fact.Source()); err != nil {
			return fmt.Errorf("assignment source is not a context-independent scalar")
		}
		return nil
	case factflow.RootAssignmentOrdinaryRootWrite:
		if _, ok := fact.DeclaredValue(); ok || fact.DeclaredValueContracts() || fact.DeclaredValueOverlays() {
			return fmt.Errorf("ordinary root write carries a declared contract or overlay")
		}
		if _, ok := fact.DeclaredAnnotationValue(); ok {
			return fmt.Errorf("ordinary root write carries annotation evidence")
		}
		if !singleCertifiedAccumulatorWrite(ctx, point, fact) {
			return fmt.Errorf("ordinary root write is not the single certified numeric-for accumulator update")
		}
		ctx.allowConstantAdd = true
		term, err := exactCompilerSourceTerm(ctx, fact.Source())
		if err != nil || !compilerConstantValue(ctx.builder.Arena(), term) {
			return fmt.Errorf("ordinary root write source is not a context-independent constant scalar")
		}
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
	term, numericIterator := exactNumericForIteratorBinding(ctx, point, fact)
	if !numericIterator {
		var err error
		ctx.allowConstantAdd = fact.Kind() == factflow.RootAssignmentOrdinaryRootWrite
		term, err = exactCompilerSourceTerm(ctx, fact.Source())
		if err != nil {
			return err
		}
	}
	if fact.Kind() == factflow.RootAssignmentOrdinaryRootWrite {
		if _, exists := ctx.locals[fact.TargetSymbol()]; !exists {
			return fmt.Errorf("symbol %d ordinary write precedes its declaration", fact.TargetSymbol())
		}
		if !compilerConstantValue(ctx.builder.Arena(), term) {
			return fmt.Errorf("symbol %d ordinary write did not lower to a constant scalar", fact.TargetSymbol())
		}
		ctx.locals[fact.TargetSymbol()] = term
		return nil
	}
	if prior, exists := ctx.locals[fact.TargetSymbol()]; exists {
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
	for _, floor := range ctx.facts.BranchNumFloorRefinements(point) {
		if floor.Cond() && floor.TargetPathRef().Equal(target) {
			return true
		}
	}
	return false
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
	if term, ok := exactSignatureExpressionTerm(ctx, source); ok {
		return term, nil
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
	// Direct-call composition binds each exact result target into row-local
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
		if active[source.ExprRef] {
			return 0, fmt.Errorf("cyclic expression source %d", source.ExprRef)
		}
		if operation, ok := ctx.facts.ExpressionOperation(source.ExprRef); ok {
			if ctx.allowConstantAdd && operation.Kind() == factflow.ExpressionOperationBinary && operation.Op() == "+" {
				if active == nil {
					active = make(map[factflow.ExprRef]bool, 1)
				}
				active[source.ExprRef] = true
				left, leftErr := exactCompilerSourceTermActive(ctx, operation.Left(), active)
				right, rightErr := exactCompilerSourceTermActive(ctx, operation.Right(), active)
				delete(active, source.ExprRef)
				if leftErr != nil || rightErr != nil || !compilerConstantValue(ctx.builder.Arena(), left) || !compilerConstantValue(ctx.builder.Arena(), right) {
					return 0, fmt.Errorf("expression addition %d operands are not context-independent constants", source.ExprRef)
				}
				value, exact := luasourcevalue.BinaryOperationValue(ctx.registry, nil, "+", ctx.builder.Arena().values[left].value, ctx.builder.Arena().values[right].value)
				if !exact || !scalarValue(ctx.registry, value) {
					return 0, fmt.Errorf("expression addition %d has no exact scalar product result", source.ExprRef)
				}
				return ctx.builder.Arena().Constant(value), nil
			}
			if operation.Kind() != factflow.ExpressionOperationBinary || operation.Op() != ".." {
				return 0, fmt.Errorf("expression operation %d is not exact string concatenation", source.ExprRef)
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
			if p.Symbol == 0 || p.Version != 0 {
				return 0, fmt.Errorf("source expression path is not canonical")
			}
			if len(p.Segments) == 0 {
				term, ok := ctx.locals[p.Symbol]
				if !ok {
					return 0, fmt.Errorf("source expression symbol %d has no exact binding", p.Symbol)
				}
				return term, nil
			}
			if owner, ok := ctx.locals[p.Symbol]; ok && iteratorProjectionDerived(ctx.builder.Arena(), owner) {
				for _, member := range p.Segments {
					owner = ctx.builder.Arena().StaticIndexValue(owner, member)
					if owner == 0 {
						return 0, fmt.Errorf("source expression iterator member is not a static scalar key")
					}
				}
				return owner, nil
			}
			binding, err := exactBoundaryPathBinding(ctx, p)
			if err != nil {
				return 0, fmt.Errorf("source expression descendant: %w", err)
			}
			term, _, err := ctx.builder.Arena().LowerBoundaryPathValue(p, binding)
			return term, err
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

func exactStringConcatSourceTerm(ctx planCompileContext, source factflow.ValueSource, active map[factflow.ExprRef]bool) (ValueTerm, error) {
	if !source.Valid() || source.Expanded || source.Adjusted || source.OpenTail {
		return 0, fmt.Errorf("non-scalar or malformed operand")
	}
	term, err := exactCompilerSourceTermActive(ctx, source, active)
	if err != nil {
		return 0, err
	}
	return term, nil
}

func exactSignatureExpressionTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, bool) {
	if source.Kind == factflow.ValueSourceCall && source.HasCallPoint {
		op, intrinsic := ctx.plan.SignatureCallOperation(source.CallPoint)
		if intrinsic {
			_, intrinsic = op.Intrinsic()
		}
		if intrinsic {
			site, ok := ctx.facts.CallSiteView(source.CallPoint)
			if ok {
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
