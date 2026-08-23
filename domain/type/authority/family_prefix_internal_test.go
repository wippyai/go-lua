package typeauthority

import (
	"testing"

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
