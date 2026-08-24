// Package inventory walks one domain package's declaration surface through
// its memberdefinition child package's Contribution() function: the member
// definitions, relations, projections, carriers and reducers a family
// declares for the generated composition to fold.
//
// Extraction is syntactic (go/ast over a go/packages typed load): a field
// assigned a plain string literal, or an identifier the type checker folds
// to a constant string (the common heapPackagePath-style const), is captured
// as that string. A field assigned anything else - a call, an identifier
// with no constant value - is captured as the rendered Go source text of the
// expression. Nothing is evaluated beyond constant folding, so a helper such
// as heapAxis() is recorded as the call text "heapAxis()", not resolved to a
// value. This package never writes to the repository and never imports
// analysis/engine.
package inventory

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Field is one struct field captured from a composite literal in the
// declaration source.
type Field struct {
	Name string
	// Value is the field's literal (or constant-folded) string when
	// available, otherwise the rendered Go source text of whatever
	// expression the field was assigned.
	Value string
	// Literal reports whether Value is a plain string literal or a
	// constant-folded string, as opposed to rendered expression source.
	Literal bool
	// Nested is set when the field's value was itself a composite literal
	// (a GoSymbol, a RelationRef, a RelationDerivation, ...), giving access
	// to its own fields.
	Nested *Record
}

// Record is one composite literal captured from the declaration surface: a
// Carrier, Relation, Projection, Reducer row, or a nested value such as a
// RelationDerivation or a RelationRef.
type Record struct {
	TypeName string
	Fields   []Field
	Pos      Position
}

// Field looks up a named field on the record.
func (r Record) Field(name string) (Field, bool) {
	for _, f := range r.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// FieldValue returns the named field's captured value, or "" when absent.
func (r Record) FieldValue(name string) string {
	f, _ := r.Field(name)
	return f.Value
}

// Position is a source location rendered file:line, relative to the module
// root when the module root is known.
type Position struct {
	File string
	Line int
}

func (p Position) String() string {
	if p.File == "" {
		return "-"
	}
	return fmt.Sprintf("%s:%d", p.File, p.Line)
}

func (p Position) IsZero() bool { return p.File == "" }

// Declaration is one Contribution() function's declared surface.
type Declaration struct {
	FuncPos Position
	Axis    Field
	Rule    Field

	Carriers    []Record
	Relations   []Record
	Projections []Record
	Reducers    []Record
}

// Package is the declaration surface of one domain package's
// memberdefinition child package.
type Package struct {
	// ImportPath is the domain package as given on the command line, e.g.
	// "domain/placement/store" (repository-relative, no module prefix).
	ImportPath string
	// MemberDefinitionRelDir is ImportPath + "/memberdefinition", the
	// directory this inventory walked.
	MemberDefinitionRelDir string
	// Present reports whether that directory exists at all.
	Present bool
	// Declarations holds every Contribution() function found (normally one).
	Declarations []Declaration
	// LoadErrors carries any go/packages load diagnostics; a present
	// directory that fails to load still reports Present true with errors
	// recorded here rather than failing the whole walk.
	LoadErrors []string

	// ModuleDir is the module root the loader resolved, used to render
	// positions relative to the repository. Empty when unknown.
	ModuleDir string
}

type walkContext struct {
	fset      *token.FileSet
	info      *types.Info
	moduleDir string
	// funcs indexes the package's own top-level, receiver-less functions by
	// name, so a one-line factory helper such as heapMethod(...) or
	// axisReference(...) can be inlined at its call site instead of being
	// rendered as opaque call-expression text.
	funcs map[string]*ast.FuncDecl
}

// scope binds a callee's parameter names to the already-scoped argument
// expressions supplied at one call site, so inlining a helper's single
// return expression can resolve the parameters it references.
type scope map[string]scopedExpr

type scopedExpr struct {
	expr  ast.Expr
	scope scope
}

const maxInlineDepth = 8

// Load walks repoRoot/domainPkg/memberdefinition and extracts every
// Contribution() declaration it finds. domainPkg is repository-relative,
// e.g. "domain/heap/allocation/empty". A missing memberdefinition directory
// is not an error: Present is false and Declarations is empty.
func Load(repoRoot, domainPkg string) (Package, error) {
	relDir := path.Join(filepath.ToSlash(domainPkg), "memberdefinition")
	pkg := Package{ImportPath: domainPkg, MemberDefinitionRelDir: relDir}

	absDir := filepath.Join(repoRoot, filepath.FromSlash(relDir))
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		return pkg, nil
	}
	pkg.Present = true

	loadedPkg, err := loadOne(repoRoot, "./"+relDir)
	if err != nil {
		return pkg, fmt.Errorf("load %s: %w", relDir, err)
	}
	if loadedPkg == nil {
		pkg.LoadErrors = append(pkg.LoadErrors, "go/packages returned no package for "+relDir)
		return pkg, nil
	}
	for _, e := range loadedPkg.Errors {
		pkg.LoadErrors = append(pkg.LoadErrors, e.Error())
	}
	if loadedPkg.Module != nil {
		pkg.ModuleDir = loadedPkg.Module.Dir
	}

	ctx := &walkContext{
		fset:      loadedPkg.Fset,
		info:      loadedPkg.TypesInfo,
		moduleDir: pkg.ModuleDir,
		funcs:     map[string]*ast.FuncDecl{},
	}
	for _, file := range loadedPkg.Syntax {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil {
				continue
			}
			ctx.funcs[fd.Name.Name] = fd
		}
	}
	for _, file := range loadedPkg.Syntax {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name.Name != "Contribution" || fd.Recv != nil || fd.Body == nil {
				continue
			}
			declaration, ok := buildDeclaration(fd, ctx)
			if ok {
				pkg.Declarations = append(pkg.Declarations, declaration)
			}
		}
	}
	sort.Slice(pkg.Declarations, func(i, j int) bool {
		return pkg.Declarations[i].FuncPos.String() < pkg.Declarations[j].FuncPos.String()
	})
	return pkg, nil
}

