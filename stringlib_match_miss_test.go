package lua

import "testing"

// A failed string.match answers one nil, as the Lua 5.4 reference interpreter
// does, so it is a single argument of an enclosing call.
func TestStringMatchMissAnswersOneNil(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		{"match", `string.match("abc", "x")`},
		{"match with init", `string.match("abc", "a", 2)`},
		{"match with captures", `string.match("abc", "(x)(y)")`},
		{"method", `("abc"):match("%d+")`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString("return " + tc.expr); err != nil {
				t.Fatal(err)
			}
			if L.GetTop() != 1 || L.Get(1) != LNil {
				t.Fatalf("got %d results (first %v), want one nil", L.GetTop(), L.Get(1))
			}
		})
	}

	t.Run("an enclosing call takes the miss as its argument", func(t *testing.T) {
		L := NewState()
		defer L.Close()
		if err := L.DoString(`return tonumber(("abc"):match("%d+"))`); err != nil {
			t.Fatalf("tonumber of a failed match: %v", err)
		}
		if L.GetTop() != 1 || L.Get(1) != LNil {
			t.Fatalf("got %d results (first %v), want one nil", L.GetTop(), L.Get(1))
		}
	})
}
