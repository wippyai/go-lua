package bind

import (
	"strings"

	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/domain/type/ambient"
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
	// TypeDeclAmbient is a declaration of the ambient namespace: a name always
	// available to annotations, declared by the ambient catalogue rather than
	// by a statement of this chunk. Ambient names occupy the outermost type
	// scope, so an authored declaration of the same name shadows one.
	TypeDeclAmbient
)

// TypeDecl records one declaration in the lexical type namespace.
type TypeDecl struct {
	ID           TypeDeclID
	Kind         TypeDeclKind
	Name         string
	NamePosition ast.Position
	Type         *ast.TypeDefStmt
	Interface    *ast.InterfaceDefStmt
	Constraint   ast.TypeExpr
	// Ambient carries the catalogue row an ambient declaration was opened
	// from. It is the whole declaration for that kind: the ambient namespace
	// has no statement to point back at.
	Ambient ambient.Declaration
}

// TypeValueRef returns the lexical declaration selected for a value-position
// type name. Binder records this evidence independently of compiler-special
// runtime-type call bases so lowerers can materialize the same declaration as
// a runtime TypeValue when the authored value occurrence requires it.
func (r *Result) TypeValueRef(ident *ast.IdentExpr) (TypeDecl, bool) {
	if r == nil || ident == nil {
		return TypeDecl{}, false
	}
	decl, ok := r.typeValueRefs[ident]
	return decl, ok && decl.ID != 0
}

