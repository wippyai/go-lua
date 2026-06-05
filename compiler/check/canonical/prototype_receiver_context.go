package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func entryValuePrototypeReceivers(receivers []metatable.MethodReceiver) []summary.EntryValuePrototypeReceiver {
	if len(receivers) == 0 {
		return nil
	}
	out := make([]summary.EntryValuePrototypeReceiver, 0, len(receivers))
	for _, receiver := range receivers {
		out = append(out, summary.EntryValuePrototypeReceiver{
			Prototype: receiver.PrototypeSym,
			Slot:      receiver.SelfSlot,
		})
	}
	return out
}

func (p *program) withPrototypeReceiverBaselines(
	ref summary.FuncRef,
	values summary.EntryValues,
	receivers []summary.EntryValuePrototypeReceiver,
	deps summary.EntryValueDependencies,
) summary.EntryValues {
	if p == nil || deps == nil || len(values) == 0 || len(receivers) == 0 {
		return values
	}
	baselines := p.prototypeReceiverBaselines(receivers, deps)
	if len(baselines) == 0 {
		return values
	}
	methodReceivers := p.facts.MethodReceivers(ref)
	if len(methodReceivers) == 0 {
		return values
	}
	var out summary.EntryValues
	for _, receiver := range methodReceivers {
		exact, hasExact := values[receiver.SelfSlot]
		baseline, hasBaseline := baselines[receiver.PrototypeSym]
		if !hasExact || !hasBaseline || exact.IsZero() || baseline.IsZero() {
			continue
		}
		composed := p.composePrototypeReceiverEntryValue(receiver.PrototypeSym, exact, baseline)
		if composed.IsZero() || product.Domain.Equal(composed, exact) {
			continue
		}
		if out == nil {
			out = make(summary.EntryValues, len(values))
			for slot, av := range values {
				out[slot] = av
			}
		}
		out[receiver.SelfSlot] = composed
	}
	if out == nil {
		return values
	}
	return out
}

func (p *program) prototypeReceiverBaselines(
	receivers []summary.EntryValuePrototypeReceiver,
	deps summary.EntryValueDependencies,
) map[cfg.SymbolID]product.AbstractValue {
	if p == nil || deps == nil || len(receivers) == 0 {
		return nil
	}
	var out map[cfg.SymbolID]product.AbstractValue
	for _, dep := range p.prototypePublisherRefs(receivers) {
		published := p.publishedPrototypes(dep)
		if len(published) == 0 {
			continue
		}
		self := deps.PrototypeSelf(dep)
		for _, proto := range published {
			av, ok := self.Value(proto)
			if !ok || av.IsZero() {
				continue
			}
			if out == nil {
				out = make(map[cfg.SymbolID]product.AbstractValue)
			}
			if prev, ok := out[proto]; ok {
				out[proto] = product.Domain.Join(prev, av)
			} else {
				out[proto] = av
			}
		}
	}
	return out
}

func (p *program) withPrototypeMethodSurfacesForRef(ref summary.FuncRef, values summary.EntryValues) summary.EntryValues {
	if p == nil || len(values) == 0 {
		return values
	}
	receivers := p.facts.MethodReceivers(ref)
	if len(receivers) == 0 {
		return values
	}
	out := make(summary.EntryValues, len(values))
	for slot, av := range values {
		out[slot] = av
	}
	for _, receiver := range receivers {
		av, ok := out[receiver.SelfSlot]
		if !ok || av.IsZero() {
			continue
		}
		out[receiver.SelfSlot] = p.withPrototypeMethodSurface(receiver.PrototypeSym, av)
	}
	return out
}

func (p *program) withPrototypeMethodSurfacesForMethodCall(ref summary.FuncRef, call *ast.FuncCallExpr, values summary.EntryValues) summary.EntryValues {
	if call == nil || call.Method == "" {
		return values
	}
	return p.withPrototypeMethodSurfacesForRef(ref, values)
}

func (p *program) withPrototypeMethodSurface(proto cfg.SymbolID, av product.AbstractValue) product.AbstractValue {
	if p == nil || p.driver == nil || proto == 0 || av.IsZero() {
		return av
	}
	meta, ok := p.prototypeMetatableValue(proto)
	if !ok || meta.IsZero() {
		return av
	}
	out, ok := product.WithMetatable(av, meta)
	if !ok || out.IsZero() {
		return av
	}
	return out
}

