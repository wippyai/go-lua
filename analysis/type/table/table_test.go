package table

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestNormalizeKeyRemovesNilAlternatives(t *testing.T) {
	if got := NormalizeKey(typeexpr.Optional(typ.String)); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("NormalizeKey(optional string) = %v, want string", got)
	}

	got := NormalizeKey(typeexpr.Union(typ.String, typ.Boolean, typ.Nil))
	want := typeexpr.Union(typ.String, typ.Boolean)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("NormalizeKey(string|boolean|nil) = %v, want %v", got, want)
	}

	if got := NormalizeKey(typ.Nil); got != typ.Never {
		t.Fatalf("NormalizeKey(nil) = %v, want never", got)
	}
}

func TestNormalizeKeyPreservesAliasToOptionalPayload(t *testing.T) {
	maybeKey := typ.NewAlias("MaybeKey", typeexpr.Optional(typ.String))
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

func TestNormalizeKeyPreservesAliasToOptionalPayloadInsideUnion(t *testing.T) {
	maybeKey := typ.NewAlias("MaybeKey", typeexpr.Optional(typ.String))
	got := NormalizeKey(typeexpr.Union(maybeKey, typ.Boolean))
	union, ok := got.(*typ.Union)
	if !ok {
		t.Fatalf("NormalizeKey(alias optional | boolean) = %T, want union", got)
	}
	if len(union.Members) != 2 {
		t.Fatalf("normalized key member count = %d, want 2", len(union.Members))
	}

	var sawAlias, sawBoolean bool
	for _, member := range union.Members {
		switch v := member.(type) {
		case *typ.Alias:
			if v.Name != "MaybeKey" || !typ.TypeEquals(v.Target, typ.String) {
				t.Fatalf("alias member = %v, want MaybeKey -> string", v)
			}
			sawAlias = true
		default:
			if member == typ.Boolean {
				sawBoolean = true
			}
			if member.Kind() == kind.Nil || member.Kind() == kind.Optional {
				t.Fatalf("normalized key member kind = %s, want non-nil key payload", member.Kind())
			}
		}
	}
	if !sawAlias || !sawBoolean {
		t.Fatalf("normalized key members saw alias=%v boolean=%v, want both", sawAlias, sawBoolean)
	}
}

func TestNormalizeKeyPolicyOutputShape(t *testing.T) {
	got := NormalizeKey(typeexpr.Union(typ.String, typ.Boolean, typ.Nil))
	union, ok := got.(*typ.Union)
	if !ok {
		t.Fatalf("NormalizeKey(string|boolean|nil) = %T, want union node", got)
	}
	if len(union.Members) != 2 {
		t.Fatalf("normalized key member count = %d, want 2", len(union.Members))
	}
	seen := map[kind.Kind]int{}
	for _, member := range union.Members {
		if member == nil {
			t.Fatal("normalized key contains nil member")
		}
		if member.Kind() == kind.Nil || member.Kind() == kind.Optional {
			t.Fatalf("normalized key member kind = %s, want non-nil key payload", member.Kind())
		}
		seen[member.Kind()]++
	}
	if seen[kind.String] != 1 || seen[kind.Boolean] != 1 {
		t.Fatalf("normalized key member kinds = %#v, want one string and one boolean", seen)
	}

	alias := typ.NewAlias("Key", typ.String)
	if got := NormalizeKey(alias); got != alias {
		t.Fatalf("NormalizeKey(non-nil alias) = %p, want original alias %p", got, alias)
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
	m := NewMap(typeexpr.Optional(typ.String), typ.Number)
	if !typ.TypeEquals(m.Key, typ.String) {
		t.Fatalf("map key = %v, want string", m.Key)
	}

	ro := NewReadonlyMap(typeexpr.Union(typ.String, typ.Nil), typ.Number)
	if !typ.TypeEquals(ro.Key, typ.String) {
		t.Fatalf("readonly map key = %v, want string", ro.Key)
	}
}

func TestNewRecordMapComponentNormalizesKey(t *testing.T) {
	rec := NewRecord().
		MapComponent(typeexpr.Optional(typ.String), typ.Number).
		Build()
	if !rec.HasMapComponent() {
		t.Fatal("record should have map component")
	}
	if !typ.TypeEquals(rec.MapKey, typ.String) {
		t.Fatalf("record map key = %v, want string", rec.MapKey)
	}
}

func TestRebuildRecordNormalizesMapKey(t *testing.T) {
	rec := RebuildRecord(typ.RecordParts{
		MapKey:   typeexpr.Optional(typ.String),
		MapValue: typ.Number,
	})
	if !typ.TypeEquals(rec.MapKey, typ.String) {
		t.Fatalf("record map key = %v, want string", rec.MapKey)
	}
}

func TestOverlayRecordMembersPreservesDeclaredSiblings(t *testing.T) {
	existing := NewRecord().
		Field("processed", NewRecord().MapComponent(typ.String, typ.Boolean).Build()).
		Field("counters", NewRecord().MapComponent(typ.String, typ.Integer).Build()).
		OptField("last_id", typ.String).
		Build()
	overlay := NewRecord().Field("last_id", typ.String).Build()

	got, ok := OverlayRecordMembers(existing, overlay)
	if !ok {
		t.Fatal("OverlayRecordMembers did not accept records")
	}
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("overlay result = %T, want record", got)
	}
	if rec.GetField("processed") == nil {
		t.Fatal("overlay dropped processed sibling")
	}
	if rec.GetField("counters") == nil {
		t.Fatal("overlay dropped counters sibling")
	}
	lastID := rec.GetField("last_id")
	if lastID == nil || lastID.Optional || !typ.TypeEquals(lastID.Type, typ.String) {
		t.Fatalf("last_id = %#v, want required string", lastID)
	}
}

func TestOverlayRecordMembersRecursesIntoNestedFieldsAndStaticMembers(t *testing.T) {
	existing := NewRecord().
		Field("state", NewRecord().
			Field("metadata", NewRecord().Field("created_at", typ.Integer).Build()).
			StaticStringIndex("raw", NewRecord().Field("old", typ.Boolean).Build()).
			Build()).
		Build()
	overlay := NewRecord().
		Field("state", NewRecord().
			Field("metadata", NewRecord().Field("updated_at", typ.Integer).Build()).
			StaticStringIndex("raw", NewRecord().Field("fresh", typ.String).Build()).
			Build()).
		Build()

	got, ok := OverlayRecordMembers(existing, overlay)
	if !ok {
		t.Fatal("OverlayRecordMembers did not accept records")
	}
	rec := got.(*typ.Record)
	state := rec.GetField("state")
	if state == nil {
		t.Fatal("state field missing")
	}
	stateRec, ok := state.Type.(*typ.Record)
	if !ok {
		t.Fatalf("state type = %T, want record", state.Type)
	}
	metadata := stateRec.GetField("metadata")
	if metadata == nil {
		t.Fatal("metadata field missing")
	}
	metadataRec, ok := metadata.Type.(*typ.Record)
	if !ok {
		t.Fatalf("metadata type = %T, want record", metadata.Type)
	}
	if metadataRec.GetField("created_at") == nil || metadataRec.GetField("updated_at") == nil {
		t.Fatalf("metadata overlay = %v, want both created_at and updated_at", metadataRec)
	}
	raw := stateRec.GetStaticStringIndex("raw")
	if raw == nil {
		t.Fatal("raw static member missing")
	}
	rawRec, ok := raw.Type.(*typ.Record)
	if !ok {
		t.Fatalf("raw static member type = %T, want record", raw.Type)
	}
	if rawRec.GetField("old") == nil || rawRec.GetField("fresh") == nil {
		t.Fatalf("raw overlay = %v, want both old and fresh", rawRec)
	}
}

func TestOverlayRecordMembersKeepsDeclaredContainerForEmptyTableWitness(t *testing.T) {
	existing := NewRecord().
		Field("items", &typ.Array{Element: typ.Unknown}).
		Build()
	overlay := NewRecord().
		Field("items", NewRecord().Build()).
		Build()

	got, ok := OverlayRecordMembers(existing, overlay)
	if !ok {
		t.Fatal("OverlayRecordMembers did not accept records")
	}
	rec := got.(*typ.Record)
	items := rec.GetField("items")
	if items == nil {
		t.Fatal("items field missing")
	}
	if _, ok := items.Type.(*typ.Array); !ok {
		t.Fatalf("items type = %v (%T), want declared array preserved", items.Type, items.Type)
	}
}

func TestOverlayRecordMembersTraversesDeepRecordsExactly(t *testing.T) {
	var existing typ.Type = NewRecord().Field("preserved", typ.Number).Build()
	var overlay typ.Type = NewRecord().Field("replacement", typ.String).Build()
	for range 257 {
		existing = NewRecord().Field("next", existing).Build()
		overlay = NewRecord().Field("next", overlay).Build()
	}

	merged := overlayRecordMembers(existing.(*typ.Record), overlay.(*typ.Record))
	mergedRecord, ok := merged.(*typ.Record)
	if !ok {
		t.Fatalf("deep overlay = %T, want record", merged)
	}
	for i := 0; i < 257; i++ {
		next := mergedRecord.GetField("next")
		if next == nil {
			t.Fatalf("deep overlay lost next at level %d", i)
		}
		mergedRecord, ok = next.Type.(*typ.Record)
		if !ok {
			t.Fatalf("next at level %d = %T, want record", i, next.Type)
		}
	}
	if mergedRecord.GetField("preserved") == nil || mergedRecord.GetField("replacement") == nil {
		t.Fatalf("deep overlay did not merge both leaf siblings: %v", mergedRecord)
	}
}

func TestOverlayRecordMembersLetsStaticStringMemberUpdateNamedField(t *testing.T) {
	existing := NewRecord().
		Field("tests", NewRecord().Build()).
		Build()
	overlay := NewRecord().
		StaticStringIndex("tests", typ.NewArray(typ.String)).
		Build()

	got, ok := OverlayRecordMembers(existing, overlay)
	if !ok {
		t.Fatal("OverlayRecordMembers did not accept records")
	}
	want := NewRecord().
		Field("tests", typ.NewArray(typ.String)).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("overlay result = %v, want %v", got, want)
	}
}

