package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/wippyai/go-lua"

type fenceGoListPackage struct {
	Dir        string
	ImportPath string
	Name       string
	GoFiles    []string
	Export     string
	Standard   bool
}

type fenceTypedPackage struct {
	meta  fenceGoListPackage
	fset  *token.FileSet
	files []*ast.File
	info  *types.Info
	pkg   *types.Package
}

type fenceOwnedType struct {
	packagePath string
	name        string
	typed       types.Type
}

type fencePackageLoader struct {
	t       testing.TB
	root    string
	metas   map[string]fenceGoListPackage
	imports types.Importer
}

// A semantic fence must ask the compiler what an expression denotes. The
// export graph supplies the same package identities used by the build, while
// each package under inspection is checked from source so aliases, method
// selections, composite literals, and local assignments remain visible.
func newFencePackageLoader(t testing.TB, patterns ...string) *fencePackageLoader {
	t.Helper()
	root := wireFenceRepositoryRootTB(t)
	args := append([]string{"list", "-deps", "-export", "-json"}, patterns...)
	command := exec.Command("go", args...)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list semantic fence graph: %v\n%s", err, exit.Stderr)
		}
		t.Fatalf("go list semantic fence graph: %v", err)
	}
	metas := make(map[string]fenceGoListPackage)
	decoder := json.NewDecoder(bytes.NewReader(output))
	for decoder.More() {
		var meta fenceGoListPackage
		if err := decoder.Decode(&meta); err != nil {
			t.Fatalf("decode go list semantic fence graph: %v", err)
		}
		metas[meta.ImportPath] = meta
	}
	lookup := func(path string) (io.ReadCloser, error) {
		meta, ok := metas[path]
		if !ok || meta.Export == "" {
			return nil, fmt.Errorf("semantic fence: no export data for %s", path)
		}
		return os.Open(meta.Export)
	}
	return &fencePackageLoader{
		t:       t,
		root:    root,
		metas:   metas,
		imports: importer.ForCompiler(token.NewFileSet(), "gc", lookup),
	}
}

func wireFenceRepositoryRootTB(t testing.TB) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil && !info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not find repository go.mod")
		}
		directory = parent
	}
}

func (loader *fencePackageLoader) modulePackages(prefix string) []fenceGoListPackage {
	var metas []fenceGoListPackage
	for _, meta := range loader.metas {
		if strings.HasPrefix(meta.ImportPath, modulePath+prefix) && len(meta.GoFiles) != 0 {
			metas = append(metas, meta)
		}
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].ImportPath < metas[j].ImportPath })
	return metas
}

func (loader *fencePackageLoader) load(meta fenceGoListPackage) *fenceTypedPackage {
	loader.t.Helper()
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(meta.GoFiles))
	for _, name := range meta.GoFiles {
		path := filepath.Join(meta.Dir, name)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			loader.t.Fatalf("parse %s: %v", path, err)
		}
		files = append(files, file)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
	}
	config := types.Config{Importer: loader.imports}
	pkg, err := config.Check(meta.ImportPath, fset, files, info)
	if err != nil {
		loader.t.Fatalf("type-check %s: %v", meta.ImportPath, err)
	}
	return &fenceTypedPackage{meta: meta, fset: fset, files: files, info: info, pkg: pkg}
}

func (loader *fencePackageLoader) source(importPath, source string) *fenceTypedPackage {
	loader.t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "mutation.go", source, 0)
	if err != nil {
		loader.t.Fatalf("parse semantic mutation: %v", err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
	}
	config := types.Config{Importer: loader.imports}
	pkg, err := config.Check(importPath, fset, []*ast.File{file}, info)
	if err != nil {
		loader.t.Fatalf("type-check semantic mutation: %v", err)
	}
	return &fenceTypedPackage{
		meta:  fenceGoListPackage{ImportPath: importPath, Name: file.Name.Name},
		fset:  fset,
		files: []*ast.File{file},
		info:  info,
		pkg:   pkg,
	}
}

func (loader *fencePackageLoader) sourceError(importPath, source string) error {
	loader.t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "mutation.go", source, 0)
	if err != nil {
		return err
	}
	config := types.Config{Importer: loader.imports}
	_, err = config.Check(importPath, fset, []*ast.File{file}, nil)
	return err
}

func (loader *fencePackageLoader) ownedType(packagePath, name string) fenceOwnedType {
	loader.t.Helper()
	pkg, err := loader.imports.Import(packagePath)
	if err != nil {
		loader.t.Fatalf("import semantic fence owner package %s: %v", packagePath, err)
	}
	object, ok := pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok {
		loader.t.Fatalf("semantic fence owner type %s.%s not found", packagePath, name)
	}
	return fenceOwnedType{packagePath: packagePath, name: name, typed: object.Type()}
}

type fenceValue struct {
	texts     map[string]struct{}
	functions map[*types.Func]struct{}
	role      bool
	astName   bool
	trusted   bool
}

func fenceKnownText(text string) fenceValue {
	return fenceValue{texts: map[string]struct{}{text: {}}}
}

func fenceTrustedText(text string) fenceValue {
	return fenceValue{texts: map[string]struct{}{text: {}}, trusted: true}
}

