package discover

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"sort"
	"strings"
)

func analyze(cp checkedPackage) Report {
	a := analyzer{cp: cp, methods: map[*types.Func]*ast.FuncDecl{}, funcs: map[*types.Func]*ast.FuncDecl{}, ownerFields: map[*types.Named]map[*types.Var]struct{}{}}
	a.index()
	a.receiverClusters()
	a.forwarders()
	a.apiFamilies()
	a.fileClusters()
	a.testClusters()
	a.duplicateBodies()
	a.switchShapes()
	a.importClusters()
	a.duplicateIndexes()
	sortCandidates(a.out)
	return Report{Package: cp.path, Candidates: a.out}
}

type analyzer struct {
	cp          checkedPackage
	methods     map[*types.Func]*ast.FuncDecl
	funcs       map[*types.Func]*ast.FuncDecl
	ownerFields map[*types.Named]map[*types.Var]struct{}
	out         []Candidate
}

func (a *analyzer) index() {
	for _, file := range a.cp.files {
		ast.Inspect(file, func(node ast.Node) bool {
			decl, ok := node.(*ast.FuncDecl)
			if !ok {
				return true
			}
			fn, _ := a.cp.info.Defs[decl.Name].(*types.Func)
			if fn == nil {
				return true
			}
			a.funcs[fn] = decl
			if decl.Recv != nil {
				a.methods[fn] = decl
			}
			return false
		})
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				name, _ := a.cp.info.Defs[typeSpec.Name].(*types.TypeName)
				if name == nil {
					continue
				}
				named, _ := name.Type().(*types.Named)
				st, _ := named.Underlying().(*types.Struct)
				if st == nil {
					continue
				}
				set := map[*types.Var]struct{}{}
				for i := 0; i < st.NumFields(); i++ {
					set[st.Field(i)] = struct{}{}
				}
				a.ownerFields[named] = set
			}
		}
	}
}

func (a *analyzer) receiverClusters() {
	// Build a bipartite graph of receiver-owned direct fields and the methods
	// that read/write them. Connected components are evidence of co-use only.
	type node struct {
		field  *types.Var
		method *types.Func
	}
	adj := map[node]map[node]struct{}{}
	add := func(x, y node) {
		if adj[x] == nil {
			adj[x] = map[node]struct{}{}
		}
		if adj[y] == nil {
			adj[y] = map[node]struct{}{}
		}
		adj[x][y] = struct{}{}
		adj[y][x] = struct{}{}
	}
	for fn, decl := range a.methods {
		owner := receiverNamed(fn)
		fields := a.ownerFields[owner]
		if len(fields) == 0 {
			continue
		}
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			selection := a.cp.info.Selections[sel]
			if selection == nil {
				return true
			}
			field, ok := selection.Obj().(*types.Var)
			if !ok || !field.IsField() {
				return true
			}
			if _, owned := fields[field]; owned {
				add(node{method: fn}, node{field: field})
			}
			return true
		})
	}
	seen := map[node]bool{}
	for start := range adj {
		if seen[start] {
			continue
		}
		var todo []node
		todo = append(todo, start)
		seen[start] = true
		var fields []*types.Var
		var methods []*types.Func
		edges := 0
		for len(todo) > 0 {
			x := todo[0]
			todo = todo[1:]
			if x.field != nil {
				fields = append(fields, x.field)
			}
			if x.method != nil {
				methods = append(methods, x.method)
			}
			for y := range adj[x] {
				edges++
				if !seen[y] {
					seen[y] = true
					todo = append(todo, y)
				}
			}
		}
		if len(fields) == 0 || len(methods) == 0 {
			continue
		}
		symbols := make([]Symbol, 0, len(fields)+len(methods))
		for _, f := range fields {
			symbols = append(symbols, a.symbol(f))
		}
		for _, m := range methods {
			symbols = append(symbols, a.symbol(m))
		}
		sortSymbols(symbols)
		owner := receiverNamed(methods[0])
		a.add(ReceiverCluster, typeID(owner), symbols, "high", []string{"type-checked receiver field/method incidence component"}, []Evidence{{Code: "field-method-edges", Detail: typeID(owner), Count: edges / 2}})
	}
}