// AmbientTypes returns the ambient declarations this chunk names, in the order
// the catalogue declares them. A lowering materializes exactly these rows.
func (r *Result) AmbientTypes() []TypeDecl {
	if r == nil {
		return nil
	}
	return cloneTypeDecls(r.ambientTypes)
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

// FunctionTypeParams returns the lexical type parameters declared by a
// function expression or static function type.
func (r *Result) FunctionTypeParams(fn ast.PositionHolder) []TypeDecl {
	if r == nil || !functionTypeParamNode(fn) {
		return nil
	}
	return cloneTypeDecls(r.functionTypeParams[fn])
}

// AssertedParam returns the formal ordinal selected by a return-position
// assertion. The ordinal is relative to the immediate containing callable
// signature, including an implicit method self formal when present. Missing,
// unnamed, and outer-scope formals intentionally have no result.
func (r *Result) AssertedParam(expr *ast.AssertsTypeExpr) (int, bool) {
	if r == nil || expr == nil {
		return 0, false
	}
	ordinal, ok := r.assertedParams[expr]
	return ordinal, ok && ordinal >= 0
}

func cloneTypeDecls(decls []TypeDecl) []TypeDecl {
	if len(decls) == 0 {
		return nil
	}
	return append([]TypeDecl(nil), decls...)
}

func functionTypeParamNode(node ast.PositionHolder) bool {
	switch node := node.(type) {
	case *ast.FunctionExpr:
		return node != nil
	case *ast.FunctionTypeExpr:
		return node != nil
	default:
		return false
	}
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

type typeUndo struct {
	name    string
	prior   TypeDecl
	existed bool
}

func (b *binder) pushTypeScope() {
	b.typeMarks = append(b.typeMarks, len(b.typeUndo))
}

func (b *binder) popTypeScope() {
	if len(b.typeMarks) == 0 {
		return
	}
	mark := b.typeMarks[len(b.typeMarks)-1]
	for i := len(b.typeUndo) - 1; i >= mark; i-- {
		undo := b.typeUndo[i]
		if undo.existed {
			b.typeHeads[undo.name] = undo.prior
		} else {
			delete(b.typeHeads, undo.name)
		}
	}
	b.typeUndo = b.typeUndo[:mark]
	b.typeMarks = b.typeMarks[:len(b.typeMarks)-1]
}

func (b *binder) defineType(name string, decl TypeDecl) {
	if name == "" || len(b.typeMarks) == 0 || decl.ID == 0 {
		return
	}
	if b.typeHeads == nil {
		b.typeHeads = make(map[string]TypeDecl)
	}
	prior, existed := b.typeHeads[name]
	b.typeUndo = append(b.typeUndo, typeUndo{name: name, prior: prior, existed: existed})
	b.typeHeads[name] = decl
}

func (b *binder) lookupType(name string) (TypeDecl, bool) {
	if name == "" {
		return TypeDecl{}, false
	}
	decl, ok := b.typeHeads[name]
	return decl, ok && decl.ID != 0
}

// declareAmbientTypes opens the outermost type scope with the ambient
// namespace. Every ambient name is one ordinary entry of the lexical type
// namespace, so an annotation resolves it exactly the way it resolves an
// authored declaration, and an authored declaration of the same name shadows
// it under the ordinary scope rule.
func (b *binder) declareAmbientTypes() {
	for _, declaration := range ambient.Declarations() {
		decl := b.result.newTypeDecl(TypeDeclAmbient, declaration.Name, nil, nil, nil)
		if decl.ID == 0 {
			continue
		}
		decl.Ambient = declaration
		b.defineType(declaration.Name, decl)
	}
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
		decl.NamePosition = param.NamePosition
		b.defineType(param.Name, decl)
		decls = append(decls, decl)
	}
	return decls
}

func (b *binder) bindTypeRef(ref *ast.TypeRefExpr) {
	if ref == nil || len(ref.Path) == 0 {
		return
	}
	if len(ref.Path) > 1 {
		// A qualified annotation can name a runtime import alias without ever
		// reading that value at runtime. Preserve the lexical symbol separately;
		// recording a runtime capture would change closure semantics.
		id, _, found := b.lookup(ref.Path[0])
		if !found {
			id = b.global(ref.Path[0])
			if b.staticOnlyGlobals == nil {
				b.staticOnlyGlobals = make(map[Symbol]struct{})
			}
			b.staticOnlyGlobals[id] = struct{}{}
		}
		if id == 0 {
			return
		}
		origin := ref.RootPosition
		if !origin.Valid() {
			origin = authoredPosition(ref)
		}
		b.result.observeGlobalAt(id, origin, true)
		b.result.qualifiedTypeRootSymbols[ref] = id
		key := qualifiedTypeAliasKey{root: id, suffix: strings.Join(ref.Path[1:], ".")}
		if alias := b.qualifiedTypeAliases[key]; alias.valid() {
			b.result.qualifiedTypeRefs[ref] = alias.copy()
			return
		}
		if alias, published := b.qualifiedTypeIndexAlias(ref.Path); published {
			b.result.qualifiedTypeRefs[ref] = alias
		}
		return
	}
	decl, ok := b.lookupType(ref.Path[0])
	if !ok {
		return
	}
	b.observeAmbientType(decl)
	b.result.typeRefs[ref] = decl
}

func (b *binder) bindPrimitiveTypeRef(expr *ast.PrimitiveTypeExpr) {
	if expr == nil {
		return
	}
	if _, builtin := statictypes.PrimitiveKindForName(expr.Name); builtin {
		return
	}
	decl, ok := b.lookupType(expr.Name)
	if !ok {
		return
	}
	b.observeAmbientType(decl)
	b.result.primitiveTypeRefs[expr] = decl
}

// qualifiedTypeIndexAlias resolves one authored qualified spelling against the
// sealed qualified type index of the target. The index is the authority for
// which owner-qualified names exist, and it is read by exact spelling: a name
// it does not publish is not a type here, and the reference stays unresolved
// so the refusal can name it. A chunk that names nothing the index publishes
// takes nothing from it.
func (b *binder) qualifiedTypeIndexAlias(path []string) (QualifiedTypeAlias, bool) {
	if len(path) < 2 {
		return QualifiedTypeAlias{}, false
	}
	if _, published := b.qualifiedTypes.Lookup(strings.Join(path, ".")); !published {
		return QualifiedTypeAlias{}, false
	}
	return QualifiedTypeAlias{Path: append([]string(nil), path...)}, true
}

// observeAmbientType records that this chunk names one ambient declaration.
// The ambient namespace is always in scope, but only a named entry becomes a
// declaration of the Program: a chunk that annotates nothing ambient carries
// no ambient row.
func (b *binder) observeAmbientType(decl TypeDecl) {
	if decl.Kind != TypeDeclAmbient || decl.ID == 0 {
		return
	}
	for _, seen := range b.result.ambientTypes {
		if seen.ID == decl.ID {
			return
		}
	}
	b.result.ambientTypes = append(b.result.ambientTypes, decl)
}
