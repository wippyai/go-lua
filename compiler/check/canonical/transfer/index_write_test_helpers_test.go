package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func testIndexWriteAdmission(
	t *testing.T,
	facts flow.IndexWriteAdmissionFacts,
	target constraint.Path,
	keyPath constraint.Path,
	keyType typ.Type,
) (product.AbstractValue, bool) {
	t.Helper()
	targetAddr, ok := flow.StableAddressOfPath(target)
	if !ok {
		t.Fatalf("stable target address for %s", target.String())
	}
	query := flow.IndexWriteAddressQuery{Target: targetAddr}
	if !keyPath.IsEmpty() {
		keyAddr, ok := flow.StableAddressOfPath(keyPath)
		if !ok {
			t.Fatalf("stable key address for %s", keyPath.String())
		}
		query.KeyPath = keyAddr
		query.HasKeyPath = true
	}
	if !typ.IsAbsentOrUnknown(keyType) {
		query.KeyValue = product.FromType(keyType)
	}
	return facts.AdmissionAtAddress(query)
}
