package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/memberaccess"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

func outcomeFromSummary(
	reg *axis.Registry,
	got summary.Summary,
	paramOffset int,
	usefulParamObligation func(int) bool,
	usefulNormalReturnParam func(int) bool,
) callpayload.CallOutcome {
	out := callpayload.CallOutcome{
		SuspensionKnown: true,
		MaySuspend:      got.MaySuspend,
	}
	if len(got.Returns) != 0 {
		out.Results = make([]callpayload.CallResult, len(got.Returns))
		for i, value := range got.Returns {
			out.Results[i] = callpayload.CallResult{Index: i, Value: value}
		}
	}
	for i, value := range got.ParamObligations {
		if usefulParamObligation == nil || !usefulParamObligation(i) {
			continue
		}
		paramIndex := i - paramOffset
		if paramIndex < 0 {
			continue
		}
		out.ParamObligations = append(out.ParamObligations, callpayload.CallParamObligation{
			ParamIndex: paramIndex,
			Value:      value,
		})
	}
	for _, obligation := range got.CapturedPathObligations {
		if !summary.UsefulParamObligation(reg, obligation.Value) {
			continue
		}
		stable, ok := pathaddr.StableFromKey(obligation.Path.PathKey())
		if !ok {
			continue
		}
		path, ok := stable.Path()
		if !ok {
			continue
		}
		out.PathObligations = append(out.PathObligations, callpayload.CallPathObligation{
			Path:  path,
			Value: obligation.Value,
		})
	}
	for i, value := range got.NormalReturnParams {
		if usefulNormalReturnParam == nil || !usefulNormalReturnParam(i) {
			continue
		}
		value = normalReturnParamCallRefinement(reg, value)
		out.ParamPathRefinements = append(out.ParamPathRefinements, callpayload.CallParamPathRefinement{
			Path:  pathdom.NewPlaceholder(i),
			Value: value,
		})
	}
	for i, condition := range got.NormalReturnParamConditions {
		value, ok := paramConditionValue(condition)
		if !ok {
			continue
		}
		out.ParamConditions = append(out.ParamConditions, callpayload.CallParamCondition{
			ParamIndex: i,
			Value:      value,
		})
	}
	for _, equality := range got.NormalReturnParamEqualities {
		if equality.Left < 0 || equality.Right < 0 || equality.Left == equality.Right {
			continue
		}
		out.ParamPathRelations = append(out.ParamPathRelations, callpayload.CallParamPathRelation{
			Kind:  callpayload.CallPathRelationEqual,
			Left:  pathdom.NewPlaceholder(equality.Left),
			Right: pathdom.NewPlaceholder(equality.Right),
		})
	}
	out.NormalReturnFacts = summary.CloneNormalReturnFacts(got.NormalReturnFacts)
	// outcomeFromSummary only receives caller-owned summaries. Passing the heap
	// map through avoids cloning the snapshot-read copy again.
	out.HeapTableObjects = got.HeapTableObjects
	out.Placements = summary.CallerFreshHeapPlacements(got.FreshHeapAllocations)
	if len(got.ReturnConditionParamRefinements) != 0 {
		out.ReturnConditionRefinements = make([]callpayload.CallReturnConditionRefinement, len(got.ReturnConditionParamRefinements))
		for i, refinement := range got.ReturnConditionParamRefinements {
			out.ReturnConditionRefinements[i] = callpayload.CallReturnConditionRefinement{
				ReturnIndex: refinement.ReturnIndex,
				ReturnValue: refinement.ReturnValue,
				Target:      refinement.Target.Clone(),
				Value:       refinement.Value,
			}
		}
	}
	if len(got.ReturnConditionSlotRefinements) != 0 {
		out.ReturnConditionSlots = make([]callpayload.CallReturnConditionSlotRefinement, len(got.ReturnConditionSlotRefinements))
		for i, refinement := range got.ReturnConditionSlotRefinements {
			out.ReturnConditionSlots[i] = callpayload.CallReturnConditionSlotRefinement{
				ReturnIndex: refinement.ReturnIndex,
				ReturnValue: refinement.ReturnValue,
				TargetIndex: refinement.TargetIndex,
				Value:       refinement.Value,
			}
		}
	}
	if len(got.ReturnPresenceRelations) != 0 {
		out.ReturnPresenceRelations = make([]callpayload.CallReturnPresenceRelation, len(got.ReturnPresenceRelations))
		for i, relation := range got.ReturnPresenceRelations {
			out.ReturnPresenceRelations[i] = callpayload.CallReturnPresenceRelation{
				TriggerIndex:    relation.TriggerIndex,
				TriggerPresence: relation.TriggerPresence,
				TargetIndex:     relation.TargetIndex,
				TargetPresence:  relation.TargetPresence,
			}
		}
	}
	return out
}

