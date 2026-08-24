package inventory

import (
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Mismatch is one discrepancy between a declared reference and what actually
// exists in the repository at HEAD.
type Mismatch struct {
	Pos    Position
	Row    string // e.g. "Reducer StorageReducer"
	Field  string // e.g. "Implementation"
	Detail string
	// Confirmed reports whether the type checker proved the reference
	// absent (a GoSymbol naming no such function/method). When false, this
	// is a "requires solve" row: the AST/type-check pass could not settle
	// the question (typically a foreign relation/projection key that may be
	// declared in an axis base the AST does not scan as a single literal).
	Confirmed bool
}

// symbolResolver type-checks the Go packages a GoSymbol names, memoized
// across the lifetime of one manifest run.
type symbolResolver struct {
	repoRoot string
	byPath   map[string]*packages.Package
}

func newSymbolResolver(repoRoot string) *symbolResolver {
	return &symbolResolver{repoRoot: repoRoot, byPath: map[string]*packages.Package{}}
}

func (r *symbolResolver) load(importPath string) *packages.Package {
	if pkg, ok := r.byPath[importPath]; ok {
		return pkg
	}
	cfg := &packages.Config{
		Dir:  r.repoRoot,
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax,
	}
	loaded, err := packages.Load(cfg, importPath)
	var pkg *packages.Package
	if err == nil && len(loaded) == 1 && len(loaded[0].Errors) == 0 {
		pkg = loaded[0]
	}
	r.byPath[importPath] = pkg
	return pkg
}

// resolveGoSymbol reports whether the GoSymbol record names a real Go
// function or method: a package-level func when Receiver is absent, a
// method on Receiver's named type otherwise.
func (r *symbolResolver) resolveGoSymbol(sym Record) (found bool, checked bool) {
	pkgPath, ok := sym.Field("PackagePath")
	if !ok || !pkgPath.Literal || pkgPath.Value == "" {
		return false, false
	}
	name, ok := sym.Field("Name")
	if !ok || !name.Literal || name.Value == "" {
		return false, false
	}
	pkg := r.load(pkgPath.Value)
	if pkg == nil {
		return false, false
	}
	receiver, hasReceiver := sym.Field("Receiver")
	if hasReceiver && receiver.Nested != nil {
		receiverName, ok := receiver.Nested.Field("Name")
		if ok && receiverName.Literal && receiverName.Value != "" {
			return r.resolveMethod(pkg, receiverName.Value, name.Value), true
		}
	}
	obj := pkg.Types.Scope().Lookup(name.Value)
	if obj == nil {
		return false, true
	}
	_, isFunc := obj.(*types.Func)
	return isFunc, true
}

func (r *symbolResolver) resolveMethod(pkg *packages.Package, typeName, methodName string) bool {
	obj := pkg.Types.Scope().Lookup(typeName)
	if obj == nil {
		return false
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return false
	}
	for i := 0; i < named.NumMethods(); i++ {
		if named.Method(i).Name() == methodName {
			return true
		}
	}
	// Pointer-receiver methods are not attached to the named type's own
	// method set; check *T as well.
	ptr := types.NewMethodSet(types.NewPointer(named))
	for i := 0; i < ptr.Len(); i++ {
		if ptr.At(i).Obj().Name() == methodName {
			return true
		}
	}
	return false
}

// domainKeyIndex is a best-effort, repository-wide census of every quoted
// string literal appearing anywhere under domain/, used to check whether a
// foreign relation/projection key a declaration references is spelled
// anywhere in the repository. It is deliberately coarse (source text, not
// composition semantics): a hit only rules a "not found" verdict in; it
// never rules a reference confirmed-broken, because axis base declarations
// are built with Go code this package does not evaluate.
type domainKeyIndex struct {
	keys map[string]struct{}
}

var quotedStringPattern = regexp.MustCompile(`"([^"\\]|\\.)*"`)

func buildDomainKeyIndex(repoRoot string) (*domainKeyIndex, error) {
	idx := &domainKeyIndex{keys: map[string]struct{}{}}
	root := filepath.Join(repoRoot, "domain")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range quotedStringPattern.FindAll(src, -1) {
			s := string(m)
			if unquoted, err := unquoteLoose(s); err == nil {
				idx.keys[unquoted] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("index domain/ keys: %w", err)
	}
	return idx, nil
}

func unquoteLoose(s string) (string, error) {
	if len(s) < 2 {
		return "", fmt.Errorf("too short")
	}
	return s[1 : len(s)-1], nil
}

func (idx *domainKeyIndex) has(key string) bool {
	_, ok := idx.keys[key]
	return ok
}

// Verify inspects every GoSymbol and foreign relation-key reference declared
// across pkg's declarations and reports the ones a read-only pass can settle
// or flag as needing the solve step. repoRoot is used to load referenced Go
// packages and to build the repository-wide key census.
func Verify(pkg Package, repoRoot string) ([]Mismatch, error) {
	if len(pkg.Declarations) == 0 {
		return nil, nil
	}
	resolver := newSymbolResolver(repoRoot)
	keyIndex, err := buildDomainKeyIndex(repoRoot)
	if err != nil {
		return nil, err
	}

	var mismatches []Mismatch
	checkSymbol := func(row string, field string, rec Record, symField string) {
		sym, ok := rec.Field(symField)
		if !ok || sym.Nested == nil {
			return
		}
		found, checked := resolver.resolveGoSymbol(*sym.Nested)
		if !checked || found {
			return
		}
		pkgPath := sym.Nested.FieldValue("PackagePath")
		name := sym.Nested.FieldValue("Name")
		receiver := ""
		if r, ok := sym.Nested.Field("Receiver"); ok && r.Nested != nil {
			receiver = r.Nested.FieldValue("Name")
		}
		detail := fmt.Sprintf("%s.%s: no such function", pkgPath, name)
		if receiver != "" {
			detail = fmt.Sprintf("%s.%s has no method %s", pkgPath, receiver, name)
		}
		mismatches = append(mismatches, Mismatch{
			Pos:       rec.Pos,
			Row:       row,
			Field:     field,
			Detail:    detail,
			Confirmed: true,
		})
	}
	checkForeignKey := func(row string, field string, rec Record, refField string) {
		ref, ok := rec.Field(refField)
		if !ok || ref.Nested == nil {
			return
		}
		member, ok := ref.Nested.Field("Member")
		if !ok || !member.Literal || member.Value == "" {
			return
		}
		if keyIndex.has(member.Value) {
			return
		}
		mismatches = append(mismatches, Mismatch{
			Pos:       rec.Pos,
			Row:       row,
			Field:     field,
			Detail:    fmt.Sprintf("member key %q not found as a string literal anywhere under domain/ - requires solve (may be an axis base declaration this pass does not evaluate; confirm with solvedump)", member.Value),
			Confirmed: false,
		})
	}

	for _, decl := range pkg.Declarations {
		for _, relation := range decl.Relations {
			row := "Relation " + relation.FieldValue("Name")
			checkSymbol(row, "CandidateResolver", relation, "CandidateResolver")
			checkSymbol(row, "CandidateOrdinal", relation, "CandidateOrdinal")
			checkSymbol(row, "CandidateAt", relation, "CandidateAt")
			checkSymbol(row, "CandidateCount", relation, "CandidateCount")
			checkSymbol(row, "Materialize", relation, "Materialize")
			checkSymbol(row, "CandidateIdentityAt", relation, "CandidateIdentityAt")
			checkForeignKey(row, "CandidateProvider", relation, "CandidateProvider")
			checkForeignKey(row, "MemberParent", relation, "MemberParent")
			if derivation, ok := relation.Field("Derivation"); ok && derivation.Nested != nil {
				checkSymbol(row, "Derivation.Build", *derivation.Nested, "Build")
				checkSymbol(row, "Derivation.Count", *derivation.Nested, "Count")
				checkSymbol(row, "Derivation.At", *derivation.Nested, "At")
			}
		}
		for _, projection := range decl.Projections {
			row := "Projection " + projection.FieldValue("Name")
			checkSymbol(row, "Accessor", projection, "Accessor")
			checkForeignKey(row, "CandidateProvider", projection, "CandidateProvider")
		}
		for _, reducer := range decl.Reducers {
			row := "Reducer " + reducer.FieldValue("Name")
			checkSymbol(row, "Implementation", reducer, "Implementation")
		}
	}

	sort.Slice(mismatches, func(i, j int) bool {
		if mismatches[i].Pos.String() != mismatches[j].Pos.String() {
			return mismatches[i].Pos.String() < mismatches[j].Pos.String()
		}
		return mismatches[i].Field < mismatches[j].Field
	})
	return mismatches, nil
}
