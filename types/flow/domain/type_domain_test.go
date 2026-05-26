package domain

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func makeTestEnv(types map[constraint.PathKey]typ.Type) constraint.Env {
	return constraint.Env{
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			if p.Symbol == 0 {
				return constraint.PathKey(p.Root)
			}
			return p.Key()
		},
		PathTypeAt: func(key constraint.PathKey) typ.Type {
			if types == nil {
				return nil
			}
			return types[key]
		},
	}
}

func TestTypeDomain_ApplyTruthy(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeTestEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	d := NewTypeDomain(env)
	atom := constraint.AtomTruthy(constraint.TermVar(key))

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	result := d.TypeAt(key)
	if result == nil {
		t.Fatal("expected narrowed type")
	}
	if result.Kind() != typ.String.Kind() {
		t.Fatalf("expected string, got %v", result)
	}
}

func TestTypeDomain_ApplyFalsy(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeTestEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	d := NewTypeDomain(env)
	atom := constraint.AtomFalsy(constraint.TermVar(key))

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	result := d.TypeAt(key)
	if result != typ.Nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestTypeDomain_ApplyHasType(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeTestEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Number, typ.Nil),
	})

	d := NewTypeDomain(env)
	atom := constraint.AtomHasType(constraint.TermVar(key), narrow.BuiltinTypeKey("number"))

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	result := d.TypeAt(key)
	if result == nil {
		t.Fatal("expected narrowed type")
	}
	if result.Kind() != typ.Number.Kind() {
		t.Fatalf("expected number, got %v", result)
	}
}

func TestTypeDomain_ApplyHasTypeOnFieldMaterializesParentShape(t *testing.T) {
	parentKey := constraint.PathKey("op")
	fieldKey := constraint.PathKey("op.from_pid")
	env := makeTestEnv(map[constraint.PathKey]typ.Type{
		parentKey: typ.Any,
		fieldKey:  typ.Any,
	})

	d := NewTypeDomain(env)
	atom := constraint.AtomHasType(constraint.TermVar(fieldKey), narrow.BuiltinTypeKey("string"))

	if ok := d.ApplyAtom(atom); !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	wantParent := typ.NewRecord().Field("from_pid", typ.String).SetOpen(true).Build()
	if got := d.TypeAt(parentKey); !typ.TypeEquals(got, wantParent) {
		t.Fatalf("parent type = %v, want %v", got, wantParent)
	}
	if got := d.TypeAt(fieldKey); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("field type = %v, want string", got)
	}
}

func TestTypeDomain_ApplyHasTypeOnClosedMissingFieldIsUnsat(t *testing.T) {
	parentKey := constraint.PathKey("op")
	fieldKey := constraint.PathKey("op.from_pid")
	env := makeTestEnv(map[constraint.PathKey]typ.Type{
		parentKey: typ.NewRecord().Field("kind", typ.String).Build(),
		fieldKey:  typ.Any,
	})

	d := NewTypeDomain(env)
	atom := constraint.AtomHasType(constraint.TermVar(fieldKey), narrow.BuiltinTypeKey("string"))

	if ok := d.ApplyAtom(atom); ok {
		t.Fatal("ApplyAtom should reject impossible field type guard")
	}
	if !d.IsUnsat() {
		t.Fatal("domain should be unsat")
	}
}

