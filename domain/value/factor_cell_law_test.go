package value

import "testing"

func TestAuthenticateFactorCellPinsSparseBottomToItsOwner(t *testing.T) {
	schema := entryProofSchema(t, 2)
	bottom := schema.Bottom()
	top := schema.Top()

	for _, fixture := range []struct {
		name      string
		value     Value
		present   bool
		available bool
		want      bool
	}{
		{name: "present-bottom", value: bottom, present: true, available: true, want: true},
		{name: "present-top", value: top, present: true, available: true, want: true},
		{name: "sparse-bottom", value: bottom, available: true, want: true},
		{name: "sparse-top", value: top, available: true},
		{name: "zero", value: Value{}, available: true},
		{name: "unavailable", value: bottom},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			got, ok := schema.AuthenticateFactorCell(fixture.value, fixture.present, fixture.available)
			if ok != fixture.want {
				t.Fatalf("AuthenticateFactorCell(_, %t, %t) ok = %t, want %t", fixture.present, fixture.available, ok, fixture.want)
			}
			if ok && !schema.Equal(got, fixture.value) {
				t.Fatal("authenticated factor cell changed the owner-issued fact")
			}
		})
	}

	foreign := entryProofSchema(t, 1)
	if _, ok := schema.AuthenticateFactorCell(foreign.Bottom(), false, true); ok {
		t.Fatal("foreign sparse Bottom crossed the Value owner fence")
	}
}