func fenceJoinValue(left, right fenceValue) fenceValue {
	out := fenceValue{role: left.role || right.role, astName: left.astName || right.astName}
	out.functions = fenceCloneFunctions(left.functions)
	for function := range right.functions {
		if out.functions == nil {
			out.functions = make(map[*types.Func]struct{})
		}
		out.functions[function] = struct{}{}
	}
	if len(left.texts) == 0 {
		out.texts = fenceCloneTexts(right.texts)
		out.trusted = right.trusted
		return out
	}
	if len(right.texts) == 0 {
		out.texts = fenceCloneTexts(left.texts)
		out.trusted = left.trusted
		return out
	}
	out.trusted = left.trusted && right.trusted
	out.texts = fenceCloneTexts(left.texts)
	for text := range right.texts {
		if len(out.texts) >= 16 {
			break
		}
		out.texts[text] = struct{}{}
	}
	return out
}

func fenceConcatValue(left, right fenceValue) fenceValue {
	out := fenceValue{role: left.role || right.role, astName: left.astName || right.astName}
	if len(left.texts) == 0 || len(right.texts) == 0 {
		return out
	}
	out.texts = make(map[string]struct{})
	for a := range left.texts {
		for b := range right.texts {
			if len(out.texts) >= 16 {
				return out
			}
			out.texts[a+b] = struct{}{}
		}
	}
	return out
}

func fenceCloneTexts(source map[string]struct{}) map[string]struct{} {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(source))
	for text := range source {
		out[text] = struct{}{}
	}
	return out
}

func fenceCloneFunctions(source map[*types.Func]struct{}) map[*types.Func]struct{} {
	if len(source) == 0 {
		return nil
	}
	out := make(map[*types.Func]struct{}, len(source))
	for function := range source {
		out[function] = struct{}{}
	}
	return out
}

type fenceSemanticAnalyzer struct {
	typed    *fenceTypedPackage
	funcs    map[*types.Func]*ast.FuncDecl
	active   map[*types.Func]bool
	values   map[ast.Expr]fenceValue
	onExpr   func(ast.Expr, fenceValue)
	maxDepth int
}

func newFenceSemanticAnalyzer(typed *fenceTypedPackage) *fenceSemanticAnalyzer {
	analyzer := &fenceSemanticAnalyzer{
		typed:    typed,
		funcs:    make(map[*types.Func]*ast.FuncDecl),
		active:   make(map[*types.Func]bool),
		values:   make(map[ast.Expr]fenceValue),
		maxDepth: 12,
	}
	for _, file := range typed.files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if object, ok := typed.info.Defs[function.Name].(*types.Func); ok {
				analyzer.funcs[object] = function
			}
		}
	}
	return analyzer
}

func (analyzer *fenceSemanticAnalyzer) analyzeAll(onExpr func(ast.Expr, fenceValue)) {
	analyzer.onExpr = onExpr
	globals := make(map[types.Object]fenceValue)
	for _, file := range analyzer.typed.files {
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range generic.Specs {
				if values, ok := specification.(*ast.ValueSpec); ok {
					analyzer.bindValueSpec(values, globals, 0)
				}
			}
		}
	}
	for object := range analyzer.funcs {
		analyzer.call(object, nil, globals, 0)
	}
}

func (analyzer *fenceSemanticAnalyzer) call(object *types.Func, arguments []fenceValue, globals map[types.Object]fenceValue, depth int) fenceValue {
	if object == nil || depth > analyzer.maxDepth || analyzer.active[object] {
		return fenceValue{}
	}
	declaration := analyzer.funcs[object]
	if declaration == nil || declaration.Body == nil {
		return fenceValue{}
	}
	analyzer.active[object] = true
	defer delete(analyzer.active, object)
	env := fenceCloneEnv(globals)
	signature, _ := object.Type().(*types.Signature)
	if signature != nil {
		for index := 0; index < signature.Params().Len() && index < len(arguments); index++ {
			env[signature.Params().At(index)] = arguments[index]
		}
		if signature.Recv() != nil && declaration.Recv != nil && len(declaration.Recv.List) != 0 &&
			len(declaration.Recv.List[0].Names) != 0 && len(arguments) > signature.Params().Len() {
			if receiver := analyzer.typed.info.Defs[declaration.Recv.List[0].Names[0]]; receiver != nil {
				env[receiver] = arguments[len(arguments)-1]
			}
		}
	}
	return analyzer.block(declaration.Body.List, env, globals, depth+1)
}

func fenceCloneEnv(source map[types.Object]fenceValue) map[types.Object]fenceValue {
	out := make(map[types.Object]fenceValue, len(source))
	for object, value := range source {
		value.texts = fenceCloneTexts(value.texts)
		value.functions = fenceCloneFunctions(value.functions)
		out[object] = value
	}
	return out
}

func fenceMergeEnv(destination map[types.Object]fenceValue, sources ...map[types.Object]fenceValue) {
	for _, source := range sources {
		for object, value := range source {
			destination[object] = fenceJoinValue(destination[object], value)
		}
	}
}

