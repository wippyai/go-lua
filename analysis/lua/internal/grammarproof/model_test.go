package grammarproof

import "testing"

func TestSnapshotCarriesIndependentEvidenceTraceAndCorpusLanes(t *testing.T) {
	snapshot := Snapshot{
		Evidence: Evidence{Productions: []Production{{Key: "expr#1", Witness: "return 1"}}},
		Traces:   []SemanticTrace{{Production: "expr#1", Source: "return 1"}},
		Corpus:   []CorpusSource{{ID: "grammar:one", Text: "return 1"}},
	}
	if len(snapshot.Evidence.Productions) != 1 || len(snapshot.Traces) != 1 || len(snapshot.Corpus) != 1 {
		t.Fatalf("snapshot lanes = %#v, want one row in each", snapshot)
	}
	if snapshot.Traces[0].Production != snapshot.Evidence.Productions[0].Key || snapshot.Corpus[0].Text == "" {
		t.Fatal("snapshot lanes lost their semantic anchors")
	}
}
