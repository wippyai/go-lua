package heap

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/program/target"
)

// TestHeapRuntimeKindUnionMapping keeps Target's closed fresh vocabulary at
// the one production mapping. Error and reflection intentionally coarsen to
// Userdata; invalid kinds never enter the Heap runtime-kind set.
func TestHeapRuntimeKindUnionMapping(t *testing.T) {
	cases := []struct {
		name string
		kind target.FreshKind
		want runtimekind.Kind
	}{
		{"table", target.FreshTable, runtimekind.Table},
		{"function", target.FreshFunction, runtimekind.Function},
		{"thread", target.FreshThread, runtimekind.Thread},
		{"userdata", target.FreshUserdata, runtimekind.Userdata},
		{"error", target.FreshError, runtimekind.Userdata},
		{"reflection", target.FreshReflection, runtimekind.Userdata},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, ok := freshRootKinds(test.kind)
			if !ok || got != runtimekind.Bit(test.want) {
				t.Fatalf("FreshKind(%v)=%b/%v, want %b/true", test.kind, got, ok, runtimekind.Bit(test.want))
			}
		})
	}
	if _, ok := freshRootKinds(target.FreshInvalid); ok {
		t.Fatal("invalid FreshKind entered Heap runtime vocabulary")
	}
}

// TestHeapRankAdmissionRejectsRepresentationOverflow distinguishes a sealed
// denominator rejection from runtime Mu policy: arithmetic overflow rejects
// the representation before any Value can be admitted or widened.
func TestHeapRankAdmissionRejectsRepresentationOverflow(t *testing.T) {
	cases := []struct {
		name string
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
