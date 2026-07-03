package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// memberCall reports calls through statically known table members that are
// impossible after active literal-discriminant branch narrowing.
type memberCall producerContext

func (p memberCall) Produce(result *body.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := producerContext(p).guardEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		fact, ok := result.Call(point)
		if !ok {
			continue
		}
		site, ok := result.CallSite(point)
		if !ok {
			continue
		}
		if _, ok := result.CallSignatureType(site); ok {
			continue
		}
		if d, ok := p.call(result, point, fact, site, envs[point]); ok {
			out = append(out, d)
		}
	}
	return out
}

func (p memberCall) call(result *body.Result, point cfg.Point, fact semantics.CallFact, site factflow.CallSite, env guardEnv) (diagnostic.Diagnostic, bool) {
	memberAccess, ok := callMemberAccessInfoForSite(site, fact)
	if !ok || memberAccess.receiver.Symbol == 0 {
		return p.expressionReceiverCall(result, point, fact, env)
	}
	return p.callForAccess(result, point, fact, memberAccess, env)
}

func (p memberCall) callForAccess(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	memberAccess callMemberAccessData,
	env guardEnv,
) (diagnostic.Diagnostic, bool) {
	if memberAccess.receiver.Symbol == 0 {
		return p.expressionReceiverCall(result, point, fact, env)
	}
	baseType, ok := p.receiverType(result, point, fact, memberAccess.receiver, memberAccess.member, env)
	if !ok {
		if currentMember, currentOK := currentMemberCallTypeWithGuard(result, point, memberAccess, env); currentOK &&
			currentMemberUsableForMemberOverride(currentMember) &&
			exactMemberAllCallable(currentMember) {
			return p.callableMemberContract(result, point, fact, memberAccess, typ.Unknown, currentMember, memberSegmentDisplay(memberAccess.member), env)
		}
		return diagnostic.Diagnostic{}, false
	}
	narrowed, narrowedByDiscriminant := applyMemberLiteralNarrowing(result, point, baseType, memberAccess.receiver, env)
	receiverType := narrowed
	reportMemberShape := narrowedByDiscriminant
	if !narrowedByDiscriminant {
		receiverType = baseType
		reportMemberShape = unionReceiver(baseType) || projectionHasNil(receiverType) || memberAccess.member.Kind == segment.SegmentIndexInt || closedConcreteRecordReceiver(result, p.resolver, point, memberAccess.receiver, receiverType)
	}
	if typ.IsNever(receiverType) || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) {
		return diagnostic.Diagnostic{}, false
	}
	if env.hasPresent(memberAccess.receiver) {
		if withoutNil := projectionWithoutNil(receiverType); withoutNil != nil && !typ.IsNever(withoutNil) {
			receiverType = withoutNil
		} else {
			return diagnostic.Diagnostic{}, false
		}
	}
	receiverType = receiverTypeWithBoundaryPresence(result, point, memberAccess.receiver, receiverType)
	if projectionHasNil(receiverType) {
		if fact.Receiver != nil && fact.Method != "" {
			return optionalMethodCallDiagnostic(memberAccess.call, displayPath(result, memberAccess.receiver), memberAccessDisplayName(result, memberAccess)), true
		}
		return memberDiagnostic(result, memberAccess, receiverType, memberSegmentDisplay(memberAccess.member), point), true
	}

	memberType, status, ok := resolvedMemberCallType(result, p.flow, point, receiverType, memberAccess, env, narrowedByDiscriminant)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	switch status {
	case typecall.MemberCallOK:
		return p.callableMemberContract(result, point, fact, memberAccess, receiverType, memberType, memberSegmentDisplay(memberAccess.member), env)
	case typecall.MemberCallMissing:
		if !reportMemberShape {
			return diagnostic.Diagnostic{}, false
		}
		return memberDiagnostic(result, memberAccess, receiverType, memberSegmentDisplay(memberAccess.member), point), true
	case typecall.MemberCallNotCallable:
		if !reportMemberShape && !projectionHasNil(memberType) {
			return diagnostic.Diagnostic{}, false
		}
		return notCallableDiagnostic(result, memberAccess, receiverType, memberType, memberSegmentDisplay(memberAccess.member), point), true
	default:
		return diagnostic.Diagnostic{}, false
	}
}

func (p memberCall) typedSignatureStructuralDiagnostic(result *body.Result, point cfg.Point, fact semantics.CallFact, site factflow.CallSite, env guardEnv) (diagnostic.Diagnostic, bool) {
	memberAccess, ok := callMemberAccessInfoForSite(site, fact)
	if !ok || memberAccess.receiver.Symbol == 0 {
		return p.expressionReceiverCall(result, point, fact, env)
	}
	return p.typedSignatureStructuralDiagnosticForAccess(result, point, fact, memberAccess, env)
}