func TestOverlayRecordMembersNoOpPreservesExistingNode(t *testing.T) {
	existing := NewRecord().Field("id", typ.String).Build()
	overlay := NewRecord().Field("id", typ.String).Build()

	got, ok := OverlayRecordMembers(existing, overlay)
	if !ok {
		t.Fatal("OverlayRecordMembers did not accept records")
	}
	if got != existing {
		t.Fatalf("OverlayRecordMembers no-op returned %p, want existing node %p", got, existing)
	}
}

func TestOverlayRecordMembersNoOpPreservesSameRecursiveIdentityWrapper(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return NewRecord().OptField("next", self).Build()
	})
	existing := NewRecord().Field("next", node).Build()
	overlay := NewRecord().Field("next", node).Build()

	got, ok := OverlayRecordMembers(existing, overlay)
	if !ok {
		t.Fatal("OverlayRecordMembers did not accept records")
	}
	if got != existing {
		t.Fatalf("recursive no-op overlay returned %p, want existing node %p", got, existing)
	}
}

func TestOverlayRecordMembersKeepsDistinctRecursiveFamiliesDistinct(t *testing.T) {
	left := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return NewRecord().OptField("next", self).Build()
	})
	right := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return NewRecord().OptField("next", self).Build()
	})
	existing := NewRecord().Field("next", left).Build()
	overlay := NewRecord().Field("next", right).Build()

	got, ok := OverlayRecordMembers(existing, overlay)
	if !ok {
		t.Fatal("OverlayRecordMembers did not accept records")
	}
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("overlay result = %T, want record", got)
	}
	next := rec.GetField("next")
	if next == nil {
		t.Fatal("next field missing")
	}
	if next.Type != right {
		t.Fatalf("overlay collapsed distinct recursive family: got %v, want %v", next.Type, right)
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
		OptField("error", typeexpr.Optional(typ.String)).
		AddStaticMember(typ.StaticMember{
			Kind:     typ.StaticMemberStringIndex,
			Name:     "raw",
			Type:     typeexpr.Union(typ.Number, typ.Nil),
			Optional: true,
		}).
		Build()
	assertField(t, built, "error", typ.String)
	assertStaticString(t, built, "raw", typ.Number)

	rebuilt := RebuildRecord(typ.RecordParts{
		Fields: []typ.Field{{
			Name:     "ok",
			Type:     typeexpr.Union(typ.Boolean, typ.Nil),
			Optional: true,
		}},
		StaticMembers: []typ.StaticMember{{
			Kind:     typ.StaticMemberIntIndex,
			Index:    1,
			Type:     typeexpr.Optional(typ.Integer),
			Optional: true,
		}},
	})
	assertField(t, rebuilt, "ok", typ.Boolean)
	assertStaticInt(t, rebuilt, 1, typ.Integer)
}

