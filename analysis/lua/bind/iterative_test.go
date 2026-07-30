package bind

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func iterativeLocal(name string, value ast.Expr) *ast.LocalAssignStmt {
	return &ast.LocalAssignStmt{Names: []string{name}, Exprs: []ast.Expr{value}}
}

func iterativeReturn(expr ast.Expr) *ast.ReturnStmt {
	return &ast.ReturnStmt{Exprs: []ast.Expr{expr}}
}

func TestIterativeBinderDeepRuntimeFamilies(t *testing.T) {
	t.Run("unary-32k", func(t *testing.T) {
		const depth = 32 * 1024
		read := &ast.IdentExpr{Value: "x"}
		var expr ast.Expr = read
		for range depth {
			expr = &ast.UnaryNotOpExpr{Expr: expr}
		}
		decl := iterativeLocal("x", &ast.NumberExpr{Value: "1"})
		result := BindChunk([]ast.Stmt{decl, iterativeReturn(expr)}, Options{})
		want, ok := result.LocalSymbolAt(decl, 0)
		if !ok {
			t.Fatal("local x missing")
		}
		if got, ok := result.SymbolOf(read); !ok || got != want {
			t.Fatalf("deep unary read = %d/%v, want %d", got, ok, want)
		}
	})

	t.Run("do-shadow-8k", func(t *testing.T) {
		const depth = 8 * 1024
		read := &ast.IdentExpr{Value: "x"}
		inner := ast.Stmt(iterativeReturn(read))
		var innermost *ast.LocalAssignStmt
		for i := depth - 1; i >= 0; i-- {
			decl := iterativeLocal("x", &ast.NumberExpr{Value: strconv.Itoa(i)})
			if innermost == nil {
				innermost = decl
			}
			inner = &ast.DoBlockStmt{Stmts: []ast.Stmt{decl, inner}}
		}
		result := BindChunk([]ast.Stmt{inner}, Options{})
		want, ok := result.LocalSymbolAt(innermost, 0)
		if !ok {
			t.Fatal("innermost shadow missing")
		}
		if got, ok := result.SymbolOf(read); !ok || got != want {
			t.Fatalf("deep shadow read = %d/%v, want %d", got, ok, want)
		}
	})

	t.Run("attr-call-table-4k", func(t *testing.T) {
		const depth = 4 * 1024
		root := &ast.IdentExpr{Value: "x"}
		var attr ast.Expr = root
		for range depth {
			attr = &ast.AttrGetExpr{
				Object:    attr,
				Key:       &ast.StringExpr{Value: "field"},
				KeySyntax: ast.AttrKeyDot,
			}
		}
		var call ast.Expr = &ast.IdentExpr{Value: "f"}
		for range depth {
			call = &ast.FuncCallExpr{Func: call}
		}
		var table ast.Expr = &ast.IdentExpr{Value: "x"}
		for range depth {
			table = &ast.TableExpr{Fields: []*ast.Field{{Value: table}}}
		}
		xDecl := iterativeLocal("x", &ast.NumberExpr{Value: "1"})
		fDecl := iterativeLocal("f", &ast.NilExpr{})
		result := BindChunk([]ast.Stmt{xDecl, fDecl, iterativeReturn(&ast.TableExpr{
			Fields: []*ast.Field{{Value: attr}, {Value: call}, {Value: table}},
		})}, Options{})
		x, _ := result.LocalSymbolAt(xDecl, 0)
		f, _ := result.LocalSymbolAt(fDecl, 0)
		if got, ok := result.SymbolOf(root); !ok || got != x {
			t.Fatalf("attribute root = %d/%v, want %d", got, ok, x)
		}
		callRoot := call
		for {
			next, ok := callRoot.(*ast.FuncCallExpr)
			if !ok {
				break
			}
			callRoot = next.Func
		}
		ident := callRoot.(*ast.IdentExpr)
		if got, ok := result.SymbolOf(ident); !ok || got != f {
			t.Fatalf("call root = %d/%v, want %d", got, ok, f)
		}
	})
}

