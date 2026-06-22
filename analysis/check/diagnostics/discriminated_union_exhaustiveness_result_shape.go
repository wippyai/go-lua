package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

type resultShapeEvidence struct {
	receiver     string
	readPath     string
	discriminant string
	requiredCase string
	readSpan     diagnostic.Span
}

type resultShapeRead struct {
	point              cfg.Point
	expr               *ast.AttrGetExpr
	receiverExpr       ast.Expr
	receiver           pathdom.Path
	readPath           pathdom.Path
	discriminant       pathdom.Path
	discriminantSuffix []segment.Segment
	required           discriminantCase
}

func (p discriminatedUnionExhaustiveness) resultShapeConsumptionDiagnostics(result *body.Result, graph cfg.Graph) []diagnostic.Diagnostic {
	envs := cachedGuardEnvironments(result)
	var out []diagnostic.Diagnostic
	seen := make(map[*ast.AttrGetExpr]struct{})
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		emit := func(expr ast.Expr) {
			for _, read := range p.resultShapeReadsInExpr(result, point, expr, seen) {
				if resultShapeRequiredCaseProven(result, point, envs[point], read.discriminant, read.required) {
					continue
				}
				if p.resultShapeCurrentTypeProvesRequired(result, point, envs[point], read.receiverExpr, read.discriminantSuffix, read.required) {
					continue
				}
				if p.resultShapeCurrentTypeProvesOther(result, point, envs[point], read.receiverExpr, read.discriminantSuffix, read.required) {
					continue
				}
				if resultShapeOtherCaseProven(result, point, envs[point], read.discriminant, read.required) {
					continue
				}
				out = append(out, newResultShapeExhaustivenessDiagnostic(resultShapeEvidence{
					receiver:     read.receiver.String(),
					readPath:     read.readPath.String(),
					discriminant: read.discriminant.String(),
					requiredCase: read.required.name,
					readSpan:     ast.SpanOf(read.expr),
				}))
			}
		}
		if fact, ok := result.LocalAssignment(point); ok {
			emit(fact.Expr)
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			emitAssignmentTargetReads(fact.Target, emit)
			emit(fact.Value)
		}
		if fact, ok := result.Call(point); ok {
			emit(fact.Call)
		}
		if fact, ok := result.ReturnFact(point); ok {
			for _, expr := range fact.Exprs {
				emit(expr)
			}
		}
		if fact, ok := result.BranchCondition(point); ok {
			emit(fact.Condition)
		}
	}
	return out
}

func (p discriminatedUnionExhaustiveness) resultShapeReadsInExpr(result *body.Result, point cfg.Point, expr ast.Expr, seen map[*ast.AttrGetExpr]struct{}) []resultShapeRead {
	var out []resultShapeRead
	p.walkResultShapeReads(result, point, expr, seen, &out, 0)
	return out
}

func (p discriminatedUnionExhaustiveness) walkResultShapeReads(result *body.Result, point cfg.Point, expr ast.Expr, seen map[*ast.AttrGetExpr]struct{}, out *[]resultShapeRead, depth int) {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		p.walkResultShapeReads(result, point, e.Object, seen, out, depth+1)
		if e.KeySyntax == ast.AttrKeyIndex {
			p.walkResultShapeReads(result, point, e.Key, seen, out, depth+1)
		}
		if _, done := seen[e]; done {
			return
		}
		seen[e] = struct{}{}
		if read, ok := p.resultShapeRead(result, point, e); ok {
			*out = append(*out, read)
		}
	case *ast.FuncCallExpr:
		p.walkResultShapeReads(result, point, e.Func, seen, out, depth+1)
		p.walkResultShapeReads(result, point, e.Receiver, seen, out, depth+1)
		for _, arg := range e.Args {
			p.walkResultShapeReads(result, point, arg, seen, out, depth+1)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				p.walkResultShapeReads(result, point, field.Key, seen, out, depth+1)
			}
			p.walkResultShapeReads(result, point, field.Value, seen, out, depth+1)
		}
	case *ast.LogicalOpExpr:
		p.walkResultShapeReads(result, point, e.Lhs, seen, out, depth+1)
		p.walkResultShapeReads(result, point, e.Rhs, seen, out, depth+1)
	case *ast.RelationalOpExpr:
		p.walkResultShapeReads(result, point, e.Lhs, seen, out, depth+1)
		p.walkResultShapeReads(result, point, e.Rhs, seen, out, depth+1)
	case *ast.StringConcatOpExpr:
		p.walkResultShapeReads(result, point, e.Lhs, seen, out, depth+1)
		p.walkResultShapeReads(result, point, e.Rhs, seen, out, depth+1)
	case *ast.ArithmeticOpExpr:
		p.walkResultShapeReads(result, point, e.Lhs, seen, out, depth+1)
		p.walkResultShapeReads(result, point, e.Rhs, seen, out, depth+1)
	case *ast.UnaryMinusOpExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	case *ast.UnaryNotOpExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	case *ast.UnaryLenOpExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	case *ast.UnaryBNotOpExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	case *ast.CastExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	case *ast.NonNilAssertExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	}
}

