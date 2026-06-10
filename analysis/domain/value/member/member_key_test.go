package member

import "testing"

func TestMemberKeyDistinguishesFieldAndStaticStringIndex(t *testing.T) {
	field := MemberField("x")
	index := MemberStringIndex("x")

	if !field.IsValid() || !index.IsValid() {
		t.Fatal("expected valid member keys")
	}
	if field == index {
		t.Fatal("field and static string-index members must remain distinct")
	}
}

func TestSortMemberKeysDeterministicStructuralOrder(t *testing.T) {
	keys := []MemberKey{
		MemberStringIndex("a"),
		MemberField("b"),
		MemberIntIndex(2),
		MemberField("a"),
		MemberStringIndex(""),
	}

	SortMemberKeys(keys)
	want := []MemberKey{
		MemberField("a"),
		MemberField("b"),
		MemberStringIndex(""),
		MemberStringIndex("a"),
		MemberIntIndex(2),
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys[%d] = %#v, want %#v (all keys %#v)", i, keys[i], want[i], keys)
		}
	}
}
