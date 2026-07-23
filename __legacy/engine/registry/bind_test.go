package registry

import "testing"

func TestBindOrderedUsesRoleOrder(t *testing.T) {
	bindings := BindOrdered(BindOptions[string, string, int]{
		Owner: "test",
		Roles: []Role[string, string]{
			{Key: "b", Value: "role-b"},
			{Key: "a", Value: "role-a"},
		},
		Handlers: map[string]int{
			"a": 1,
			"b": 2,
		},
		Valid:   func(v int) bool { return v > 0 },
		KeyName: func(k string) string { return k },
	})
	if len(bindings) != 2 {
		t.Fatalf("bindings = %d, want 2", len(bindings))
	}
	if bindings[0].Key != "b" || bindings[0].Role != "role-b" || bindings[0].Handler != 2 {
		t.Fatalf("bindings[0] = %#v, want role-b handler", bindings[0])
	}
	if bindings[1].Key != "a" || bindings[1].Role != "role-a" || bindings[1].Handler != 1 {
		t.Fatalf("bindings[1] = %#v, want role-a handler", bindings[1])
	}
}

func TestBindOrderedRejectsMissingInvalidAndOrphanHandlers(t *testing.T) {
	roles := []Role[string, string]{{Key: "a", Value: "role-a"}}
	valid := func(v int) bool { return v > 0 }
	name := func(k string) string { return k }

	requirePanic(t, func() {
		_ = BindOrdered(BindOptions[string, string, int]{
			Owner:    "test",
			Roles:    roles,
			Handlers: map[string]int{},
			Valid:    valid,
			KeyName:  name,
		})
	})
	requirePanic(t, func() {
		_ = BindOrdered(BindOptions[string, string, int]{
			Owner:    "test",
			Roles:    roles,
			Handlers: map[string]int{"a": 0},
			Valid:    valid,
			KeyName:  name,
		})
	})
	requirePanic(t, func() {
		_ = BindOrdered(BindOptions[string, string, int]{
			Owner:    "test",
			Roles:    roles,
			Handlers: map[string]int{"a": 1, "b": 2},
			Valid:    valid,
			KeyName:  name,
		})
	})
}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