func normalReturnParamCallRefinement(reg *axis.Registry, value product.Value) product.Value {
	if reg == nil {
		return value
	}
	p := product.PresenceOf(value)
	if !presence.Equal(p, presence.Present()) && !presence.Equal(p, presence.Absent()) {
		return value
	}
	t, ok := typevalue.TypeOf(reg, value)
	if ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) {
		return value
	}
	kind := product.Get(reg, value, runtimekind.Key)
	if !kind.IsTop() && !kind.IsBottom() {
		return value
	}
	return product.NewWithPresence(reg, product.ShapeTop, p)
}

func paramMemberReturnPresenceRelations(
	ctx transfer.NodeContext,
	ks *keyspace.KeySpace,
	site factflow.CallSiteView,
	got summary.Summary,
	facts factflow.Facts,
	returnPresenceRelations ReturnPresenceRelationsForPathFunc,
) []callpayload.CallReturnPresenceRelation {
	if returnPresenceRelations == nil || len(got.ParamMemberReturnSlots) == 0 {
		return nil
	}
	type memberKey struct {
		receiver int
		member   segment.Segment
	}
	argCount := site.ArgumentSourceCount()
	slotsByMember := make(map[memberKey]map[int]int)
	for _, slot := range got.ParamMemberReturnSlots {
		if slot.ReceiverParam < 0 || slot.ReceiverParam >= argCount ||
			!memberaccess.Valid(slot.Member) || slot.ReturnIndex < 0 || slot.MemberResultIndex < 0 {
			continue
		}
		key := memberKey{receiver: slot.ReceiverParam, member: slot.Member}
		slots := slotsByMember[key]
		if slots == nil {
			slots = make(map[int]int)
			slotsByMember[key] = slots
		}
		slots[slot.MemberResultIndex] = slot.ReturnIndex
	}
	if len(slotsByMember) == 0 {
		return nil
	}
	var out []callpayload.CallReturnPresenceRelation
	for key, slots := range slotsByMember {
		source, ok := site.ArgumentSourceAt(key.receiver)
		if !ok {
			continue
		}
		memberPaths := argumentMemberPaths(ks, facts, source, key.member)
		if len(memberPaths) == 0 {
			continue
		}
		for _, memberPath := range memberPaths {
			for _, relation := range returnPresenceRelations(ctx.Point, memberPath) {
				trigger, ok := slots[relation.TriggerIndex]
				if !ok {
					continue
				}
				target, ok := slots[relation.TargetIndex]
				if !ok {
					continue
				}
				out = append(out, callpayload.CallReturnPresenceRelation{
					TriggerIndex:    trigger,
					TriggerPresence: relation.TriggerPresence,
					TargetIndex:     target,
					TargetPresence:  relation.TargetPresence,
				})
			}
		}
	}
	return out
}

func argumentMemberPaths(ks *keyspace.KeySpace, facts factflow.Facts, source factflow.ValueSource, member segment.Segment) []pathdom.Path {
	if !memberaccess.Valid(member) {
		return nil
	}
	p, ok := argumentSourcePath(ks, facts, source)
	if !ok || p.IsEmpty() {
		return nil
	}
	return memberaccess.Paths(p, member)
}

func argumentSourcePath(ks *keyspace.KeySpace, facts factflow.Facts, source factflow.ValueSource) (pathdom.Path, bool) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		return facts.ExpressionPathRef(source.ExprRef)
	}
	if source.Kind != factflow.ValueSourcePath || source.PathKey == "" || ks == nil {
		return pathdom.Path{}, false
	}
	key, ok := ks.FromStateKey(source.PathKey)
	if !ok || key.Sym == 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{
		Symbol:   key.Sym,
		Segments: ks.Segments(key),
	}, true
}

