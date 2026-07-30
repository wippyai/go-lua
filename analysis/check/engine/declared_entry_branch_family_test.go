package engine_test

import (
	"strings"
	"testing"
)

// An uncalled body whose branch is stated over its own declared formals is
// decided by the declaration alone whatever the condition's family: the entry
// seeds every formal with the declared type that joins the arguments the
// declaration admits, so both arms are reached by some admissible call and an
// obligation left unproven inside an arm belongs to this declaration. These
// tests pin that the admitted family is not a fixed list.

const branchFamilyProvider = `local function provide(): string?
    return nil
end
`

func branchFamilyBody(parameter, condition string) string {
	return branchFamilyProvider + `local function f(` + parameter + `)
    if ` + condition + ` then
        local v: string = provide()
        return v
    end
    return "x"
end
return f
`
}

// TestFormalConditionAdmitsEveryPredicateFamily pins that each of these
// conditions is stated wholly over the declared formal, so each admits the same
// assignment obligation. A length floor is one member of that class, not its
// definition.
func TestFormalConditionAdmitsEveryPredicateFamily(t *testing.T) {
	cases := []struct{ parameter, condition string }{
		{"s: string", "#s >= 1"},
		{"flag: boolean", "flag"},
		{"flag: boolean", "not flag"},
		{"s: string?", "s ~= nil"},
		{"s: string?", "s == nil"},
		{"s: string", `s == "a"`},
		{"s: string", `s ~= "a"`},
		{"n: integer", "n > 3"},
		{"a: string, b: string", "a == b"},
		{"v: string | integer", `type(v) == "string"`},
	}
	for _, item := range cases {
		diagnostics := checkSource(t, branchFamilyBody(item.parameter, item.condition))
		if !strings.Contains(diagnosticSummaries(diagnostics), "type.assignment") {
			t.Errorf("condition %q over declared formals left the body dormant:\n%s",
				item.condition, diagnosticSummaries(diagnostics))
		}
	}
}

// TestForeignRootedConditionKeepsBodyDormant pins the other side of the same
// relation: a condition rooted outside the declared formals rests on an
// authority this entry never establishes, so the body waits for a call.
func TestForeignRootedConditionKeepsBodyDormant(t *testing.T) {
	diagnostics := checkSource(t, `local function provide(): string?
    return nil
end
local ready = provide() ~= nil
local function f(s: string)
    if ready then
        local v: string = provide()
        return v
    end
    return s
end
return f
`)
	if strings.Contains(diagnosticSummaries(diagnostics), "type.assignment") {
		t.Fatalf("a condition rooted outside the declared formals admitted the body:\n%s",
			diagnosticSummaries(diagnostics))
	}
}

// The local union relay establishes two roots: its declared formals and the
// path bound to the admitted call's result. A branch over either is decided by
// the entry, whatever family states it.

const unionRelayProvider = `local function pick(k: string): { kind: string, a: string } | { kind: string, b: number }
    if k == "a" then return { kind = "a", a = "x" } end
    return { kind = "b", b = 1 }
end
`

func unionRelayBody(condition string) string {
	return unionRelayProvider + `local function f(k: string)
    local r = pick(k)
    if ` + condition + ` then
        local v: string = r.a
        return v
    end
    return ""
end
return f
`
}

// TestUnionRelayAdmitsEveryDiscriminantFamily pins that a discriminant over the
// admitted call result is admitted by its rootedness, not by its family.
func TestUnionRelayAdmitsEveryDiscriminantFamily(t *testing.T) {
	for _, condition := range []string{
		"r.kind == k",
		`r.kind == "a"`,
		`r.kind ~= "a"`,
		"r.kind",
		"r",
		"#r.kind >= 1",
		"#k >= 1",
	} {
		diagnostics := checkSource(t, unionRelayBody(condition))
		if !strings.Contains(diagnosticSummaries(diagnostics), "type.assignment") {
			t.Errorf("discriminant %q over the admitted relay left the body dormant:\n%s",
				condition, diagnosticSummaries(diagnostics))
		}
	}
}

// TestUnionRelayRejectsForeignDiscriminant pins that a branch naming a root the
// relay never established keeps the body demand-driven.
func TestUnionRelayRejectsForeignDiscriminant(t *testing.T) {
	diagnostics := checkSource(t, unionRelayProvider+`local ready = pick("a")
local function f(k: string)
    local r = pick(k)
    if ready.kind == k then
        local v: string = r.a
        return v
    end
    return ""
end
return f
`)
	if strings.Contains(diagnosticSummaries(diagnostics), "type.assignment") {
		t.Fatalf("a discriminant rooted outside the relay admitted the body:\n%s",
			diagnosticSummaries(diagnostics))
	}
}
