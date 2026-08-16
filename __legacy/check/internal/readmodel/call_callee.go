package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/callcontract"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func (r Reader) callCalleeReport(point cfg.Point, site factflow.CallSiteView) CallCalleeReport {
	if r.result == nil {
		return CallCalleeReport{}
	}
	memberAccess := site.CalleeMemberAccess()
	var receiverPath path.Path
	var member segment.Segment
	var hasMemberPath bool
	p := site.CalleePathRef()
	if p.IsEmpty() {
		if report, ok := r.expressionReceiverMethodCalleeReport(point, site); ok {
			return report
		}
		receiverPath, member, hasMemberPath = site.CalleeMemberAccessPath()
		if !hasMemberPath || receiverPath.IsEmpty() {
			return CallCalleeReport{}
		}
		p = receiverPath.Append(member)
	} else if memberAccess {
		receiverPath, member, hasMemberPath = site.CalleeMemberAccessPath()
	}
	if memberAccess && hasMemberPath {
		if receiverType, ok := r.memberReceiverNilableAtCall(point, site); ok {
			calleeType := receiverType
			callable := false
			nilableReceiver := true
			if site.MethodName() == "" {
				if memberType, ok := callcontract.MemberType(receiverType, member); ok {
					if callcontract.TypeCallableIgnoringNil(memberType) {
						callable = false
					} else {
						calleeType = memberType
						nilableReceiver = false
					}
				}
			}
			return readapi.PlanCallCalleeReport(readapi.CallCalleeReportPlan{
				CallableName:    r.callContractSourceName(site),
				Type:            calleeType,
				Callable:        callable,
				MemberAccess:    true,
				NilableReceiver: nilableReceiver,
				Span:            sourceSpanFromFactflow(site.CalleeSpan()),
				CallSpan:        sourceSpanFromFactflow(site.CallSpan()),
			})
		}
		if r.memberCalleePathProvenCallable(point, site, receiverPath, member) {
			return CallCalleeReport{}
		}
		if report, ok := r.missingMemberCalleeReport(point, site, receiverPath, member); ok {
			return report
		}
		if r.memberMissingOwnedByMemberProducer(point, receiverPath, member) {
			return CallCalleeReport{}
		}
		if r.memberCalleeCallableFromDiscriminantProof(point, site) {
			return CallCalleeReport{}
		}
		if r.memberCalleeCallableFromReceiver(point, site) {
			return CallCalleeReport{}
		}
	}
	// Resolve the runtime callee from the stabilized pre-call coordinate. A
	// call node's boundary output is post-transfer state; reading the callee
	// path from that output can resurrect the prepared declaration value after
	// an interprocedural effect has updated the same root immediately before
	// this call. CallCalleeValueAtBoundary owns the same point input and source
	// semantics as call execution, so diagnostics and execution observe one
	// canonical callee value.
	value, ok := r.result.CallCalleeValueAtBoundary(point, site)
	if !ok {
		return CallCalleeReport{}
	}
	t, ok := r.ValueTypeWithPresence(value)
	if !ok {
		return CallCalleeReport{}
	}
	if r.calleeValueProvenFunction(value) && !readapi.TypeIncludesNil(t) {
		return CallCalleeReport{}
	}
	if declared, ok := r.declaredCalleeType(p); ok {
		if readapi.CallCalleeDeclaredNilOwnedByDeclaration(t, declared) {
			return CallCalleeReport{}
		}
		if readapi.CallCalleeDeclaredTypeMoreInformative(t, declared) {
			t = declared
		}
	}
	if memberAccess && readapi.TypeIncludesNil(t) {
		if receiverType, ok := r.memberReceiverNilableAtCall(point, site); ok {
			t = receiverType
		} else {
			if callcontract.TypeCallableIgnoringNil(t) {
				return readapi.PlanCallCalleeReport(readapi.CallCalleeReportPlan{
					CallableName: r.callContractSourceName(site),
					Type:         t,
					Callable:     true,
					MemberAccess: true,
					Span:         sourceSpanFromFactflow(site.CalleeSpan()),
					CallSpan:     sourceSpanFromFactflow(site.CallSpan()),
				})
			}
			return CallCalleeReport{}
		}
	}
	callable := callcontract.TypeCallable(t)
	return readapi.PlanCallCalleeReport(readapi.CallCalleeReportPlan{
		CallableName:                 r.callContractSourceName(site),
		Type:                         t,
		Callable:                     callable,
		MemberAccess:                 memberAccess,
		ImpreciseMemberRequiresProof: r.impreciseMemberCalleeRequiresProof(point, site),
		Span:                         sourceSpanFromFactflow(site.CalleeSpan()),
		CallSpan:                     sourceSpanFromFactflow(site.CallSpan()),
	})
}

