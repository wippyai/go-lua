package bind

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestParsedQualifiedTypeRootUsesExactLexicalOccurrence(t *testing.T) {
	stmts, err := parse.ParseString(`
local module = {}
type Outer = module.User
do
	local module = {}
	type Inner = module.User
end
type Bare = Outer
`, "qualified_type_roots.lua")
	if err != nil {
		t.Fatal(err)
	}
	outerLocal := stmts[0].(*ast.LocalAssignStmt)
	outerRef := stmts[1].(*ast.TypeDefStmt).Type.(*ast.TypeRefExpr)
	block := stmts[2].(*ast.DoBlockStmt)
	innerLocal := block.Stmts[0].(*ast.LocalAssignStmt)
	innerRef := block.Stmts[1].(*ast.TypeDefStmt).Type.(*ast.TypeRefExpr)
	bare := stmts[3].(*ast.TypeDefStmt).Type

	result := BindChunk(stmts, typeindex.Table{})
	outer := mustLocalAt(t, result, outerLocal, 0)
	inner := mustLocalAt(t, result, innerLocal, 0)
	if got, ok := result.QualifiedTypeRootSymbol(outerRef); !ok || got != outer {
		t.Fatalf("outer root = %d/%v, want %d", got, ok, outer)
	}
	if got, ok := result.QualifiedTypeRootSymbol(innerRef); !ok || got != inner {
		t.Fatalf("inner root = %d/%v, want %d", got, ok, inner)
	}
	if bareRef, ok := bare.(*ast.TypeRefExpr); ok {
		if got, found := result.QualifiedTypeRootSymbol(bareRef); found || got != 0 {
			t.Fatalf("bare root = %d/%v, want absent", got, found)
		}
	}
}

func TestParsedStaticOnlyQualifiedGlobalMintsIdentityWithoutRuntimeEvidence(t *testing.T) {
	stmts, err := parse.ParseString(`
type Remote = stream.Stream
`, "static_only_qualified_global.lua")
	if err != nil {
		t.Fatal(err)
	}
	ref := stmts[0].(*ast.TypeDefStmt).Type.(*ast.TypeRefExpr)
	result := BindChunk(stmts, typeindex.Table{})

	root, ok := result.QualifiedTypeRootSymbol(ref)
	if !ok || root == 0 {
		t.Fatal("static-only qualified global has no root identity")
	}
	if kind, known := result.Kind(root); !known || kind != SymbolGlobal {
		t.Fatalf("root kind = %v/%v, want Global", kind, known)
	}
	if len(result.implicitGlobalUses) != 0 {
		t.Fatalf("static-only root recorded runtime implicit uses: %#v", result.implicitGlobalUses)
	}
}

func TestParsedQualifiedGlobalSharesLaterRuntimeIdentityAndPreservesUseEvidence(t *testing.T) {
	stmts, err := parse.ParseString(`
type Remote = stream.Stream
local runtime = stream
`, "qualified_global_then_runtime.lua")
	if err != nil {
		t.Fatal(err)
	}
	ref := stmts[0].(*ast.TypeDefStmt).Type.(*ast.TypeRefExpr)
	runtime := stmts[1].(*ast.LocalAssignStmt).Exprs[0].(*ast.IdentExpr)
	result := BindChunk(stmts, typeindex.Table{})

	root, rootOK := result.QualifiedTypeRootSymbol(ref)
	use, useOK := result.SymbolOf(runtime)
	if !rootOK || !useOK || root == 0 || use != root {
		t.Fatalf("root/use = %d/%v and %d/%v, want one global identity", root, rootOK, use, useOK)
	}
	if !result.IsImplicitGlobalUse(runtime) || len(result.implicitGlobalUses) != 1 {
		t.Fatalf("runtime occurrence did not retain exact implicit-use evidence")
	}
}

