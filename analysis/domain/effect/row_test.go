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
	r := Empty.With(BorrowAll{}, Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 1}})
	if !r.HasBorrow() {
		t.Error("Should have borrow")
	}

	if !r.HasStore() {
		t.Error("Should have store")
	}

	if r.Pure() {
		t.Error("Should not be pure")
	}
}

func TestRowWithDuplicate(t *testing.T) {
	r := Empty.With(BorrowAll{}, BorrowAll{})
	count := 0

	for _, l := range r.Labels {
		if _, ok := l.(BorrowAll); ok {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Should have exactly 1 borrow_all, got %d", count)
	}
}

func TestRowString(t *testing.T) {
	tests := []struct {
		row  Row
		want string
	}{
		{Empty, "{}"},
		{BorrowsOnly(), "{borrow_all}"},
		{Row{Labels: []Label{BorrowAll{}, Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 1}}}}, "{borrow_all, store(param[0] into param[1])}"},
		{Open("rho", BorrowAll{}), "{borrow_all | rho}"},
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
		r1 := Row{Labels: []Label{BorrowAll{}}}
		r2 := Row{Labels: []Label{Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 1}}}}
		u := Union(r1, r2)

		if !u.HasBorrow() || !u.HasStore() {
			t.Error("Union should have both borrow and store")
		}

		if !u.IsClosed() {
			t.Error("Union of closed rows should be closed")
		}
	})

	t.Run("open row", func(t *testing.T) {
		r1 := Open("rho", BorrowAll{})
		r2 := Row{Labels: []Label{Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 1}}}}
		u := Union(r1, r2)

		if !u.HasBorrow() || !u.HasStore() {
			t.Error("Union should have both effects")
		}

		if !u.IsOpen() {
			t.Error("Union with open row should be open")
		}
	})

	t.Run("unknown absorbs", func(t *testing.T) {
		r := Row{Labels: []Label{BorrowAll{}}}
		u := Union(r, Unknown)

		if !u.IsUnknown() {
			t.Error("Union with Unknown should be Unknown")
		}
	})

	t.Run("deduplication", func(t *testing.T) {
		r1 := Row{Labels: []Label{BorrowAll{}}}
		r2 := Row{Labels: []Label{BorrowAll{}, Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 1}}}}
		u := Union(r1, r2)

		count := 0

		for _, l := range u.Labels {
			if _, ok := l.(BorrowAll); ok {
				count++
			}
		}

		if count != 1 {
			t.Errorf("Union should deduplicate, got %d borrow_all labels", count)
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
		{Empty, BorrowsOnly(), false},
		{BorrowsOnly(), BorrowsOnly(), true},
		{Row{Labels: []Label{BorrowAll{}, Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 1}}}}, Row{Labels: []Label{Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 1}}, BorrowAll{}}}, true},
		{Open("rho"), Open("rho"), true},
		{Open("rho"), Open("sigma"), false},
		{BorrowsOnly(), Open("rho", BorrowAll{}), false},
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
	r := Row{Labels: []Label{BorrowAll{}, Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 1}}, Freeze{Param: ParamRef{Index: 0}}}}
	filtered := r.Without(func(l Label) bool {
		_, ok := l.(Store)
		return ok
	})

	if filtered.HasStore() {
		t.Error("Without should remove Store")
	}

	if !filtered.HasBorrow() {
		t.Error("Without should keep BorrowAll")
	}

	if !filtered.Has(func(l Label) bool { _, ok := l.(Freeze); return ok }) {
		t.Error("Without should keep Freeze")
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

func TestBorrowStoreEffects(t *testing.T) {
	// BorrowsOnly
	r := BorrowsOnly()
	if !r.HasBorrow() {
		t.Error("BorrowsOnly should have borrow")
	}

	if !r.BorrowsAllParams() {
		t.Error("BorrowsOnly should borrow all params")
	}

	// Borrow param
	r2 := Row{Labels: []Label{Borrow{Param: ParamRef{Index: 0}}}}
	if !r2.HasBorrow() {
		t.Error("Borrow should have borrow")
	}

	b := r2.GetBorrow(0)
	if b == nil {
		t.Error("Should find borrow for param 0")
	}

	if r2.GetBorrow(1) != nil {
		t.Error("Should not find borrow for param 1")
	}

	// StoresParam
	r3 := StoresParam(0, 1)
	if !r3.HasStore() {
		t.Error("StoresParam should have store")
	}

	s := r3.GetStore(0)
	if s == nil {
		t.Error("Should find store for param 0")
	}

	if r3.GetStore(1) != nil {
		t.Error("Should not find store for param 1")
	}

	// OnlyBorrows
	if !r2.OnlyBorrows() {
		t.Error("BorrowsParam should only borrow")
	}

	if r3.OnlyBorrows() {
		t.Error("StoresParam should not only borrow")
	}

	if Empty.OnlyBorrows() {
		t.Error("Empty should not only borrow")
	}

	// Mutate with borrow
	r4 := Row{Labels: []Label{Borrow{Param: ParamRef{Index: 0}}, Mutate{Target: ParamRef{Index: 0}}}}
	if r4.OnlyBorrows() {
		t.Error("Borrow+Mutate should not be only borrows")
	}
}

func TestNewRow(t *testing.T) {
	r := Row{Labels: []Label{BorrowAll{}, Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 1}}}}
	if !r.HasBorrow() || !r.HasStore() {
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