func TestIterativeBinderDeepTypeAndFunction(t *testing.T) {
	t.Run("type-8k", func(t *testing.T) {
		const depth = 8 * 1024
		ref := &ast.TypeRefExpr{Path: []string{"Root"}}
		var declared ast.TypeExpr = ref
		for range depth {
			declared = &ast.OptionalTypeExpr{Inner: declared}
		}
		root := &ast.TypeDefStmt{Name: "Root", Type: &ast.PrimitiveTypeExpr{Name: "number"}}
		local := &ast.LocalAssignStmt{Names: []string{"x"}, Types: []ast.TypeExpr{declared}}
		result := BindChunk([]ast.Stmt{root, local}, Options{})
		want, ok := result.TypeDef(root)
		if !ok {
			t.Fatal("Root type declaration missing")
		}
		if got, ok := result.TypeRef(ref); !ok || got.ID != want.ID {
			t.Fatalf("deep type ref = %d/%v, want %d", got.ID, ok, want.ID)
		}
	})

	t.Run("function-capture-8k", func(t *testing.T) {
		const depth = 8 * 1024
		read := &ast.IdentExpr{Value: "x"}
		var expr ast.Expr = read
		for range depth {
			expr = &ast.FunctionExpr{Stmts: []ast.Stmt{iterativeReturn(expr)}}
		}
		decl := iterativeLocal("x", &ast.NumberExpr{Value: "1"})
		result := BindChunk([]ast.Stmt{decl, iterativeReturn(expr)}, Options{})
		origins := result.FunctionOrigins()
		if len(origins) != depth {
			t.Fatalf("FunctionOrigins = %d, want %d", len(origins), depth)
		}
		for i := 1; i < len(origins); i++ {
			if origins[i].Parent != origins[i-1].Func {
				t.Fatalf("function %d parent = %p, want %p", i, origins[i].Parent, origins[i-1].Func)
			}
		}
		edges := 0
		result.ForEachEntryCapture(func(_ *ast.FunctionExpr, capture Capture) bool {
			if capture.Captured != 0 {
				edges++
			}
			return true
		})
		if edges != depth {
			t.Fatalf("capture boundary edges = %d, want %d", edges, depth)
		}
	})
}

func TestIterativeBinderWideListsAndDeferredVisibility(t *testing.T) {
	const width = 8 * 1024
	names := make([]string, width)
	values := make([]ast.Expr, width)
	reads := make([]ast.Expr, width)
	for i := range width {
		names[i] = "x" + strconv.Itoa(i)
		values[i] = &ast.NumberExpr{Value: strconv.Itoa(i)}
		reads[i] = &ast.IdentExpr{Value: names[i]}
	}
	decl := &ast.LocalAssignStmt{Names: names, Exprs: values}
	result := BindChunk([]ast.Stmt{decl, &ast.ReturnStmt{Exprs: reads}}, Options{})
	for _, index := range []int{0, width / 2, width - 1} {
		want, ok := result.LocalSymbolAt(decl, index)
		if !ok {
			t.Fatalf("local %d missing", index)
		}
		if got, ok := result.SymbolOf(reads[index].(*ast.IdentExpr)); !ok || got != want {
			t.Fatalf("read %d = %d/%v, want %d", index, got, ok, want)
		}
	}

	fRead := &ast.IdentExpr{Value: "f"}
	gRead := &ast.IdentExpr{Value: "g"}
	f := &ast.FunctionExpr{Stmts: []ast.Stmt{iterativeReturn(gRead)}}
	g := &ast.FunctionExpr{Stmts: []ast.Stmt{iterativeReturn(fRead)}}
	group := &ast.LocalAssignStmt{
		Names: []string{"f", "g"},
		Exprs: []ast.Expr{f, g},
	}
	deferred := BindChunk([]ast.Stmt{group}, Options{})
	fID, _ := deferred.LocalSymbolAt(group, 0)
	gID, _ := deferred.LocalSymbolAt(group, 1)
	if got, ok := deferred.SymbolOf(fRead); !ok || got != fID {
		t.Fatalf("deferred f read = %d/%v, want %d", got, ok, fID)
	}
	if got, ok := deferred.SymbolOf(gRead); !ok || got != gID {
		t.Fatalf("deferred g read = %d/%v, want %d", got, ok, gID)
	}
}