func loadOne(repoRoot, pattern string) (*packages.Package, error) {
	cfg := &packages.Config{
		Dir: repoRoot,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedModule,
	}
	loaded, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, err
	}
	if len(loaded) == 0 {
		return nil, nil
	}
	return loaded[0], nil
}

func buildDeclaration(fd *ast.FuncDecl, ctx *walkContext) (Declaration, bool) {
	var decl Declaration
	decl.FuncPos = position(ctx, fd.Pos())

	var lit *ast.CompositeLit
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return true
		}
		if cl, ok := ret.Results[0].(*ast.CompositeLit); ok {
			lit = cl
		}
		return true
	})
	if lit == nil {
		return decl, false
	}

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name := exprString(kv.Key, ctx.fset)
		switch name {
		case "Axis":
			decl.Axis = fieldFromValue(kv.Value, ctx, nil, 0)
		case "Rule":
			decl.Rule = fieldFromValue(kv.Value, ctx, nil, 0)
		case "Carriers":
			decl.Carriers = explodeList(kv.Value, ctx, nil, 0)
		case "Relations":
			decl.Relations = explodeList(kv.Value, ctx, nil, 0)
		case "Projections":
			decl.Projections = explodeList(kv.Value, ctx, nil, 0)
		case "Reducers":
			decl.Reducers = explodeList(kv.Value, ctx, nil, 0)
		}
	}
	return decl, true
}

func explodeList(value ast.Expr, ctx *walkContext, sc scope, depth int) []Record {
	cl, ok := value.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var records []Record
	for _, elt := range cl.Elts {
		inner, ok := elt.(*ast.CompositeLit)
		if !ok {
			continue
		}
		records = append(records, genericRecord(inner, ctx, sc, depth))
	}
	return records
}

func genericRecord(lit *ast.CompositeLit, ctx *walkContext, sc scope, depth int) Record {
	rec := Record{
		TypeName: typeNameOf(lit, ctx),
		Pos:      position(ctx, lit.Pos()),
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			// A positional element of a slice-of-struct literal (each row
			// of Relations, Reducers, ReducerInputs, ...): recurse so its
			// own fields are captured under an auto-numbered name instead
			// of being flattened to source text.
			f := fieldFromValue(elt, ctx, sc, depth)
			f.Name = fmt.Sprintf("_%d", len(rec.Fields))
			rec.Fields = append(rec.Fields, f)
			continue
		}
		name := exprString(kv.Key, ctx.fset)
		f := fieldFromValue(kv.Value, ctx, sc, depth)
		f.Name = name
		rec.Fields = append(rec.Fields, f)
	}
	return rec
}

// typeNameOf renders a composite literal's type. A literal nested inside a
// slice/array of structs elides its own type (e.g. the second row of
// []Relation{{...}, {...}}), so lit.Type is nil; the type checker still
// resolved one, and that is used instead of guessing from context.
func typeNameOf(lit *ast.CompositeLit, ctx *walkContext) string {
	if lit.Type != nil {
		return exprString(lit.Type, ctx.fset)
	}
	if ctx.info != nil {
		if tv, ok := ctx.info.Types[lit]; ok && tv.Type != nil {
			return shortTypeName(tv.Type.String())
		}
	}
	return ""
}

func shortTypeName(s string) string {
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		return s[idx+1:]
	}
	return s
}