func memberCallParamObligations(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	got summary.Summary,
	fn *typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
	typeValues *typevalue.Cache,
) []callpayload.CallParamObligation {
	if ctx.Registry == nil || sources == nil || len(got.ParamMemberCallObligations) == 0 {
		return nil
	}
	argCount := site.ArgumentSourceCount()
	if argCount == 0 {
		return nil
	}
	var out []callpayload.CallParamObligation
	for _, obligation := range got.ParamMemberCallObligations {
		if obligation.ReceiverParam < 0 || obligation.ReceiverParam >= argCount ||
			obligation.ArgParam < 0 || obligation.ArgParam >= argCount ||
			obligation.MemberParamIndex < 0 || !memberaccess.Valid(obligation.Member) {
			continue
		}
		receiverSource, ok := site.ArgumentSourceAt(obligation.ReceiverParam)
		if !ok {
			continue
		}
		receiverValue, ok := sources.ValueOfSource(ctx.Point, receiverSource, in, read)
		if !ok {
			continue
		}
		receiverType, ok := typeFromValue(ctx.Registry, receiverValue)
		if !ok {
			continue
		}
		receiverType, ok = projectMemberObligationReceiver(receiverType, obligation.ReceiverPath)
		if !ok {
			continue
		}
		callable, status, ok := memberaccess.Callable(receiverType, obligation.Member)
		if status != typecall.MemberCallOK || !ok || callable == nil || obligation.MemberParamIndex >= len(callable.Params) {
			continue
		}
		want := callable.Params[obligation.MemberParamIndex].Type
		if want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		value := typeWitnessValue(ctx.Registry, typeValues, want)
		if !summary.UsefulParamObligation(ctx.Registry, value) {
			continue
		}
		out = append(out, callpayload.CallParamObligation{
			ParamIndex: obligation.ArgParam,
			Value:      value,
			Origin: callpayload.CallParamObligationOrigin{
				HasOrigin:        true,
				ReceiverParam:    obligation.ReceiverParam,
				ReceiverPath:     obligation.ReceiverPath,
				Member:           obligation.Member,
				ArgParam:         obligation.ArgParam,
				MemberParamIndex: obligation.MemberParamIndex,
				SubjectLabel:     memberCallParamSubjectLabel(fn, obligation),
				ProviderLabel:    memberCallParamProviderLabel(fn, obligation),
			},
		})
	}
	return out
}

func memberCallParamObligationOriginsFromSummary(reg *axis.Registry, got summary.Summary, fn *typ.Function) []callpayload.CallParamObligation {
	if reg == nil || len(got.ParamMemberCallObligations) == 0 || len(got.ParamObligations) == 0 {
		return nil
	}
	var out []callpayload.CallParamObligation
	for _, obligation := range got.ParamMemberCallObligations {
		if obligation.ArgParam < 0 || obligation.ArgParam >= len(got.ParamObligations) ||
			obligation.MemberParamIndex < 0 || !memberaccess.Valid(obligation.Member) {
			continue
		}
		value := got.ParamObligations[obligation.ArgParam]
		if !summary.UsefulParamObligation(reg, value) {
			continue
		}
		out = append(out, callpayload.CallParamObligation{
			ParamIndex: obligation.ArgParam,
			Value:      value,
			Origin: callpayload.CallParamObligationOrigin{
				HasOrigin:        true,
				ReceiverParam:    obligation.ReceiverParam,
				ReceiverPath:     obligation.ReceiverPath,
				Member:           obligation.Member,
				ArgParam:         obligation.ArgParam,
				MemberParamIndex: obligation.MemberParamIndex,
				SubjectLabel:     memberCallParamSubjectLabel(fn, obligation),
				ProviderLabel:    memberCallParamProviderLabel(fn, obligation),
			},
		})
	}
	return out
}

func memberCallParamSubjectLabel(fn *typ.Function, obligation summary.ParamMemberCallObligation) string {
	if obligation.SubjectLabel != "" {
		return obligation.SubjectLabel
	}
	name := functionParamName(fn, obligation.ArgParam)
	if name == "" {
		return ""
	}
	return "argument " + nonNegativeDecimal(obligation.ArgParam+1) + " (" + name + ")"
}

