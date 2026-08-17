package static

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
)

func (w *Writer) runType(current walkStep) error {
	if current.typeExpr == nil || current.typeHost == 0 {
		return fmt.Errorf("lualower: absent static type expression")
	}
	switch expr := current.typeExpr.(type) {
	case *ast.AnnotatedTypeExpr:
		if expr.Inner == nil {
			return fmt.Errorf("lualower: annotated type has no inner type")
		}
		w.push(walkStep{kind: finishAnnotatedWalk, node: expr, typeHost: current.typeHost, body: current.body, span: current.span})
		return w.scheduleType(expr.Inner, current.typeHost, current.body, w.typeSpan(expr.Inner))
	case *ast.FunctionTypeExpr:
		fixed, variadic, err := FunctionTypeShape(expr)
		if err != nil {
			return err
		}
		signature, params, err := w.BeginSignature(expr, current.typeHost)
		if err != nil {
			return err
		}
		w.push(walkStep{kind: signatureGenericsWalk, node: expr, typeBase: signature, typeExpr: variadic, typeParams: params, ordinal: fixed, typeHost: signature, body: current.body, span: current.span})
		return nil
	case *ast.AssertsTypeExpr:
		if expr.NarrowTo == nil {
			term, err := w.Assertion(expr, 0)
			return w.publish(term, err)
		}
		w.push(walkStep{kind: finishAssertionWalk, node: expr, body: current.body, span: current.span})
		return w.scheduleType(expr.NarrowTo, current.typeHost, current.body, w.typeSpan(expr.NarrowTo))
	case *ast.ArrayTypeExpr:
		w.push(walkStep{kind: finishArrayWalk, node: expr, typeHost: current.typeHost, body: current.body, span: current.span})
		return w.scheduleType(expr.Element, current.typeHost, current.body, w.typeSpan(expr.Element))
	case *ast.MapTypeExpr:
		w.push(walkStep{kind: finishMapKeyWalk, node: expr, typeHost: current.typeHost, body: current.body, span: current.span})
		return w.scheduleType(expr.Key, current.typeHost, current.body, w.typeSpan(expr.Key))
	case *ast.RecordTypeExpr:
		w.push(walkStep{kind: recordFieldsWalk, node: expr, mark: w.FieldMark(), typeHost: current.typeHost, body: current.body, span: current.span})
		return nil
	case *ast.OptionalTypeExpr:
		w.push(walkStep{kind: finishOptionalWalk, node: expr, body: current.body, span: current.span})
		return w.scheduleType(expr.Inner, current.typeHost, current.body, w.typeSpan(expr.Inner))
	case *ast.UnionTypeExpr:
		mark := w.Mark()
		w.push(walkStep{kind: finishUnionWalk, node: expr, staticMark: mark, body: current.body, span: current.span})
		w.push(walkStep{kind: typeListWalk, types: expr.Types, typeHost: current.typeHost, staticMark: mark, body: current.body, span: current.span})
		return nil
	case *ast.IntersectionTypeExpr:
		mark := w.Mark()
		w.push(walkStep{kind: finishIntersectionWalk, node: expr, staticMark: mark, body: current.body, span: current.span})
		w.push(walkStep{kind: typeListWalk, types: expr.Types, typeHost: current.typeHost, staticMark: mark, body: current.body, span: current.span})
		return nil
	case *ast.GenericTypeExpr:
		if expr.Base == nil {
			return fmt.Errorf("lualower: generic type has no base")
		}
		w.push(walkStep{kind: finishGenericBaseWalk, node: expr, typeHost: current.typeHost, staticMark: w.Mark(), body: current.body, span: current.span})
		return w.scheduleType(expr.Base, current.typeHost, current.body, w.typeSpan(expr.Base))
	case *ast.TypeOfExpr:
		if expr.Expr == nil {
			return fmt.Errorf("lualower: typeof has no expression")
		}
		w.staticDepth++
		w.push(walkStep{kind: finishTypeOfWalk, node: expr, typeHost: current.typeHost, body: current.body, span: current.span})
		return w.expression(expr.Expr, current.body, w.expressionSpan(expr.Expr))
	case *ast.KeyOfExpr:
		w.push(walkStep{kind: finishKeyOfWalk, node: expr, body: current.body, span: current.span})
		return w.scheduleType(expr.Inner, current.typeHost, current.body, w.typeSpan(expr.Inner))
	case *ast.IndexAccessExpr:
		mark := w.Mark()
		w.push(walkStep{kind: finishIndexWalk, node: expr, staticMark: mark, body: current.body, span: current.span})
		w.push(walkStep{kind: indexChildrenWalk, node: expr, staticMark: mark, typeHost: current.typeHost, body: current.body, span: current.span})
		return nil
	case *ast.ConditionalTypeExpr:
		mark := w.Mark()
		w.push(walkStep{kind: finishConditionalWalk, node: expr, staticMark: mark, body: current.body, span: current.span})
		w.push(walkStep{kind: conditionalChildrenWalk, node: expr, staticMark: mark, typeHost: current.typeHost, body: current.body, span: current.span})
		return nil
	default:
		term, found, err := w.leaf(expr)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("lualower: unsupported type expression %T", expr)
		}
		return w.publish(term, err)
	}
}

