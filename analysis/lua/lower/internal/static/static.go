// Package static lowers parser-authored static type syntax into Program.
package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/lexical"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Writer owns static declaration identity while one Program is unfinished.
// Declaration IDs, rather than source names, are the sole lexical reference
// authority; names are retained only as authored reference spelling.
type Writer struct {
	static           *assembly.Collector
	flow             *assembly.Collector
	binding          *bind.Result
	scopes           *lexical.Bodies
	sourceName       string
	terms            map[bind.TypeDeclID]keyspace.Term
	children         []keyspace.Term
	fields           []keyspace.Term
	interfaceMembers []assembly.StaticInterfaceMember
	params           []assembly.StaticParameter
	generics         []keyspace.Term

	// phases and steps are the static vertical's one iterative walk. They are
	// deliberately private: source selects only the next owner token and never
	// carries a static constructor's cursor, child range, or containment
	// judgment.
	phases      *continuation.Stack
	expressions *continuation.Expressions
	evaluations *eval.Values
	steps       []walkStep
	staticDepth int
}

// StaticDepth is positive precisely while an ordinary expression or Values
// continuation is an operand of authored static syntax. Function lowering
// uses this fact to validate the binder's static-containment evidence.
func (w *Writer) StaticDepth() int {
	if w == nil {
		return 0
	}
	return w.staticDepth
}

// Mark starts one LIFO ordered child range for an iterative type walk.
func (w *Writer) Mark() int {
	if w == nil {
		return -1
	}
	return len(w.children)
}

func (w *Writer) FieldMark() int {
	if w == nil {
		return -1
	}
	return len(w.fields)
}

// InterfaceMemberMark starts one LIFO range for an interface's exact authored
// member sequence. It is deliberately separate from record-field scratch: a
// record has only fields, while an interface's sequence contains both variants.
func (w *Writer) InterfaceMemberMark() int {
	if w == nil {
		return -1
	}
	return len(w.interfaceMembers)
}

// Append retains one completed static type child in source order.
func (w *Writer) Append(term keyspace.Term) error {
	if w == nil || term == 0 {
		return fmt.Errorf("lualower: invalid static type child")
	}
	w.children = append(w.children, term)
	return nil
}

func (w *Writer) AppendField(term keyspace.Term) error {
	if w == nil || term == 0 {
		return fmt.Errorf("lualower: invalid static field child")
	}
	w.fields = append(w.fields, term)
	return nil
}

func (w *Writer) AppendInterfaceField(term keyspace.Term) error {
	if w == nil || term == 0 {
		return fmt.Errorf("lualower: invalid interface field child")
	}
	w.interfaceMembers = append(w.interfaceMembers, assembly.StaticInterfaceMember{
		Kind: programstatic.InterfaceField, Field: term,
	})
	return nil
}

func (w *Writer) AppendInterfaceMethod(name string, namePosition ast.Position, signature keyspace.Term) error {
	if w == nil || name == "" || signature == 0 {
		return fmt.Errorf("lualower: invalid interface method child")
	}
	w.interfaceMembers = append(w.interfaceMembers, assembly.StaticInterfaceMember{
		Kind:      programstatic.InterfaceMethod,
		Name:      name,
		Span:      w.nameSpan(namePosition),
		Signature: signature,
	})
	return nil
}

// Take completes one scalar child hold.
func (w *Writer) Take(mark int) (keyspace.Term, error) {
	if w == nil || mark < 0 || mark != len(w.children)-1 {
		return 0, fmt.Errorf("lualower: incomplete static type child")
	}
	term := w.children[mark]
	w.children = w.children[:mark]
	return term, nil
}

// TakeCallTypeArgs releases one ordered static argument range for an already
// declared executable Call. Keeping this range here ensures static children
// never enter runtime Values assembly.
func (w *Writer) TakeCallTypeArgs(mark, count int) ([]keyspace.Term, error) {
	return w.rangeTerms(mark, count)
}

// Clean reports whether the iterative type-child scratch is empty.
func (w *Writer) Clean() bool {
	return w != nil && len(w.children) == 0 && len(w.fields) == 0 &&
		len(w.interfaceMembers) == 0 && len(w.params) == 0 && len(w.generics) == 0 &&
		len(w.steps) == 0 && w.staticDepth == 0
}