func (p memberCall) typedSignatureStructuralDiagnosticForAccess(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	memberAccess callMemberAccessData,
	env guardEnv,
) (diagnostic.Diagnostic, bool) {
	if memberAccess.receiver.Symbol == 0 {
		return p.expressionReceiverCall(result, point, fact, env)
	}
	baseType, ok := p.receiverType(result, point, fact, memberAccess.receiver, memberAccess.member, env)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	narrowed, narrowedByDiscriminant := applyMemberLiteralNarrowing(result, point, baseType, memberAccess.receiver, env)
	receiverType := narrowed
	if !narrowedByDiscriminant {
		receiverType = baseType
	}
	if typ.IsNever(receiverType) || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) {
		return diagnostic.Diagnostic{}, false
	}
	if env.hasPresent(memberAccess.receiver) {
		if withoutNil := projectionWithoutNil(receiverType); withoutNil != nil && !typ.IsNever(withoutNil) {
			receiverType = withoutNil
		} else {
			return diagnostic.Diagnostic{}, false
		}
	}
	receiverType = receiverTypeWithBoundaryPresence(result, point, memberAccess.receiver, receiverType)
	if projectionHasNil(receiverType) {
		if fact.Receiver != nil && fact.Method != "" {
			return optionalMethodCallDiagnostic(fact.Call, displayPath(result, memberAccess.receiver), memberAccessDisplayName(result, memberAccess)), true
		}
		return memberDiagnostic(result, memberAccess, receiverType, memberSegmentDisplay(memberAccess.member), point), true
	}
	memberType, status := memberCallType(receiverType, memberAccess.member)
	switch status {
	case typecall.MemberCallMissing:
		return memberDiagnostic(result, memberAccess, receiverType, memberSegmentDisplay(memberAccess.member), point), true
	case typecall.MemberCallNotCallable:
		if memberType == nil {
			memberType = typ.Unknown
		}
		return notCallableDiagnostic(result, memberAccess, receiverType, memberType, memberSegmentDisplay(memberAccess.member), point), true
	}
	return diagnostic.Diagnostic{}, false
}

func currentMemberCallType(result *body.Result, point cfg.Point, memberAccess callMemberAccessData) (typ.Type, bool) {
	if result == nil || memberAccess.receiver.IsEmpty() {
		return nil, false
	}
	memberPath := memberAccess.receiver.Append(memberAccess.member)
	query := newDiagnosticQuery(result)
	value, ok := query.PathValueBeforeBoundary(point, memberPath)
	if !ok {
		value, ok = query.PathValueAtBoundary(point, memberPath)
	}
	if !ok {
		return nil, false
	}
	return query.ValueTypeWithPresence(value)
}

func currentMemberCallTypeWithGuard(result *body.Result, point cfg.Point, memberAccess callMemberAccessData, env guardEnv) (typ.Type, bool) {
	memberType, ok := currentMemberCallType(result, point, memberAccess)
	if !ok {
		return nil, false
	}
	if memberCallGuardedPresent(memberAccess, env) {
		if withoutNil := projectionWithoutNil(memberType); withoutNil != nil && !typ.IsNever(withoutNil) {
			memberType = withoutNil
		}
	}
	return memberType, true
}

func memberCallGuardedPresent(memberAccess callMemberAccessData, env guardEnv) bool {
	memberPath := memberAccess.receiver.Append(memberAccess.member)
	return env.hasPresent(memberPath) || env.hasTruthy(memberPath)
}

func resolvedMemberCallType(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, receiverType typ.Type, memberAccess callMemberAccessData, env guardEnv, structuralNarrowed bool) (typ.Type, typecall.MemberCallStatus, bool) {
	if memberAccess.receiver.IsEmpty() {
		return nil, typecall.MemberCallMissing, false
	}
	switch memberCallInvalidationByPriorCall(result, flow, point, memberAccess.receiver, memberAccess.member) {
	case memberCallInvalidationStale:
		return nil, typecall.MemberCallMissing, false
	case memberCallInvalidationResolved:
		currentMember, ok := currentMemberCallTypeWithGuard(result, point, memberAccess, env)
		if !ok || currentMember == nil || typ.IsAny(currentMember) || typ.IsUnknown(currentMember) || typ.IsNever(currentMember) {
			return nil, typecall.MemberCallMissing, false
		}
		status := typecall.MemberCallOK
		if exactMemberCallUnsafe(currentMember) {
			status = typecall.MemberCallNotCallable
		}
		return currentMember, status, true
	}
	memberType, status := memberCallType(receiverType, memberAccess.member)
	if structuralNarrowed && status != typecall.MemberCallOK {
		return memberType, status, true
	}
	if currentMember, ok := currentMemberCallTypeWithGuard(result, point, memberAccess, env); ok &&
		currentMemberUsableForMemberOverride(currentMember) &&
		currentMemberCanOverrideStructuralMember(currentMember, memberType) {
		if status == typecall.MemberCallMissing && projectionHasNil(currentMember) {
			return memberType, status, true
		}
		memberType = currentMember
		if exactMemberCallUnsafe(currentMember) || projectionHasNil(currentMember) {
			status = typecall.MemberCallNotCallable
		} else {
			status = typecall.MemberCallOK
		}
	}
	return memberType, status, true
}