func TestIterativeBinderWideTypeList(t *testing.T) {
	const width = 8 * 1024
	root := &ast.TypeDefStmt{Name: "Root", Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	refs := make([]ast.TypeExpr, width)
	for i := range refs {
		refs[i] = &ast.TypeRefExpr{Path: []string{"Root"}}
	}
	typed := &ast.LocalAssignStmt{
		Names: []string{"x"},
		Types: []ast.TypeExpr{&ast.UnionTypeExpr{Types: refs}},
	}
	result := BindChunk([]ast.Stmt{root, typed}, Options{})
	want, ok := result.TypeDef(root)
	if !ok {
		t.Fatal("Root type declaration missing")
	}
	for _, index := range []int{0, width / 2, width - 1} {
		ref := refs[index].(*ast.TypeRefExpr)
		if got, ok := result.TypeRef(ref); !ok || got.ID != want.ID {
			t.Fatalf("type ref %d = %d/%v, want %d", index, got.ID, ok, want.ID)
		}
	}
}

func TestIterativeBinderCaptureGridEmitsOnlyBoundaryEdges(t *testing.T) {
	const (
		width = 64
		depth = 64
	)
	names := make([]string, width)
	values := make([]ast.Expr, width)
	reads := make([]ast.Expr, width)
	for i := range width {
		names[i] = "x" + strconv.Itoa(i)
		values[i] = &ast.NumberExpr{Value: strconv.Itoa(i)}
		reads[i] = &ast.IdentExpr{Value: names[i]}
	}
	var expr ast.Expr = &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.ReturnStmt{Exprs: reads}}}
	for i := 1; i < depth; i++ {
		expr = &ast.FunctionExpr{Stmts: []ast.Stmt{iterativeReturn(expr)}}
	}
	result := BindChunk([]ast.Stmt{
		&ast.LocalAssignStmt{Names: names, Exprs: values},
		iterativeReturn(expr),
	}, Options{})
	edges := 0
	result.ForEachEntryCapture(func(_ *ast.FunctionExpr, capture Capture) bool {
		if capture.Captured != 0 {
			edges++
		}
		return true
	})
	if want := width * depth; edges != want {
		t.Fatalf("capture edges = %d, want %d", edges, want)
	}
}

func TestIterativeBinderTypeOfHasIdentityOnly(t *testing.T) {
	runtimeRead := &ast.IdentExpr{Value: "x"}
	bodyRead := &ast.IdentExpr{Value: "x"}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"p"}},
		Stmts:   []ast.Stmt{iterativeReturn(bodyRead)},
	}
	query := &ast.TypeOfExpr{Expr: &ast.TableExpr{Fields: []*ast.Field{
		{Value: runtimeRead},
		{Value: &ast.FuncCallExpr{Func: fn}},
	}}}
	decl := iterativeLocal("x", &ast.NumberExpr{Value: "1"})
	typed := &ast.LocalAssignStmt{Names: []string{"y"}, Types: []ast.TypeExpr{query}}
	result := BindChunk([]ast.Stmt{decl, typed}, Options{})
	x, _ := result.LocalSymbolAt(decl, 0)
	if got, ok := result.SymbolOf(runtimeRead); !ok || got != x {
		t.Fatalf("TypeOf identity = %d/%v, want %d", got, ok, x)
	}
	if result.HasRead(x) {
		t.Fatal("TypeOf created a runtime read")
	}
	if _, ok := result.FunctionOrigin(fn); ok {
		t.Fatal("TypeOf function signature created a runtime function origin")
	}
	if _, ok := result.SymbolOf(bodyRead); ok {
		t.Fatal("TypeOf traversed a function body")
	}
}

func TestIterativeBinderCastVisitsAnnotationOnce(t *testing.T) {
	ref := &ast.TypeRefExpr{Path: []string{"T"}}
	cast := &ast.CastExpr{
		Expr: &ast.NumberExpr{Value: "1"},
		Type: &ast.FunctionTypeExpr{
			TypeParams: []ast.TypeParamExpr{{Name: "T"}},
			Returns:    []ast.TypeExpr{ref},
		},
	}
	result := BindChunk([]ast.Stmt{iterativeReturn(cast)}, Options{})
	decl, ok := result.TypeRef(ref)
	if !ok {
		t.Fatal("cast annotation type parameter reference missing")
	}
	if decl.ID != 1 {
		t.Fatalf("cast annotation visited more than once: type parameter ID = %d, want 1", decl.ID)
	}
}

func TestIterativeBinderWidePendingFunctionsScaleAdditively(t *testing.T) {
	measure := func(width int) (bytes int64, ns int64) {
		names := make([]string, width)
		exprs := make([]ast.Expr, width)
		for i := range width {
			names[i] = "f" + strconv.Itoa(i)
			exprs[i] = &ast.FunctionExpr{}
		}
		stmts := []ast.Stmt{&ast.LocalAssignStmt{Names: names, Exprs: exprs}}
		result := testing.Benchmark(func(b *testing.B) {
			for range b.N {
				BindChunk(stmts, Options{})
			}
		})
		return result.AllocedBytesPerOp(), result.NsPerOp()
	}
	smallBytes, smallNS := measure(512)
	largeBytes, largeNS := measure(1024)
	if largeBytes > smallBytes*26/10+64*1024 {
		t.Fatalf("pending-function bytes are non-linear: 512=%d 1024=%d", smallBytes, largeBytes)
	}
	if largeNS > smallNS*13/4 {
		t.Fatalf("pending-function time is non-linear: 512=%d 1024=%d", smallNS, largeNS)
	}
}