// New creates a static writer over the typed Collector Static and Flow
// capabilities. Flow is used only for executable function contracts and the
// binder-owned global Cell selector; all authored static rows use Static.
func New(
	phases *continuation.Stack,
	staticRoot *assembly.Collector,
	flowRoot *assembly.Collector,
	binding *bind.Result,
	scopes *lexical.Bodies,
	expressions *continuation.Expressions,
	evaluations *eval.Values,
	sourceName string,
) *Writer {
	return &Writer{
		static:      staticRoot,
		flow:        flowRoot,
		binding:     binding,
		scopes:      scopes,
		sourceName:  sourceName,
		phases:      phases,
		expressions: expressions,
		evaluations: evaluations,
	}
}

// Predeclare records identities for the direct alias and interface statements
// in body.
// Parameters are deliberately deferred until the lowering walk reaches the
// declaration's authored source turn.
func (w *Writer) Predeclare(body keyspace.Term, stmts []ast.Stmt) error {
	if w == nil || w.binding == nil {
		return fmt.Errorf("lualower: static writer is not initialized")
	}
	for _, stmt := range stmts {
		switch def := stmt.(type) {
		case *ast.TypeDefStmt:
			if w.terms == nil {
				w.terms = make(map[bind.TypeDeclID]keyspace.Term)
			}
			decl, ok := w.binding.TypeDef(def)
			if !ok || decl.Kind != bind.TypeDeclAlias || decl.ID == 0 || decl.Name != def.Name {
				return fmt.Errorf("lualower: missing type alias binding for %q", def.Name)
			}
			if _, exists := w.terms[decl.ID]; exists {
				return fmt.Errorf("lualower: duplicate type alias identity for %q", def.Name)
			}
			term := w.static.Alias(
				w.span(def), w.nameSpan(def.NamePosition), body, def.Name,
			)
			if term == 0 {
				return fmt.Errorf("lualower: could not predeclare type alias %q", def.Name)
			}
			w.terms[decl.ID] = term
		case *ast.InterfaceDefStmt:
			if w.terms == nil {
				w.terms = make(map[bind.TypeDeclID]keyspace.Term)
			}
			decl, ok := w.binding.InterfaceDef(def)
			if !ok || decl.Kind != bind.TypeDeclInterface || decl.ID == 0 || decl.Name != def.Name {
				return fmt.Errorf("lualower: missing interface binding for %q", def.Name)
			}
			if _, exists := w.terms[decl.ID]; exists {
				return fmt.Errorf("lualower: duplicate interface identity for %q", def.Name)
			}
			term := w.static.Interface(
				w.span(def), w.nameSpan(def.NamePosition), body, def.Name,
			)
			if term == 0 {
				return fmt.Errorf("lualower: could not predeclare interface %q", def.Name)
			}
			w.terms[decl.ID] = term
		}
	}
	return nil
}

// BeginAlias installs one predeclared alias's ordered parameter hosts and
// returns the existing identity for the active Body source sequence.
func (w *Writer) BeginAlias(def *ast.TypeDefStmt) (keyspace.Term, error) {
	alias, decl, err := w.alias(def)
	if err != nil {
		return 0, err
	}
	params := w.binding.TypeDefParams(def)
	if len(params) != len(def.TypeParams) {
		return 0, fmt.Errorf("lualower: type alias parameter binding/source count mismatch for %q", decl.Name)
	}
	terms := make([]keyspace.Term, 0, len(params))
	for index, param := range params {
		if param.ID == 0 || param.Kind != bind.TypeDeclParam || param.Name == "" {
			return 0, fmt.Errorf("lualower: invalid type parameter on %q", decl.Name)
		}
		if _, exists := w.terms[param.ID]; exists {
			return 0, fmt.Errorf("lualower: duplicate type parameter identity %q", param.Name)
		}
		if index >= len(def.TypeParams) || def.TypeParams[index].Name != param.Name {
			return 0, fmt.Errorf("lualower: invalid type parameter source position %q", param.Name)
		}
		term := w.static.TypeParam(w.nameSpan(param.NamePosition), alias, param.Name)
		if term == 0 {
			return 0, fmt.Errorf("lualower: could not declare type parameter %q", param.Name)
		}
		w.terms[param.ID] = term
		terms = append(terms, term)
	}
	if !w.static.AliasParams(alias, terms) {
		return 0, fmt.Errorf("lualower: could not set type parameters for %q", decl.Name)
	}
	return alias, nil
}

