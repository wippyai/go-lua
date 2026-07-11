package manifest

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Regression: a resolved module type alias can retain a recursive generic
// declaration graph. The manifest boundary must seal that graph as a finite
// wire projection rather than handing a live cycle to encoding/json.
func TestManifestEncodeRecursiveGenericProjectionTerminatesAndRoundTrips(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	node := typ.NewGeneric("Node", []*typ.TypeParam{param}, nil)
	node.SetBody(typetable.NewRecord().Field("next", typ.Instantiate(node, param)).Build())

	m := New("fixture/recursive-generic")
	m.DefineType("Node", node)

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	got, ok := decoded.Types["Node"].(*typ.Generic)
	if !ok {
		t.Fatalf("decoded Node = %T, want *typ.Generic", decoded.Types["Node"])
	}
	body, ok := got.Body.(*typ.Record)
	if !ok {
		t.Fatalf("decoded Node body = %T, want *typ.Record", got.Body)
	}
	next, ok := body.GetField("next").Type.(*typ.Instantiated)
	if !ok || next.Generic != got || len(next.TypeArgs) != 1 || next.TypeArgs[0] != got.TypeParams[0] {
		t.Fatalf("decoded Node.next = %#v, want the decoded Node generic", next)
	}
}
