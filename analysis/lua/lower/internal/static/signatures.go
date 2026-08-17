package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// BeginSignature reserves a source-only callable under scope, then declares
// its exact bound generic identities beneath that signature host. The returned
// declarations remain in source order for the iterative caller to fill their
// constraints through FinishParam.
func (w *Writer) BeginSignature(expr *ast.FunctionTypeExpr, scope keyspace.Term) (keyspace.Term, []bind.TypeDecl, error) {
	if w == nil || w.binding == nil || expr == nil {
		return 0, nil, fmt.Errorf("lualower: invalid function type")
	}
	if _, _, err := FunctionTypeShape(expr); err != nil {
		return 0, nil, err
	}
	params := w.binding.FunctionTypeParams(expr)
	if len(params) != len(expr.TypeParams) {
		return 0, nil, fmt.Errorf("lualower: missing function type parameter bindings")
	}
	signature := w.static.TypeFunction(w.span(expr), scope)
	if signature == 0 {
		return 0, nil, fmt.Errorf("lualower: could not declare function type")
	}
	if len(w.generics) != 0 {
		return 0, nil, fmt.Errorf("lualower: unfinished function-type generic scratch")
	}
	if len(params) != 0 && w.terms == nil {
		w.terms = make(map[bind.TypeDeclID]keyspace.Term)
	}
	for index, param := range params {
		if param.ID == 0 || param.Kind != bind.TypeDeclParam || param.Name == "" || param.Name != expr.TypeParams[index].Name {
			return 0, nil, fmt.Errorf("lualower: invalid function type parameter binding")
		}
		if _, exists := w.terms[param.ID]; exists {
			return 0, nil, fmt.Errorf("lualower: duplicate type parameter identity %q", param.Name)
		}
		term := w.static.TypeParam(w.nameSpan(param.NamePosition), signature, param.Name)
		if term == 0 {
			return 0, nil, fmt.Errorf("lualower: could not declare function type parameter %q", param.Name)
		}
		w.terms[param.ID] = term
		w.generics = append(w.generics, term)
	}
	if !w.static.TypeFunctionGenerics(signature, w.generics) {
		return 0, nil, fmt.Errorf("lualower: could not set function type parameters")
	}
	w.generics = w.generics[:0]
	return signature, params, nil
}

// BeginFunctionHeader adds generic declarations directly to an executable
// Function. It shares the same binder identity table and constraint completion
// path as aliases and source-only function types.
func (w *Writer) BeginFunctionHeader(expr *ast.FunctionExpr, function keyspace.Term) ([]bind.TypeDecl, error) {
	if w == nil || w.binding == nil || expr == nil || function == 0 {
		return nil, fmt.Errorf("lualower: invalid function header")
	}
	params := w.binding.FunctionTypeParams(expr)
	if len(params) != len(expr.TypeParams) {
		return nil, fmt.Errorf("lualower: missing function type parameter bindings")
	}
	if len(w.generics) != 0 {
		return nil, fmt.Errorf("lualower: unfinished function generic scratch")
	}
	if len(params) != 0 && w.terms == nil {
		w.terms = make(map[bind.TypeDeclID]keyspace.Term)
	}
	for index, param := range params {
		declared := expr.TypeParams[index]
		if param.ID == 0 || param.Kind != bind.TypeDeclParam || param.Name == "" || param.Name != declared.Name {
			return nil, fmt.Errorf("lualower: invalid function type parameter binding")
		}
		if _, exists := w.terms[param.ID]; exists {
			return nil, fmt.Errorf("lualower: duplicate type parameter identity %q", param.Name)
		}
		term := w.static.TypeParam(w.nameSpan(param.NamePosition), function, param.Name)
		if term == 0 {
			return nil, fmt.Errorf("lualower: could not declare function type parameter %q", param.Name)
		}
		w.terms[param.ID] = term
		w.generics = append(w.generics, term)
	}
	if !w.flow.SetFunctionGenerics(function, w.generics) {
		return nil, fmt.Errorf("lualower: could not set function type parameters")
	}
	w.generics = w.generics[:0]
	return params, nil
}

