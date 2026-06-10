package relation

import (
	"fmt"
	"testing"

	luatable "github.com/wippyai/go-lua/analysis/lua/table"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

func TestJoinReturnSlot_PreservesUnknownOverNil(t *testing.T) {
	if got := JoinReturnSlot(Unknown, Nil); !TypeEquals(got, Unknown) {
		t.Fatalf("JoinReturnSlot(unknown, nil) = %v, want unknown", got)
	}
	if got := JoinReturnSlot(Nil, Unknown); !TypeEquals(got, Unknown) {
		t.Fatalf("JoinReturnSlot(nil, unknown) = %v, want unknown", got)
	}
}

func TestJoinReturnSlot_PreservesUnknownOverConcrete(t *testing.T) {
	rec := NewRecord().Field("value", String).Build()
	if got := JoinReturnSlot(Unknown, rec); !TypeEquals(got, Unknown) {
		t.Fatalf("JoinReturnSlot(unknown, record) = %v, want unknown", got)
	}
	if got := JoinReturnSlot(rec, Unknown); !TypeEquals(got, Unknown) {
		t.Fatalf("JoinReturnSlot(record, unknown) = %v, want unknown", got)
	}
}

func TestIsClosedUnionAnnotationRecognizesInstantiatedGenericUnion(t *testing.T) {
	tp := NewTypeParam("T", nil)
	result := NewGeneric("Result", []*TypeParam{tp}, NewUnion(
		NewRecord().Field("ok", LiteralBool(true)).Field("value", tp).Build(),
		NewRecord().Field("ok", LiteralBool(false)).Field("error", String).Build(),
	))

	if !IsClosedUnionAnnotation(Instantiate(result, NewRecord().Field("id", String).Build())) {
		t.Fatal("IsClosedUnionAnnotation(Result<User>) = false, want true")
	}
}

// TestJoinReturnSlot_PrefersConcreteScalarOverUnknown locks the evidence-lattice
// semantics for a converged slot: a bare unknown is unresolved evidence ("no
// value yet"), not the dynamic top, so its least upper bound with a concrete
// scalar is the scalar. Widening the scalar back to unknown is a precision drop;
// when one record observation carries full_path: string and another carries the
// not-yet-solved full_path: unknown, joining the field must keep string so a
// later string concatenation does not falsely error.
func TestJoinReturnSlot_PrefersConcreteScalarOverUnknown(t *testing.T) {
	scalars := []Type{String, Number, Integer, Boolean, LiteralString("k")}
	for _, scalar := range scalars {
		if got := JoinReturnSlot(scalar, Unknown); !TypeEquals(got, scalar) {
			t.Fatalf("JoinReturnSlot(%v, unknown) = %v, want %v", scalar, got, scalar)
		}
		if got := JoinReturnSlot(Unknown, scalar); !TypeEquals(got, scalar) {
			t.Fatalf("JoinReturnSlot(unknown, %v) = %v, want %v", scalar, got, scalar)
		}
	}
}

func TestJoinCompatibleRecordsPreservesStaticBracketMembers(t *testing.T) {
	left := NewRecord().
		Field("name", String).
		StaticStringIndex("raw-key", Number).
		Build()
	right := NewRecord().
		Field("name", String).
		Build()

	got, ok := JoinCompatibleRecords(left, right)
	if !ok {
		t.Fatal("JoinCompatibleRecords ok=false")
	}
	rec, ok := got.(*Record)
	if !ok {
		t.Fatalf("JoinCompatibleRecords = %T %[1]v, want record", got)
	}
	field := rec.GetField("name")
	if field == nil || field.Optional || !TypeEquals(field.Type, String) {
		t.Fatalf("dot field name = %#v, want required string", field)
	}
	member := rec.GetStaticStringIndex("raw-key")
	if member == nil {
		t.Fatalf("static member [\"raw-key\"] missing from %v", rec)
	}
	if !member.Optional {
		t.Fatalf("static member [\"raw-key\"] = %#v, want optional after missing branch", member)
	}
	if !TypeEquals(member.Type, Number) {
		t.Fatalf("static member [\"raw-key\"] type = %v, want number", member.Type)
	}
}

func TestJoinCompatibleRecordsPreservesSharedMetatable(t *testing.T) {
	meta := NewRecord().
		Field("__index", NewRecord().Field("run", Func().Build()).Build()).
		Build()
	left := NewRecord().
		Field("id", String).
		Metatable(meta).
		Build()
	right := NewRecord().
		Field("id", String).
		Field("name", String).
		Metatable(meta).
		Build()

	got, ok := JoinCompatibleRecords(left, right)
	if !ok {
		t.Fatal("JoinCompatibleRecords ok=false")
	}
	rec, ok := got.(*Record)
	if !ok {
		t.Fatalf("JoinCompatibleRecords = %T %[1]v, want record", got)
	}
	if rec.Metatable == nil || !TypeEquals(rec.Metatable, meta) {
		t.Fatalf("merged metatable = %v, want %v", rec.Metatable, meta)
	}
	name := rec.GetField("name")
	if name == nil || !name.Optional || !TypeEquals(name.Type, String) {
		t.Fatalf("merged name field = %#v, want optional string", name)
	}
}

func TestJoinReturnSlotDoesNotDropMetatableWhenPeerHasNone(t *testing.T) {
	meta := NewRecord().
		Field("__index", NewRecord().Field("run", Func().Build()).Build()).
		Build()
	withMeta := NewRecord().
		Field("id", String).
		Metatable(meta).
		Build()
	withoutMeta := NewRecord().
		Field("id", String).
		Field("name", String).
		Build()

	got := JoinReturnSlot(withMeta, withoutMeta)
	union, ok := got.(*Union)
	if !ok {
		t.Fatalf("JoinReturnSlot(with metatable, without metatable) = %T %[1]v, want union alternatives", got)
	}
	foundMeta := false
	foundPlain := false
	for _, member := range union.Members {
		rec, ok := member.(*Record)
		if !ok {
			continue
		}
		if rec.Metatable != nil {
			foundMeta = true
		} else {
			foundPlain = true
		}
	}
	if !foundMeta || !foundPlain {
		t.Fatalf("joined union members = %v, want metatable and plain alternatives", union.Members)
	}
}

// TestJoinUnionFieldSlot_KeepsScalarFieldOverUnknownPeer proves the field-slot
// join keeps the precise scalar field when one record observation has not yet
// resolved it.
func TestJoinUnionFieldSlot_KeepsScalarFieldOverUnknownPeer(t *testing.T) {
	if got := JoinUnionFieldSlot(String, Unknown, false); !TypeEquals(got, String) {
		t.Fatalf("slot join(string, unknown) = %v, want string", got)
	}
	if got := JoinUnionFieldSlot(Unknown, String, false); !TypeEquals(got, String) {
		t.Fatalf("slot join(unknown, string) = %v, want string", got)
	}
}

func TestClosedRecordConflictFastPath_NoRequiredDiscriminants(t *testing.T) {
	records := []*Record{
		NewRecord().
			Field("from", Func().Param("self", Self).Returns(Self).Build()).
			Field("where", Func().Param("self", Self).Param("clause", String).Returns(Self).Build()).
			Build(),
		NewRecord().
			Field("from", Func().Param("self", Self).Returns(Self).Build()).
			Field("where", Func().Param("self", Self).Param("clause", String).Returns(Self).Build()).
			Field("limit", Number).
			Build(),
	}

	if closedRecordSetHasConflictingRequiredLiteralField(records) {
		t.Fatal("records without required literal discriminants reported a conflict")
	}
}

func TestCoalesceProductUnion_OpenRecordsWithoutDiscriminantsDoesNotPanic(t *testing.T) {
	left := NewRecord().
		SetOpen(true).
		Field("content", String).
		Build()
	right := NewRecord().
		SetOpen(true).
		Field("content", NewArray(String)).
		Build()

	_ = CoalesceProductUnion(NewUnion(left, right))
}

func TestJoinReturnSlot_PreservesAnyOverNil(t *testing.T) {
	if got := JoinReturnSlot(Any, Nil); !TypeEquals(got, Any) {
		t.Fatalf("JoinReturnSlot(any, nil) = %v, want any", got)
	}
	if got := JoinReturnSlot(Nil, Any); !TypeEquals(got, Any) {
		t.Fatalf("JoinReturnSlot(nil, any) = %v, want any", got)
	}
}

func TestJoinReturnSlot_PrefersArrayOverEmptyRecord(t *testing.T) {
	empty := NewRecord().Build()
	arr := NewArray(String)

	if got := JoinReturnSlot(empty, arr); !TypeEquals(got, arr) {
		t.Fatalf("JoinReturnSlot({}, string[]) = %v, want string[]", got)
	}
	if got := JoinReturnSlot(arr, empty); !TypeEquals(got, arr) {
		t.Fatalf("JoinReturnSlot(string[], {}) = %v, want string[]", got)
	}
}

func TestJoinBranchOutcome_PreservesUnknownWithNil(t *testing.T) {
	got := JoinBranchOutcome(Unknown, Nil)
	opt, ok := got.(*Optional)
	if !ok || !TypeEquals(opt.Inner, Unknown) {
		t.Fatalf("JoinBranchOutcome(unknown, nil) = %v, want unknown?", got)
	}

	got = JoinBranchOutcome(Nil, Unknown)
	opt, ok = got.(*Optional)
	if !ok || !TypeEquals(opt.Inner, Unknown) {
		t.Fatalf("JoinBranchOutcome(nil, unknown) = %v, want unknown?", got)
	}
}

func TestJoinBranchOutcome_PreservesUnknownOverConcrete(t *testing.T) {
	if got := JoinBranchOutcome(Unknown, String); !TypeEquals(got, Unknown) {
		t.Fatalf("JoinBranchOutcome(unknown, string) = %v, want unknown", got)
	}
	if got := JoinBranchOutcome(String, Unknown); !TypeEquals(got, Unknown) {
		t.Fatalf("JoinBranchOutcome(string, unknown) = %v, want unknown", got)
	}
}

func TestJoinBranchOutcome_PreservesSoftRuntimeAlternative(t *testing.T) {
	left := NewOptional(NewArray(Any))
	right := NewArray(Number)
	got := JoinBranchOutcome(left, right)
	want := NewUnion(left, right)
	if !TypeEquals(got, want) {
		t.Fatalf("JoinBranchOutcome(%v, %v) = %v, want %v", left, right, got, want)
	}
}

func TestJoinBranchOutcome_DoesNotCollapseSoftToNil(t *testing.T) {
	got := JoinBranchOutcome(Any, Nil)
	if TypeEquals(got, Nil) {
		t.Fatalf("JoinBranchOutcome(any, nil) collapsed to nil: %v", got)
	}
}

func TestJoinReturnSlot_MergesRecordFieldsAsOptional(t *testing.T) {
	base := NewRecord().
		Field("status_code", Number).
		Field("message", String).
		Build()
	withDetails := NewRecord().
		Field("status_code", Number).
		Field("message", String).
		Field("code", String).
		Field("type", String).
		Build()

	got := JoinReturnSlot(base, withDetails)
	rec, ok := got.(*Record)
	if !ok {
		t.Fatalf("JoinReturnSlot(record, record) = %T, want *Record", got)
	}
	fields := map[string]Field{}
	for _, f := range rec.Fields {
		fields[f.Name] = f
	}

	if !TypeEquals(fields["status_code"].Type, Number) || fields["status_code"].Optional {
		t.Fatalf("status_code mismatch: %#v", fields["status_code"])
	}
	if !TypeEquals(fields["message"].Type, String) || fields["message"].Optional {
		t.Fatalf("message mismatch: %#v", fields["message"])
	}
	if !fields["code"].Optional || !TypeEquals(fields["code"].Type, String) {
		t.Fatalf("code should be optional string, got %#v", fields["code"])
	}
	if !fields["type"].Optional || !TypeEquals(fields["type"].Type, String) {
		t.Fatalf("type should be optional string, got %#v", fields["type"])
	}
}

func TestCoalesceCompatibleRecords_BatchMergesOpenMapRecordFamily(t *testing.T) {
	members := make([]Type, 0, 256)
	for i := 0; i < cap(members); i++ {
		members = append(members, NewRecord().
			SetOpen(true).
			Field("name", LiteralString("suite")).
			Field("index", LiteralInt(int64(i))).
			MapComponent(String, NewArray(Any)).
			Build())
	}

	got := CoalesceCompatibleRecords(members)
	if len(got) != 1 {
		t.Fatalf("CoalesceCompatibleRecords(open map family) len = %d, want 1", len(got))
	}
	rec, ok := got[0].(*Record)
	if !ok {
		t.Fatalf("coalesced family = %T %[1]v, want record", got[0])
	}
	if !rec.Open || !rec.HasMapComponent() {
		t.Fatalf("coalesced record lost open/map shape: %v", rec)
	}
	index := rec.GetField("index")
	if index == nil || !TypeEquals(index.Type, Integer) {
		t.Fatalf("coalesced index field = %v, want integer", index)
	}
}

func TestJoinReturnSlot_CoalescesCompatibleRecursiveRecordFields(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Build()
	})
	base := NewRecord().
		Field("node", node).
		Field("status", String).
		Build()
	withDetails := NewRecord().
		Field("node", node).
		Field("status", String).
		Field("details", String).
		Build()

	got := JoinReturnSlot(base, withDetails)
	rec, ok := got.(*Record)
	if !ok {
		t.Fatalf("recursive record-field join = %T %[1]v, want record", got)
	}
	details := rec.GetField("details")
	if details == nil || !details.Optional || !TypeEquals(details.Type, String) {
		t.Fatalf("details field = %v, want optional string", details)
	}
}

