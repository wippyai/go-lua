package grammarproof

import "testing"

func TestGrammarCorpusUsesUniqueRequiredWitnessIDs(t *testing.T) {
	seen := make(map[string]bool, len(grammarCorpus))
	for _, witness := range grammarCorpus {
		if witness.id == "" || witness.text == "" && witness.id != "grammar:empty" {
			t.Fatalf("invalid grammar witness %#v", witness)
		}
		if seen[witness.id] {
			t.Fatalf("duplicate grammar witness %q", witness.id)
		}
		seen[witness.id] = true
		if !witness.required {
			t.Fatalf("grammar witness %q is not required", witness.id)
		}
	}
}