func memberCallParamProviderLabel(fn *typ.Function, obligation summary.ParamMemberCallObligation) string {
	if obligation.ProviderLabel != "" {
		return obligation.ProviderLabel
	}
	root := functionParamName(fn, obligation.ReceiverParam)
	if root == "" {
		root = "argument " + nonNegativeDecimal(obligation.ReceiverParam+1)
	}
	var segs []segment.Segment
	if obligation.ReceiverPath != "" {
		var ok bool
		segs, ok = pathaddr.RelativeStaticMemberSuffixSegments(obligation.ReceiverPath)
		if !ok {
			return ""
		}
	}
	segs = append(segs, obligation.Member)
	return root + segment.FormatSegments(segs)
}

func nonNegativeDecimal(n int) string {
	if n <= 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func functionParamName(fn *typ.Function, index int) string {
	if fn == nil || index < 0 || index >= len(fn.Params) {
		return ""
	}
	return fn.Params[index].Name
}

func projectMemberObligationReceiver(receiver typ.Type, receiverPath pathaddr.SuffixKey) (typ.Type, bool) {
	if receiver == nil {
		return nil, false
	}
	if receiverPath == "" {
		return receiver, true
	}
	segments, ok := pathaddr.RelativeStaticMemberSuffixSegments(receiverPath)
	if !ok {
		return nil, false
	}
	return luatypeprojection.ApplySegments(receiver, segments)
}

// paramReturnExposures lowers return-param aliasing into covariant call-boundary
// exposures. For each ReturnParamPathAlias the callee records (a parameter stored
// into a returned container slot), it emits an exposure whose contract is the
// callee's declared return type projected at the aliased return slot member. That
// projected member type, not the parameter's own declared type, is the sound
// contract: a callee that covariantly stores a narrow parameter into a wider
// return field exposes the argument object at the wider field type, so a write
// through the caller's returned view can launder a wider value back into the
// argument. The caller eager-widens the argument's source object toward this
// contract, mirroring the in-body covariant mutable-view exposure.
func paramReturnExposures(reg *axis.Registry, typeValues *typevalue.Cache, argCount int, got summary.Summary, fn *typ.Function) []callpayload.CallParamExposure {
	if reg == nil || fn == nil || len(got.ReturnParamPathAliases) == 0 {
		return nil
	}
	returns := callResultReturnTypes(got, fn.Returns)
	var out []callpayload.CallParamExposure
	for _, alias := range got.ReturnParamPathAliases {
		paramIndex, ok := alias.Source.RootPlaceholderIndex()
		if !ok || paramIndex < 0 || paramIndex >= argCount {
			continue
		}
		if alias.ReturnIndex < 0 || alias.ReturnIndex >= len(returns) {
			continue
		}
		returnType := returns[alias.ReturnIndex]
		if returnType == nil {
			continue
		}
		contract := returnType
		if alias.Member != "" {
			memberSegments, ok := pathaddr.RelativeStaticMemberSuffixSegments(alias.Member)
			if !ok || len(memberSegments) == 0 {
				continue
			}
			contract, ok = luatypeprojection.ApplySegments(returnType, memberSegments)
			if !ok {
				continue
			}
		}
		exposure, ok := newParamExposure(reg, typeValues, pathdom.NewPlaceholder(paramIndex), contract)
		if !ok {
			continue
		}
		out = append(out, exposure)
	}
	return out
}

// paramStoreRelationExposures lowers param-to-param store relations into covariant
// call-boundary exposures (Route 1). Each StoreRelationFact records that the
// callee stores one parameter (Source, a bare placeholder) into a member slot of
// another parameter (Into, a placeholder with member segments). The destination
// slot type, not the source parameter's own declared type, is the sound contract:
// a callee that covariantly stores a narrow parameter into a wider destination
// slot exposes the argument object at the wider slot type, so a write through the
// caller's destination view can launder a wider value back into the source
// argument. The contract is the destination parameter's declared type projected at
// the store member, and the caller eager-widens the source argument toward it.
func paramStoreRelationExposures(reg *axis.Registry, typeValues *typevalue.Cache, argCount int, got summary.Summary, fn *typ.Function) []callpayload.CallParamExposure {
	if reg == nil || fn == nil || len(got.NormalReturnFacts.StoreRelations) == 0 {
		return nil
	}
	var out []callpayload.CallParamExposure
	for _, relation := range got.NormalReturnFacts.StoreRelations {
		if !relation.Source.IsPlaceholder() || len(relation.Source.Segments) != 0 {
			continue
		}
		sourceIndex := relation.Source.PlaceholderIndex()
		if sourceIndex < 0 || sourceIndex >= argCount {
			continue
		}
		if !relation.Into.IsPlaceholder() || len(relation.Into.Segments) == 0 {
			continue
		}
		intoIndex := relation.Into.PlaceholderIndex()
		if intoIndex < 0 || intoIndex >= len(fn.Params) {
			continue
		}
		destType := fn.Params[intoIndex].Type
		if destType == nil {
			continue
		}
		contract, ok := luatypeprojection.ApplySegments(destType, relation.Into.Segments)
		if !ok {
			continue
		}
		exposure, ok := newParamExposure(reg, typeValues, pathdom.NewPlaceholder(sourceIndex), contract)
		if !ok {
			continue
		}
		out = append(out, exposure)
	}
	return out
}

// paramDirectMutationExposures lowers direct callee-side writes through a
// parameter path into covariant call-boundary exposures. A normal-return
// invalidation below $N proves the callee may have mutated the caller's argument
// through its declared parameter view, so the caller must widen that argument to
// the callee's parameter contract before later narrow reads.
func paramDirectMutationExposures(reg *axis.Registry, typeValues *typevalue.Cache, argCount int, got summary.Summary, fn *typ.Function) []callpayload.CallParamExposure {
	if reg == nil || fn == nil || len(got.NormalReturnFacts.PathInvalidations) == 0 {
		return nil
	}
	var out []callpayload.CallParamExposure
	for _, invalidation := range got.NormalReturnFacts.PathInvalidations {
		if !invalidation.Path.IsPlaceholder() || len(invalidation.Path.Segments) == 0 {
			continue
		}
		paramIndex := invalidation.Path.PlaceholderIndex()
		if paramIndex < 0 || paramIndex >= argCount || paramIndex >= len(fn.Params) {
			continue
		}
		contract := fn.Params[paramIndex].Type
		if contract == nil {
			continue
		}
		exposure, ok := newParamExposure(reg, typeValues, pathdom.NewPlaceholder(paramIndex), contract)
		if !ok {
			continue
		}
		out = append(out, exposure)
	}
	return out
}

// paramSinkExposures lowers param-to-sink store exposures into covariant
// call-boundary exposures (Route 2). Each ParamSinkExposure records that the
// callee stores one parameter (Source, a bare placeholder) into a member slot of a
// persistent sink (a captured upvalue or a global) the caller cannot track writes
// back through. The carried Contract is the sink's slot type, computed in-body
// where the sink's container type is available: it is the real exposure type,
// since a covariant store of a narrow parameter into a wider sink slot is
// well-typed and a later write through the sink launders a wider value back into
// the argument. The caller eager-widens the source argument toward the carried
// slot type.
func paramSinkExposures(reg *axis.Registry, typeValues *typevalue.Cache, argCount int, got summary.Summary) []callpayload.CallParamExposure {
	if reg == nil || len(got.ParamSinkExposures) == 0 {
		return nil
	}
	var out []callpayload.CallParamExposure
	for _, sink := range got.ParamSinkExposures {
		paramIndex, ok := sink.Source.PlaceholderIndex()
		if !ok || paramIndex < 0 || paramIndex >= argCount {
			continue
		}
		contract, ok := typevalue.TypeOf(reg, sink.Contract)
		if !ok {
			continue
		}
		exposure, ok := newParamExposure(reg, typeValues, pathdom.NewPlaceholder(paramIndex), contract)
		if !ok {
			continue
		}
		out = append(out, exposure)
	}
	return out
}

// newParamExposure builds a unified call-boundary exposure for a callee-relative
// source placeholder toward a destination-slot contract type. It gates on a
// mutable record/array contract and carries the contract's witness-bearing value.
func newParamExposure(reg *axis.Registry, typeValues *typevalue.Cache, source pathdom.Path, contract typ.Type) (callpayload.CallParamExposure, bool) {
	if contract == nil || typ.IsAny(contract) || typ.IsUnknown(contract) || refinement.ContainsFreeTypeParam(contract) {
		return callpayload.CallParamExposure{}, false
	}
	kind, ok := covariantExposureKind(contract)
	if !ok {
		return callpayload.CallParamExposure{}, false
	}
	value := typeWitnessValue(reg, typeValues, contract)
	return callpayload.CallParamExposure{
		Source:   source,
		Contract: value,
		Kind:     kind,
	}, true
}

// covariantExposureKind selects the widening kind for a mutable container
// contract: an opaque-array element widen for an array, a record field rebuild
// for a record. Any other shape is not a mutable container view and emits no
// exposure. The lowering twin transferfacts.covariantExposureKind must classify
// identically; the layered architecture keeps factflow type-independent and this
// package's imports bounded, so the two cannot share one helper.
func covariantExposureKind(contract typ.Type) (factflow.CovariantExposureKind, bool) {
	switch unaliasType(contract).(type) {
	case *typ.Array:
		return factflow.CovariantExposureArray, true
	case *typ.Record:
		return factflow.CovariantExposureRecord, true
	default:
		return 0, false
	}
}

func unaliasType(t typ.Type) typ.Type {
	for {
		alias, ok := t.(*typ.Alias)
		if !ok || alias == nil {
			return t
		}
		t = alias.UnaliasedTarget()
	}
}

func functionTypeParamObligationsForSite(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	site factflow.CallSiteView,
	fn *typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) []callpayload.CallParamObligation {
	if ctx.Registry == nil || fn == nil || len(fn.Params) == 0 {
		return nil
	}
	paramOffset := 0
	if site.MethodName() != "" && callableConsumesMethodReceiver(ctx, site, fn, sources, in, read, nil) {
		paramOffset = 1
	}
	return functionTypeParamObligationsFrom(ctx.Registry, typeValues, site.ArgumentSourceCount(), fn, paramOffset)
}

func methodReceiverType(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (typ.Type, bool) {
	if ctx.Registry == nil {
		return nil, false
	}
	method := site.MethodName()
	var rootType typ.Type
	var hasRootType bool
	if receiverPath, ok := site.ReceiverPath(); ok && receiverPath.Symbol != 0 && len(receiverPath.Segments) == 0 {
		value := in.ReadSymbolValue(ctx.Registry, receiverPath.Symbol)
		if t, ok := typeFromValue(ctx.Registry, value); ok {
			rootType, hasRootType = t, true
			if receiverTypeHasCallableMember(t, method) {
				return t, true
			}
		}
	}
	if sources != nil {
		source, ok := site.ReceiverSource()
		if ok {
			value, ok := sources.ValueOfSource(ctx.Point, source, in, read)
			if ok {
				if t, ok := typeFromValue(ctx.Registry, value); ok {
					return t, true
				}
			}
		}
	}
	if hasRootType {
		return rootType, true
	}
	return nil, false
}

func receiverTypeHasCallableMember(receiverType typ.Type, method string) bool {
	if receiverType == nil || method == "" {
		return false
	}
	fn, status, ok := typecall.MemberCallable(receiverType, method)
	return ok && status == typecall.MemberCallOK && fn != nil
}

func functionTypeParamObligationsFrom(reg *axis.Registry, typeValues *typevalue.Cache, argCount int, fn *typ.Function, paramOffset int) []callpayload.CallParamObligation {
	if reg == nil || fn == nil || len(fn.Params) == 0 || paramOffset >= len(fn.Params) {
		return nil
	}
	limit := argCount
	if limit > len(fn.Params)-paramOffset {
		limit = len(fn.Params) - paramOffset
	}
	var out []callpayload.CallParamObligation
	for i := 0; i < limit; i++ {
		want := fn.Params[i+paramOffset].Type
		if want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		value := typeWitnessValue(reg, typeValues, want)
		if !summary.UsefulParamObligation(reg, value) {
			continue
		}
		out = append(out, callpayload.CallParamObligation{
			ParamIndex:       i,
			Value:            value,
			SignatureSurface: true,
		})
	}
	return out
}

func paramConditionValue(condition summary.ParamCondition) (bool, bool) {
	switch condition {
	case summary.ParamConditionTruthy:
		return true, true
	case summary.ParamConditionFalsy:
		return false, true
	default:
		return false, false
	}
}