// Host returns the exact Program host for a bound alias, interface, or type
// parameter.
func (w *Writer) Host(decl bind.TypeDecl) (keyspace.Term, bool) {
	if w == nil || decl.ID == 0 {
		return 0, false
	}
	term, ok := w.terms[decl.ID]
	return term, ok
}

// FinishParam fills a predeclared parameter's one exact constraint attachment.
// A zero constraint explicitly denotes an unconstrained parameter.
func (w *Writer) FinishParam(decl bind.TypeDecl, constraint keyspace.Term) error {
	term, ok := w.Host(decl)
	if !ok || decl.Kind != bind.TypeDeclParam || !w.static.TypeParamConstraint(term, constraint) {
		return fmt.Errorf("lualower: could not finalize type parameter %q", decl.Name)
	}
	return nil
}

// FinishAlias fills a predeclared source-indexed alias with its lowered type.
func (w *Writer) FinishAlias(def *ast.TypeDefStmt, target keyspace.Term) error {
	alias, decl, err := w.alias(def)
	if err != nil {
		return err
	}
	if !w.static.AliasTarget(alias, target) {
		return fmt.Errorf("lualower: could not finalize type alias %q", decl.Name)
	}
	return nil
}

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

// Assertion lowers one parser-admitted assertion type expression with its
// exact binder immediate-formal ordinal when it has one. An unresolved source
// name is retained as -1 for later context-sensitive Rules.
func (w *Writer) Assertion(expr *ast.AssertsTypeExpr, narrow keyspace.Term) (keyspace.Term, error) {
	if w == nil || w.binding == nil || expr == nil || (expr.NarrowTo == nil) != (narrow == 0) {
		return 0, fmt.Errorf("lualower: invalid assertion type")
	}
	ordinal := -1
	if bound, ok := w.binding.AssertedParam(expr); ok {
		ordinal = bound
	}
	bound := ordinal >= 0
	if !bound {
		ordinal = 0
	}
	return w.term(w.static.TypeAsserts(
		w.span(expr), w.nameSpan(expr.ParamPosition),
		expr.ParamName, bound, uint32(ordinal), narrow,
	), "assertion type")
}

// leaf lowers one static type leaf. It reports false only to its own
// iterative type walker, which schedules structural children explicitly.
func (w *Writer) leaf(expr ast.TypeExpr) (term keyspace.Term, found bool, err error) {
	if w == nil || w.binding == nil || expr == nil {
		return 0, true, fmt.Errorf("lualower: invalid static type leaf")
	}
	switch expr := expr.(type) {
	case *ast.PrimitiveTypeExpr:
		term, err = w.PrimitiveOrRef(expr)
		return term, true, err
	case *ast.LiteralTypeExpr:
		term, err = w.Literal(expr)
		return term, true, err
	case *ast.TypeRefExpr:
		term, err = w.TypeRef(expr)
		return term, true, err
	default:
		return 0, false, nil
	}
}

// PrimitiveOrRef lowers a parser primitive token either as its closed built-in
// kind or as a binder-resolved/unresolved bare declaration reference.
func (w *Writer) PrimitiveOrRef(expr *ast.PrimitiveTypeExpr) (keyspace.Term, error) {
	if w == nil || w.binding == nil || expr == nil || expr.Name == "" {
		return 0, fmt.Errorf("lualower: invalid primitive type")
	}
	span := w.span(expr)
	if decl, ok := w.binding.PrimitiveTypeRef(expr); ok {
		return w.declarationRef(span, []string{expr.Name}, decl)
	}
	if kind, ok := programstatic.PrimitiveKindForName(expr.Name); ok {
		return w.term(w.static.Primitive(span, kind), "primitive type")
	}
	return w.term(w.static.Unresolved(span, []string{expr.Name}, 0), "unresolved primitive reference")
}

