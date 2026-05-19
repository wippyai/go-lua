package typ

import "testing"

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
	rec := NewRecord().MapComponent(NewOptional(String), Number).Build()
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
