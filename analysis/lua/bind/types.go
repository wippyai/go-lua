package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// TypeDeclID identifies a lexical type declaration independently of value
// symbols.
type TypeDeclID uint64

// TypeDeclKind classifies entries in the lexical type namespace.
type TypeDeclKind uint8

const (
	TypeDeclAlias TypeDeclKind = iota + 1
	TypeDeclInterface
	TypeDeclParam
)

// TypeDecl records one declaration in the lexical type namespace.
type TypeDecl struct {
	ID         TypeDeclID
	Kind       TypeDeclKind
	Name       string
	Type       *ast.TypeDefStmt
	Interface  *ast.InterfaceDefStmt
	Constraint ast.TypeExpr
}

// Stmt returns the declaration statement for alias and interface declarations.
func (d TypeDecl) Stmt() ast.Stmt {
	switch d.Kind {
	case TypeDeclAlias:
		return d.Type
	case TypeDeclInterface:
		return d.Interface
	default:
		return nil
	}
}

// TypeValueRef returns the lexical type declaration named by an identifier
// used in value position, such as the receiver in Point:is(v) or callee in
// Point(v).
func (r *Result) TypeValueRef(ident *ast.IdentExpr) (TypeDecl, bool) {
	if r == nil || ident == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.typeValueRefs[ident]
	return decl, ok && decl.ID != 0
}

// TypeRef returns the lexical type declaration bound to ref.
func (r *Result) TypeRef(ref *ast.TypeRefExpr) (TypeDecl, bool) {
	if r == nil || ref == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.typeRefs[ref]
	return decl, ok && decl.ID != 0
}

// PrimitiveTypeRef returns the lexical type declaration bound to a non-built-in
// primitive-name type expression.
func (r *Result) PrimitiveTypeRef(expr *ast.PrimitiveTypeExpr) (TypeDecl, bool) {
	if r == nil || expr == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.primitiveTypeRefs[expr]
	return decl, ok && decl.ID != 0
}

// TypeDef returns the lexical type declaration introduced by stmt.
func (r *Result) TypeDef(stmt *ast.TypeDefStmt) (TypeDecl, bool) {
	if r == nil || stmt == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.typeDefDecls[stmt]
	return decl, ok && decl.ID != 0
}

// InterfaceDef returns the lexical type declaration introduced by stmt.
func (r *Result) InterfaceDef(stmt *ast.InterfaceDefStmt) (TypeDecl, bool) {
	if r == nil || stmt == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.interfaceDecls[stmt]
	return decl, ok && decl.ID != 0
}

// TypeDefParams returns the lexical type parameters declared by stmt.
func (r *Result) TypeDefParams(stmt *ast.TypeDefStmt) []TypeDecl {
	if r == nil || stmt == nil {
		return nil
	}
	return cloneTypeDecls(r.typeDefParams[stmt])
}

// FunctionTypeParams returns the lexical type parameters declared by fn.
func (r *Result) FunctionTypeParams(fn *ast.FunctionExpr) []TypeDecl {
	if r == nil || fn == nil {
		return nil
	}
	return cloneTypeDecls(r.functionTypeParams[fn])
}

// MethodReceiverType returns the type declaration that types the implicit
// self receiver of a colon-method function. It is the sibling type whose name
// matches the method receiver, recorded when the method is bound.
func (r *Result) MethodReceiverType(fn *ast.FunctionExpr) (TypeDecl, bool) {
	if r == nil || fn == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.methodReceiverTypes[fn]
	return decl, ok && decl.ID != 0
}

func (r *Result) newTypeDecl(kind TypeDeclKind, name string, typeDef *ast.TypeDefStmt, iface *ast.InterfaceDefStmt, constraint ast.TypeExpr) TypeDecl {
	if name == "" {
		return TypeDecl{}
	}
	r.nextTypeDeclID++
	decl := TypeDecl{
		ID:         r.nextTypeDeclID,
		Kind:       kind,
		Name:       name,
		Type:       typeDef,
		Interface:  iface,
		Constraint: constraint,
	}
	switch kind {
	case TypeDeclAlias:
		if typeDef != nil {
			r.typeDefDecls[typeDef] = decl
		}
	case TypeDeclInterface:
		if iface != nil {
			r.interfaceDecls[iface] = decl
		}
	}
	return decl
}

