package observation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestInvocationIdentityCommitsParentCallerAndCallOccurrence(t *testing.T) {
	var callerA, callerB lexicalidentity.StableLexicalBodyID
	callerA[0], callerB[0] = 1, 2
	callA := Occurrence{Point: wir.DebugPointID{Ordinal: 3, Phase: wir.DebugPhaseCall}, Kind: CallInvocation}
	callB := callA
	callB.Point.Ordinal++
	base, ok := ExtendInvocation(InvocationID{}, callerA, callA)
	repeat, _ := ExtendInvocation(InvocationID{}, callerA, callA)
	otherCaller, _ := ExtendInvocation(InvocationID{}, callerB, callA)
	otherCall, _ := ExtendInvocation(InvocationID{}, callerA, callB)
	var parent InvocationID
	parent[0] = 9
	otherParent, _ := ExtendInvocation(parent, callerA, callA)
	if !ok || base != repeat || base == otherCaller || base == otherCall || base == otherParent {
		t.Fatalf("invocation separation failed: %x %x %x %x %x", base, repeat, otherCaller, otherCall, otherParent)
	}
	invalid := callA
	invalid.Kind = CallResult
	if got, ok := ExtendInvocation(InvocationID{}, callerA, invalid); ok || got != (InvocationID{}) {
		t.Fatal("non-invocation occurrence admitted")
	}
}