// Literal lowers a parser literal type without accepting opaque values.
func (w *Writer) Literal(expr *ast.LiteralTypeExpr) (keyspace.Term, error) {
	if w == nil || expr == nil {
		return 0, fmt.Errorf("lualower: invalid literal type")
	}
	span := w.span(expr)
	switch value := expr.Value.(type) {
	case bool:
		return w.term(w.static.LiteralBool(span, value), "boolean literal type")
	case int64:
		return w.term(w.static.LiteralInteger(span, value), "integer literal type")
	case float64:
		return w.term(w.static.LiteralFloat(span, value), "float literal type")
	case string:
		return w.term(w.static.LiteralString(span, value), "string literal type")
	case nil:
		return 0, fmt.Errorf("lualower: malformed numeric literal type")
	default:
		return 0, fmt.Errorf("lualower: unsupported literal type %T", value)
	}
}

// Optional completes an OptionalTypeExpr after its one child has been lowered.
func (w *Writer) Optional(expr *ast.OptionalTypeExpr, inner keyspace.Term) (keyspace.Term, error) {
	if expr == nil {
		return 0, fmt.Errorf("lualower: nil optional type")
	}
	return w.term(w.static.Optional(w.span(expr), inner), "optional type")
}

// Union completes a UnionTypeExpr from one ordered child range.
func (w *Writer) Union(expr *ast.UnionTypeExpr, mark, count int) (keyspace.Term, error) {
	if expr == nil {
		return 0, fmt.Errorf("lualower: nil union type")
	}
	members, err := w.rangeTerms(mark, count)
	if err != nil {
		return 0, err
	}
	return w.term(w.static.Union(w.span(expr), members), "union type")
}

// Intersection completes an IntersectionTypeExpr after its ordered members
// have been lowered.
func (w *Writer) Intersection(expr *ast.IntersectionTypeExpr, mark, count int) (keyspace.Term, error) {
	if expr == nil {
		return 0, fmt.Errorf("lualower: nil intersection type")
	}
	members, err := w.rangeTerms(mark, count)
	if err != nil {
		return 0, err
	}
	return w.term(w.static.Intersection(w.span(expr), members), "intersection type")
}

// Generic completes a GenericTypeExpr after its base and ordered arguments are
// lowered by the caller.
func (w *Writer) Generic(expr *ast.GenericTypeExpr, base keyspace.Term, mark, count int) (keyspace.Term, error) {
	if expr == nil {
		return 0, fmt.Errorf("lualower: nil generic type")
	}
	args, err := w.rangeTerms(mark, count)
	if err != nil {
		return 0, err
	}
	return w.term(w.static.Generic(w.span(expr), base, args), "generic type")
}

// Array completes parser-authored array syntax after its element type has been
// lowered. The static walker schedules element and array annotations around
// this exact structural construction.
func (w *Writer) Array(expr *ast.ArrayTypeExpr, element keyspace.Term) (keyspace.Term, error) {
	if w == nil || expr == nil || element == 0 {
		return 0, fmt.Errorf("lualower: invalid array type")
	}
	return w.term(w.static.Array(w.span(expr), element, expr.Readonly), "array type")
}

// Map completes parser-authored map syntax after its ordered key and value
// children have been lowered.
func (w *Writer) Map(expr *ast.MapTypeExpr, key, value keyspace.Term) (keyspace.Term, error) {
	if w == nil || expr == nil || key == 0 || value == 0 {
		return 0, fmt.Errorf("lualower: invalid map type")
	}
	return w.term(w.static.Map(w.span(expr), key, value, expr.Readonly), "map type")
}

// Field creates one shared TypeField identity before its annotations are
// lowered. Record and Interface both consume these same Terms.
func (w *Writer) Field(field ast.RecordFieldExpr, typ keyspace.Term) (keyspace.Term, error) {
	if w == nil || field.Name == "" || typ == 0 {
		return 0, fmt.Errorf("lualower: invalid static field")
	}
	return w.term(w.static.Field(
		w.nameSpan(field.NamePosition), field.Name, typ, field.Optional,
	), "static field")
}

