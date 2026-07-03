package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

type directFunctionContract struct {
	name         string
	declSpan     ast.Span
	source       *typ.Function
	genericTrace typecall.GenericCallTrace
	params       []directCallParam
	returns      []directCallResult
	variadic     directCallParam
	hasVararg    bool
}

type directCallParam struct {
	typ          typ.Type
	display      string
	declSpan     ast.Span
	explicit     bool
	optional     bool
	implicitSelf bool
}

type directCallResult struct {
	typ       typ.Type
	declSpan  ast.Span
	declLabel string
	explicit  bool
}

func (c directFunctionContract) paramDeclSpan(index int) ast.Span {
	if index >= 0 && index < len(c.params) {
		return c.params[index].declSpan
	}
	if c.hasVararg {
		return c.variadic.declSpan
	}
	return ast.Span{}
}

func lowerDirectFunctionContract(fn *ast.FunctionExpr, resolver typeannotation.Resolver) (directFunctionContract, bool) {
	if fn == nil {
		return directFunctionContract{}, false
	}
	contract := directFunctionContract{
		params:  make([]directCallParam, 0),
		returns: make([]directCallResult, 0),
	}
	if fnType, ok := lowerFunctionExprType(fn, resolver); ok {
		contract.source = fnType
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
	fillFunctionReturnDeclarationSpans(&contract, fn)
	return contract, true
}

func lowerDirectFunctionContractForResult(result *body.Result, fn *ast.FunctionExpr, resolver typeannotation.Resolver) (directFunctionContract, bool) {
	if result != nil {
		if slots := result.FunctionParamSlots(fn); len(slots) != 0 {
			return lowerDirectFunctionContractFromParamSlots(fn, slots, resolver)
		}
		if owner := functionResultForExpr(result, fn); owner != nil && owner != result {
			if slots := owner.FunctionParamSlots(fn); len(slots) != 0 {
				return lowerDirectFunctionContractFromParamSlots(fn, slots, resolver)
			}
		}
	}
	return lowerDirectFunctionContract(fn, resolver)
}

func lowerDirectFunctionContractFromParamSlots(fn *ast.FunctionExpr, slots []bind.ParamSlot, resolver typeannotation.Resolver) (directFunctionContract, bool) {
	if fn == nil {
		return directFunctionContract{}, false
	}
	contract := directFunctionContract{
		params:  make([]directCallParam, 0, len(slots)),
		returns: make([]directCallResult, 0, len(fn.ReturnTypes)),
	}
	if fnType, ok := lowerFunctionExprType(fn, resolver); ok {
		contract.source = fnType
	}
	for _, slot := range slots {
		param, ok := lowerDirectCallParam(slot.Type, resolver)
		if !ok {
			return directFunctionContract{}, false
		}
		if slot.ImplicitSelf {
			param.implicitSelf = true
			if slot.Type == nil {
				param.explicit = true
				param.optional = false
			}
		}
		if slot.Vararg {
			contract.hasVararg = true
			contract.variadic = param
			continue
		}
		contract.params = append(contract.params, param)
	}
	for _, retExpr := range fn.ReturnTypes {
		ret, ok := lowerDirectCallResult(retExpr, resolver)
		if !ok {
			return directFunctionContract{}, false
		}
		contract.returns = append(contract.returns, ret)
	}
	fillFunctionReturnDeclarationSpans(&contract, fn)
	return contract, true
}

func lowerDirectFunctionContractInResultScope(result *body.Result, fn *ast.FunctionExpr, resolver typeannotation.Resolver) (directFunctionContract, bool) {
	for current := resolver; current != nil; current = parentTypeResolver(current) {
		if contract, ok := lowerDirectFunctionContractForResult(result, fn, current); ok {
			return contract, true
		}
	}
	return lowerDirectFunctionContractForResult(result, fn, nil)
}

func parentTypeResolver(resolver typeannotation.Resolver) typeannotation.Resolver {
	if r, ok := resolver.(*resultResolver); ok {
		return r.parent
	}
	return nil
}

func lowerDirectFunctionType(fn *typ.Function) directFunctionContract {
	contract := directFunctionContract{
		source:  fn,
		params:  make([]directCallParam, 0, len(fn.Params)),
		returns: make([]directCallResult, 0, len(fn.Returns)),
	}
	for _, param := range fn.Params {
		optional := param.Optional || isOptionalType(param.Type)
		explicit := directContractTypeExplicit(param.Type)
		if !explicit {
			optional = true
		}
		contract.params = append(contract.params, directCallParam{
			typ:          param.Type,
			display:      "",
			explicit:     explicit,
			optional:     optional,
			implicitSelf: param.Name == "self",
		})
	}
	for _, ret := range fn.Returns {
		contract.returns = append(contract.returns, directCallResult{
			typ:      ret,
			explicit: directContractTypeExplicit(ret),
		})
	}
	if fn.Variadic != nil {
		contract.hasVararg = true
		contract.variadic = directCallParam{
			typ:      fn.Variadic,
			display:  "",
			explicit: directContractTypeExplicit(fn.Variadic),
			optional: isOptionalType(fn.Variadic) || typ.IsAny(fn.Variadic) || typ.IsUnknown(fn.Variadic),
		}
	}
	return contract
}

func lowerFunctionExprType(fn *ast.FunctionExpr, resolver typeannotation.Resolver) (*typ.Function, bool) {
	if fn == nil {
		return nil, false
	}
	expr := &ast.FunctionTypeExpr{
		TypeParams: fn.TypeParams,
		Returns:    fn.ReturnTypes,
	}
	if fn.ParList != nil {
		expr.Params = make([]ast.FunctionParamExpr, 0, len(fn.ParList.Names))
		for i, name := range fn.ParList.Names {
			t := typeExprAt(fn.ParList.Types, i)
			if t == nil {
				return nil, false
			}
			expr.Params = append(expr.Params, ast.FunctionParamExpr{Name: name, Type: t})
		}
		if fn.ParList.HasVargs {
			if fn.ParList.VarargType == nil {
				return nil, false
			}
			expr.Variadic = fn.ParList.VarargType
		}
	}
	lowered, ok := lowerType(expr, resolver)
	if !ok {
		return nil, false
	}
	fnType, ok := lowered.(*typ.Function)
	return fnType, ok
}

func lowerDirectCallResult(expr ast.TypeExpr, resolver typeannotation.Resolver) (directCallResult, bool) {
	if expr == nil {
		return directCallResult{}, true
	}
	t, ok := lowerType(expr, resolver)
	if !ok {
		return directCallResult{}, false
	}
	return directCallResult{
		typ:       t,
		declSpan:  ast.SpanOf(expr),
		declLabel: labelDeclaredReturn,
		explicit:  !typ.IsAny(t) && !typ.IsUnknown(t),
	}, true
}

func fillFunctionReturnDeclarationSpans(contract *directFunctionContract, fn *ast.FunctionExpr) {
	if contract == nil || fn == nil || len(contract.returns) == 0 {
		return
	}
	fallback := functionDeclarationLineSpan(fn)
	if !fallback.Valid() {
		return
	}
	for i := range contract.returns {
		if contract.returns[i].declSpan.Valid() {
			continue
		}
		contract.returns[i].declSpan = fallback
		contract.returns[i].declLabel = labelCalleeDeclaration
	}
}

func functionDeclarationLineSpan(fn *ast.FunctionExpr) ast.Span {
	span := ast.SpanOf(fn)
	if !span.Valid() {
		return ast.Span{}
	}
	if span.EndLine != 0 && span.EndLine != span.StartLine {
		span.EndLine = span.StartLine
		span.EndCol = span.StartCol + len("function")
	}
	return span
}

func lowerDirectCallParam(expr ast.TypeExpr, resolver typeannotation.Resolver) (directCallParam, bool) {
	if expr == nil {
		return directCallParam{optional: true}, true
	}
	t, ok := lowerType(expr, resolver)
	if !ok {
		return directCallParam{}, false
	}
	explicit := directContractTypeExplicit(t)
	optional := !explicit || isOptionalType(t)
	return directCallParam{
		typ:      t,
		display:  display.AnnotationOrType(expr, t),
		declSpan: ast.SpanOf(expr),
		explicit: explicit,
		optional: optional,
	}, true
}

func directContractTypeExplicit(t typ.Type) bool {
	return t != nil && !typ.IsAny(t) && !typ.IsUnknown(t)
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
	ret, ok := c.returnResult(index)
	if !ok {
		return nil, false
	}
	return ret.returnType()
}

func (c directFunctionContract) declaredReturnType(index int) (typ.Type, bool) {
	ret, ok := c.returnResult(index)
	if !ok {
		return nil, false
	}
	return ret.declaredReturnType()
}

func (c directFunctionContract) returnResult(index int) (directCallResult, bool) {
	if index < 0 || index >= len(c.returns) {
		return directCallResult{}, false
	}
	return c.returns[index], true
}

func (r directCallResult) returnType() (typ.Type, bool) {
	if !r.explicit || r.typ == nil || typ.IsAny(r.typ) || typ.IsUnknown(r.typ) {
		return nil, false
	}
	return r.typ, true
}

func (r directCallResult) declaredReturnType() (typ.Type, bool) {
	return r.typ, r.typ != nil
}

func typeExprAt(exprs []ast.TypeExpr, index int) ast.TypeExpr {
	if index >= 0 && index < len(exprs) {
		return exprs[index]
	}
	return nil
}

func isOptionalType(t typ.Type) bool {
	return unwrap.IsOptionalLike(t)
}
