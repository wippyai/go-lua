package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/compiler/ast"
)

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
	if kind, ok := statictypes.PrimitiveKindForName(expr.Name); ok {
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
		kind, ok := statictypes.PrimitiveKindForName(value.Name)
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

// TypeValueTarget lowers a value-position type-name occurrence to the exact
// Static target used by a runtime TypeValue. The caller has already selected
// the call-argument context; ordinary identifier lowering never enters this
// path.
func (w *Writer) TypeValueTarget(ident *ast.IdentExpr) (keyspace.Term, error) {
	if w == nil || w.binding == nil || ident == nil || ident.Value == "" {
		return 0, fmt.Errorf("lualower: invalid type-value reference")
	}
	decl, ok := w.binding.TypeValueRef(ident)
	if !ok || decl.ID == 0 || decl.Name != ident.Value {
		return 0, fmt.Errorf("lualower: identifier %q is not a type-value reference", ident.Value)
	}
	return w.declarationRef(w.span(ident), []string{ident.Value}, decl)
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