func (p *program) prototypeMetatableValue(proto cfg.SymbolID) (product.AbstractValue, bool) {
	if p == nil || p.driver == nil || proto == 0 {
		return product.AbstractValue{}, false
	}
	protoType, ok := p.prototypeSurfaceType(proto, typ.NewRecord().Build())
	if !ok || typ.IsAbsentOrUnknown(protoType) {
		return product.AbstractValue{}, false
	}
	meta := typ.NewRecord().Field("__index", protoType).Build()
	return product.FromType(meta), true
}

func (p *program) prototypeSurfaceType(proto cfg.SymbolID, base typ.Type) (typ.Type, bool) {
	if p == nil || p.driver == nil || proto == 0 {
		return base, false
	}
	surface := base
	hasMethodSurface := false
	for _, method := range p.facts.PrototypeMethods() {
		if method.PrototypeSym != proto || method.FuncRef == (flow.FunctionRef{}) || method.Field == (constraint.Segment{}) {
			continue
		}
		sig := p.driver.declaredSignatureForRef(p, canonref.FromFlow(method.FuncRef))
		if typ.IsAbsentOrUnknown(sig) {
			continue
		}
		switch method.Field.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			surface = typ.ExtendRecordWithField(surface, method.Field.Name, sig)
			hasMethodSurface = true
		}
	}
	if surface == nil {
		return typ.Unknown, hasMethodSurface
	}
	return surface, hasMethodSurface
}

func (p *program) MergeEntryValues(ref summary.FuncRef, fixed, fallback summary.EntryValues) summary.EntryValues {
	merged := summary.MergeEntryValuesWithFixed(fixed, fallback)
	if p == nil || len(fixed) == 0 || len(fallback) == 0 || len(merged) == 0 {
		return merged
	}
	receivers := p.facts.MethodReceivers(ref)
	if len(receivers) == 0 {
		return merged
	}
	var out summary.EntryValues
	for _, receiver := range receivers {
		exact, hasExact := fixed[receiver.SelfSlot]
		baseline, hasBaseline := fallback[receiver.SelfSlot]
		if !hasExact || !hasBaseline || exact.IsZero() || baseline.IsZero() {
			continue
		}
		composed := p.composePrototypeReceiverEntryValue(receiver.PrototypeSym, exact, baseline)
		if composed.IsZero() || product.Domain.Equal(composed, exact) {
			continue
		}
		if out == nil {
			out = make(summary.EntryValues, len(merged))
			for slot, av := range merged {
				out[slot] = av
			}
		}
		out[receiver.SelfSlot] = composed
	}
	if out == nil {
		return merged
	}
	return out
}

func (p *program) composePrototypeReceiverEntryValue(proto cfg.SymbolID, exact, baseline product.AbstractValue) product.AbstractValue {
	if p == nil || proto == 0 || exact.IsZero() || baseline.IsZero() {
		return product.AbstractValue{}
	}
	exactType := product.ProjectValueOrUnknown(exact)
	baselineType := product.ProjectValueOrUnknown(baseline)
	if typ.IsAny(exactType) || typ.IsUnknown(exactType) {
		return p.withPrototypeMethodSurface(proto, baseline)
	}
	if !p.receiverTypeCarriesPrototypeSurface(proto, exactType) && !receiverTypeSharesRequiredBaselineField(exactType, baselineType) {
		return exact
	}
	composed, ok := receiverInvariantOverlayType(exactType, baselineType)
	if !ok || typ.IsAbsentOrUnknown(composed) {
		return exact
	}
	return p.withPrototypeMethodSurface(proto, product.FromType(composed))
}

func receiverTypeSharesRequiredBaselineField(exact, baseline typ.Type) bool {
	exactNonNil, _ := value.SplitNilable(exact)
	baselineNonNil, _ := value.SplitNilable(baseline)
	exactRec, exactOK := receiverBaselineRecord(exactNonNil, 0)
	baselineRec, baselineOK := receiverBaselineRecord(baselineNonNil, 0)
	if !exactOK || !baselineOK {
		return false
	}
	for _, field := range baselineRec.Fields {
		if field.Optional {
			continue
		}
		if exactRec.GetField(field.Name) != nil {
			return true
		}
	}
	for _, member := range baselineRec.StaticMembers {
		if member.Optional {
			continue
		}
		if receiverStaticMember(exactRec, member) != nil {
			return true
		}
	}
	return false
}

