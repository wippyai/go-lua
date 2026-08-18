package heap

import (
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// TestHeapRuntimeKindUnionMapping keeps Target's closed fresh vocabulary at
// the one production mapping. Error and reflection intentionally coarsen to
// Userdata; invalid kinds never enter the Heap runtime-kind set.
func TestHeapRuntimeKindUnionMapping(t *testing.T) {
	cases := []struct {
		name string
		kind schematype.FreshClass
		want runtimekind.Kind
	}{
		{"table", schematype.FreshClassTable, runtimekind.Table},
		{"function", schematype.FreshClassFunction, runtimekind.Function},
		{"thread", schematype.FreshClassThread, runtimekind.Thread},
		{"userdata", schematype.FreshClassUserdata, runtimekind.Userdata},
		{"error", schematype.FreshClassError, runtimekind.Userdata},
		{"reflection", schematype.FreshClassReflection, runtimekind.Userdata},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, ok := freshRootKinds(test.kind)
			if !ok || got != runtimekind.Bit(test.want) {
				t.Fatalf("FreshKind(%v)=%b/%v, want %b/true", test.kind, got, ok, runtimekind.Bit(test.want))
			}
		})
	}
	if _, ok := freshRootKinds(schematype.FreshClassInvalid); ok {
		t.Fatal("invalid FreshKind entered Heap runtime vocabulary")
	}
}

// TestHeapRankAdmissionRejectsRepresentationOverflow distinguishes a sealed
// denominator rejection from runtime Mu policy: arithmetic overflow rejects
// the representation before any Value can be admitted or widened.
func TestHeapRankAdmissionRejectsRepresentationOverflow(t *testing.T) {
	cases := []struct {
		name  string
		owner *schema
	}{
		{"present overflow", &schema{presentPotential: ^uint64(0), referenceCount: 0}},
		{"reference overflow", &schema{presentPotential: 0, referenceCount: ^uint64(0)}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.owner.sealWidenRankBounds() {
				t.Fatal("unrepresentable fixed-coordinate rank was admitted")
			}
		})
	}
}
