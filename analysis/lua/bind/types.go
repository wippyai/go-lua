package bind

import (
	"strings"

	programstatic "github.com/wippyai/go-lua/analysis/program/static"
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
	ID           TypeDeclID
	Kind         TypeDeclKind
	Name         string
	NamePosition ast.Position
	Type         *ast.TypeDefStmt
	Interface    *ast.InterfaceDefStmt
	Constraint   ast.TypeExpr
}

// RuntimeTypeValueKind is the closed authority class for one
// compiler-special runtime type base.
type RuntimeTypeValueKind uint8

const (
	RuntimeTypeValuePrimitive RuntimeTypeValueKind = iota + 1
	RuntimeTypeValueDeclaration
)

// RuntimeTypeValue is binder-owned evidence that Base is a compiler-special
// runtime type base. It is deliberately per identifier occurrence: a caller
// must still establish that the occurrence appears in an allowed call form.
//
// Decl is populated only for RuntimeTypeValueDeclaration. Base, Name, and
// Decl preserve the exact source and authority selected by the
// binder; its value-symbol identity is available through Result.SymbolOf.
type RuntimeTypeValue struct {
	Kind RuntimeTypeValueKind
	Base *ast.IdentExpr
	Name string
	Decl TypeDecl
}

// QualifiedTypeAlias is the exact type-namespace meaning introduced by a
// value-level declaration such as M.User = User or M.User = protocol.User.
// Decl is used for lexical declarations; Path is used for module aliases.
type QualifiedTypeAlias struct {
	Decl TypeDecl
	Path []string
}

// StaticTypePublication records one proven type-namespace member published by
// an assignment. Index identifies its original LHS/RHS pair; unrelated or
// unmatched pairs in the same assignment remain ordinary runtime syntax.
// Source is the exact authored RHS path and Alias is the exact declaration or
// qualified path published by that pair. The value-root identity and member
// suffix are transient binder inputs, not a second public projection.
type StaticTypePublication struct {
	Index  uint32
	Source []string
	Alias  QualifiedTypeAlias
}

type qualifiedTypeAliasKey struct {
	root   Symbol
	suffix string
}

