package interproc

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/compiler/check/infer/captured"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
)

// Store is the minimal store interface required to record post-flow interproc facts.
type Store interface {
	api.StoreReader

	MergeInterprocFactsNext(key api.GraphKey, delta api.Facts)
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool)
}

// StoreFactsFromResult records post-flow interproc facts for the current iteration.
// Facts are written into InterprocFactsNext and become visible after FixpointSwap.
//
// interner is the compilation-scoped recursive-family interner; inferred-return
// sealing widens family bodies only through it, so one compilation can never
// mutate the type state another observes.
func StoreFactsFromResult(
	store Store,
	fn *ast.FunctionExpr,
	result *api.FuncResult,
	parent *scope.State,
	interner *typ.RecursiveFamilyInterner,
) {
	if store == nil || result == nil || result.Graph == nil {
		return
	}
	writer := newInterprocFactWriter(store)
	writer.writeLiteralSignatures(result.Graph, parent, result.LiteralSignatureLookup())

	fnSym := cfg.SymbolID(0)
	if fn != nil {
		if resolvedSym, ok := store.SymbolForFunc(fn); ok && resolvedSym != 0 {
			fnSym = resolvedSym
		}
	}
	// Collect parameter evidence regardless of whether the function has a symbol.
	CollectParameterEvidenceFromResult(store, result, parent, fnSym)

	if fnSym == 0 {
		return
	}
	storeCapturedFactsFromResult(store, writer, fn, fnSym, result)

	fnType := functionfact.SolvedSignatureFromResult(result, fn)
	if fnType == nil {
		return
	}
	fnType = sealClassReturns(fnType)
	narrowSummary := returnsummary.Normalize(fnType.Returns)
	narrowSummary = sealRecursiveReturns(narrowSummary, fnSym, interner)
	summary := functionfact.PostflowReturnSummary(fn, narrowSummary)

	publicSeed := returns.BuildPostflowSeedFunctionType(result, fn)
	signature := functionfact.CanonicalPostflowSignature(
		fnType,
		publicSeed,
		narrowSummary,
		fn != nil && len(fn.ReturnTypes) > 0,
		fn != nil && fn.ParList != nil && fn.ParList.HasVargs,
	)
	envReturns := functionfact.ExtractEnvironmentReturns(result, fnSym, observation.FromFuncResult(result, nil))
	signature = attachReturnLengthEnsures(signature, result)
	builder := functionfact.NewBuilder()
	builder.AddSummary(fnSym, summary)
	builder.AddNarrow(fnSym, narrowSummary)
	builder.AddSignature(fnSym, signature)
	builder.AddEnvReturns(fnSym, envReturns)
	delta := interprocdomain.FunctionFactsDelta(builder.Build())
	writer.mergeParentFactsForSymbol(fnSym, delta)
}

// attachReturnLengthEnsures proves the constant arm of each return slot's length
// postcondition from the flow solution and attaches it to the signature spec as
// a constraint.ExprCompare over constraint.RL(i). For each normal return whose
// i-th expression is an identifier, it reads the flow solution's proven length
// lower bound for that identifier at the return point; across all live normal
// returns it takes the minimum (a slot's guaranteed length is the weakest proof
// on any returning path), and emits len(ret_i) >= k only when k >= 1. A slot
// missing on any live path, or with no proof on some path, contributes no fact,
// so a conditional/data-dependent return weakens the bound exactly as soundness
// requires. The postcondition is instantiated at call sites onto the assigned
// target's numeric length lower bound so a literal index read narrows.
func attachReturnLengthEnsures(signature *typ.Function, result *api.FuncResult) *typ.Function {
	if signature == nil || result == nil {
		return signature
	}
	flowOps := result.SolvedFlow()
	if flowOps == nil || result.Graph == nil {
		return signature
	}
	rets := result.Evidence.Returns
	if len(rets) == 0 {
		return signature
	}

	// minLower[i] is the smallest proven length lower bound for return slot i
	// across all live normal returns; seen[i] records that every such return
	// supplied a value for slot i (a slot missing on any path proves nothing).
	minLower := make(map[int]int64)
	seen := make(map[int]int)
	liveReturns := 0
	for _, ret := range rets {
		p := ret.Point
		info := ret.Info
		if info == nil || flowOps.IsPointDead(p) {
			continue
		}
		liveReturns++
		for i := range info.Exprs {
			sym := cfg.SymbolID(0)
			if i < len(info.Symbols) {
				sym = info.Symbols[i]
			}
			if sym == 0 {
				// A non-identifier return slot (literal, call, expression) carries no
				// tracked length identity; it proves no lower bound.
				continue
			}
			name := ""
			if i < len(info.Names) {
				name = info.Names[i]
			}
			path := constraint.Path{Root: name, Symbol: sym}
			lower, _, ok := flowOps.LengthBoundsAt(p, path)
			if !ok || lower < 0 {
				lower = 0
			}
			if cur, exists := minLower[i]; !exists || lower < cur {
				minLower[i] = lower
			}
			seen[i]++
		}
	}
	if liveReturns == 0 {
		return signature
	}

	var ensures []constraint.ExprCompare
	for i, lower := range minLower {
		if seen[i] != liveReturns || lower < 1 {
			// The slot is unproven on some live path, or carries no positive bound.
			continue
		}
		ensures = append(ensures, constraint.GeExpr(constraint.RL(i), constraint.C(lower)))
	}
	ensures = append(ensures, returnLengthRelationEnsures(result.ReturnRelations)...)
	ensures = append(ensures, returnLengthParamEnsures(result)...)
	return functionfact.AttachReturnLengthEnsures(signature, ensures)
}

