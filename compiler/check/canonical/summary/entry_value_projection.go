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
	"github.com/wippyai/go-lua/types/flow/numeric"
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

func entryRuntimeSlot(callee FuncRef, call *ast.FuncCallExpr, runtimeIdx int, slotOf EntryValueParamSlot) (int, bool) {
	if slotOf == nil {
		return 0, false
	}
	_, slot, ok := slotOf(callee, call, runtimeIdx)
	return slot, ok
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
	Ref               FuncRef
	EntryCells        flow.CaptureCells
	EntryFunctionRefs flow.FunctionRefs
	EntryClosureRefs  flow.ClosureRefs
	EntryFacts        flow.BoundaryFacts
}

// CallEntryTargetResolver resolves one call site plus caller point state to the
// set of callee targets and their corresponding entry axes.
type CallEntryTargetResolver func(call *ast.FuncCallExpr, in *flow.PointState) []CallEntryTarget

// CallEntryCallbackResolver resolves a function-valued callback argument to the
// canonical function refs it should seed. rawSym is the CFG-normalized argument
// symbol when the call site provides one; expression fallback is only secondary.
// The bool reports whether resolution was authoritative; a present-but-unknown
// product axis may return (nil, true), deliberately blocking static fallback.
type CallEntryCallbackResolver func(arg ast.Expr, rawSym cfg.SymbolID, in *flow.PointState) ([]FuncRef, bool)

// CallEntryExpectedArgType returns the contextual type expected for a concrete
// source argument at a call site. Method shape and forced-receiver evidence are
// call-site facts, so the projection receives the point and full CallInfo rather
// than only the bare FuncCallExpr.
type CallEntryExpectedArgType func(point cfg.Point, info *cfg.CallInfo, in *flow.PointState, argIdx int) typ.Type

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
// entry-value vector. Seeds are additional evidence, not fixed-slot fallback.
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
// entry context with aggregate fallback evidence.
type EntryValueContextMerge struct {
	Fixed    EntryValues
	Fallback EntryValues
}

