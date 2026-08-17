package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactKeysExposeCanonicalDenseExactHandles(t *testing.T) {
	input, index := keyFaultFixture()
	component := finalizeSource(t, input, index)
	keys := component.View().Keys()
	if got, want := keys.ExactCount(), 3; got != want {
		t.Fatalf("ExactCount = %d, want %d", got, want)
	}
	for ordinal, want := range []keyspace.LiteralValue{
		{Kind: keyspace.LiteralInteger, Integer: 1},
		{Kind: keyspace.LiteralString, String: "a"},
		{Kind: keyspace.LiteralString, String: "z"},
	} {
		key, got, ok := keys.ExactAt(ordinal)
		if !ok || key != keyspace.Key(ordinal+1) || got != want {
			t.Fatalf("ExactAt(%d) = %v/%#v/%v, want %d/%#v/true", ordinal, key, got, ok, ordinal+1, want)
		}
	}
	if _, ok := keys.Exact(0); ok {
		t.Fatal("Exact accepted the zero dynamic-key handle")
	}
}
