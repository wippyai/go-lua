package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// DirectCallContract reports contract mismatches for direct function calls that
// resolve to a known callable contract.
type DirectCallContract Config

func (p DirectCallContract) Produce(result *check.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	defs := directCallDefinitions(result)
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
		d, ok := p.call(result, point, fact, site, defs[site.CalleeSymbol()])
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
	callable, ok := typeaccess.Callable(baseType)
	if !ok {
		return directNotCallableDiagnostic(point, fact.Call, name, baseType), true
	}
	contract := directFunctionContract{
		fn:         callable,
		name:       name,
		declSpan:   ast.SpanOf(fact.Call),
		paramTypes: nil,
	}
	return p.directFunctionCall(point, fact.Call, contract)
}

type directFunctionContract struct {
	fn         *typ.Function
	name       string
	declSpan   ast.Span
	paramTypes []ast.TypeExpr
	variadic   ast.TypeExpr
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
	required := minRequiredArgs(contract.fn)
	if len(args) < required {
		return tooFewArgsDiagnostic(point, call, contract.name, required, len(args), contract.declSpan), true
	}
	max := len(contract.fn.Params)
	for i, arg := range args {
		var want typ.Type
		var wantExpr ast.TypeExpr
		if i < len(contract.fn.Params) {
			want = contract.fn.Params[i].Type
			if i < len(contract.paramTypes) {
				wantExpr = contract.paramTypes[i]
			}
		} else if contract.fn.Variadic != nil {
			want = contract.fn.Variadic
			wantExpr = contract.variadic
		} else {
			break
		}
		if want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
			continue
		}
		got, ok := valueexpr.LiteralType(arg)
		if !ok {
			continue
		}
		if subtype.IsSubtype(got, want) {
			continue
		}
		return argTypeDiagnostic(point, call, contract.name, i, got, want, arg, wantExpr, contract.declSpan), true
	}
	if len(args) > max && contract.fn.Variadic == nil {
		return diagnostic.Diagnostic{}, false
	}
	return diagnostic.Diagnostic{}, false
}

func lowerDirectFunctionContract(fn *ast.FunctionExpr, resolver typeannotation.Resolver) (directFunctionContract, bool) {
	if fn == nil {
		return directFunctionContract{}, false
	}
	fnTypeExpr := &ast.FunctionTypeExpr{
		TypeParams: copyTypeParams(fn.TypeParams),
		Params:     make([]ast.FunctionParamExpr, 0),
		Returns:    functionReturnTypes(fn),
	}
	if fn.ParList != nil {
		fnTypeExpr.Params = make([]ast.FunctionParamExpr, len(fn.ParList.Names))
		for i := range fn.ParList.Names {
			paramType := typeExprOrUnknown(fn.ParList.Types, i)
			fnTypeExpr.Params[i] = ast.FunctionParamExpr{
				Name: fn.ParList.Names[i],
				Type: paramType,
			}
		}
		if fn.ParList.HasVargs {
			fnTypeExpr.Variadic = typeExprOrUnknown([]ast.TypeExpr{fn.ParList.VarargType}, 0)
		}
	}
	typType, ok := typeannotation.Type(fnTypeExpr, resolver)
	if !ok {
		return directFunctionContract{}, false
	}
	fnType, ok := typType.(*typ.Function)
	if !ok {
		return directFunctionContract{}, false
	}
	return directFunctionContract{
		fn:         fnType,
		paramTypes: functionParamTypes(fn),
		variadic:   functionVariadicType(fn),
	}, true
}

func functionParamTypes(fn *ast.FunctionExpr) []ast.TypeExpr {
	if fn == nil || fn.ParList == nil || len(fn.ParList.Types) == 0 {
		return nil
	}
	out := make([]ast.TypeExpr, len(fn.ParList.Names))
	for i := range out {
		out[i] = typeExprOrUnknown(fn.ParList.Types, i)
	}
	return out
}

func functionVariadicType(fn *ast.FunctionExpr) ast.TypeExpr {
	if fn == nil || fn.ParList == nil || !fn.ParList.HasVargs {
		return nil
	}
	return typeExprOrUnknown([]ast.TypeExpr{fn.ParList.VarargType}, 0)
}

func functionReturnTypes(fn *ast.FunctionExpr) []ast.TypeExpr {
	if fn == nil || len(fn.ReturnTypes) == 0 {
		return nil
	}
	out := make([]ast.TypeExpr, len(fn.ReturnTypes))
	for i := range out {
		out[i] = typeExprOrUnknown(fn.ReturnTypes, i)
	}
	return out
}

func typeExprOrUnknown(exprs []ast.TypeExpr, index int) ast.TypeExpr {
	if index >= 0 && index < len(exprs) && exprs[index] != nil {
		return exprs[index]
	}
	return &ast.PrimitiveTypeExpr{Name: "unknown"}
}

func copyTypeParams(in []ast.TypeParamExpr) []ast.TypeParamExpr {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.TypeParamExpr, len(in))
	copy(out, in)
	return out
}

func minRequiredArgs(fn *typ.Function) int {
	if fn == nil {
		return 0
	}
	required := 0
	for i, param := range fn.Params {
		if !param.Optional {
			required = i + 1
		}
	}
	return required
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

func argTypeDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, index int, got, want typ.Type, arg ast.Expr, wantExpr ast.TypeExpr, declSpan ast.Span) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	argSpan := ast.SpanOf(arg)
	paramSpan := declSpan
	if wantExpr != nil {
		paramSpan = ast.SpanOf(wantExpr)
	}
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
				Span:    paramSpan,
				Message: fmt.Sprintf("%s parameter %d declares %s", name, index+1, formatType(want)),
			},
		),
		Labels: []diagnostic.Label{{Span: argSpan, Message: "argument value"}},
	}
}

func directCallDefinitions(result *check.Result) map[symbol.ID]*ast.FunctionExpr {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	out := make(map[symbol.ID]*ast.FunctionExpr)
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
			if fn, ok := fact.Expr.(*ast.FunctionExpr); ok {
				out[fact.Symbol] = fn
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
			if fn, ok := fact.Value.(*ast.FunctionExpr); ok {
				out[fact.Symbol] = fn
			}
		}
		fact, ok := result.FunctionDefinition(point)
		if !ok || !fact.HasTargetSymbol || fact.TargetSymbol == 0 || fact.Func == nil {
			continue
		}
		out[fact.TargetSymbol] = fact.Func
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