func currentMemberUsableForMemberOverride(t typ.Type) bool {
	if t == nil || typ.IsNever(t) || typ.Nil.Equals(t) || topLikeType(t) {
		return false
	}
	if present := projectionWithoutNil(t); present != nil && !typ.IsNever(present) && topLikeType(present) {
		return false
	}
	return true
}

func currentMemberCanOverrideStructuralMember(current, structural typ.Type) bool {
	if structural == nil || typ.IsNever(structural) || topLikeType(structural) {
		return true
	}
	return !(subtype.IsSubtype(structural, current) && !subtype.IsSubtype(current, structural))
}

func exactMemberAllCallable(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	if callable, ok := typecall.Callable(t); ok && callable != nil {
		return true
	}
	if union, ok := transparentExpectedType(t).(*typ.Union); ok && len(union.Members) != 0 {
		for _, member := range union.Members {
			if !exactMemberAllCallable(member) {
				return false
			}
		}
		return true
	}
	return false
}

func exactMemberCallUnsafe(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	return !exactMemberAllCallable(t)
}

// expressionReceiverCall handles a colon-method call whose receiver is an
// expression with no resolvable symbol path (e.g. make()[1]:topic()). When the
// receiver type is provably optional, calling a method on it without a nil check
// is unsound and is reported here.
func (p memberCall) expressionReceiverCall(result *body.Result, point cfg.Point, fact semantics.CallFact, env guardEnv) (diagnostic.Diagnostic, bool) {
	if fact.Receiver == nil || fact.Method == "" || fact.Call == nil {
		return diagnostic.Diagnostic{}, false
	}
	receiverType, ok := newFlowExpressionTyper(result, p.resolver, point, env).typeOf(fact.Receiver)
	if !ok || receiverType == nil {
		return diagnostic.Diagnostic{}, false
	}
	if typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return diagnostic.Diagnostic{}, false
	}
	if !projectionHasNil(receiverType) {
		return diagnostic.Diagnostic{}, false
	}
	return optionalMethodCallDiagnostic(fact.Call, "", fact.Method), true
}

func optionalMethodCallDiagnostic(call *ast.FuncCallExpr, receiverName, callName string) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	subject := "receiver"
	if receiverName != "" {
		subject = "receiver " + receiverName
	}
	target := ""
	if callName != "" {
		target = " at call to " + callName
	}
	guardSubject := subject
	if receiverName == "" {
		guardSubject = "the receiver"
	}
	callTarget := "this method call"
	if callName != "" {
		callTarget = "calling " + callName
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeOptionalMethodCall,
		Severity: diagnostic.SeverityError,
		Message:  optionalMethodCallMessage(),
		Labels:   []diagnostic.Label{sourceLabel(span, labelMethodCall)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: optionalMethodReceiverEvidence(subject, target),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnostic.TrustUnknown,
				Span:    span,
				Message: optionalMethodMissingNilCheckEvidence(guardSubject, callTarget),
			},
		),
		Help: optionalMethodCallHelp(receiverName, callName),
	})
}

func (p memberCall) callableMemberContract(result *body.Result, point cfg.Point, fact semantics.CallFact, memberAccess callMemberAccessData, receiverType, memberType typ.Type, member string, env guardEnv) (diagnostic.Diagnostic, bool) {
	site, ok := result.CallSite(point)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	contract, ok := p.memberFunctionContract(result, point, fact, site, receiverType, memberType)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	contract.name = memberCallContractName(result, memberAccess)
	contract.declSpan = ast.SpanOf(fact.Call)
	return directFunctionCallContractDiagnostic(producerContext(p), result, point, fact, site, contract, nil, env)
}

func (p memberCall) memberFunctionContract(result *body.Result, point cfg.Point, fact semantics.CallFact, site factflow.CallSite, receiverType, memberType typ.Type) (directFunctionContract, bool) {
	bindReceiverSelf := fact.Receiver != nil && fact.Method != ""
	if access, ok := callMemberAccessInfoForSite(site, fact); ok {
		switch memberCallInvalidationByPriorCall(result, p.flow, point, access.receiver, access.member) {
		case memberCallInvalidationStale:
			return directFunctionContract{}, false
		case memberCallInvalidationResolved:
			currentMember, ok := currentMemberCallType(result, point, access)
			if !ok {
				return directFunctionContract{}, false
			}
			return memberFunctionTypeContract(receiverType, currentMember, bindReceiverSelf)
		}
		memberPath := access.receiver.Append(access.member)
		fn, defPoint, hasDefinition := dominatingFunctionDefinitionForPathWithPoint(result, point, memberPath)
		if hasDefinition && !memberPathReassignedAfterDefinition(result, p.flow, defPoint, point, memberPath) {
			return p.memberFunctionDefinitionContract(result, fn)
		}
		if currentMember, ok := currentMemberCallType(result, point, access); ok && currentMemberUsableForMemberOverride(currentMember) {
			return memberFunctionTypeContract(receiverType, currentMember, bindReceiverSelf)
		}
		if hasDefinition {
			return p.memberFunctionDefinitionContract(result, fn)
		}
	}
	return memberFunctionTypeContract(receiverType, memberType, bindReceiverSelf)
}