// Record completes one ordered TypeField range owned by Writer scratch.
func (w *Writer) Record(expr *ast.RecordTypeExpr, mark, count int) (keyspace.Term, error) {
	if w == nil || expr == nil || count != len(expr.Fields) {
		return 0, fmt.Errorf("lualower: invalid record type")
	}
	for _, field := range expr.Fields {
		if field.Name == "" {
			return 0, fmt.Errorf("lualower: record field is unsupported")
		}
	}
	if mark < 0 || count < 0 || mark > len(w.fields) || len(w.fields)-mark != count {
		return 0, fmt.Errorf("lualower: incomplete record fields")
	}
	term := w.static.Record(w.span(expr), w.fields[mark:], expr.Readonly)
	w.fields = w.fields[:mark]
	return w.term(term, "record type")
}

// DeclareAnnotation reserves an authored annotation before its ordinary
// expression-list arguments are lowered into canonical Values.
func (w *Writer) DeclareAnnotation(expr ast.AnnotationExpr, scope, target keyspace.Term) (keyspace.Term, error) {
	if w == nil || scope == 0 || target == 0 || expr.Name == "" {
		return 0, fmt.Errorf("lualower: invalid annotation")
	}
	return w.term(w.static.Annotation(w.span(&expr), scope, target, expr.Name), "annotation")
}

func (w *Writer) FillAnnotation(annotation, values keyspace.Term) error {
	if w == nil || !w.static.AnnotationValues(annotation, values) {
		return fmt.Errorf("lualower: could not finalize annotation")
	}
	return nil
}

// TypeOf lowers typeof with the exact declaration/parameter host and an
// already-lowered ordinary expression operand.
func (w *Writer) TypeOf(expr *ast.TypeOfExpr, host, operand keyspace.Term) (keyspace.Term, error) {
	if w == nil || expr == nil || operand == 0 {
		return 0, fmt.Errorf("lualower: invalid typeof")
	}
	return w.term(w.static.TypeOf(w.span(expr), host, operand), "typeof")
}

// KeyOf completes parser-authored keyof syntax after its child has been
// lowered by the static walker.
func (w *Writer) KeyOf(expr *ast.KeyOfExpr, inner keyspace.Term) (keyspace.Term, error) {
	if w == nil || expr == nil || inner == 0 {
		return 0, fmt.Errorf("lualower: invalid keyof type")
	}
	return w.term(w.static.KeyOf(w.span(expr), inner), "keyof type")
}

// IndexAccess completes parser-authored indexed access after its two ordered
// children have been lowered by the static walker.
func (w *Writer) IndexAccess(
	expr *ast.IndexAccessExpr,
	mark int,
) (keyspace.Term, error) {
	if w == nil || expr == nil {
		return 0, fmt.Errorf("lualower: invalid indexed-access type")
	}
	children, err := w.rangeTerms(mark, 2)
	if err != nil {
		return 0, err
	}
	return w.term(w.static.IndexAccess(w.span(expr), children[0], children[1]), "indexed-access type")
}

// Conditional completes parser-authored conditional syntax without applying
// any type-domain branch semantics.
func (w *Writer) Conditional(
	expr *ast.ConditionalTypeExpr,
	mark int,
) (keyspace.Term, error) {
	if w == nil || expr == nil {
		return 0, fmt.Errorf("lualower: invalid conditional type")
	}
	children, err := w.rangeTerms(mark, 4)
	if err != nil {
		return 0, err
	}
	return w.term(w.static.Conditional(
		w.span(expr), children[0], children[1], children[2], children[3],
	), "conditional type")
}

