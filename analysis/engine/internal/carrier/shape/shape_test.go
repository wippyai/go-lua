package shape

import "testing"

func TestSealFreezesPhysicalWidth(t *testing.T) {
	owner, ok := Seal(3)
	if !ok || owner.Count() != 3 {
		t.Fatal("seal")
	}
	for index := 0; index < 3; index++ {
		if !owner.ValidSlot(Slot(index)) {
			t.Fatalf("slot %d is not valid", index)
		}
	}
	if owner.ValidSlot(-1) || owner.ValidSlot(3) {
		t.Fatal("out-of-width slot accepted")
	}
}

func TestSealAdmitsZeroFactorWidth(t *testing.T) {
	owner, ok := Seal(0)
	if !ok || owner == nil || owner.Count() != 0 || owner.ValidSlot(0) || owner.ValidSlot(-1) {
		t.Fatal("zero-width shape")
	}
}