func TestRecordNormalizationPolicyOutputShape(t *testing.T) {
	ann := []annotation.Annotation{{Name: "tag"}}
	rec := RebuildRecord(typ.RecordParts{
		Fields: []typ.Field{
			{
				Name:     "union",
				Type:     typeexpr.Union(typ.String, typ.Boolean, typ.Nil),
				Optional: true,
			},
			{
				Name:     "alias",
				Type:     typ.NewAlias("MaybeString", typeexpr.Optional(typ.String)),
				Optional: true,
			},
			{
				Name:     "annotated",
				Type:     typ.NewAnnotated(typeexpr.Optional(typ.Integer), ann),
				Optional: true,
			},
		},
		StaticMembers: []typ.StaticMember{{
			Kind:     typ.StaticMemberStringIndex,
			Name:     "raw",
			Type:     typeexpr.Optional(typ.Number),
			Optional: true,
			Readonly: true,
		}},
		MapKey:   typ.NewAlias("MaybeKey", typeexpr.Optional(typ.String)),
		MapValue: typ.Boolean,
		Open:     true,
	})

	if !rec.Open {
		t.Fatal("record rebuild dropped open flag")
	}
	keyAlias, ok := rec.MapKey.(*typ.Alias)
	if !ok {
		t.Fatalf("record map key = %T, want alias-preserved normalized key", rec.MapKey)
	}
	if keyAlias.Name != "MaybeKey" || keyAlias.Target != typ.String {
		t.Fatalf("record map key alias = %s -> %v, want MaybeKey -> string", keyAlias.Name, keyAlias.Target)
	}
	if rec.MapValue != typ.Boolean {
		t.Fatalf("record map value = %v, want boolean singleton", rec.MapValue)
	}

	unionField := rec.GetField("union")
	if unionField == nil || !unionField.Optional {
		t.Fatalf("union field = %#v, want optional field", unionField)
	}
	unionPayload, ok := unionField.Type.(*typ.Union)
	if !ok {
		t.Fatalf("union field payload = %T, want union node", unionField.Type)
	}
	if len(unionPayload.Members) != 2 {
		t.Fatalf("union field payload member count = %d, want 2", len(unionPayload.Members))
	}
	for _, member := range unionPayload.Members {
		if member.Kind() == kind.Nil || member.Kind() == kind.Optional {
			t.Fatalf("union field payload retained nil-capable member kind %s", member.Kind())
		}
	}

	aliasField := rec.GetField("alias")
	if aliasField == nil || !aliasField.Optional {
		t.Fatalf("alias field = %#v, want optional field", aliasField)
	}
	if aliasField.Type != typ.String {
		t.Fatalf("alias field payload = %T %v, want structural string singleton", aliasField.Type, aliasField.Type)
	}
	if _, ok := aliasField.Type.(*typ.Alias); ok {
		t.Fatal("alias field payload preserved alias, want structural payload projection")
	}

	annotatedField := rec.GetField("annotated")
	if annotatedField == nil || !annotatedField.Optional {
		t.Fatalf("annotated field = %#v, want optional field", annotatedField)
	}
	annotatedPayload, ok := annotatedField.Type.(*typ.Annotated)
	if !ok {
		t.Fatalf("annotated field payload = %T, want annotated node", annotatedField.Type)
	}
	if annotatedPayload.Inner != typ.Integer {
		t.Fatalf("annotated inner = %v, want integer singleton", annotatedPayload.Inner)
	}
	if len(annotatedPayload.Annotations) != 1 || annotatedPayload.Annotations[0].Name != "tag" {
		t.Fatalf("annotations = %#v, want preserved tag annotation", annotatedPayload.Annotations)
	}

	static := rec.GetStaticStringIndex("raw")
	if static == nil || !static.Optional || !static.Readonly {
		t.Fatalf("static member = %#v, want optional readonly member", static)
	}
	if static.Type != typ.Number {
		t.Fatalf("static member payload = %T %v, want number singleton", static.Type, static.Type)
	}
}

