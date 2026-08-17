package bind

import (
	"strings"

	"github.com/wippyai/go-lua/compiler/ast"
)

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

func (a QualifiedTypeAlias) valid() bool {
	return a.Decl.ID != 0 || len(a.Path) != 0
}

func (a QualifiedTypeAlias) copy() QualifiedTypeAlias {
	a.Path = append([]string(nil), a.Path...)
	return a
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
