package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// walkKind is private to the static vertical. The shared phase stack records
// only that Static runs next; it never becomes a second semantic instruction
// language for type syntax.
type walkKind uint8

const (
	aliasConstraintsWalk walkKind = iota + 1
	finishAliasWalk
	interfaceExtendsWalk
	interfaceMembersWalk
	typeWalk
	finishAnnotatedWalk
	typeListWalk
	appendTypeWalk
	finishOptionalWalk
	finishUnionWalk
	finishIntersectionWalk
	finishGenericBaseWalk
	finishGenericWalk
	finishTypeOfWalk
	finishKeyOfWalk
	indexChildrenWalk
	finishIndexWalk
	conditionalChildrenWalk
	finishConditionalWalk
	annotationsWalk
	finishAnnotationWalk
	finishArrayWalk
	finishMapKeyWalk
	finishMapWalk
	recordFieldsWalk
	finishFieldWalk
	finishInterfaceFieldWalk
	appendInterfaceMethodWalk
	signatureGenericsWalk
	signatureParamsWalk
	finishSignatureVariadicWalk
	signatureReturnsWalk
	finishSignatureWalk
	finishAssertionWalk
	finishDeclaredCellTypeWalk
	finishParamWalk
)

type walkStep struct {
	kind walkKind

	alias       *ast.TypeDefStmt
	iface       *ast.InterfaceDefStmt
	typeExpr    ast.TypeExpr
	types       []ast.TypeExpr
	typeParam   bind.TypeDecl
	typeParams  []bind.TypeDecl
	annotations []ast.AnnotationExpr
	field       ast.RecordFieldExpr
	member      ast.InterfaceMember
	node        ast.PositionHolder

	index      int
	ordinal    int
	mark       int
	staticMark int
	typeHost   keyspace.Term
	typeBase   keyspace.Term
	annotation keyspace.Term
	variadic   keyspace.Term
	body       keyspace.Term
	span       source.Span
}

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

func (w *Writer) runAliasConstraints(current walkStep) error {
	if current.alias == nil || current.index < 0 || current.index > len(current.typeParams) {
		return fmt.Errorf("lualower: invalid type parameter cursor")
	}
	if current.index == len(current.typeParams) {
		w.push(walkStep{kind: finishAliasWalk, alias: current.alias, body: current.body, span: current.span})
		return w.scheduleType(current.alias.Type, current.typeHost, current.body, w.typeSpan(current.alias.Type))
	}
	param := current.typeParams[current.index]
	if param.ID == 0 || param.Kind != bind.TypeDeclParam {
		return fmt.Errorf("lualower: invalid type parameter binding")
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
		return fmt.Errorf("lualower: type parameter was not predeclared")
	}
	w.push(current)
	w.push(walkStep{kind: finishParamWalk, typeParam: param, body: current.body, span: current.span})
	return w.scheduleType(param.Constraint, host, current.body, w.typeSpan(param.Constraint))
}

func (w *Writer) finishAlias(current walkStep) error {
	if current.alias == nil {
		return fmt.Errorf("lualower: invalid type alias completion")
	}
	return w.FinishAlias(current.alias, w.result())
}

func (w *Writer) runInterfaceExtends(current walkStep) error {
	if current.iface == nil || current.index < 0 || current.index > len(current.iface.Extends) {
		return fmt.Errorf("lualower: invalid interface extends cursor")
	}
	if current.index == len(current.iface.Extends) {
		w.push(walkStep{kind: interfaceMembersWalk, iface: current.iface, typeBase: current.typeBase, staticMark: current.staticMark, mark: current.mark, body: current.body, span: current.span})
		return nil
	}
	extend := current.iface.Extends[current.index]
	if extend == nil {
		return fmt.Errorf("lualower: absent interface extends reference at index %d", current.index)
	}
	current.index++
	w.push(current)
	w.push(walkStep{kind: appendTypeWalk, body: current.body, span: current.span})
	return w.scheduleType(extend, current.typeBase, current.body, w.typeSpan(extend))
}

func (w *Writer) runInterfaceMembers(current walkStep) error {
	if current.iface == nil || current.index < 0 || current.index > len(current.iface.Members) {
		return fmt.Errorf("lualower: invalid interface member cursor")
	}
	if current.index == len(current.iface.Members) {
		return w.FinishInterface(current.iface, current.staticMark, current.mark)
	}
	member := current.iface.Members[current.index]
	current.index++
	switch member.Kind {
	case ast.InterfaceFieldMember:
		if member.Name == "" || member.Type == nil {
			return fmt.Errorf("lualower: invalid interface field %d", current.index-1)
		}
		w.push(current)
		w.push(walkStep{kind: finishInterfaceFieldWalk, member: member, typeHost: current.typeBase, body: current.body, span: current.span})
		return w.scheduleType(member.Type, current.typeBase, current.body, w.typeSpan(member.Type))
	case ast.InterfaceMethodMember:
		if member.Name == "" {
			return fmt.Errorf("lualower: invalid interface method %d", current.index-1)
		}
		if _, ok := member.Type.(*ast.FunctionTypeExpr); !ok {
			return fmt.Errorf("lualower: interface method %q has non-function signature", member.Name)
		}
		w.push(current)
		w.push(walkStep{kind: appendInterfaceMethodWalk, member: member, body: current.body, span: current.span})
		return w.scheduleType(member.Type, current.typeBase, current.body, w.typeSpan(member.Type))
	default:
		return fmt.Errorf("lualower: invalid interface member kind %d", member.Kind)
	}
}

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

