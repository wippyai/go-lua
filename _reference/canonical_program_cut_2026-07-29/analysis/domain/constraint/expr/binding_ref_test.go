package expr

import "testing"

func TestBindingRefOwnsReferenceKeyAndDisplaySpelling(t *testing.T) {
	tests := []struct {
		name    string
		ref     bindingRef
		display string
		key     string
	}{
		{name: "var", ref: varBindingRef("x"), display: "x", key: "x"},
		{name: "var len", ref: lenBindingRef("items"), display: "len(items)", key: "items.len"},
		{name: "param", ref: paramBindingRef(2), display: "param[2]", key: "param[2]"},
		{name: "param len", ref: paramLenBindingRef(2), display: "len(param[2])", key: "param[2].len"},
		{name: "ret", ref: retBindingRef(1), display: "ret[1]", key: "ret[1]"},
		{name: "ret len", ref: retLenBindingRef(1), display: "len(ret[1])", key: "ret[1].len"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ref.String(); got != tc.display {
				t.Fatalf("String() = %q, want %q", got, tc.display)
			}
			if got := tc.ref.Key(); got != tc.key {
				t.Fatalf("Key() = %q, want %q", got, tc.key)
			}
			if got := tc.ref.Variables(); len(got) != 1 || got[0] != tc.key {
				t.Fatalf("Variables() = %v, want [%q]", got, tc.key)
			}
			if got, ok := tc.ref.Eval(map[string]int64{tc.key: 42}); !ok || got != 42 {
				t.Fatalf("Eval() = %d/%v, want 42/true", got, ok)
			}
			if got := tc.ref.Substitute(map[string]Expr{tc.key: C(7)}, V("self")); !ExprEquals(got, C(7)) {
				t.Fatalf("Substitute() = %s, want 7", got)
			}
		})
	}
}
