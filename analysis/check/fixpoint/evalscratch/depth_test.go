package evalscratch

import "testing"

func TestDepthIsBoundedAndMetered(t *testing.T) {
	depth := NewDepth(2)
	if index, ok := depth.Push(); !ok || index != 0 {
		t.Fatalf("first push = %d/%t", index, ok)
	}
	if index, ok := depth.Push(); !ok || index != 1 {
		t.Fatalf("second push = %d/%t", index, ok)
	}
	if _, ok := depth.Push(); ok || depth.OverflowCount() != 1 {
		t.Fatalf("overflow push = %t, count %d", ok, depth.OverflowCount())
	}
	if !depth.Pop() || !depth.Pop() || depth.Pop() {
		t.Fatal("pop did not preserve bounded stack order")
	}
	depth.Reset()
	if index, ok := depth.Push(); !ok || index != 0 {
		t.Fatalf("push after reset = %d/%t", index, ok)
	}
}