func (analyzer *fenceSemanticAnalyzer) block(statements []ast.Stmt, env, globals map[types.Object]fenceValue, depth int) fenceValue {
	returned := fenceValue{}
	for _, statement := range statements {
		switch item := statement.(type) {
		case *ast.AssignStmt:
			values := make([]fenceValue, len(item.Rhs))
			for index, expression := range item.Rhs {
				values[index] = analyzer.expr(expression, env, globals, depth)
			}
			for index, expression := range item.Lhs {
				identifier, ok := expression.(*ast.Ident)
				if !ok {
					analyzer.expr(expression, env, globals, depth)
					continue
				}
				object := analyzer.typed.info.Uses[identifier]
				if item.Tok == token.DEFINE {
					object = analyzer.typed.info.Defs[identifier]
				}
				if object == nil || len(values) == 0 {
					continue
				}
				valueIndex := index
				if valueIndex >= len(values) {
					valueIndex = len(values) - 1
				}
				env[object] = values[valueIndex]
			}
		case *ast.DeclStmt:
			declaration, ok := item.Decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range declaration.Specs {
				if values, ok := specification.(*ast.ValueSpec); ok {
					analyzer.bindValueSpec(values, env, depth)
				}
			}
		case *ast.ExprStmt:
			analyzer.expr(item.X, env, globals, depth)
		case *ast.ReturnStmt:
			for _, expression := range item.Results {
				returned = fenceJoinValue(returned, analyzer.expr(expression, env, globals, depth))
			}
		case *ast.IfStmt:
			if item.Init != nil {
				analyzer.block([]ast.Stmt{item.Init}, env, globals, depth)
			}
			analyzer.expr(item.Cond, env, globals, depth)
			thenEnv := fenceCloneEnv(env)
			elseEnv := fenceCloneEnv(env)
			returned = fenceJoinValue(returned, analyzer.block(item.Body.List, thenEnv, globals, depth))
			if item.Else != nil {
				returned = fenceJoinValue(returned, analyzer.statement(item.Else, elseEnv, globals, depth))
			}
			fenceMergeEnv(env, thenEnv, elseEnv)
		case *ast.ForStmt:
			if item.Init != nil {
				analyzer.block([]ast.Stmt{item.Init}, env, globals, depth)
			}
			if item.Cond != nil {
				analyzer.expr(item.Cond, env, globals, depth)
			}
			loopEnv := fenceCloneEnv(env)
			returned = fenceJoinValue(returned, analyzer.block(item.Body.List, loopEnv, globals, depth))
			if item.Post != nil {
				analyzer.block([]ast.Stmt{item.Post}, loopEnv, globals, depth)
			}
			fenceMergeEnv(env, loopEnv)
		case *ast.RangeStmt:
			analyzer.expr(item.X, env, globals, depth)
			loopEnv := fenceCloneEnv(env)
			returned = fenceJoinValue(returned, analyzer.block(item.Body.List, loopEnv, globals, depth))
			fenceMergeEnv(env, loopEnv)
		case *ast.BlockStmt:
			returned = fenceJoinValue(returned, analyzer.block(item.List, env, globals, depth))
		case *ast.SwitchStmt:
			if item.Init != nil {
				analyzer.block([]ast.Stmt{item.Init}, env, globals, depth)
			}
			if item.Tag != nil {
				analyzer.expr(item.Tag, env, globals, depth)
			}
			for _, clause := range item.Body.List {
				branch := fenceCloneEnv(env)
				caseClause := clause.(*ast.CaseClause)
				for _, expression := range caseClause.List {
					analyzer.expr(expression, branch, globals, depth)
				}
				returned = fenceJoinValue(returned, analyzer.block(caseClause.Body, branch, globals, depth))
				fenceMergeEnv(env, branch)
			}
		case *ast.TypeSwitchStmt:
			if item.Init != nil {
				analyzer.block([]ast.Stmt{item.Init}, env, globals, depth)
			}
			analyzer.block([]ast.Stmt{item.Assign}, env, globals, depth)
			for _, clause := range item.Body.List {
				branch := fenceCloneEnv(env)
				returned = fenceJoinValue(returned, analyzer.block(clause.(*ast.CaseClause).Body, branch, globals, depth))
				fenceMergeEnv(env, branch)
			}
		case *ast.GoStmt:
			analyzer.expr(item.Call, env, globals, depth)
		case *ast.DeferStmt:
			analyzer.expr(item.Call, env, globals, depth)
		case *ast.SendStmt:
			analyzer.expr(item.Chan, env, globals, depth)
			analyzer.expr(item.Value, env, globals, depth)
		case *ast.IncDecStmt:
			analyzer.expr(item.X, env, globals, depth)
		case *ast.LabeledStmt:
			returned = fenceJoinValue(returned, analyzer.statement(item.Stmt, env, globals, depth))
		}
	}
	return returned
}

func (analyzer *fenceSemanticAnalyzer) statement(statement ast.Stmt, env, globals map[types.Object]fenceValue, depth int) fenceValue {
	if block, ok := statement.(*ast.BlockStmt); ok {
		return analyzer.block(block.List, env, globals, depth)
	}
	return analyzer.block([]ast.Stmt{statement}, env, globals, depth)
}

func (analyzer *fenceSemanticAnalyzer) bindValueSpec(specification *ast.ValueSpec, env map[types.Object]fenceValue, depth int) {
	values := make([]fenceValue, len(specification.Values))
	for index, expression := range specification.Values {
		values[index] = analyzer.expr(expression, env, env, depth)
	}
	for index, name := range specification.Names {
		object := analyzer.typed.info.Defs[name]
		if object == nil || len(values) == 0 {
			continue
		}
		valueIndex := index
		if valueIndex >= len(values) {
			valueIndex = len(values) - 1
		}
		env[object] = values[valueIndex]
	}
}