func (p discriminatedUnionExhaustiveness) resultShapeRead(result *body.Result, point cfg.Point, expr *ast.AttrGetExpr) (resultShapeRead, bool) {
	member, ok := staticMemberReadName(expr)
	if !ok || member == "ok" {
		return resultShapeRead{}, false
	}
	receiverPath, ok := result.ExpressionPath(expr.Object)
	if !ok || receiverPath.Symbol == 0 {
		return resultShapeRead{}, false
	}
	readPath, ok := result.ExpressionPath(expr)
	if !ok || readPath.Symbol == 0 {
		return resultShapeRead{}, false
	}
	receiverType, ok := newStructuralFlowExpressionTyper(result, p.resolver, point, guardEnv{}).broadType(expr.Object)
	if !ok {
		return resultShapeRead{}, false
	}
	discriminant, suffix, required, ok := resultShapeRequiredCaseForMember(receiverPath, receiverType, member)
	if !ok {
		return resultShapeRead{}, false
	}
	return resultShapeRead{
		point:              point,
		expr:               expr,
		receiverExpr:       expr.Object,
		receiver:           receiverPath,
		readPath:           readPath,
		discriminant:       discriminant,
		discriminantSuffix: suffix,
		required:           required,
	}, true
}

func resultShapeRequiredCaseForMember(receiver pathdom.Path, receiverType typ.Type, member string) (pathdom.Path, []segment.Segment, discriminantCase, bool) {
	_, cases, ok := variant.OriginCasesOfType(receiverType)
	if !ok || len(cases) < 2 {
		return pathdom.Path{}, nil, discriminantCase{}, false
	}
	requiredIndex, ok := singleOriginCaseWithField(cases, member)
	if !ok {
		return pathdom.Path{}, nil, discriminantCase{}, false
	}
	for _, domain := range literalDiscriminantDomainsForCases(receiver, cases) {
		for _, c := range domain.cases {
			if c.index == requiredIndex {
				return domain.target, append([]segment.Segment(nil), domain.suffix...), c, true
			}
		}
	}
	return pathdom.Path{}, nil, discriminantCase{}, false
}

func singleOriginCaseWithField(cases []variant.OriginCase, member string) (int, bool) {
	required := -1
	for _, c := range cases {
		if _, ok := access.Field(c.Type, member); !ok {
			continue
		}
		if required >= 0 {
			return 0, false
		}
		required = c.Index
	}
	return required, required >= 0
}

func (p discriminatedUnionExhaustiveness) resultShapeCurrentTypeProvesRequired(result *body.Result, point cfg.Point, env guardEnv, expr ast.Expr, discriminantSuffix []segment.Segment, required discriminantCase) bool {
	if required.literal == nil {
		return false
	}
	current, ok := newStructuralFlowExpressionTyper(result, p.resolver, point, env).typeOf(expr)
	if !ok || current == nil {
		return false
	}
	field, ok := variant.FieldAtPath(current, discriminantSuffix)
	return ok && typ.TypeEquals(field, required.literal)
}

func (p discriminatedUnionExhaustiveness) resultShapeCurrentTypeProvesOther(result *body.Result, point cfg.Point, env guardEnv, expr ast.Expr, discriminantSuffix []segment.Segment, required discriminantCase) bool {
	if required.literal == nil {
		return false
	}
	current, ok := newStructuralFlowExpressionTyper(result, p.resolver, point, env).typeOf(expr)
	if !ok || current == nil {
		return false
	}
	field, ok := variant.FieldAtPath(current, discriminantSuffix)
	if !ok {
		return false
	}
	lit, ok := unwrap.Annotated(field).(*typ.Literal)
	return ok && !typ.TypeEquals(lit, required.literal)
}

type literalDiscriminantDomain struct {
	target pathdom.Path
	suffix []segment.Segment
	cases  []discriminantCase
}

func literalDiscriminantDomainsForCases(receiver pathdom.Path, cases []variant.OriginCase) []literalDiscriminantDomain {
	if len(cases) == 0 {
		return nil
	}
	var out []literalDiscriminantDomain
	domains, ok := variant.LiteralDiscriminantDomainsForCases(cases)
	if !ok {
		return nil
	}
	for _, domain := range domains {
		suffix := domain.Suffix
		target := receiver.AppendSegments(suffix)
		domainCases, ok := literalDiscriminantCasesFor(target, suffix, cases)
		if !ok {
			continue
		}
		out = append(out, literalDiscriminantDomain{
			target: target,
			suffix: append([]segment.Segment(nil), suffix...),
			cases:  domainCases,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].target.String() < out[j].target.String()
	})
	return out
}

