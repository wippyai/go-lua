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
		Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}},
		Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
	)
	if !r.HasMutate() {
		t.Error("Should have mutation")
	}

	if r.GetReturn(0) == nil {
		t.Error("Should have return")
	}

	if r.Pure() {
		t.Error("Should not be pure")
	}
}

func TestRowWithDuplicate(t *testing.T) {
	r := Empty.With(
		Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}},
		Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}},
	)
	count := 0

	for _, l := range r.Labels {
		if _, ok := l.(Mutate); ok {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Should have exactly 1 mutation, got %d", count)
	}
}

func TestRowString(t *testing.T) {
	tests := []struct {
		row  Row
		want string
	}{
		{Empty, "{}"},
		{Mutates(0, Unchanged{}), "{mutate(param[0], unchanged)}"},
		{Row{Labels: []Label{
			Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}},
			Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
		}}, "{mutate(param[0], unchanged), ret[0].type = elem(param[0])}"},
		{Open("rho", Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}}), "{mutate(param[0], unchanged) | rho}"},
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
		r1 := Row{Labels: []Label{Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}}}}
		r2 := Row{Labels: []Label{Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}}}
		u := Union(r1, r2)

		if !u.HasMutate() || u.GetReturn(0) == nil {
			t.Error("Union should have both mutation and return")
		}

		if !u.IsClosed() {
			t.Error("Union of closed rows should be closed")
		}
	})

	t.Run("open row", func(t *testing.T) {
		r1 := Open("rho", Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}})
		r2 := Row{Labels: []Label{Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}}}
		u := Union(r1, r2)

		if !u.HasMutate() || u.GetReturn(0) == nil {
			t.Error("Union should have both effects")
		}

		if !u.IsOpen() {
			t.Error("Union with open row should be open")
		}
	})

	t.Run("unknown absorbs", func(t *testing.T) {
		r := Row{Labels: []Label{Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}}}}
		u := Union(r, Unknown)

		if !u.IsUnknown() {
			t.Error("Union with Unknown should be Unknown")
		}
	})

	t.Run("deduplication", func(t *testing.T) {
		r1 := Row{Labels: []Label{Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}}}}
		r2 := Row{Labels: []Label{
			Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}},
			Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
		}}
		u := Union(r1, r2)

		count := 0

		for _, l := range u.Labels {
			if _, ok := l.(Mutate); ok {
				count++
			}
		}

		if count != 1 {
			t.Errorf("Union should deduplicate, got %d mutation labels", count)
		}
	})
}

func TestMutateEffect(t *testing.T) {
	m := Mutates(0, ElementUnion{Source: ParamRef{Index: 1}})

	if !m.HasMutate() {
		t.Error("Should have mutation")
	}

	got := m.GetMutate(0)
	if got == nil {
		t.Error("Should find mutation for param 0")
	}

	if m.GetMutate(1) != nil {
		t.Error("Should not find mutation for param 1")
	}
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
		{Empty, Mutates(0, Unchanged{}), false},
		{Mutates(0, Unchanged{}), Mutates(0, Unchanged{}), true},
		{Row{Labels: []Label{
			Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}},
			Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
		}}, Row{Labels: []Label{
			Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
			Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}},
		}}, true},
		{Open("rho"), Open("rho"), true},
		{Open("rho"), Open("sigma"), false},
		{Mutates(0, Unchanged{}), Open("rho", Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}}), false},
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
		Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}},
		Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
		Iterator{Source: ParamRef{Index: 0}, Kind: IterateIndexed},
	}}
	filtered := r.Without(func(l Label) bool {
		_, ok := l.(Return)
		return ok
	})

	if filtered.GetReturn(0) != nil {
		t.Error("Without should remove Return")
	}

	if !filtered.HasMutate() {
		t.Error("Without should keep Mutate")
	}

	if !filtered.HasIterator() {
		t.Error("Without should keep Iterator")
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

func TestIteratorEffects(t *testing.T) {
	r := Row{Labels: []Label{Iterator{Source: ParamRef{Index: 0}, Kind: IterateIndexed}}}

	if !r.HasIterator() {
		t.Error("Should have iterator")
	}

	iter := r.GetIterator()
	if iter == nil {
		t.Error("Should find iterator")
	}

	if !r.IsIndexedIterator() {
		t.Error("Should be indexed iterator")
	}

	if r.IsKeyedIterator() {
		t.Error("Should not be keyed iterator")
	}

	// Keyed iterator
	r2 := Row{Labels: []Label{Iterator{Source: ParamRef{Index: 0}, Kind: IterateKeyed}}}
	if !r2.IsKeyedIterator() {
		t.Error("Should be keyed iterator")
	}

	if r2.IsIndexedIterator() {
		t.Error("Should not be indexed iterator")
	}

	// No iterator
	r3 := Empty
	if r3.IsIndexedIterator() || r3.IsKeyedIterator() {
		t.Error("Empty row should not be any iterator")
	}
}

func TestTableMutatorEffects(t *testing.T) {
	r := Row{Labels: []Label{TableMutator{Target: ParamRef{Index: 0}, Value: ParamRef{Index: 1}}}}

	if !r.HasTableMutator() {
		t.Error("Should have table mutator")
	}

	mut := r.GetTableMutator()
	if mut == nil {
		t.Error("Should find table mutator")
	}

	// No mutator
	if Empty.GetTableMutator() != nil {
		t.Error("Empty row should not have table mutator")
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
		Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}},
		Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}},
	}}
	if !r.HasMutate() || r.GetReturn(0) == nil {
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