func TestJoinReturnSlot_CoalescesCompatibleRecursiveUnionMembers(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Build()
	})
	left := NewUnion(
		NewRecord().Field("node", node).Field("status", String).Build(),
		NewRecord().Field("node", node).Field("status", String).Field("left", String).Build(),
	)
	right := NewUnion(
		NewRecord().Field("node", node).Field("status", String).Build(),
		NewRecord().Field("node", node).Field("status", String).Field("right", String).Build(),
	)

	got := JoinReturnSlot(left, right)
	rec, ok := got.(*Record)
	if !ok {
		t.Fatalf("recursive union join = %T %[1]v, want record", got)
	}
	leftField := rec.GetField("left")
	rightField := rec.GetField("right")
	if leftField == nil || !leftField.Optional || rightField == nil || !rightField.Optional {
		t.Fatalf("recursive union fields left=%v right=%v, want optional fields", leftField, rightField)
	}
}

func TestJoinReturnSlot_RecursiveFamilyFoldUsesIdentityJoinKeys(t *testing.T) {
	left := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Field("metadata", NewRecord().
				Field("owner", self).
				Field("tags", NewMap(String, NewArray(self))).
				Build()).
			Build()
	})
	right := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Field("metadata", NewRecord().
				Field("owner", self).
				Field("tags", NewMap(String, NewArray(self))).
				Field("full_path", String).
				Build()).
			Build()
	})

	got := JoinReturnSlot(left, right)
	rec, ok := got.(*Recursive)
	if !ok {
		t.Fatalf("recursive family join = %T %[1]v, want recursive product", got)
	}
	body, ok := rec.Body.(*Record)
	if !ok {
		t.Fatalf("recursive family body = %T %[1]v, want record", rec.Body)
	}
	metadata := body.GetField("metadata")
	if metadata == nil {
		t.Fatalf("recursive family body missing metadata field: %v", body)
	}
	metadataRecord, ok := metadata.Type.(*Record)
	if !ok {
		t.Fatalf("metadata field = %T %[1]v, want record", metadata.Type)
	}
	fullPath := metadataRecord.GetField("full_path")
	if fullPath == nil || !fullPath.Optional || !TypeEquals(fullPath.Type, String) {
		t.Fatalf("metadata.full_path = %v, want optional string", fullPath)
	}
}

