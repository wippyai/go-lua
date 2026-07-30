package caseset

import (
	"reflect"
	"testing"
)

func TestNewOwnsAndCanonicalizesSource(t *testing.T) {
	source := []int{4, 1, 4, 2}
	set := New(source)
	source[0] = 99

	view := set.View()
	if got := viewValues(view); !reflect.DeepEqual(got, []int{1, 2, 4}) {
		t.Fatalf("New source mutation changed set: got %v", got)
	}
}

func TestViewIsAllocationFree(t *testing.T) {
	set := New([]int{1, 2, 3, 4})
	if allocs := testing.AllocsPerRun(1000, func() {
		view := set.View()
		for i := 0; i < view.Len(); i++ {
			_ = view.At(i)
		}
	}); allocs != 0 {
		t.Fatalf("Set.View indexed read allocations = %v, want 0", allocs)
	}
}

func viewValues(view View) []int {
	out := make([]int, view.Len())
	for i := 0; i < view.Len(); i++ {
		out[i] = view.At(i)
	}
	return out
}
