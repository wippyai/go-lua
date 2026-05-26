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
	writer.writeLiteralSignatures(result.Graph, parent, result.LiteralSignatures)

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
	builder := functionfact.NewBuilder()
	builder.AddSummary(fnSym, summary)
	builder.AddNarrow(fnSym, narrowSummary)
	builder.AddSignature(fnSym, signature)
	builder.AddEnvReturns(fnSym, envReturns)
	delta := interprocdomain.FunctionFactsDelta(builder.Build())
	writer.mergeParentFactsForSymbol(fnSym, delta)
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

	fields := captured.FieldFactsFromEvidence(result.Evidence.CapturedFields, func(point cfg.Point, target constraint.Path, static typ.Type, source flow.AssignmentSource) typ.Type {
		if result.FlowSolution == nil {
			return static
		}
		return result.FlowSolution.AssignedValueTypeAt(point, target, static, source)
	})
	if len(fields) > 0 {
		writer.mergeParentFactsForSymbol(fnSym, interprocdomain.CapturedFieldAssignsDelta(fnSym, fields))
	}

	mutations := captured.ContainerMutationsFromEvidence(result.Evidence.CapturedContainers, captured.MutatorTypeObservers{
		Value: func(point cfg.Point, valuePath constraint.Path, static typ.Type, template flow.ValueTemplate) typ.Type {
			if result.FlowSolution == nil {
				return static
			}
			return result.FlowSolution.MutatorValueTypeAt(point, valuePath, static, template)
		},
		Key: func(point cfg.Point, keyPath constraint.Path, static typ.Type) typ.Type {
			if result.FlowSolution == nil {
				return static
			}
			return result.FlowSolution.MutatorKeyTypeAt(point, keyPath, static)
		},
	})
	if len(mutations) > 0 {
		writer.mergeParentFactsForSymbol(fnSym, interprocdomain.CapturedContainerMutationsDelta(fnSym, mutations))
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
		store:       store,
		parent:      parent,
		currentKey:  currentKey,
		currentOK:   currentOK,
		currentSym:  currentSym,
		bodyContext: paramevidence.NewBodyPreconditionContext(graph, result, bindings).WithCurrentFunctionSymbol(currentSym),
		scopes:      result.Scopes,
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
			Observer: observation.FromFuncResult(result, functionfact.StoreProjectionLookup(store, functionfact.ProjectionSibling, api.PhaseScopeCompute, parent)),
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
	store                    Store
	parent                   *scope.State
	currentKey               api.GraphKey
	currentOK                bool
	currentSym               cfg.SymbolID
	bodyContext              paramevidence.BodyPreconditionContext
	scopes                   map[cfg.Point]*scope.State
	callEntry                paramevidence.CallEntryProjector
	expectedReceiverType     func(string) typ.Type
	methodResolvedOnReceiver func(cfg.Point, ast.Expr, string) bool
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
	if c == nil || c.store == nil || !c.currentOK || c.currentSym == 0 {
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
	preconditions := c.bodyContext.PreconditionsFromCall(p, evidence, expectedReceiver)
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
