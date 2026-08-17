package parseruses

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/astcodec"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/occurrence"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/parserproducts"
	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

func TestBuildConsumesOnlySealedProducts(t *testing.T) {
	products := testProducts()
	evidence, err := Build(products)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Validate(products); err != nil {
		t.Fatal(err)
	}
	if evidence.ProductsDigest != products.Digest || len(evidence.UsePaths) != 2 {
		t.Fatalf("incomplete parser-use evidence: %#v", evidence)
	}
	if evidence.UsePaths[0].Term == 0 || evidence.UsePaths[1].Term == 0 {
		t.Fatal("direct routes lost their sealed action-term identities")
	}
	first, second := evidence.Canonical(), evidence.Canonical()
	if len(first) == 0 || !bytes.Equal(first, second) {
		t.Fatal("canonical parser-use evidence is not deterministic")
	}
}

func TestEvidenceRejectsProductsDriftAndReorderedRoutes(t *testing.T) {
	products := testProducts()
	evidence, err := Build(products)
	if err != nil {
		t.Fatal(err)
	}
	stale := products
	stale.Digest = "other-products"
	if err := evidence.Validate(stale); err == nil {
		t.Fatal("stale parser-products evidence was accepted")
	}
	reordered := evidence
	reordered.UsePaths = append([]UsePath(nil), evidence.UsePaths...)
	reordered.UsePaths[0], reordered.UsePaths[1] = reordered.UsePaths[1], reordered.UsePaths[0]
	reordered.Digest = digest(reordered)
	if err := reordered.Validate(products); err == nil {
		t.Fatal("reordered parser-use routes were accepted")
	}
	missing := evidence
	missing.UsePaths = missing.UsePaths[1:]
	missing.Digest = digest(missing)
	if err := missing.Validate(products); err == nil {
		t.Fatal("missing direct route was accepted")
	}
}

func TestTypedTermsRejectMissingCarrierRoute(t *testing.T) {
	products := testProducts()
	products.ProductLaws[0].Products[0].Fields[0].Kind = parserproducts.ActionValueZero
	products.ProductLaws[0].Products[0].Fields[0].Term = 0
	products = sealProducts(products)
	if _, err := Build(products); err == nil {
		t.Fatal("missing typed direct route was accepted")
	}
}

func TestRendererChecksArtifactAndCurrentDetaches(t *testing.T) {
	products := testProducts()
	evidence, err := Build(products)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := render(evidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(rendered), "\n") {
		if len(line) > 320 {
			t.Fatalf("generated source line exceeds 320 bytes: %d", len(line))
		}
	}
	path := t.TempDir() + "/evidence_gen.go"
	if err := os.WriteFile(path, rendered, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Generate(products, path, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Generate(products, path, true); err == nil {
		t.Fatal("stale generated evidence was accepted")
	}
	prior := Generated
	defer func() { Generated = prior }()
	Generated = evidence
	current, err := Current(products)
	if err != nil {
		t.Fatal(err)
	}
	current.UsePaths[0].Term++
	if Generated.UsePaths[0].Term == current.UsePaths[0].Term {
		t.Fatal("Current exposed generated evidence backing storage")
	}
}

func TestGeneratedProductsPreserveTypedHelperAndEditEvidence(t *testing.T) {
	evidence, err := Build(parserproducts.Generated)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Validate(parserproducts.Generated); err != nil {
		t.Fatal(err)
	}
	if len(evidence.HelperUsePaths) == 0 || len(evidence.MutationUsePaths) == 0 {
		t.Fatal("generated parser products lost helper or edit routes")
	}
	for _, path := range evidence.HelperUsePaths {
		if path.Instance.Root == 0 || len(path.Applications) == 0 {
			t.Fatal("helper route lost typed instance geometry")
		}
	}
	for _, path := range evidence.MutationUsePaths {
		if path.Edit.Value == 0 || path.Edit.Place.StepCount == 0 {
			t.Fatal("mutation route lost sealed edit geometry")
		}
	}
}

func TestSealedProductsIntegration(t *testing.T) {
	root := parserUsesRepositoryRoot(t)
	products, err := parserproducts.Current(root)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := Build(products)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Validate(products); err != nil || len(evidence.Canonical()) == 0 {
		t.Fatalf("sealed parser-products integration failed: %v", err)
	}
}

func TestGeneratedEvidenceMatchesSealedProducts(t *testing.T) {
	products, err := parserproducts.Current(parserUsesRepositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := Build(products)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(Generated, expected) {
		t.Fatalf("generated mismatch: products=%t digest=%t slots=%t(%d/%d) direct=%t(%d/%d) helpers=%t(%d/%d) mutations=%t(%d/%d) tails=%t(%d/%d) lvalues=%t(%d/%d)", Generated.ProductsDigest == expected.ProductsDigest, Generated.Digest == expected.Digest, reflect.DeepEqual(Generated.UseSlots, expected.UseSlots), len(Generated.UseSlots), len(expected.UseSlots), reflect.DeepEqual(Generated.UsePaths, expected.UsePaths), len(Generated.UsePaths), len(expected.UsePaths), reflect.DeepEqual(Generated.HelperUsePaths, expected.HelperUsePaths), len(Generated.HelperUsePaths), len(expected.HelperUsePaths), reflect.DeepEqual(Generated.MutationUsePaths, expected.MutationUsePaths), len(Generated.MutationUsePaths), len(expected.MutationUsePaths), reflect.DeepEqual(Generated.ValuesTails, expected.ValuesTails), len(Generated.ValuesTails), len(expected.ValuesTails), reflect.DeepEqual(Generated.LValuePaths, expected.LValuePaths), len(Generated.LValuePaths), len(expected.LValuePaths))
	}
}

// parserUsesRepositoryRoot walks up from this test source until it finds the
// directory that owns go.mod. Anchoring on the module marker keeps the proof
// independent of where the grammarproof tree sits inside the module.
func parserUsesRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("discover parser-uses test path")
	}
	root := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("module root: no go.mod above test file")
		}
		root = parent
	}
}

