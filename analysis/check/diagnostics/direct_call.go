package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// DirectCallContract reports contract mismatches for direct function calls that
// resolve to a known callable contract.
type DirectCallContract Config

func (p DirectCallContract) Produce(result *check.Result) []diagnostic.Diagnostic {
	return produceDirectCallContract(result, Config(p), nil)
}

func produceDirectCallContract(result *check.Result, config Config, inherited map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	defs := directCallDefinitions(result, inherited)
	producer := DirectCallContract(config)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if !ok || fact.Call == nil {
			continue
		}
		if _, _, _, ok := callMemberAccess(fact); ok {
			continue
		}
		site, ok := result.CallSite(point)
		if !ok || site.CalleeSymbol() == 0 {
			continue
		}
		d, ok := producer.call(result, point, fact, site, defs[site.CalleeSymbol()])
		if !ok {
			continue
		}
		out = append(out, d)
	}
	return out
}

func (p DirectCallContract) call(
	result *check.Result,
	point cfg.Point,
	fact semantics.CallFact,
	site factflow.CallSite,
	def *ast.FunctionExpr,
) (diagnostic.Diagnostic, bool) {
	name := result.SymbolName(site.CalleeSymbol())
	if name == "" {
		name = "call target"
	}

	if def != nil {
		return p.callFunction(point, fact, name, def)
	}

	baseExpr, ok := result.SymbolTypeAnnotation(site.CalleeSymbol())
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	baseType, ok := typeannotation.Type(baseExpr, p.Resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	if typ.IsAny(baseType) || typ.IsUnknown(baseType) {
		return diagnostic.Diagnostic{}, false
	}
	callable, ok := typecall.Callable(baseType)
	if !ok {
		return directNotCallableDiagnostic(point, fact.Call, name, baseType), true
	}
	contract := lowerDirectFunctionType(callable)
	contract.name = name
	contract.declSpan = ast.SpanOf(fact.Call)
	return p.directFunctionCall(point, fact.Call, contract)
}

type directFunctionContract struct {
	name      string
	declSpan  ast.Span
	params    []directCallParam
	returns   []directCallResult
	variadic  directCallParam
	hasVararg bool
}

type directCallParam struct {
	typ      typ.Type
	explicit bool
	optional bool
}

type directCallResult struct {
	typ      typ.Type
	explicit bool
}

func (p DirectCallContract) callFunction(
	point cfg.Point,
	fact semantics.CallFact,
	name string,
	fn *ast.FunctionExpr,
) (diagnostic.Diagnostic, bool) {
	contract, ok := lowerDirectFunctionContract(fn, p.Resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	contract.name = name
	contract.declSpan = ast.SpanOf(fn)
	return p.directFunctionCall(point, fact.Call, contract)
}

func (p DirectCallContract) directFunctionCall(
	point cfg.Point,
	call *ast.FuncCallExpr,
	contract directFunctionContract,
) (diagnostic.Diagnostic, bool) {
	args := call.Args
	required := contract.requiredArity()
	if len(args) < required {
		return tooFewArgsDiagnostic(point, call, contract.name, required, len(args), contract.declSpan), true
	}
	for i, arg := range args {
		var want typ.Type
		if i < len(contract.params) {
			param := contract.params[i]
			want = param.typ
			if !param.explicit || want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
				continue
			}
		} else if contract.hasVararg {
			want = contract.variadic.typ
			if !contract.variadic.explicit || want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
				continue
			}
		} else {
			break
		}
		got, ok := valueexpr.LiteralType(arg)
		if !ok {
			continue
		}
		if refinement.ContainsFreeTypeParam(want) {
			continue
		}
		if subtype.IsSubtype(got, want) {
			continue
		}
		return argTypeDiagnostic(point, call, contract.name, i, got, want, arg, contract.declSpan), true
	}
	return diagnostic.Diagnostic{}, false
}

func lowerDirectFunctionContract(fn *ast.FunctionExpr, resolver typeannotation.Resolver) (directFunctionContract, bool) {
	if fn == nil {
		return directFunctionContract{}, false
	}
	contract := directFunctionContract{
		params:  make([]directCallParam, 0),
		returns: make([]directCallResult, 0),
	}
	if fn.ParList != nil {
		contract.params = make([]directCallParam, 0, len(fn.ParList.Names))
		for i := range fn.ParList.Names {
			param, ok := lowerDirectCallParam(typeExprAt(fn.ParList.Types, i), resolver)
			if !ok {
				return directFunctionContract{}, false
			}
			contract.params = append(contract.params, param)
		}
		if fn.ParList.HasVargs {
			variadic, ok := lowerDirectCallParam(fn.ParList.VarargType, resolver)
			if !ok {
				return directFunctionContract{}, false
			}
			contract.hasVararg = true
			contract.variadic = variadic
		}
	}
	contract.returns = make([]directCallResult, 0, len(fn.ReturnTypes))
	for _, retExpr := range fn.ReturnTypes {
		ret, ok := lowerDirectCallResult(retExpr, resolver)
		if !ok {
			return directFunctionContract{}, false
		}
		contract.returns = append(contract.returns, ret)
	}
	return contract, true
}