func (analyzer *fenceSemanticAnalyzer) expr(expression ast.Expr, env, globals map[types.Object]fenceValue, depth int) fenceValue {
	if expression == nil {
		return fenceValue{}
	}
	var value fenceValue
	if typed := analyzer.typed.info.Types[expression]; typed.Value != nil && typed.Value.Kind() == constant.String {
		text := constant.StringVal(typed.Value)
		if fenceOwnerStringConstant(analyzer.typed.info, expression) {
			value = fenceTrustedText(text)
		} else {
			value = fenceKnownText(text)
		}
	}
	switch item := expression.(type) {
	case *ast.ParenExpr:
		value = fenceJoinValue(value, analyzer.expr(item.X, env, globals, depth))
	case *ast.Ident:
		object := analyzer.typed.info.Uses[item]
		if object == nil {
			object = analyzer.typed.info.Defs[item]
		}
		if bound, ok := env[object]; ok && (len(bound.texts) != 0 || len(bound.functions) != 0 || bound.role || bound.astName) {
			value = bound
		} else if constantObject, ok := object.(*types.Const); ok && constantObject.Val().Kind() == constant.String {
			value = fenceJoinValue(value, fenceKnownText(constant.StringVal(constantObject.Val())))
		} else if function, ok := object.(*types.Func); ok {
			value.functions = map[*types.Func]struct{}{function: {}}
		}
	case *ast.SelectorExpr:
		analyzer.expr(item.X, env, globals, depth)
		if selection := analyzer.typed.info.Selections[item]; selection != nil {
			if function, ok := selection.Obj().(*types.Func); ok {
				value.functions = map[*types.Func]struct{}{function: {}}
			}
			if fenceNamedType(selection.Obj().Type(), modulePath+"/analysis/check/fixpoint/equation", "OperandRole") {
				value.role = true
			}
			if selection.Obj().Name() == "Value" &&
				fenceNamedType(selection.Recv(), modulePath+"/compiler/ast", "IdentExpr") {
				value.astName = true
			}
		} else if function, ok := analyzer.typed.info.Uses[item.Sel].(*types.Func); ok {
			value.functions = map[*types.Func]struct{}{function: {}}
		}
	case *ast.BinaryExpr:
		left := analyzer.expr(item.X, env, globals, depth)
		right := analyzer.expr(item.Y, env, globals, depth)
		if item.Op == token.ADD && fenceTextCarrier(analyzer.typed.info.TypeOf(expression)) {
			value = fenceJoinValue(value, fenceConcatValue(left, right))
		}
	case *ast.UnaryExpr:
		analyzer.expr(item.X, env, globals, depth)
	case *ast.IndexExpr:
		analyzer.expr(item.X, env, globals, depth)
		analyzer.expr(item.Index, env, globals, depth)
	case *ast.IndexListExpr:
		analyzer.expr(item.X, env, globals, depth)
		for _, index := range item.Indices {
			analyzer.expr(index, env, globals, depth)
		}
	case *ast.SliceExpr:
		value = fenceJoinValue(value, analyzer.expr(item.X, env, globals, depth))
	case *ast.CompositeLit:
		if text, ok := analyzer.byteComposite(item); ok {
			value = fenceJoinValue(value, fenceKnownText(text))
		}
		for _, element := range item.Elts {
			switch element := element.(type) {
			case *ast.KeyValueExpr:
				analyzer.expr(element.Key, env, globals, depth)
				analyzer.expr(element.Value, env, globals, depth)
			case ast.Expr:
				analyzer.expr(element, env, globals, depth)
			}
		}
	case *ast.CallExpr:
		value = fenceJoinValue(value, analyzer.callExpr(item, env, globals, depth))
	case *ast.KeyValueExpr:
		analyzer.expr(item.Key, env, globals, depth)
		value = fenceJoinValue(value, analyzer.expr(item.Value, env, globals, depth))
	case *ast.StarExpr:
		value = fenceJoinValue(value, analyzer.expr(item.X, env, globals, depth))
	case *ast.TypeAssertExpr:
		value = fenceJoinValue(value, analyzer.expr(item.X, env, globals, depth))
	case *ast.FuncLit:
		// Function literals are inspected when invoked directly. Their body is
		// otherwise inert and cannot construct a protocol value.
	}
	if fenceNamedType(analyzer.typed.info.TypeOf(expression), modulePath+"/analysis/check/fixpoint/equation", "OperandRole") {
		value.role = true
	}
	analyzer.values[expression] = fenceJoinValue(analyzer.values[expression], value)
	if analyzer.onExpr != nil {
		analyzer.onExpr(expression, value)
	}
	return value
}

