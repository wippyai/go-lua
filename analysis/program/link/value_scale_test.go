package link

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// This semantic scale law exercises one large, single-shard value universe.
// It asserts published Value correspondence rather than timing a particular
// Seal implementation.
func TestValueUniverseScalePreservesEveryIntegerOccurrence(t *testing.T) {
	const literalCount = 4096
	var text strings.Builder
	text.Grow(literalCount * 14)
	text.WriteString("return ")
	for index := 0; index < literalCount; index++ {
		if index != 0 {
			text.WriteString(", ")
		}
		text.WriteString(strconv.Itoa(index))
	}
	p := source(t, text.String())
	link := linked(t, contract(t), linkproject.Module{Name: "main", Program: p})
	shard := onlyShard(t, link, p)
	_, shardOK := link.Project().Mounts().Index(shard)
	values := link.Boundary().Values()
	integers := p.Source().Literals().Integers()
	if integers.Count() != literalCount {
		t.Fatalf("fixture integer occurrences = %d, want %d", integers.Count(), literalCount)
	}
	for index := 0; index < integers.Count(); index++ {
		term, _, _, ok := integers.At(index)
		if !ok {
			t.Fatalf("missing integer occurrence %d", index)
		}
		value, ok := values.Of(shard, term)
		if !shardOK || !ok {
			t.Fatalf("missing Link Value for integer occurrence %d", index)
		}
		gotShard, gotTerm, ok := values.Origin(value)
		if !ok || gotShard != shard || gotTerm != term {
			t.Fatalf("integer occurrence %d changed identity: %v/%d/%t", index, gotShard, gotTerm, ok)
		}
	}
	if _, ok := values.Of(shard, keyspace.Term(0)); ok {
		t.Fatal("zero Program Term acquired a Link Value")
	}
}