func TestTypeDomain_ApplyIsNil(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeTestEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	d := NewTypeDomain(env)
	atom := constraint.AtomEq(constraint.TermVar(key), constraint.TermNil())

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	result := d.TypeAt(key)
	if result != typ.Nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestTypeDomain_ApplyIsNilOnFieldMaterializesParentShape(t *testing.T) {
	parentKey := constraint.PathKey("raw")
	fieldKey := constraint.PathKey("raw.kind")
	env := makeTestEnv(map[constraint.PathKey]typ.Type{
		parentKey: typ.Any,
		fieldKey:  typ.Any,
	})

	d := NewTypeDomain(env)
	atom := constraint.AtomEq(constraint.TermVar(fieldKey), constraint.TermNil())

	if ok := d.ApplyAtom(atom); !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	wantParent := typ.NewRecord().Field("kind", typ.Nil).SetOpen(true).Build()
	if got := d.TypeAt(parentKey); !typ.TypeEquals(got, wantParent) {
		t.Fatalf("parent type = %v, want %v", got, wantParent)
	}
	if got := d.TypeAt(fieldKey); !typ.TypeEquals(got, typ.Nil) {
		t.Fatalf("field type = %v, want nil", got)
	}
}

func TestTypeDomain_ApplyIsNil_RespectsPriorTruthyNarrowing(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeTestEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	cases := []struct {
		name  string
		atoms []constraint.Atom
	}{
		{
			name: "truthy then isnil",
			atoms: []constraint.Atom{
				constraint.AtomTruthy(constraint.TermVar(key)),
				constraint.AtomEq(constraint.TermVar(key), constraint.TermNil()),
			},
		},
		{
			name: "isnil then truthy",
			atoms: []constraint.Atom{
				constraint.AtomEq(constraint.TermVar(key), constraint.TermNil()),
				constraint.AtomTruthy(constraint.TermVar(key)),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewTypeDomain(env)

			if ok := d.ApplyAtom(tc.atoms[0]); !ok {
				t.Fatalf("first atom unexpectedly failed; unsat=%v", d.IsUnsat())
			}
			if ok := d.ApplyAtom(tc.atoms[1]); ok {
				t.Fatal("second atom should make the domain unsatisfiable")
			}
			if !d.IsUnsat() {
				t.Fatal("domain should be marked unsatisfiable")
			}
		})
	}
}

func TestTypeDomain_ApplyNotNil(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeTestEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	d := NewTypeDomain(env)
	atom := constraint.AtomNe(constraint.TermVar(key), constraint.TermNil())

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	result := d.TypeAt(key)
	if result == nil {
		t.Fatal("expected narrowed type")
	}
	if result.Kind() != typ.String.Kind() {
		t.Fatalf("expected string, got %v", result)
	}
}

func TestTypeDomain_Unsat_OnImpossibleNarrowing(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeTestEnv(map[constraint.PathKey]typ.Type{
		key: typ.String, // Only string
	})

	d := NewTypeDomain(env)
	// Apply falsy to string-only type should be unsat
	atom := constraint.AtomFalsy(constraint.TermVar(key))

	ok := d.ApplyAtom(atom)
	if ok {
		t.Fatal("ApplyAtom should fail on impossible narrowing")
	}
	if !d.IsUnsat() {
		t.Fatal("domain should be unsat")
	}
}

func TestTypeDomain_Clone(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeTestEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	d := NewTypeDomain(env)
	d.ApplyAtom(constraint.AtomTruthy(constraint.TermVar(key)))

	clone := d.Clone().(*TypeDomain)

	if !typ.TypeEquals(d.TypeAt(key), clone.TypeAt(key)) {
		t.Fatal("clone should have same narrowed type")
	}

	// Modify clone
	clone.Narrowed[key] = typ.Number
	if d.TypeAt(key).Kind() != typ.String.Kind() {
		t.Fatal("original should be unchanged")
	}
}

func TestTypeDomain_Join(t *testing.T) {
	env := makeTestEnv(nil)

	a := NewTypeDomain(env)
	a.Narrowed["x"] = typ.String

	b := NewTypeDomain(env)
	b.Narrowed["x"] = typ.Number

	joined := a.Join(b).(*TypeDomain)
	result := joined.TypeAt("x")

	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(u.Members))
	}
}

func TestTypeDomain_JoinCoalescesRecursiveFamilies(t *testing.T) {
	env := makeTestEnv(nil)
	key := constraint.PathKey("node")
	base := typ.NewRecursive("FlowA", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	withPath := typ.NewRecursive("FlowB", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})

	a := NewTypeDomain(env)
	a.Narrowed[key] = base
	b := NewTypeDomain(env)
	b.Narrowed[key] = withPath

	ab := a.Join(b).(*TypeDomain).NarrowedTypeAt(key)
	ba := b.Join(a).(*TypeDomain).NarrowedTypeAt(key)
	if !typ.TypeEquals(ab, ba) {
		t.Fatalf("recursive domain join differs by order: %v vs %v", ab, ba)
	}
	if ab.Hash() != ba.Hash() {
		t.Fatalf("recursive domain join hash differs by order: %d vs %d", ab.Hash(), ba.Hash())
	}
	rec, ok := ab.(*typ.Recursive)
	if !ok {
		t.Fatalf("joined recursive family = %T %[1]v, want recursive", ab)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T, want record", rec.Body)
	}
	fullPath := body.GetField("full_path")
	if fullPath == nil || !fullPath.Optional || !typ.TypeEquals(fullPath.Type, typ.String) {
		t.Fatalf("full_path field = %v, want optional string", fullPath)
	}
}

func TestTypeDomain_Join_DropsNonCommonKeys(t *testing.T) {
	env := makeTestEnv(nil)

	a := NewTypeDomain(env)
	a.Narrowed["x"] = typ.String

	b := NewTypeDomain(env)
	// b has no key "x"

	joined := a.Join(b).(*TypeDomain)
	if _, hasKey := joined.Narrowed["x"]; hasKey {
		t.Fatal("key should be dropped when not in both sides")
	}
}

func TestTypeDomain_Join_UnsatSideIgnored(t *testing.T) {
	env := makeTestEnv(nil)

	a := NewTypeDomain(env)
	a.Narrowed["x"] = typ.String

	b := NewTypeDomain(env)
	b.Unsat = true

	joined := a.Join(b).(*TypeDomain)
	if joined.TypeAt("x").Kind() != typ.String.Kind() {
		t.Fatal("should use non-unsat side")
	}
}