func (p memberCall) memberFunctionDefinitionContract(result *body.Result, fn *ast.FunctionExpr) (directFunctionContract, bool) {
	contract, ok := lowerDirectFunctionContractInResultScope(result, fn, p.resolver)
	if !ok {
		return directFunctionContract{}, false
	}
	if selfType, ok := implicitSelfEntryType(result, fn); ok && len(contract.params) != 0 && contract.params[0].implicitSelf {
		contract.params[0].typ = selfType
		contract.params[0].explicit = true
		contract.params[0].optional = false
	}
	return contract, true
}

func memberPathReassignedAfterDefinition(result *body.Result, flow *diagnosticFlowCache, defPoint, point cfg.Point, target path.Path) bool {
	graph := result.Graph()
	if graph == nil || defPoint == 0 || point == 0 || target.IsEmpty() {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == defPoint {
			continue
		}
		if !diagnosticCanReach(flow, graph, defPoint, candidate) || !diagnosticCanReach(flow, graph, candidate, point) {
			continue
		}
		if callOutcomeInvalidatesMemberPath(result, candidate, target) {
			return true
		}
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok || !ordinaryAssignmentInvalidatesMemberPath(fact, target) {
			continue
		}
		return true
	}
	return false
}

func ordinaryAssignmentInvalidatesMemberPath(fact semantics.OrdinaryAssignmentFact, target path.Path) bool {
	if fact.HasPath {
		return pathHasPrefixStaticEquiv(target, fact.Path)
	}
	return fact.HasSymbol && target.Symbol != 0 && fact.Symbol == target.Symbol
}

func pathHasPrefixStaticEquiv(candidate, prefix path.Path) bool {
	if !samePathRootIgnoringVersion(candidate, prefix) {
		return false
	}
	return pathaddr.SegmentsHasPrefix(candidate.Segments, prefix.Segments)
}

type memberCallInvalidation uint8

const (
	memberCallInvalidationNone memberCallInvalidation = iota
	memberCallInvalidationResolved
	memberCallInvalidationStale
)

func (i memberCallInvalidation) invalidated() bool {
	return i != memberCallInvalidationNone
}

func memberCallInvalidationByPriorCall(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, receiver path.Path, member segment.Segment) memberCallInvalidation {
	if result == nil || receiver.IsEmpty() {
		return memberCallInvalidationNone
	}
	graph := result.Graph()
	if graph == nil {
		return memberCallInvalidationNone
	}
	out := memberCallInvalidationNone
	for _, candidate := range graph.RPO() {
		if candidate == point {
			continue
		}
		if !diagnosticCanReach(flow, graph, candidate, point) {
			continue
		}
		switch callOutcomeInvalidatesMemberAccess(result, candidate, receiver, member) {
		case memberCallInvalidationStale:
			return memberCallInvalidationStale
		case memberCallInvalidationResolved:
			out = memberCallInvalidationResolved
		}
	}
	return out
}

func callOutcomeInvalidatesMemberPath(result *body.Result, point cfg.Point, target path.Path) bool {
	if result == nil || target.IsEmpty() {
		return false
	}
	site, ok := result.CallSite(point)
	if !ok {
		return false
	}
	outcome, ok := result.CallOutcomeAt(point)
	if !ok {
		return false
	}
	targets, ok := callOutcomeGuardInvalidationPaths(result, site, outcome)
	if !ok {
		return false
	}
	for _, invalidated := range targets {
		if invalidationPathReachesPath(result, point, invalidated.path, target).invalidated() {
			return true
		}
	}
	return false
}

func callOutcomeInvalidatesMemberAccess(result *body.Result, point cfg.Point, receiver path.Path, member segment.Segment) memberCallInvalidation {
	if result == nil || receiver.IsEmpty() {
		return memberCallInvalidationNone
	}
	site, ok := result.CallSite(point)
	if !ok {
		return memberCallInvalidationNone
	}
	outcome, ok := result.CallOutcomeAt(point)
	if !ok {
		return memberCallInvalidationNone
	}
	targets, ok := callOutcomeGuardInvalidationPaths(result, site, outcome)
	if !ok {
		return memberCallInvalidationNone
	}
	out := memberCallInvalidationNone
	for _, invalidated := range targets {
		for _, memberPath := range memberAccessPaths(receiver, member) {
			switch invalidationPathReachesPath(result, point, invalidated.path, memberPath) {
			case memberCallInvalidationStale:
				return memberCallInvalidationStale
			case memberCallInvalidationResolved:
				out = memberCallInvalidationResolved
			}
		}
	}
	return out
}