// returnLengthRelationEnsures lowers canonical summary relations to public
// contract postconditions. This is the normalized path for facts such as
// len(ret_i) >= len(param_j): the relation is stored by return/parameter slot,
// not by source names or flow-input paths.
func returnLengthRelationEnsures(rels flow.ReturnRelations) []constraint.ExprCompare {
	lengthParams := rels.LengthParams()
	if len(lengthParams) == 0 {
		return nil
	}
	ensures := make([]constraint.ExprCompare, 0, len(lengthParams))
	for _, rel := range lengthParams {
		if rel.ReturnIndex < 0 || rel.ParamIndex < 0 {
			continue
		}
		ensures = append(ensures, constraint.GeExpr(constraint.RL(rel.ReturnIndex), constraint.PL(rel.ParamIndex)))
	}
	return ensures
}

// returnLengthParamEnsures proves the relational arm of return-length
// postconditions: a returned accumulator whose length is tied to a parameter's
// length. A loop that appends once per iteration over pairs(param_j) into an
// accumulator establishes #acc >= len(param_j) (the param's key cardinality); the
// loop-length extractor records this as a LoopInsertLength with Source set to the
// parameter path. When that accumulator is returned in slot i, this emits
// len(ret_i) >= len(param_j) as an ExprCompare over RL(i) and PL(j). Only a
// relation the body proves is emitted; a non-parameter source or a returned slot
// that is not the accumulator yields nothing.
func returnLengthParamEnsures(result *api.FuncResult) []constraint.ExprCompare {
	if result == nil || result.FlowInputs == nil || result.Graph == nil {
		return nil
	}
	lils := result.FlowInputs.LoopInsertLengths
	if len(lils) == 0 {
		return nil
	}
	paramIndex := paramIndexBySymbol(result.Graph)
	if len(paramIndex) == 0 {
		return nil
	}
	rets := result.Evidence.Returns
	flowOps := result.SolvedFlow()
	var ensures []constraint.ExprCompare
	seen := make(map[[2]int]struct{})
	for _, lil := range lils {
		if lil.Source.Symbol == 0 || lil.Target.Symbol == 0 {
			continue
		}
		j, ok := paramIndex[lil.Source.Symbol]
		if !ok {
			continue
		}
		for _, retSlot := range returnSlotsForSymbol(rets, flowOps, lil.Target.Symbol) {
			key := [2]int{retSlot, j}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			ensures = append(ensures, constraint.GeExpr(constraint.RL(retSlot), constraint.PL(j)))
		}
	}
	return ensures
}

// paramIndexBySymbol maps each parameter symbol to its zero-based index.
func paramIndexBySymbol(graph *cfg.Graph) map[cfg.SymbolID]int {
	syms := graph.ParamSymbols()
	if len(syms) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]int, len(syms))
	for i, sym := range syms {
		if sym != 0 {
			out[sym] = i
		}
	}
	return out
}