func TestParsedQualifiedGlobalRuntimeWriteEstablishesLaterRead(t *testing.T) {
	stmts, err := parse.ParseString(`
type Remote = stream.Stream
stream = {}
local runtime = stream
`, "qualified_global_runtime_write.lua")
	if err != nil {
		t.Fatal(err)
	}
	ref := stmts[0].(*ast.TypeDefStmt).Type.(*ast.TypeRefExpr)
	write := stmts[1].(*ast.AssignStmt).Lhs[0].(*ast.IdentExpr)
	read := stmts[2].(*ast.LocalAssignStmt).Exprs[0].(*ast.IdentExpr)
	result := BindChunk(stmts, typeindex.Table{})

	root, rootOK := result.QualifiedTypeRootSymbol(ref)
	writeID, writeOK := result.SymbolOf(write)
	readID, readOK := result.SymbolOf(read)
	if !rootOK || !writeOK || !readOK || root == 0 || writeID != root || readID != root {
		t.Fatalf("root/write/read = %d/%v %d/%v %d/%v, want one global identity",
			root, rootOK, writeID, writeOK, readID, readOK)
	}
	if result.IsImplicitGlobalUse(read) || len(result.implicitGlobalUses) != 0 {
		t.Fatalf("read after runtime write was labeled implicit")
	}
}

func TestParsedQualifiedGlobalYieldsToExactLocalShadow(t *testing.T) {
	stmts, err := parse.ParseString(`
type Outer = stream.Stream
do
	local stream = {}
	type Inner = stream.Stream
end
`, "qualified_global_local_shadow.lua")
	if err != nil {
		t.Fatal(err)
	}
	outerRef := stmts[0].(*ast.TypeDefStmt).Type.(*ast.TypeRefExpr)
	block := stmts[1].(*ast.DoBlockStmt)
	local := block.Stmts[0].(*ast.LocalAssignStmt)
	innerRef := block.Stmts[1].(*ast.TypeDefStmt).Type.(*ast.TypeRefExpr)
	result := BindChunk(stmts, typeindex.Table{})

	outer, outerOK := result.QualifiedTypeRootSymbol(outerRef)
	inner, innerOK := result.QualifiedTypeRootSymbol(innerRef)
	localID := mustLocalAt(t, result, local, 0)
	if !outerOK || !innerOK || outer == 0 || inner != localID || outer == inner {
		t.Fatalf("outer/inner/local = %d/%v %d/%v %d", outer, outerOK, inner, innerOK, localID)
	}
	if len(result.implicitGlobalUses) != 0 {
		t.Fatalf("static roots recorded runtime implicit uses: %#v", result.implicitGlobalUses)
	}
}

func TestStaticPublicationCarriesBinderIssuedSourceRoot(t *testing.T) {
	stmts, err := parse.ParseString(`
local Source = {}
local Target = {}
type User = number
Source.User = User
Target.User = Source.User
`, "static_publication_root_evidence.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts, typeindex.Table{})
	bare := stmts[3].(*ast.AssignStmt)
	qualified := stmts[4].(*ast.AssignStmt)
	bareEntries := result.StaticTypePublications(bare)
	qualifiedEntries := result.StaticTypePublications(qualified)
	if len(bareEntries) != 1 || !bareEntries[0].Valid() || bareEntries[0].Root != 0 {
		t.Fatalf("bare publication = %#v, want valid zero-root evidence", bareEntries)
	}
	if len(qualifiedEntries) != 1 || !qualifiedEntries[0].Valid() {
		t.Fatalf("qualified publication = %#v, want valid evidence", qualifiedEntries)
	}
	rhs := qualified.Rhs[0].(*ast.AttrGetExpr).Object.(*ast.IdentExpr)
	root, ok := result.SymbolOf(rhs)
	if !ok || root == 0 || qualifiedEntries[0].Root != root {
		t.Fatalf("qualified source root = %d/%v, publication root = %d; want binder identity", root, ok, qualifiedEntries[0].Root)
	}
}
