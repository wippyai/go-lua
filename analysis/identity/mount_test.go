package identity

import "testing"

func TestMountIDAvailabilityChecksEveryByte(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*MountID)
		available bool
	}{
		{name: "zero is unavailable"},
		{name: "first byte", mutate: func(id *MountID) { id[0] = 1 }, available: true},
		{name: "last byte", mutate: func(id *MountID) { id[len(id)-1] = 1 }, available: true},
		{name: "interior byte", mutate: func(id *MountID) { id[17] = 0x80 }, available: true},
		{
			name:      "saturated",
			mutate:    func(id *MountID) { *id = MountID{} },
			available: false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var id MountID
			if testCase.mutate != nil {
				testCase.mutate(&id)
			}
			if got := id.Available(); got != testCase.available {
				t.Fatalf("Available = %t, want %t", got, testCase.available)
			}
		})
	}
	for index := range (MountID{}) {
		var id MountID
		id[index] = 1
		if !id.Available() {
			t.Fatalf("byte %d is not checked", index)
		}
	}
}

func TestMountIDDistinguishesMounts(t *testing.T) {
	first := MountID{1, 2, 3}
	second := first
	second[31] = 9
	if first == second {
		t.Fatalf("distinct mounts compare equal")
	}
	if carried := carryMount(first); carried != first {
		t.Fatalf("carrying a mount changed it")
	}
}

func TestMountIDCarryAndCompareDoNotAllocate(t *testing.T) {
	id := MountID{7}
	other := MountID{8}
	allocations := testing.AllocsPerRun(64, func() {
		if carryMount(id) == other || !id.Available() {
			t.Fatalf("comparison result changed")
		}
	})
	if allocations != 0 {
		t.Fatalf("carry and compare allocated %v times", allocations)
	}
}

//go:noinline
func carryMount(id MountID) MountID { return id }