// returnSlotsForSymbol returns the return slot indices that return the given
// symbol on a live normal-return path. A slot returning a different value, or a
// slot on a dead path, is not included.
func returnSlotsForSymbol(rets []api.ReturnEvidence, flowOps api.FlowOps, sym cfg.SymbolID) []int {
	var slots []int
	seen := make(map[int]struct{})
	for _, ret := range rets {
		if ret.Info == nil || (flowOps != nil && flowOps.IsPointDead(ret.Point)) {
			continue
		}
		for i := range ret.Info.Symbols {
			if ret.Info.Symbols[i] != sym {
				continue
			}
			if _, dup := seen[i]; dup {
				continue
			}
			seen[i] = struct{}{}
			slots = append(slots, i)
		}
	}
	return slots
}

// sealClassReturns seals a constructor's instance return into its class
// recursive family. A function returning setmetatable({}, Class) yields an
// instance whose metatable is the cyclic class; sealing it keyed by the class
// metatable surface gives the inter-procedural fixpoint a finite representative
// instead of a metatable that unfolds a level deeper every iteration.
func sealClassReturns(fnType *typ.Function) *typ.Function {
	if fnType == nil || len(fnType.Returns) == 0 {
		return fnType
	}
	rets := sealClassInstances(fnType.Returns)
	if rets == nil {
		return fnType
	}
	if sealed := typjoin.WithReturns(fnType, rets); sealed != nil {
		return sealed
	}
	return fnType
}

// sealRecursiveReturns ties each self-referential return slot into the function's
// one keyed recursive family. A function whose return embeds its own prior return
// (rec returns o where o.x is rec(o.child)) yields a structural tower that grows a
// level deeper every inter-procedural iteration; the value-domain self-embedding
// fold mints a fresh recursion variable each time, so the summary never reaches a
// fixed point.
//
// The owner key is the function symbol plus slot index, the stable producer
// identity supplied by the checker. Interning by that key returns the one
// canonical family handle across every iteration, and the body is widened in
// place under the stable identity, so the summary is Equal to itself once the
// body settles and the inter-procedural fixpoint converges. Non-recursive slots
// are returned unchanged.
func sealRecursiveReturns(summary []typ.Type, fnSym cfg.SymbolID, interner *typ.RecursiveFamilyInterner) []typ.Type {
	if len(summary) == 0 || fnSym == 0 || interner == nil {
		return summary
	}
	out := summary
	for i, slot := range summary {
		sealed, ok := sealRecursiveReturnSlot(slot, fnSym, i, interner)
		if !ok {
			continue
		}
		if &out[0] == &summary[0] {
			out = append([]typ.Type(nil), summary...)
		}
		out[i] = sealed
	}
	return out
}

// sealRecursiveReturnSlot interns a self-referential return slot into the keyed
// family owned by (fnSym, slot). It folds the slot's self-embedding tower into a
// recursive node, rebinds that node's self-reference to the keyed family handle,
// and widens the family body in place with the value-domain convergence merge so
// precision drift still iterates while identity stays fixed. It reports false for
// a slot that is neither recursive nor a self-embedding tower.
func sealRecursiveReturnSlot(slot typ.Type, fnSym cfg.SymbolID, index int, interner *typ.RecursiveFamilyInterner) (typ.Type, bool) {
	if slot == nil || interner == nil {
		return nil, false
	}
	// Only the value domain's unstable inferred recursion needs an owner key: a
	// declared recursive type (a named mu or an alias-wrapped family) is already
	// canonical by its declaration and carries a nominal identity the program reads
	// (method lookups, alias names). Re-keying it would erase that identity, so the
	// seal applies only to inferred structural self-embedding.
	if !inferredRecursiveReturn(slot) {
		return nil, false
	}
	key := typ.FamilyKey{Namespace: "ret", Owner: fmt.Sprintf("%d#%d", fnSym, index)}
	family := interner.Intern(key)

	body, ok := recursiveReturnBody(slot, family)
	if !ok || body == nil {
		return nil, false
	}
	// A bare reference to the family carries no new body equation; the family is
	// already sealed, so return it unchanged. Sealing only proceeds for a slot that
	// contributes structural content to the body slot.
	if typ.IsRecursiveRef(body, family) {
		if family.Body == nil {
			return nil, false
		}
		return family, true
	}
	interner.Widen(family, body, value.MergeForConvergence)
	if family.Body == nil {
		return nil, false
	}
	// Always return the bare keyed handle: an iteration whose summary was fed back
	// arrives as a one-level-deeper unfolding of the family, so collapsing it to the
	// handle keeps the stored summary identical to itself across iterations.
	return family, true
}

