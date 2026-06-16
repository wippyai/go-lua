package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/normalize"
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
	envs := guardEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if !ok {
			continue
		}
		if site, ok := result.CallSite(point); ok {
			if _, hasSignature := result.CallSignature(site); hasSignature {
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
	receiver, member, callExpr, ok := callMemberAccess(fact)
	if !ok || receiver.Symbol == 0 {
		return p.expressionReceiverCall(result, point, fact, env)
	}
	baseType, ok := p.receiverType(result, point, fact, receiver, env)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	narrowed, narrowedByDiscriminant := applyLiteralNarrowing(baseType, receiver, env)
	receiverType := narrowed
	reportMemberShape := narrowedByDiscriminant
	if !narrowedByDiscriminant {
		receiverType = baseType
		reportMemberShape = unionReceiver(baseType) || projectionHasNil(receiverType)
	}
	if typ.IsNever(receiverType) || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) {
		return diagnostic.Diagnostic{}, false
	}
	if projectionHasNil(receiverType) {
		if fact.Receiver != nil && fact.Method != "" {
			return optionalMethodCallDiagnostic(callExpr), true
		}
		return memberDiagnostic(result, fact, callExpr, receiverType, member, point), true
	}

	memberType, status := typecall.MemberCall(receiverType, member)
	switch status {
	case typecall.MemberCallOK:
		return p.callableMemberContract(result, point, fact, receiverType, memberType, member)
	case typecall.MemberCallMissing:
		if !reportMemberShape {
			return diagnostic.Diagnostic{}, false
		}
		return memberDiagnostic(result, fact, callExpr, receiverType, member, point), true
	case typecall.MemberCallNotCallable:
		if !reportMemberShape {
			return diagnostic.Diagnostic{}, false
		}
		return notCallableDiagnostic(result, fact, callExpr, receiverType, memberType, member, point), true
	default:
		return diagnostic.Diagnostic{}, false
	}
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
	return optionalMethodCallDiagnostic(fact.Call), true
}

func optionalMethodCallDiagnostic(call *ast.FuncCallExpr) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeOptionalMethodCall,
		Severity: diagnostic.SeverityError,
		Message:  "cannot call method on optional value without nil check",
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: "receiver type at call is optional",
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "method call on optional receiver"}},
	}
}

func (p memberCall) callableMemberContract(result *body.Result, point cfg.Point, fact semantics.CallFact, receiverType, memberType typ.Type, member string) (diagnostic.Diagnostic, bool) {
	callable, ok := typecall.Callable(memberType)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	if substituted, ok := subst.Self(callable, receiverType).(*typ.Function); ok {
		callable = substituted
	}
	contract := lowerDirectFunctionType(callable)
	contract.name = memberCallContractName(result, fact, member)
	contract.declSpan = ast.SpanOf(fact.Call)
	if fact.Receiver != nil && fact.Method != "" {
		contract = colonMemberCallContract(receiverType, contract)
	}
	return directCallContract(p).directFunctionCall(result, point, fact, contract, nil, guardEnv{})
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
	if contract.source != nil && len(contract.source.Params) > 0 && contract.source.Params[0].Name == "self" {
		return true
	}
	self := contract.params[0]
	if !self.explicit || self.typ == nil || typ.IsAny(self.typ) || typ.IsUnknown(self.typ) {
		return false
	}
	return subtype.IsSubtype(receiverType, self.typ)
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
	name := result.SymbolName(callRootSymbol(fact))
	if name == "" {
		name = "receiver"
	}
	if member == "" {
		return name
	}
	return name + "." + member
}