func (analyzer *fenceSemanticAnalyzer) callExpr(call *ast.CallExpr, env, globals map[types.Object]fenceValue, depth int) fenceValue {
	analyzer.expr(call.Fun, env, globals, depth)
	arguments := make([]fenceValue, len(call.Args))
	for index, argument := range call.Args {
		arguments[index] = analyzer.expr(argument, env, globals, depth)
	}
	if analyzer.typed.info.Types[call.Fun].IsType() {
		if len(arguments) == 1 && fenceTextCarrier(analyzer.typed.info.TypeOf(call)) {
			return arguments[0]
		}
		return fenceValue{}
	}
	if literal, ok := call.Fun.(*ast.FuncLit); ok {
		local := fenceCloneEnv(env)
		if signature, ok := analyzer.typed.info.TypeOf(literal.Type).(*types.Signature); ok {
			for index := 0; index < signature.Params().Len() && index < len(arguments); index++ {
				local[signature.Params().At(index)] = arguments[index]
			}
		}
		return analyzer.block(literal.Body.List, local, globals, depth+1)
	}
	object := fenceCalledFunction(analyzer.typed.info, call)
	if object != nil {
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			selection := analyzer.typed.info.Selections[selector]
			if selection != nil &&
				fenceNamedType(selection.Recv(), "strings", "Builder") {
				receiverObject := types.Object(nil)
				if identifier, ok := selector.X.(*ast.Ident); ok {
					receiverObject = analyzer.typed.info.Uses[identifier]
					if receiverObject == nil {
						receiverObject = analyzer.typed.info.Defs[identifier]
					}
				}
				switch object.Name() {
				case "WriteString":
					if receiverObject != nil && len(arguments) == 1 {
						current := env[receiverObject]
						if len(current.texts) == 0 {
							current = fenceKnownText("")
						}
						env[receiverObject] = fenceConcatValue(current, arguments[0])
					}
					return fenceValue{}
				case "String":
					if receiverObject != nil {
						return env[receiverObject]
					}
				}
			}
		}
		if object.Pkg() != nil && object.Pkg().Path() == "strings" {
			switch object.Name() {
			case "Clone":
				if len(arguments) == 1 {
					return arguments[0]
				}
			case "Repeat":
				if len(arguments) == 2 {
					if count, ok := fenceIntegerConstant(analyzer.typed.info, call.Args[1]); ok && count >= 0 && count <= 64 {
						out := fenceValue{role: arguments[0].role, astName: arguments[0].astName}
						out.texts = make(map[string]struct{})
						for text := range arguments[0].texts {
							out.texts[strings.Repeat(text, int(count))] = struct{}{}
						}
						return out
					}
				}
			case "Join":
				if len(call.Args) == 2 {
					if parts, ok := analyzer.stringComposite(call.Args[0], env, globals, depth); ok {
						for separator := range arguments[1].texts {
							return fenceKnownText(strings.Join(parts, separator))
						}
					}
				}
			}
		}
		if object.Pkg() != nil && object.Pkg().Path() == "fmt" && object.Name() == "Sprintf" && len(arguments) > 0 {
			for format := range arguments[0].texts {
				if strings.Count(format, "%s") == len(arguments)-1 && strings.ReplaceAll(format, "%s", "") == "" {
					out := fenceKnownText("")
					for _, argument := range arguments[1:] {
						out = fenceConcatValue(out, argument)
					}
					return out
				}
			}
		}
		if declaration := analyzer.funcs[object]; declaration != nil {
			signature, _ := object.Type().(*types.Signature)
			if signature == nil || signature.Results().Len() != 1 ||
				!fenceTextCarrier(signature.Results().At(0).Type()) {
				return fenceValue{}
			}
			receiverArguments := arguments
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
				receiverArguments = append(receiverArguments, analyzer.expr(selector.X, env, globals, depth))
			}
			return analyzer.call(object, receiverArguments, globals, depth+1)
		}
		if object.Name() == "Wire" {
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
				receiver := analyzer.expr(selector.X, env, globals, depth)
				if receiver.role || fenceNamedType(analyzer.typed.info.TypeOf(selector.X), modulePath+"/analysis/check/fixpoint/equation", "OperandRole") {
					return fenceValue{role: true}
				}
			}
		}
	}
	if identifier, ok := call.Fun.(*ast.Ident); ok {
		if builtin, ok := analyzer.typed.info.Uses[identifier].(*types.Builtin); ok && builtin.Name() == "append" && len(arguments) != 0 {
			out := arguments[0]
			for _, argument := range arguments[1:] {
				out = fenceConcatValue(out, argument)
			}
			return out
		}
	}
	if fenceNamedType(analyzer.typed.info.TypeOf(call), modulePath+"/analysis/check/fixpoint/equation", "OperandRole") {
		return fenceValue{role: true}
	}
	return fenceValue{}
}

func (analyzer *fenceSemanticAnalyzer) byteComposite(literal *ast.CompositeLit) (string, bool) {
	typed := analyzer.typed.info.TypeOf(literal)
	if !fenceByteSequence(typed) {
		return "", false
	}
	out := make([]byte, 0, len(literal.Elts))
	for _, element := range literal.Elts {
		expression, ok := element.(ast.Expr)
		if !ok {
			return "", false
		}
		value, ok := fenceIntegerConstant(analyzer.typed.info, expression)
		if !ok || value < 0 || value > 255 {
			return "", false
		}
		out = append(out, byte(value))
	}
	return string(out), true
}

