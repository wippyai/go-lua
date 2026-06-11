// Package diagnostics produces checker diagnostics from completed analysis
// results. It is intentionally post-solve: diagnostics may observe facts, but
// they do not publish facts back into the fixed point.
package diagnostics

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

const (
	CodeAssignmentType diagnostic.Code = "type.assignment"
	CodeMissingMember  diagnostic.Code = "type.member.missing"
	CodeNotCallable    diagnostic.Code = "type.call.not_callable"
)

type Config struct {
	Registry *axis.Registry
	Resolver typeannotation.Resolver
}

type Producer interface {
	Produce(*check.Result) []diagnostic.Diagnostic
}

func Produce(result *check.Result, config Config) []diagnostic.Diagnostic {
	return produceWithResolver(result, config, nil)
}

func produceWithResolver(result *check.Result, config Config, parent typeannotation.Resolver) []diagnostic.Diagnostic {
	resolver := newResultResolver(result, config.Resolver, parent)
	config.Resolver = resolver
	var out []diagnostic.Diagnostic
	out = append(out, AnnotationAssignability(config).Produce(result)...)
	out = append(out, MemberCall(config).Produce(result)...)
	for _, fn := range result.FunctionResults() {
		out = append(out, produceWithResolver(fn, config, resolver)...)
	}
	return out
}

// AnnotationAssignability reports clear contradictions between a local
// annotation and a syntactically known source literal. Broader flow-to-type
// projection belongs in later producers once the relevant value axes own it.
type AnnotationAssignability Config

func (p AnnotationAssignability) Produce(result *check.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok {
			continue
		}
		if d, ok := p.localAssignment(fact); ok {
			out = append(out, d)
		}
	}
	return out
}

func (p AnnotationAssignability) localAssignment(fact semantics.LocalAssignmentFact) (diagnostic.Diagnostic, bool) {
	if fact.Type == nil || fact.Expr == nil {
		return diagnostic.Diagnostic{}, false
	}
	want, ok := typeannotation.Type(fact.Type, p.Resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	got, ok := literalType(fact.Expr)
	if !ok || !clearMismatch(got, want) {
		return diagnostic.Diagnostic{}, false
	}
	return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type), true
}

func assignmentDiagnostic(name string, want, got typ.Type, expr ast.Expr, annotation ast.TypeExpr) diagnostic.Diagnostic {
	exprSpan := ast.SpanOf(expr)
	typeSpan := ast.SpanOf(annotation)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      exprSpan.StartLine,
			Column:    exprSpan.StartCol,
			EndLine:   exprSpan.EndLine,
			EndColumn: exprSpan.EndCol,
		},
		Span:     exprSpan,
		Code:     CodeAssignmentType,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("cannot assign %s to %s", formatType(got), formatType(want)),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    exprSpan,
				Message: fmt.Sprintf("source expression is %s", formatType(got)),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    typeSpan,
				Message: fmt.Sprintf("%s is annotated %s", name, formatType(want)),
			},
		),
		Labels: []diagnostic.Label{
			{Span: exprSpan, Message: "assigned value"},
			{Span: typeSpan, Message: "declared type"},
		},
	}
}

func clearMismatch(got, want typ.Type) bool {
	return got != nil && want != nil && !subtype.IsSubtype(got, want)
}

func literalType(expr ast.Expr) (typ.Type, bool) {
	return valueexpr.LiteralType(expr)
}

func formatType(t typ.Type) string {
	if t == nil {
		return "unknown"
	}
	return t.String()
}

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

type literalConstraint struct {
	target path.Path
	value  string
}

type literalEnv struct {
	constraints []literalConstraint
}

func literalEnvironments(result *check.Result) map[cfg.Point]literalEnv {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	in := make(map[cfg.Point]literalEnv)
	out := make(map[cfg.Point]literalEnv)
	for _, point := range graph.RPO() {
		in[point] = literalEnv{}
		out[point] = literalEnv{}
	}
	changed := true
	for changed {
		changed = false
		for _, point := range graph.RPO() {
			nextIn := joinPredecessorEnvs(result, graph, point, out)
			nextOut := nextIn
			if !envEqual(in[point], nextIn) {
				in[point] = nextIn
				changed = true
			}
			if !envEqual(out[point], nextOut) {
				out[point] = nextOut
				changed = true
			}
		}
	}
	return in
}

func joinPredecessorEnvs(result *check.Result, graph cfg.Graph, point cfg.Point, out map[cfg.Point]literalEnv) literalEnv {
	preds := graph.Predecessors(point)
	if len(preds) == 0 {
		return literalEnv{}
	}
	var env literalEnv
	for i, pred := range preds {
		edgeEnv := applyLiteralEdge(result, graph, pred, point, out[pred])
		if i == 0 {
			env = edgeEnv
			continue
		}
		env = joinEnvs(env, edgeEnv)
	}
	return env
}

