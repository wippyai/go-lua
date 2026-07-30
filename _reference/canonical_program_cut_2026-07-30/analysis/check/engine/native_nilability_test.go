package engine

import (
	"reflect"
	"strings"
	"testing"
)

func TestNativeNilabilityPublishesOnlyExactNilPathRefinements(t *testing.T) {
	result, err := Check(`
local function run(x: string?, flag: boolean?): string
    if x then return x end
    if flag then return "flag" end
    return "none"
end
return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	var xRows, flagRows []NativeFact
	for _, fact := range result.Native.Facts() {
		if fact.Family != "nilability" {
			continue
		}
		switch fact.Subject {
		case "x":
			xRows = append(xRows, fact)
		case "flag":
			flagRows = append(flagRows, fact)
		}
	}
	if len(xRows) != 2 {
		t.Fatalf("x nilability rows = %#v, want a combined branch/non-nil fact and exact nil edge", xRows)
	}
	if len(flagRows) != 0 {
		t.Fatalf("boolean truthiness rows = %#v, want none because false remains possible", flagRows)
	}
	for _, row := range xRows {
		want := []NativeRevocation{{Established: "contract", Revoked: "contract/nilability", Event: "write.local"}}
		if row.Trust != NativeTrustProven || row.Established == "" || !reflect.DeepEqual(row.Revocations, want) {
			t.Fatalf("x row = %#v, want a proven local invalidation contract", row)
		}
	}
}

func TestNativeNilabilityCapturesCompleteInvalidationClass(t *testing.T) {
	result, err := Check(`
type Thunk = () -> ()
local function run(x: number?, notify: (Thunk) -> ()): number
    if x == nil then return 0 end
    local function clear() x = nil end
    notify(clear)
    return x
end
return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	var got []NativeFact
	for _, fact := range result.Native.Facts() {
		if fact.Family == "nilability" && fact.Subject == "x" && strings.Contains(fact.Value, "nilability=non_nil") {
			got = append(got, fact)
		}
	}
	if len(got) != 1 {
		t.Fatalf("captured x rows = %#v, want one", got)
	}
	want := []NativeRevocation{
		{Established: "contract", Revoked: "contract/nilability", Event: "write.local"},
		{Established: "contract", Revoked: "contract/nilability", Event: "write.upvalue"},
		{Established: "contract", Revoked: "contract/nilability", Event: "call.opaque"},
	}
	if !reflect.DeepEqual(got[0].Revocations, want) {
		t.Fatalf("captured x revocations = %#v, want %#v", got[0].Revocations, want)
	}
	if got[0].Event != "write.local" || got[0].Established == "" || got[0].Revoked == "" {
		t.Fatalf("captured x interval = %#v, want a published contract interval", got[0])
	}
	first, err := Check(`local function f(x: string?): string if x then return x end return "" end return f`)
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	second, err := Check(`local function f(x: string?): string if x then return x end return "" end return f`)
	if err != nil {
		t.Fatalf("second check: %v", err)
	}
	if !reflect.DeepEqual(first.Native.Facts(), second.Native.Facts()) {
		t.Fatal("nilability projection is not deterministic")
	}
}
