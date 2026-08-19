package change

import "testing"

func collectSlots(s Slots) []int {
	var members []int
	for index, ok := s.Next(0); ok; index, ok = s.Next(index + 1) {
		members = append(members, index)
	}
	return members
}

func TestSlotsEnumerateEveryMemberInOrder(t *testing.T) {
	cases := []struct {
		name string
		set  []int
		want []int
	}{
		{name: "empty", set: nil, want: nil},
		{name: "one word", set: []int{0, 5, 63}, want: []int{0, 5, 63}},
		{name: "across words", set: []int{64, 3, 191, 128}, want: []int{3, 64, 128, 191}},
		{name: "repeats are idempotent", set: []int{7, 7, 7}, want: []int{7}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var slots Slots
			for _, index := range item.set {
				if !slots.Set(index) {
					t.Fatalf("Set(%d) refused", index)
				}
			}
			got := collectSlots(slots)
			if len(got) != len(item.want) {
				t.Fatalf("members=%v want %v", got, item.want)
			}
			for index := range got {
				if got[index] != item.want[index] {
					t.Fatalf("members=%v want %v", got, item.want)
				}
			}
			if slots.Empty() != (len(item.want) == 0) {
				t.Fatalf("Empty()=%v with members %v", slots.Empty(), got)
			}
			if slots.Count() != len(item.want) {
				t.Fatalf("Count()=%d want %d", slots.Count(), len(item.want))
			}
			for _, index := range item.want {
				if !slots.Test(index) {
					t.Fatalf("Test(%d) false for a member", index)
				}
			}
			if slots.Test(-1) || slots.Test(4096) {
				t.Fatal("Test admitted a slot outside the backing")
			}
		})
	}
}

func TestSlotsUnionIntoGrowsTheDestination(t *testing.T) {
	cases := []struct {
		name   string
		source []int
		dest   []int
		want   []int
	}{
		{name: "into empty", source: []int{2, 130}, dest: nil, want: []int{2, 130}},
		{name: "narrow into wide", source: []int{1}, dest: []int{200}, want: []int{1, 200}},
		{name: "wide into narrow", source: []int{200}, dest: []int{1}, want: []int{1, 200}},
		{name: "overlapping", source: []int{5, 6}, dest: []int{6, 7}, want: []int{5, 6, 7}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var source, dest Slots
			for _, index := range item.source {
				source.Set(index)
			}
			for _, index := range item.dest {
				dest.Set(index)
			}
			if !source.UnionInto(&dest) {
				t.Fatal("UnionInto refused")
			}
			got := collectSlots(dest)
			if len(got) != len(item.want) {
				t.Fatalf("members=%v want %v", got, item.want)
			}
			for index := range got {
				if got[index] != item.want[index] {
					t.Fatalf("members=%v want %v", got, item.want)
				}
			}
			if sourceMembers := collectSlots(source); len(sourceMembers) != len(item.source) {
				t.Fatalf("UnionInto mutated the source: %v", sourceMembers)
			}
		})
	}
	if (Slots{}).UnionInto(nil) {
		t.Fatal("UnionInto admitted a nil destination")
	}
}

func TestSlotsClearKeepsTheBackingAndDropsEveryMember(t *testing.T) {
	var slots Slots
	slots.Set(3)
	slots.Set(200)
	words := len(slots.words)
	slots.Clear()
	if !slots.Empty() || slots.Count() != 0 {
		t.Fatalf("Clear left %v", collectSlots(slots))
	}
	if len(slots.words) != words {
		t.Fatalf("Clear released the backing: %d words want %d", len(slots.words), words)
	}
	if _, ok := slots.Next(0); ok {
		t.Fatal("Next found a member after Clear")
	}
}

func TestNilSlotsRefuseMutation(t *testing.T) {
	var slots *Slots
	if slots.Set(1) {
		t.Fatal("a nil set admitted a member")
	}
	slots.Clear()
	var negative Slots
	if negative.Set(-1) {
		t.Fatal("a negative slot was admitted")
	}
}