func TestRecordNormalizationNoOpPreservesShape(t *testing.T) {
	fields := []typ.Field{
		{Name: "name", Type: typ.String},
		{Name: "status", Type: typ.Boolean, Readonly: true},
	}
	members := []typ.StaticMember{{
		Kind: typ.StaticMemberStringIndex,
		Name: "raw",
		Type: typ.Number,
	}}
	parts := typ.RecordParts{
		Fields:        fields,
		StaticMembers: members,
		MapKey:        typ.String,
		MapValue:      typ.Boolean,
	}

	got := recordPartsWithTableNormalization(parts)
	if len(got.Fields) != len(fields) {
		t.Fatalf("field count = %d, want %d", len(got.Fields), len(fields))
	}
	for i := range fields {
		if got.Fields[i] != fields[i] {
			t.Fatalf("field[%d] = %#v, want %#v", i, got.Fields[i], fields[i])
		}
	}
	if len(got.StaticMembers) != len(members) {
		t.Fatalf("static member count = %d, want %d", len(got.StaticMembers), len(members))
	}
	for i := range members {
		if got.StaticMembers[i] != members[i] {
			t.Fatalf("static member[%d] = %#v, want %#v", i, got.StaticMembers[i], members[i])
		}
	}
	if got.MapKey != typ.String || got.MapValue != typ.Boolean {
		t.Fatalf("no-op map component = [%v]: %v, want original string:boolean", got.MapKey, got.MapValue)
	}
}

