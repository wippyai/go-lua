package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

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

func TestMemberFromSegmentKeepsStaticStringIndexStructural(t *testing.T) {
	key, ok := MemberFromSegment(constraint.Segment{Kind: constraint.SegmentIndexString, Name: "x-y"})
	if !ok {
		t.Fatal("expected static string-index segment to lower")
	}
	if key.Kind() != MemberKindStringIndex || key.Name() != "x-y" {
		t.Fatalf("key = %#v, want string-index x-y", key)
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