func (analyzer *fenceSemanticAnalyzer) stringComposite(expression ast.Expr, env, globals map[types.Object]fenceValue, depth int) ([]string, bool) {
	literal, ok := expression.(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	var out []string
	for _, element := range literal.Elts {
		expression, ok := element.(ast.Expr)
		if !ok {
			return nil, false
		}
		value := analyzer.expr(expression, env, globals, depth)
		if len(value.texts) != 1 {
			return nil, false
		}
		for text := range value.texts {
			out = append(out, text)
		}
	}
	return out, true
}

func fenceIntegerConstant(info *types.Info, expression ast.Expr) (int64, bool) {
	typed := info.Types[expression]
	if typed.Value == nil {
		return 0, false
	}
	return constant.Int64Val(typed.Value)
}

func fenceTextCarrier(typed types.Type) bool {
	if typed == nil {
		return false
	}
	underlying := types.Unalias(typed).Underlying()
	if basic, ok := underlying.(*types.Basic); ok {
		return basic.Info()&types.IsString != 0
	}
	return fenceByteSequence(typed)
}

func fenceByteSequence(typed types.Type) bool {
	if typed == nil {
		return false
	}
	var element types.Type
	switch sequence := types.Unalias(typed).Underlying().(type) {
	case *types.Slice:
		element = sequence.Elem()
	case *types.Array:
		element = sequence.Elem()
	default:
		return false
	}
	basic, ok := types.Unalias(element).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Uint8
}

func fenceNamedType(typed types.Type, packagePath, name string) bool {
	if typed == nil {
		return false
	}
	typed = types.Unalias(typed)
	if pointer, ok := typed.(*types.Pointer); ok {
		typed = types.Unalias(pointer.Elem())
	}
	named, ok := typed.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == packagePath && named.Obj().Name() == name
}

func fenceCalledFunction(info *types.Info, call *ast.CallExpr) *types.Func {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		object, _ := info.Uses[function].(*types.Func)
		return object
	case *ast.SelectorExpr:
		if selection := info.Selections[function]; selection != nil {
			object, _ := selection.Obj().(*types.Func)
			return object
		}
		object, _ := info.Uses[function.Sel].(*types.Func)
		return object
	default:
		return nil
	}
}

func fenceSemanticCalledFunctions(analyzer *fenceSemanticAnalyzer, call *ast.CallExpr) map[*types.Func]struct{} {
	found := make(map[*types.Func]struct{})
	if function := fenceCalledFunction(analyzer.typed.info, call); function != nil {
		found[function] = struct{}{}
	}
	for function := range analyzer.values[call.Fun].functions {
		found[function] = struct{}{}
	}
	return found
}

func fenceFunctionResultIncludesBool(signature *types.Signature) bool {
	if signature == nil {
		return false
	}
	for index := 0; index < signature.Results().Len(); index++ {
		if fenceTypeContainsBool(signature.Results().At(index).Type(), make(map[types.Type]bool)) {
			return true
		}
	}
	return false
}

func fenceTypeContainsBool(typed types.Type, visiting map[types.Type]bool) bool {
	if typed == nil {
		return false
	}
	typed = types.Unalias(typed)
	if named, ok := typed.(*types.Named); ok && named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == modulePath+"/analysis/check/engine" &&
		named.Obj().Name() == "admissionLaneResult" {
		return false
	}
	if visiting[typed] {
		return false
	}
	visiting[typed] = true
	defer delete(visiting, typed)
	switch value := typed.Underlying().(type) {
	case *types.Basic:
		return value.Kind() == types.Bool
	case *types.Struct:
		for index := 0; index < value.NumFields(); index++ {
			if fenceTypeContainsBool(value.Field(index).Type(), visiting) {
				return true
			}
		}
	}
	return false
}

func fenceSemanticTexts(typed *fenceTypedPackage, forbidden []string, ownedFiles map[string]bool, prefixOnly bool) []string {
	var found []string
	seen := make(map[string]bool)
	analyzer := newFenceSemanticAnalyzer(typed)
	analyzer.analyzeAll(func(expression ast.Expr, value fenceValue) {
		if ownedFiles[filepath.Clean(typed.fset.Position(expression.Pos()).Filename)] {
			return
		}
		for text := range value.texts {
			if value.trusted {
				continue
			}
			for _, pattern := range forbidden {
				matches := strings.Contains(text, pattern)
				if prefixOnly {
					matches = strings.HasPrefix(text, pattern)
				}
				if matches {
					position := typed.fset.Position(expression.Pos()).String()
					key := position + ":" + strconv.Quote(pattern)
					if !seen[key] {
						seen[key] = true
						found = append(found, key)
					}
				}
			}
		}
	})
	sort.Strings(found)
	return found
}

func fenceOwnerStringConstant(info *types.Info, expression ast.Expr) bool {
	var object types.Object
	switch item := expression.(type) {
	case *ast.Ident:
		object = info.Uses[item]
	case *ast.SelectorExpr:
		object = info.Uses[item.Sel]
	}
	constantObject, ok := object.(*types.Const)
	return ok && constantObject.Pkg() != nil &&
		constantObject.Pkg().Path() == modulePath+"/analysis/check/fixpoint/shapefact"
}

func fenceRawRoleParsers(typed *fenceTypedPackage) []string {
	var found []string
	seen := make(map[token.Pos]bool)
	analyzer := newFenceSemanticAnalyzer(typed)
	analyzer.analyzeAll(func(expression ast.Expr, _ fenceValue) {
		call, ok := expression.(*ast.CallExpr)
		if !ok || seen[call.Pos()] {
			return
		}
		for function := range fenceSemanticCalledFunctions(analyzer, call) {
			if function.Pkg() == nil || function.Pkg().Path() != "strings" {
				continue
			}
			switch function.Name() {
			case "HasPrefix", "TrimPrefix", "HasSuffix", "TrimSuffix", "Cut", "CutPrefix":
			default:
				continue
			}
			for _, argument := range call.Args {
				value := analyzer.values[argument]
				if value.role {
					seen[call.Pos()] = true
					found = append(found, typed.fset.Position(call.Pos()).String()+":"+function.FullName())
					break
				}
			}
		}
	})
	sort.Strings(found)
	return found
}