// inferredRecursiveReturnName is the recursion-variable name the value-domain
// self-embedding fold mints; a return slot carrying it is unstable inferred
// recursion (a fresh node each iteration) eligible for owner keying.
const inferredRecursiveReturnName = "Inferred"

// inferredRecursiveReturn reports whether slot is the value domain's unstable
// inferred recursion rather than a declared nominal type. An already-keyed family,
// an Inferred fold node, or a bare structural self-embedding tower (no declared
// recursive or alias node) qualifies; a declared named mu (e.g. a source `type`
// recursion) or an alias-wrapped family does not, so its nominal identity is kept.
func inferredRecursiveReturn(slot typ.Type) bool {
	switch v := slot.(type) {
	case *typ.Recursive:
		if v == nil {
			return false
		}
		if _, keyed := typ.FamilyKeyOf(v); keyed {
			return true
		}
		return v.Name == inferredRecursiveReturnName
	case *typ.Alias:
		return false
	default:
		// A bare structural cycle with no declared recursive/alias wrapper is an
		// inferred self-embedding tower; a slot that wraps a declared recursive or
		// alias keeps that declared identity and is not re-keyed.
		return !containsDeclaredRecursion(slot)
	}
}

// containsDeclaredRecursion reports whether t embeds a declared recursive type (a
// named mu other than the inferred fold) or an alias, the nominal identities the
// seal must not erase.
func containsDeclaredRecursion(t typ.Type) bool {
	return typ.Contains(t, func(n typ.Type) bool {
		switch v := n.(type) {
		case *typ.Recursive:
			if v == nil {
				return false
			}
			if _, keyed := typ.FamilyKeyOf(v); keyed {
				return false
			}
			return v.Name != inferredRecursiveReturnName
		case *typ.Alias:
			return true
		default:
			return false
		}
	})
}

// recursiveReturnBody derives the body equation of family from a self-referential
// return slot. Three forms reach here across iterations, all of which must collapse
// to one body under the family handle:
//
//   - The slot already embeds the family (a prior seal fed back, possibly under one
//     or more extra record levels): every reference to family is the recursion edge,
//     so the immediate structure with those references is the body.
//   - The slot is an already-recursive node: its body is the equation, with the old
//     recursion variable rebound to the family handle.
//   - The slot is a bare self-embedding tower: the value-domain fold ties it into a
//     recursion variable, which is then rebound to the family handle.
func recursiveReturnBody(slot typ.Type, family *typ.Recursive) (typ.Type, bool) {
	if typ.ContainsRecursiveRef(slot, family) {
		return typ.CollapseUnfoldingToFamily(slot, family), true
	}
	if rec, ok := slot.(*typ.Recursive); ok && rec != nil && rec.Body != nil {
		return typ.RebindRecursiveRef(rec.Body, rec, family), true
	}
	if folded, ok := value.FoldSelfEmbedding(slot, slot); ok {
		if rec, ok := folded.(*typ.Recursive); ok && rec != nil && rec.Body != nil {
			return typ.RebindRecursiveRef(rec.Body, rec, family), true
		}
	}
	return nil, false
}