// TypeRef lowers a bare or qualified parser reference using binder evidence.
func (w *Writer) TypeRef(ref *ast.TypeRefExpr) (keyspace.Term, error) {
	if w == nil || w.binding == nil || w.scopes == nil || ref == nil {
		return 0, fmt.Errorf("lualower: invalid type reference")
	}
	if len(ref.Path) == 0 {
		return 0, fmt.Errorf("lualower: invalid type reference path")
	}
	for _, part := range ref.Path {
		if part == "" {
			return 0, fmt.Errorf("lualower: empty type reference component")
		}
	}
	span := w.span(ref)
	var root keyspace.Term
	if len(ref.Path) > 1 {
		rootID, ok := w.binding.QualifiedTypeRootSymbol(ref)
		if !ok {
			return 0, fmt.Errorf("lualower: qualified type reference has no root symbol")
		}
		root, ok = w.scopes.Resolve(rootID)
		if !ok {
			kind, known := w.binding.Kind(rootID)
			name := w.binding.Name(rootID)
			if !known || kind != bind.SymbolGlobal || name == "" || name != ref.Path[0] {
				return 0, fmt.Errorf("lualower: qualified type reference root is not visible")
			}
			if !ref.RootPosition.Valid() {
				return 0, fmt.Errorf("lualower: qualified global type root has no token span")
			}
			identity, global := w.binding.GlobalIdentityOf(rootID)
			if !global || !identity.Matches(ref.Path[0]) {
				return 0, fmt.Errorf("lualower: qualified type reference root is not visible")
			}
			root = w.flow.Global(identity)
		}
		if root == 0 {
			return 0, fmt.Errorf("lualower: could not materialize qualified type root")
		}
	}
	if decl, ok := w.binding.TypeRef(ref); ok {
		target, targetErr := w.declarationTarget(decl)
		if targetErr != nil {
			return 0, targetErr
		}
		return w.term(w.static.Declaration(span, ref.Path, root, target), "declaration type reference")
	}
	if resolved, ok := w.binding.QualifiedTypeRef(ref); ok {
		if resolved.Decl.ID != 0 {
			target, targetErr := w.declarationTarget(resolved.Decl)
			if targetErr != nil {
				return 0, targetErr
			}
			return w.term(w.static.Declaration(span, ref.Path, root, target), "declaration type reference")
		}
		if len(resolved.Path) != 0 {
			for _, part := range resolved.Path {
				if part == "" {
					return 0, fmt.Errorf("lualower: invalid canonical type path")
				}
			}
			return w.term(w.static.Canonical(span, ref.Path, resolved.Path, root), "canonical type reference")
		}
	}
	return w.term(w.static.Unresolved(span, ref.Path, root), "unresolved type reference")
}

// RuntimeTypeTarget lowers the binder's closed runtime-type authority to its
// exact static Program target. It accepts only the authority classes selected
// for a compiler-special call base; ordinary value syntax remains outside this
// path.
func (w *Writer) RuntimeTypeTarget(span source.Span, value bind.RuntimeTypeValue) (keyspace.Term, error) {
	if w == nil || value.Base == nil || value.Name == "" || value.Base.Value != value.Name {
		return 0, fmt.Errorf("lualower: invalid runtime type value")
	}
	switch value.Kind {
	case bind.RuntimeTypeValuePrimitive:
		if value.Decl.ID != 0 {
			return 0, fmt.Errorf("lualower: primitive runtime type value %q has a declaration", value.Name)
		}
		kind, ok := programstatic.PrimitiveKindForName(value.Name)
		if !ok || !kind.RuntimeLoadable() {
			return 0, fmt.Errorf("lualower: unsupported runtime primitive type %q", value.Name)
		}
		return w.term(w.static.Primitive(span, kind), "runtime primitive type")
	case bind.RuntimeTypeValueDeclaration:
		if value.Decl.ID == 0 || value.Decl.Name != value.Name {
			return 0, fmt.Errorf("lualower: invalid runtime declaration type %q", value.Name)
		}
		if value.Decl.Kind != bind.TypeDeclAlias && value.Decl.Kind != bind.TypeDeclInterface {
			return 0, fmt.Errorf("lualower: unsupported runtime declaration type %q", value.Name)
		}
		return w.declarationRef(span, []string{value.Name}, value.Decl)
	default:
		return 0, fmt.Errorf("lualower: unsupported runtime type value %q", value.Name)
	}
}