func TestJoinReturnSlot_RecursiveFamilyFoldUsesProductFamilyRelation(t *testing.T) {
	left := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Field("hooks", recursiveSuiteVariantUnion("left", 48)).
			Build()
	})
	right := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Field("hooks", recursiveSuiteVariantUnion("right", 48)).
			Field("full_path", String).
			Build()
	})

	got := JoinReturnSlot(left, right)
	rec, ok := got.(*Recursive)
	if !ok {
		t.Fatalf("recursive family join = %T %[1]v, want recursive product", got)
	}
	body, ok := rec.Body.(*Record)
	if !ok {
		t.Fatalf("recursive family body = %T %[1]v, want record", rec.Body)
	}
	if hooks := body.GetField("hooks"); hooks == nil {
		t.Fatalf("recursive family body missing hooks field: %v", body)
	}
	fullPath := body.GetField("full_path")
	if fullPath == nil || !fullPath.Optional || !TypeEquals(fullPath.Type, String) {
		t.Fatalf("full_path = %v, want optional string", fullPath)
	}
}

func recursiveSuiteVariantUnion(prefix string, n int) Type {
	members := make([]Type, 0, n)
	for i := 0; i < n; i++ {
		label := fmt.Sprintf("%s_%02d", prefix, i)
		members = append(members, NewRecursive("Suite", func(self Type) Type {
			return NewRecord().
				Field("name", String).
				Field("children", NewArray(self)).
				Field(label, String).
				Build()
		}))
	}
	return NewUnion(members...)
}

