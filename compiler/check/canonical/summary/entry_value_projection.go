package summary

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// EntryValueParamSlot maps a normalized runtime argument index to the callee's
// source parameter and runtime slot. Runtime index 0 is the receiver for method
// calls and the first argument for plain calls.
type EntryValueParamSlot func(callee FuncRef, call *ast.FuncCallExpr, runtimeIdx int) (sourceParam int, slot int, ok bool)

// EntryValueParamSlotCount reports the finite fixed runtime parameter slots for
// a callee. Omitted-argument projection is bounded by this layout; variadic tail
// arguments are deliberately excluded because omission has no exact nil slot.
type EntryValueParamSlotCount func(callee FuncRef, call *ast.FuncCallExpr) int

type entryRuntimeArg struct {
	RuntimeIdx  int
	Expr        ast.Expr
	SourceParam int
	Slot        int
}

func entryRuntimeArgs(callee FuncRef, call *ast.FuncCallExpr, slotOf EntryValueParamSlot) []entryRuntimeArg {
	if call == nil || slotOf == nil {
		return nil
	}
	var out []entryRuntimeArg
	for runtimeIdx := 0; runtimeIdx < callsite.RuntimeArgExprCount(call); runtimeIdx++ {
		sourceParam, slot, ok := slotOf(callee, call, runtimeIdx)
		if !ok {
			continue
		}
		out = append(out, entryRuntimeArg{
			RuntimeIdx:  runtimeIdx,
			Expr:        callsite.RuntimeArgExprAt(call, runtimeIdx),
			SourceParam: sourceParam,
			Slot:        slot,
		})
	}
	return out
}

// EntryValueParamAnnotated reports whether a callee source parameter is fixed
// and therefore must not be inferred from aggregate entry evidence. Refinable
// structural annotations should return false here.
type EntryValueParamAnnotated func(callee FuncRef, sourceParam int) bool

// EntryValueEvaluator evaluates a call argument in the caller's current point
// state.
type EntryValueEvaluator func(in *flow.PointState, expr ast.Expr) (product.AbstractValue, bool)

// EntryValuesNormalizer rewrites direct call-site entry values before they are
// interned into a context key. It receives the source call so callers can limit
// topology enrichments to the call forms that actually carry that topology.
type EntryValuesNormalizer func(callee FuncRef, call *ast.FuncCallExpr, values EntryValues) EntryValues

// EntryReferenceProjection returns the callee-owned reference-axis vocabulary
// used to normalize exact call-entry contexts.
type EntryReferenceProjection func(callee FuncRef) flow.ReferencePathProjection

// CallEntryTarget is a resolved callee target plus the captured-entry context that
// must be used when summarizing that callee under this call.
type CallEntryTarget struct {
	Ref             FuncRef
	EntryReferences flow.ReferenceContext
	EntryFacts      flow.BoundaryFacts
}

// CallEntryTargetResolver resolves one call site plus caller point state to the
// set of callee targets and their corresponding entry axes.
type CallEntryTargetResolver func(call *ast.FuncCallExpr, in *flow.PointState) []CallEntryTarget

// CallEntryCallbackResolver resolves a function-valued callback argument to the
// canonical function refs it should seed. rawSym is the CFG-normalized argument
// symbol when the call site provides one; expression projection is only secondary.
// The bool reports whether resolution was authoritative; a present-but-unknown
// product axis may return (nil, true), deliberately blocking immutable topology
// projection.
type CallEntryCallbackResolver func(arg ast.Expr, rawSym cfg.SymbolID, in *flow.PointState) ([]FuncRef, bool)

// CallEntryExpectedArgTypes returns the contextual types expected at one call
// site. Method shape and forced-receiver evidence are call-site facts, so the
// projection receives the point and full CallInfo rather than only the bare
// FuncCallExpr. Callback projection indexes this vector instead of rebuilding
// call-boundary evidence once per argument.
type CallEntryExpectedArgTypes func(point cfg.Point, info *cfg.CallInfo, in *flow.PointState) []typ.Type

// EntryValuePrototypeReceiver maps a prototype-self relation to the callee entry
// parameter slot that receives runtime self.
type EntryValuePrototypeReceiver struct {
	Prototype cfg.SymbolID
	Slot      int
}

