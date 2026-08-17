package parserproducts

import "testing"

func TestGeneratedParserProductSequencesNameDestinationAndSegments(t *testing.T) {
	rows := generatedSequences()
	if len(rows) == 0 || len(rows) != len(Generated.Sequences) {
		t.Fatalf("generated sequences = %d/%d", len(rows), len(Generated.Sequences))
	}
	for _, row := range rows {
		if row.Production == "" || row.Scope == 0 || row.Destination.Tag == "" {
			t.Fatalf("incomplete generated sequence %#v", row)
		}
		for _, segment := range row.Segments {
			if segment.Kind == SequenceSegmentInvalid || segment.Term == 0 {
				t.Fatalf("incomplete generated sequence segment %#v", segment)
			}
		}
	}
}
