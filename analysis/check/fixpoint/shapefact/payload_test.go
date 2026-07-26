package shapefact_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve payload test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}

func TestPayloadCodecRoundTripsEveryDialectForm(t *testing.T) {
	target, ok := shapefact.EncodeTarget(typ.String)
	if !ok {
		t.Fatal("encode target")
	}
	table, ok := shapefact.EncodeTable(shapefact.Table{
		Closed:  true,
		Members: []shapefact.Member{{Suffix: ".answer", Present: true, Value: "scalar/number/42"}},
	})
	if !ok {
		t.Fatal("encode table")
	}
	function := front.CallableValue(typ.Func().Param("value", typ.String).Returns(typ.Boolean).Build())
	values := [][]byte{
		[]byte("scalar/top"),
		[]byte("scalar/nil"),
		[]byte("scalar/boolean"),
		[]byte("scalar/bool/true"),
		[]byte("scalar/bool/false"),
		[]byte("scalar/bool/optional-nil-comparison"),
		[]byte("scalar/number/-1.25"),
		[]byte(`scalar/string/"value"`),
		[]byte("scalar/table"),
		[]byte("scalar/function"),
		[]byte(function),
		[]byte(`scalar/claim/claim-kind/1/claim-type/"number"`),
		[]byte(`scalar/claim/claim-kind/2/claim-type/non-nil`),
		[]byte(`scalar/claim/claim-kind/3/"any"`),
		[]byte(`scalar/claim/claim-kind/4/claim-type/"string"`),
		[]byte("scalar/external-callback-any"),
		[]byte("scalar/channel/op-1"),
		[]byte("scalar/channel-entry/2f2f/cGF0aC9zeW0x"),
		[]byte("scalar/channel-summary/cGF0aC9zeW0x"),
		[]byte("scalar/declaration/ZGVjbGFyYXRpb24"),
		[]byte("scalar/provider/cHJvdmlkZXI"),
		[]byte("scalar/resource/op-1"),
		table,
		target,
	}
	missing, ok := shapefact.EncodeMemberMissing(typ.String)
	if !ok {
		t.Fatal("encode member-missing")
	}
	values = append(values, missing)

	for _, value := range values {
		payload, decoded := shapefact.Decode(value)
		if !decoded {
			t.Errorf("Decode(%q) failed", value)
			continue
		}
		encoded, encodedOK := shapefact.Encode(payload)
		if !encodedOK || string(encoded) != string(value) {
			t.Errorf("Encode(Decode(%q)) = %q/%v", value, encoded, encodedOK)
		}
	}

	payload, ok := shapefact.Decode([]byte(function))
	if !ok {
		t.Fatal("decode precise function")
	}
	if functionType, typed := payload.FunctionType(); !typed || functionType == nil {
		t.Fatal("precise function payload lost its canonical type")
	}
	claim, ok := shapefact.DecodeClaim([]byte(`scalar/claim/claim-kind/3/claim-type/"string"`))
	if !ok || claim.Kind != wir.ClaimAnnotation || string(claim.Target) != `claim-type/"string"` {
		t.Fatalf("decoded claim = %+v/%v", claim, ok)
	}
}

func TestPayloadCodecRoundTripsPublishedCorpusValues(t *testing.T) {
	fixtures := filepath.Join(repositoryRoot(t), "testdata", "fixtures")
	seen := 0
	err := filepath.WalkDir(fixtures, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "manifest.json" {
			return nil
		}
		wire, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var manifest any
		if err := json.Unmarshal(wire, &manifest); err != nil {
			return err
		}
		walkPublishedValues(manifest, func(value string) {
			if !strings.HasPrefix(value, "scalar/") && !strings.HasPrefix(value, "shape/") {
				return
			}
			seen++
			payload, ok := shapefact.Decode([]byte(value))
			if !ok {
				t.Errorf("%s: published payload %q did not decode", path, value)
				return
			}
			encoded, ok := shapefact.Encode(payload)
			if !ok || string(encoded) != value {
				t.Errorf("%s: round trip %q = %q/%v", path, value, encoded, ok)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 10 {
		t.Fatalf("published payload corpus unexpectedly small: %d", seen)
	}
}

func TestScalarPayloadClassificationDoesNotAllocate(t *testing.T) {
	for _, value := range [][]byte{
		[]byte("path/sym1"),
		[]byte("scalar/number/1"),
		[]byte(`scalar/string/"value"`),
		[]byte("scalar/function"),
		[]byte(`scalar/claim/claim-kind/3/"any"`),
	} {
		value := value
		if allocations := testing.AllocsPerRun(100, func() { shapefact.Decode(value) }); allocations != 0 {
			t.Errorf("Decode(%q) allocated %v times", value, allocations)
		}
	}
}

func TestDecodeLiteralTypePreservesPredicateSubset(t *testing.T) {
	for _, value := range [][]byte{
		[]byte(`scalar/string/"value"`),
		[]byte("scalar/bool/true"),
		[]byte("scalar/number/1"),
	} {
		if _, ok := shapefact.DecodeLiteralType(value); !ok {
			t.Fatalf("DecodeLiteralType(%q) rejected a literal predicate", value)
		}
	}
	for _, value := range [][]byte{
		[]byte("scalar/nil"),
		[]byte("scalar/boolean"),
		[]byte("scalar/number/1.5"),
	} {
		if _, ok := shapefact.DecodeLiteralType(value); ok {
			t.Fatalf("DecodeLiteralType(%q) admitted a non-predicate witness", value)
		}
	}
}

func walkPublishedValues(value any, visit func(string)) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "value" {
				if text, ok := child.(string); ok {
					visit(text)
				}
				continue
			}
			walkPublishedValues(child, visit)
		}
	case []any:
		for _, child := range value {
			walkPublishedValues(child, visit)
		}
	}
}

// A payload class is protocol data, so only the codec may test its wire
// prefix. Consumers must switch on Decode's declared forms and scalar kinds.
func TestNoPayloadLiteralPrefixTestsOutsideCodec(t *testing.T) {
	root := filepath.Join(repositoryRoot(t), "analysis")
	codecDir := filepath.Join(root, "check", "fixpoint", "shapefact")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == codecDir {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if regexp.MustCompile(`_\s*,\s*[A-Za-z][A-Za-z0-9_]*\s*:?=\s*shapefact\.DecodeTarget`).Match(source) {
			t.Errorf("%s: shape-target classification must switch on shapefact.Decode", path)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "HasPrefix" && selector.Sel.Name != "CutPrefix" && selector.Sel.Name != "TrimPrefix") {
				return true
			}
			for _, argument := range call.Args {
				literal, ok := argument.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				text, err := strconv.Unquote(literal.Value)
				if err == nil && (strings.HasPrefix(text, "scalar/") || strings.HasPrefix(text, "shape/")) {
					t.Errorf("%s: payload wire-prefix test must use shapefact.Decode", path)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