func (r Reader) calleeValueProvenFunction(value product.Value) bool {
	if r.result == nil || r.result.Registry() == nil {
		return false
	}
	kinds := product.Get(r.result.Registry(), value, runtimekind.Key)
	return !kinds.IsTop() && !kinds.IsBottom() &&
		kinds.Contains(runtimekind.Function) &&
		!kinds.Contains(runtimekind.Nil)
}

func (r Reader) impreciseMemberCalleeRequiresProof(point cfg.Point, site factflow.CallSiteView) bool {
	if !site.CalleeMemberAccess() {
		return false
	}
	receiver, ok := r.callReceiverType(point, site)
	return ok && receiver != nil && !typ.IsAny(receiver) && !typ.IsUnknown(receiver) && !typ.IsNever(receiver)
}

func (r Reader) memberCalleeCallableFromDiscriminantProof(point cfg.Point, site factflow.CallSiteView) bool {
	memberType, ok := r.discriminantProvenMemberType(point, site)
	return ok && callcontract.TypeCallable(memberType)
}

func (r Reader) memberCalleePathProvenCallable(point cfg.Point, site factflow.CallSiteView, receiverPath path.Path, member segment.Segment) bool {
	if r.result == nil {
		return false
	}
	calleePath := site.CalleePathRef()
	if calleePath.IsEmpty() {
		calleePath = receiverPath.Append(member)
	}
	if calleePath.IsEmpty() {
		return false
	}
	value, ok := r.result.PathValueAtBoundary(point, calleePath)
	if !ok {
		return false
	}
	t, ok := r.ValueTypeWithPresence(value)
	if !ok || readapi.TypeIncludesNil(t) {
		return false
	}
	if r.calleeValueProvenFunction(value) {
		return true
	}
	return callcontract.TypeCallable(t)
}

func (r Reader) memberCalleeCallableFromReceiver(point cfg.Point, site factflow.CallSiteView) bool {
	name, ok := memberCallableName(site)
	if !ok {
		return false
	}
	receiver, ok := r.callReceiverType(point, site)
	if !ok || receiver == nil || typevalue.TypeIncludesNil(receiver) {
		return false
	}
	fn, status, ok := callcontract.MemberCallable(receiver, name)
	return ok && status == callcontract.MemberCallOK && fn != nil
}

func (r Reader) missingMemberCalleeReport(point cfg.Point, site factflow.CallSiteView, receiverPath path.Path, member segment.Segment) (CallCalleeReport, bool) {
	if _, ok := r.discriminantProvenMemberType(point, site); ok {
		return CallCalleeReport{}, false
	}
	receiverType, ok := r.callReceiverType(point, site)
	if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return CallCalleeReport{}, false
	}
	if typevalue.TypeIncludesNil(receiverType) {
		return CallCalleeReport{}, false
	}
	_, status, ok := callcontract.MemberCall(receiverType, member)
	if !ok || status != callcontract.MemberCallMissing {
		return CallCalleeReport{}, false
	}
	if !r.reportMissingMemberShape(point, receiverPath, member, receiverType) {
		return CallCalleeReport{}, false
	}
	memberName := callCalleeMemberSegmentDisplay(member)
	if memberName == "" {
		return CallCalleeReport{}, false
	}
	name := r.callContractSourceName(site)
	if name == "" {
		name = "receiver"
	}
	return CallCalleeReport{
		Kind:         readapi.CallCalleeReportMissingMember,
		CallableName: name,
		Type:         receiverType,
		MemberAccess: true,
		MemberName:   memberName,
		Span:         sourceSpanFromFactflow(site.CallSpan()),
	}, true
}