func (p *program) receiverTypeCarriesPrototypeSurface(proto cfg.SymbolID, t typ.Type) bool {
	if p == nil || proto == 0 || !receiverTypeHasConcreteMetatable(t, 0) {
		return false
	}
	for _, method := range p.facts.PrototypeMethods() {
		if method.PrototypeSym != proto || method.FuncRef == (flow.FunctionRef{}) {
			continue
		}
		switch method.Field.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			if _, ok := core.Method(t, method.Field.Name); ok {
				return true
			}
		}
	}
	return false
}

func receiverTypeHasConcreteMetatable(t typ.Type, depth int) bool {
	if depth > 32 || t == nil {
		return false
	}
	t = unwrap.Alias(t)
	switch v := t.(type) {
	case *typ.Record:
		return v.Metatable != nil && !typ.IsMetatableUnconstrained(v.Metatable)
	case *typ.Optional:
		return receiverTypeHasConcreteMetatable(v.Inner, depth+1)
	case *typ.Union:
		seen := false
		for _, member := range v.Members {
			if unwrap.IsNilType(member) {
				continue
			}
			seen = true
			if !receiverTypeHasConcreteMetatable(member, depth+1) {
				return false
			}
		}
		return seen
	case *typ.Recursive:
		if v.Body == nil || v.Body == v {
			return false
		}
		return receiverTypeHasConcreteMetatable(v.Body, depth+1)
	case *typ.Generic:
		return receiverTypeHasConcreteMetatable(v.Body, depth+1)
	case *typ.Instantiated:
		resolved, err := core.ResolveInstantiated(v)
		if err != nil {
			return false
		}
		return receiverTypeHasConcreteMetatable(resolved, depth+1)
	default:
		return false
	}
}

func receiverInvariantOverlayType(exact, baseline typ.Type) (typ.Type, bool) {
	if exact == nil || baseline == nil {
		return exact, false
	}
	exactNonNil, exactNilable := value.SplitNilable(exact)
	baselineNonNil, _ := value.SplitNilable(baseline)
	if exactNonNil == nil || baselineNonNil == nil {
		return exact, false
	}
	overlaid, ok := receiverInvariantOverlayNonNil(exactNonNil, baselineNonNil, 0)
	if !ok || overlaid == nil {
		return exact, false
	}
	if exactNilable {
		overlaid = typ.NewOptional(overlaid)
	}
	return overlaid, true
}

func receiverInvariantOverlayNonNil(exact, baseline typ.Type, depth int) (typ.Type, bool) {
	if depth > 32 || exact == nil || baseline == nil {
		return exact, false
	}
	switch e := unwrap.Alias(exact).(type) {
	case *typ.Record:
		base, ok := receiverBaselineRecord(baseline, depth+1)
		if !ok {
			return exact, false
		}
		return overlayReceiverRecordInvariant(e, base), true
	case *typ.Optional:
		inner, ok := receiverInvariantOverlayNonNil(e.Inner, baseline, depth+1)
		if !ok {
			return exact, false
		}
		return typ.NewOptional(inner), true
	case *typ.Union:
		members := make([]typ.Type, 0, len(e.Members))
		changed := false
		for _, member := range e.Members {
			if unwrap.IsNilType(member) {
				members = append(members, member)
				continue
			}
			overlaid, ok := receiverInvariantOverlayNonNil(member, baseline, depth+1)
			if ok {
				members = append(members, overlaid)
				changed = true
			} else {
				members = append(members, member)
			}
		}
		if !changed {
			return exact, false
		}
		return typ.NewUnion(members...), true
	case *typ.Recursive:
		if e.Body == nil || e.Body == e {
			return exact, false
		}
		return receiverInvariantOverlayNonNil(e.Body, baseline, depth+1)
	case *typ.Generic:
		return receiverInvariantOverlayNonNil(e.Body, baseline, depth+1)
	case *typ.Instantiated:
		resolved, err := core.ResolveInstantiated(e)
		if err != nil {
			return exact, false
		}
		return receiverInvariantOverlayNonNil(resolved, baseline, depth+1)
	default:
		return exact, false
	}
}

