package authored

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

func TestArtifactSectionRoundTripInjectsCountsAndPreservesContentID(t *testing.T) {
	cases := []struct {
		name  string
		input Input
	}{
		{name: "values-tables-functions", input: flowFixtureInput()},
		{name: "access-storage", input: accessStorageFixtureInput()},
		{name: "functions-calls", input: functionCallFixtureInput()},
		{name: "operators", input: operatorFixtureInput()},
		{name: "control", input: controlFixtureInput()},
		{name: "claims", input: claimFixtureInput()},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			view := buildArtifactView(t, test.input)
			first := encodeArtifactSection(t, view)
			second := encodeArtifactSection(t, view)
			if !bytes.Equal(first, second) {
				t.Fatal("equal authored components produced different artifact bytes")
			}

			reader, err := framing.NewReader(first, len(first))
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.Header("program/flow-test", 1); err != nil {
				t.Fatal(err)
			}
			decoded, err := ReadArtifactSection(reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.Finish(); err != nil {
				t.Fatal(err)
			}
			if decoded.Counts != ([keyspace.FamilyCount]uint32{}) {
				t.Fatalf("decoded Counts = %#v; want root-injection zero", decoded.Counts)
			}
			decoded.Counts = test.input.Counts
			replayed := buildArtifactView(t, decoded)
			if replayed.ContentID() != view.ContentID() {
				t.Fatalf("replayed ContentID = %x; want %x", replayed.ContentID(), view.ContentID())
			}
		})
	}
}