func TestOptionalFieldBuilderKeepsProductCoalescingAtPolicyBoundary(t *testing.T) {
	left := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("next", NewOptional(self)).
			Build()
	})
	right := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("next", NewOptional(self)).
			Field("name", String).
			Build()
	})

	rec := NewRecord().
		OptField("node", NewUnion(Nil, left, right)).
		Build()
	field := rec.GetField("node")
	if field == nil {
		t.Fatal("optional field was not built")
	}
	union, ok := field.Type.(*Union)
	if !ok || len(union.Members) != 2 {
		t.Fatalf("optional field construction coalesced product union: %T %[1]v", field.Type)
	}

	coalesced := CoalesceProductUnion(field.Type)
	if _, ok := coalesced.(*Recursive); !ok {
		t.Fatalf("explicit product-union coalescing = %T %[1]v, want recursive family", coalesced)
	}
}

func TestJoinReturnSlot_MergesMissingOpenRecordFieldWithUnknownTail(t *testing.T) {
	candidate := NewTuple(NewRecord().
		Field("content", NewRecord().
			Field("parts", NewTuple(NewRecord().Field("text", String).Build())).
			Build()).
		Build())
	stream := NewRecord().
		Field("candidates", candidate).
		Field("status_code", Number).
		Build()
	decoded := NewRecord().
		Field("metadata", NewRecord().Build()).
		Field("status_code", Number).
		SetOpen(true).
		Build()

	got := JoinReturnSlot(stream, decoded)
	rec, ok := got.(*Record)
	if !ok {
		t.Fatalf("JoinReturnSlot(stream, decoded) = %T, want *Record", got)
	}
	field := rec.GetField("candidates")
	if field == nil {
		t.Fatalf("merged record lost candidates field: %v", rec)
	}
	if field.Optional {
		t.Fatalf("candidates should merge with the open row tail, not absence: %#v", field)
	}
	if !TypeEquals(field.Type, Unknown) {
		t.Fatalf("candidates = %v, want unknown from open row tail", field.Type)
	}
}