func invalidationPathReachesPath(result *body.Result, point cfg.Point, invalidated, target path.Path) memberCallInvalidation {
	if invalidated.IsEmpty() || target.IsEmpty() {
		return memberCallInvalidationNone
	}
	if target.HasPrefix(invalidated) {
		return memberCallInvalidationResolved
	}
	if len(invalidated.Segments) > len(target.Segments) {
		return memberCallInvalidationNone
	}
	invalidatedRoot := invalidated.RootOnly()
	if !pathRootReadable(result, point, invalidatedRoot) && memberSegmentsHavePrefix(target.Segments, invalidated.Segments) {
		return memberCallInvalidationStale
	}
	for prefixLen := 0; prefixLen <= len(target.Segments); prefixLen++ {
		if len(invalidated.Segments) > len(target.Segments)-prefixLen {
			break
		}
		if !memberSegmentsHavePrefix(target.Segments[prefixLen:], invalidated.Segments) {
			continue
		}
		targetPrefix := target
		targetPrefix.Segments = append(target.Segments[:0:0], target.Segments[:prefixLen]...)
		if pathsShareExactIdentity(result, point, invalidatedRoot, targetPrefix) {
			return memberCallInvalidationResolved
		}
	}
	return memberCallInvalidationNone
}

func pathRootReadable(result *body.Result, point cfg.Point, root path.Path) bool {
	if result == nil || root.IsEmpty() {
		return false
	}
	_, ok := newDiagnosticQuery(result).PathValueAtBoundary(point, root)
	return ok
}

func memberSegmentsHavePrefix(candidate, prefix []segment.Segment) bool {
	if len(prefix) > len(candidate) {
		return false
	}
	for i := range prefix {
		if candidate[i] != prefix[i] {
			return false
		}
	}
	return true
}

func memberAccessPaths(receiver path.Path, member segment.Segment) []path.Path {
	if receiver.IsEmpty() {
		return nil
	}
	switch member.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		if member.Name == "" {
			return nil
		}
		return []path.Path{receiver.Field(member.Name), receiver.IndexStr(member.Name)}
	case segment.SegmentIndexInt:
		return []path.Path{receiver.IndexInt(member.Index)}
	default:
		return nil
	}
}

func memberFunctionTypeContract(receiverType, memberType typ.Type, bindReceiverSelf bool) (directFunctionContract, bool) {
	callable, ok := typecall.Callable(memberType)
	if !ok {
		return directFunctionContract{}, false
	}
	if bindReceiverSelf {
		if substituted, ok := subst.Self(callable, receiverType).(*typ.Function); ok {
			callable = substituted
		}
	}
	return lowerDirectFunctionType(callable), true
}

func implicitSelfEntryType(result *body.Result, fn *ast.FunctionExpr) (typ.Type, bool) {
	child := functionResultForExpr(result, fn)
	if child == nil {
		return nil, false
	}
	for _, slot := range child.FunctionParamSlots(fn) {
		if !slot.ImplicitSelf || slot.Symbol == 0 {
			continue
		}
		entry, ok := child.EntryState()
		reg := child.Registry()
		if !ok || reg == nil {
			return nil, false
		}
		key := statekey.SymbolValue(slot.Symbol)
		if key == 0 {
			return nil, false
		}
		value := entry.ReadValue(reg, key)
		if product.Equal(reg, value, product.Bottom(reg)) {
			return nil, false
		}
		t, ok := newDiagnosticQuery(child).ValueTypeWithPresence(value)
		if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
			return nil, false
		}
		return t, true
	}
	return nil, false
}

func functionResultForExpr(result *body.Result, fn *ast.FunctionExpr) *body.Result {
	if result == nil || fn == nil {
		return nil
	}
	if result.Function() == fn {
		return result
	}
	for _, child := range result.FunctionResults() {
		if found := functionResultForExpr(child, fn); found != nil {
			return found
		}
	}
	return nil
}

func colonMemberCallContract(receiverType typ.Type, contract directFunctionContract) directFunctionContract {
	if !colonMemberCallConsumesReceiver(contract, receiverType) {
		return contract
	}
	return memberContractWithoutReceiver(contract)
}

func colonMemberCallConsumesReceiver(contract directFunctionContract, receiverType typ.Type) bool {
	if len(contract.params) == 0 {
		return false
	}
	if contract.params[0].implicitSelf {
		return true
	}
	if contract.source != nil && typecall.CallableConsumesReceiver(contract.source, receiverType) {
		return true
	}
	self := contract.params[0]
	if !self.explicit || self.typ == nil || typ.IsAny(self.typ) || typ.IsUnknown(self.typ) {
		return false
	}
	return typecall.ParamConsumesReceiver("", self.typ, receiverType)
}

