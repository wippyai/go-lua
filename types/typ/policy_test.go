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

func TestJoinReturnSlot_PreservesAnyOverNil(t *testing.T) {
	if got := JoinReturnSlot(Any, Nil); !TypeEquals(got, Any) {
		t.Fatalf("JoinReturnSlot(any, nil) = %v, want any", got)
	}
	if got := JoinReturnSlot(Nil, Any); !TypeEquals(got, Any) {
		t.Fatalf("JoinReturnSlot(nil, any) = %v, want any", got)
	}
}

func TestJoinReturnPaths_AnyAbsorbsConcreteReturn(t *testing.T) {
	stream := NewRecord().Field("headers", NewMap(String, Any)).Build()
	if got := JoinReturnPaths(Any, stream); got != Any {
		t.Fatalf("JoinReturnPaths(any, stream) = %v, want any", got)
	}
	if got := JoinReturnPaths(stream, Any); got != Any {
		t.Fatalf("JoinReturnPaths(stream, any) = %v, want any", got)
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

func TestJoinBranchOutcome_PrefersConcreteOverSoft(t *testing.T) {
	left := NewOptional(NewArray(Any))
	right := NewArray(Number)
	got := JoinBranchOutcome(left, right)
	if got == nil || got.String() != "number[]" {
		t.Fatalf("JoinBranchOutcome(%v, %v) = %v, want number[]", left, right, got)
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

// Never holds no values: joining it with any type yields that type, including
// soft placeholders such as an unannotated any.
func TestJoinPreferNonSoft_NeverIsIdentity(t *testing.T) {
	for _, other := range []Type{Any, Nil, String, NewRecord().Field("id", String).Build()} {
		if got := JoinPreferNonSoft(other, Never); !TypeEquals(got, other) {
			t.Fatalf("join(%s, never) = %s, want %s", other, got, other)
		}
		if got := JoinPreferNonSoft(Never, other); !TypeEquals(got, other) {
			t.Fatalf("join(never, %s) = %s, want %s", other, got, other)
		}
	}
}

func TestJoinBranchOutcome_UnknownOperandDominates(t *testing.T) {
	empty := NewRecord().Build()
	rec := NewRecord().Field("id", String).Build()
	for _, other := range []Type{empty, rec, String} {
		if got := JoinBranchOutcome(Unknown, other); !TypeEquals(got, Unknown) {
			t.Fatalf("JoinBranchOutcome(unknown, %v) = %v, want unknown", other, got)
		}
		if got := JoinBranchOutcome(other, Unknown); !TypeEquals(got, Unknown) {
			t.Fatalf("JoinBranchOutcome(%v, unknown) = %v, want unknown", other, got)
		}
	}
}

func TestJoinBranchOutcome_UnresolvedOperandStaysPending(t *testing.T) {
	got := JoinBranchOutcome(Unresolved, String)
	u, ok := got.(*Union)
	if !ok || !u.Contains(Unresolved) {
		t.Fatalf("JoinBranchOutcome(unresolved, string) = %v, want union keeping unresolved", got)
	}
}

func TestJoinBranchOutcome_FalsyAlternativeKeepsSoftTable(t *testing.T) {
	value := NewMap(String, NewMap(String, Unknown))
	for _, falsy := range []Type{False, Nil} {
		want := NewUnion(falsy, value)
		for _, operands := range [][2]Type{{falsy, value}, {value, falsy}} {
			if got := JoinBranchOutcome(operands[0], operands[1]); !TypeEquals(got, want) {
				t.Fatalf("JoinBranchOutcome(%v, %v) = %v, want %v", operands[0], operands[1], got, want)
			}
		}
	}
}