func (a *analyzer) forwarders() {
	for fn, decl := range a.methods {
		if decl.Body == nil || len(decl.Body.List) != 1 {
			continue
		}
		call, ok := onlyForwardCall(decl.Body.List[0])
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		selection := a.cp.info.Selections[sel]
		if selection == nil || selection.Kind() != types.MethodVal {
			continue
		}
		if !passesExactParams(a.cp.info, decl, call) {
			continue
		}
		target, ok := selection.Obj().(*types.Func)
		if !ok {
			continue
		}
		a.add(Forwarder, objectID(fn), []Symbol{a.symbol(fn), a.symbol(target)}, "high",
			[]string{"single-statement receiver method forwarding exact parameters"},
			[]Evidence{{Code: "forward-target", Detail: objectID(target), Count: 1}})
	}
}

func onlyForwardCall(stmt ast.Stmt) (*ast.CallExpr, bool) {
	switch stmt := stmt.(type) {
	case *ast.ExprStmt:
		call, ok := stmt.X.(*ast.CallExpr)
		return call, ok
	case *ast.ReturnStmt:
		if len(stmt.Results) != 1 {
			return nil, false
		}
		call, ok := stmt.Results[0].(*ast.CallExpr)
		return call, ok
	default:
		return nil, false
	}
}

func passesExactParams(info *types.Info, decl *ast.FuncDecl, call *ast.CallExpr) bool {
	var params []types.Object
	if decl.Type.Params != nil {
		for _, f := range decl.Type.Params.List {
			for _, n := range f.Names {
				params = append(params, info.Defs[n])
			}
		}
	}
	if len(params) != len(call.Args) {
		return false
	}
	for i, arg := range call.Args {
		ident, ok := arg.(*ast.Ident)
		if !ok || info.Uses[ident] != params[i] {
			return false
		}
	}
	return true
}

func (a *analyzer) apiFamilies() {
	groups := map[string][]*types.Func{}
	for fn := range a.funcs {
		family := apiName(fn.Name())
		if family == "" {
			continue
		}
		owner := a.cp.path
		if recv := receiverNamed(fn); recv != nil {
			owner = typeID(recv)
		}
		groups[owner+"/"+family] = append(groups[owner+"/"+family], fn)
	}
	for key, funcs := range groups {
		if len(funcs) < 2 {
			continue
		}
		symbols := make([]Symbol, 0, len(funcs))
		for _, fn := range funcs {
			symbols = append(symbols, a.symbol(fn))
		}
		sortSymbols(symbols)
		a.add(APIFamily, key, symbols, "high", []string{"declared API family name"}, []Evidence{{Code: "family-members", Detail: strings.TrimPrefix(key, a.cp.path+"/"), Count: len(symbols)}})
	}
}

func apiName(name string) string {
	for _, suffix := range []string{"Count", "At", "Find", "Rebind"} {
		if name == suffix || strings.HasSuffix(name, suffix) {
			return suffix
		}
	}
	return ""
}

func (a *analyzer) fileClusters() {
	for _, file := range a.cp.files {
		var symbols []Symbol
		for _, decl := range file.Decls {
			for _, name := range declaredNames(decl) {
				if object := a.cp.info.Defs[name]; object != nil {
					symbols = append(symbols, a.symbol(object))
				}
			}
		}
		if len(symbols) < 2 {
			continue
		}
		sortSymbols(symbols)
		path := position(a.cp.fset, file.Pos()).Path
		a.add(FileCluster, path, symbols, "low", []string{"same source file; co-location is not ownership"}, []Evidence{{Code: "file-declarations", Detail: path, Count: len(symbols)}})
	}
}

func declaredNames(decl ast.Decl) []*ast.Ident {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		return []*ast.Ident{d.Name}
	case *ast.GenDecl:
		var out []*ast.Ident
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				out = append(out, s.Name)
			case *ast.ValueSpec:
				out = append(out, s.Names...)
			}
		}
		return out
	}
	return nil
}

