package route

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestDeriveRefusesWithoutTheThreeSealedOwners(t *testing.T) {
	if _, ok := Derive(nil, nil, heapdomain.Schema{}, calldomain.CallCoordinate{}, valuedomain.Value{}); ok {
		t.Fatal("dispatch relation admitted absent Call, Value, and Heap owners")
	}
}