func fenceAdmissionBypasses(typed *fenceTypedPackage) []string {
	var found []string
	for _, file := range typed.files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != "selectAdmissionLane" || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				functionType := typed.info.TypeOf(call.Fun)
				if functionType == nil {
					return true
				}
				signature, _ := functionType.Underlying().(*types.Signature)
				if !fenceFunctionResultIncludesBool(signature) {
					return true
				}
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
					if selection := typed.info.Selections[selector]; selection != nil {
						if field, ok := selection.Obj().(*types.Var); ok && field.IsField() && field.Name() == "Admit" {
							return true
						}
						if method, ok := selection.Obj().(*types.Func); ok && method.Name() == "admitted" &&
							fenceNamedType(selection.Recv(), modulePath+"/analysis/check/engine", "admissionLaneResult") {
							return true
						}
					}
				}
				found = append(found, typed.fset.Position(call.Pos()).String())
				return true
			})
		}
	}
	sort.Strings(found)
	return found
}

func fenceWIRTraversalReferences(typed *fenceTypedPackage) []string {
	var found []string
	for _, file := range typed.files {
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			selection := typed.info.Selections[selector]
			if selection == nil || selection.Obj().Pkg() == nil ||
				selection.Obj().Pkg().Path() != modulePath+"/analysis/ir/wir" {
				return true
			}
			if !fenceNamedType(selection.Recv(), modulePath+"/analysis/ir/wir", "Body") {
				return true
			}
			switch selection.Obj().Name() {
			case "Instr", "Len", "PointInstructions":
				found = append(found, typed.fset.Position(selector.Pos()).String()+":"+selection.Obj().Name())
			}
			return true
		})
	}
	sort.Strings(found)
	return found
}

func fenceRevokerConstructions(typed *fenceTypedPackage) []string {
	var found []string
	for _, file := range typed.files {
		ast.Inspect(file, func(node ast.Node) bool {
			var expression ast.Expr
			switch item := node.(type) {
			case *ast.CompositeLit:
				expression = item
			case *ast.CallExpr:
				expression = item
			default:
				return true
			}
			if fenceRevokerCollectionType(typed.info.TypeOf(expression)) {
				found = append(found, typed.fset.Position(expression.Pos()).String())
			}
			return true
		})
	}
	sort.Strings(found)
	return found
}

func fenceRevokerCollectionType(typed types.Type) bool {
	if typed == nil {
		return false
	}
	switch collection := types.Unalias(typed).Underlying().(type) {
	case *types.Slice:
		return fenceNamedType(collection.Elem(), modulePath+"/analysis/check/fixpoint/factkey", "FamilyID")
	case *types.Map:
		return fenceNamedType(collection.Key(), modulePath+"/analysis/check/fixpoint/factkey", "FamilyID")
	default:
		return false
	}
}

func fenceReachableRawASTNameComparisons(typed *fenceTypedPackage, rootName string) []string {
	var found []string
	analyzer := newFenceSemanticAnalyzer(typed)
	analyzer.onExpr = func(expression ast.Expr, _ fenceValue) {
		comparison, ok := expression.(*ast.BinaryExpr)
		if !ok || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
			return
		}
		left := analyzer.values[comparison.X]
		right := analyzer.values[comparison.Y]
		if (left.astName && len(right.texts) != 0) || (right.astName && len(left.texts) != 0) {
			found = append(found, typed.fset.Position(comparison.Pos()).String())
		}
	}
	globals := make(map[types.Object]fenceValue)
	for object, declaration := range analyzer.funcs {
		if declaration.Name.Name == rootName {
			analyzer.call(object, nil, globals, 0)
		}
	}
	sort.Strings(found)
	return found
}

func fenceReachableStringParserLiterals(typed *fenceTypedPackage, rootName, forbidden string) []string {
	var found []string
	analyzer := newFenceSemanticAnalyzer(typed)
	analyzer.onExpr = func(expression ast.Expr, _ fenceValue) {
		call, ok := expression.(*ast.CallExpr)
		if !ok {
			return
		}
		for function := range fenceSemanticCalledFunctions(analyzer, call) {
			if function.Pkg() == nil || function.Pkg().Path() != "strings" {
				continue
			}
			switch function.Name() {
			case "HasSuffix", "TrimSuffix", "CutSuffix":
			default:
				continue
			}
			for _, argument := range call.Args {
				for text := range analyzer.values[argument].texts {
					if text == forbidden {
						found = append(found, typed.fset.Position(call.Pos()).String())
						return
					}
				}
			}
		}
	}
	globals := make(map[types.Object]fenceValue)
	for object, declaration := range analyzer.funcs {
		if declaration.Name.Name == rootName {
			analyzer.call(object, nil, globals, 0)
		}
	}
	sort.Strings(found)
	return found
}

