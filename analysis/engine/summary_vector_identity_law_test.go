package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// A summary surface carries the digest of the key vector it names. The digest
// is minted once, when the surface is declared, and every later reader
// authenticates the pair it is handed instead of deriving a second identity of
// its own: a surface whose stored digest disagrees with its keys names a
// vector the engine never declared and does not bind.
func TestSummarySurfaceIsAuthenticAgainstItsOwnKeyVector(t *testing.T) {
	keys := []uint64{1, 3, 7}
	digest := summaryVectorDigestSource(newSummaryKeyVector(keys))
	if digest == ([32]byte{}) {
		t.Fatal("summary vector digest")
	}
	surface := equation.Surface{Factor: compositionKeyOf(coldKey(0x51)), Form: equation.SurfaceReadSummary,
		Content: digest, Semantic: compositionKeyOf(coldKey(0x52)), Normalizer: compositionKeyOf(coldKey(0x52))}
	if !authenticSummaryVector(surface, keys) {
		t.Fatal("a surface refused the key vector its own digest was minted from")
	}
	for _, law := range []struct {
		name string
		keys []uint64
	}{
		{"reordered", []uint64{3, 1, 7}},
		{"shortened", []uint64{1, 3}},
		{"extended", []uint64{1, 3, 7, 9}},
		{"altered", []uint64{1, 4, 7}},
		{"empty", nil},
	} {
		if authenticSummaryVector(surface, law.keys) {
			t.Fatalf("a %s key vector authenticated against another vector's digest", law.name)
		}
	}
	blank := surface
	blank.Content = [32]byte{}
	foreign := surface
	foreign.Content[0] ^= 0xff
	if authenticSummaryVector(blank, keys) || authenticSummaryVector(foreign, keys) {
		t.Fatal("a surface carrying no digest or a foreign digest authenticated against a key vector")
	}
}

// The summary key vector reaches its identity through exactly one derivation.
// Two functions over two containers at two phases mint the same intended
// preimage and nothing compares them, so a divergence would silently give one
// summary read two identities. The law is structural: one deriver, and its
// only callers are the declaration that mints the digest into the surface and
// the bind that authenticates the surface it is handed.
func TestSummaryVectorDigestIsDerivedAtOnePlace(t *testing.T) {
	set := token.NewFileSet()
	packages, err := parser.ParseDir(set, ".", func(entry fs.FileInfo) bool {
		return !strings.HasSuffix(entry.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse engine package: %v", err)
	}
	parsed, present := packages["engine"]
	if !present {
		t.Fatal("engine package source")
	}
	derivers := map[string]bool{}
	callers := map[string]int{}
	for _, file := range parsed.Files {
		for _, declaration := range file.Decls {
			function, isFunc := declaration.(*ast.FuncDecl)
			if !isFunc || function.Recv != nil || !strings.HasPrefix(function.Name.Name, "summaryVectorDigest") {
				continue
			}
			derivers[function.Name.Name] = true
		}
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, isFunc := node.(*ast.FuncDecl)
			if !isFunc {
				return true
			}
			ast.Inspect(declaration.Body, func(inner ast.Node) bool {
				call, isCall := inner.(*ast.CallExpr)
				if !isCall {
					return true
				}
				name, isName := call.Fun.(*ast.Ident)
				if isName && strings.HasPrefix(name.Name, "summaryVectorDigest") {
					callers[declaration.Name.Name]++
				}
				return true
			})
			return false
		})
	}
	if len(derivers) != 1 || !derivers["summaryVectorDigestSource"] {
		t.Fatalf("the summary key vector has %d derivations, want one: %v", len(derivers), derivers)
	}
	if len(callers) != 2 || callers["summaryReadSurface"] != 1 || callers["authenticSummaryVector"] != 1 {
		t.Fatalf("the summary vector digest is derived outside its mint and its authentication: %v", callers)
	}
}
