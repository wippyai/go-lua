package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/callcontract"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (r Reader) callCalleeReport(point cfg.Point, site factflow.CallSite) CallCalleeReport {
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
		if report, ok := r.missingMemberCalleeReport(point, site, receiverPath, member); ok {
			return report
		}
		if r.memberCalleeCallableFromReceiver(point, site) {
			return CallCalleeReport{}
		}
	}
	value, ok := r.result.PathValueAtBoundary(point, p)
	if !ok {
		return CallCalleeReport{}
	}
	t, ok := r.ValueTypeWithPresence(value)
	if !ok {
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
			if calleeTypeCallableIgnoringNil(t) {
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
	callable := calleeTypeCallable(t)
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

func (r Reader) impreciseMemberCalleeRequiresProof(point cfg.Point, site factflow.CallSite) bool {
	if !site.CalleeMemberAccess() {
		return false
	}
	receiver, ok := r.callReceiverType(point, site)
	return ok && receiver != nil && !typ.IsAny(receiver) && !typ.IsUnknown(receiver) && !typ.IsNever(receiver)
}

func (r Reader) memberCalleeCallableFromReceiver(point cfg.Point, site factflow.CallSite) bool {
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

func (r Reader) missingMemberCalleeReport(point cfg.Point, site factflow.CallSite, receiverPath path.Path, member segment.Segment) (CallCalleeReport, bool) {
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

func (r Reader) expressionReceiverMethodCalleeReport(point cfg.Point, site factflow.CallSite) (CallCalleeReport, bool) {
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
		CallableName: r.callContractSourceName(site),
		Type:         receiver,
		Callable:     false,
		MemberAccess: true,
		Span:         sourceSpanFromFactflow(site.CalleeSpan()),
		CallSpan:     sourceSpanFromFactflow(site.CallSpan()),
	}), true
}

func (r Reader) memberReceiverNilableAtCall(point cfg.Point, site factflow.CallSite) (typ.Type, bool) {
	if site.MethodName() == "" {
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

func calleeTypeCallable(t typ.Type) bool {
	if t == nil || readapi.ObligationTypeIsGradual(t) || typ.IsNever(t) {
		return false
	}
	if _, ok := callcontract.Callable(t); ok {
		return true
	}
	if union, ok := t.(*typ.Union); ok && len(union.Members) != 0 {
		for _, member := range union.Members {
			if !calleeTypeCallable(member) {
				return false
			}
		}
		return true
	}
	return false
}

func calleeTypeCallableIgnoringNil(t typ.Type) bool {
	return calleeTypeCallable(body.TypeWithoutOptionalNil(t))
}

func (r Reader) declaredCalleeType(p path.Path) (typ.Type, bool) {
	if p.Symbol == 0 || len(p.Segments) != 0 {
		return nil, false
	}
	return r.result.SymbolDeclaredType(p.Symbol)
}

func (r Reader) memberCallFunctionType(point cfg.Point, site factflow.CallSite) (*typ.Function, bool) {
	method, ok := memberCallableName(site)
	if !ok {
		return nil, false
	}
	receiver, ok := r.callReceiverType(point, site)
	if !ok {
		return nil, false
	}
	fn, status, ok := callcontract.MemberCallable(receiver, method)
	return fn, ok && status == callcontract.MemberCallOK && fn != nil
}

func memberCallableName(site factflow.CallSite) (string, bool) {
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

func (r Reader) callReceiverType(point cfg.Point, site factflow.CallSite) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	if source, ok := site.ReceiverSource(); ok {
		if receiver, ok := r.receiverSourceType(point, source); ok && callcontract.ReceiverTypeUsable(receiver) {
			return receiver, true
		}
	}
	if p, ok := site.ReceiverPath(); ok && !p.IsEmpty() {
		return r.memberReceiverPathType(point, p)
	}
	if p, _, ok := site.CalleeMemberAccessPath(); ok && !p.IsEmpty() {
		return r.memberReceiverPathType(point, p)
	}
	return nil, false
}

func (r Reader) memberReceiverPathType(point cfg.Point, p path.Path) (typ.Type, bool) {
	if value, ok := r.result.PathValueAtBoundary(point, p); ok {
		if receiver, ok := r.ValueTypeWithPresence(value); ok && callcontract.ReceiverTypeUsable(receiver) {
			return receiver, true
		}
	}
	if p.Symbol != 0 {
		if receiver, ok := r.result.SymbolDeclaredType(p.Symbol); ok {
			return receiver, true
		}
	}
	return nil, false
}

func (r Reader) receiverSourceType(point cfg.Point, source factflow.ValueSource) (typ.Type, bool) {
	value, ok := r.callArgumentValue(point, source)
	if !ok {
		return nil, false
	}
	return r.ValueTypeWithPresence(value)
}
