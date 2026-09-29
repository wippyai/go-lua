package core

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestInstantiateGeneric(t *testing.T) {
	t.Run("nil generic", func(t *testing.T) {
		_, err := InstantiateGeneric(nil, nil)
		if !errors.Is(err, ErrNotGeneric) {
			t.Errorf("expected ErrNotGeneric, got %v", err)
		}
	})

	t.Run("wrong type arg count", func(t *testing.T) {
		g := typ.NewGeneric("G",
			[]*typ.TypeParam{{Name: "T"}},
			typ.NewTypeParam("T", nil),
		)

		_, err := InstantiateGeneric(g, []typ.Type{typ.String, typ.Integer})
		if !errors.Is(err, ErrTypeArgCount) {
			t.Errorf("expected ErrTypeArgCount, got %v", err)
		}
	})

	t.Run("constraint violation", func(t *testing.T) {
		g := typ.NewGeneric("G",
			[]*typ.TypeParam{{Name: "T", Constraint: typ.String}},
			typ.NewTypeParam("T", nil),
		)

		_, err := InstantiateGeneric(g, []typ.Type{typ.Integer})
		if !errors.Is(err, ErrConstraintViolation) {
			t.Errorf("expected ErrConstraintViolation, got %v", err)
		}
	})

	t.Run("valid instantiation", func(t *testing.T) {
		g := typ.NewGeneric("G",
			[]*typ.TypeParam{{Name: "T"}},
			typ.NewArray(typ.NewTypeParam("T", nil)),
		)

		result, err := InstantiateGeneric(g, []typ.Type{typ.String})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result == nil {
			t.Error("expected non-nil result")
		}
		// Result should be Array of String
		if arr, ok := result.(*typ.Array); ok {
			if arr.Element != typ.String {
				t.Errorf("expected string element, got %v", arr.Element)
			}
		} else {
			t.Errorf("expected Array, got %T", result)
		}
	})
}

func TestResolveInstantiated(t *testing.T) {
	g := typ.NewGeneric("G",
		[]*typ.TypeParam{{Name: "T"}},
		typ.NewArray(typ.NewTypeParam("T", nil)),
	)
	inst := typ.Instantiate(g, typ.String)

	result, err := ResolveInstantiated(inst)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if arr, ok := result.(*typ.Array); ok {
		if arr.Element != typ.String {
			t.Error("expected String element")
		}
	} else {
		t.Errorf("expected Array, got %T", result)
	}
}

func TestCollectTypeParams(t *testing.T) {
	t.Run("nil type", func(t *testing.T) {
		params := CollectTypeParams(nil)
		if len(params) != 0 {
			t.Error("expected empty")
		}
	})

	t.Run("no type params", func(t *testing.T) {
		params := CollectTypeParams(typ.String)
		if len(params) != 0 {
			t.Error("expected empty")
		}
	})

	t.Run("single type param", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)

		params := CollectTypeParams(tp)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in array", func(t *testing.T) {
		arr := typ.NewArray(typ.NewTypeParam("T", nil))

		params := CollectTypeParams(arr)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in map", func(t *testing.T) {
		m := typ.NewMap(typ.NewTypeParam("K", nil), typ.NewTypeParam("V", nil))

		params := CollectTypeParams(m)
		if len(params) != 2 {
			t.Errorf("expected 2, got %d", len(params))
		}
	})

	t.Run("type param in function", func(t *testing.T) {
		fn := typ.Func().
			Param("x", typ.NewTypeParam("T", nil)).
			Variadic(typ.NewTypeParam("U", nil)).
			Returns(typ.NewTypeParam("V", nil)).
			Build()

		params := CollectTypeParams(fn)
		if len(params) != 3 {
			t.Errorf("expected 3, got %d", len(params))
		}
	})

	t.Run("type param in record", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.NewTypeParam("T", nil)).Build()

		params := CollectTypeParams(rec)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in tuple", func(t *testing.T) {
		tuple := typ.NewTuple(typ.NewTypeParam("T", nil), typ.NewTypeParam("U", nil))

		params := CollectTypeParams(tuple)
		if len(params) != 2 {
			t.Errorf("expected 2, got %d", len(params))
		}
	})

	t.Run("type param in optional", func(t *testing.T) {
		opt := typ.NewOptional(typ.NewTypeParam("T", nil))

		params := CollectTypeParams(opt)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in union", func(t *testing.T) {
		union := typ.NewUnion(typ.NewTypeParam("T", nil), typ.String)

		params := CollectTypeParams(union)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in intersection", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.NewTypeParam("T", nil)).Build()
		inter := typ.NewIntersection(rec, typ.NewRecord().Build())

		params := CollectTypeParams(inter)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in alias", func(t *testing.T) {
		alias := typ.NewAlias("A", typ.NewTypeParam("T", nil))

		params := CollectTypeParams(alias)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("type param in instantiated", func(t *testing.T) {
		g := typ.NewGeneric("G", []*typ.TypeParam{{Name: "U"}}, typ.NewTypeParam("U", nil))
		inst := typ.Instantiate(g, typ.NewTypeParam("T", nil))

		params := CollectTypeParams(inst)
		if len(params) != 1 {
			t.Errorf("expected 1, got %d", len(params))
		}
	})

	t.Run("ref type", func(t *testing.T) {
		ref := typ.NewRef("mod", "Type")

		params := CollectTypeParams(ref)
		if len(params) != 0 {
			t.Error("expected empty for ref")
		}
	})
}

func TestHasTypeParams(t *testing.T) {
	if HasTypeParams(typ.String) {
		t.Error("expected false for primitive")
	}

	if !HasTypeParams(typ.NewTypeParam("T", nil)) {
		t.Error("expected true for type param")
	}

	if !HasTypeParams(typ.NewArray(typ.NewTypeParam("T", nil))) {
		t.Error("expected true for array with type param")
	}
}