// EntryValuePrototypeSource is one summarized function component that may
// publish runtime self values for one or more prototypes.
type EntryValuePrototypeSource struct {
	Prototypes []cfg.SymbolID
	Self       flow.PrototypeSelf
}

// EntryValueAggregation is the summary-owned fold from caller entry-value and
// prototype-self components into a callee's entry-value seed.
type EntryValueAggregation struct {
	Callee                FuncRef
	HasInferredSlots      bool
	EachCallerEntryValues func(yield func(EntryValues))
	PrototypeReceivers    []EntryValuePrototypeReceiver
	EachPrototypeSource   func(yield func(EntryValuePrototypeSource))
	SlotDeclared          func(slot int) bool
}

// EntryValueSeed is declaration-context evidence for one callee entry slot.
// Summary owns lowering seeds into product entry values so callers do not
// mutate the EntryValues carrier directly.
type EntryValueSeed struct {
	Slot int
	Type typ.Type
}

// EntryValuesWithSeeds joins declaration-context seed types into an existing
// entry-value vector. Seeds are additional evidence, not fixed-slot substitutes.
func EntryValuesWithSeeds(values EntryValues, seeds []EntryValueSeed) EntryValues {
	out := values
	for _, seed := range seeds {
		out = joinEntryValue(out, seed.Slot, product.FromType(seed.Type))
	}
	return out
}

// EntryValuesFromFunctionParams lowers a callback signature's concrete
// parameter types into entry evidence for contextual callback synthesis.
func EntryValuesFromFunctionParams(fn *typ.Function) EntryValues {
	if fn == nil || len(fn.Params) == 0 {
		return nil
	}
	var out EntryValues
	for slot, param := range fn.Params {
		if param.Type == nil || typ.IsAbsentOrUnknown(param.Type) || typ.IsAny(param.Type) || typ.ContainsTypeParam(param.Type) {
			continue
		}
		out = joinEntryValue(out, slot, product.FromType(param.Type))
	}
	return out
}

// EntryValueContextMerge applies the summary-key rule for combining an exact
// entry context with aggregate projection evidence.
type EntryValueContextMerge struct {
	Fixed     EntryValues
	Aggregate EntryValues
}

// Values returns fixed entry evidence plus aggregate slots not explicitly fixed.
func (m EntryValueContextMerge) Values() EntryValues {
	return mergeEntryValuesWithAggregate(m.Fixed, m.Aggregate)
}

// joinEntryValue adds av to one callee entry slot under product-domain join.
func joinEntryValue(out EntryValues, slot int, av product.AbstractValue) EntryValues {
	if slot < 0 || av.IsZero() {
		return out
	}
	if out == nil {
		out = make(EntryValues)
	}
	if prev, had := out[slot]; had {
		out[slot] = product.Domain.Join(prev, av)
	} else {
		out[slot] = av
	}
	return out
}

func joinObservedEntryValue(out EntryValues, slot int, av product.AbstractValue) EntryValues {
	if slot < 0 || av.IsZero() {
		return out
	}
	if t := av.ProjectValue(); t == nil || typ.IsAbsentOrUnknown(t) {
		return out
	}
	return joinEntryValue(out, slot, av)
}

// AggregateEntryFacts folds caller-projected path proofs for one callee. Facts
// are must evidence: every possible caller contributes a BoundaryFacts value,
// with Top meaning "this caller proves nothing finite".
func AggregateEntryFacts(eachCallerFacts func(yield func(flow.BoundaryFacts))) flow.BoundaryFacts {
	if eachCallerFacts == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	out := flow.BoundaryFactsDomain.Bottom()
	seen := false
	eachCallerFacts(func(facts flow.BoundaryFacts) {
		if !facts.HasProof() {
			facts = flow.BoundaryFactsDomain.Top()
		}
		out = flow.BoundaryFactsDomain.Join(out, facts)
		seen = true
	})
	if !seen || flow.BoundaryFactsDomain.Equal(out, flow.BoundaryFactsDomain.Bottom()) {
		return flow.BoundaryFactsDomain.Top()
	}
	return out
}