// sealClassInstances seals each constructor instance return in a summary vector
// in place, preserving the vector length so the inter-procedural summary slot
// stays stable. Returns nil when nothing was sealed.
func sealClassInstances(returns []typ.Type) []typ.Type {
	if len(returns) == 0 {
		return nil
	}
	out := make([]typ.Type, len(returns))
	changed := false
	for i, ret := range returns {
		sealed := metatable.SealClassInstanceReturn(ret)
		out[i] = sealed
		if !typ.SameNode(sealed, ret) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return out
}

func storeCapturedFactsFromResult(
	store Store,
	writer interprocFactWriter,
	fn *ast.FunctionExpr,
	fnSym cfg.SymbolID,
	result *api.FuncResult,
) {
	if store == nil || fn == nil || fnSym == 0 || result == nil || result.Graph == nil {
		return
	}

	transferValues := result.TransferValueFacts()
	fields := captured.FieldFactsFromEvidence(result.Evidence.CapturedFields, func(point cfg.Point, target constraint.Path, static typ.Type, source flow.AssignmentSource) typ.Type {
		if transferValues == nil {
			return static
		}
		return transferValues.AssignedValueTypeAt(point, target, static, source)
	})
	if len(fields) > 0 {
		writer.mergeParentFactsForSymbol(fnSym, interprocdomain.CapturedFieldAssignsDelta(fnSym, fields))
	}
}

// CollectParameterEvidenceFromResult reduces transfer-discovered call evidence
// into canonical parameter facts using solved abstract-interpreter evidence.
func CollectParameterEvidenceFromResult(store Store, result *api.FuncResult, parent *scope.State, currentSym cfg.SymbolID) {
	if store == nil || result == nil || result.Graph == nil {
		return
	}
	graph := result.Graph

	moduleBindings := store.ModuleBindings()
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = moduleBindings
	}
	preAssignTargets := checkcallsite.PreAssignmentTargetsByCall(result.Evidence.Assignments)
	hasFunctionRef := func(sym cfg.SymbolID) bool {
		return sym != 0 && store.FunctionRefBySym(sym) != nil
	}
	var currentKey api.GraphKey
	currentOK := false
	if currentSym == 0 {
		if currentFn := store.FuncForGraph(graph); currentFn != nil {
			currentSym, _ = store.SymbolForFunc(currentFn)
		}
	}
	if currentSym != 0 {
		currentKey, currentOK = store.ParentGraphKeyForSymbol(currentSym)
	}
	collector := parameterEvidenceCollector{
		store:                        store,
		parent:                       parent,
		currentKey:                   currentKey,
		currentOK:                    currentOK,
		currentSym:                   currentSym,
		skipCurrentBodyPreconditions: result.FlowProjection != nil,
		bodyContext:                  paramevidence.NewBodyPreconditionContext(graph, result, bindings).WithCurrentFunctionSymbol(currentSym),
		scopes:                       result.Scopes,
		callEntry: paramevidence.NewCallEntryProjector(paramevidence.CallEntryConfig{
			Result:           result,
			Graph:            graph,
			Bindings:         bindings,
			ModuleBindings:   moduleBindings,
			PreAssignTargets: preAssignTargets,
			HasFunctionRef:   hasFunctionRef,
			EvidenceAllowed: func(sym cfg.SymbolID, idx int) bool {
				return callArgumentEvidenceAllowedForSymbol(store, sym, idx)
			},
			Observer: observation.FromFuncResult(result, functionfact.StoreProjectionLookup(store, functionfact.ProjectionSibling, api.SynthModeDeclared, parent)),
			ArgumentObservation: func(point cfg.Point, arg ast.Expr, current typ.Type) typ.Type {
				return observation.ObservedArgumentType(result, point, arg, current, bindings)
			},
		}),
		expectedReceiverType: func(method string) typ.Type {
			if method == "" {
				return nil
			}
			return checkcallsite.ExpectedReceiverTypeForMethod(result.QueryContext, result.TypeOps, method)
		},
		methodResolvedOnReceiver: func(p cfg.Point, receiver ast.Expr, method string) bool {
			if receiver == nil || method == "" || result.TypeOps == nil {
				return false
			}
			receiverType := observation.FromFuncResult(result, nil).TypeOf(receiver, p)
			if typ.IsAbsentOrUnknown(receiverType) || typ.IsAny(receiverType) {
				return false
			}
			_, ok := result.TypeOps.Method(result.QueryContext, receiverType, method)
			return ok
		},
	}

	for _, evidence := range result.Evidence.Calls {
		collector.collectCallEvidence(evidence)
	}
}

type parameterEvidenceCollector struct {
	store      Store
	parent     *scope.State
	currentKey api.GraphKey
	currentOK  bool
	currentSym cfg.SymbolID
	// The current flow solves parameter body contracts inside its single
	// FunctionState = Points x Contracts fixed point. Re-running this older
	// postflow body-precondition collector on the same result creates a second
	// public contract source and can export stale hard obligations after guarded
	// uses. Keep call-entry evidence below, but leave current-function body/public
	// params to the solved summary.
	skipCurrentBodyPreconditions bool
	bodyContext                  paramevidence.BodyPreconditionContext
	scopes                       map[cfg.Point]*scope.State
	callEntry                    paramevidence.CallEntryProjector
	expectedReceiverType         func(string) typ.Type
	methodResolvedOnReceiver     func(cfg.Point, ast.Expr, string) bool
}