func (r Reader) memberMissingOwnedByMemberProducer(point cfg.Point, receiverPath path.Path, member segment.Segment) bool {
	receiverType, ok := r.memberReceiverPathType(point, receiverPath)
	if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return false
	}
	if typevalue.TypeIncludesNil(receiverType) {
		return false
	}
	_, status, ok := callcontract.MemberCall(receiverType, member)
	return ok && status == callcontract.MemberCallMissing &&
		!r.reportMissingMemberShape(point, receiverPath, member, receiverType)
}

func (r Reader) reportMissingMemberShape(point cfg.Point, receiver path.Path, member segment.Segment, receiverType typ.Type) bool {
	if member.Kind == segment.SegmentIndexInt {
		return true
	}
	if body.TypeIsUnionReceiver(receiverType) {
		return true
	}
	if !body.TypeIsClosedConcreteRecord(receiverType) {
		return false
	}
	if r.result != nil && !receiver.IsEmpty() {
		if value, ok := r.result.PathValueAtBoundary(point, receiver); ok && r.ValueHasExactIdentity(value) {
			return false
		}
	}
	if receiver.Symbol == 0 || r.result == nil {
		return false
	}
	if r.result.SymbolHasTypeAnnotation(receiver.Symbol) {
		return true
	}
	kind, ok := r.result.SymbolKind(receiver.Symbol)
	return ok && (kind == symbol.Param || kind == symbol.Global)
}

func callCalleeMemberSegmentDisplay(member segment.Segment) string {
	switch member.Kind {
	case segment.SegmentField:
		return member.Name
	case segment.SegmentIndexString, segment.SegmentIndexInt:
		return segment.FormatSegments([]segment.Segment{member})
	default:
		return ""
	}
}

func (r Reader) expressionReceiverMethodCalleeReport(point cfg.Point, site factflow.CallSiteView) (CallCalleeReport, bool) {
	if !site.CalleeMemberAccess() || site.MethodName() == "" {
		return CallCalleeReport{}, false
	}
	if _, _, ok := site.CalleeMemberAccessPath(); ok {
		return CallCalleeReport{}, false
	}
	receiver, ok := r.callReceiverType(point, site)
	if !ok || receiver == nil || typ.IsAny(receiver) || typ.IsUnknown(receiver) || typ.IsNever(receiver) {
		return CallCalleeReport{}, false
	}
	if !typevalue.TypeIncludesNil(receiver) {
		return CallCalleeReport{}, false
	}
	return readapi.PlanCallCalleeReport(readapi.CallCalleeReportPlan{
		CallableName:    r.callContractSourceName(site),
		Type:            receiver,
		Callable:        false,
		MemberAccess:    true,
		NilableReceiver: true,
		Span:            sourceSpanFromFactflow(site.CalleeSpan()),
		CallSpan:        sourceSpanFromFactflow(site.CallSpan()),
	}), true
}

func (r Reader) memberReceiverNilableAtCall(point cfg.Point, site factflow.CallSiteView) (typ.Type, bool) {
	if site.MethodName() == "" && !site.CalleeMemberAccess() {
		return nil, false
	}
	receiver, ok := r.callReceiverType(point, site)
	if !ok || receiver == nil || typ.IsAny(receiver) || typ.IsUnknown(receiver) || typ.IsNever(receiver) {
		return nil, false
	}
	if !typevalue.TypeIncludesNil(receiver) {
		return nil, false
	}
	return receiver, true
}

func (r Reader) declaredCalleeType(p path.Path) (typ.Type, bool) {
	if p.Symbol == 0 || len(p.Segments) != 0 {
		return nil, false
	}
	return r.result.SymbolDeclaredType(p.Symbol)
}

func (r Reader) memberCallFunctionType(point cfg.Point, site factflow.CallSiteView) (*typ.Function, bool) {
	method, ok := memberCallableName(site)
	if !ok {
		return nil, false
	}
	if memberType, ok := r.discriminantProvenMemberType(point, site); ok {
		return callcontract.Callable(memberType)
	}
	receiver, ok := r.callReceiverType(point, site)
	if !ok {
		return nil, false
	}
	fn, status, ok := callcontract.MemberCallable(receiver, method)
	return fn, ok && status == callcontract.MemberCallOK && fn != nil
}