func TestRecordMapKeyRemovesImpossibleNil(t *testing.T) {
	rec := luatable.RebuildRecord(RecordParts{
		MapKey:   NewOptional(String),
		MapValue: Number,
	})
	if !TypeEquals(rec.MapKey, String) {
		t.Fatalf("record map key = %v, want string", rec.MapKey)
	}
}

func TestJoinReturnSlot_PreservesDiscriminatedRecordUnion(t *testing.T) {
	a := NewRecord().
		Field("kind", LiteralString("a")).
		Field("value", Number).
		Build()
	b := NewRecord().
		Field("kind", LiteralString("b")).
		Field("value", String).
		Build()

	got := JoinReturnSlot(a, b)
	if _, ok := got.(*Union); !ok {
		t.Fatalf("JoinReturnSlot(discriminated records) = %T, want *Union", got)
	}
}

// TestJoinReturnSlot_PreservesStructuralDiscriminantUnion proves discriminant
// detection is structural, not a field-name list: a shared required literal field
// with conflicting values whose erasure leaves disjoint required payloads keeps the
// records as a discriminated union regardless of the field name.
func TestJoinReturnSlot_PreservesStructuralDiscriminantUnion(t *testing.T) {
	a := NewRecord().
		Field("shape_kind", LiteralString("circle")).
		Field("r", Number).
		Build()
	b := NewRecord().
		Field("shape_kind", LiteralString("square")).
		Field("s", Number).
		Build()

	got := JoinReturnSlot(a, b)
	if _, ok := got.(*Union); !ok {
		t.Fatalf("JoinReturnSlot(shape_kind discriminated records) = %T %[1]v, want *Union", got)
	}

	tags := requiredDiscriminantTags(a)
	if tags["shape_kind"] != LiteralString("circle").Hash() {
		t.Fatalf("shape_kind not recognized as a structural discriminant tag: %v", tags)
	}
}

// TestJoinReturnSlot_CoalescesBroadScalarSharedField is the negative: a shared
// field that is a broad scalar rather than a literal is not a discriminant, so the
// records coalesce instead of forming a union.
func TestJoinReturnSlot_CoalescesBroadScalarSharedField(t *testing.T) {
	a := NewRecord().
		Field("count", Number).
		Field("a", Number).
		Build()
	b := NewRecord().
		Field("count", Number).
		Field("b", Number).
		Build()

	if got := JoinReturnSlot(a, b); !isRecordLike(got) {
		t.Fatalf("JoinReturnSlot(broad scalar shared field) = %T %[1]v, want coalesced record", got)
	}
}

func isRecordLike(t Type) bool {
	switch v := t.(type) {
	case *Record:
		return true
	case *Optional:
		return isRecordLike(v.Inner)
	case *Alias:
		return isRecordLike(v.Target)
	default:
		return false
	}
}

