package bind

import (
	"strings"

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

// QualifiedTypeAlias is the exact type-namespace meaning introduced by a
// value-level declaration such as M.User = User or M.User = protocol.User.
// Decl is used for lexical declarations; Path is used for module aliases.
type QualifiedTypeAlias struct {
	Decl TypeDecl
	Path []string
}

type qualifiedTypeAliasKey struct {
	root   symbol.ID
	suffix string
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

// QualifiedTypeRef returns the alias declaration bound to a qualified type
// reference. It is separate from TypeRef because an alias may target an
// imported module type rather than a lexical TypeDecl.
func (r *Result) QualifiedTypeRef(ref *ast.TypeRefExpr) (QualifiedTypeAlias, bool) {
	if r == nil || ref == nil {
		return QualifiedTypeAlias{}, false
	}
	alias, ok := r.qualifiedTypeRefs[ref]
	return alias.copy(), ok && alias.valid()
}

// QualifiedTypeAliases returns direct aliases declared on root. The returned
// map is a copy so module publication cannot mutate binder state.
func (r *Result) QualifiedTypeAliases(root symbol.ID) map[string]QualifiedTypeAlias {
	if r == nil || root == 0 {
		return nil
	}
	out := make(map[string]QualifiedTypeAlias)
	for key, alias := range r.qualifiedTypeAliases {
		if key.root != root || !alias.valid() || strings.Contains(key.suffix, ".") {
			continue
		}
		out[key.suffix] = alias.copy()
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (a QualifiedTypeAlias) valid() bool {
	return a.Decl.ID != 0 || len(a.Path) != 0
}

func (a QualifiedTypeAlias) copy() QualifiedTypeAlias {
	a.Path = append([]string(nil), a.Path...)
	return a
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
		if fn := b.currentFunction(); fn != nil {
			if id, _, ok := b.lookup(ref.Path[0]); ok && id != 0 {
				b.result.recordQualifiedTypeRoot(fn, ref.Path[0], id)
				key := qualifiedTypeAliasKey{root: id, suffix: strings.Join(ref.Path[1:], ".")}
				if alias := b.result.qualifiedTypeAliases[key]; alias.valid() {
					b.result.qualifiedTypeRefs[ref] = alias.copy()
				}
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

func (b *binder) recordQualifiedTypeAliases(stmt *ast.AssignStmt) {
	if b == nil || b.result == nil || stmt == nil || len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	for i, lhs := range stmt.Lhs {
		root, suffix, ok := dottedTypeValuePath(lhs)
		if !ok || len(suffix) == 0 {
			continue
		}
		rootID, ok := b.result.SymbolOf(root)
		if !ok || rootID == 0 {
			continue
		}
		alias, ok := b.qualifiedTypeAliasSource(stmt.Rhs[i])
		if !ok {
			continue
		}
		key := qualifiedTypeAliasKey{root: rootID, suffix: strings.Join(suffix, ".")}
		if previous, exists := b.result.qualifiedTypeAliases[key]; exists && !qualifiedTypeAliasEqual(previous, alias) {
			b.result.qualifiedTypeAliases[key] = QualifiedTypeAlias{}
			continue
		}
		b.result.qualifiedTypeAliases[key] = alias.copy()
	}
}

func (b *binder) qualifiedTypeAliasSource(expr ast.Expr) (QualifiedTypeAlias, bool) {
	if ident, ok := expr.(*ast.IdentExpr); ok {
		decl, found := b.result.TypeValueRef(ident)
		return QualifiedTypeAlias{Decl: decl}, found
	}
	root, suffix, ok := dottedTypeValuePath(expr)
	if !ok || len(suffix) == 0 {
		return QualifiedTypeAlias{}, false
	}
	if rootID, found := b.result.SymbolOf(root); found {
		key := qualifiedTypeAliasKey{root: rootID, suffix: strings.Join(suffix, ".")}
		if alias := b.result.qualifiedTypeAliases[key]; alias.valid() {
			return alias.copy(), true
		}
	}
	path := make([]string, 1, len(suffix)+1)
	path[0] = root.Value
	path = append(path, suffix...)
	return QualifiedTypeAlias{Path: path}, true
}

func dottedTypeValuePath(expr ast.Expr) (*ast.IdentExpr, []string, bool) {
	var reverse []string
	for {
		switch e := expr.(type) {
		case *ast.IdentExpr:
			if e.Value == "" {
				return nil, nil, false
			}
			for left, right := 0, len(reverse)-1; left < right; left, right = left+1, right-1 {
				reverse[left], reverse[right] = reverse[right], reverse[left]
			}
			return e, reverse, true
		case *ast.AttrGetExpr:
			if e.KeySyntax != ast.AttrKeyDot {
				return nil, nil, false
			}
			name := ast.KeyName(e.Key)
			if name == "" {
				return nil, nil, false
			}
			reverse = append(reverse, name)
			expr = e.Object
		default:
			return nil, nil, false
		}
	}
}

func qualifiedTypeAliasEqual(left, right QualifiedTypeAlias) bool {
	if left.Decl.ID != right.Decl.ID || len(left.Path) != len(right.Path) {
		return false
	}
	for i := range left.Path {
		if left.Path[i] != right.Path[i] {
			return false
		}
	}
	return true
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