func TestRecordNormalizationDoesNotSplitRequiredNilablePayloads(t *testing.T) {
	fieldType := typeexpr.Optional(typ.String)
	memberType := typeexpr.Optional(typ.Number)
	rec := RebuildRecord(typ.RecordParts{
		Fields: []typ.Field{{
			Name: "present_field",
			Type: fieldType,
		}},
		StaticMembers: []typ.StaticMember{{
			Kind: typ.StaticMemberStringIndex,
			Name: "present_static",
			Type: memberType,
		}},
	})

	field := rec.GetField("present_field")
	if field == nil || field.Optional {
		t.Fatalf("field = %#v, want required field", field)
	}
	if !typ.TypeEquals(field.Type, fieldType) {
		t.Fatalf("field type = %v, want original %v", field.Type, fieldType)
	}

	member := rec.GetStaticStringIndex("present_static")
	if member == nil || member.Optional {
		t.Fatalf("static member = %#v, want required member", member)
	}
	if !typ.TypeEquals(member.Type, memberType) {
		t.Fatalf("static member type = %v, want original %v", member.Type, memberType)
	}
}

func TestSplitNilableFieldType(t *testing.T) {
	t.Run("optional", func(t *testing.T) {
		inner, optional := splitNilableFieldType(typeexpr.Optional(typ.String))
		if !optional {
			t.Fatal("expected optional")
		}
		if !typ.TypeEquals(inner, typ.String) {
			t.Fatalf("inner = %v, want string", inner)
		}
	})

	t.Run("union with nil", func(t *testing.T) {
		inner, optional := splitNilableFieldType(typeexpr.Union(typ.String, typ.Boolean, typ.Nil))
		if !optional {
			t.Fatal("expected optional")
		}
		want := typeexpr.Union(typ.String, typ.Boolean)
		if !typ.TypeEquals(inner, want) {
			t.Fatalf("inner = %v, want %v", inner, want)
		}
	})

	t.Run("alias to optional", func(t *testing.T) {
		maybeString := typ.NewAlias("MaybeString", typeexpr.Optional(typ.String))
		inner, optional := splitNilableFieldType(maybeString)
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
		inner, optional := splitNilableFieldType(name)
		if optional {
			t.Fatal("did not expect optional")
		}
		if inner != name {
			t.Fatalf("inner = %v, want original alias", inner)
		}
	})

	t.Run("nil", func(t *testing.T) {
		inner, optional := splitNilableFieldType(typ.Nil)
		if !optional {
			t.Fatal("expected optional")
		}
		if inner != typ.Never {
			t.Fatalf("inner = %v, want never", inner)
		}
	})

	t.Run("annotated optional", func(t *testing.T) {
		ann := []annotation.Annotation{{Name: "tag"}}
		inner, optional := splitNilableFieldType(typ.NewAnnotated(typeexpr.Optional(typ.String), ann))
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

func TestPresentReadonlyEntryValue(t *testing.T) {
	t.Run("optional", func(t *testing.T) {
		got := PresentReadonlyEntryValue(typeexpr.Optional(typ.String))
		if !typ.TypeEquals(got, typ.String) {
			t.Fatalf("present readonly entry = %v, want string", got)
		}
	})

	t.Run("union with nil", func(t *testing.T) {
		got := PresentReadonlyEntryValue(typeexpr.Union(typ.String, typ.Boolean, typ.Nil))
		want := typeexpr.Union(typ.String, typ.Boolean)
		if !typ.TypeEquals(got, want) {
			t.Fatalf("present readonly entry = %v, want %v", got, want)
		}
	})

	t.Run("alias to optional", func(t *testing.T) {
		maybeString := typ.NewAlias("MaybeString", typeexpr.Optional(typ.String))
		got := PresentReadonlyEntryValue(maybeString)
		if !typ.TypeEquals(got, typ.String) {
			t.Fatalf("present readonly entry = %v, want string", got)
		}
		if _, ok := got.(*typ.Alias); ok {
			t.Fatalf("present readonly entry = %v, want structural payload", got)
		}
	})

	t.Run("non nilable unchanged", func(t *testing.T) {
		got := PresentReadonlyEntryValue(typ.String)
		if got != typ.String {
			t.Fatalf("present readonly entry = %v, want original string type", got)
		}
	})

	t.Run("absent type stays absent", func(t *testing.T) {
		if got := PresentReadonlyEntryValue(nil); got != nil {
			t.Fatalf("present readonly entry = %v, want nil", got)
		}
	})
}

func TestEntryValueShape(t *testing.T) {
	payload, optional := EntryValueShape(typeexpr.Optional(typ.String))
	if !optional {
		t.Fatal("optional string did not produce absent-capable entry shape")
	}
	if !typ.TypeEquals(payload, typ.String) {
		t.Fatalf("entry payload = %v, want string", payload)
	}

	payload, optional = EntryValueShape(typ.Number)
	if optional {
		t.Fatal("number produced absent-capable entry shape")
	}
	if payload != typ.Number {
		t.Fatalf("entry payload = %v, want original number", payload)
	}
}

func TestBuiltinTopMarker(t *testing.T) {
	tableTop := typ.NewInterface("table", nil)
	nonTableIface := typ.NewInterface("Reader", nil)
	aliasedTable := typ.NewAlias("TTable", tableTop)
	annotatedTable := typ.NewAnnotated(tableTop, []annotation.Annotation{{Name: "source"}})

	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"builtin table marker", tableTop, true},
		{"aliased builtin table marker", aliasedTable, true},
		{"annotated builtin table marker", annotatedTable, true},
		{"non-table interface", nonTableIface, false},
		{"interface with method", typ.NewInterface("table", []typ.Method{{Name: "x", Type: typ.Func().Build()}}), false},
		{"string", typ.String, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBuiltinTopMarker(tt.t); got != tt.want {
				t.Errorf("IsBuiltinTopMarker() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLike(t *testing.T) {
	rec := NewRecord().Build()
	recursiveTable := typ.NewRecursive("T", func(typ.Type) typ.Type {
		return rec
	})

	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"record", rec, true},
		{"map", NewMap(typ.String, typ.Number), true},
		{"readonly map", NewReadonlyMap(typ.String, typ.Number), true},
		{"array", typ.NewArray(typ.String), true},
		{"tuple", typ.NewTuple(typ.String), true},
		{"interface", typ.NewInterface("Reader", nil), true},
		{"intersection", typeexpr.Intersection(rec, typ.NewInterface("Reader", nil)), true},
		{"alias to table", typ.NewAlias("Alias", rec), true},
		{"recursive table", recursiveTable, true},
		{"builtin top marker", typ.NewInterface("table", nil), true},
		{"string", typ.String, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLike(tt.t); got != tt.want {
				t.Errorf("IsLike() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplitNilableFieldPreservesRecursiveUnionMemberHashes(t *testing.T) {
	recA := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return NewRecord().Field("next", typeexpr.Optional(self)).Build()
	})
	recB := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return NewRecord().Field("next", typeexpr.Optional(self)).Field("name", typ.String).Build()
	})
	u, ok := typeexpr.Union(typ.Nil, recA, recB).(*typ.Union)
	if !ok {
		t.Fatalf("expected union")
	}

	got, optional := splitNilableFieldType(u)
	if !optional {
		t.Fatalf("expected optional")
	}
	want := typeexpr.Union(recA, recB)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("inner = %v, want %v", got, want)
	}
	if got.Hash() != want.Hash() {
		t.Fatalf("hash = %d, want %d", got.Hash(), want.Hash())
	}
}
