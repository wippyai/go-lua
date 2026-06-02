package bind

import "testing"

func TestPredeclaredGlobalNames(t *testing.T) {
	t.Parallel()

	got := PredeclaredGlobalNames(map[string]int{
		"print": 1,
		"":      2,
		"error": 3,
		"pairs": 4,
	})
	want := []string{"error", "pairs", "print"}
	if len(got) != len(want) {
		t.Fatalf("PredeclaredGlobalNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PredeclaredGlobalNames() = %v, want %v", got, want)
		}
	}
}

func TestPredeclaredGlobalNamesEmpty(t *testing.T) {
	t.Parallel()

	if got := PredeclaredGlobalNames[int](nil); got != nil {
		t.Fatalf("PredeclaredGlobalNames(nil) = %v, want nil", got)
	}
	if got := PredeclaredGlobalNames(map[string]int{"": 1}); len(got) != 0 {
		t.Fatalf("PredeclaredGlobalNames(empty-name only) = %v, want empty", got)
	}
}