// fieldFromValue captures one field's assigned expression: a literal or
// constant-folded string, a local-scope parameter reference, a call to a
// same-package single-return factory helper (inlined up to maxInlineDepth),
// a nested composite literal, or - the fallback - the expression's rendered
// source text.
func fieldFromValue(value ast.Expr, ctx *walkContext, sc scope, depth int) Field {
	if lit, ok := value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		if s, err := strconv.Unquote(lit.Value); err == nil {
			return Field{Value: s, Literal: true}
		}
	}
	if s, ok := constantString(value, ctx.info); ok {
		return Field{Value: s, Literal: true}
	}
	if id, ok := value.(*ast.Ident); ok {
		if bound, ok := sc[id.Name]; ok {
			return fieldFromValue(bound.expr, ctx, bound.scope, depth)
		}
	}
	if call, ok := value.(*ast.CallExpr); ok {
		// A conversion to a named string type (schema.Key(key), member.Carrier(name), ...)
		// does not change the value once its argument resolves to a string,
		// so it is unwrapped rather than rendered as call-expression text.
		if len(call.Args) == 1 && isTypeConversion(call, ctx.info) {
			if arg := fieldFromValue(call.Args[0], ctx, sc, depth); arg.Literal {
				return Field{Value: arg.Value, Literal: true}
			}
		}
		if depth < maxInlineDepth {
			if inlined, inlinedScope, ok := tryInlineCall(call, ctx, sc); ok {
				return fieldFromValue(inlined, ctx, inlinedScope, depth+1)
			}
		}
	}
	if cl, ok := value.(*ast.CompositeLit); ok {
		nested := genericRecord(cl, ctx, sc, depth)
		return Field{Value: exprString(value, ctx.fset), Nested: &nested}
	}
	return Field{Value: exprString(value, ctx.fset)}
}

// tryInlineCall inlines a call to a same-package, receiver-less function
// whose body is exactly one return statement - the trivial factory-helper
// shape this codebase writes GoSymbol and RelationRef constructors with
// (heapMethod, axisReference, heapAxis, ...). It reports ok=false for
// anything else (multi-statement bodies, variadic or unnamed parameters,
// calls to functions outside the package), which callers render as source
// text instead of guessing.
func tryInlineCall(call *ast.CallExpr, ctx *walkContext, callerScope scope) (ast.Expr, scope, bool) {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	fd, ok := ctx.funcs[ident.Name]
	if !ok || fd.Body == nil || len(fd.Body.List) != 1 {
		return nil, nil, false
	}
	ret, ok := fd.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil, nil, false
	}
	newScope := scope{}
	if fd.Type.Params != nil {
		argIndex := 0
		for _, field := range fd.Type.Params.List {
			if len(field.Names) == 0 {
				return nil, nil, false
			}
			if _, variadic := field.Type.(*ast.Ellipsis); variadic {
				return nil, nil, false
			}
			for _, paramName := range field.Names {
				if argIndex >= len(call.Args) {
					return nil, nil, false
				}
				newScope[paramName.Name] = scopedExpr{expr: call.Args[argIndex], scope: callerScope}
				argIndex++
			}
		}
	}
	return ret.Results[0], newScope, true
}

// isTypeConversion reports whether call is a conversion to a named type
// (schema.Key(...), member.Carrier(...)) rather than a function call.
func isTypeConversion(call *ast.CallExpr, info *types.Info) bool {
	if info == nil {
		return false
	}
	var used *ast.Ident
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		used = fn
	case *ast.SelectorExpr:
		used = fn.Sel
	default:
		return false
	}
	obj, ok := info.Uses[used]
	if !ok {
		return false
	}
	_, isType := obj.(*types.TypeName)
	return isType
}

// constantString reports the string value of expr when the type checker
// folded it to a constant string - the heapPackagePath-style named-const
// pattern this codebase writes GoSymbol package paths and keys with.
func constantString(expr ast.Expr, info *types.Info) (string, bool) {
	if info == nil {
		return "", false
	}
	tv, ok := info.Types[expr]
	if !ok || tv.Value == nil {
		return "", false
	}
	if tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

func exprString(expr ast.Node, fset *token.FileSet) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, expr); err != nil {
		return fmt.Sprintf("<unrenderable: %v>", err)
	}
	return buf.String()
}

func position(ctx *walkContext, pos token.Pos) Position {
	p := ctx.fset.Position(pos)
	file := p.Filename
	if ctx.moduleDir != "" {
		if rel, err := filepath.Rel(ctx.moduleDir, file); err == nil {
			file = filepath.ToSlash(rel)
		}
	}
	return Position{File: file, Line: p.Line}
}