func TestJoinReturnSlot_PreservesNestedDiscriminatedRecordUnion(t *testing.T) {
	chanInt := NewAlias("__test_ChanInt", NewRecord().
		Field("__tag", LiteralString("int")).
		Build())
	chanStr := NewAlias("__test_ChanStr", NewRecord().
		Field("__tag", LiteralString("str")).
		Build())
	errCase := NewRecord().
		Field("channel", chanInt).
		Field("value", NewRecord().Field("error", String).Build()).
		Field("ok", Boolean).
		Build()
	dataCase := NewRecord().
		Field("channel", chanStr).
		Field("value", NewRecord().Field("data", Number).Build()).
		Field("ok", Boolean).
		Build()

	got := JoinReturnSlot(errCase, dataCase)
	union, ok := got.(*Union)
	if !ok {
		t.Fatalf("JoinReturnSlot(nested discriminated records) = %T %[1]v, want *Union", got)
	}
	if len(union.Members) != 2 {
		t.Fatalf("nested discriminated union members = %d, want 2", len(union.Members))
	}
}

func TestRequiredDiscriminantTags_PreservesNestedNonRecursiveTag(t *testing.T) {
	chanInt := NewAlias("__test_ChanInt", NewRecord().
		Field("__tag", LiteralString("int")).
		Build())
	errCase := NewRecord().
		Field("channel", chanInt).
		Field("value", NewRecord().Field("error", String).Build()).
		Build()

	tags := requiredDiscriminantTags(errCase)
	if tags["channel.__tag"] != LiteralString("int").Hash() {
		t.Fatalf("nested channel tag was not summarized: %v", tags)
	}
}

func TestRequiredDiscriminantTags_RecursiveCycleSummarizesFiniteTags(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("kind", LiteralString("node")).
			Field("next", self).
			Build()
	})

	tags := requiredDiscriminantTags(node)
	if tags["kind"] != LiteralString("node").Hash() {
		t.Fatalf("recursive top-level tag was not summarized: %v", tags)
	}
	if _, ok := tags["next.kind"]; ok {
		t.Fatalf("recursive discriminant summary unfolded through self: %v", tags)
	}
}

func TestFoldedProductFamilyMembersCollapseEquivalentRecursiveProducts(t *testing.T) {
	members := make([]Type, 0, 128)
	for i := 0; i < cap(members); i++ {
		members = append(members, NewRecursive("Node", func(self Type) Type {
			return NewRecord().
				Field("name", String).
				OptField("next", self).
				Build()
		}))
	}

	state := newReturnJoinState()
	state.recursiveFamilyFold = true
	got := state.coalesceProductUnionMembers(members)
	if len(got) != 1 {
		t.Fatalf("folded recursive products = %d members, want 1", len(got))
	}
}

func TestJoinReturnSlot_MessageLiteralMismatchStillCoalesces(t *testing.T) {
	a := NewRecord().
		Field("status_code", LiteralInt(401)).
		Field("message", LiteralString("invalid key")).
		Build()
	b := NewRecord().
		Field("status_code", LiteralInt(400)).
		Field("message", LiteralString("invalid model")).
		Field("error", NewRecord().Field("type", String).Build()).
		Build()

	got := JoinReturnSlot(a, b)
	rec, ok := got.(*Record)
	if !ok {
		t.Fatalf("JoinReturnSlot(non-discriminant literal mismatch) = %T, want *Record", got)
	}
	errorField := rec.GetField("error")
	if errorField == nil || !errorField.Optional {
		t.Fatalf("expected optional error field after coalescing, got %v", got)
	}
	messageField := rec.GetField("message")
	if messageField == nil || !TypeEquals(messageField.Type, String) {
		t.Fatalf("non-discriminant literal message field should widen to string, got %v", messageField)
	}
	statusField := rec.GetField("status_code")
	if statusField == nil || !TypeEquals(statusField.Type, Integer) {
		t.Fatalf("non-discriminant integer literal status should widen to integer, got %v", statusField)
	}
}

