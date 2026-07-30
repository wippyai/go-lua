package engine

import (
	"strings"
	"testing"
)

// checkDiagnosticKeys is the family/message projection these metatable cases
// judge. A member read that composes with an `__index` chain must be decided by
// the composed surface, so both the presence and the reason of a refutation are
// part of the expectation.
func checkDiagnosticKeys(t *testing.T, source string) []string {
	t.Helper()
	result, err := Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	rows := make([]string, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		rows = append(rows, diagnosticCode(diagnostic.Key)+": "+string(diagnostic.Value))
	}
	return rows
}

func TestMetatableIndexSurfaceTypesInheritedRead(t *testing.T) {
	got := checkDiagnosticKeys(t, `
local Proto = { kind = "circle" }
Proto.__index = Proto
local obj = setmetatable({ radius = 1 }, Proto)
local tag: string = obj.kind
local own: number = obj.radius
return tag, own`)
	if len(got) != 0 {
		t.Fatalf("diagnostics = %#v, want none: the prototype supplies a string tag and the instance its own radius", got)
	}
}

func TestMetatableIndexSurfaceRefutesInheritedType(t *testing.T) {
	got := checkDiagnosticKeys(t, `
local Proto = { kind = "circle" }
Proto.__index = Proto
local obj = setmetatable({ radius = 1 }, Proto)
local tag: number = obj.kind
return tag`)
	want := `type.assignment: cannot assign obj.kind because it is string, not number`
	if len(got) != 1 || got[0] != want {
		t.Fatalf("diagnostics = %#v, want exactly %q", got, want)
	}
}

func TestMetatableIndexSurfaceRefutesMemberAbsentFromWholeChain(t *testing.T) {
	got := checkDiagnosticKeys(t, `
local Base = { kind = "circle" }
local Proto = { __index = Base }
local obj = setmetatable({ radius = 1 }, Proto)
local nope: string = obj.absent
return nope`)
	want := `type.assignment: cannot assign obj.absent because it is nil, not string`
	if len(got) != 1 || got[0] != want {
		t.Fatalf("diagnostics = %#v, want exactly %q", got, want)
	}
}

func TestMetatableIndexSurfaceComposesInstanceAgainstDeclaredRecord(t *testing.T) {
	got := checkDiagnosticKeys(t, `
type Circle = { kind: "circle", radius: number }
local Proto = { kind = "circle" }
Proto.__index = Proto
local ok: Circle = setmetatable({ radius = 1 }, Proto)
return ok`)
	if len(got) != 0 {
		t.Fatalf("diagnostics = %#v, want none: the metatable supplies the tag the record declares", got)
	}
}

func TestMetatableIndexSurfaceRefutesInstanceMissingItsPayload(t *testing.T) {
	got := checkDiagnosticKeys(t, `
type Circle = { kind: "circle", radius: number }
local Proto = { kind = "circle" }
Proto.__index = Proto
local bad: Circle = setmetatable({ side = 1 }, Proto)
return bad`)
	if len(got) != 1 || !strings.HasPrefix(got[0], "type.assignment: ") {
		t.Fatalf("diagnostics = %#v, want one assignment refutation: the metatable supplies the tag but never the payload", got)
	}
}

func TestMetatableIndexSurfaceRevokedByRepointedIndex(t *testing.T) {
	got := checkDiagnosticKeys(t, `
local first = { kind = "circle", radius = 2 }
local second = { kind = "square" }
local meta = { __index = first }
local obj = setmetatable({}, meta)
meta.__index = second
local r: number = obj.radius
return r`)
	want := `type.assignment: cannot assign obj.radius because it is nil, not number`
	if len(got) != 1 || got[0] != want {
		t.Fatalf("diagnostics = %#v, want exactly %q: the repointed base no longer supplies the payload", got, want)
	}
}

func TestMetatableIndexSurfaceReadsThroughSealedChainBeforeRepoint(t *testing.T) {
	got := checkDiagnosticKeys(t, `
local first = { kind = "circle", radius = 2 }
local meta = { __index = first }
local obj = setmetatable({}, meta)
local r: number = obj.radius
return r`)
	if len(got) != 0 {
		t.Fatalf("diagnostics = %#v, want none: the sealed base supplies the payload", got)
	}
}

func TestMetatableIndexSurfaceKeepsNoProofForFunctionIndex(t *testing.T) {
	// A function `__index` is a dispatch the value lattice does not model, so
	// the composed surface is withheld and the read stays unproven.
	got := checkDiagnosticKeys(t, `
local function supply(t, key): string
    return "circle"
end
local Proto = { __index = supply }
local obj = setmetatable({ radius = 1 }, Proto)
local tag: string = obj.kind
return tag`)
	if len(got) != 1 || !strings.HasPrefix(got[0], "lint.claim.unproven: ") {
		t.Fatalf("diagnostics = %#v, want one unproven claim: a function metamethod is opaque", got)
	}
}

func TestMetatableIndexSurfaceNearestLinkShadowsFarther(t *testing.T) {
	got := checkDiagnosticKeys(t, `
local Root = { kind = "root" }
local Middle = { kind = "middle" }
Middle.__index = Root
local Near = { __index = Middle }
local obj = setmetatable({}, Near)
local tag: "middle" = obj.kind
return tag`)
	if len(got) != 0 {
		t.Fatalf("diagnostics = %#v, want none: the nearest link on the chain supplies the tag", got)
	}
}