func TestIterativeBinderDeepLookupScalesLinearly(t *testing.T) {
	build := func(depth int) ([]ast.Stmt, *ast.IdentExpr, *ast.TypeRefExpr, *ast.LocalAssignStmt, *ast.TypeDefStmt) {
		var body []ast.Stmt
		var deepestRead *ast.IdentExpr
		var deepestRef *ast.TypeRefExpr
		for i := depth - 1; i >= 0; i-- {
			read := &ast.IdentExpr{Value: "x"}
			ref := &ast.TypeRefExpr{Path: []string{"Root"}}
			if deepestRead == nil {
				deepestRead, deepestRef = read, ref
			}
			body = []ast.Stmt{
				&ast.FuncCallStmt{Expr: read},
				&ast.LocalAssignStmt{
					Names: []string{"y" + strconv.Itoa(i)},
					Types: []ast.TypeExpr{ref},
				},
				&ast.DoBlockStmt{Stmts: body},
			}
		}
		root := &ast.TypeDefStmt{Name: "Root", Type: &ast.PrimitiveTypeExpr{Name: "number"}}
		x := iterativeLocal("x", &ast.NumberExpr{Value: "1"})
		return []ast.Stmt{root, x, &ast.DoBlockStmt{Stmts: body}}, deepestRead, deepestRef, x, root
	}
	measure := func(depth int) (bytes int64, ns int64) {
		stmts, read, ref, xDecl, rootDecl := build(depth)
		result := BindChunk(stmts, Options{})
		x, _ := result.LocalSymbolAt(xDecl, 0)
		if got, ok := result.SymbolOf(read); !ok || got != x {
			t.Fatalf("depth %d read = %d/%v, want %d", depth, got, ok, x)
		}
		root, _ := result.TypeDef(rootDecl)
		if got, ok := result.TypeRef(ref); !ok || got.ID != root.ID {
			t.Fatalf("depth %d type ref = %d/%v, want %d", depth, got.ID, ok, root.ID)
		}
		bench := testing.Benchmark(func(b *testing.B) {
			for range b.N {
				BindChunk(stmts, Options{})
			}
		})
		return bench.AllocedBytesPerOp(), bench.NsPerOp()
	}
	smallBytes, smallNS := measure(512)
	largeBytes, largeNS := measure(1024)
	if largeBytes > smallBytes*26/10+64*1024 {
		t.Fatalf("deep lookup bytes are non-linear: 512=%d 1024=%d", smallBytes, largeBytes)
	}
	if largeNS > smallNS*13/4 {
		t.Fatalf("deep lookup time is non-linear: 512=%d 1024=%d", smallNS, largeNS)
	}
}

func TestIterativeBinderDeterministicAcrossFreshTrees(t *testing.T) {
	build := func() ([]ast.Stmt, *ast.IdentExpr, *ast.LocalAssignStmt) {
		read := &ast.IdentExpr{Value: "x"}
		decl := iterativeLocal("x", &ast.NumberExpr{Value: "1"})
		fn := &ast.FunctionExpr{Stmts: []ast.Stmt{iterativeReturn(read)}}
		return []ast.Stmt{decl, iterativeReturn(fn)}, read, decl
	}
	leftAST, leftRead, leftDecl := build()
	rightAST, rightRead, rightDecl := build()
	left := BindChunk(leftAST, Options{})
	right := BindChunk(rightAST, Options{})
	leftLocal, _ := left.LocalSymbolAt(leftDecl, 0)
	rightLocal, _ := right.LocalSymbolAt(rightDecl, 0)
	leftReadID, _ := left.SymbolOf(leftRead)
	rightReadID, _ := right.SymbolOf(rightRead)
	if leftLocal != rightLocal || leftReadID != rightReadID {
		t.Fatalf("fresh bind identities differ: local %d/%d read %d/%d",
			leftLocal, rightLocal, leftReadID, rightReadID)
	}
	leftOrigins, rightOrigins := left.FunctionOrigins(), right.FunctionOrigins()
	if len(leftOrigins) != len(rightOrigins) || len(leftOrigins) != 1 {
		t.Fatalf("fresh origins = %d/%d", len(leftOrigins), len(rightOrigins))
	}
	if kind, ok := left.Kind(leftOrigins[0].Symbol); !ok || kind != symbol.Function {
		t.Fatalf("function kind = %v/%v", kind, ok)
	}
	if leftOrigins[0].Symbol != rightOrigins[0].Symbol ||
		leftOrigins[0].Kind != rightOrigins[0].Kind {
		t.Fatalf("fresh origins differ: %#v / %#v", leftOrigins[0], rightOrigins[0])
	}
}
