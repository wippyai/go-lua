package typeauthority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/subtype"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestFamilyPrefixSubtypeBitsMatchFreshProver(t *testing.T) {
	family, err := SealFamily("test/family-prefix", []typ.Type{
		typ.String,
		typ.NewArray(typ.Integer),
		typ.NewArray(typ.NewArray(typ.String)),
	})
	if err != nil {
		t.Fatal(err)
	}
	prefix := family.prefix
	if prefix == nil || len(prefix.rows) == 0 || len(prefix.closedRows) == 0 {
		t.Fatal("family prefix universe")
	}
	if len(prefix.construction) != len(prefix.rows) || len(prefix.subtypeBits) == 0 {
		t.Fatal("family prefix relation")
	}
	var prover subtype.Batch
	for leftPosition, leftRow := range prefix.closedRows {
		left := prefix.construction[leftRow-1]
		for rightPosition, rightRow := range prefix.closedRows {
			want := prover.IsSubtype(left, prefix.construction[rightRow-1])
			word := prefix.subtypeBits[leftPosition*prefix.subtypeStride+int(rightPosition)>>6]
			got := word&(1<<(uint(rightPosition)&63)) != 0
			if got != want {
				t.Fatalf("prefix subtype %d <: %d = %t, prover = %t", leftRow, rightRow, got, want)
			}
		}
	}
}

func TestFamilyPrefixMergeKeepsPrefixSubtypeBits(t *testing.T) {
	familyValues := []typ.Type{typ.String, typ.NewArray(typ.Integer)}
	family, err := SealFamily("test/family-prefix-merge", familyValues)
	if err != nil {
		t.Fatal(err)
	}
	familyOnly, familyInners, _ := sealFamilyRuntime(t, family, nil)
	localType := typ.NewArray(typ.Boolean)
	mixed, mixedInners, extra := sealFamilyRuntime(t, family, []typ.Type{localType})
	if len(extra) != 1 {
		t.Fatal("local inner")
	}
	if mixed.Count() != familyOnly.Count()+1 {
		t.Fatalf("mixed Runtime count = %d, family-only = %d", mixed.Count(), familyOnly.Count())
	}
	for left := 0; left < family.Count(); left++ {
		for right := 0; right < family.Count(); right++ {
			want, wantOK := familyOnly.Subtype(familyInners[left], familyInners[right])
			got, gotOK := mixed.Subtype(mixedInners[left], mixedInners[right])
			if !wantOK || !gotOK || got != want {
				t.Fatalf("prefix pair %d <: %d mixed=%t/%t family-only=%t/%t", left, right, got, gotOK, want, wantOK)
			}
		}
	}
	var prover subtype.Batch
	for index := 0; index < family.Count(); index++ {
		gotLeft, leftOK := mixed.Subtype(extra[0], mixedInners[index])
		gotRight, rightOK := mixed.Subtype(mixedInners[index], extra[0])
		if !leftOK || !rightOK {
			t.Fatalf("local/family member %d subtype undecided", index)
		}
		if gotLeft != prover.IsSubtype(localType, familyValues[index]) {
			t.Fatalf("local <: family member %d", index)
		}
		if gotRight != prover.IsSubtype(familyValues[index], localType) {
			t.Fatalf("family member %d <: local", index)
		}
	}
}

func sealFamilyRuntime(t *testing.T, family *Family, extra []typ.Type) (*Runtime, []RuntimeInner, []RuntimeInner) {
	t.Helper()
	authority := &Authority{linkID: identity.ContentID{1}, artifact: &artifactAuthority{}}
	inputs := make([]RuntimeInput, 0, family.Count()+len(extra))
	for index := 0; index < family.Count(); index++ {
		input, ok := family.Input(index, authority)
		if !ok {
			t.Fatalf("family member %d", index)
		}
		inputs = append(inputs, input)
	}
	for _, value := range extra {
		input, ok := authority.RuntimeInputForType(value)
		if !ok {
			t.Fatal("local RuntimeInput")
		}
		inputs = append(inputs, input)
	}
	runtime, inners, err := SealRuntime(authority, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(inners) != len(inputs) {
		t.Fatalf("SealRuntime inners = %d, inputs = %d", len(inners), len(inputs))
	}
	return runtime, inners[:family.Count()], inners[family.Count():]
}