// Values returns fixed entry evidence plus fallback slots not explicitly fixed.
func (m EntryValueContextMerge) Values() EntryValues {
	return mergeEntryValuesWithFixed(m.Fixed, m.Fallback)
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

func joinCallEntryValue(out CallEntryValues, callee FuncRef, slot int, av product.AbstractValue) CallEntryValues {
	values := joinObservedEntryValue(out[callee], slot, av)
	if len(values) == 0 {
		return out
	}
	if out == nil {
		out = make(CallEntryValues)
	}
	out[callee] = values
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

// mergeEntryValuesWithFixed merges fallback evidence into fixed entry evidence,
// preserving every slot already present in fixed. This is the summary-key rule:
// explicit EntryValuesKey context wins; aggregate CallEntryValues fills only
// unspecified slots.
func mergeEntryValuesWithFixed(fixed, fallback EntryValues) EntryValues {
	if len(fixed) == 0 {
		return entryValuesDomain.Join(fallback, nil)
	}
	out := cloneEntryValues(fixed)
	for slot, av := range fallback {
		if _, ok := out[slot]; ok {
			continue
		}
		out[slot] = av
	}
	return entryValuesDomain.Join(out, nil)
}

// directCallEntryProductValuesWithParamCount projects already-solved runtime
// argument product values, including exact nil for omitted fixed parameters when
// supplied with a finite callee parameter layout.
func directCallEntryProductValuesWithParamCount(
	call *ast.FuncCallExpr,
	callee FuncRef,
	runtimeValues []product.AbstractValue,
	slotOf EntryValueParamSlot,
	slotCount EntryValueParamSlotCount,
) EntryValues {
	if call == nil || slotOf == nil {
		return nil
	}
	var out EntryValues
	for runtimeIdx, av := range runtimeValues {
		if av.IsZero() || product.Domain.Equal(av, product.Domain.Top()) {
			continue
		}
		slot, ok := entryRuntimeSlot(callee, call, runtimeIdx, slotOf)
		if !ok {
			continue
		}
		out = joinObservedEntryValue(out, slot, av)
	}
	out = joinOmittedFixedArgNil(out, len(runtimeValues), callee, call, slotOf, slotCount)
	if len(out) == 0 {
		return nil
	}
	return out
}

func joinOmittedFixedArgNil(out EntryValues, supplied int, callee FuncRef, call *ast.FuncCallExpr, slotOf EntryValueParamSlot, slotCount EntryValueParamSlotCount) EntryValues {
	if slotOf == nil || slotCount == nil || supplied < 0 {
		return out
	}
	limit := slotCount(callee, call)
	if limit <= supplied {
		return out
	}
	seenSlots := make(map[int]struct{}, supplied)
	for _, arg := range entryRuntimeArgs(callee, call, slotOf) {
		if arg.RuntimeIdx >= supplied {
			continue
		}
		if arg.Slot >= 0 {
			seenSlots[arg.Slot] = struct{}{}
		}
	}
	nilValue := product.FromType(typ.Nil)
	for runtimeIdx := supplied; runtimeIdx < limit; runtimeIdx++ {
		slot, ok := entryRuntimeSlot(callee, call, runtimeIdx, slotOf)
		if !ok {
			continue
		}
		if slot < 0 {
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

// CallEntryContextProjection projects exact call-site contexts from one solved
// caller state. Unlike Summary.CallEntryValues, this relation is not joined by
// callee: diagnostics use it to replay every reachable callee under the finite
// context that an actual call site produced.
type CallEntryContextProjection struct {
	Graph               *cfg.Graph
	State               state.FunctionState
	ResolveTargets      CallEntryTargetResolver
	ResolveCallback     CallEntryCallbackResolver
	ExpectedArgType     CallEntryExpectedArgType
	ParamSlot           EntryValueParamSlot
	ParamSlotCount      EntryValueParamSlotCount
	ParamPath           EntryReferenceParamPath
	ArgPath             EntryReferenceArgPath
	ReferenceArgSources EntryReferenceArgSources
	EvalArg             EntryValueEvaluator
	NormalizeValues     EntryValuesNormalizer
	ReferencePaths      EntryReferenceProjection
}

// DirectProductValues projects already-solved runtime argument values for one
// callee using this projection's runtime-slot layout.
func (p CallEntryContextProjection) DirectProductValues(callee FuncRef, call *ast.FuncCallExpr, runtimeValues []product.AbstractValue) EntryValues {
	return directCallEntryProductValuesWithParamCount(call, callee, runtimeValues, p.ParamSlot, p.ParamSlotCount)
}

// DirectFacts projects caller point-local path facts into parameter-relative
// facts for one callee using this projection's runtime-slot and argument-path
// layout.
func (p CallEntryContextProjection) DirectFacts(
	callee FuncRef,
	call *ast.FuncCallExpr,
	keyPresence flow.KeyPresenceFacts,
	num *numeric.State,
	indexWrites flow.IndexWriteAdmissionFacts,
) flow.BoundaryFacts {
	return directCallEntryFacts(directCallEntryFactInput{
		Call:        call,
		Callee:      callee,
		ParamSlot:   p.ParamSlot,
		ArgPath:     p.ArgPath,
		KeyPresence: keyPresence,
		Num:         num,
		IndexWrites: indexWrites,
	})
}

// DirectReferences projects callable-reference axes for one direct call using
// this projection's runtime-slot, parameter-path, and argument-path layout.
func (p CallEntryContextProjection) DirectReferences(
	callee FuncRef,
	call *ast.FuncCallExpr,
	state *flow.PointState,
	functionRefs flow.FunctionRefs,
	closureRefs flow.ClosureRefs,
	argSources EntryReferenceArgSources,
) (flow.FunctionRefs, flow.ClosureRefs) {
	return directCallEntryReferences(directCallEntryReferenceInput{
		Call:                call,
		Callee:              callee,
		ParamSlot:           p.ParamSlot,
		ParamPath:           p.ParamPath,
		ArgPath:             p.ArgPath,
		FunctionRefs:        functionRefs,
		ClosureRefs:         closureRefs,
		ReferenceProjection: p.referenceProjection(callee),
		LimitReferencePaths: p.ReferencePaths != nil,
		State:               state,
		ArgSources:          argSources,
	})
}

// ProjectKeys returns deterministic, de-duplicated callee summary keys for every
// module-local call site in Graph.
func (p CallEntryContextProjection) ProjectKeys() []Key {
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
			values := p.directProductValues(target.Ref, site.Call, &site.ArgState)
			if p.NormalizeValues != nil {
				values = p.NormalizeValues(target.Ref, site.Call, values)
			}
			refs, closures := p.directReferenceAxes(target.Ref, site.Call, &site.ArgState)
			refs = flow.FunctionRefsDomain.Join(target.EntryFunctionRefs, refs)
			closures = flow.ClosureRefsDomain.Join(target.EntryClosureRefs, closures)
			facts := flow.MergeBoundaryFactProofs(target.EntryFacts, p.directFacts(target.Ref, site.Call, &site.ArgState))
			key := NewKeyWithEntryContextFacts(
				target.Ref,
				target.EntryCells,
				refs,
				closures,
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
// CallEntryContextProjection: calls provide callee contexts at invocation sites;
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
	captured := closureCapturedSymbols(closure)
	entryCells := closure.EntryCells()
	liveRefs := flow.ProjectFunctionRefsBySymbols(point.FunctionRefs, captured)
	liveClosures := flow.ProjectClosureRefsBySymbols(point.ClosureRefs, captured)
	entryRefs := closure.EntryFunctionRefs()
	entryClosures := closure.EntryClosureRefs()
	if p.ReferencePaths != nil {
		projection := p.ReferencePaths(ref)
		entryCells = entryCells.ProjectPaths(projection)
		liveRefs = flow.ProjectFunctionRefsByReferencePaths(point.FunctionRefs, projection)
		liveClosures = flow.ProjectClosureRefsByReferencePaths(point.ClosureRefs, projection)
		entryRefs = flow.ProjectFunctionRefsByReferencePaths(entryRefs, projection)
		entryClosures = flow.ProjectClosureRefsByReferencePaths(entryClosures, projection)
	}
	return NewKeyWithEntryContextFacts(
		ref,
		entryCells,
		flow.OverlayFunctionRefs(entryRefs, liveRefs),
		flow.OverlayClosureRefs(entryClosures, liveClosures),
		nil,
		flow.BoundaryFactsDomain.Top(),
	)
}

func closureCapturedSymbols(closure flow.ClosureRef) []cfg.SymbolID {
	var symbols []cfg.SymbolID
	for _, entry := range closure.EntryCells().Entries() {
		if entry.Symbol != 0 {
			symbols = append(symbols, entry.Symbol)
		}
	}
	symbols = append(symbols, flow.FunctionRefRootSymbols(closure.EntryFunctionRefs())...)
	symbols = append(symbols, flow.ClosureRefRootSymbols(closure.EntryClosureRefs())...)
	if len(symbols) == 0 {
		return nil
	}
	slices.Sort(symbols)
	return slices.Compact(symbols)
}

func (p CallEntryContextProjection) directProductValues(callee FuncRef, call *ast.FuncCallExpr, in *flow.PointState) EntryValues {
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
	return directCallEntryProductValuesWithParamCount(call, callee, argValues, p.ParamSlot, p.ParamSlotCount)
}

func (p CallEntryContextProjection) directReferenceAxes(callee FuncRef, call *ast.FuncCallExpr, in *flow.PointState) (flow.FunctionRefs, flow.ClosureRefs) {
	if p.ParamSlot == nil || p.ParamPath == nil || in == nil {
		return flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()
	}
	return p.DirectReferences(callee, call, in, in.FunctionRefs, in.ClosureRefs, p.ReferenceArgSources)
}

func (p CallEntryContextProjection) directFacts(callee FuncRef, call *ast.FuncCallExpr, in *flow.PointState) flow.BoundaryFacts {
	if in == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	return p.DirectFacts(callee, call, in.KeyPresence, in.Num, in.IndexWrites)
}

func (p CallEntryContextProjection) referenceProjection(callee FuncRef) flow.ReferencePathProjection {
	if p.ReferencePaths == nil {
		return flow.ReferencePathProjection{}
	}
	return p.ReferencePaths(callee)
}

func (p CallEntryContextProjection) callbackEntryKeys(site callEntrySite) []Key {
	var keys []Key
	for _, callback := range callEntryCallbacks(site, p.ResolveCallback, p.ExpectedArgType, p.ReferenceArgSources.ClosureRefs) {
		for _, ref := range callback.Refs {
			emitted := false
			if callback.HasClosures {
				for _, closure := range callback.Closures.Refs() {
					if canonref.FromFlow(closure.Ref) != ref {
						continue
					}
					keys = append(keys, NewKeyWithEntryContextFacts(
						ref,
						closure.EntryCells(),
						closure.EntryFunctionRefs(),
						closure.EntryClosureRefs(),
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

// CallEntryValueProjection projects all call-entry values from one solved caller
// function state into its caller-visible Summary.CallEntryValues component.
type CallEntryValueProjection struct {
	Graph           *cfg.Graph
	State           state.FunctionState
	ResolveTargets  CallEntryTargetResolver
	ResolveCallback CallEntryCallbackResolver
	ExpectedArgType CallEntryExpectedArgType
	ParamSlot       EntryValueParamSlot
	ParamAnnotated  EntryValueParamAnnotated
	EvalArg         EntryValueEvaluator
}

// Project returns the finite caller-to-callee entry-value summary component.
func (p CallEntryValueProjection) Project() CallEntryValues {
	if p.Graph == nil || p.ResolveTargets == nil || p.ParamSlot == nil || p.EvalArg == nil {
		return nil
	}
	var out CallEntryValues
	p.Graph.EachCallSite(func(point cfg.Point, info *cfg.CallInfo) {
		site, ok := newCallEntrySite(p.State, point, info)
		if !ok {
			return
		}
		for _, target := range site.targets(p.ResolveTargets) {
			for _, arg := range entryRuntimeArgs(target.Ref, site.Call, p.ParamSlot) {
				if arg.Expr == nil || (p.ParamAnnotated != nil && p.ParamAnnotated(target.Ref, arg.SourceParam)) {
					continue
				}
				av, ok := p.EvalArg(&site.ArgState, arg.Expr)
				if !ok {
					continue
				}
				out = joinCallEntryValue(out, target.Ref, arg.Slot, av)
			}
		}
		out = p.projectCallbackEntryValues(out, site)
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p CallEntryValueProjection) projectCallbackEntryValues(out CallEntryValues, site callEntrySite) CallEntryValues {
	if p.ExpectedArgType == nil {
		return out
	}
	for _, callback := range callEntryCallbacks(site, p.ResolveCallback, p.ExpectedArgType, nil) {
		if !callback.HasValues || len(callback.Values) == 0 {
			continue
		}
		for _, ref := range callback.Refs {
			for slot, av := range callback.Values {
				out = joinCallEntryValue(out, ref, slot, av)
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
	expected CallEntryExpectedArgType,
	resolveClosures EntryClosureRefArgResolver,
) []callEntryCallback {
	if resolve == nil || site.Call == nil {
		return nil
	}
	var out []callEntryCallback
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
			if fn := unwrap.Function(expected(site.Point, site.Info, &site.EventState, argIdx)); fn != nil {
				callback.Values, callback.HasValues = callbackExpectedEntryValues(fn)
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

func sortFuncRefs(refs []FuncRef) {
	canonref.SortFuncRefs(refs)
}