type typeScope struct {
	names map[string]TypeDecl
}

func (b *binder) pushTypeScope() {
	b.typeScopes = append(b.typeScopes, typeScope{names: make(map[string]TypeDecl)})
}

func (b *binder) popTypeScope() {
	if len(b.typeScopes) == 0 {
		return
	}
	b.typeScopes = b.typeScopes[:len(b.typeScopes)-1]
}

func (b *binder) defineType(name string, decl TypeDecl) {
	if name == "" || len(b.typeScopes) == 0 || decl.ID == 0 {
		return
	}
	b.typeScopes[len(b.typeScopes)-1].names[name] = decl
}

func (b *binder) lookupType(name string) (TypeDecl, bool) {
	if name == "" {
		return TypeDecl{}, false
	}
	for i := len(b.typeScopes) - 1; i >= 0; i-- {
		if decl, ok := b.typeScopes[i].names[name]; ok && decl.ID != 0 {
			return decl, true
		}
	}
	return TypeDecl{}, false
}

// declareTypeDef introduces the alias name into the current type scope. It is
// idempotent per statement: hoisting may run before the in-order walk reaches
// the statement, and bindTypeDef must not re-declare it.
func (b *binder) declareTypeDef(stmt *ast.TypeDefStmt) {
	if stmt == nil {
		return
	}
	if _, ok := b.result.typeDefDecls[stmt]; ok {
		return
	}
	decl := b.result.newTypeDecl(TypeDeclAlias, stmt.Name, stmt, nil, nil)
	b.defineType(stmt.Name, decl)
}

// declareInterfaceDef introduces the interface name into the current type
// scope. It is idempotent per statement.
func (b *binder) declareInterfaceDef(stmt *ast.InterfaceDefStmt) {
	if stmt == nil {
		return
	}
	if _, ok := b.result.interfaceDecls[stmt]; ok {
		return
	}
	decl := b.result.newTypeDecl(TypeDeclInterface, stmt.Name, nil, stmt, nil)
	b.defineType(stmt.Name, decl)
}

func (b *binder) bindTypeDef(stmt *ast.TypeDefStmt) {
	if stmt == nil {
		return
	}
	b.declareTypeDef(stmt)
	b.bindTypeParamConstraints(stmt.TypeParams)
	b.pushTypeScope()
	params := b.defineTypeParams(stmt.TypeParams)
	if len(params) > 0 {
		b.result.typeDefParams[stmt] = params
	}
	b.bindTypeExpr(stmt.Type)
	b.popTypeScope()
}

func (b *binder) bindInterfaceDef(stmt *ast.InterfaceDefStmt) {
	if stmt == nil {
		return
	}
	b.declareInterfaceDef(stmt)
	for _, ref := range stmt.Extends {
		b.bindTypeRef(ref)
	}
	for _, field := range stmt.Fields {
		b.bindTypeExpr(field.Type)
	}
	for _, method := range stmt.Methods {
		if method.Type != nil {
			b.bindTypeExpr(method.Type)
		}
	}
}

func (b *binder) bindTypeParamConstraints(params []ast.TypeParamExpr) {
	for _, param := range params {
		b.bindTypeExpr(param.Constraint)
	}
}

func (b *binder) defineTypeParams(params []ast.TypeParamExpr) []TypeDecl {
	if len(params) == 0 {
		return nil
	}
	decls := make([]TypeDecl, 0, len(params))
	for _, param := range params {
		decl := b.result.newTypeDecl(TypeDeclParam, param.Name, nil, nil, param.Constraint)
		if decl.ID == 0 {
			continue
		}
		b.defineType(param.Name, decl)
		decls = append(decls, decl)
	}
	return decls
}

func (b *binder) bindTypeExprs(exprs []ast.TypeExpr) {
	for _, expr := range exprs {
		b.bindTypeExpr(expr)
	}
}

