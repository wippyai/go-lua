package index

import "testing"

func TestRawSelectionIndexIsLinearExactAndAllocationFreeWhenWarm(t *testing.T) {
	for _, test := range []struct {
		name  string
		count int
	}{{"small", 64}, {"large", 4096}} {
		t.Run(test.name, func(t *testing.T) {
			count := test.count
			tags := make([]uint64, count)
			for index := range tags {
				// Spread source and route-like bits across both halves of the tag.
				tags[index] = uint64(index+1)<<32 | uint64(index+1)
			}
			var selected rawSelectionIndex
			calls := 0
			at := func(index int) (uint64, bool) {
				calls++
				return tags[index], true
			}
			if !selected.build(count, at) || calls != count {
				t.Fatalf("cold build calls=%d, want %d", calls, count)
			}
			if len(selected.entries) < count*2 || len(selected.entries) >= count*4 {
				t.Fatalf("entry capacity=%d, want [2N,4N)", len(selected.entries))
			}
			for ordinal, tag := range tags {
				got, ok := selected.ordinal(tag)
				if !ok || got != ordinal {
					t.Fatalf("ordinal tag=%d got=%d,%t want=%d,true", tag, got, ok, ordinal)
				}
			}
			if _, ok := selected.ordinal(0); ok {
				t.Fatal("zero tag was admitted")
			}
			if _, ok := selected.ordinal(1); ok {
				t.Fatal("missing tag was admitted")
			}

			calls = 0
			if allocations := testing.AllocsPerRun(100, func() {
				calls = 0
				if !selected.build(count, at) || calls != count {
					panic("warm raw selection build")
				}
				for ordinal, tag := range tags {
					if got, ok := selected.ordinal(tag); !ok || got != ordinal {
						panic("warm raw selection probe")
					}
				}
			}); allocations != 0 {
				t.Fatalf("warm build and probe allocated %v times", allocations)
			}
		})
	}
}
