package typepresentation

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPresentationLabelsDoNotAffectSemanticIdentity(t *testing.T) {
	left := NewFunction(nil, []Param{{Label: "key", Type: typ.String}}, nil, []typ.Type{typ.Any})
	right := NewFunction(nil, []Param{{Label: "component_id", Type: typ.String}}, nil, []typ.Type{typ.Any})
	if !left.Equal(right) || left.Hash() != right.Hash() {
		t.Fatal("presentation label entered semantic identity")
	}
	if left.Semantic().String() != "fun(string) -> any" || right.Semantic().String() != left.Semantic().String() {
		t.Fatalf("semantic spellings = %q / %q", left.Semantic(), right.Semantic())
	}
	if label, _ := left.Label(0); label != "key" {
		t.Fatalf("left label = %q", label)
	}
	if label, _ := right.Label(0); label != "component_id" {
		t.Fatalf("right label = %q", label)
	}
}

func TestReceiverConventionIsSemantic(t *testing.T) {
	receiver := NewFunction(nil, []Param{{Label: "self", Type: typ.Any, Receiver: true}}, nil, nil)
	ordinary := NewFunction(nil, []Param{{Label: "self", Type: typ.Any}}, nil, nil)
	if receiver.Equal(ordinary) || receiver.Hash() == ordinary.Hash() {
		t.Fatal("receiver convention collapsed into presentation")
	}
	if receiver.Semantic().Params[0].Name != "self" || ordinary.Semantic().Params[0].Name != "" {
		t.Fatal("semantic receiver marker was not canonical")
	}
}

func TestRecursiveGenericChildrenRemainShared(t *testing.T) {
	param := typ.NewTypeParam("T", typ.String)
	recursive := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("next", self).Field("value", param).Build()
	})
	got := NewFunction([]*typ.TypeParam{param}, []Param{{Label: "node", Type: recursive}}, nil, []typ.Type{param})
	if got.Semantic().Params[0].Type != recursive || got.Semantic().Returns[0] != param {
		t.Fatal("recursive/generic children were traversed or copied")
	}
	if got.Semantic().TypeParams[0] != param {
		t.Fatal("generic binder identity was not preserved")
	}
}

func BenchmarkConstructLargeRecursiveFunction(b *testing.B) {
	recursive := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		record := typetable.NewRecord().Field("next", self)
		for i := 0; i < 128; i++ {
			record.Field("payload_"+string(rune(i+1)), typ.String)
		}
		return record.Build()
	})
	params := make([]Param, 256)
	for i := range params {
		params[i] = Param{Label: "source-label", Type: recursive}
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewFunction(nil, params, nil, []typ.Type{recursive})
	}
}

func BenchmarkSemanticHashAndEquality(b *testing.B) {
	recursive := typ.NewRecursive("Node", func(self typ.Type) typ.Type { return typ.NewArray(self) })
	left := NewFunction(nil, []Param{{Label: "left", Type: recursive}}, nil, []typ.Type{recursive})
	right := NewFunction(nil, []Param{{Label: "right", Type: recursive}}, nil, []typ.Type{recursive})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if left.Hash() != right.Hash() || !left.Equal(right) {
			b.Fatal("semantic identity changed")
		}
	}
}
