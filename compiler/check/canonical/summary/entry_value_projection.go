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

// CallEntryTarget is a resolved callee target plus the captured-entry context that
// must be used when summarizing that callee under this call.
type CallEntryTarget struct {
	Ref               FuncRef
	EntryCells        flow.CaptureCells
	EntryFunctionRefs flow.FunctionRefs
	EntryClosureRefs  flow.ClosureRefs
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

// JoinEntryValue adds av to one callee entry slot under product-domain join.
func JoinEntryValue(out EntryValues, slot int, av product.AbstractValue) EntryValues {
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

// JoinObservedEntryValue adds av when it carries informative runtime value
// evidence. It rejects top-like projections so summaries do not learn precision
// from unknown/absent arguments.
func JoinObservedEntryValue(out EntryValues, slot int, av product.AbstractValue) EntryValues {
	if slot < 0 || av.IsZero() {
		return out
	}
	if t := av.ProjectValue(); t == nil || typ.IsAbsentOrUnknown(t) {
		return out
	}
	return JoinEntryValue(out, slot, av)
}

// JoinCallEntryValue adds av to the caller-summary map for one callee.
func JoinCallEntryValue(out CallEntryValues, callee FuncRef, slot int, av product.AbstractValue) CallEntryValues {
	values := JoinObservedEntryValue(out[callee], slot, av)
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
				out = JoinEntryValue(out, slot, av)
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
				out = JoinEntryValue(out, receiver.Slot, av)
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

// MergeEntryValuesWithFixed merges fallback evidence into fixed entry evidence,
// preserving every slot already present in fixed. This is the summary-key rule:
// explicit EntryValuesKey context wins; aggregate CallEntryValues fills only
// unspecified slots.
func MergeEntryValuesWithFixed(fixed, fallback EntryValues) EntryValues {
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

// DirectCallEntryValues projects one concrete call site's runtime arguments into
// the callee entry-value context key. Exact context keys carry declared and
// inferred slots alike; the callee entry reducer composes these values with
// declared annotations. Aggregate CallEntryValues fallback remains responsible
// for skipping declared slots.
func DirectCallEntryValues(
	call *ast.FuncCallExpr,
	callee FuncRef,
	typeOf func(ast.Expr) typ.Type,
	slotOf EntryValueParamSlot,
) EntryValues {
	return DirectCallEntryValuesWithParamCount(call, callee, typeOf, slotOf, nil)
}

// DirectCallEntryValuesWithParamCount projects one concrete call site's runtime
// arguments into the callee entry-value context key, including exact nil for
// omitted fixed parameters when the caller supplies the callee's finite parameter
// layout.
func DirectCallEntryValuesWithParamCount(
	call *ast.FuncCallExpr,
	callee FuncRef,
	typeOf func(ast.Expr) typ.Type,
	slotOf EntryValueParamSlot,
	slotCount EntryValueParamSlotCount,
) EntryValues {
	if call == nil || typeOf == nil || slotOf == nil {
		return nil
	}
	var out EntryValues
	for runtimeIdx := 0; runtimeIdx < callsite.RuntimeArgExprCount(call); runtimeIdx++ {
		arg := callsite.RuntimeArgExprAt(call, runtimeIdx)
		if arg == nil {
			continue
		}
		_, slot, ok := slotOf(callee, call, runtimeIdx)
		if !ok {
			continue
		}
		t := typeOf(arg)
		if t == nil || typ.IsAbsentOrUnknown(t) {
			continue
		}
		out = JoinEntryValue(out, slot, product.FromType(t))
	}
	out = joinOmittedFixedArgNil(out, callsite.RuntimeArgExprCount(call), callee, call, slotOf, slotCount)
	if len(out) == 0 {
		return nil
	}
	return out
}

// DirectCallEntryProductValues projects one concrete call site's already-solved
// runtime argument product values into the callee entry-value context key.
func DirectCallEntryProductValues(
	call *ast.FuncCallExpr,
	callee FuncRef,
	runtimeValues []product.AbstractValue,
	slotOf EntryValueParamSlot,
) EntryValues {
	return DirectCallEntryProductValuesWithParamCount(call, callee, runtimeValues, slotOf, nil)
}

// DirectCallEntryProductValuesWithParamCount projects already-solved runtime
// argument product values, including exact nil for omitted fixed parameters when
// supplied with a finite callee parameter layout.
func DirectCallEntryProductValuesWithParamCount(
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
		_, slot, ok := slotOf(callee, call, runtimeIdx)
		if !ok {
			continue
		}
		out = JoinObservedEntryValue(out, slot, av)
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
	for runtimeIdx := 0; runtimeIdx < supplied; runtimeIdx++ {
		_, slot, ok := slotOf(callee, call, runtimeIdx)
		if ok && slot >= 0 {
			seenSlots[slot] = struct{}{}
		}
	}
	nilValue := product.FromType(typ.Nil)
	for runtimeIdx := supplied; runtimeIdx < limit; runtimeIdx++ {
		_, slot, ok := slotOf(callee, call, runtimeIdx)
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
		out = JoinObservedEntryValue(out, slot, nilValue)
	}
	return out
}

// CallEntryContextProjection projects exact call-site contexts from one solved
// caller state. Unlike Summary.CallEntryValues, this relation is not joined by
// callee: diagnostics use it to replay every reachable callee under the finite
// context that an actual call site produced.
type CallEntryContextProjection struct {
	Graph              *cfg.Graph
	State              state.FunctionState
	ResolveTargets     CallEntryTargetResolver
	ResolveCallback    CallEntryCallbackResolver
	ExpectedArgType    CallEntryExpectedArgType
	ParamSlot          EntryValueParamSlot
	ParamSlotCount     EntryValueParamSlotCount
	ParamPath          EntryReferenceParamPath
	ArgPath            EntryReferenceArgPath
	FunctionArgRefs    EntryFunctionRefArgResolver
	FunctionArgRefTree EntryFunctionRefsArgResolver
	ClosureArgRefs     EntryClosureRefArgResolver
	ClosureArgRefTree  EntryClosureRefsArgResolver
	EvalArg            EntryValueEvaluator
	NormalizeValues    EntryValuesNormalizer
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
		if info == nil || info.Call == nil {
			return
		}
		in, ok := p.callEventState(point)
		if !ok {
			return
		}
		targets := p.ResolveTargets(info.Call, &in)
		for _, target := range targets {
			values := p.directProductValues(target.Ref, info.Call, &in)
			if p.NormalizeValues != nil {
				values = p.NormalizeValues(target.Ref, info.Call, values)
			}
			refs := flow.FunctionRefsDomain.Join(target.EntryFunctionRefs, p.directFunctionRefs(target.Ref, info.Call, &in))
			closures := flow.ClosureRefsDomain.Join(target.EntryClosureRefs, p.directClosureRefs(target.Ref, info.Call, &in))
			key := NewKeyWithEntryContext(
				target.Ref,
				target.EntryCells,
				refs,
				closures,
				values,
			)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
		for _, key := range p.callbackEntryKeys(point, info, &in) {
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
	State state.FunctionState
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
		out = p.projectClosureRefs(out, seen, points[point].ClosureRefs)
	}
	return out
}

func (p ClosureEntryContextProjection) projectClosureRefs(out []Key, seen map[Key]struct{}, refs flow.ClosureRefs) []Key {
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
			key := NewKeyWithEntryContext(
				ref,
				closure.EntryCells(),
				closure.EntryFunctionRefs(),
				closure.EntryClosureRefs(),
				nil,
			)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
	}
	return out
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
	return DirectCallEntryProductValuesWithParamCount(call, callee, argValues, p.ParamSlot, p.ParamSlotCount)
}

func (p CallEntryContextProjection) directFunctionRefs(callee FuncRef, call *ast.FuncCallExpr, in *flow.PointState) flow.FunctionRefs {
	if p.ParamSlot == nil || p.ParamPath == nil || in == nil {
		return flow.FunctionRefsDomain.Bottom()
	}
	return DirectCallEntryFunctionRefs(DirectCallEntryReferenceInput{
		Call:                   call,
		Callee:                 callee,
		ParamSlot:              p.ParamSlot,
		ParamPath:              p.ParamPath,
		ArgPath:                p.ArgPath,
		FunctionRefs:           in.FunctionRefs,
		State:                  in,
		ResolveFunctionArg:     p.FunctionArgRefs,
		ResolveFunctionArgRefs: p.FunctionArgRefTree,
	})
}

func (p CallEntryContextProjection) directClosureRefs(callee FuncRef, call *ast.FuncCallExpr, in *flow.PointState) flow.ClosureRefs {
	if p.ParamSlot == nil || p.ParamPath == nil || in == nil {
		return flow.ClosureRefsDomain.Bottom()
	}
	return DirectCallEntryClosureRefs(DirectCallEntryReferenceInput{
		Call:                  call,
		Callee:                callee,
		ParamSlot:             p.ParamSlot,
		ParamPath:             p.ParamPath,
		ArgPath:               p.ArgPath,
		ClosureRefs:           in.ClosureRefs,
		State:                 in,
		ResolveClosureArg:     p.ClosureArgRefs,
		ResolveClosureArgRefs: p.ClosureArgRefTree,
	})
}

func (p CallEntryContextProjection) callbackEntryKeys(point cfg.Point, info *cfg.CallInfo, in *flow.PointState) []Key {
	call := callInfoCall(info)
	if p.ResolveCallback == nil || call == nil || in == nil {
		return nil
	}
	var keys []Key
	for argIdx, arg := range call.Args {
		if arg == nil {
			continue
		}
		refs, ok := p.ResolveCallback(arg, callInfoArgSymbol(info, argIdx), in)
		if !ok || len(refs) == 0 {
			continue
		}
		var values EntryValues
		if p.ExpectedArgType != nil {
			if fn := unwrap.Function(p.ExpectedArgType(point, info, in, argIdx)); fn != nil {
				values, _ = callbackExpectedEntryValues(fn)
			}
		}
		var closures flow.ClosureRefSet
		var closureOK bool
		if p.ClosureArgRefs != nil {
			closures, closureOK = p.ClosureArgRefs(argIdx, arg, in)
		}
		for _, ref := range refs {
			emitted := false
			if closureOK {
				for _, closure := range closures.Refs() {
					if canonref.FromFlow(closure.Ref) != ref {
						continue
					}
					keys = append(keys, NewKeyWithEntryContext(
						ref,
						closure.EntryCells(),
						closure.EntryFunctionRefs(),
						closure.EntryClosureRefs(),
						values,
					))
					emitted = true
				}
			}
			if !emitted {
				keys = append(keys, NewKeyWithEntryValues(ref, flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), values))
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
		if info == nil || info.Call == nil {
			return
		}
		in, ok := p.callEventState(point)
		if !ok {
			return
		}
		for _, target := range p.ResolveTargets(info.Call, &in) {
			for runtimeIdx := 0; runtimeIdx < callsite.RuntimeArgExprCount(info.Call); runtimeIdx++ {
				arg := callsite.RuntimeArgExprAt(info.Call, runtimeIdx)
				if arg == nil {
					continue
				}
				sourceParam, slot, ok := p.ParamSlot(target.Ref, info.Call, runtimeIdx)
				if !ok || (p.ParamAnnotated != nil && p.ParamAnnotated(target.Ref, sourceParam)) {
					continue
				}
				av, ok := p.EvalArg(&in, arg)
				if !ok {
					continue
				}
				out = JoinCallEntryValue(out, target.Ref, slot, av)
			}
		}
		out = p.projectCallbackEntryValues(out, point, info, &in)
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p CallEntryValueProjection) projectCallbackEntryValues(out CallEntryValues, point cfg.Point, info *cfg.CallInfo, in *flow.PointState) CallEntryValues {
	call := callInfoCall(info)
	if p.ResolveCallback == nil || p.ExpectedArgType == nil || call == nil || in == nil {
		return out
	}
	for argIdx, arg := range call.Args {
		if arg == nil {
			continue
		}
		fn := unwrap.Function(p.ExpectedArgType(point, info, in, argIdx))
		if fn == nil {
			continue
		}
		refs, ok := p.ResolveCallback(arg, callInfoArgSymbol(info, argIdx), in)
		if !ok || len(refs) == 0 {
			continue
		}
		values, ok := callbackExpectedEntryValues(fn)
		if !ok || len(values) == 0 {
			continue
		}
		for _, ref := range refs {
			for slot, av := range values {
				out = JoinCallEntryValue(out, ref, slot, av)
			}
		}
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
		values = JoinEntryValue(values, slot, product.FromType(param.Type))
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

func (p CallEntryValueProjection) callEventState(point cfg.Point) (flow.PointState, bool) {
	return callEventState(p.State, point)
}

func (p CallEntryContextProjection) callEventState(point cfg.Point) (flow.PointState, bool) {
	return callEventState(p.State, point)
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

func sortFuncRefs(refs []FuncRef) {
	canonref.SortFuncRefs(refs)
}