func (a *analyzer) testClusters() {
	for fn, decl := range a.funcs {
		if !strings.HasPrefix(fn.Name(), "Test") || decl.Body == nil {
			continue
		}
		seen := map[types.Object]bool{}
		var symbols []Symbol
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			obj := a.cp.info.Uses[id]
			if obj == nil || seen[obj] || obj.Pkg() != a.cp.pkg || obj == fn {
				return true
			}
			seen[obj] = true
			symbols = append(symbols, a.symbol(obj))
			return true
		})
		if len(symbols) == 0 {
			continue
		}
		symbols = append(symbols, a.symbol(fn))
		sortSymbols(symbols)
		a.add(TestCluster, objectID(fn), symbols, "high", []string{"type-checked test references package declarations"}, []Evidence{{Code: "test-references", Detail: objectID(fn), Count: len(symbols) - 1}})
	}
}

func (a *analyzer) duplicateBodies() {
	groups := map[string][]*types.Func{}
	for fn, decl := range a.funcs {
		if decl.Body == nil {
			continue
		}
		groups[alphaBlock(a.cp, decl.Body)] = append(groups[alphaBlock(a.cp, decl.Body)], fn)
	}
	for fingerprint, funcs := range groups {
		if len(funcs) < 2 {
			continue
		}
		symbols := make([]Symbol, 0, len(funcs))
		for _, fn := range funcs {
			symbols = append(symbols, a.symbol(fn))
		}
		sortSymbols(symbols)
		a.add(DuplicateBody, shortHash(fingerprint), symbols, "high", []string{"formatted function bodies match after local identifier alpha-normalization"}, []Evidence{{Code: "alpha-body", Detail: shortHash(fingerprint), Count: len(symbols)}})
	}
}

func (a *analyzer) switchShapes() {
	type occurrence struct {
		owner *types.Func
		pos   Position
	}
	groups := map[string][]occurrence{}
	for fn, decl := range a.funcs {
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			for _, stmt := range sw.Body.List {
				clause := stmt.(*ast.CaseClause)
				block := &ast.BlockStmt{List: clause.Body}
				fp := alphaBlock(a.cp, block)
				groups[fp] = append(groups[fp], occurrence{fn, position(a.cp.fset, clause.Pos())})
			}
			return true
		})
	}
	for fp, cases := range groups {
		if len(cases) < 2 {
			continue
		}
		var symbols []Symbol
		var positions []Position
		seen := map[*types.Func]bool{}
		for _, c := range cases {
			positions = append(positions, c.pos)
			if !seen[c.owner] {
				seen[c.owner] = true
				symbols = append(symbols, a.symbol(c.owner))
			}
		}
		sortSymbols(symbols)
		sortPositions(positions)
		a.addAt(SwitchCaseShape, shortHash(fp), symbols, positions, "high", []string{"switch case bodies match after local identifier alpha-normalization"}, []Evidence{{Code: "alpha-case", Detail: shortHash(fp), Count: len(cases)}})
	}
}

func (a *analyzer) importClusters() {
	groups := map[string][]Position{}
	for _, file := range a.cp.files {
		for _, spec := range file.Imports {
			path := strings.Trim(spec.Path.Value, "\"")
			pos := position(a.cp.fset, spec.Pos())
			groups[path] = append(groups[path], pos)
		}
	}
	for path, positions := range groups {
		a.addAt(ImportCluster, path, []Symbol{{ID: "import:" + path, Name: path, Kind: "import", Position: positions[0]}}, positions, "high", []string{"declared import; consumer calls are intentionally not inferred textually"}, []Evidence{{Code: "declared-import", Detail: path, Count: len(positions)}})
	}
}