// PublicationRef creates the one authored TypeRef owned by a static type
// publication. Source spelling and binder resolution remain distinct: a
// declaration target uses its exact lexical identity, while a module target
// uses the existing canonical TypeRef path.
func (w *Writer) PublicationRef(
	span source.Span,
	source []string,
	root keyspace.Term,
	alias bind.QualifiedTypeAlias,
) (keyspace.Term, error) {
	if w == nil {
		return 0, fmt.Errorf("lualower: invalid publication type reference")
	}
	if len(source) == 0 {
		return 0, fmt.Errorf("lualower: invalid publication type reference path")
	}
	for _, part := range source {
		if part == "" {
			return 0, fmt.Errorf("lualower: empty publication type reference component")
		}
	}
	if alias.Decl.ID != 0 {
		if len(alias.Path) != 0 {
			return 0, fmt.Errorf("lualower: ambiguous publication type reference")
		}
		target, targetErr := w.declarationTarget(alias.Decl)
		if targetErr != nil {
			return 0, targetErr
		}
		return w.term(w.static.Declaration(span, source, root, target), "declaration publication type reference")
	}
	if len(alias.Path) == 0 {
		return 0, fmt.Errorf("lualower: unresolved publication type reference")
	}
	for _, part := range alias.Path {
		if part == "" {
			return 0, fmt.Errorf("lualower: invalid canonical publication type path")
		}
	}
	return w.term(w.static.Canonical(span, source, alias.Path, root), "canonical publication type reference")
}

func (w *Writer) declarationRef(span source.Span, source []string, decl bind.TypeDecl) (keyspace.Term, error) {
	target, err := w.declarationTarget(decl)
	if err != nil {
		return 0, err
	}
	if len(source) == 0 {
		return 0, fmt.Errorf("lualower: invalid declaration type reference path")
	}
	for _, part := range source {
		if part == "" {
			return 0, fmt.Errorf("lualower: empty declaration type reference component")
		}
	}
	return w.term(w.static.Declaration(span, source, 0, target), "declaration type reference")
}

func (w *Writer) declarationTarget(decl bind.TypeDecl) (keyspace.Term, error) {
	target, ok := w.Host(decl)
	if !ok || (decl.Kind != bind.TypeDeclAlias &&
		decl.Kind != bind.TypeDeclInterface && decl.Kind != bind.TypeDeclParam) {
		return 0, fmt.Errorf("lualower: unavailable type declaration %q", decl.Name)
	}
	return target, nil
}

func (w *Writer) alias(def *ast.TypeDefStmt) (keyspace.Term, bind.TypeDecl, error) {
	if w == nil || w.binding == nil || def == nil {
		return 0, bind.TypeDecl{}, fmt.Errorf("lualower: invalid type alias")
	}
	decl, ok := w.binding.TypeDef(def)
	if !ok || decl.Kind != bind.TypeDeclAlias {
		return 0, bind.TypeDecl{}, fmt.Errorf("lualower: missing type alias binding")
	}
	term, ok := w.Host(decl)
	if !ok {
		return 0, bind.TypeDecl{}, fmt.Errorf("lualower: type alias %q was not predeclared", decl.Name)
	}
	return term, decl, nil
}

func (w *Writer) term(term keyspace.Term, what string) (keyspace.Term, error) {
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not create %s", what)
	}
	return term, nil
}

func (w *Writer) rangeTerms(mark, count int) ([]keyspace.Term, error) {
	if w == nil || mark < 0 || count < 0 || mark > len(w.children) || len(w.children)-mark != count {
		return nil, fmt.Errorf("lualower: incomplete static type children")
	}
	terms := w.children[mark:]
	w.children = w.children[:mark]
	return terms, nil
}

func (w *Writer) span(holder ast.PositionHolder) source.Span {
	if holder == nil {
		return source.Span{File: w.sourceName}
	}
	span, ok := coord.Build(w.sourceName, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return coord.Invalid(w.sourceName)
	}
	return span
}

func (w *Writer) nameSpan(position ast.Position) source.Span {
	file := position.File
	if file == "" {
		file = w.sourceName
	}
	span, ok := coord.Build(file, position.Line, position.Column, position.EndLine, position.EndColumn)
	if !ok {
		return coord.Invalid(file)
	}
	return span
}