func receiverBaselineRecord(t typ.Type, depth int) (*typ.Record, bool) {
	if depth > 32 || t == nil {
		return nil, false
	}
	switch b := unwrap.Alias(t).(type) {
	case *typ.Record:
		return b, true
	case *typ.Optional:
		return receiverBaselineRecord(b.Inner, depth+1)
	case *typ.Union:
		var joined typ.Type
		for _, member := range b.Members {
			if unwrap.IsNilType(member) {
				continue
			}
			rec, ok := receiverBaselineRecord(member, depth+1)
			if !ok {
				continue
			}
			if joined == nil {
				joined = rec
			} else {
				joined = typ.JoinReturnSlot(joined, rec)
			}
		}
		if joined == nil {
			return nil, false
		}
		return receiverBaselineRecord(joined, depth+1)
	case *typ.Recursive:
		if b.Body == nil || b.Body == b {
			return nil, false
		}
		return receiverBaselineRecord(b.Body, depth+1)
	case *typ.Generic:
		return receiverBaselineRecord(b.Body, depth+1)
	case *typ.Instantiated:
		resolved, err := core.ResolveInstantiated(b)
		if err != nil {
			return nil, false
		}
		return receiverBaselineRecord(resolved, depth+1)
	default:
		return nil, false
	}
}

func overlayReceiverRecordInvariant(exact, baseline *typ.Record) typ.Type {
	if exact == nil {
		return baseline
	}
	if baseline == nil {
		return exact
	}
	builder := typ.NewRecord().SetOpen(exact.Open || baseline.Open)
	for _, field := range exact.Fields {
		next := field
		if base := baseline.GetField(field.Name); base != nil && !base.Optional {
			next.Type = receiverInvariantSlotType(field.Type, base.Type)
			next.Optional = false
			next.Readonly = field.Readonly || base.Readonly
		}
		addRecordField(builder, next)
	}
	for _, field := range baseline.Fields {
		if field.Optional || exact.GetField(field.Name) != nil {
			continue
		}
		next := field
		next.Optional = false
		addRecordField(builder, next)
	}
	for _, member := range exact.StaticMembers {
		next := member
		if base := receiverStaticMember(baseline, member); base != nil && !base.Optional {
			next.Type = receiverInvariantSlotType(member.Type, base.Type)
			next.Optional = false
			next.Readonly = member.Readonly || base.Readonly
		}
		builder.AddStaticMember(next)
	}
	for _, member := range baseline.StaticMembers {
		if member.Optional || receiverStaticMember(exact, member) != nil {
			continue
		}
		next := member
		next.Optional = false
		builder.AddStaticMember(next)
	}
	if exact.HasMapComponent() {
		builder.MapComponent(exact.MapKey, exact.MapValue)
	} else if baseline.HasMapComponent() {
		builder.MapComponent(baseline.MapKey, baseline.MapValue)
	}
	if exact.Metatable != nil {
		builder.Metatable(exact.Metatable)
	} else if baseline.Metatable != nil {
		builder.Metatable(baseline.Metatable)
	}
	return builder.Build()
}

func addRecordField(builder *typ.RecordBuilder, field typ.Field) {
	switch {
	case field.Optional && field.Readonly:
		builder.OptReadonlyField(field.Name, field.Type)
	case field.Optional:
		builder.OptField(field.Name, field.Type)
	case field.Readonly:
		builder.ReadonlyField(field.Name, field.Type)
	default:
		builder.Field(field.Name, field.Type)
	}
}

func receiverStaticMember(rec *typ.Record, member typ.StaticMember) *typ.StaticMember {
	if rec == nil {
		return nil
	}
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return rec.GetStaticStringIndex(member.Name)
	case typ.StaticMemberIntIndex:
		return rec.GetStaticIntIndex(member.Index)
	default:
		return nil
	}
}

func receiverInvariantSlotType(exact, baseline typ.Type) typ.Type {
	if exact == nil {
		return baseline
	}
	if baseline == nil {
		return exact
	}
	if receiverCurrentSequenceCoversBaselineSeed(exact, baseline) {
		return exact
	}
	return value.MergeForConvergence(exact, baseline)
}