func (a *analyzer) duplicateIndexes() {
	type occurrence struct {
		target, source types.Object
		pos            Position
	}
	constructed := map[types.Object]bool{}
	for _, file := range a.cp.files {
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, rhs := range assign.Rhs {
				if i >= len(assign.Lhs) || !a.isMakeMap(rhs) {
					continue
				}
				if target := expressionObject(a.cp, assign.Lhs[i]); target != nil {
					constructed[target] = true
				}
			}
			return true
		})
	}
	groups := map[string][]occurrence{}
	for _, file := range a.cp.files {
		ast.Inspect(file, func(n ast.Node) bool {
			r, ok := n.(*ast.RangeStmt)
			if !ok {
				return true
			}
			source := expressionObject(a.cp, r.X)
			if source == nil {
				return true
			}
			ast.Inspect(r.Body, func(child ast.Node) bool {
				assign, ok := child.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for _, lhs := range assign.Lhs {
					index, ok := lhs.(*ast.IndexExpr)
					if !ok {
						continue
					}
					target := expressionObject(a.cp, index.X)
					if target != nil && constructed[target] {
						groups[objectID(source)] = append(groups[objectID(source)], occurrence{target, source, position(a.cp.fset, index.Pos())})
					}
				}
				return true
			})
			return true
		})
	}
	for source, occurrences := range groups {
		if len(occurrences) < 2 {
			continue
		}
		uniq := map[types.Object]bool{}
		var symbols []Symbol
		var positions []Position
		for _, o := range occurrences {
			positions = append(positions, o.pos)
			if !uniq[o.target] {
				uniq[o.target] = true
				symbols = append(symbols, a.symbol(o.target))
			}
		}
		if len(symbols) < 2 {
			continue
		}
		symbols = append(symbols, a.symbol(occurrences[0].source))
		sortSymbols(symbols)
		sortPositions(positions)
		a.addAt(DuplicateIndex, source, symbols, positions, "high", []string{"map/index assignments are populated by range over the same type-checked source relation"}, []Evidence{{Code: "shared-range-source", Detail: source, Count: len(occurrences)}})
	}
}

func (a *analyzer) isMakeMap(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || len(call.Args) == 0 {
		return false
	}
	builtin, ok := a.cp.info.Uses[ident].(*types.Builtin)
	if !ok || builtin.Name() != "make" {
		return false
	}
	_, ok = call.Args[0].(*ast.MapType)
	return ok
}

func receiverNamed(fn *types.Func) *types.Named {
	sig, _ := fn.Type().(*types.Signature)
	if sig == nil || sig.Recv() == nil {
		return nil
	}
	return namedType(sig.Recv().Type())
}
func namedType(t types.Type) *types.Named {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, _ := t.(*types.Named)
	return n
}
func expressionObject(cp checkedPackage, expr ast.Expr) types.Object {
	switch e := expr.(type) {
	case *ast.Ident:
		if o := cp.info.Uses[e]; o != nil {
			return o
		}
		return cp.info.Defs[e]
	case *ast.SelectorExpr:
		if s := cp.info.Selections[e]; s != nil {
			return s.Obj()
		}
	}
	return nil
}
func (a *analyzer) symbol(obj types.Object) Symbol {
	return Symbol{ID: objectID(obj), Name: obj.Name(), Kind: objectKind(obj), Position: position(a.cp.fset, obj.Pos())}
}
func (a *analyzer) add(kind Kind, key string, symbols []Symbol, confidence string, reasons []string, evidence []Evidence) {
	positions := positionsForSymbols(symbols)
	a.addAt(kind, key, symbols, positions, confidence, reasons, evidence)
}
func (a *analyzer) addAt(kind Kind, key string, symbols []Symbol, positions []Position, confidence string, reasons []string, evidence []Evidence) {
	sortSymbols(symbols)
	sortPositions(positions)
	sort.Strings(reasons)
	sort.Slice(evidence, func(i, j int) bool {
		return evidence[i].Code < evidence[j].Code || evidence[i].Code == evidence[j].Code && evidence[i].Detail < evidence[j].Detail
	})
	a.out = append(a.out, Candidate{Kind: kind, Key: key, Package: a.cp.path, Symbols: symbols, Positions: positions, Confidence: confidence, Reasons: reasons, Evidence: evidence})
}