func memberContractWithoutReceiver(contract directFunctionContract) directFunctionContract {
	shifted := contract
	shifted.params = append([]directCallParam(nil), contract.params[1:]...)
	if contract.source != nil && len(contract.source.Params) > 0 {
		sourceParams := append([]typ.Param(nil), contract.source.Params[1:]...)
		shifted.source = typ.RebuildFunction(typ.FunctionParts{
			TypeParams: contract.source.TypeParams,
			Params:     sourceParams,
			Variadic:   contract.source.Variadic,
			Returns:    contract.source.Returns,
		})
	}
	return shifted
}

func memberCallContractName(result *body.Result, access callMemberAccessData) string {
	if name := memberAccessDisplayName(result, access); name != "" {
		return name
	}
	return "receiver"
}

func memberAccessDisplayName(result *body.Result, access callMemberAccessData) string {
	member := memberSegmentDisplay(access.member)
	if member == "" {
		return ""
	}
	if result == nil || access.receiver.IsEmpty() {
		return memberPathName("receiver", member)
	}
	if receiver := displayPath(result, access.receiver); receiver != "" {
		return memberPathName(receiver, member)
	}
	return memberPathName("receiver", member)
}

func (p memberCall) receiverType(result *body.Result, point cfg.Point, fact semantics.CallFact, receiver path.Path, member segment.Segment, env guardEnv) (typ.Type, bool) {
	if receiverExpr := memberReceiverExpr(fact); receiverExpr != nil {
		flowType, flowOK := newFlowExpressionTyper(result, p.resolver, point, env).typeOf(receiverExpr)
		structuralType, structuralOK := newStructuralFlowExpressionTyper(result, p.resolver, point, env).typeOf(receiverExpr)
		callResultType, callResultOK := p.callResultReceiverType(result, point, receiver)
		if flowOK && structuralOK && preferStructuralReceiverType(flowType, structuralType) {
			if callResultOK && preferCallResultReceiverType(structuralType, callResultType, member) {
				return callResultType, true
			}
			return structuralType, true
		}
		if flowOK {
			if callResultOK && preferCallResultReceiverType(flowType, callResultType, member) {
				return callResultType, true
			}
			return flowType, true
		}
		if structuralOK {
			if callResultOK && preferCallResultReceiverType(structuralType, callResultType, member) {
				return callResultType, true
			}
			return structuralType, true
		}
		if callResultOK {
			return callResultType, true
		}
	}
	if baseExpr, ok := result.SymbolTypeAnnotation(receiver.Symbol); ok {
		return lowerType(baseExpr, p.resolver)
	}
	if callResultType, ok := p.callResultReceiverType(result, point, receiver); ok {
		return callResultType, true
	}
	value, ok := newDiagnosticQuery(result).PathValueAtBoundary(point, receiver)
	if !ok {
		return nil, false
	}
	return receiverTypeFromBoundary(result, value)
}

func memberReceiverExpr(fact semantics.CallFact) ast.Expr {
	if fact.Receiver != nil {
		return fact.Receiver
	}
	if fact.Call == nil || fact.Call.Func == nil {
		return nil
	}
	if attr, ok := fact.Call.Func.(*ast.AttrGetExpr); ok {
		return attr.Object
	}
	return nil
}

func (p memberCall) callResultReceiverType(result *body.Result, point cfg.Point, receiver path.Path) (typ.Type, bool) {
	if result == nil || receiver.Symbol == 0 {
		return nil, false
	}
	root, ok := dominatingCallResultRootType(result, producerContext(p), point, receiver.Symbol, nil)
	if !ok || root == nil || typ.IsAny(root) || typ.IsUnknown(root) {
		return nil, false
	}
	if len(receiver.Segments) == 0 {
		return root, true
	}
	return expectedTypeAtSegments(root, receiver.Segments)
}

func preferCallResultReceiverType(current, callResult typ.Type, member segment.Segment) bool {
	if current == nil || callResult == nil || typ.SameNodeOrAcyclicEqual(current, callResult) {
		return false
	}
	callResultMember, callResultStatus := memberCallType(callResult, member)
	if callResultStatus != typecall.MemberCallOK || callResultMember == nil || exactMemberCallUnsafe(callResultMember) {
		return false
	}
	currentMember, currentStatus := memberCallType(current, member)
	if currentStatus != typecall.MemberCallOK {
		return true
	}
	return currentMember != nil && (exactMemberCallUnsafe(currentMember) || projectionHasNil(currentMember))
}

func preferStructuralReceiverType(flowType, structuralType typ.Type) bool {
	if flowType == nil || structuralType == nil || typ.SameNodeOrAcyclicEqual(flowType, structuralType) {
		return false
	}
	if projectionHasNil(flowType) && !projectionHasNil(structuralType) {
		return receiverRecordLike(flowType) || receiverRecordLike(structuralType)
	}
	return receiverRecordLike(flowType) && receiverRecordLike(structuralType)
}