func TestJoinReturnSlot_NestedEqualLiteralFamilyCoalescesPayloadLiterals(t *testing.T) {
	a := NewRecord().
		Field("type", LiteralString("CREATE_DATA")).
		Field("payload", NewRecord().
			Field("data_type", LiteralString("NODE_INPUT")).
			Field("content_type", LiteralString("text/plain")).
			Field("content", String).
			Build()).
		Build()
	b := NewRecord().
		Field("type", LiteralString("CREATE_DATA")).
		Field("payload", NewRecord().
			Field("data_type", LiteralString("NODE_INPUT")).
			Field("content_type", LiteralString("dataflow/reference")).
			Field("content", LiteralString("")).
			Field("node_id", String).
			Build()).
		Build()

	got := JoinReturnSlot(a, b)
	rec, ok := got.(*Record)
	if !ok {
		t.Fatalf("JoinReturnSlot(nested equal literal family) = %T %[1]v, want *Record", got)
	}
	payloadField := rec.GetField("payload")
	if payloadField == nil {
		t.Fatalf("joined payload field missing in %v", got)
	}
	payload, ok := payloadField.Type.(*Record)
	if !ok {
		t.Fatalf("joined payload = %T %[1]v, want *Record", payloadField.Type)
	}
	if field := payload.GetField("data_type"); field == nil || !TypeEquals(field.Type, LiteralString("NODE_INPUT")) {
		t.Fatalf("payload data_type = %v, want NODE_INPUT literal", field)
	}
	if field := payload.GetField("content_type"); field == nil || !TypeEquals(field.Type, String) {
		t.Fatalf("payload content_type = %v, want string", field)
	}
	if field := payload.GetField("node_id"); field == nil || !field.Optional {
		t.Fatalf("payload node_id = %v, want optional", field)
	}
}

func TestJoinUnionFieldSlot_WidensAccumulatedNonDiscriminantLiteralUnion(t *testing.T) {
	// A non-discriminant data field widens its differing literals to the base so a
	// many-member union read does not explode into a literal union.
	acc := JoinUnionFieldSlot(LiteralString("a"), LiteralString("b"), false)
	got := JoinUnionFieldSlot(acc, LiteralString("c"), false)
	if !TypeEquals(got, String) {
		t.Fatalf("JoinUnionFieldSlot(non-discriminant literals) = %v, want string", got)
	}

	// A field the caller identifies as the union's discriminant keeps its distinct
	// literal alternatives for path-sensitive narrowing.
	tag := JoinUnionFieldSlot(LiteralString("a"), LiteralString("b"), true)
	if _, ok := tag.(*Union); !ok {
		t.Fatalf("JoinUnionFieldSlot(discriminant literals) = %T %[1]v, want discriminant union", tag)
	}
}

func TestJoinUnionFieldSlot_JoinsArrayElementsPointwise(t *testing.T) {
	left := NewArray(NewRecord().
		Field("name", LiteralString("a")).
		Field("line", LiteralInt(1)).
		Build())
	right := NewArray(NewRecord().
		Field("name", LiteralString("b")).
		Field("line", LiteralInt(2)).
		Build())

	got, ok := JoinUnionFieldSlot(left, right, false).(*Array)
	if !ok {
		t.Fatalf("JoinUnionFieldSlot(children arrays) = %T, want array", got)
	}
	elem, ok := got.Element.(*Record)
	if !ok {
		t.Fatalf("joined array element = %T %[1]v, want record", got.Element)
	}
	name := elem.GetField("name")
	if name == nil || !TypeEquals(name.Type, String) {
		t.Fatalf("joined child name = %v, want string", name)
	}
	line := elem.GetField("line")
	if line == nil || !TypeEquals(line.Type, Integer) {
		t.Fatalf("joined child line = %v, want integer", line)
	}
}

func TestJoinReturnSlot_CoalescesUnionRecordMember(t *testing.T) {
	base := NewRecord().
		Field("status_code", Number).
		Field("message", String).
		Build()
	withDetails := NewRecord().
		Field("status_code", Number).
		Field("message", String).
		Field("code", String).
		Field("type", String).
		Build()
	unionWithNil := NewUnion(Nil, base)

	got := JoinReturnSlot(unionWithNil, withDetails)
	opt, ok := got.(*Optional)
	if !ok {
		t.Fatalf("JoinReturnSlot(union, record) = %T, want *Optional", got)
	}
	merged := unaliasRecord(opt.Inner)
	if merged == nil {
		t.Fatalf("expected merged record member, got %T", opt.Inner)
	}
	codeField := merged.GetField("code")
	if codeField == nil || !codeField.Optional || !TypeEquals(codeField.Type, String) {
		t.Fatalf("expected optional code:string after coalescing, got %v", codeField)
	}
	typeField := merged.GetField("type")
	if typeField == nil || !typeField.Optional || !TypeEquals(typeField.Type, String) {
		t.Fatalf("expected optional type:string after coalescing, got %v", typeField)
	}
}

