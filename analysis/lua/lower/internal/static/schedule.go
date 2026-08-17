package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ScheduleAlias lowers one source-classified type alias. It publishes the
// predeclared identity at its authored Body turn before traversing children.
func (w *Writer) ScheduleAlias(def *ast.TypeDefStmt, body keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if def == nil || body == 0 || body != w.scopes.Owner() || span.File == "" || span != w.span(def) {
		return fmt.Errorf("lualower: nil type alias")
	}
	alias, err := w.BeginAlias(def)
	if err != nil {
		return err
	}
	if err := w.scopes.Append(alias); err != nil {
		return err
	}
	w.push(walkStep{
		kind:       aliasConstraintsWalk,
		alias:      def,
		typeHost:   alias,
		typeParams: w.binding.TypeDefParams(def),
		body:       body,
		span:       span,
	})
	return nil
}

// ScheduleInterface lowers one source-classified interface declaration.
func (w *Writer) ScheduleInterface(def *ast.InterfaceDefStmt, body keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if def == nil || body == 0 || body != w.scopes.Owner() || span.File == "" || span != w.span(def) {
		return fmt.Errorf("lualower: nil interface declaration")
	}
	iface, err := w.BeginInterface(def)
	if err != nil {
		return err
	}
	if err := w.scopes.Append(iface); err != nil {
		return err
	}
	w.push(walkStep{
		kind:       interfaceExtendsWalk,
		iface:      def,
		typeBase:   iface,
		staticMark: w.Mark(),
		mark:       w.InterfaceMemberMark(),
		body:       body,
		span:       span,
	})
	return nil
}

// ScheduleType starts one complete authored static type walk. Its result is
// always published closed, including when a child expression itself is an
// open call or vararg producer.
func (w *Writer) ScheduleType(expr ast.TypeExpr, host, body keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if !validTypeExpr(expr) || host == 0 || body == 0 || body != w.scopes.Owner() || span.File == "" || span != w.typeSpan(expr) {
		return fmt.Errorf("lualower: invalid static type schedule")
	}
	return w.scheduleType(expr, host, body, span)
}

func (w *Writer) scheduleType(expr ast.TypeExpr, host, body keyspace.Term, span source.Span) error {
	if !validTypeExpr(expr) || host == 0 || body == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid static type continuation")
	}
	w.push(walkStep{kind: typeWalk, typeExpr: expr, typeHost: host, body: body, span: span})
	return nil
}

func (w *Writer) scheduleAnnotations(
	annotations []ast.AnnotationExpr,
	scope, target, body keyspace.Term,
	span source.Span,
) error {
	if scope == 0 || target == 0 || body == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid annotation continuation")
	}
	if len(annotations) == 0 {
		w.phases.SetResult(target, false)
		return nil
	}
	w.push(walkStep{
		kind: annotationsWalk, annotations: annotations, typeHost: scope, typeBase: target,
		body: body, span: span,
	})
	return nil
}

// ScheduleDeclaredCellType lowers one authored Cell type and attaches the
// resulting closed static Term to that Cell. Lexical owns Cell visibility, but
// it never writes a static relation itself.
func (w *Writer) ScheduleDeclaredCellType(expr ast.TypeExpr, cell, body keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if !validTypeExpr(expr) || cell == 0 || body == 0 || body != w.scopes.Owner() || span.File == "" || span != w.typeSpan(expr) {
		return fmt.Errorf("lualower: invalid declared Cell type schedule")
	}
	w.push(walkStep{kind: finishDeclaredCellTypeWalk, typeExpr: expr, typeHost: cell, body: body, span: span})
	return w.scheduleType(expr, cell, body, span)
}