func receiverRecordLike(t typ.Type) bool {
	return receiverRecordLikeDepth(t, 0)
}

func receiverRecordLikeDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Record:
		return true
	case *typ.Optional:
		return receiverRecordLikeDepth(tt.Inner, depth+1)
	case *typ.Recursive:
		return receiverRecordLikeDepth(tt.Body, depth+1)
	default:
		return false
	}
}

func receiverTypeFromBoundary(result *body.Result, value product.Value) (typ.Type, bool) {
	return newDiagnosticQuery(result).ValueTypeWithPresence(value)
}

func receiverTypeWithBoundaryPresence(result *body.Result, point cfg.Point, receiver path.Path, t typ.Type) typ.Type {
	if result == nil || receiver.IsEmpty() || t == nil || !projectionHasNil(t) {
		return t
	}
	query := newDiagnosticQuery(result)
	value, ok := query.PathValueAtBoundary(point, receiver)
	if !ok {
		return t
	}
	if presence.Equal(product.PresenceOf(value), presence.Present()) {
		withoutNil := projectionWithoutNil(t)
		if withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil
		}
	}
	boundaryType, ok := query.ValueTypeWithPresence(value)
	if !ok || boundaryType == nil || typ.IsAny(boundaryType) || typ.IsUnknown(boundaryType) || projectionHasNil(boundaryType) {
		return t
	}
	withoutNil := projectionWithoutNil(t)
	if withoutNil == nil || typ.IsNever(withoutNil) {
		return t
	}
	return withoutNil
}

type callMemberAccessData struct {
	receiver path.Path
	member   segment.Segment
	call     *ast.FuncCallExpr
}

func memberSegmentDisplay(member segment.Segment) string {
	switch member.Kind {
	case segment.SegmentField:
		return member.Name
	case segment.SegmentIndexString, segment.SegmentIndexInt:
		return segment.FormatSegments([]segment.Segment{member})
	default:
		return ""
	}
}

func memberPathName(root, member string) string {
	if member == "" {
		return root
	}
	if member[0] == '[' {
		return root + member
	}
	return root + "." + member
}

func memberCallType(receiverType typ.Type, member segment.Segment) (typ.Type, typecall.MemberCallStatus) {
	switch member.Kind {
	case segment.SegmentField:
		return typecall.MemberCall(receiverType, member.Name)
	case segment.SegmentIndexString:
		return typecall.IndexedMemberCall(receiverType, typ.LiteralString(member.Name))
	case segment.SegmentIndexInt:
		return typecall.IndexedMemberCall(receiverType, typ.LiteralInt(int64(member.Index)))
	default:
		return nil, typecall.MemberCallMissing
	}
}

func applyLiteralNarrowing(base typ.Type, receiver path.Path, env guardEnv) (typ.Type, bool) {
	if base == nil || (len(env.constraints) == 0 && len(env.truthy) == 0 && len(env.falsy) == 0) {
		return base, false
	}
	out := base
	changed := false
	for _, c := range env.constraints {
		suffix, ok := suffixFromReceiver(receiver, c.target)
		if !ok {
			continue
		}
		if c.negated {
			if narrowed, ok := variant.NarrowByPathLiteralNot(out, suffix, c.value); ok {
				out = narrowed
				changed = true
			}
		} else {
			if narrowed, ok := variant.NarrowByPathLiteral(out, suffix, c.value); ok {
				out = narrowed
				changed = true
			}
		}
	}
	for _, target := range env.truthy {
		suffix, ok := suffixFromReceiver(receiver, target)
		if !ok {
			continue
		}
		if narrowed, ok := variant.NarrowByPathTruthy(out, suffix); ok {
			out = narrowed
			changed = true
		}
	}
	for _, target := range env.falsy {
		suffix, ok := suffixFromReceiver(receiver, target)
		if !ok {
			continue
		}
		if narrowed, ok := variant.NarrowByPathFalsy(out, suffix); ok {
			out = narrowed
			changed = true
		}
	}
	return out, changed
}

func applyMemberLiteralNarrowing(result *body.Result, point cfg.Point, base typ.Type, receiver path.Path, env guardEnv) (typ.Type, bool) {
	out, changed := applyLiteralNarrowing(base, receiver, env)
	if result == nil || base == nil || len(env.constraints) == 0 || receiver.IsEmpty() {
		return out, changed
	}
	for _, c := range env.constraints {
		if _, ok := suffixFromReceiver(receiver, c.target); ok {
			continue
		}
		suffix, ok := suffixFromEquivalentReceiver(result, point, receiver, c.target)
		if !ok {
			continue
		}
		if c.negated {
			if narrowed, ok := variant.NarrowByPathLiteralNot(out, suffix, c.value); ok {
				out = narrowed
				changed = true
			}
		} else {
			if narrowed, ok := variant.NarrowByPathLiteral(out, suffix, c.value); ok {
				out = narrowed
				changed = true
			}
		}
	}
	return out, changed
}

