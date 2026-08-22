package program_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
)

func TestCallIdentityAtIssuesCompleteFormalAndTypeArgumentIdentities(t *testing.T) {
	input, err := lower.Lower(lower.Source{Name: "call-identity-law.lua", Text: []byte(`
local function apply<T>(value: T): T
  return value
end
return apply::<string>(1), apply::<number>(2)
`)})
	if err != nil {
		t.Fatal(err)
	}
	if input == nil || !input.Available() {
		t.Fatal("lowered Program unavailable")
	}
	calls := input.Flow().Authored().Calls()
	found := false
	for index := 0; index < calls.Count(); index++ {
		call, callOK := calls.At(index)
		count, countOK := input.Static().Contracts().Calls().TypeArgumentCount(call)
		if !callOK || !countOK || count == 0 {
			continue
		}
		identities, identityOK := input.CallIdentityAt(index)
		if !identityOK || !identities.Call.Available() || !identities.Formal.Available() ||
			!identities.TypeArguments.Available() || len(identities.TypeArgumentAt) != count {
			t.Fatalf("CallIdentityAt(%d) incomplete: ok=%v formal=%v types=%v children=%d/%d", index, identityOK, identities.Formal.Available(), identities.TypeArguments.Available(), len(identities.TypeArgumentAt), count)
		}
		for childIndex, childID := range identities.TypeArgumentAt {
			if !childID.Available() {
				t.Fatalf("CallIdentityAt(%d) type argument %d unavailable", index, childIndex)
			}
		}
		found = true
		break
	}
	if !found {
		t.Fatal("fixture did not issue an authored call with type arguments")
	}
}
