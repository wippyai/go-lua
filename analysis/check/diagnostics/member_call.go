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
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	if !narrowedByDiscriminant {
		return diagnostic.Diagnostic{}, false
	}
	if typ.IsNever(narrowed) || typ.IsAny(narrowed) || typ.IsUnknown(narrowed) {
		return diagnostic.Diagnostic{}, false
	}
	memberType, ok := typeaccess.Field(narrowed, member)
	if !ok {
		return memberDiagnostic(result, fact, callExpr, narrowed, member, point), true
	}
	if typ.IsAny(memberType) || typ.IsUnknown(memberType) {
		return diagnostic.Diagnostic{}, false
	}
	if _, ok := typeaccess.Callable(memberType); !ok {
		return notCallableDiagnostic(result, fact, callExpr, narrowed, memberType, member, point), true
	}
	return diagnostic.Diagnostic{}, false
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
		if narrowed, ok := discriminant.NarrowByPathLiteral(out, suffix, typ.LiteralString(c.value)); ok {
			out = narrowed
			changed = true
		}
	}
	return out, changed
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
				Message: fmt.Sprintf("active discriminant narrowing gives receiver type %s", formatType(receiver)),
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
				Message: fmt.Sprintf("member type after narrowing is %s", formatType(memberType)),
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