// Run executes exactly one static-private continuation selected from the
// shared phase stack. Runtime operands use the narrow expression inbox or
// direct Values authority; no callback, second work stack, or shadow result
// is introduced.
func (w *Writer) Run() error {
	if err := w.ready(); err != nil {
		return err
	}
	if len(w.steps) == 0 {
		return fmt.Errorf("lualower: missing static continuation")
	}
	last := len(w.steps) - 1
	current := w.steps[last]
	w.steps = w.steps[:last]
	if current.body == 0 || current.body != w.scopes.Owner() {
		return fmt.Errorf("lualower: static continuation Body is not active")
	}

	switch current.kind {
	case aliasConstraintsWalk:
		return w.runAliasConstraints(current)
	case finishAliasWalk:
		return w.finishAlias(current)
	case interfaceExtendsWalk:
		return w.runInterfaceExtends(current)
	case interfaceMembersWalk:
		return w.runInterfaceMembers(current)
	case typeWalk:
		return w.runType(current)
	case finishAnnotatedWalk:
		expr, ok := current.node.(*ast.AnnotatedTypeExpr)
		if !ok || expr == nil || expr.Inner == nil {
			return fmt.Errorf("lualower: invalid annotated type continuation")
		}
		return w.scheduleAnnotations(expr.Annotations, current.typeHost, w.result(), current.body, w.annotationsSpan(expr.Annotations, current.span))
	case typeListWalk:
		return w.runTypeList(current)
	case appendTypeWalk:
		term, _ := w.phases.Result()
		return w.Append(term)
	case finishOptionalWalk:
		expr, ok := current.node.(*ast.OptionalTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid optional type continuation")
		}
		term, err := w.Optional(expr, w.result())
		return w.publish(term, err)
	case finishUnionWalk:
		expr, ok := current.node.(*ast.UnionTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid union type continuation")
		}
		term, err := w.Union(expr, current.staticMark, len(expr.Types))
		return w.publish(term, err)
	case finishIntersectionWalk:
		expr, ok := current.node.(*ast.IntersectionTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid intersection type continuation")
		}
		term, err := w.Intersection(expr, current.staticMark, len(expr.Types))
		return w.publish(term, err)
	case finishGenericBaseWalk:
		expr, ok := current.node.(*ast.GenericTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid generic base continuation")
		}
		if err := w.Append(w.result()); err != nil {
			return err
		}
		base, err := w.Take(current.staticMark)
		if err != nil {
			return err
		}
		mark := w.Mark()
		w.push(walkStep{kind: finishGenericWalk, node: expr, typeBase: base, staticMark: mark, body: current.body, span: current.span})
		w.push(walkStep{kind: typeListWalk, types: expr.Args, typeHost: current.typeHost, staticMark: mark, body: current.body, span: current.span})
		return nil
	case finishGenericWalk:
		expr, ok := current.node.(*ast.GenericTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid generic type continuation")
		}
		term, err := w.Generic(expr, current.typeBase, current.staticMark, len(expr.Args))
		return w.publish(term, err)
	case finishTypeOfWalk:
		expr, ok := current.node.(*ast.TypeOfExpr)
		if !ok || expr == nil || w.staticDepth <= 0 {
			return fmt.Errorf("lualower: invalid typeof continuation")
		}
		w.staticDepth--
		term, err := w.TypeOf(expr, current.typeHost, w.result())
		return w.publish(term, err)
	case finishKeyOfWalk:
		expr, ok := current.node.(*ast.KeyOfExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid keyof continuation")
		}
		term, err := w.KeyOf(expr, w.result())
		return w.publish(term, err)
	case indexChildrenWalk:
		return w.runIndexChildren(current)
	case finishIndexWalk:
		expr, ok := current.node.(*ast.IndexAccessExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid indexed-access continuation")
		}
		term, err := w.IndexAccess(expr, current.staticMark)
		return w.publish(term, err)
	case conditionalChildrenWalk:
		return w.runConditionalChildren(current)
	case finishConditionalWalk:
		expr, ok := current.node.(*ast.ConditionalTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid conditional continuation")
		}
		term, err := w.Conditional(expr, current.staticMark)
		return w.publish(term, err)
	case annotationsWalk:
		return w.runAnnotations(current)
	case finishAnnotationWalk:
		if w.staticDepth <= 0 {
			return fmt.Errorf("lualower: invalid annotation continuation")
		}
		w.staticDepth--
		values, open := w.phases.Result()
		if values == 0 || open {
			return fmt.Errorf("lualower: annotation arguments are not closed Values")
		}
		if err := w.FillAnnotation(current.annotation, values); err != nil {
			return err
		}
		w.phases.SetResult(current.typeBase, false)
		return nil
	case finishArrayWalk:
		expr, ok := current.node.(*ast.ArrayTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid array continuation")
		}
		term, err := w.Array(expr, w.result())
		return w.publish(term, err)
	case finishMapKeyWalk:
		expr, ok := current.node.(*ast.MapTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid map key continuation")
		}
		w.push(walkStep{kind: finishMapWalk, node: expr, typeBase: w.result(), body: current.body, span: current.span})
		return w.scheduleType(expr.Value, current.typeHost, current.body, w.typeSpan(expr.Value))
	case finishMapWalk:
		expr, ok := current.node.(*ast.MapTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid map continuation")
		}
		term, err := w.Map(expr, current.typeBase, w.result())
		return w.publish(term, err)
	case recordFieldsWalk:
		return w.runRecordFields(current)
	case finishFieldWalk:
		field, err := w.Field(current.field, w.result())
		if err != nil {
			return err
		}
		return w.AppendField(field)
	case finishInterfaceFieldWalk:
		field := ast.RecordFieldExpr{
			Name: current.member.Name, NamePosition: current.member.NamePosition,
			Type: current.member.Type, Optional: current.member.Optional,
		}
		term, err := w.Field(field, w.result())
		if err != nil {
			return err
		}
		return w.AppendInterfaceField(term)
	case appendInterfaceMethodWalk:
		return w.AppendInterfaceMethod(current.member.Name, current.member.NamePosition, w.result())
	case signatureGenericsWalk:
		return w.runSignatureGenerics(current)
	case signatureParamsWalk:
		return w.runSignatureParams(current)
	case finishSignatureVariadicWalk:
		expr, ok := current.node.(*ast.FunctionTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid function type variadic continuation")
		}
		w.push(walkStep{
			kind: signatureReturnsWalk, node: expr, typeBase: current.typeBase,
			variadic: w.result(), ordinal: current.ordinal,
			staticMark: current.staticMark, mark: w.Mark(), typeHost: current.typeHost,
			body: current.body, span: current.span,
		})
		return nil
	case signatureReturnsWalk:
		return w.runSignatureReturns(current)
	case finishSignatureWalk:
		expr, ok := current.node.(*ast.FunctionTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid function type continuation")
		}
		term, err := w.FinishSignature(expr, current.typeBase, current.staticMark, current.ordinal, current.mark, len(expr.Returns), current.variadic)
		return w.publish(term, err)
	case finishAssertionWalk:
		expr, ok := current.node.(*ast.AssertsTypeExpr)
		if !ok || expr == nil {
			return fmt.Errorf("lualower: invalid assertion type continuation")
		}
		term, err := w.Assertion(expr, w.result())
		return w.publish(term, err)
	case finishDeclaredCellTypeWalk:
		if err := w.DeclareCellTypeAt(current.typeHost, current.span, w.result()); err != nil {
			return err
		}
		return nil
	case finishParamWalk:
		return w.FinishParam(current.typeParam, w.result())
	default:
		return fmt.Errorf("lualower: unknown static continuation %d", current.kind)
	}
}
