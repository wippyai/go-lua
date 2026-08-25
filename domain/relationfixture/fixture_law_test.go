package relationfixture_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestTheFixtureSealsRealOwnerAuthorities states what every binding law rests
// on: the authorities the bindings carry are the production ones, sealed from
// a real program, not stand-ins that would prove a binding against nothing.
func TestTheFixtureSealsRealOwnerAuthorities(t *testing.T) {
	fixture := relationfixture.New(t)
	if !fixture.Heap.Valid() || !fixture.Values.Valid() || !fixture.Root.Valid() {
		t.Fatal("the fixture did not seal the owner authorities the bindings carry")
	}
	if fixture.Calls == nil || fixture.Packs == nil || fixture.Topology == nil {
		t.Fatal("the fixture did not seal every owner a binding reads through")
	}
	t.Logf("heap keys=%d value coordinates=%d module loads=%d", fixture.Heap.KeyCount(), fixture.Values.CoordinateCount(), moduleLoadCount(fixture.Values))
}

func moduleLoadCount(values *valuedomain.Schema) int {
	count := 0
	for {
		if _, ok := values.ModuleLoadCallAt(count); !ok {
			return count
		}
		count++
	}
}
