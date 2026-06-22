package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subst"
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
	envs := cachedGuardEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		fact, ok := result.Call(point)
		if !ok {
			continue
		}
		if site, ok := result.CallSite(point); ok {
			if hasTypedCallSignature(result, site) {
				continue
			}
		}
		if d, ok := p.call(result, point, fact, envs[point]); ok {
			out = append(out, d)
		}
	}
	return out
}

func (p memberCall) call(result *body.Result, point cfg.Point, fact semantics.CallFact, env guardEnv) (diagnostic.Diagnostic, bool) {
	memberAccess, ok := callMemberAccessInfo(fact)
	if !ok || memberAccess.receiver.Symbol == 0 {
		return p.expressionReceiverCall(result, point, fact, env)
	}
	baseType, ok := p.receiverType(result, point, fact, memberAccess.receiver, env)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	narrowed, narrowedByDiscriminant := applyMemberLiteralNarrowing(result, point, baseType, memberAccess.receiver, env)
	receiverType := narrowed
	reportMemberShape := narrowedByDiscriminant
	if !narrowedByDiscriminant {
		receiverType = baseType
		reportMemberShape = unionReceiver(baseType) || projectionHasNil(receiverType) || memberAccess.member.Kind == segment.SegmentIndexInt
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
			return optionalMethodCallDiagnostic(memberAccess.call, displayPath(result, memberAccess.receiver), memberCallDisplayName(result, fact, memberSegmentDisplay(memberAccess.member))), true
		}
		return memberDiagnostic(result, fact, memberAccess.call, receiverType, memberSegmentDisplay(memberAccess.member), point), true
	}

	memberType, status, ok := resolvedMemberCallType(result, p.flow, point, receiverType, memberAccess)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	switch status {
	case typecall.MemberCallOK:
		return p.callableMemberContract(result, point, fact, receiverType, memberType, memberSegmentDisplay(memberAccess.member))
	case typecall.MemberCallMissing:
		if !reportMemberShape {
			return diagnostic.Diagnostic{}, false
		}
		return memberDiagnostic(result, fact, memberAccess.call, receiverType, memberSegmentDisplay(memberAccess.member), point), true
	case typecall.MemberCallNotCallable:
		if !reportMemberShape && !projectionHasNil(memberType) {
			return diagnostic.Diagnostic{}, false
		}
		return notCallableDiagnostic(result, fact, memberAccess.call, receiverType, memberType, memberSegmentDisplay(memberAccess.member), point), true
	default:
		return diagnostic.Diagnostic{}, false
	}
}

func (p memberCall) typedSignatureStructuralDiagnostic(result *body.Result, point cfg.Point, fact semantics.CallFact, env guardEnv) (diagnostic.Diagnostic, bool) {
	memberAccess, ok := callMemberAccessInfo(fact)
	if !ok || memberAccess.receiver.Symbol == 0 {
		return p.expressionReceiverCall(result, point, fact, env)
	}
	baseType, ok := p.receiverType(result, point, fact, memberAccess.receiver, env)
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
			return optionalMethodCallDiagnostic(fact.Call, displayPath(result, memberAccess.receiver), memberCallDisplayName(result, fact, memberSegmentDisplay(memberAccess.member))), true
		}
		return memberDiagnostic(result, fact, memberAccess.call, receiverType, memberSegmentDisplay(memberAccess.member), point), true
	}
	memberType, status, ok := resolvedMemberCallType(result, p.flow, point, receiverType, memberAccess)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	switch status {
	case typecall.MemberCallMissing:
		return memberDiagnostic(result, fact, memberAccess.call, receiverType, memberSegmentDisplay(memberAccess.member), point), true
	case typecall.MemberCallNotCallable:
		if memberType == nil {
			memberType = typ.Unknown
		}
		return notCallableDiagnostic(result, fact, memberAccess.call, receiverType, memberType, memberSegmentDisplay(memberAccess.member), point), true
	}
	return diagnostic.Diagnostic{}, false
}

func currentMemberCallType(result *body.Result, point cfg.Point, memberAccess callMemberAccessData) (typ.Type, bool) {
	if result == nil || memberAccess.receiver.IsEmpty() {
		return nil, false
	}
	memberPath := memberAccess.receiver.Append(memberAccess.member)
	value, ok := result.PathValueAtBoundary(point, memberPath)
	if !ok {
		return nil, false
	}
	return readmodel.New(result).ValueTypeWithPresence(value)
}

