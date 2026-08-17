package bind

import (
	"github.com/wippyai/go-lua/analysis/program/static"
	"github.com/wippyai/go-lua/compiler/ast"
)

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

// runtimePrimitiveTypeName is the VM's OP_LOADTYPE builtin subset of the
// canonical primitive vocabulary. `function` is a static primitive spelling
// but has no runtime type singleton, while `self` is context-relative.
func runtimePrimitiveTypeName(name string) bool {
	kind, ok := static.PrimitiveKindForName(name)
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
