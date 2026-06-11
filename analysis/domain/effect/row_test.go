package effect

import "testing"

func hasTestLabel(r Row, label testLabel) bool {
	return r.Has(func(l Label) bool {
		return l.Equals(label)
	})
}

func TestEmpty(t *testing.T) {
	if !Empty.Pure() {
		t.Error("Empty should be pure")
	}

	if !Empty.IsClosed() {
		t.Error("Empty should be closed")
	}

	if Empty.String() != "{}" {
		t.Errorf("Empty.String() = %q, want {}", Empty.String())
	}
}

func TestUnknown(t *testing.T) {
	if Unknown.Pure() {
		t.Error("Unknown should not be pure")
	}

	if !Unknown.IsOpen() {
		t.Error("Unknown should be open")
	}

	if !Unknown.IsUnknown() {
		t.Error("Unknown should be unknown")
	}
}

func TestRowWith(t *testing.T) {
	a := testLabel{name: "a"}
	b := testLabel{name: "b"}
	r := Empty.With(a, b)

	if !hasTestLabel(r, a) {
		t.Error("Should have first label")
	}

	if !hasTestLabel(r, b) {
		t.Error("Should have second label")
	}

	if r.Pure() {
		t.Error("Should not be pure")
	}
}

func TestRowWithDuplicate(t *testing.T) {
	a := testLabel{name: "a"}
	r := Empty.With(a, a)

	if len(r.Labels) != 1 {
		t.Errorf("Should have exactly 1 label, got %d", len(r.Labels))
	}
}

func TestRowString(t *testing.T) {
	a := testLabel{name: "a"}
	b := testLabel{name: "b"}
	tests := []struct {
		row  Row
		want string
	}{
		{Empty, "{}"},
		{Row{Labels: []Label{a}}, "{a}"},
		{Row{Labels: []Label{a, b}}, "{a, b}"},
		{Open("rho", a), "{a | rho}"},
		{Open("rho"), "{rho}"},
	}

	for _, tt := range tests {
		if got := tt.row.String(); got != tt.want {
			t.Errorf("Row.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestUnion(t *testing.T) {
	t.Run("closed rows", func(t *testing.T) {
		a := testLabel{name: "a"}
		b := testLabel{name: "b"}
		r1 := Row{Labels: []Label{a}}
		r2 := Row{Labels: []Label{b}}
		u := Union(r1, r2)

		if !hasTestLabel(u, a) || !hasTestLabel(u, b) {
			t.Error("Union should have both effects")
		}

		if !u.IsClosed() {
			t.Error("Union of closed rows should be closed")
		}
	})

	t.Run("open row", func(t *testing.T) {
		a := testLabel{name: "a"}
		b := testLabel{name: "b"}
		r1 := Open("rho", a)
		r2 := Row{Labels: []Label{b}}
		u := Union(r1, r2)

		if !hasTestLabel(u, a) || !hasTestLabel(u, b) {
			t.Error("Union should have both effects")
		}

		if !u.IsOpen() {
			t.Error("Union with open row should be open")
		}
	})

	t.Run("unknown absorbs", func(t *testing.T) {
		r := Row{Labels: []Label{testLabel{name: "a"}}}
		u := Union(r, Unknown)

		if !u.IsUnknown() {
			t.Error("Union with Unknown should be Unknown")
		}
	})

	t.Run("deduplication", func(t *testing.T) {
		a := testLabel{name: "a"}
		b := testLabel{name: "b"}
		r1 := Row{Labels: []Label{a}}
		r2 := Row{Labels: []Label{a, b}}
		u := Union(r1, r2)

		if len(u.Labels) != 2 {
			t.Errorf("Union should deduplicate, got %d labels", len(u.Labels))
		}
	})
}

func TestRowEquals(t *testing.T) {
	a := testLabel{name: "a"}
	b := testLabel{name: "b"}
	tests := []struct {
		r1, r2 Row
		want   bool
	}{
		{Empty, Empty, true},
		{Empty, Row{Labels: []Label{a}}, false},
		{Row{Labels: []Label{a}}, Row{Labels: []Label{a}}, true},
		{Row{Labels: []Label{a, b}}, Row{Labels: []Label{b, a}}, true},
		{Open("rho"), Open("rho"), true},
		{Open("rho"), Open("sigma"), false},
		{Row{Labels: []Label{a}}, Open("rho", a), false},
	}

	for _, tt := range tests {
		got := tt.r1.Equals(tt.r2)
		if got != tt.want {
			t.Errorf("%v.Equals(%v) = %v, want %v", tt.r1, tt.r2, got, tt.want)
		}
	}
}

func TestRowHashIgnoresLabelOrder(t *testing.T) {
	a := testLabel{name: "a"}
	b := testLabel{name: "b"}
	left := Row{Labels: []Label{a, b}}
	right := Row{Labels: []Label{b, a}}

	if !left.Equals(right) {
		t.Fatal("test setup expected equal rows")
	}
	if left.Hash() != right.Hash() {
		t.Fatalf("equal rows should have equal hashes: %d != %d", left.Hash(), right.Hash())
	}
}

func TestRowCloneCopiesTailAndLabels(t *testing.T) {
	a := testLabel{name: "a"}
	row := Open("rho", a)
	clone := row.Clone()

	if !row.Equals(clone) {
		t.Fatalf("clone = %v, want %v", clone, row)
	}
	clone.Labels[0] = testLabel{name: "b"}
	clone.Tail.Name = "sigma"
	if row.Labels[0].String() != "a" || row.Tail.Name != "rho" {
		t.Fatalf("clone mutation changed source row: %v", row)
	}
}

func TestReads(t *testing.T) {
	r := Empty
	if !r.Pure() {
		t.Error("Reads should be pure (empty)")
	}

	if r.String() != "{}" {
		t.Errorf("Empty.String() = %q, want {}", r.String())
	}
}

func TestRowWithout(t *testing.T) {
	a := testLabel{name: "a"}
	b := testLabel{name: "b"}
	c := testLabel{name: "c"}
	r := Row{Labels: []Label{a, b, c}}
	filtered := r.Without(func(l Label) bool {
		return l.Equals(a)
	})

	if hasTestLabel(filtered, a) {
		t.Error("Without should remove matching label")
	}

	if !hasTestLabel(filtered, b) {
		t.Error("Without should keep non-matching label")
	}

	if !hasTestLabel(filtered, c) {
		t.Error("Without should keep later non-matching label")
	}
}

func TestVarString(t *testing.T) {
	v := &Var{Name: "rho"}
	if got := v.String(); got != "rho" {
		t.Errorf("Var.String() = %q, want 'rho'", got)
	}

	var nilVar *Var
	if got := nilVar.String(); got != "" {
		t.Errorf("nil Var.String() = %q, want ''", got)
	}
}

func TestRowStringOpenNoLabels(t *testing.T) {
	r := Row{Tail: &Var{Name: "rho"}}
	if got := r.String(); got != "{rho}" {
		t.Errorf("Open row no labels.String() = %q, want '{rho}'", got)
	}
}

func TestNewRow(t *testing.T) {
	a := testLabel{name: "a"}
	b := testLabel{name: "b"}
	r := Row{Labels: []Label{a, b}}
	if !hasTestLabel(r, a) || !hasTestLabel(r, b) {
		t.Error("Row should have the given labels")
	}
}

func TestRowEqualsNonRow(t *testing.T) {
	r := Empty
	if r.Equals("not a row") {
		t.Error("Row should not equal non-Row type")
	}
}