func testProducts() parserproducts.Evidence {
	products := parserproducts.Evidence{
		GrammarDigest:      "grammar",
		ParserSourceDigest: "parser",
		SchemaDigest:       "schema",
		IngressDigest:      "ingress",
		Fields: []parserproducts.FieldState{
			{Form: "Form", Field: "Left", State: astcodec.FieldStatePresent, Context: occurrence.ContextExpression, Disposition: parserproducts.DispositionObserved, Source: "source", ParserLaw: 1},
			{Form: "Form", Field: "Right", State: astcodec.FieldStatePresent, Context: occurrence.ContextExpression, Disposition: parserproducts.DispositionObserved, Source: "source", ParserLaw: 1},
		},
		ProductLaws: []parserproducts.ProductLaw{{
			Production: "form", Nonterminal: "form", RHS: []string{"expr", "expr"}, ActionDigest: "action", Scope: 1, Form: parserproducts.ActionFormDirectConstruct,
			Products: []parserproducts.ConstructorProduct{{Ordinal: 1, Constructor: "Form", Fields: []parserproducts.ProductField{{Field: "Left", Kind: parserproducts.ActionValueTerm, Term: 1}, {Field: "Right", Kind: parserproducts.ActionValueTerm, Term: 2}}}},
		}},
		Sequences: []parserproducts.SequenceLaw{{Production: "form", Scope: 1, Destination: parserproducts.SequenceDestination{Tag: "form"}, Construction: parserproducts.SequenceConstructionForward, Segments: []parserproducts.SequenceSegment{}}},
		Carriers: []parserproducts.Carrier{
			{Form: "Form", Field: "Left", Class: parsersource.ConstructorExpression, ChildType: "Expr", Cardinality: astcodec.FieldStatePresent},
			{Form: "Form", Field: "Right", Class: parsersource.ConstructorExpression, ChildType: "Expr", Cardinality: astcodec.FieldStatePresent},
		},
		ActionTerms: parserproducts.ActionTerms{
			Symbols: []parserproducts.ActionSymbol{
				{Kind: parserproducts.ActionSymbolField, Text: "Left"},
				{Kind: parserproducts.ActionSymbolField, Text: "Right"},
				{Kind: parserproducts.ActionSymbolOwner, Text: "form"},
			},
			Scopes: []parserproducts.ActionScope{{Kind: parserproducts.ActionScopeProduction, Owner: 3, Inputs: 2, Results: 1}},
			Terms: []parserproducts.ActionTerm{
				{Scope: 1, Kind: parserproducts.ActionTermInput, Slot: 0, EdgeStart: 0, EdgeCount: 0},
				{Scope: 1, Kind: parserproducts.ActionTermInput, Slot: 1, EdgeStart: 0, EdgeCount: 0},
			},
			Edges:        []parserproducts.ActionEdge{},
			ChainTails:   []parserproducts.ChainTail{},
			PlaceSteps:   []parserproducts.PlaceStep{},
			GuardSymbols: []parserproducts.ActionSymbolID{},
		},
	}
	return sealProducts(products)
}

func sealProducts(products parserproducts.Evidence) parserproducts.Evidence {
	sum := sha256.Sum256(products.Canonical())
	products.Digest = hex.EncodeToString(sum[:])
	return products
}
