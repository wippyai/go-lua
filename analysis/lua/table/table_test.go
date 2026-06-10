package table

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestNormalizeKeyRemovesNilAlternatives(t *testing.T) {
	if got := NormalizeKey(typ.NewOptional(typ.String)); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("NormalizeKey(optional string) = %v, want string", got)
	}

	got := NormalizeKey(typ.NewUnion(typ.String, typ.Boolean, typ.Nil))
	want := typ.NewUnion(typ.String, typ.Boolean)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("NormalizeKey(string|boolean|nil) = %v, want %v", got, want)
	}

	if got := NormalizeKey(typ.Nil); got != typ.Never {
		t.Fatalf("NormalizeKey(nil) = %v, want never", got)
	}
}

func TestNormalizeKeyPreservesAliasToOptionalPayload(t *testing.T) {
	maybeKey := typ.NewAlias("MaybeKey", typ.NewOptional(typ.String))
	got := NormalizeKey(maybeKey)
	alias, ok := got.(*typ.Alias)
	if !ok {
		t.Fatalf("NormalizeKey(alias optional) = %T, want alias", got)
	}
	if alias.Name != "MaybeKey" {
		t.Fatalf("alias name = %q, want MaybeKey", alias.Name)
	}
	if !typ.TypeEquals(alias.Target, typ.String) {
		t.Fatalf("alias target = %v, want string", alias.Target)
	}
}

func TestConstructorsNormalizeKeys(t *testing.T) {
	m := NewMap(typ.NewOptional(typ.String), typ.Number)
	if !typ.TypeEquals(m.Key, typ.String) {
		t.Fatalf("map key = %v, want string", m.Key)
	}

	ro := NewReadonlyMap(typ.NewUnion(typ.String, typ.Nil), typ.Number)
	if !typ.TypeEquals(ro.Key, typ.String) {
		t.Fatalf("readonly map key = %v, want string", ro.Key)
	}
}

func TestRebuildRecordNormalizesMapKey(t *testing.T) {
	rec := RebuildRecord(typ.RecordParts{
		MapKey:   typ.NewOptional(typ.String),
		MapValue: typ.Number,
	})
	if !typ.TypeEquals(rec.MapKey, typ.String) {
		t.Fatalf("record map key = %v, want string", rec.MapKey)
	}
}

func TestSplitNilableField(t *testing.T) {
	t.Run("optional", func(t *testing.T) {
		inner, optional := SplitNilableField(typ.NewOptional(typ.String))
		if !optional {
			t.Fatal("expected optional")
		}
		if !typ.TypeEquals(inner, typ.String) {
			t.Fatalf("inner = %v, want string", inner)
		}
	})

	t.Run("union with nil", func(t *testing.T) {
		inner, optional := SplitNilableField(typ.NewUnion(typ.String, typ.Boolean, typ.Nil))
		if !optional {
			t.Fatal("expected optional")
		}
		want := typ.NewUnion(typ.String, typ.Boolean)
		if !typ.TypeEquals(inner, want) {
			t.Fatalf("inner = %v, want %v", inner, want)
		}
	})

	t.Run("alias to optional", func(t *testing.T) {
		maybeString := typ.NewAlias("MaybeString", typ.NewOptional(typ.String))
		inner, optional := SplitNilableField(maybeString)
		if !optional {
			t.Fatal("expected optional")
		}
		if !typ.TypeEquals(inner, typ.String) {
			t.Fatalf("inner = %v, want string", inner)
		}
		if _, ok := inner.(*typ.Alias); ok {
			t.Fatalf("inner = %v, want structural payload", inner)
		}
	})

	t.Run("non optional alias preserved", func(t *testing.T) {
		name := typ.NewAlias("Name", typ.String)
		inner, optional := SplitNilableField(name)
		if optional {
			t.Fatal("did not expect optional")
		}
		if inner != name {
			t.Fatalf("inner = %v, want original alias", inner)
		}
	})

	t.Run("nil", func(t *testing.T) {
		inner, optional := SplitNilableField(typ.Nil)
		if !optional {
			t.Fatal("expected optional")
		}
		if inner != typ.Never {
			t.Fatalf("inner = %v, want never", inner)
		}
	})
}
