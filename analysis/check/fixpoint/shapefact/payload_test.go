package shapefact_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

func TestPayloadCodecDecodesEveryDialectForm(t *testing.T) {
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
		if _, decoded := shapefact.Decode(value); !decoded {
			t.Errorf("Decode(%q) failed", value)
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

func TestPayloadCodecDecodesPublishedCorpusValues(t *testing.T) {
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
			if _, ok := shapefact.Decode([]byte(value)); !ok {
				t.Errorf("%s: published payload %q did not decode", path, value)
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