func TestIsRefinableAnnotation(t *testing.T) {
	tests := []struct {
		name string
		t    Type
		want bool
	}{
		{"nil", nil, false},
		{"any", Any, false},
		{"unknown", Unknown, false},
		{"optional any", NewOptional(Any), false},
		{"array any", NewArray(Any), true},
		{"map string any", NewMap(String, Any), true},
		{"map string any array", NewMap(String, NewArray(Any)), true},
		{"open table top", NewRecord().SetOpen(true).Build(), true},
		{"array or open table top", NewUnion(NewArray(Any), NewRecord().SetOpen(true).Build()), true},
		{"record map any", NewRecord().MapComponent(String, Any).Build(), true},
		{"record", NewRecord().Field("id", String).Build(), false},
	}

	for _, tt := range tests {
		if got := IsRefinableAnnotation(tt.t); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCoalesceCompatibleRecords_MergesExtraFieldsAsOptional(t *testing.T) {
	base := NewRecord().
		Field("status_code", Number).
		Field("message", String).
		Build()
	withDetails := NewRecord().
		Field("status_code", Number).
		Field("message", String).
		Field("code", String).
		Field("type", String).
		Build()

	result := CoalesceCompatibleRecords([]Type{base, withDetails})
	if len(result) != 1 {
		t.Fatalf("expected one merged record, got %d", len(result))
	}

	rec, ok := result[0].(*Record)
	if !ok {
		t.Fatalf("expected record, got %T", result[0])
	}
	fields := map[string]Field{}
	for _, f := range rec.Fields {
		fields[f.Name] = f
	}
	if !fields["code"].Optional || !TypeEquals(fields["code"].Type, String) {
		t.Fatalf("expected optional code:string, got %#v", fields["code"])
	}
	if !fields["type"].Optional || !TypeEquals(fields["type"].Type, String) {
		t.Fatalf("expected optional type:string, got %#v", fields["type"])
	}
}

func TestCoalesceCompatibleRecords_PreservesDiscriminatedUnion(t *testing.T) {
	a := NewRecord().
		Field("kind", LiteralString("a")).
		Field("x", Number).
		Build()
	b := NewRecord().
		Field("kind", LiteralString("b")).
		Field("y", String).
		Build()

	result := CoalesceCompatibleRecords([]Type{a, b})
	if len(result) != 2 {
		t.Fatalf("expected discriminated variants to remain separate, got %d", len(result))
	}
}

func TestProjectUnionMembersLeavesRecursiveFamiliesForPolicyCoalescing(t *testing.T) {
	base := NewRecursive("SuiteA", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Build()
	})
	withPath := NewRecursive("SuiteB", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Field("full_path", String).
			Build()
	})
	u, ok := NewUnion(base, Boolean, withPath).(*Union)
	if !ok {
		t.Fatalf("expected union")
	}

	got := ProjectUnionMembers(u, func(member Type) Type {
		if member == Boolean {
			return True
		}
		return member
	})
	union, ok := got.(*Union)
	if !ok {
		t.Fatalf("projected recursive family union = %T %v, want union", got, got)
	}
	recursiveCount := 0
	for _, member := range union.Members {
		if _, ok := member.(*Recursive); ok {
			recursiveCount++
		}
	}
	if recursiveCount != 2 {
		t.Fatalf("projected union kept %d recursive family members, want 2: %v", recursiveCount, got)
	}

	coalesced := CoalesceProductUnion(got)
	coalescedUnion, ok := coalesced.(*Union)
	if !ok {
		t.Fatalf("policy-coalesced projection = %T %v, want union", coalesced, coalesced)
	}
	recursiveCount = 0
	for _, member := range coalescedUnion.Members {
		if _, ok := member.(*Recursive); ok {
			recursiveCount++
		}
	}
	if recursiveCount != 1 {
		t.Fatalf("policy coalescing kept %d recursive family members, want 1: %v", recursiveCount, coalesced)
	}
}

func TestOptionalFieldKeepsProductCoalescingAtPolicyBoundaryAfterNilRemoval(t *testing.T) {
	base := NewRecursive("SuiteA", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Build()
	})
	withPath := NewRecursive("SuiteB", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Field("full_path", String).
			Build()
	})
	record := NewRecord().
		OptField("parent", NewUnion(Nil, base, withPath)).
		Build()
	field := record.GetField("parent")
	if field == nil {
		t.Fatal("missing parent field")
	}
	if _, ok := field.Type.(*Union); !ok {
		t.Fatalf("optional recursive family field = %T %v, want explicit union before policy coalescing", field.Type, field.Type)
	}
	if _, ok := CoalesceProductUnion(field.Type).(*Recursive); !ok {
		t.Fatalf("explicit recursive family coalescing = %T %v, want recursive product", field.Type, field.Type)
	}
}
