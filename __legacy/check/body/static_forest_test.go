package body

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestPreparedStaticForestBuildsEachLexicalBodyExactlyOnce(t *testing.T) {
	stmts := parseChunk(t, nestedFunctionSource(3))
	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.Functions()
	wantBodies := 1 + len(functions)
	if wantBodies != 4 {
		t.Fatalf("fixture lexical bodies = %d, want 4", wantBodies)
	}

	// Reproduce the removed preparation shape explicitly: recursively lower the
	// chunk, then recursively lower every lexical function again. A depth-three
	// chain constructs 4+3+2+1 bodies.
	legacyLowerings := 0
	observe := func(*ast.FunctionExpr) { legacyLowerings++ }
	rootBuilt := cfgbuild.BuildChunk(stmts, bindings)
	_ = wirlower.LowerWithResolverAndOptions("chunk", stmts, bindings, rootBuilt, typeresolve.New(bindings), wirlower.Options{ObserveBodyLowered: observe})
	for _, fn := range functions {
		built := cfgbuild.BuildFunction(fn, bindings)
		_ = wirlower.LowerFunctionWithResolverAndOptions("function", fn, bindings, built, typeresolve.New(bindings), wirlower.Options{ObserveBodyLowered: observe})
	}
	if legacyLowerings != 10 {
		t.Fatalf("recursive prepare lowerings = %d, want triangular 10", legacyLowerings)
	}

	stats := &Stats{}
	forest, err := PrepareBoundChunkForest(stmts, bindings, Config{Registry: standard.Registry(), Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	if stats.LexicalCFGBuilds != wantBodies || stats.LexicalWIRLowerings != wantBodies {
		t.Fatalf("forest CFG/WIR builds = %d/%d, want exactly %d/%d", stats.LexicalCFGBuilds, stats.LexicalWIRLowerings, wantBodies, wantBodies)
	}
	if stats.StaticChunkPrepares != 1 || stats.StaticFunctionPrepares != len(functions) {
		t.Fatalf("forest static prepares = chunk:%d function:%d, want 1/%d", stats.StaticChunkPrepares, stats.StaticFunctionPrepares, len(functions))
	}

	// The source forest is pointer-shared: attaching a child proto is not a copy
	// or a hidden re-lowering. Follow the chain from the chunk and require each
	// proto to be the exact WIR/CFG owned by that function's Static.
	owner := forest.Root()
	for depth, fn := range functions {
		protos := owner.wir.Protos()
		if len(protos) != 1 {
			t.Fatalf("owner depth %d protos = %d, want 1", depth, len(protos))
		}
		child := forest.Function(fn)
		if child == nil || protos[0].Body != child.wir || protos[0].Graph != child.cfg.Graph {
			t.Fatalf("owner depth %d did not reuse child prepared WIR/CFG", depth)
		}
		wantProtoName := "chunk.fn0"
		if depth > 0 {
			wantProtoName = "function.fn0"
		}
		if protos[0].Name != wantProtoName {
			t.Fatalf("owner depth %d proto name = %q, want legacy hierarchical %q", depth, protos[0].Name, wantProtoName)
		}
		owner = child
	}
}

func TestPreparedStaticForestPreservesPerBodySemanticIdentity(t *testing.T) {
	stmts := parseChunk(t, nestedFunctionSource(3))
	bindings := bind.BindChunk(stmts, bind.Options{})
	config := Config{
		Registry:      standard.Registry(),
		UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("nested-forest-production-unit")),
	}
	forest, err := PrepareBoundChunkForest(stmts, bindings, config)
	if err != nil {
		t.Fatal(err)
	}
	legacyRoot, err := PrepareBoundChunk(stmts, bindings, config)
	if err != nil {
		t.Fatal(err)
	}
	assertStaticIdentityEqual(t, legacyRoot, forest.Root(), "chunk")
	for index, fn := range bindings.Functions() {
		legacy, prepareErr := PrepareBoundFunction(fn, bindings, config)
		if prepareErr != nil {
			t.Fatal(prepareErr)
		}
		assertStaticIdentityEqual(t, legacy, forest.Function(fn), fmt.Sprintf("function[%d]", index))
	}
}

func assertStaticIdentityEqual(t *testing.T, legacy, forest *Static, label string) {
	t.Helper()
	if legacy == nil || forest == nil {
		t.Fatalf("%s static missing: legacy=%p forest=%p", label, legacy, forest)
	}
	if got, want := forest.IdentityDigest(), legacy.IdentityDigest(); got != want {
		t.Fatalf("%s identity = %x, want legacy %x", label, got, want)
	}
	if got, want := forest.BoundaryEnvironmentDigest(), legacy.BoundaryEnvironmentDigest(); got != want {
		t.Fatalf("%s boundary environment changed", label)
	}
}

func BenchmarkPreparedStaticForestNestedPathology(b *testing.B) {
	stmts := parseChunk(b, nestedFunctionSource(64))
	bindings := bind.BindChunk(stmts, bind.Options{})
	config := Config{
		Registry:      standard.Registry(),
		UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("nested-forest-production-unit")),
	}
	b.Run("recursive-per-owner", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := PrepareBoundChunk(stmts, bindings, config); err != nil {
				b.Fatal(err)
			}
			for _, fn := range bindings.Functions() {
				if _, err := PrepareBoundFunction(fn, bindings, config); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("source-owned-forest", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := PrepareBoundChunkForest(stmts, bindings, config); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func nestedFunctionSource(depth int) string {
	var source strings.Builder
	for index := 0; index < depth; index++ {
		fmt.Fprintf(&source, "local function f%d()\n", index)
	}
	source.WriteString("return 1\n")
	for index := depth - 1; index >= 0; index-- {
		if index+1 < depth {
			fmt.Fprintf(&source, "return f%d()\n", index+1)
		}
		source.WriteString("end\n")
	}
	if depth > 0 {
		source.WriteString("return f0()\n")
	}
	return source.String()
}
