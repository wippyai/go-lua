package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestProvenanceRouteEvidenceLabel(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(1), "item")
	source := constraint.NewPath(cfg.SymbolID(2), "items")

	tests := []struct {
		name string
		kind ProvenanceRouteKind
		want string
	}{
		{
			name: "identity alias",
			kind: ProvenanceRouteIdentityAlias,
			want: "item aliases items",
		},
		{
			name: "indexed iterator",
			kind: ProvenanceRouteIndexedIterator,
			want: "item comes from indexed iterator source items",
		},
		{
			name: "keyed iterator",
			kind: ProvenanceRouteKeyedIterator,
			want: "item comes from keyed iterator source items",
		},
		{
			name: "append element field",
			kind: ProvenanceRouteAppendElementField,
			want: "item comes from appended element source items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (ProvenanceRoute{Kind: tt.kind, Source: source}).EvidenceLabel(target)
			if got != tt.want {
				t.Fatalf("EvidenceLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppendElementFieldRouteQueryForPath(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(1), "node_order").
		Field("entry").
		Field("id")

	got, ok := AppendElementFieldRouteQueryForPath(path)
	if !ok {
		t.Fatal("AppendElementFieldRouteQueryForPath returned false")
	}
	if got.ArrayPath.String() != "node_order.entry" {
		t.Fatalf("ArrayPath = %q, want node_order.entry", got.ArrayPath.String())
	}
	if len(got.Field) != 1 || got.Field[0].Kind != constraint.SegmentField || got.Field[0].Name != "id" {
		t.Fatalf("Field = %#v, want final id field", got.Field)
	}

	if _, ok := AppendElementFieldRouteQueryForPath(constraint.NewPath(cfg.SymbolID(2), "root")); ok {
		t.Fatal("root-only path produced append field query")
	}
}