func resolvedMemberCallType(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, receiverType typ.Type, memberAccess callMemberAccessData) (typ.Type, typecall.MemberCallStatus, bool) {
	if memberAccess.receiver.IsEmpty() {
		return nil, typecall.MemberCallMissing, false
	}
	switch memberCallInvalidationByPriorCall(result, flow, point, memberAccess.receiver, memberAccess.member) {
	case memberCallInvalidationStale:
		return nil, typecall.MemberCallMissing, false
	case memberCallInvalidationResolved:
		currentMember, ok := currentMemberCallType(result, point, memberAccess)
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
	if currentMember, ok := currentMemberCallType(result, point, memberAccess); ok && status == typecall.MemberCallOK {
		memberType = currentMember
		if exactMemberCallUnsafe(currentMember) {
			status = typecall.MemberCallNotCallable
		}
	}
	return memberType, status, true
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

func (p memberCall) callableMemberContract(result *body.Result, point cfg.Point, fact semantics.CallFact, receiverType, memberType typ.Type, member string) (diagnostic.Diagnostic, bool) {
	contract, ok := p.memberFunctionContract(result, point, fact, receiverType, memberType)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	contract.name = memberCallContractName(result, fact, member)
	contract.declSpan = ast.SpanOf(fact.Call)
	if fact.Receiver != nil && fact.Method != "" {
		contract = colonMemberCallContract(receiverType, contract)
	}
	return directCallContract(p).directFunctionCall(result, point, fact, contract, nil, guardEnv{})
}

func (p memberCall) memberFunctionContract(result *body.Result, point cfg.Point, fact semantics.CallFact, receiverType, memberType typ.Type) (directFunctionContract, bool) {
	if access, ok := callMemberAccessInfo(fact); ok {
		switch memberCallInvalidationByPriorCall(result, p.flow, point, access.receiver, access.member) {
		case memberCallInvalidationStale:
			return directFunctionContract{}, false
		case memberCallInvalidationResolved:
			currentMember, ok := currentMemberCallType(result, point, access)
			if !ok {
				return directFunctionContract{}, false
			}
			return memberFunctionTypeContract(receiverType, currentMember)
		}
		memberPath := access.receiver.Append(access.member)
		fn, defPoint, hasDefinition := dominatingFunctionDefinitionForPathWithPoint(result, point, memberPath)
		if hasDefinition && !memberPathReassignedAfterDefinition(result, p.flow, defPoint, point, memberPath) {
			return p.memberFunctionDefinitionContract(result, fn)
		}
		if currentMember, ok := currentMemberCallType(result, point, access); ok {
			return memberFunctionTypeContract(receiverType, currentMember)
		}
		if hasDefinition {
			return p.memberFunctionDefinitionContract(result, fn)
		}
	}
	return memberFunctionTypeContract(receiverType, memberType)
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
		return target.HasPrefix(fact.Path)
	}
	return fact.HasSymbol && target.Symbol != 0 && fact.Symbol == target.Symbol
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
		if invalidationPathReachesPath(result, point, invalidated, target).invalidated() {
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
			switch invalidationPathReachesPath(result, point, invalidated, memberPath) {
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
	_, ok := result.PathValueAtBoundary(point, root)
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

func memberFunctionTypeContract(receiverType, memberType typ.Type) (directFunctionContract, bool) {
	callable, ok := typecall.Callable(memberType)
	if !ok {
		return directFunctionContract{}, false
	}
	if substituted, ok := subst.Self(callable, receiverType).(*typ.Function); ok {
		callable = substituted
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
		t, ok := readmodel.New(child).ValueTypeWithPresence(value)
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

func memberCallContractName(result *body.Result, fact semantics.CallFact, member string) string {
	if name := memberCallDisplayName(result, fact, member); name != "" {
		return name
	}
	return "receiver"
}

func memberCallDisplayName(result *body.Result, fact semantics.CallFact, member string) string {
	if result != nil {
		if fact.HasCalleePath && !fact.CalleePath.IsEmpty() {
			if name := displayPath(result, fact.CalleePath); name != "" {
				return name
			}
		}
		if fact.HasReceiverPath && !fact.ReceiverPath.IsEmpty() {
			if receiver := displayPath(result, fact.ReceiverPath); receiver != "" {
				return memberPathName(receiver, member)
			}
		}
	}
	if result != nil {
		name := result.SymbolName(callRootSymbol(fact))
		if name != "" {
			return memberPathName(name, member)
		}
	}
	return ""
}

func (p memberCall) receiverType(result *body.Result, point cfg.Point, fact semantics.CallFact, receiver path.Path, env guardEnv) (typ.Type, bool) {
	if fact.Receiver != nil {
		flowType, flowOK := newFlowExpressionTyper(result, p.resolver, point, env).typeOf(fact.Receiver)
		structuralType, structuralOK := newStructuralFlowExpressionTyper(result, p.resolver, point, env).typeOf(fact.Receiver)
		if flowOK && structuralOK && preferStructuralReceiverType(flowType, structuralType) {
			return structuralType, true
		}
		if flowOK {
			return flowType, true
		}
		if structuralOK {
			return structuralType, true
		}
	}
	if baseExpr, ok := result.SymbolTypeAnnotation(receiver.Symbol); ok {
		return lowerType(baseExpr, p.resolver)
	}
	value, ok := result.PathValueAtBoundary(point, receiver)
	if !ok {
		return nil, false
	}
	return receiverTypeFromBoundary(result, value)
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
	return readmodel.New(result).ValueTypeWithPresence(value)
}

func receiverTypeWithBoundaryPresence(result *body.Result, point cfg.Point, receiver path.Path, t typ.Type) typ.Type {
	if result == nil || receiver.IsEmpty() || len(receiver.Segments) == 0 || t == nil || !projectionHasNil(t) {
		return t
	}
	value, ok := result.PathValueAtBoundary(point, receiver)
	if !ok {
		return t
	}
	boundaryType, ok := readmodel.New(result).ValueTypeWithPresence(value)
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

func callMemberAccessInfo(fact semantics.CallFact) (callMemberAccessData, bool) {
	if fact.Call == nil {
		return callMemberAccessData{}, false
	}
	if fact.HasReceiverPath && fact.Method != "" {
		return callMemberAccessData{
			receiver: fact.ReceiverPath,
			member:   segment.Segment{Kind: segment.SegmentField, Name: fact.Method},
			call:     fact.Call,
		}, true
	}
	if !fact.HasCalleePath || len(fact.CalleePath.Segments) == 0 {
		return callMemberAccessData{}, false
	}
	last := fact.CalleePath.Segments[len(fact.CalleePath.Segments)-1]
	switch last.Kind {
	case segment.SegmentField, segment.SegmentIndexString, segment.SegmentIndexInt:
		receiver := fact.CalleePath.Parent()
		if receiver.IsEmpty() {
			return callMemberAccessData{}, false
		}
		return callMemberAccessData{
			receiver: receiver,
			member:   last,
			call:     fact.Call,
		}, memberSegmentDisplay(last) != ""
	default:
		return callMemberAccessData{}, false
	}
}

func callMemberAccess(fact semantics.CallFact) (path.Path, string, *ast.FuncCallExpr, bool) {
	access, ok := callMemberAccessInfo(fact)
	if !ok {
		return path.Path{}, "", nil, false
	}
	return access.receiver, memberSegmentDisplay(access.member), access.call, true
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
	if base == nil || len(env.constraints) == 0 {
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
	leftValue, leftOK := result.PathValueAtBoundary(point, left)
	rightValue, rightOK := result.PathValueAtBoundary(point, right)
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

func memberDiagnostic(result *body.Result, fact semantics.CallFact, call *ast.FuncCallExpr, receiver typ.Type, member string, point cfg.Point) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	memberPath := memberCallDisplayName(result, fact, member)
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

func notCallableDiagnostic(result *body.Result, fact semantics.CallFact, call *ast.FuncCallExpr, receiver, memberType typ.Type, member string, point cfg.Point) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	memberPath := memberCallDisplayName(result, fact, member)
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

func callRootSymbol(fact semantics.CallFact) symbol.ID {
	if fact.HasReceiverPath {
		return fact.ReceiverPath.Symbol
	}
	return fact.CalleePath.Symbol
}