func (w *Writer) push(next walkStep) {
	w.steps = append(w.steps, next)
	w.phases.Push(continuation.Static)
}

func (w *Writer) expression(expr ast.Expr, body keyspace.Term, span source.Span) error {
	if !validRuntimeExpr(expr) || body == 0 || span.File == "" {
		return fmt.Errorf("lualower: absent static expression operand")
	}
	return w.expressions.Push(expr, body, span)
}

func (w *Writer) values(exprs []ast.Expr, body keyspace.Term, span source.Span) error {
	return w.evaluations.ScheduleValues(exprs, body, span)
}

func (w *Writer) result() keyspace.Term {
	term, _ := w.phases.Result()
	return term
}

func (w *Writer) publish(term keyspace.Term, err error) error {
	if err != nil {
		return err
	}
	w.phases.SetResult(term, false)
	return nil
}

func (w *Writer) ready() error {
	if w == nil || w.binding == nil || w.scopes == nil || w.phases == nil ||
		w.expressions == nil || w.evaluations == nil {
		return fmt.Errorf("lualower: static writer is not scheduled")
	}
	return nil
}

func (w *Writer) typeSpan(expr ast.TypeExpr) source.Span {
	if !validTypeExpr(expr) {
		return coord.Invalid(w.sourceName)
	}
	return w.span(expr)
}

func (w *Writer) annotationsSpan(annotations []ast.AnnotationExpr, fallback source.Span) source.Span {
	if len(annotations) == 0 {
		return fallback
	}
	return w.span(&annotations[0])
}

func (w *Writer) expressionSpan(expr ast.Expr) source.Span {
	if !validRuntimeExpr(expr) {
		return coord.Invalid(w.sourceName)
	}
	return w.span(expr)
}

func validTypeExpr(expr ast.TypeExpr) bool {
	switch node := expr.(type) {
	case *ast.AnnotatedTypeExpr:
		return node != nil && node.Inner != nil
	case *ast.PrimitiveTypeExpr:
		return node != nil
	case *ast.OptionalTypeExpr:
		return node != nil
	case *ast.UnionTypeExpr:
		return node != nil
	case *ast.IntersectionTypeExpr:
		return node != nil
	case *ast.ArrayTypeExpr:
		return node != nil
	case *ast.MapTypeExpr:
		return node != nil
	case *ast.RecordTypeExpr:
		return node != nil
	case *ast.FunctionTypeExpr:
		return node != nil
	case *ast.AssertsTypeExpr:
		return node != nil
	case *ast.TypeRefExpr:
		return node != nil
	case *ast.GenericTypeExpr:
		return node != nil
	case *ast.LiteralTypeExpr:
		return node != nil
	case *ast.TypeOfExpr:
		return node != nil
	case *ast.KeyOfExpr:
		return node != nil
	case *ast.IndexAccessExpr:
		return node != nil
	case *ast.ConditionalTypeExpr:
		return node != nil
	default:
		return false
	}
}

func validRuntimeExpr(expr ast.Expr) bool {
	switch node := expr.(type) {
	case *ast.TrueExpr:
		return node != nil
	case *ast.FalseExpr:
		return node != nil
	case *ast.NilExpr:
		return node != nil
	case *ast.NumberExpr:
		return node != nil
	case *ast.StringExpr:
		return node != nil
	case *ast.Comma3Expr:
		return node != nil
	case *ast.IdentExpr:
		return node != nil
	case *ast.AttrGetExpr:
		return node != nil
	case *ast.TableExpr:
		return node != nil
	case *ast.FuncCallExpr:
		return node != nil
	case *ast.LogicalOpExpr:
		return node != nil
	case *ast.RelationalOpExpr:
		return node != nil
	case *ast.StringConcatOpExpr:
		return node != nil
	case *ast.ArithmeticOpExpr:
		return node != nil
	case *ast.UnaryMinusOpExpr:
		return node != nil
	case *ast.UnaryNotOpExpr:
		return node != nil
	case *ast.UnaryLenOpExpr:
		return node != nil
	case *ast.UnaryBNotOpExpr:
		return node != nil
	case *ast.FunctionExpr:
		return node != nil
	case *ast.CastExpr:
		return node != nil
	case *ast.NonNilAssertExpr:
		return node != nil
	default:
		return false
	}
}