func literalDiscriminantCasesFor(target pathdom.Path, suffix []segment.Segment, cases []variant.OriginCase) ([]discriminantCase, bool) {
	out := make([]discriminantCase, 0, len(cases))
	var seen []typ.Type
	for _, c := range cases {
		lit, ok := discriminantCaseLiteral(c.Type, suffix)
		if !ok || !resultShapeLiteralSupported(lit) {
			return nil, false
		}
		for _, previous := range seen {
			if typ.TypeEquals(previous, lit) {
				return nil, false
			}
		}
		seen = append(seen, lit)
		out = append(out, discriminantCase{
			index:   c.Index,
			name:    discriminantCaseName(target, suffix, c.Type),
			literal: lit,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].index < out[j].index
	})
	return out, true
}

func resultShapeLiteralSupported(lit *typ.Literal) bool {
	if lit == nil {
		return false
	}
	switch lit.Value.(type) {
	case bool, string:
		return true
	default:
		return false
	}
}

func resultShapeRequiredCaseProven(result *body.Result, point cfg.Point, env guardEnv, discriminant pathdom.Path, required discriminantCase) bool {
	if required.literal == nil {
		return false
	}
	return guardEnvProvesLiteral(result, point, env, discriminant, required.literal)
}

func resultShapeOtherCaseProven(result *body.Result, point cfg.Point, env guardEnv, discriminant pathdom.Path, required discriminantCase) bool {
	if required.literal == nil {
		return false
	}
	for _, proof := range literalGuardTargets(env) {
		if typ.TypeEquals(proof.literal, required.literal) {
			continue
		}
		if proof.target.Equal(discriminant) || resultShapeDiscriminantsEquivalent(result, point, proof.target, discriminant) {
			return true
		}
	}
	return false
}

func guardEnvProvesLiteral(result *body.Result, point cfg.Point, env guardEnv, target pathdom.Path, lit typ.Type) bool {
	if typ.TypeEquals(lit, typ.True) && env.hasTruthy(target) {
		return true
	}
	if typ.TypeEquals(lit, typ.False) && env.hasFalsy(target) {
		return true
	}
	for _, c := range env.constraints {
		if !c.target.Equal(target) {
			continue
		}
		if !c.negated && typ.TypeEquals(c.value, lit) {
			return true
		}
		if c.negated && typ.TypeEquals(lit, typ.True) && typ.TypeEquals(c.value, typ.False) {
			return true
		}
		if c.negated && typ.TypeEquals(lit, typ.False) && typ.TypeEquals(c.value, typ.True) {
			return true
		}
	}
	for _, candidate := range equivalentLiteralGuardTargets(env, lit) {
		if candidate.Equal(target) {
			continue
		}
		if resultShapeDiscriminantsEquivalent(result, point, candidate, target) {
			return true
		}
	}
	return false
}

type literalGuardTarget struct {
	target  pathdom.Path
	literal typ.Type
}

func literalGuardTargets(env guardEnv) []literalGuardTarget {
	var out []literalGuardTarget
	for _, p := range env.truthy {
		out = append(out, literalGuardTarget{target: p.Clone(), literal: typ.True})
	}
	for _, p := range env.falsy {
		out = append(out, literalGuardTarget{target: p.Clone(), literal: typ.False})
	}
	for _, c := range env.constraints {
		if !c.negated {
			out = append(out, literalGuardTarget{target: c.target.Clone(), literal: c.value})
			continue
		}
		if typ.TypeEquals(c.value, typ.True) {
			out = append(out, literalGuardTarget{target: c.target.Clone(), literal: typ.False})
			continue
		}
		if typ.TypeEquals(c.value, typ.False) {
			out = append(out, literalGuardTarget{target: c.target.Clone(), literal: typ.True})
		}
	}
	return out
}

func equivalentLiteralGuardTargets(env guardEnv, lit typ.Type) []pathdom.Path {
	var out []pathdom.Path
	for _, proof := range literalGuardTargets(env) {
		if typ.TypeEquals(proof.literal, lit) {
			out = append(out, proof.target)
		}
	}
	return out
}

func resultShapeDiscriminantsEquivalent(result *body.Result, point cfg.Point, left, right pathdom.Path) bool {
	if result == nil || left.IsEmpty() || right.IsEmpty() || len(left.Segments) != len(right.Segments) {
		return false
	}
	if result.PathsEquivalentAtBoundary(point, left, right) {
		return true
	}
	if len(left.Segments) == 0 {
		return false
	}
	leftRoot := left.RootOnly()
	rightRoot := right.RootOnly()
	return sameSegments(left.Segments, right.Segments) &&
		pathsShareExactIdentity(result, point, leftRoot, rightRoot)
}