// FinishFunctionReturns records the exact runtime Function return clause.
func (w *Writer) FinishFunctionReturns(expr *ast.FunctionExpr, function keyspace.Term, mark, count int) error {
	if w == nil || expr == nil || count != len(expr.ReturnTypes) {
		return fmt.Errorf("lualower: invalid function return completion")
	}
	returns, err := w.rangeTerms(mark, count)
	if err != nil {
		return err
	}
	if !w.flow.SetFunctionReturns(function, expr.ReturnsKnown, returns) {
		return fmt.Errorf("lualower: could not finalize function returns")
	}
	return nil
}

// FinishSignature completes a source-only callable from fixed parameter and
// return child ranges accumulated by the iterative lowerer. Descriptor scratch
// belongs to Writer, so no per-signature parameter allocation is needed.
func (w *Writer) FinishSignature(expr *ast.FunctionTypeExpr, signature keyspace.Term, paramMark, fixedCount, returnMark, returnCount int, variadic keyspace.Term) (keyspace.Term, error) {
	if w == nil || expr == nil || returnCount != len(expr.Returns) {
		return 0, fmt.Errorf("lualower: invalid function type completion")
	}
	expectedFixed, expectedVariadic, err := FunctionTypeShape(expr)
	if err != nil || fixedCount != expectedFixed || (expectedVariadic == nil) != (variadic == 0) {
		return 0, fmt.Errorf("lualower: invalid function type variadic child")
	}
	returns, err := w.rangeTerms(returnMark, returnCount)
	if err != nil {
		return 0, err
	}
	fixed, err := w.rangeTerms(paramMark, fixedCount)
	if err != nil {
		return 0, err
	}
	if len(w.params) != 0 {
		return 0, fmt.Errorf("lualower: unfinished function-type parameter scratch")
	}
	for index, param := range expr.Params[:fixedCount] {
		name := ""
		nameSpan := source.Span{}
		if param.Name != "" {
			name = param.Name
			nameSpan = w.nameSpan(param.NamePosition)
		}
		w.params = append(w.params, assembly.StaticParameter{
			Name: name, Span: nameSpan, Type: fixed[index],
		})
	}
	variadicSpan := source.Span{}
	if variadic != 0 {
		variadicSpan = w.nameSpan(expr.VariadicPosition)
	}
	if !w.static.TypeFunctionParameters(signature, w.params) ||
		!w.static.TypeFunctionVariadic(signature, variadicSpan, variadic) ||
		!w.static.TypeFunctionReturns(signature, expr.Returns != nil, returns) {
		w.params = w.params[:0]
		return 0, fmt.Errorf("lualower: could not finalize function type")
	}
	w.params = w.params[:0]
	return signature, nil
}

// FunctionTypeShape validates the canonical source shape: fixed parameters are
// separate from one optional variadic tail.
func FunctionTypeShape(expr *ast.FunctionTypeExpr) (fixedCount int, variadic ast.TypeExpr, err error) {
	if expr == nil {
		return 0, nil, fmt.Errorf("lualower: nil function type")
	}
	fixedCount = len(expr.Params)
	for index, param := range expr.Params {
		if param.Type == nil {
			return 0, nil, fmt.Errorf("lualower: function type parameter %d has no type", index)
		}
	}
	if expr.Variadic == nil && expr.VariadicPosition != (ast.Position{}) {
		return 0, nil, fmt.Errorf("lualower: function type variadic position without variadic type")
	}
	if expr.Variadic != nil && !expr.VariadicPosition.Valid() {
		return 0, nil, fmt.Errorf("lualower: function type variadic has no marker position")
	}
	variadic = expr.Variadic
	return fixedCount, variadic, nil
}