// RuntimeTypeValue returns compiler-special runtime type-base evidence for
// ident. It is absent for ordinary value occurrences, including a value-local
// or parameter shadow of an otherwise authoritative type name.
func (r *Result) RuntimeTypeValue(ident *ast.IdentExpr) (RuntimeTypeValue, bool) {
	if r == nil || ident == nil {
		return RuntimeTypeValue{}, false
	}
	value, ok := r.runtimeTypeValues[ident]
	if !ok || value.Kind == 0 || value.Base != ident || value.Name == "" {
		return RuntimeTypeValue{}, false
	}
	if value.Kind == RuntimeTypeValueDeclaration && value.Decl.ID == 0 {
		return RuntimeTypeValue{}, false
	}
	return value, true
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

// StaticTypePublications returns binder-owned publication evidence for the
// proven pairs in stmt. It returns nil when the assignment has no such pairs.
func (r *Result) StaticTypePublications(stmt *ast.AssignStmt) []StaticTypePublication {
	if r == nil || stmt == nil {
		return nil
	}
	return cloneStaticTypePublications(r.staticTypePublications[stmt])
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

func cloneTypeDecls(decls []TypeDecl) []TypeDecl {
	if len(decls) == 0 {
		return nil
	}
	return append([]TypeDecl(nil), decls...)
}

func cloneStaticTypePublications(publications []StaticTypePublication) []StaticTypePublication {
	if len(publications) == 0 {
		return nil
	}
	out := make([]StaticTypePublication, len(publications))
	for i, publication := range publications {
		out[i] = StaticTypePublication{
			Index:  publication.Index,
			Source: append([]string(nil), publication.Source...),
			Alias:  publication.Alias.copy(),
		}
	}
	return out
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

// runtimePrimitiveTypeName is the VM's OP_LOADTYPE builtin subset of the
// canonical primitive vocabulary. `function` is a static primitive spelling
// but has no runtime type singleton, while `self` is context-relative.
func runtimePrimitiveTypeName(name string) bool {
	kind, ok := programstatic.PrimitiveKindForName(name)
	return ok && kind.RuntimeLoadable()
}

// recordChunkRuntimeTypeNames records precisely the declarations that have
// production runtime-type authority: direct declarations in the chunk being
// bound. Nested declarations intentionally do not participate.
func (b *binder) recordChunkRuntimeTypeNames(stmts []ast.Stmt) {
	if b == nil || b.result == nil {
		return
	}
	for _, stmt := range stmts {
		var decl TypeDecl
		switch stmt := stmt.(type) {
		case *ast.TypeDefStmt:
			decl, _ = b.result.TypeDef(stmt)
		case *ast.InterfaceDefStmt:
			decl, _ = b.result.InterfaceDef(stmt)
		default:
			continue
		}
		if decl.ID != 0 && decl.Name != "" {
			if b.runtimeChunkTypes == nil {
				b.runtimeChunkTypes = make(map[string]TypeDecl)
			}
			b.runtimeChunkTypes[decl.Name] = decl
		}
	}
}

// runtimeTypeValueAuthority resolves the exact production authority for a
// prospective runtime type base. A type-like spelling must also resolve to a
// global value identity; any active local, parameter, upvalue, or pending
// local-function name rejects the special interpretation. OP_LOADTYPE resolves
// its eight builtins before manifest/source names, so that order is binding
// law rather than a preference.
func (b *binder) runtimeTypeValueAuthority(ident *ast.IdentExpr) (RuntimeTypeValue, bool) {
	if b == nil || b.result == nil || ident == nil || ident.Value == "" {
		return RuntimeTypeValue{}, false
	}
	if _, global, found := b.lookup(ident.Value); found && !global {
		return RuntimeTypeValue{}, false
	}
	if runtimePrimitiveTypeName(ident.Value) {
		return RuntimeTypeValue{
			Kind: RuntimeTypeValuePrimitive,
			Base: ident,
			Name: ident.Value,
		}, true
	}
	if decl, ok := b.runtimeChunkTypes[ident.Value]; ok && decl.ID != 0 {
		return RuntimeTypeValue{
			Kind: RuntimeTypeValueDeclaration,
			Base: ident,
			Name: ident.Value,
			Decl: decl,
		}, true
	}
	return RuntimeTypeValue{}, false
}

// bindRuntimeTypeValue records a type-base symbol identity without creating
// an ordinary runtime read, capture, implicit-global use, or direct-call use.
func (b *binder) bindRuntimeTypeValue(value RuntimeTypeValue) {
	if b == nil || b.result == nil || value.Base == nil || value.Name == "" || value.Base.Value != value.Name || value.Kind == 0 {
		return
	}
	id, _, found := b.lookup(value.Name)
	if !found {
		id = b.global(value.Name)
	}
	if id == 0 {
		return
	}
	b.result.identSymbols[value.Base] = id
	if kind, ok := b.result.Kind(id); ok && kind == SymbolGlobal {
		// A runtime type-base occurrence alone is not a mutable Program Cell.
		// If a later ordinary/static value occurrence selects this identity,
		// that path upgrades it through observeGlobal(..., true).
		b.result.observeGlobal(id, value.Base, false)
	}
	if b.result.runtimeTypeValues == nil {
		b.result.runtimeTypeValues = make(map[*ast.IdentExpr]RuntimeTypeValue)
	}
	b.result.runtimeTypeValues[value.Base] = value
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
		}
		return
	}
	decl, ok := b.lookupType(ref.Path[0])
	if !ok {
		return
	}
	b.result.typeRefs[ref] = decl
}

// recordStaticTypePublications recognizes each independently proven static
// type-publication pair, records its evidence, and projects the canonical
// alias lookup from that evidence. It runs after LHS binding so the
// evidence reuses normal runtime symbol identities; publication is additive.
func (b *binder) recordStaticTypePublications(stmt *ast.AssignStmt) {
	if b == nil || b.result == nil || stmt == nil || len(stmt.Lhs) == 0 || len(stmt.Rhs) == 0 {
		return
	}
	limit := len(stmt.Lhs)
	if len(stmt.Rhs) < limit {
		limit = len(stmt.Rhs)
	}
	var publications []StaticTypePublication
	for i := 0; i < limit; i++ {
		lhs := stmt.Lhs[i]
		root, suffix, ok := dottedTypeValuePath(lhs)
		if !ok || len(suffix) == 0 {
			continue
		}
		candidate, ok := b.staticPublicationSourceCandidate(stmt.Rhs[i])
		if !ok {
			continue
		}
		rootID, bound := b.result.SymbolOf(root)
		if !bound || rootID == 0 {
			continue
		}
		alias, sourcePath, ok := b.bindStaticPublicationSource(candidate)
		if !ok {
			continue
		}
		if publications == nil {
			publications = make([]StaticTypePublication, 0, limit-i)
		}
		publications = append(publications, StaticTypePublication{
			Index:  uint32(i),
			Source: sourcePath,
			Alias:  alias.copy(),
		})
		b.projectQualifiedTypeAlias(rootID, suffix, alias)
	}
	if len(publications) == 0 {
		return
	}
	if b.result.staticTypePublications == nil {
		b.result.staticTypePublications = make(map[*ast.AssignStmt][]StaticTypePublication)
	}
	b.result.staticTypePublications[stmt] = cloneStaticTypePublications(publications)
}

type staticPublicationSourceCandidate struct {
	alias  QualifiedTypeAlias
	source []string
}

// staticPublicationSourceCandidate is intentionally side-effect-free so a
// later non-publication pair cannot leak partial evidence from a mixed
// assignment into the binder result.
func (b *binder) staticPublicationSourceCandidate(expr ast.Expr) (staticPublicationSourceCandidate, bool) {
	if ident, ok := expr.(*ast.IdentExpr); ok {
		decl, found := b.staticTypeValueDecl(ident)
		if !found {
			return staticPublicationSourceCandidate{}, false
		}
		return staticPublicationSourceCandidate{
			alias:  QualifiedTypeAlias{Decl: decl},
			source: []string{ident.Value},
		}, true
	}
	root, suffix, dotted := dottedTypeValuePath(expr)
	// PublicationRef retains one authored package/name pair. A longer dotted
	// RHS is not a proven publication until the canonical source reference can
	// represent it.
	if !dotted || root == nil || len(suffix) != 1 {
		return staticPublicationSourceCandidate{}, false
	}
	alias, found := b.qualifiedTypeAliasSource(expr)
	if !found || !alias.valid() {
		return staticPublicationSourceCandidate{}, false
	}
	return staticPublicationSourceCandidate{
		alias:  alias.copy(),
		source: []string{root.Value, suffix[0]},
	}, true
}

func (b *binder) projectQualifiedTypeAlias(root Symbol, suffix []string, alias QualifiedTypeAlias) {
	if b == nil || b.result == nil || root == 0 || len(suffix) == 0 || !alias.valid() {
		return
	}
	key := qualifiedTypeAliasKey{root: root, suffix: strings.Join(suffix, ".")}
	if previous, exists := b.qualifiedTypeAliases[key]; exists && !qualifiedTypeAliasEqual(previous, alias) {
		b.qualifiedTypeAliases[key] = QualifiedTypeAlias{}
		return
	}
	if b.qualifiedTypeAliases == nil {
		b.qualifiedTypeAliases = make(map[qualifiedTypeAliasKey]QualifiedTypeAlias)
	}
	b.qualifiedTypeAliases[key] = alias.copy()
}

func (b *binder) bindStaticPublicationSource(candidate staticPublicationSourceCandidate) (QualifiedTypeAlias, []string, bool) {
	if candidate.alias.valid() && len(candidate.source) != 0 {
		return candidate.alias.copy(), append([]string(nil), candidate.source...), true
	}
	return QualifiedTypeAlias{}, nil, false
}

// staticTypeValueDecl resolves the lexical declaration used by canonical
// publication evidence. It is intentionally separate from the closed
// RuntimeTypeValue authority used for compiler-special call bases.
func (b *binder) staticTypeValueDecl(ident *ast.IdentExpr) (TypeDecl, bool) {
	if b == nil || b.result == nil || ident == nil || ident.Value == "" {
		return TypeDecl{}, false
	}
	decl, typed := b.lookupType(ident.Value)
	if !typed || (decl.Kind != TypeDeclAlias && decl.Kind != TypeDeclInterface && decl.Kind != TypeDeclParam) {
		return TypeDecl{}, false
	}
	_, global, found := b.lookup(ident.Value)
	if found && !global {
		return TypeDecl{}, false
	}
	return decl, true
}

func (b *binder) qualifiedTypeAliasSource(expr ast.Expr) (QualifiedTypeAlias, bool) {
	if ident, ok := expr.(*ast.IdentExpr); ok {
		decl, found := b.typeValueRefs[ident]
		return QualifiedTypeAlias{Decl: decl}, found
	}
	root, suffix, ok := dottedTypeValuePath(expr)
	if !ok || len(suffix) == 0 {
		return QualifiedTypeAlias{}, false
	}
	if rootID, found := b.result.SymbolOf(root); found {
		key := qualifiedTypeAliasKey{root: rootID, suffix: strings.Join(suffix, ".")}
		if alias := b.qualifiedTypeAliases[key]; alias.valid() {
			return alias.copy(), true
		}
	}
	// A dotted value expression is not type evidence. Imported/module paths
	// become canonical only after an authoritative resolver or an earlier
	// proven publication has established this exact symbol/path pair.
	return QualifiedTypeAlias{}, false
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

// QualifiedTypeRootSymbol returns the exact lexical value symbol selected as
// the root of one authored qualified type reference. Bare references have no
// value root.
func (r *Result) QualifiedTypeRootSymbol(ref *ast.TypeRefExpr) (Symbol, bool) {
	if r == nil || ref == nil {
		return 0, false
	}
	id, ok := r.qualifiedTypeRootSymbols[ref]
	return id, ok && id != 0
}

func (b *binder) bindPrimitiveTypeRef(expr *ast.PrimitiveTypeExpr) {
	if expr == nil {
		return
	}
	if _, builtin := programstatic.PrimitiveKindForName(expr.Name); builtin {
		return
	}
	decl, ok := b.lookupType(expr.Name)
	if !ok {
		return
	}
	b.result.primitiveTypeRefs[expr] = decl
}