func applyLiteralEdge(result *check.Result, graph cfg.Graph, from, to cfg.Point, env literalEnv) literalEnv {
	cond, ok := graph.EdgeCond(from, to)
	if !ok {
		return env
	}
	if result == nil {
		return env
	}
	fact, ok := result.BranchCondition(from)
	if !ok {
		return env
	}
	return applyBranchLiteral(env, fact.Check, cond)
}

func applyBranchLiteral(env literalEnv, check branchcond.Check, cond bool) literalEnv {
	if check.Kind == branchcond.CheckLiteralEqual && cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString})
	}
	if check.Kind == branchcond.CheckLiteralNot && !cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString})
	}
	return env
}

func (e literalEnv) with(c literalConstraint) literalEnv {
	if c.target.IsEmpty() || c.value == "" {
		return e
	}
	out := literalEnv{constraints: append([]literalConstraint(nil), e.constraints...)}
	for i, existing := range out.constraints {
		if existing.target.Equal(c.target) {
			out.constraints[i] = c
			sortEnv(out)
			return out
		}
	}
	out.constraints = append(out.constraints, c)
	sortEnv(out)
	return out
}

func joinEnvs(a, b literalEnv) literalEnv {
	if len(a.constraints) == 0 || len(b.constraints) == 0 {
		return literalEnv{}
	}
	var out literalEnv
	for _, left := range a.constraints {
		for _, right := range b.constraints {
			if left.value == right.value && left.target.Equal(right.target) {
				out.constraints = append(out.constraints, left)
				break
			}
		}
	}
	sortEnv(out)
	return out
}

func envEqual(a, b literalEnv) bool {
	if len(a.constraints) != len(b.constraints) {
		return false
	}
	sortEnv(a)
	sortEnv(b)
	for i := range a.constraints {
		if a.constraints[i].value != b.constraints[i].value || !a.constraints[i].target.Equal(b.constraints[i].target) {
			return false
		}
	}
	return true
}

func sortEnv(e literalEnv) {
	sort.Slice(e.constraints, func(i, j int) bool {
		left := e.constraints[i]
		right := e.constraints[j]
		if left.target.Root != right.target.Root {
			return left.target.Root < right.target.Root
		}
		if left.target.Symbol != right.target.Symbol {
			return left.target.Symbol < right.target.Symbol
		}
		leftSuffix := segment.FormatSegments(left.target.Segments)
		rightSuffix := segment.FormatSegments(right.target.Segments)
		if leftSuffix != rightSuffix {
			return leftSuffix < rightSuffix
		}
		return left.value < right.value
	})
}

type resultResolver struct {
	types    map[string]ast.TypeExpr
	cache    map[string]typ.Type
	active   map[string]bool
	explicit typeannotation.Resolver
	parent   typeannotation.Resolver
}

func newResultResolver(result *check.Result, explicit, parent typeannotation.Resolver) *resultResolver {
	r := &resultResolver{
		types:    make(map[string]ast.TypeExpr),
		cache:    make(map[string]typ.Type),
		active:   make(map[string]bool),
		explicit: explicit,
		parent:   parent,
	}
	if result == nil || result.Graph() == nil {
		return r
	}
	for _, point := range result.Graph().RPO() {
		fact, ok := result.TypeDefinition(point)
		if !ok || fact.Kind != cfgfacts.TypeDefinitionAlias || fact.Type == nil || fact.Type.Name == "" || fact.Type.Type == nil {
			continue
		}
		r.types[fact.Type.Name] = fact.Type.Type
	}
	return r
}

func (r *resultResolver) ResolveTypeRef(path []string) (typ.Type, bool) {
	if len(path) != 1 {
		return resolveFallback(path, r.explicit, r.parent)
	}
	name := path[0]
	if t, ok := r.cache[name]; ok {
		return t, true
	}
	expr, ok := r.types[name]
	if !ok {
		return resolveFallback(path, r.explicit, r.parent)
	}
	if r.active[name] {
		return typ.NewRef("", name), true
	}
	r.active[name] = true
	t, ok := typeannotation.Type(expr, r)
	delete(r.active, name)
	if !ok {
		return resolveFallback(path, r.explicit, r.parent)
	}
	r.cache[name] = t
	return t, true
}

func resolveFallback(path []string, resolvers ...typeannotation.Resolver) (typ.Type, bool) {
	for _, resolver := range resolvers {
		if resolver == nil {
			continue
		}
		if t, ok := resolver.ResolveTypeRef(path); ok {
			return t, true
		}
	}
	return nil, false
}