func (b *binder) bindTypeExpr(expr ast.TypeExpr) {
	switch expr := expr.(type) {
	case nil:
	case *ast.PrimitiveTypeExpr:
		b.bindPrimitiveTypeRef(expr)
	case *ast.SelfTypeExpr, *ast.LiteralTypeExpr:
	case *ast.OptionalTypeExpr:
		b.bindTypeExpr(expr.Inner)
	case *ast.UnionTypeExpr:
		b.bindTypeExprs(expr.Types)
	case *ast.IntersectionTypeExpr:
		b.bindTypeExprs(expr.Types)
	case *ast.ArrayTypeExpr:
		b.bindTypeExpr(expr.Element)
	case *ast.MapTypeExpr:
		b.bindTypeExpr(expr.Key)
		b.bindTypeExpr(expr.Value)
	case *ast.RecordTypeExpr:
		for _, field := range expr.Fields {
			b.bindTypeExpr(field.Type)
		}
	case *ast.FunctionTypeExpr:
		b.bindTypeParamConstraints(expr.TypeParams)
		b.pushTypeScope()
		b.defineTypeParams(expr.TypeParams)
		for _, param := range expr.Params {
			b.bindTypeExpr(param.Type)
		}
		b.bindTypeExpr(expr.Variadic)
		b.bindTypeExprs(expr.Returns)
		b.popTypeScope()
	case *ast.AssertsTypeExpr:
		b.bindTypeExpr(expr.NarrowTo)
	case *ast.TypeRefExpr:
		b.bindTypeRef(expr)
	case *ast.GenericTypeExpr:
		b.bindTypeRef(expr.Base)
		b.bindTypeExprs(expr.Args)
	case *ast.MetaTypeExpr:
		b.bindTypeExpr(expr.Inner)
	case *ast.TupleTypeExpr:
		b.bindTypeExprs(expr.Elements)
	case *ast.TypeOfExpr:
		b.bindTypeQueryExpr(expr.Expr)
	case *ast.KeyOfExpr:
		b.bindTypeExpr(expr.Inner)
	case *ast.IndexAccessExpr:
		b.bindTypeExpr(expr.Object)
		b.bindTypeExpr(expr.Index)
	case *ast.ConditionalTypeExpr:
		b.bindTypeExpr(expr.Check)
		b.bindTypeExpr(expr.Extends)
		b.bindTypeExpr(expr.Then)
		b.bindTypeExpr(expr.Else)
	}
}

func (b *binder) bindTypeRef(ref *ast.TypeRefExpr) {
	if ref == nil || len(ref.Path) == 0 {
		return
	}
	if len(ref.Path) > 1 {
		// A qualified annotation can name a runtime import alias without ever
		// reading that value at runtime. Preserve the lexical symbol separately;
		// recording a runtime capture would change closure semantics.
		if fn := b.currentFunction(); fn != nil {
			if id, _, ok := b.lookup(ref.Path[0]); ok && id != 0 {
				b.result.recordQualifiedTypeRoot(fn, ref.Path[0], id)
			}
		}
		return
	}
	decl, ok := b.lookupType(ref.Path[0])
	if !ok {
		return
	}
	b.result.typeRefs[ref] = decl
}

func (r *Result) recordQualifiedTypeRoot(fn *ast.FunctionExpr, name string, id symbol.ID) {
	if r == nil || fn == nil || name == "" || id == 0 {
		return
	}
	roots := r.qualifiedTypeRoots[fn]
	if roots == nil {
		roots = make(map[string]symbol.ID)
		r.qualifiedTypeRoots[fn] = roots
	}
	if previous, exists := roots[name]; exists && previous != id {
		// A body-wide alias projection cannot safely represent two lexical
		// declarations with the same spelling.
		roots[name] = 0
		return
	}
	roots[name] = id
}

// QualifiedTypeRoots returns exact value symbols used as roots of qualified
// annotations in fn. A zero symbol marks a scope-ambiguous spelling.
func (r *Result) QualifiedTypeRoots(fn *ast.FunctionExpr) map[string]symbol.ID {
	if r == nil || fn == nil || len(r.qualifiedTypeRoots[fn]) == 0 {
		return nil
	}
	out := make(map[string]symbol.ID, len(r.qualifiedTypeRoots[fn]))
	for name, id := range r.qualifiedTypeRoots[fn] {
		out[name] = id
	}
	return out
}

func (b *binder) bindPrimitiveTypeRef(expr *ast.PrimitiveTypeExpr) {
	if expr == nil || typ.BuiltinPrimitiveName(expr.Name) {
		return
	}
	decl, ok := b.lookupType(expr.Name)
	if !ok {
		return
	}
	b.result.primitiveTypeRefs[expr] = decl
}