func lowerDirectFunctionType(fn *typ.Function) directFunctionContract {
	contract := directFunctionContract{
		params:  make([]directCallParam, 0, len(fn.Params)),
		returns: make([]directCallResult, 0, len(fn.Returns)),
	}
	for _, param := range fn.Params {
		optional := param.Optional || isOptionalType(param.Type)
		explicit := param.Type != nil && !typ.IsAny(param.Type) && !typ.IsUnknown(param.Type)
		if !explicit {
			optional = true
		}
		contract.params = append(contract.params, directCallParam{
			typ:      param.Type,
			explicit: explicit,
			optional: optional,
		})
	}
	for _, ret := range fn.Returns {
		contract.returns = append(contract.returns, directCallResult{
			typ:      ret,
			explicit: ret != nil && !typ.IsAny(ret) && !typ.IsUnknown(ret),
		})
	}
	if fn.Variadic != nil {
		contract.hasVararg = true
		contract.variadic = directCallParam{
			typ:      fn.Variadic,
			explicit: !typ.IsAny(fn.Variadic) && !typ.IsUnknown(fn.Variadic),
			optional: isOptionalType(fn.Variadic) || typ.IsAny(fn.Variadic) || typ.IsUnknown(fn.Variadic),
		}
	}
	return contract
}

func lowerDirectCallResult(expr ast.TypeExpr, resolver typeannotation.Resolver) (directCallResult, bool) {
	if expr == nil {
		return directCallResult{}, true
	}
	t, ok := typeannotation.Type(expr, resolver)
	if !ok {
		return directCallResult{}, false
	}
	return directCallResult{
		typ:      t,
		explicit: !typ.IsAny(t) && !typ.IsUnknown(t),
	}, true
}

func lowerDirectCallParam(expr ast.TypeExpr, resolver typeannotation.Resolver) (directCallParam, bool) {
	if expr == nil {
		return directCallParam{optional: true}, true
	}
	t, ok := typeannotation.Type(expr, resolver)
	if !ok {
		return directCallParam{}, false
	}
	explicit := !typ.IsAny(t) && !typ.IsUnknown(t)
	optional := !explicit || isOptionalType(t)
	return directCallParam{
		typ:      t,
		explicit: explicit,
		optional: optional,
	}, true
}

func (c directFunctionContract) requiredArity() int {
	required := 0
	for i, param := range c.params {
		if param.explicit && !param.optional {
			required = i + 1
		}
	}
	return required
}

func (c directFunctionContract) returnType(index int) (typ.Type, bool) {
	if index < 0 || index >= len(c.returns) {
		return nil, false
	}
	ret := c.returns[index]
	if !ret.explicit || ret.typ == nil || typ.IsAny(ret.typ) || typ.IsUnknown(ret.typ) {
		return nil, false
	}
	return ret.typ, true
}

func typeExprAt(exprs []ast.TypeExpr, index int) ast.TypeExpr {
	if index >= 0 && index < len(exprs) {
		return exprs[index]
	}
	return nil
}

func isOptionalType(t typ.Type) bool {
	if t == nil {
		return false
	}
	_, ok := unwrap.Annotated(t).(*typ.Optional)
	return ok
}

func directNotCallableDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, calleeType typ.Type) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeDirectCallNotCallable,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("%s is %s, not callable", name, formatType(calleeType)),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("call at CFG point %d resolves to %s", point, name),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    span,
				Message: fmt.Sprintf("%s is annotated %s", name, formatType(calleeType)),
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "non-callable call target"}},
	}
}

func tooFewArgsDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, want, got int, declSpan ast.Span) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeDirectCallTooFewArgs,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("%s expects %d arguments, got %d", name, want, got),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("call at CFG point %d passes %d arguments", point, got),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    declSpan,
				Message: fmt.Sprintf("%s declares %d parameters", name, want),
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "too few arguments"}},
	}
}

func argTypeDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, index int, got, want typ.Type, arg ast.Expr, declSpan ast.Span) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	argSpan := ast.SpanOf(arg)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeDirectCallArgType,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("argument %d is %s, not %s", index+1, formatType(got), formatType(want)),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    argSpan,
				Message: fmt.Sprintf("argument %d is %s", index+1, formatType(got)),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    declSpan,
				Message: fmt.Sprintf("%s parameter %d declares %s", name, index+1, formatType(want)),
			},
		),
		Labels: []diagnostic.Label{{Span: argSpan, Message: "argument value"}},
	}
}

func directCallDefinitions(result *check.Result, parent map[symbol.ID]*ast.FunctionExpr) map[symbol.ID]*ast.FunctionExpr {
	graph := result.Graph()
	if graph == nil {
		return parent
	}
	var out map[symbol.ID]*ast.FunctionExpr
	if len(parent) != 0 {
		out = make(map[symbol.ID]*ast.FunctionExpr, len(parent))
		for id, fn := range parent {
			out[id] = fn
		}
	}
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
			if fn, ok := fact.Expr.(*ast.FunctionExpr); ok {
				if out == nil {
					out = make(map[symbol.ID]*ast.FunctionExpr)
				}
				out[fact.Symbol] = fn
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
			if fn, ok := fact.Value.(*ast.FunctionExpr); ok {
				if out == nil {
					out = make(map[symbol.ID]*ast.FunctionExpr)
				}
				out[fact.Symbol] = fn
			}
		}
		fact, ok := result.FunctionDefinition(point)
		if !ok || !fact.HasTargetSymbol || fact.TargetSymbol == 0 || fact.Func == nil {
			continue
		}
		if out == nil {
			out = make(map[symbol.ID]*ast.FunctionExpr)
		}
		out[fact.TargetSymbol] = fact.Func
	}
	if len(out) == 0 {
		return parent
	}
	return out
}
