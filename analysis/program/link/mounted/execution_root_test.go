package mounted

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestExecutionRootRequiresCompleteIdentity(t *testing.T) {
	available := orderLawID("available")
	if !(ExecutionRoot{Mount: available, Body: available, Entry: available}).Available() {
		t.Fatal("complete execution root was unavailable")
	}
	for _, root := range []ExecutionRoot{
		{Mount: identity.ContentID{}, Body: available, Entry: available},
		{Mount: available, Body: identity.ContentID{}, Entry: available},
		{Mount: available, Body: available, Entry: identity.ContentID{}},
	} {
		if root.Available() {
			t.Fatalf("incomplete execution root was admitted: %#v", root)
		}
	}
	if CompareExecutionRoot(ExecutionRoot{Mount: available, Body: available, Entry: orderLawID("low")}, ExecutionRoot{Mount: available, Body: available, Entry: orderLawID("high")}) >= 0 {
		t.Fatal("execution root order ignored entry identity")
	}
}
