package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (w *Writer) runSignatureGenerics(current walkStep) error {
	expr, ok := current.node.(*ast.FunctionTypeExpr)
	if !ok || expr == nil || current.index < 0 || current.index > len(current.typeParams) {
		return fmt.Errorf("lualower: invalid function type generic cursor")
	}
	if current.index == len(current.typeParams) {
		w.push(walkStep{kind: signatureParamsWalk, node: expr, typeBase: current.typeBase, typeExpr: current.typeExpr, ordinal: current.ordinal, staticMark: w.Mark(), typeHost: current.typeHost, body: current.body, span: current.span})
		return nil
	}
	param := current.typeParams[current.index]
	if param.ID == 0 || param.Kind != bind.TypeDeclParam || current.index >= len(expr.TypeParams) {
		return fmt.Errorf("lualower: invalid function type parameter binding")
	}
	current.index++
	if param.Constraint == nil {
		if err := w.FinishParam(param, 0); err != nil {
			return err
		}
		w.push(current)
		return nil
	}
	host, ok := w.Host(param)
	if !ok {
		return fmt.Errorf("lualower: function type parameter was not predeclared")
	}
	w.push(current)
	w.push(walkStep{kind: finishParamWalk, typeParam: param, body: current.body, span: current.span})
	return w.scheduleType(param.Constraint, host, current.body, w.typeSpan(param.Constraint))
}

func (w *Writer) runSignatureParams(current walkStep) error {
	expr, ok := current.node.(*ast.FunctionTypeExpr)
	if !ok || expr == nil || current.index < 0 || current.index > current.ordinal || current.ordinal > len(expr.Params) {
		return fmt.Errorf("lualower: invalid function type parameter cursor")
	}
	if current.index == current.ordinal {
		returnMark := w.Mark()
		if current.typeExpr == nil {
			w.push(walkStep{kind: signatureReturnsWalk, node: expr, typeBase: current.typeBase, ordinal: current.ordinal, staticMark: current.staticMark, mark: returnMark, typeHost: current.typeHost, body: current.body, span: current.span})
			return nil
		}
		w.push(walkStep{kind: finishSignatureVariadicWalk, node: expr, typeBase: current.typeBase, ordinal: current.ordinal, staticMark: current.staticMark, typeHost: current.typeHost, body: current.body, span: current.span})
		return w.scheduleType(current.typeExpr, current.typeHost, current.body, w.typeSpan(current.typeExpr))
	}
	param := expr.Params[current.index]
	if param.Type == nil {
		return fmt.Errorf("lualower: invalid function type fixed parameter")
	}
	current.index++
	w.push(current)
	w.push(walkStep{kind: appendTypeWalk, body: current.body, span: current.span})
	return w.scheduleType(param.Type, current.typeHost, current.body, w.typeSpan(param.Type))
}

func (w *Writer) runSignatureReturns(current walkStep) error {
	expr, ok := current.node.(*ast.FunctionTypeExpr)
	if !ok || expr == nil || current.index < 0 || current.index > len(expr.Returns) {
		return fmt.Errorf("lualower: invalid function type return cursor")
	}
	if current.index == len(expr.Returns) {
		w.push(walkStep{kind: finishSignatureWalk, node: expr, typeBase: current.typeBase, variadic: current.variadic, ordinal: current.ordinal, staticMark: current.staticMark, mark: current.mark, body: current.body, span: current.span})
		return nil
	}
	returnType := expr.Returns[current.index]
	if returnType == nil {
		return fmt.Errorf("lualower: absent function type return at index %d", current.index)
	}
	current.index++
	w.push(current)
	w.push(walkStep{kind: appendTypeWalk, body: current.body, span: current.span})
	return w.scheduleType(returnType, current.typeHost, current.body, w.typeSpan(returnType))
}
