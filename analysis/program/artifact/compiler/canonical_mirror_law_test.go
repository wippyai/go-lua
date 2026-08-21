package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/allocation"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/localtransfer"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/heapindex"
)

// TestCanonicalPublicationKeepsIssuedHeapAndStaticRows proves the compiler
// hands publication the rows its producers already sealed, rather than
// retaining a parallel local vocabulary to replay at freeze time.
func TestCanonicalPublicationKeepsIssuedHeapAndStaticRows(t *testing.T) {
	input, err := lower.Lower(lower.Source{Name: "canonical-mirror.lua", Text: []byte("return {}\n")})
	if err != nil {
		t.Fatal(err)
	}
	state := &compiler{input: input}
	if failure := state.copyValuesFailure(); failure.Available() {
		t.Fatalf("value fixture failed: %v", failure)
	}
	allocations, fault := allocation.Build(allocation.Input{Program: input, Values: state.values})
	if fault.Failed() || allocations == nil {
		t.Fatalf("allocation fixture fault=%#v bundle=%v", fault, allocations)
	}
	index, indexOK := heapindex.NewIndex(
		identity.ContentID{1}, false,
		identity.ContentID{2}, identity.ContentID{}, identity.ContentID{},
		heapindex.LensExact, 7,
		identity.ContentID{3}, identity.ContentID{4}, 2,
	)
	if !indexOK {
		t.Fatal("heap index constructor rejected valid canonical row")
	}
	typeValue, typeValueOK := programschema.NewStaticTypeValue(
		identity.ContentID{5}, identity.ContentID{6}, identity.ContentID{7}, identity.ContentID{8}, "T",
	)
	if !typeValueOK {
		t.Fatal("static type value constructor rejected valid canonical row")
	}

	transfers := localtransfer.New(artifactFormat())
	if fault := transfers.Seal(); fault.Failed() {
		t.Fatalf("seal empty local transfer owner: %#v", fault)
	}
	publication, publicationOK := canonicalPublication(&compiler{
		allocations:      allocations,
		localTransfer:    transfers,
		heapIndexes:      []heapindex.Index{index},
		staticTypeValues: []programschema.StaticTypeValue{typeValue},
	}, nil)
	if !publicationOK {
		t.Fatal("canonical publication rejected canonical rows")
	}
	if len(publication.HeapIndexes) != 1 || publication.HeapIndexes[0] != index {
		t.Fatalf("heap indexes were not passed through canonically: %#v", publication.HeapIndexes)
	}
	if len(publication.StaticTypeValues) != 1 || publication.StaticTypeValues[0] != typeValue {
		t.Fatalf("static type values were not passed through canonically: %#v", publication.StaticTypeValues)
	}
}
