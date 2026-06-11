package effect

import "testing"

func TestUnionEmpty(t *testing.T) {
	result := Union(Empty, Empty)
	if !result.Pure() {
		t.Error("union of empty rows should be empty")
	}
}

func TestUnionWithEmpty(t *testing.T) {
	r := Empty.With(testLabel{name: "a"})
	result := Union(r, Empty)

	if len(result.Labels) != 1 {
		t.Errorf("expected 1 label, got %d", len(result.Labels))
	}
}

func TestUnionCombinesLabels(t *testing.T) {
	r1 := Empty.With(testLabel{name: "a"})
	r2 := Empty.With(testLabel{name: "b"})
	result := Union(r1, r2)

	if len(result.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(result.Labels))
	}
}

func TestUnionDeduplicates(t *testing.T) {
	label := testLabel{name: "a"}
	r1 := Empty.With(label)
	r2 := Empty.With(label)
	result := Union(r1, r2)

	if len(result.Labels) != 1 {
		t.Errorf("expected 1 label (deduplicated), got %d", len(result.Labels))
	}
}

func TestUnionWithUnknown(t *testing.T) {
	r := Empty.With(testLabel{name: "a"})
	result := Union(r, Unknown)

	if !result.IsUnknown() {
		t.Error("union with Unknown should be Unknown")
	}
}

func TestUnionPreservesTail(t *testing.T) {
	r1 := Open("e1", testLabel{name: "a"})
	r2 := Empty.With(testLabel{name: "b"})
	result := Union(r1, r2)

	if !result.IsOpen() {
		t.Error("union should preserve open tail")
	}
}

func TestIntersectEmpty(t *testing.T) {
	r := Empty.With(testLabel{name: "a"})
	result := Intersect(r, Empty)

	if !result.Pure() {
		t.Error("intersect with empty should be empty")
	}
}

func TestIntersectCommonLabels(t *testing.T) {
	a := testLabel{name: "a"}
	r1 := Row{Labels: []Label{a, testLabel{name: "b"}}}
	r2 := Row{Labels: []Label{a, testLabel{name: "c"}}}
	result := Intersect(r1, r2)

	if len(result.Labels) != 1 {
		t.Errorf("expected 1 common label, got %d", len(result.Labels))
	}

	if !result.Labels[0].Equals(a) {
		t.Error("common label should be preserved")
	}
}

func TestIntersectNoCommon(t *testing.T) {
	r1 := Empty.With(testLabel{name: "a"})
	r2 := Empty.With(testLabel{name: "b"})
	result := Intersect(r1, r2)

	if len(result.Labels) != 0 {
		t.Errorf("expected no common labels, got %d", len(result.Labels))
	}
}

func TestSubsetEmpty(t *testing.T) {
	if !Subset(Empty, Empty.With(testLabel{name: "a"})) {
		t.Error("empty should be subset of any row")
	}
}

func TestSubsetSame(t *testing.T) {
	r := Empty.With(testLabel{name: "a"})
	if !Subset(r, r) {
		t.Error("row should be subset of itself")
	}
}

func TestSubsetSmaller(t *testing.T) {
	a := testLabel{name: "a"}
	r1 := Empty.With(a)
	r2 := Row{Labels: []Label{a, testLabel{name: "b"}}}

	if !Subset(r1, r2) {
		t.Error("smaller row should be subset of larger")
	}
}

func TestSubsetLargerNotSubset(t *testing.T) {
	a := testLabel{name: "a"}
	b := testLabel{name: "b"}
	r1 := Row{Labels: []Label{a, b}}
	r2 := Empty.With(a)

	if Subset(r1, r2) {
		t.Error("larger row should not be subset of smaller closed row")
	}
}

func TestSubsetUnknown(t *testing.T) {
	r := Empty.With(testLabel{name: "a"})
	if !Subset(r, Unknown) {
		t.Error("any row should be subset of Unknown")
	}

	if Subset(Unknown, r) {
		t.Error("Unknown should not be subset of closed row")
	}
}

func TestSubsetOpenTailOnlyIsNotSubsetOfClosedEmpty(t *testing.T) {
	if Subset(Open("rho"), Empty) {
		t.Error("open tail should not be subset of closed empty row")
	}
}

func TestSubsetOpenTailWithMatchingLabelsIsNotSubsetOfClosedRow(t *testing.T) {
	io := testLabel{name: "io"}
	if Subset(Open("rho", io), Empty.With(io)) {
		t.Error("open row should not be subset of closed row even when labels match")
	}
}

func TestSubsetClosedLabelsAreSubsetOfOpenRowWithMatchingLabels(t *testing.T) {
	io := testLabel{name: "io"}
	if !Subset(Empty.With(io), Open("rho", io)) {
		t.Error("closed row should be subset of open row with matching labels")
	}
}

func TestOpen(t *testing.T) {
	r := Open("e", testLabel{name: "a"})

	if !r.IsOpen() {
		t.Error("should be open")
	}

	if r.Tail.Name != "e" {
		t.Errorf("tail name: got %q, want %q", r.Tail.Name, "e")
	}

	if len(r.Labels) != 1 {
		t.Errorf("labels: got %d, want 1", len(r.Labels))
	}
}