func (r Reader) discriminantProvenMemberType(point cfg.Point, site factflow.CallSiteView) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	receiver, member, ok := site.CalleeMemberAccessPath()
	if !ok || receiver.IsEmpty() {
		return nil, false
	}
	switch member.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return r.result.DiscriminantProvenMemberTypeBeforeBoundary(point, receiver, member.Name)
	default:
		return nil, false
	}
}

func memberCallableName(site factflow.CallSiteView) (string, bool) {
	if method := site.MethodName(); method != "" {
		return method, true
	}
	_, member, ok := site.CalleeMemberAccessPath()
	if !ok {
		return "", false
	}
	switch member.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return member.Name, member.Name != ""
	default:
		return "", false
	}
}

func (r Reader) callReceiverType(point cfg.Point, site factflow.CallSiteView) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	if p, ok := site.ReceiverPath(); ok && !p.IsEmpty() {
		return r.memberReceiverPathType(point, p)
	}
	if source, ok := site.ReceiverSource(); ok {
		if receiver, ok := r.receiverSourceType(point, source); ok && callcontract.ReceiverTypeUsable(receiver) {
			return receiver, true
		}
	}
	if p, _, ok := site.CalleeMemberAccessPath(); ok && !p.IsEmpty() {
		return r.memberReceiverPathType(point, p)
	}
	return nil, false
}

func (r Reader) memberReceiverPathType(point cfg.Point, p path.Path) (typ.Type, bool) {
	if receiver, ok := r.memberReceiverTypeFromValue(point, p, func() (product.Value, bool) {
		return r.result.PathValueBeforeBoundary(point, p)
	}); ok {
		return receiver, true
	}
	if receiver, ok := r.memberReceiverTypeFromValue(point, p, func() (product.Value, bool) {
		return r.result.PathValueAtBoundary(point, p)
	}); ok {
		return receiver, true
	}
	return r.declaredReceiverPathType(p)
}

func (r Reader) memberReceiverTypeFromValue(point cfg.Point, p path.Path, read func() (product.Value, bool)) (typ.Type, bool) {
	if read == nil {
		return nil, false
	}
	if value, ok := read(); ok {
		if receiver, ok := r.ValueTypeWithPresence(value); ok && callcontract.ReceiverTypeUsable(receiver) {
			if typevalue.TypeIncludesNil(receiver) && r.result.DominatingRequiredMemberReadProvesPathPresent(point, p) {
				receiver = body.TypeWithoutOptionalNil(receiver)
			}
			if declared, declaredOK := r.declaredReceiverPathType(p); declaredOK &&
				r.result.SymbolHasTypeAnnotation(p.Symbol) &&
				preferDeclaredReceiverType(receiver, declared) {
				return declared, true
			}
			return receiver, true
		}
	}
	return nil, false
}

func (r Reader) declaredReceiverPathType(p path.Path) (typ.Type, bool) {
	if p.Symbol == 0 {
		return nil, false
	}
	receiver, ok := r.result.SymbolDeclaredType(p.Symbol)
	if !ok {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return receiver, true
	}
	return luatypeprojection.ApplySegments(receiver, p.Segments)
}

func preferDeclaredReceiverType(current, declared typ.Type) bool {
	if current == nil || declared == nil {
		return false
	}
	if typ.IsAny(declared) || typ.IsUnknown(declared) {
		return true
	}
	currentContract := callcontract.ReceiverContractType(body.TypeWithoutOptionalNil(current))
	declaredContract := callcontract.ReceiverContractType(body.TypeWithoutOptionalNil(declared))
	return typevalue.TypeIncludesNil(current) &&
		!typevalue.TypeIncludesNil(declared) &&
		callcontract.ReceiverTypeUsable(currentContract) &&
		callcontract.ReceiverTypeUsable(declaredContract)
}

func (r Reader) receiverSourceType(point cfg.Point, source factflow.ValueSource) (typ.Type, bool) {
	value, ok := r.callArgumentValue(point, source)
	if !ok {
		return nil, false
	}
	return r.ValueTypeWithPresence(value)
}
