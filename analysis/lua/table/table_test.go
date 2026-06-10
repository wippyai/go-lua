package table

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
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

func TestProjectNilLeavesAbsentTypeAlone(t *testing.T) {
	got, nilable := withoutNil(nil, nilProjectionStructural)
	if nilable {
		t.Fatal("did not expect nilable")
	}
	if got != nil {
		t.Fatalf("project nil absent type = %v, want nil", got)
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

func TestNewRecordMapComponentNormalizesKey(t *testing.T) {
	rec := NewRecord().
		MapComponent(typ.NewOptional(typ.String), typ.Number).
		Build()
	if !rec.HasMapComponent() {
		t.Fatal("record should have map component")
	}
	if !typ.TypeEquals(rec.MapKey, typ.String) {
		t.Fatalf("record map key = %v, want string", rec.MapKey)
	}
}

func TestFreshConstructorsReturnFreshNodes(t *testing.T) {
	arr := NewFreshArray()
	if arr == nil || !arr.Fresh {
		t.Fatalf("NewFreshArray() = %#v, want fresh array", arr)
	}

	rec := NewFreshEmptyRecord()
	if rec == nil || !rec.Fresh {
		t.Fatalf("NewFreshEmptyRecord() = %#v, want fresh record", rec)
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

func TestRecordConstructionSplitsNilableOptionalPayloads(t *testing.T) {
	assertField := func(t *testing.T, rec *typ.Record, name string, want typ.Type) {
		t.Helper()
		field := rec.GetField(name)
		if field == nil || !field.Optional {
			t.Fatalf("field %q = %#v, want optional field", name, field)
		}
		if !typ.TypeEquals(field.Type, want) {
			t.Fatalf("field %q type = %v, want %v", name, field.Type, want)
		}
	}
	assertStaticString := func(t *testing.T, rec *typ.Record, name string, want typ.Type) {
		t.Helper()
		member := rec.GetStaticStringIndex(name)
		if member == nil || !member.Optional {
			t.Fatalf("static %q = %#v, want optional member", name, member)
		}
		if !typ.TypeEquals(member.Type, want) {
			t.Fatalf("static %q type = %v, want %v", name, member.Type, want)
		}
	}
	assertStaticInt := func(t *testing.T, rec *typ.Record, index int64, want typ.Type) {
		t.Helper()
		member := rec.GetStaticIntIndex(index)
		if member == nil || !member.Optional {
			t.Fatalf("static %d = %#v, want optional member", index, member)
		}
		if !typ.TypeEquals(member.Type, want) {
			t.Fatalf("static %d type = %v, want %v", index, member.Type, want)
		}
	}

	built := NewRecord().
		OptField("error", typ.NewOptional(typ.String)).
		AddStaticMember(typ.StaticMember{
			Kind:     typ.StaticMemberStringIndex,
			Name:     "raw",
			Type:     typ.NewUnion(typ.Number, typ.Nil),
			Optional: true,
		}).
		Build()
	assertField(t, built, "error", typ.String)
	assertStaticString(t, built, "raw", typ.Number)

	rebuilt := RebuildRecord(typ.RecordParts{
		Fields: []typ.Field{{
			Name:     "ok",
			Type:     typ.NewUnion(typ.Boolean, typ.Nil),
			Optional: true,
		}},
		StaticMembers: []typ.StaticMember{{
			Kind:     typ.StaticMemberIntIndex,
			Index:    1,
			Type:     typ.NewOptional(typ.Integer),
			Optional: true,
		}},
	})
	assertField(t, rebuilt, "ok", typ.Boolean)
	assertStaticInt(t, rebuilt, 1, typ.Integer)
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

	t.Run("annotated optional", func(t *testing.T) {
		ann := []annotation.Annotation{{Name: "tag"}}
		inner, optional := SplitNilableField(typ.NewAnnotated(typ.NewOptional(typ.String), ann))
		if !optional {
			t.Fatal("expected optional")
		}
		annotated, ok := inner.(*typ.Annotated)
		if !ok {
			t.Fatalf("inner = %T, want annotated", inner)
		}
		if !typ.TypeEquals(annotated.Inner, typ.String) {
			t.Fatalf("annotated inner = %v, want string", annotated.Inner)
		}
	})
}

func TestSplitNilableFieldPreservesRecursiveUnionMemberHashes(t *testing.T) {
	recA := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().Field("next", typ.NewOptional(self)).Build()
	})
	recB := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().Field("next", typ.NewOptional(self)).Field("name", typ.String).Build()
	})
	u, ok := typ.NewUnion(typ.Nil, recA, recB).(*typ.Union)
	if !ok {
		t.Fatalf("expected union")
	}

	got, optional := SplitNilableField(u)
	if !optional {
		t.Fatalf("expected optional")
	}
	want := typ.NewUnion(recA, recB)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("inner = %v, want %v", got, want)
	}
	if got.Hash() != want.Hash() {
		t.Fatalf("hash = %d, want %d", got.Hash(), want.Hash())
	}
}