func suffixFromEquivalentReceiver(result *body.Result, point cfg.Point, receiver, target path.Path) ([]segment.Segment, bool) {
	if result == nil || receiver.IsEmpty() || target.IsEmpty() || len(target.Segments) == 0 {
		return nil, false
	}
	for prefixLen := len(target.Segments); prefixLen >= 0; prefixLen-- {
		prefix := target
		prefix.Segments = append(target.Segments[:0:0], target.Segments[:prefixLen]...)
		if prefix.Equal(target) || prefix.Equal(receiver) {
			continue
		}
		if !pathsShareExactIdentity(result, point, receiver, prefix) {
			continue
		}
		return append([]segment.Segment(nil), target.Segments[prefixLen:]...), true
	}
	return nil, false
}

func pathsShareExactIdentity(result *body.Result, point cfg.Point, left, right path.Path) bool {
	if result == nil || left.IsEmpty() || right.IsEmpty() || left.Equal(right) {
		return false
	}
	if result.PathsEquivalentAtBoundary(point, left, right) {
		return true
	}
	query := newDiagnosticQuery(result)
	leftValue, leftOK := query.PathValueAtBoundary(point, left)
	rightValue, rightOK := query.PathValueAtBoundary(point, right)
	if !leftOK || !rightOK {
		return false
	}
	reg := result.Registry()
	if reg == nil {
		return false
	}
	leftID, leftOK := product.Get(reg, leftValue, identity.Key).ID()
	rightID, rightOK := product.Get(reg, rightValue, identity.Key).ID()
	return leftOK && rightOK && leftID == rightID
}

func unionReceiver(t typ.Type) bool {
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return true
	case *typ.Alias:
		return unionReceiver(v.UnaliasedTarget())
	default:
		return false
	}
}

func closedConcreteRecordReceiver(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, receiver path.Path, t typ.Type) bool {
	if !closedMemberReceiverRoot(result, resolver, receiver) {
		return false
	}
	if result != nil && !receiver.IsEmpty() {
		query := newDiagnosticQuery(result)
		if value, ok := query.PathValueAtBoundary(point, receiver); ok && query.ValueHasExactIdentity(value) {
			return false
		}
	}
	rec, ok := transparentExpectedType(t).(*typ.Record)
	if !ok || rec == nil || rec.Open || rec.HasMapComponent() {
		return false
	}
	return len(rec.Fields) != 0 || len(rec.StaticMembers) != 0
}

func closedMemberReceiverRoot(result *body.Result, resolver typeannotation.Resolver, receiver path.Path) bool {
	if result == nil || receiver.Symbol == 0 {
		return false
	}
	if expr, ok := result.SymbolTypeAnnotation(receiver.Symbol); ok {
		t, ok := lowerType(expr, resolver)
		return !ok || !topLikeType(t)
	}
	kind, ok := result.SymbolKind(receiver.Symbol)
	return ok && (kind == symbol.Param || kind == symbol.Global)
}

func suffixFromReceiver(receiver, target path.Path) ([]segment.Segment, bool) {
	if receiver.Symbol != target.Symbol || receiver.Root != target.Root || len(target.Segments) <= len(receiver.Segments) {
		return nil, false
	}
	for i := range receiver.Segments {
		if receiver.Segments[i] != target.Segments[i] {
			return nil, false
		}
	}
	return append([]segment.Segment(nil), target.Segments[len(receiver.Segments):]...), true
}

func memberDiagnostic(result *body.Result, access callMemberAccessData, receiver typ.Type, member string, point cfg.Point) diagnostic.Diagnostic {
	span := ast.SpanOf(access.call)
	memberPath := memberAccessDisplayName(result, access)
	if memberPath == "" {
		memberPath = "receiver"
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeMissingMember,
		Severity: diagnostic.SeverityError,
		Message:  missingMemberMessage(receiver, member),
		Labels:   []diagnostic.Label{sourceLabel(span, labelMemberCall)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: receiverForMemberEvidence(memberPath, receiver),
			},
		),
		Help: missingMemberHelp(member),
	})
}

func notCallableDiagnostic(result *body.Result, access callMemberAccessData, receiver, memberType typ.Type, member string, point cfg.Point) diagnostic.Diagnostic {
	span := ast.SpanOf(access.call)
	memberPath := memberAccessDisplayName(result, access)
	if memberPath == "" {
		memberPath = "receiver"
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeNotCallable,
		Severity: diagnostic.SeverityError,
		Message:  memberNotCallableMessage(memberPath, receiver, memberType, member),
		Labels:   []diagnostic.Label{sourceLabel(span, labelMemberCall)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: memberTypeAtCallEvidence(memberPath, memberType),
			},
		),
		Help: memberNotCallableHelp(memberPath),
	})
}