func position(fset *token.FileSet, pos token.Pos) Position {
	p := fset.PositionFor(pos, false)
	return Position{Path: strings.ReplaceAll(p.Filename, "\\", "/"), Line: p.Line, Column: p.Column}
}
func positionsForSymbols(symbols []Symbol) []Position {
	out := make([]Position, 0, len(symbols))
	for _, s := range symbols {
		out = append(out, s.Position)
	}
	return out
}
func typeID(n *types.Named) string {
	if n == nil {
		return ""
	}
	return objectID(n.Obj())
}
func objectID(obj types.Object) string {
	if obj == nil {
		return ""
	}
	pkg := ""
	if obj.Pkg() != nil {
		pkg = obj.Pkg().Path() + "."
	}
	return pkg + obj.Name() + "@" + fmt.Sprint(obj.Pos())
}
func objectKind(obj types.Object) string {
	switch obj.(type) {
	case *types.Func:
		return "func"
	case *types.Var:
		return "field-or-var"
	case *types.TypeName:
		return "type"
	default:
		return fmt.Sprintf("%T", obj)
	}
}
func sortSymbols(s []Symbol) { sort.Slice(s, func(i, j int) bool { return s[i].ID < s[j].ID }) }
func sortPositions(p []Position) {
	sort.Slice(p, func(i, j int) bool {
		if p[i].Path != p[j].Path {
			return p[i].Path < p[j].Path
		}
		if p[i].Line != p[j].Line {
			return p[i].Line < p[j].Line
		}
		return p[i].Column < p[j].Column
	})
}
func sortCandidates(c []Candidate) {
	sort.Slice(c, func(i, j int) bool {
		if c[i].Kind != c[j].Kind {
			return c[i].Kind < c[j].Kind
		}
		if c[i].Key != c[j].Key {
			return c[i].Key < c[j].Key
		}
		return strings.Join(symbolIDs(c[i].Symbols), ",") < strings.Join(symbolIDs(c[j].Symbols), ",")
	})
}
func symbolIDs(s []Symbol) []string {
	out := make([]string, len(s))
	for i := range s {
		out[i] = s[i].ID
	}
	return out
}
func shortHash(s string) string { sum := sha256.Sum256([]byte(s)); return fmt.Sprintf("%x", sum[:8]) }

func alphaBlock(cp checkedPackage, block *ast.BlockStmt) string {
	// Formatting a temporarily alpha-renamed in-memory AST is read-only to the
	// filesystem. Restore every spelling before return even on format failure.
	var changed []struct {
		id  *ast.Ident
		old string
	}
	names := map[types.Object]string{}
	next := 0
	ast.Inspect(block, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := cp.info.Defs[id]
		if obj == nil {
			obj = cp.info.Uses[id]
		}
		if !isLocal(obj, cp.pkg) {
			return true
		}
		name, ok := names[obj]
		if !ok {
			name = fmt.Sprintf("v%d", next)
			next++
			names[obj] = name
		}
		changed = append(changed, struct {
			id  *ast.Ident
			old string
		}{id, id.Name})
		id.Name = name
		return true
	})
	defer func() {
		for i := len(changed) - 1; i >= 0; i-- {
			changed[i].id.Name = changed[i].old
		}
	}()
	var out strings.Builder
	if err := format.Node(&out, cp.fset, block); err != nil {
		return "invalid:" + fmt.Sprint(block.Pos())
	}
	return out.String()
}
func isLocal(obj types.Object, pkg *types.Package) bool {
	if obj == nil {
		return false
	}
	if v, ok := obj.(*types.Var); ok && v.IsField() {
		return false
	}
	if _, ok := obj.(*types.Func); ok {
		return false
	}
	if _, ok := obj.(*types.TypeName); ok {
		return false
	}
	return obj.Parent() != nil && pkg != nil && obj.Parent() != pkg.Scope()
}
