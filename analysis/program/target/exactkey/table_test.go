package exactkey

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func TestCompileDeduplicatesAndSealsCanonicalDirectory(t *testing.T) {
	first := keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "first"}
	second := keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "second"}
	table, err := Compile([]keyspace.LiteralValue{second, first, first, second})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := table.Count(); got != 2 {
		t.Fatalf("Count = %d, want 2 after duplicate input", got)
	}
	for index, want := range []keyspace.LiteralValue{first, second} {
		key, ok := table.At(index)
		if !ok {
			t.Fatalf("At(%d) failed", index)
		}
		if got, gotOK := table.Value(key); !gotOK || got != want {
			t.Fatalf("Value(%d) = %#v/%v, want %#v", key, got, gotOK, want)
		}
		if got, gotOK := table.Handle(want); !gotOK || got != vocabulary.ExactKey(index+1) {
			t.Fatalf("Handle(%#v) = %d/%v, want %d", want, got, gotOK, index+1)
		}
	}
	if _, ok := table.At(-1); ok {
		t.Fatal("At(-1) succeeded")
	}
	if _, ok := table.Value(vocabulary.ExactKey(table.Count() + 1)); ok {
		t.Fatal("Value(out of range) succeeded")
	}
}
