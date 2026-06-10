package effect

import "testing"

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
	r := Empty.With(
		Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
		ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
	)

	if r.GetReturn(0) == nil {
		t.Error("Should have return")
	}

	if r.GetErrorReturn(0) == nil {
		t.Error("Should have error return")
	}

	if r.Pure() {
		t.Error("Should not be pure")
	}
}

func TestRowWithDuplicate(t *testing.T) {
	r := Empty.With(
		Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
		Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
	)
	count := 0

	for _, l := range r.Labels {
		if _, ok := l.(Return); ok {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Should have exactly 1 return, got %d", count)
	}
}

func TestRowString(t *testing.T) {
	tests := []struct {
		row  Row
		want string
	}{
		{Empty, "{}"},
		{Returns(0, ElementOf{Source: ParamRef{Index: 0}}), "{ret[0].type = elem(param[0])}"},
		{Row{Labels: []Label{
			Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
			ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
		}}, "{ret[0].type = elem(param[0]), errret(val[0], err[1])}"},
		{Open("rho", Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}), "{ret[0].type = elem(param[0]) | rho}"},
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
		r1 := Row{Labels: []Label{Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}}}
		r2 := Row{Labels: []Label{ErrorReturn{ValueIndex: 0, ErrorIndex: 1}}}
		u := Union(r1, r2)

		if u.GetReturn(0) == nil || u.GetErrorReturn(0) == nil {
			t.Error("Union should have both effects")
		}

		if !u.IsClosed() {
			t.Error("Union of closed rows should be closed")
		}
	})

	t.Run("open row", func(t *testing.T) {
		r1 := Open("rho", Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}})
		r2 := Row{Labels: []Label{ErrorReturn{ValueIndex: 0, ErrorIndex: 1}}}
		u := Union(r1, r2)

		if u.GetReturn(0) == nil || u.GetErrorReturn(0) == nil {
			t.Error("Union should have both effects")
		}

		if !u.IsOpen() {
			t.Error("Union with open row should be open")
		}
	})

	t.Run("unknown absorbs", func(t *testing.T) {
		r := Row{Labels: []Label{Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}}}
		u := Union(r, Unknown)

		if !u.IsUnknown() {
			t.Error("Union with Unknown should be Unknown")
		}
	})

	t.Run("deduplication", func(t *testing.T) {
		r1 := Row{Labels: []Label{Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}}}
		r2 := Row{Labels: []Label{
			Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
			ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
		}}
		u := Union(r1, r2)

		count := 0

		for _, l := range u.Labels {
			if _, ok := l.(Return); ok {
				count++
			}
		}

		if count != 1 {
			t.Errorf("Union should deduplicate, got %d return labels", count)
		}
	})
}

func TestReturnEffect(t *testing.T) {
	r := Returns(0, ElementOf{Source: ParamRef{Index: 0}})

	got := r.GetReturn(0)
	if got == nil {
		t.Error("Should find return for index 0")
	}

	if r.GetReturn(1) != nil {
		t.Error("Should not find return for index 1")
	}
}

func TestRowEquals(t *testing.T) {
	tests := []struct {
		r1, r2 Row
		want   bool
	}{
		{Empty, Empty, true},
		{Empty, Returns(0, ElementOf{Source: ParamRef{Index: 0}}), false},
		{Returns(0, ElementOf{Source: ParamRef{Index: 0}}), Returns(0, ElementOf{Source: ParamRef{Index: 0}}), true},
		{Row{Labels: []Label{
			Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
			ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
		}}, Row{Labels: []Label{
			ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
			Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
		}}, true},
		{Open("rho"), Open("rho"), true},
		{Open("rho"), Open("sigma"), false},
		{Returns(0, ElementOf{Source: ParamRef{Index: 0}}), Open("rho", Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}), false},
	}

	for _, tt := range tests {
		got := tt.r1.Equals(tt.r2)
		if got != tt.want {
			t.Errorf("%v.Equals(%v) = %v, want %v", tt.r1, tt.r2, got, tt.want)
		}
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
	r := Row{Labels: []Label{
		Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
		ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
		CorrelatedReturn{Indices: []int{0, 1}},
	}}
	filtered := r.Without(func(l Label) bool {
		_, ok := l.(Return)
		return ok
	})

	if filtered.GetReturn(0) != nil {
		t.Error("Without should remove Return")
	}

	if filtered.GetErrorReturn(0) == nil {
		t.Error("Without should keep ErrorReturn")
	}

	if filtered.GetCorrelatedReturn(1) == nil {
		t.Error("Without should keep CorrelatedReturn")
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

func TestReturnLengthEffect(t *testing.T) {
	r := Row{Labels: []Label{ReturnLength{ReturnIndex: 0, Length: nil}}}

	rl := r.GetReturnLength(0)
	if rl == nil {
		t.Error("Should find return length")
	}

	if r.GetReturnLength(1) != nil {
		t.Error("Should not find return length for wrong index")
	}
}

func TestErrorReturnEffect(t *testing.T) {
	r := Row{Labels: []Label{ErrorReturn{ValueIndex: 0, ErrorIndex: 1}}}

	er := r.GetErrorReturn(0)
	if er == nil {
		t.Error("Should find error return")
	}

	if r.GetErrorReturn(1) != nil {
		t.Error("Should not find error return for wrong value index")
	}
}

func TestNewRow(t *testing.T) {
	r := Row{Labels: []Label{
		Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
		ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
	}}
	if r.GetReturn(0) == nil || r.GetErrorReturn(0) == nil {
		t.Error("Row should have the given labels")
	}
}

func TestGetCorrelatedReturn(t *testing.T) {
	r := Row{Labels: []Label{CorrelatedReturn{Indices: []int{0, 1, 2}}}}

	cr := r.GetCorrelatedReturn(1)
	if cr == nil {
		t.Error("Should find correlated return for index 1")
	}

	if r.GetCorrelatedReturn(5) != nil {
		t.Error("Should not find correlated return for index 5")
	}

	if Empty.GetCorrelatedReturn(0) != nil {
		t.Error("Empty row should not have correlated return")
	}
}

func TestRowEqualsNonRow(t *testing.T) {
	r := Empty
	if r.Equals("not a row") {
		t.Error("Row should not equal non-Row type")
	}
}