func (c *parameterEvidenceCollector) collectCallEvidence(evidence api.CallEvidence) {
	p := evidence.Point
	info := evidence.Info
	if c == nil || info == nil || checkcallsite.RuntimeArgCount(info) == 0 {
		return
	}
	c.recordCurrentBodyPreconditions(p, evidence)

	calleeSym := c.callEntry.CalleeSymbol(info)
	if calleeSym == 0 {
		return
	}
	parentKey, ok := functionfact.GraphKeyForSymbol(c.store, calleeSym, c.parent)
	if !ok {
		return
	}
	facts := functionfact.EntryParamsFacts(c.callEntry.EntryEvidence(p, evidence, calleeSym))
	if len(facts) == 0 {
		return
	}
	c.store.MergeInterprocFactsNext(parentKey, interprocdomain.FunctionFactsDelta(facts))
}

func (c *parameterEvidenceCollector) recordCurrentBodyPreconditions(p cfg.Point, evidence api.CallEvidence) {
	if c == nil || c.store == nil || c.skipCurrentBodyPreconditions || !c.currentOK || c.currentSym == 0 {
		return
	}
	var expectedReceiver typ.Type
	if info := evidence.Info; info != nil && info.Method != "" && c.expectedReceiverType != nil {
		if c.callEntry.CalleeSymbol(info) == 0 &&
			!checkcallsite.ReceiverIsScopedSelf(info, selfTypeAt(c.scopes, p)) &&
			(c.methodResolvedOnReceiver == nil || !c.methodResolvedOnReceiver(p, info.Receiver, info.Method)) {
			expectedReceiver = c.expectedReceiverType(info.Method)
		}
	}
	preconditions := c.bodyContext.PreconditionsFromCall(p, evidence, expectedReceiver, c.calleeParamInferred(evidence.Info))
	if preconditions.IsZero() {
		return
	}
	builder := functionfact.NewBuilder()
	builder.AddBodyParams(c.currentSym, preconditions.Body)
	builder.AddPublicParams(c.currentSym, preconditions.Public)
	facts := builder.Build()
	if len(facts) > 0 {
		c.store.MergeInterprocFactsNext(c.currentKey, interprocdomain.FunctionFactsDelta(facts))
	}
}

// calleeParamInferred returns a predicate over explicit argument indices that
// reports whether the resolved callee's source parameter for that argument is
// unannotated and therefore inferred from its own callsites. Such an expectation
// is an in-progress LUB rather than a concrete annotation, so it must not be
// recorded as a contravariant hard precondition on the caller parameter. The
// callsite arg-check still enforces a concrete callee contract once the callee's
// inferred parameter converges.
func (c parameterEvidenceCollector) calleeParamInferred(info *cfg.CallInfo) func(argIdx int) bool {
	if info == nil || c.store == nil {
		return nil
	}
	calleeSym := c.callEntry.CalleeSymbol(info)
	if calleeSym == 0 {
		return nil
	}
	fn, graph := sourceFunctionAndGraphForSymbol(c.store, calleeSym)
	if fn == nil {
		return nil
	}
	method := checkcallsite.IsMethodCallInfo(info)
	return func(argIdx int) bool {
		if argIdx < 0 {
			return false
		}
		runtimeIdx := argIdx
		if method {
			runtimeIdx++
		}
		return returns.SourceParamIsUnannotated(fn, graph, runtimeIdx)
	}
}

func selfTypeAt(scopes map[cfg.Point]*scope.State, p cfg.Point) typ.Type {
	if len(scopes) == 0 {
		return nil
	}
	sc := scopes[p]
	if sc == nil {
		return nil
	}
	return sc.SelfType()
}

func callArgumentEvidenceAllowedForSymbol(store Store, sym cfg.SymbolID, idx int) bool {
	if store == nil || sym == 0 || idx < 0 {
		return true
	}
	fn, graph := sourceFunctionAndGraphForSymbol(store, sym)
	return returns.SourceParamReceivesCallEvidence(fn, graph, idx)
}

func sourceFunctionAndGraphForSymbol(store Store, sym cfg.SymbolID) (*ast.FunctionExpr, *cfg.Graph) {
	if store == nil || sym == 0 {
		return nil, nil
	}
	fn := store.FuncForSymbol(sym)
	var graph *cfg.Graph
	if ref := store.FunctionRefBySym(sym); ref != nil && ref.GraphID != 0 {
		graph = store.Graphs()[ref.GraphID]
	}
	return fn, graph
}