func fenceScratchOwnerTypes(typed *fenceTypedPackage) []string {
	allowed := map[string]bool{
		modulePath + "/analysis/check/fixpoint/equation.EvaluatorScratch":   true,
		modulePath + "/analysis/check/fixpoint/interproc.ProjectionScratch": true,
	}
	var found []string
	sanctionedShapes := make(map[*ast.StructType]bool)
	for _, file := range typed.files {
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range generic.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structure, ok := typeSpec.Type.(*ast.StructType)
				if ok && allowed[typed.pkg.Path()+"."+typeSpec.Name.Name] {
					sanctionedShapes[structure] = true
				}
			}
		}
	}
	scope := typed.pkg.Scope()
	for _, name := range scope.Names() {
		typeName, ok := scope.Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		qualified := typed.pkg.Path() + "." + name
		if qualified == modulePath+"/analysis/check/fixpoint/evalscratch.Depth" {
			continue
		}
		scratchLike := name == "EvaluatorScratch" || name == "ProjectionScratch" ||
			fenceTypeContainsScratch(typeName.Type(), make(map[types.Type]bool))
		if allowed[qualified] {
			if !scratchLike {
				found = append(found, qualified+": sanctioned scratch owner lost its scratch shape")
			}
			continue
		}
		if scratchLike {
			found = append(found, qualified)
		}
	}
	for _, file := range typed.files {
		ast.Inspect(file, func(node ast.Node) bool {
			structure, ok := node.(*ast.StructType)
			if !ok || sanctionedShapes[structure] ||
				!fenceTypeContainsScratch(typed.info.TypeOf(structure), make(map[types.Type]bool)) {
				return true
			}
			found = append(found, typed.fset.Position(structure.Pos()).String()+": local or anonymous scratch owner")
			return true
		})
	}
	sort.Strings(found)
	return found
}

func fenceDisplacedRepresentationConstructions(typed *fenceTypedPackage, owners []fenceOwnedType) []string {
	var found []string
	for _, name := range typed.pkg.Scope().Names() {
		typeName, ok := typed.pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		for _, owner := range owners {
			if typed.pkg.Path() == owner.packagePath {
				continue
			}
			if !fenceExactNamedType(typeName.Type(), owner) &&
				fenceSameRepresentation(typeName.Type(), owner.typed) {
				found = append(found, typed.pkg.Path()+"."+name+" mirrors "+owner.packagePath+"."+owner.name)
			}
		}
	}
	for _, file := range typed.files {
		ast.Inspect(file, func(node ast.Node) bool {
			var expression ast.Expr
			switch item := node.(type) {
			case *ast.CompositeLit:
				expression = item
			case *ast.CallExpr:
				expression = item
			default:
				return true
			}
			expressionType := typed.info.TypeOf(expression)
			for _, owner := range owners {
				if typed.pkg.Path() == owner.packagePath || fenceExactNamedType(expressionType, owner) {
					continue
				}
				if fenceSameRepresentation(expressionType, owner.typed) {
					found = append(found, typed.fset.Position(expression.Pos()).String()+" constructs "+owner.packagePath+"."+owner.name+" representation")
				}
			}
			return true
		})
	}
	sort.Strings(found)
	return found
}

func fenceSameRepresentation(left, right types.Type) bool {
	if left == nil || right == nil {
		return false
	}
	return types.Identical(types.Unalias(left).Underlying(), types.Unalias(right).Underlying())
}

func fenceExactNamedType(typed types.Type, owner fenceOwnedType) bool {
	if typed == nil {
		return false
	}
	typed = types.Unalias(typed)
	if pointer, ok := typed.(*types.Pointer); ok {
		typed = types.Unalias(pointer.Elem())
	}
	named, ok := typed.(*types.Named)
	return ok && named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == owner.packagePath && named.Obj().Name() == owner.name
}

func fenceDecodeTargetClassifications(typed *fenceTypedPackage) []string {
	var found []string
	for _, file := range typed.files {
		ast.Inspect(file, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || len(assignment.Lhs) < 2 || len(assignment.Rhs) != 1 {
				return true
			}
			ignored, ok := assignment.Lhs[0].(*ast.Ident)
			call, callOK := assignment.Rhs[0].(*ast.CallExpr)
			if !ok || ignored.Name != "_" || !callOK {
				return true
			}
			function := fenceCalledFunction(typed.info, call)
			if function != nil && function.Pkg() != nil &&
				function.Pkg().Path() == modulePath+"/analysis/check/fixpoint/shapefact" &&
				function.Name() == "DecodeTarget" {
				found = append(found, typed.fset.Position(call.Pos()).String())
			}
			return true
		})
	}
	sort.Strings(found)
	return found
}

func fenceTypeContainsScratch(typed types.Type, visiting map[types.Type]bool) bool {
	if typed == nil {
		return false
	}
	typed = types.Unalias(typed)
	if pointer, ok := typed.(*types.Pointer); ok {
		typed = types.Unalias(pointer.Elem())
	}
	if visiting[typed] {
		return false
	}
	visiting[typed] = true
	defer delete(visiting, typed)
	if fenceNamedType(typed, modulePath+"/analysis/check/fixpoint/evalscratch", "Depth") ||
		fenceNamedType(typed, modulePath+"/analysis/check/fixpoint/equation", "EvaluatorScratch") ||
		fenceNamedType(typed, modulePath+"/analysis/check/fixpoint/interproc", "ProjectionScratch") {
		return true
	}
	structure, ok := typed.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for index := 0; index < structure.NumFields(); index++ {
		if fenceTypeContainsScratch(structure.Field(index).Type(), visiting) {
			return true
		}
	}
	return false
}