// AggregateEntryValues folds all summary-visible entry evidence for one callee.
// SlotDeclared denotes fixed, non-refinable declarations. Refinable structural
// annotations such as {any} are intentionally not fixed: EntrySeedEffect composes
// caller evidence with the declaration instead of letting the annotation erase
// useful interior shape.
func AggregateEntryValues(in EntryValueAggregation) EntryValues {
	var out EntryValues
	declared := func(slot int) bool {
		return in.SlotDeclared != nil && in.SlotDeclared(slot)
	}
	if in.HasInferredSlots && in.EachCallerEntryValues != nil {
		in.EachCallerEntryValues(func(values EntryValues) {
			for slot, av := range values {
				if declared(slot) {
					continue
				}
				out = joinEntryValue(out, slot, av)
			}
		})
	}
	receivers := make([]EntryValuePrototypeReceiver, 0, len(in.PrototypeReceivers))
	for _, receiver := range in.PrototypeReceivers {
		if receiver.Prototype == 0 || receiver.Slot < 0 || declared(receiver.Slot) {
			continue
		}
		receivers = append(receivers, receiver)
	}
	if len(receivers) > 0 && in.EachPrototypeSource != nil {
		in.EachPrototypeSource(func(source EntryValuePrototypeSource) {
			for _, receiver := range receivers {
				if !entryValueSourcePublishes(source, receiver.Prototype) {
					continue
				}
				av, ok := source.Self.Value(receiver.Prototype)
				if !ok || av.IsZero() {
					continue
				}
				out = joinEntryValue(out, receiver.Slot, av)
			}
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func entryValueSourcePublishes(source EntryValuePrototypeSource, proto cfg.SymbolID) bool {
	if proto == 0 {
		return false
	}
	for _, published := range source.Prototypes {
		if published == proto {
			return true
		}
	}
	return false
}

// mergeEntryValuesWithAggregate merges aggregate evidence into fixed entry evidence,
// preserving every slot already present in fixed. This is the summary-key rule:
// explicit EntryValuesKey context wins; aggregate CallEntryPublication fills only
// unspecified slots.
func mergeEntryValuesWithAggregate(fixed, aggregate EntryValues) EntryValues {
	if len(fixed) == 0 {
		return entryValuesDomain.Join(aggregate, nil)
	}
	out := cloneEntryValues(fixed)
	for slot, av := range aggregate {
		if _, ok := out[slot]; ok {
			continue
		}
		out[slot] = av
	}
	return entryValuesDomain.Join(out, nil)
}

type directCallEntryProductValueInput struct {
	Call           *ast.FuncCallExpr
	Callee         FuncRef
	RuntimeValues  []product.AbstractValue
	ParamSlot      EntryValueParamSlot
	ParamSlotCount EntryValueParamSlotCount
	ParamAnnotated EntryValueParamAnnotated
}

// directCallEntryProductValues projects already-solved runtime argument product
// values, including exact nil for omitted fixed parameters when supplied with a
// finite callee parameter layout.
func directCallEntryProductValues(in directCallEntryProductValueInput) EntryValues {
	if in.Call == nil || in.ParamSlot == nil {
		return nil
	}
	var out EntryValues
	for runtimeIdx, av := range in.RuntimeValues {
		if av.IsZero() || product.Domain.Equal(av, product.Domain.Top()) {
			continue
		}
		sourceParam, slot, ok := in.ParamSlot(in.Callee, in.Call, runtimeIdx)
		if !ok {
			continue
		}
		if in.ParamAnnotated != nil && in.ParamAnnotated(in.Callee, sourceParam) {
			continue
		}
		out = joinObservedEntryValue(out, slot, av)
	}
	out = joinOmittedFixedArgNil(out, in)
	if len(out) == 0 {
		return nil
	}
	return out
}

func withoutMethodReceiverEntryValue(values EntryValues, paramSlot EntryValueParamSlot, callee FuncRef, call *ast.FuncCallExpr) EntryValues {
	fresh, slot := methodReceiverEntryValueIsFresh(values, paramSlot, callee, call)
	if !fresh {
		return values
	}
	out := make(EntryValues, len(values)-1)
	for k, v := range values {
		if k != slot {
			out[k] = v
		}
	}
	return out
}

func methodReceiverEntryValueIsFresh(values EntryValues, paramSlot EntryValueParamSlot, callee FuncRef, call *ast.FuncCallExpr) (bool, int) {
	if len(values) == 0 || paramSlot == nil || call == nil || call.Method == "" {
		return false, 0
	}
	_, slot, ok := paramSlot(callee, call, 0)
	if !ok {
		return false, 0
	}
	av, ok := values[slot]
	if !ok || !av.IsFreshAllocation() {
		return false, 0
	}
	return true, slot
}

func joinOmittedFixedArgNil(out EntryValues, in directCallEntryProductValueInput) EntryValues {
	supplied := len(in.RuntimeValues)
	if in.ParamSlot == nil || in.ParamSlotCount == nil || supplied < 0 {
		return out
	}
	limit := in.ParamSlotCount(in.Callee, in.Call)
	if limit <= supplied {
		return out
	}
	seenSlots := make(map[int]struct{}, supplied)
	for _, arg := range entryRuntimeArgs(in.Callee, in.Call, in.ParamSlot) {
		if arg.RuntimeIdx >= supplied {
			continue
		}
		if arg.Slot >= 0 {
			seenSlots[arg.Slot] = struct{}{}
		}
	}
	nilValue := product.FromType(typ.Nil)
	for runtimeIdx := supplied; runtimeIdx < limit; runtimeIdx++ {
		sourceParam, slot, ok := in.ParamSlot(in.Callee, in.Call, runtimeIdx)
		if !ok {
			continue
		}
		if slot < 0 {
			continue
		}
		if in.ParamAnnotated != nil && in.ParamAnnotated(in.Callee, sourceParam) {
			continue
		}
		if _, seen := seenSlots[slot]; seen {
			continue
		}
		seenSlots[slot] = struct{}{}
		out = joinObservedEntryValue(out, slot, nilValue)
	}
	return out
}

// CallEntryProjection projects call-site entry evidence from one solved caller
// state. Exact summary keys and aggregate publication are two views of the same
// direct call-entry evidence; they must not rebuild separate value/fact routes.
type CallEntryProjection struct {
	CallerRef           FuncRef
	Graph               *cfg.Graph
	State               state.FunctionState
	ResolveTargets      CallEntryTargetResolver
	ResolveCallback     CallEntryCallbackResolver
	ExpectedArgTypes    CallEntryExpectedArgTypes
	ParamSlot           EntryValueParamSlot
	ParamAnnotated      EntryValueParamAnnotated
	ParamSlotCount      EntryValueParamSlotCount
	ParamPath           EntryReferenceParamPath
	ArgPath             EntryReferenceArgPath
	ReferenceArgSources EntryReferenceArgSources
	BoundaryArgSources  EntryBoundaryFactArgSources
	EvalArg             EntryValueEvaluator
	NormalizeValues     EntryValuesNormalizer
	ReferencePaths      EntryReferenceProjection
}

// DirectEntryEvidenceInput is the normalized caller-side evidence for one
// resolved call target. Runtime argument values, callable references, and path
// facts are kept together because they describe the same call-boundary frame.
type DirectEntryEvidenceInput struct {
	Callee FuncRef
	Call   *ast.FuncCallExpr

	State         *flow.PointState
	RuntimeValues []product.AbstractValue

	ParamAnnotated EntryValueParamAnnotated
	References     flow.ReferenceContext
	ArgSources     EntryReferenceArgSources
	ArgFacts       EntryBoundaryFactArgSources

	BoundaryFacts flow.BoundaryFactProjectionInput
}

// DirectEntryEvidence is the callee-entry evidence produced by one direct call.
type DirectEntryEvidence struct {
	Values     EntryValues
	References flow.ReferenceContext
	Facts      flow.BoundaryFacts
}

// DirectEvidence projects all direct call-entry axes for one resolved target.
// Call-entry consumers should use this carrier instead of independently
// rebuilding value, reference, and fact routes from the same call arguments.
func (p CallEntryProjection) DirectEvidence(in DirectEntryEvidenceInput) DirectEntryEvidence {
	return DirectEntryEvidence{
		Values: directCallEntryProductValues(directCallEntryProductValueInput{
			Call:           in.Call,
			Callee:         in.Callee,
			RuntimeValues:  in.RuntimeValues,
			ParamSlot:      p.ParamSlot,
			ParamSlotCount: p.ParamSlotCount,
			ParamAnnotated: in.ParamAnnotated,
		}),
		References: directCallEntryReferences(directCallEntryReferenceInput{
			Call:                in.Call,
			Callee:              in.Callee,
			ParamSlot:           p.ParamSlot,
			ParamPath:           p.ParamPath,
			ArgPath:             p.ArgPath,
			References:          in.References,
			ReferenceProjection: p.referenceProjection(in.Callee),
			LimitReferencePaths: p.ReferencePaths != nil,
			State:               in.State,
			ArgSources:          in.ArgSources,
		}),
		Facts: directCallEntryFacts(directCallEntryFactInput{
			Call:          in.Call,
			Callee:        in.Callee,
			ParamSlot:     p.ParamSlot,
			ArgPath:       p.ArgPath,
			ArgValues:     in.RuntimeValues,
			State:         in.State,
			ArgFacts:      in.ArgFacts,
			BoundaryFacts: in.BoundaryFacts,
		}),
	}
}

func (p CallEntryProjection) directEvidenceFromPoint(callee FuncRef, call *ast.FuncCallExpr, in *flow.PointState, annotated EntryValueParamAnnotated) DirectEntryEvidence {
	if in == nil {
		return DirectEntryEvidence{
			References: flow.ReferenceContextBottom(),
			Facts:      flow.BoundaryFactsDomain.Top(),
		}
	}
	return p.DirectEvidence(DirectEntryEvidenceInput{
		Callee:         callee,
		Call:           call,
		State:          in,
		RuntimeValues:  p.runtimeArgValues(call, in),
		ParamAnnotated: annotated,
		References:     flow.ReferenceContextFromPoint(in),
		ArgSources:     p.ReferenceArgSources,
		ArgFacts:       p.BoundaryArgSources,
		BoundaryFacts:  flow.BoundaryFactProjectionInputOfPoint(in),
	})
}

func (p CallEntryProjection) runtimeArgValues(call *ast.FuncCallExpr, in *flow.PointState) []product.AbstractValue {
	if p.ParamSlot == nil || p.EvalArg == nil || call == nil || in == nil {
		return nil
	}
	argValues := make([]product.AbstractValue, callsite.RuntimeArgExprCount(call))
	for i := range argValues {
		arg := callsite.RuntimeArgExprAt(call, i)
		if arg == nil {
			continue
		}
		av, ok := p.EvalArg(in, arg)
		if !ok {
			continue
		}
		argValues[i] = av
	}
	return argValues
}

// ProjectKeys returns deterministic, de-duplicated callee summary keys for every
// module-local call site in Graph.
func (p CallEntryProjection) ProjectKeys() []Key {
	if p.Graph == nil || p.ResolveTargets == nil {
		return nil
	}
	seen := make(map[Key]struct{})
	var out []Key
	p.Graph.EachCallSite(func(point cfg.Point, info *cfg.CallInfo) {
		site, ok := newCallEntrySite(p.State, point, info)
		if !ok {
			return
		}
		for _, target := range site.targets(p.ResolveTargets) {
			evidence := p.directEvidenceFromPoint(target.Ref, site.Call, &site.ArgState, nil)
			values := evidence.Values
			if p.NormalizeValues != nil {
				values = p.NormalizeValues(target.Ref, site.Call, values)
			}
			references := target.EntryReferences.Join(evidence.References)
			facts := flow.UnionBoundaryFactProofs(target.EntryFacts, evidence.Facts)
			if target.Ref == p.CallerRef {
				// Self-recursive calls must close over the summary fixed point. Boundary
				// facts and receiver products are replayed into the callee state, but
				// using mutable self proof state as exact-key identity creates a fresh
				// context on every lap.
				if fresh, _ := methodReceiverEntryValueIsFresh(values, p.ParamSlot, target.Ref, site.Call); fresh {
					values = withoutMethodReceiverEntryValue(values, p.ParamSlot, target.Ref, site.Call)
				}
				facts = flow.BoundaryFactsDomain.Top()
			}
			key := NewKeyWithReferenceContext(
				target.Ref,
				references,
				values,
				facts,
			)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
		for _, key := range p.callbackEntryKeys(site) {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
	})
	return out
}

// ClosureEntryContextProjection projects declaration-time closure environments
// from one solved function state. It is the diagnostic counterpart to
// CallEntryProjection: calls provide callee contexts at invocation sites;
// closure values provide nested-function contexts at allocation/definition
// sites, including captured lexical narrows for closures that are not called in
// the current module.
type ClosureEntryContextProjection struct {
	State          state.FunctionState
	ReferencePaths EntryReferenceProjection
}

// ProjectKeys returns deterministic, de-duplicated summary keys for every
// finite closure value carried by the solved point state.
func (p ClosureEntryContextProjection) ProjectKeys() []Key {
	seen := make(map[Key]struct{})
	var out []Key
	out = p.projectPointMap(out, seen, p.State.Points)
	out = p.projectPointMap(out, seen, p.State.InPoints)
	return out
}

func (p ClosureEntryContextProjection) projectPointMap(out []Key, seen map[Key]struct{}, points map[cfg.Point]flow.PointState) []Key {
	if len(points) == 0 {
		return out
	}
	ordered := make([]cfg.Point, 0, len(points))
	for point := range points {
		ordered = append(ordered, point)
	}
	slices.Sort(ordered)
	for _, point := range ordered {
		out = p.projectClosureRefs(out, seen, points[point])
	}
	return out
}

func (p ClosureEntryContextProjection) projectClosureRefs(out []Key, seen map[Key]struct{}, point flow.PointState) []Key {
	refs := point.ClosureRefs
	if len(refs) == 0 {
		return out
	}
	paths := make([]constraint.PathKey, 0, len(refs))
	for path := range refs {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		for _, closure := range refs[path].Refs() {
			ref := canonref.FromFlow(closure.Ref)
			if ref == (FuncRef{}) {
				continue
			}
			key := p.closureEntryContextKey(ref, closure, point)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
	}
	return out
}

func (p ClosureEntryContextProjection) closureEntryContextKey(ref FuncRef, closure flow.ClosureRef, point flow.PointState) Key {
	entry := closure.EntryReferenceContext()
	live := flow.ReferenceContextWithStaticMembersFromPoint(&point).ProjectSymbols(entry.RootSymbols())
	if p.ReferencePaths != nil {
		projection := p.ReferencePaths(ref)
		entry = entry.ProjectPaths(projection)
		live = flow.ReferenceContextWithStaticMembersFromPoint(&point).ProjectPaths(projection)
	}
	return NewKeyWithReferenceContext(
		ref,
		flow.OverlayReferenceContext(entry, live),
		nil,
		flow.BoundaryFactsDomain.Top(),
	)
}

func (p CallEntryProjection) referenceProjection(callee FuncRef) flow.ReferencePathProjection {
	if p.ReferencePaths == nil {
		return flow.ReferencePathProjection{}
	}
	return p.ReferencePaths(callee)
}

func (p CallEntryProjection) callbackEntryKeys(site callEntrySite) []Key {
	var keys []Key
	for _, callback := range callEntryCallbacks(site, p.ResolveCallback, p.ExpectedArgTypes, p.ReferenceArgSources.ClosureRefs) {
		for _, ref := range callback.Refs {
			emitted := false
			if callback.HasClosures {
				for _, closure := range callback.Closures.Refs() {
					if canonref.FromFlow(closure.Ref) != ref {
						continue
					}
					keys = append(keys, NewKeyWithReferenceContext(
						ref,
						closure.EntryReferenceContext(),
						callback.Values,
						flow.BoundaryFactsDomain.Top(),
					))
					emitted = true
				}
			}
			if !emitted {
				keys = append(keys, NewDefaultKey(ref, callback.Values))
			}
		}
	}
	return keys
}

// Project returns finite caller-to-callee entry-publication summary components.
func (p CallEntryProjection) ProjectPublications() CallEntryPublications {
	if p.Graph == nil || p.ResolveTargets == nil || p.ParamSlot == nil || p.EvalArg == nil {
		return nil
	}
	blockedFacts := make(map[FuncRef]struct{})
	var out CallEntryPublications
	p.Graph.EachCallSite(func(point cfg.Point, info *cfg.CallInfo) {
		site, ok := newCallEntrySite(p.State, point, info)
		if !ok {
			return
		}
		for _, target := range site.targets(p.ResolveTargets) {
			// Publication is aggregate entry evidence. Fixed source annotations
			// remain fixed at their declaration site; exact call-entry contexts
			// still retain the full DirectEvidence payload.
			evidence := p.directEvidenceFromPoint(target.Ref, site.Call, &site.ArgState, p.ParamAnnotated)
			out = joinCallEntryPublicationValues(out, target.Ref, evidence.Values)
			facts := flow.UnionBoundaryFactProofs(target.EntryFacts, evidence.Facts)
			out = joinCallEntryPublicationFacts(out, blockedFacts, target.Ref, facts)
		}
		out = p.projectCallbackEntryValues(out, site)
	})
	return out
}

func joinCallEntryPublicationValues(out CallEntryPublications, ref FuncRef, values EntryValues) CallEntryPublications {
	for slot, av := range values {
		out = joinCallEntryPublicationValue(out, ref, slot, av)
	}
	return out
}

func joinCallEntryPublicationValue(out CallEntryPublications, ref FuncRef, slot int, av product.AbstractValue) CallEntryPublications {
	if slot < 0 || av.IsZero() {
		return out
	}
	if t := av.ProjectValue(); t == nil || typ.IsAbsentOrUnknown(t) {
		return out
	}
	if out == nil {
		out = make(CallEntryPublications)
	}
	pub := out[ref]
	pub.Values = joinEntryValue(pub.Values, slot, av)
	if !pub.Facts.HasProof() {
		pub.Facts = flow.BoundaryFactsDomain.Top()
	}
	out[ref] = pub
	return out
}

// joinCallEntryPublicationFacts folds finite must-facts per callee. A call to the same
// callee that proves no finite fact weakens the callee to Top and blocks later
// call sites from reintroducing a definite proof.
func joinCallEntryPublicationFacts(out CallEntryPublications, blocked map[FuncRef]struct{}, ref FuncRef, facts flow.BoundaryFacts) CallEntryPublications {
	if out == nil {
		out = make(CallEntryPublications)
	}
	pub := out[ref]
	if _, weak := blocked[ref]; weak {
		pub.Facts = flow.BoundaryFactsDomain.Top()
		out[ref] = pub
		return out
	}
	if !facts.HasProof() {
		pub.Facts = flow.BoundaryFactsDomain.Top()
		out[ref] = pub
		blocked[ref] = struct{}{}
		return out
	}
	if pub.Facts.HasProof() {
		facts = flow.BoundaryFactsDomain.Join(pub.Facts, facts)
		if !facts.HasProof() {
			pub.Facts = flow.BoundaryFactsDomain.Top()
			out[ref] = pub
			blocked[ref] = struct{}{}
			return out
		}
	}
	pub.Facts = facts.Clone()
	out[ref] = pub
	return out
}

func (p CallEntryProjection) projectCallbackEntryValues(out CallEntryPublications, site callEntrySite) CallEntryPublications {
	if p.ExpectedArgTypes == nil {
		return out
	}
	for _, callback := range callEntryCallbacks(site, p.ResolveCallback, p.ExpectedArgTypes, nil) {
		if !callback.HasValues || len(callback.Values) == 0 {
			continue
		}
		for _, ref := range callback.Refs {
			for slot, av := range callback.Values {
				out = joinCallEntryPublicationValue(out, ref, slot, av)
			}
		}
	}
	return out
}

type callEntryCallback struct {
	Refs        []FuncRef
	Values      EntryValues
	HasValues   bool
	Closures    flow.ClosureRefSet
	HasClosures bool
}

func callEntryCallbacks(
	site callEntrySite,
	resolve CallEntryCallbackResolver,
	expected CallEntryExpectedArgTypes,
	resolveClosures EntryClosureRefArgResolver,
) []callEntryCallback {
	if resolve == nil || site.Call == nil {
		return nil
	}
	var out []callEntryCallback
	var expectedTypes []typ.Type
	loadedExpected := false
	for argIdx, arg := range site.Call.Args {
		if arg == nil {
			continue
		}
		refs, ok := resolve(arg, callInfoArgSymbol(site.Info, argIdx), &site.EventState)
		if !ok || len(refs) == 0 {
			continue
		}
		callback := callEntryCallback{Refs: refs}
		if expected != nil {
			if !loadedExpected {
				expectedTypes = expected(site.Point, site.Info, &site.EventState)
				loadedExpected = true
			}
			if argIdx < len(expectedTypes) {
				if fn := unwrap.Function(expectedTypes[argIdx]); fn != nil {
					callback.Values, callback.HasValues = callbackExpectedEntryValues(fn)
				}
			}
		}
		if resolveClosures != nil {
			callback.Closures, callback.HasClosures = resolveClosures(argIdx, arg, &site.EventState)
		}
		out = append(out, callback)
	}
	return out
}

func callbackExpectedEntryValues(fn *typ.Function) (EntryValues, bool) {
	if fn == nil {
		return nil, false
	}
	if len(fn.Params) == 0 {
		return nil, true
	}
	var values EntryValues
	for slot, param := range fn.Params {
		if param.Type == nil || typ.ContainsTypeParam(param.Type) {
			continue
		}
		values = joinEntryValue(values, slot, product.FromType(param.Type))
	}
	return values, len(values) != 0
}

func callInfoCall(info *cfg.CallInfo) *ast.FuncCallExpr {
	if info == nil {
		return nil
	}
	return info.Call
}

func callInfoArgSymbol(info *cfg.CallInfo, argIdx int) cfg.SymbolID {
	if info == nil || argIdx < 0 || argIdx >= len(info.ArgSymbols) {
		return 0
	}
	return info.ArgSymbols[argIdx]
}

type callEntrySite struct {
	Point       cfg.Point
	Info        *cfg.CallInfo
	Call        *ast.FuncCallExpr
	EventState  flow.PointState
	TargetState flow.PointState
	ArgState    flow.PointState
}

func newCallEntrySite(fs state.FunctionState, point cfg.Point, info *cfg.CallInfo) (callEntrySite, bool) {
	call := callInfoCall(info)
	if call == nil {
		return callEntrySite{}, false
	}
	event, ok := callEventState(fs, point)
	if !ok {
		return callEntrySite{}, false
	}
	target := callTargetState(fs, point, event)
	return callEntrySite{
		Point:       point,
		Info:        info,
		Call:        call,
		EventState:  event,
		TargetState: target,
		ArgState:    callArgumentState(call, target, event),
	}, true
}

func (s callEntrySite) targets(resolve CallEntryTargetResolver) []CallEntryTarget {
	return resolveCallEntryTargets(resolve, s.Call, s.TargetState, s.EventState)
}

func callEventState(fs state.FunctionState, point cfg.Point) (flow.PointState, bool) {
	// Call arguments are evaluated at the call event inside the node transfer,
	// after any same-node stage facts such as generic-for target binding have
	// been installed. Use the point OUT-state as the canonical call-event stage;
	// fall back to IN-state only for older synthetic states that do not populate
	// Points.
	in, ok := fs.Points[point]
	if ok {
		return in, true
	}
	in, ok = fs.InPoints[point]
	return in, ok
}

func callTargetState(fs state.FunctionState, point cfg.Point, event flow.PointState) flow.PointState {
	if in, ok := fs.InPoints[point]; ok {
		return in
	}
	return event
}

func callArgumentState(call *ast.FuncCallExpr, target, event flow.PointState) flow.PointState {
	if call != nil && call.Method != "" {
		return target
	}
	return event
}

func resolveCallEntryTargets(resolve CallEntryTargetResolver, call *ast.FuncCallExpr, target, event flow.PointState) []CallEntryTarget {
	if resolve == nil {
		return nil
	}
	targets := resolve(call, &target)
	if len(targets) != 0 || flow.PointStateDomain.Equal(target, event) {
		return targets
	}
	return resolve(call, &event)
}