func (w *Writer) runTypeList(current walkStep) error {
	if current.index < 0 || current.index > len(current.types) {
		return fmt.Errorf("lualower: invalid static type-list cursor")
	}
	if current.index == len(current.types) {
		return nil
	}
	expr := current.types[current.index]
	if expr == nil {
		return fmt.Errorf("lualower: absent static type expression at index %d", current.index)
	}
	current.index++
	w.push(current)
	w.push(walkStep{kind: appendTypeWalk, body: current.body, span: current.span})
	return w.scheduleType(expr, current.typeHost, current.body, w.typeSpan(expr))
}

func (w *Writer) runIndexChildren(current walkStep) error {
	expr, ok := current.node.(*ast.IndexAccessExpr)
	if !ok || expr == nil || current.index < 0 || current.index > 2 || current.staticMark < 0 || w.Mark() != current.staticMark+current.index {
		return fmt.Errorf("lualower: invalid indexed-access child cursor")
	}
	if current.index == 2 {
		return nil
	}
	child := expr.Object
	if current.index == 1 {
		child = expr.Index
	}
	if child == nil {
		return fmt.Errorf("lualower: absent indexed-access child")
	}
	current.index++
	w.push(current)
	w.push(walkStep{kind: appendTypeWalk, body: current.body, span: current.span})
	return w.scheduleType(child, current.typeHost, current.body, w.typeSpan(child))
}

func (w *Writer) runConditionalChildren(current walkStep) error {
	expr, ok := current.node.(*ast.ConditionalTypeExpr)
	if !ok || expr == nil || current.index < 0 || current.index > 4 || current.staticMark < 0 || w.Mark() != current.staticMark+current.index {
		return fmt.Errorf("lualower: invalid conditional type child cursor")
	}
	if current.index == 4 {
		return nil
	}
	children := [4]ast.TypeExpr{expr.Check, expr.Extends, expr.Then, expr.Else}
	child := children[current.index]
	if child == nil {
		return fmt.Errorf("lualower: absent conditional type child")
	}
	current.index++
	w.push(current)
	w.push(walkStep{kind: appendTypeWalk, body: current.body, span: current.span})
	return w.scheduleType(child, current.typeHost, current.body, w.typeSpan(child))
}

func (w *Writer) runAnnotations(current walkStep) error {
	if current.typeHost == 0 || current.typeBase == 0 || current.index < 0 || current.index > len(current.annotations) {
		return fmt.Errorf("lualower: invalid annotation cursor")
	}
	if current.index == len(current.annotations) {
		w.phases.SetResult(current.typeBase, false)
		return nil
	}
	annotation := current.annotations[current.index]
	term, err := w.DeclareAnnotation(annotation, current.typeHost, current.typeBase)
	if err != nil {
		return err
	}
	current.index++
	w.staticDepth++
	w.push(current)
	w.push(walkStep{kind: finishAnnotationWalk, annotation: term, typeBase: current.typeBase, body: current.body, span: w.span(&annotation)})
	return w.values(annotation.Args, current.body, w.span(&annotation))
}

func (w *Writer) runRecordFields(current walkStep) error {
	expr, ok := current.node.(*ast.RecordTypeExpr)
	if !ok || expr == nil || current.index < 0 || current.index > len(expr.Fields) {
		return fmt.Errorf("lualower: invalid record-field cursor")
	}
	if current.index == len(expr.Fields) {
		term, err := w.Record(expr, current.mark, len(expr.Fields))
		return w.publish(term, err)
	}
	field := expr.Fields[current.index]
	if field.Type == nil {
		return fmt.Errorf("lualower: absent record field type %d", current.index)
	}
	current.index++
	w.push(current)
	w.push(walkStep{kind: finishFieldWalk, field: field, typeHost: current.typeHost, body: current.body, span: current.span})
	return w.scheduleType(field.Type, current.typeHost, current.body, w.typeSpan(field.Type))
}