func receiverCurrentSequenceCoversBaselineSeed(exact, baseline typ.Type) bool {
	exactInner, _ := value.SplitNilable(exact)
	baselineInner, _ := value.SplitNilable(baseline)
	if exactInner == nil || baselineInner == nil {
		return false
	}
	exactArray, exactOK := unwrap.Alias(exactInner).(*typ.Array)
	baselineArray, baselineOK := unwrap.Alias(baselineInner).(*typ.Array)
	if !exactOK || !baselineOK || exactArray == nil || baselineArray == nil {
		return false
	}
	if !receiverSequenceSeed(baselineArray) || receiverSequenceSeed(exactArray) {
		return false
	}
	if typ.IsNever(exactArray.Element) {
		return false
	}
	return true
}

func receiverSequenceSeed(arr *typ.Array) bool {
	return arr != nil && (arr.Fresh || typ.IsNever(arr.Element))
}

func (p *program) normalizeCapturedMethodReceiverCells(
	g *cfg.Graph,
	in *flow.PointState,
	cells flow.CaptureCells,
	captured []cfg.SymbolID,
) flow.CaptureCells {
	if p == nil || g == nil || in == nil || cells.IsTop() || len(captured) == 0 {
		return cells
	}
	current, ok := p.refByGraph(g)
	if !ok {
		return cells
	}
	params := g.ParamSymbols()
	for _, receiver := range p.facts.MethodReceivers(current) {
		if receiver.PrototypeSym == 0 || receiver.SelfSlot < 0 || receiver.SelfSlot >= len(params) {
			continue
		}
		selfSym := params[receiver.SelfSlot]
		if selfSym == 0 || !symbolInList(captured, selfSym) {
			continue
		}
		exact, ok := cells.Value(selfSym)
		if !ok || exact.IsZero() {
			exact, ok = flow.SymbolValue(*in, selfSym)
		}
		if !ok || exact.IsZero() {
			continue
		}
		baseline, ok := in.PrototypeSelf.Value(receiver.PrototypeSym)
		if !ok || baseline.IsZero() {
			baseline = exact
		}
		composed := p.composePrototypeReceiverEntryValue(receiver.PrototypeSym, exact, baseline)
		if composed.IsZero() || product.Domain.Equal(composed, exact) {
			composed = p.withPrototypeMethodSurface(receiver.PrototypeSym, exact)
		}
		if composed.IsZero() {
			continue
		}
		cells = cells.With(selfSym, composed)
	}
	return cells
}

func (p *program) normalizeCapturedMethodReceiverCellsFromCells(
	g *cfg.Graph,
	cells flow.CaptureCells,
	captured []cfg.SymbolID,
) flow.CaptureCells {
	if p == nil || g == nil || cells.IsTop() || len(captured) == 0 {
		return cells
	}
	current, ok := p.refByGraph(g)
	if !ok {
		return cells
	}
	params := g.ParamSymbols()
	for _, receiver := range p.facts.MethodReceivers(current) {
		if receiver.PrototypeSym == 0 || receiver.SelfSlot < 0 || receiver.SelfSlot >= len(params) {
			continue
		}
		selfSym := params[receiver.SelfSlot]
		if selfSym == 0 || !symbolInList(captured, selfSym) {
			continue
		}
		exact, ok := cells.Value(selfSym)
		if !ok || exact.IsZero() {
			continue
		}
		composed := p.withPrototypeMethodSurface(receiver.PrototypeSym, exact)
		if composed.IsZero() || product.Domain.Equal(composed, exact) {
			continue
		}
		cells = cells.With(selfSym, composed)
	}
	return cells
}

func (p *program) withCapturedPrototypeReceiverSurface(owner summary.FuncRef, sym cfg.SymbolID, av product.AbstractValue) product.AbstractValue {
	if p == nil || sym == 0 || av.IsZero() {
		return av
	}
	g := p.Graph(owner)
	if g == nil {
		return av
	}
	params := g.ParamSymbols()
	for _, receiver := range p.facts.MethodReceivers(owner) {
		if receiver.PrototypeSym == 0 || receiver.SelfSlot < 0 || receiver.SelfSlot >= len(params) {
			continue
		}
		if params[receiver.SelfSlot] != sym {
			continue
		}
		withSurface := p.withPrototypeMethodSurface(receiver.PrototypeSym, av)
		if !withSurface.IsZero() {
			return withSurface
		}
	}
	return av
}

func symbolInList(symbols []cfg.SymbolID, want cfg.SymbolID) bool {
	for _, sym := range symbols {
		if sym == want {
			return true
		}
	}
	return false
}
