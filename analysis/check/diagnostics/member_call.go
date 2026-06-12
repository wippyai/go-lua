package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// MemberCall reports calls through statically known table members that are
// impossible after active literal-discriminant branch narrowing.
type MemberCall Config

func (p MemberCall) Produce(result *check.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := literalEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if !ok {
			continue
		}
		if d, ok := p.call(result, point, fact, envs[point]); ok {
			out = append(out, d)
		}
	}
	return out
}

func (p MemberCall) call(result *check.Result, point cfg.Point, fact semantics.CallFact, env literalEnv) (diagnostic.Diagnostic, bool) {
	receiver, member, callExpr, ok := callMemberAccess(fact)
	if !ok || receiver.Symbol == 0 {
		return diagnostic.Diagnostic{}, false
	}
	baseExpr, ok := result.SymbolTypeAnnotation(receiver.Symbol)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	baseType, ok := typeannotation.Type(baseExpr, p.Resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	narrowed, narrowedByDiscriminant := applyLiteralNarrowing(baseType, receiver, env)
	receiverType := narrowed
	if !narrowedByDiscriminant {
		if !unionReceiver(baseType) {
			return diagnostic.Diagnostic{}, false
		}
		receiverType = baseType
	}
	if typ.IsNever(receiverType) || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) {
		return diagnostic.Diagnostic{}, false
	}

	memberType, status := typeaccess.MemberCall(receiverType, member)
	switch status {
	case typeaccess.MemberCallOK:
		return diagnostic.Diagnostic{}, false
	case typeaccess.MemberCallMissing:
		return memberDiagnostic(result, fact, callExpr, receiverType, member, point), true
	case typeaccess.MemberCallNotCallable:
		return notCallableDiagnostic(result, fact, callExpr, receiverType, memberType, member, point), true
	default:
		return diagnostic.Diagnostic{}, false
	}
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

func applyLiteralNarrowing(base typ.Type, receiver path.Path, env literalEnv) (typ.Type, bool) {
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
			if narrowed, ok := discriminant.NarrowByPathLiteral(out, suffix, lit); ok {
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
		current, ok = typeaccess.Field(t, seg.Name)
	case segment.SegmentIndexInt:
		current, ok = typeaccess.Index(t, typ.LiteralInt(int64(seg.Index)))
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

func memberDiagnostic(result *check.Result, fact semantics.CallFact, call *ast.FuncCallExpr, receiver typ.Type, member string, point cfg.Point) diagnostic.Diagnostic {
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

func notCallableDiagnostic(result *check.Result, fact semantics.CallFact, call *ast.FuncCallExpr, receiver, memberType typ.Type, member string, point cfg.Point) diagnostic.Diagnostic {
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