func (p memberCall) receiverType(result *body.Result, point cfg.Point, fact semantics.CallFact, receiver path.Path, env guardEnv) (typ.Type, bool) {
	if fact.Receiver != nil {
		if t, ok := newFlowExpressionTyper(result, p.resolver, point, env).typeOf(fact.Receiver); ok {
			return t, true
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

func receiverTypeFromBoundary(result *body.Result, value product.Value) (typ.Type, bool) {
	return readmodel.New(result).ValueTypeWithPresence(value)
}

func callMemberAccess(fact semantics.CallFact) (path.Path, string, *ast.FuncCallExpr, bool) {
	if fact.Call == nil {
		return path.Path{}, "", nil, false
	}
	if fact.HasReceiverPath && fact.Method != "" {
		return fact.ReceiverPath, fact.Method, fact.Call, true
	}
	if !fact.HasCalleePath || len(fact.CalleePath.Segments) == 0 {
		return path.Path{}, "", nil, false
	}
	last := fact.CalleePath.Segments[len(fact.CalleePath.Segments)-1]
	switch last.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		receiver := fact.CalleePath.Parent()
		if receiver.IsEmpty() {
			return path.Path{}, "", nil, false
		}
		return receiver, last.Name, fact.Call, last.Name != ""
	default:
		return path.Path{}, "", nil, false
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
		lit := typ.LiteralString(c.value)
		if c.negated {
			if narrowed, ok := narrowByPathLiteralNot(out, suffix, lit); ok {
				out = narrowed
				changed = true
			}
		} else {
			if narrowed, ok := variant.NarrowByPathLiteral(out, suffix, lit); ok {
				out = narrowed
				changed = true
			}
		}
	}
	return out, changed
}

func narrowByPathLiteralNot(t typ.Type, suffix []segment.Segment, lit typ.Type) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || lit == nil {
		return nil, false
	}
	narrowed, ok := narrowByPathLiteralNotDepth(t, suffix, lit, 0)
	if !ok || narrowed == nil || typ.SameNodeOrAcyclicEqual(narrowed, t) {
		return narrowed, false
	}
	return narrowed, true
}

func narrowByPathLiteralNotDepth(t typ.Type, suffix []segment.Segment, lit typ.Type, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return narrowByPathLiteralNotDepth(v.UnaliasedTarget(), suffix, lit, depth+1)
	case *typ.Optional:
		return narrowByPathLiteralNotDepth(v.Inner, suffix, lit, depth+1)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if !pathAdmitsLiteral(member, suffix, lit, depth+1) {
				out = append(out, member)
			}
		}
		if len(out) == len(v.Members) {
			return t, false
		}
		if len(out) == 0 {
			return typ.Never, true
		}
		return normalize.UnionForEvidence(out...), true
	default:
		if pathAdmitsLiteral(t, suffix, lit, depth+1) {
			return typ.Never, true
		}
		return t, false
	}
}

func pathAdmitsLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type, depth int) bool {
	field, ok := typeAtPath(t, suffix, depth+1)
	return ok && subtype.IsSubtype(lit, field)
}

func typeAtPath(t typ.Type, suffix []segment.Segment, depth int) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	seg := suffix[0]
	var current typ.Type
	var ok bool
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		current, ok = access.Field(t, seg.Name)
	case segment.SegmentIndexInt:
		current, ok = access.Index(t, typ.LiteralInt(int64(seg.Index)))
	default:
		ok = false
	}
	if !ok {
		return nil, false
	}
	if len(suffix) == 1 {
		return current, true
	}
	return typeAtPath(current, suffix[1:], depth+1)
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
	name := result.SymbolName(callRootSymbol(fact))
	if name == "" {
		name = "receiver"
	}
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeMissingMember,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("%s has no member %q", formatType(receiver), member),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("call at CFG point %d reads %s.%s", point, name, member),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("receiver type at call is %s", formatType(receiver)),
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "missing member call"}},
	}
}

func notCallableDiagnostic(result *body.Result, fact semantics.CallFact, call *ast.FuncCallExpr, receiver, memberType typ.Type, member string, point cfg.Point) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	name := result.SymbolName(callRootSymbol(fact))
	if name == "" {
		name = "receiver"
	}
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeNotCallable,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("%s.%s is %s, not callable", formatType(receiver), member, formatType(memberType)),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("call at CFG point %d reads %s.%s", point, name, member),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("member type at call is %s", formatType(memberType)),
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "non-callable member"}},
	}
}

func callRootSymbol(fact semantics.CallFact) symbol.ID {
	if fact.HasReceiverPath {
		return fact.ReceiverPath.Symbol
	}
	return fact.CalleePath.Symbol
}
