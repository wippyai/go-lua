package formal

import "testing"

func TestVocabularyIsClosedAndStable(t *testing.T) {
	want := []string{"in", "mid", "out"}
	for index, vocabulary := range []Vocabulary{Input, Middle, Output} {
		if !vocabulary.Valid() || vocabulary.String() != want[index] {
			t.Fatalf("vocabulary %d = %q valid=%t", vocabulary, vocabulary, vocabulary.Valid())
		}
	}
	for _, vocabulary := range []Vocabulary{Invalid, Output + 1} {
		if vocabulary.Valid() || vocabulary.String() != "invalid" {
			t.Fatalf("invalid vocabulary %d admitted", vocabulary)
		}
	}
}
