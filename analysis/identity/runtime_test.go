package identity

import "testing"

func TestGenerationOrdering(t *testing.T) {
	cases := []struct {
		name     string
		left     Generation
		right    Generation
		precedes bool
	}{
		{name: "earlier precedes later", left: 1, right: 2, precedes: true},
		{name: "later does not precede earlier", left: 2, right: 1},
		{name: "equal does not precede", left: 2, right: 2},
		{name: "unavailable left", left: 0, right: 1},
		{name: "unavailable right", left: 1, right: 0},
		{name: "both unavailable", left: 0, right: 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.left.Precedes(testCase.right); got != testCase.precedes {
				t.Fatalf("Precedes = %t, want %t", got, testCase.precedes)
			}
		})
	}
}

func TestGenerationNext(t *testing.T) {
	cases := []struct {
		name    string
		current Generation
		next    Generation
	}{
		{name: "unpublished opens first revision", current: 0, next: 1},
		{name: "advance", current: 41, next: 42},
		{name: "exhausted saturates to unavailable", current: ^Generation(0), next: 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			next := testCase.current.Next()
			if next != testCase.next {
				t.Fatalf("Next = %d, want %d", next, testCase.next)
			}
			if testCase.current.Available() && next.Available() && !testCase.current.Precedes(next) {
				t.Fatalf("Next does not follow the current revision")
			}
		})
	}
}

func TestLocatorValidity(t *testing.T) {
	const (
		store      = StoreID(7)
		generation = Generation(3)
		slot       = uint32(11)
	)
	cases := []struct {
		name       string
		locator    Locator[uint32]
		store      StoreID
		generation Generation
		available  bool
		valid      bool
	}{
		{
			name:    "issued locator validates against its own store and revision",
			locator: NewLocator(store, generation, slot), store: store, generation: generation,
			available: true, valid: true,
		},
		{
			name:    "advanced store invalidates",
			locator: NewLocator(store, generation, slot), store: store, generation: generation + 1,
			available: true,
		},
		{
			name:    "earlier revision invalidates",
			locator: NewLocator(store, generation, slot), store: store, generation: generation - 1,
			available: true,
		},
		{
			name:    "other store invalidates",
			locator: NewLocator(store, generation, slot), store: store + 1, generation: generation,
			available: true,
		},
		{
			name:    "unavailable store yields the zero locator",
			locator: NewLocator(0, generation, slot), store: 0, generation: generation,
		},
		{
			name:    "unavailable generation yields the zero locator",
			locator: NewLocator(store, 0, slot), store: store, generation: 0,
		},
		{
			name:  "zero locator never validates",
			store: 0, generation: 0,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.locator.Available(); got != testCase.available {
				t.Fatalf("Available = %t, want %t", got, testCase.available)
			}
			if got := testCase.locator.Valid(testCase.store, testCase.generation); got != testCase.valid {
				t.Fatalf("Valid = %t, want %t", got, testCase.valid)
			}
		})
	}
}

func TestLocatorKeepsSlotOfOwnerType(t *testing.T) {
	type slot struct {
		row    uint32
		column uint32
	}
	locator := NewLocator(StoreID(1), Generation(1), slot{row: 4, column: 5})
	if locator.Slot != (slot{row: 4, column: 5}) {
		t.Fatalf("Slot = %+v", locator.Slot)
	}
	if NewLocator(StoreID(0), Generation(1), slot{row: 4}).Slot != (slot{}) {
		t.Fatalf("rejected locator retained a slot")
	}
}

func TestStoreIDAvailability(t *testing.T) {
	if StoreID(0).Available() {
		t.Fatalf("zero StoreID reports available")
	}
	if !StoreID(1).Available() {
		t.Fatalf("nonzero StoreID reports unavailable")
	}
}
