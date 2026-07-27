package front_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// densityRelationStated reports whether any branch of this body carries a
// proven counter/sequence pair.
func densityRelationStated(t *testing.T, source string) bool {
	t.Helper()
	artifact, err := front.CompileBody(source)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, operation := range artifact.Equations {
		for _, operand := range operation.Operands {
			if operand.Role.InFamily(equation.RoleFamilyDensityRelation) {
				return true
			}
		}
	}
	return false
}

// TestDensityCarrierStatedForPairedCounterAndAppend pins the inductive proof.
// The counter enters the cycle at zero beside an empty constructor, and the
// single increment and the single append at the slot it names share the cycle's
// guards, so every arrival back at the header preserves `counter == #container`.
func TestDensityCarrierStatedForPairedCounterAndAppend(t *testing.T) {
	if !densityRelationStated(t, `
local src = { "a", "b" }
local n = 0
local buf = {}
for _, s in ipairs(src) do
    n = n + 1
    buf[n] = s
end
if n >= 1 then
    local first = buf[1]
end
`) {
		t.Fatal("a counter paired with its own append stated no density relation")
	}
}

// TestDensityCarrierWithheldOutsideTheFragment pins the withdrawal side. The
// proof reads the body's complete write set, so a step other than one, a seed
// other than zero, a constructor that already holds entries, an append the
// increment does not share guards with, an append that reads the count before
// it is raised, a write to either half outside the pair, and a capture handed
// to a closure each leave the pair unproven.
func TestDensityCarrierWithheldOutsideTheFragment(t *testing.T) {
	for _, item := range []struct{ name, body string }{
		{"conditional append", `
    n = n + 1
    if s ~= "" then
        buf[n] = s
    end`},
		{"step of two", `
    n = n + 2
    buf[n] = s`},
		{"append at another path", `
    n = n + 1
    buf[#buf] = s`},
		{"append before the increment", `
    buf[n] = s
    n = n + 1`},
		{"append through a suffix", `
    n = n + 1
    buf[n].field = s`},
		{"append of nil", `
    n = n + 1
    buf[n] = nil`},
	} {
		t.Run(item.name, func(t *testing.T) {
			if densityRelationStated(t, `
local src = { "a", "b" }
local n = 0
local buf = {}
for _, s in ipairs(src) do`+item.body+`
end
if n >= 1 then
    local first = buf[1]
end
`) {
				t.Fatal("a write outside the paired fragment still stated a density relation")
			}
		})
	}
}

// TestDensityCarrierWithheldOutsideTheCycle pins the seed and post-loop halves
// of the same write set: a second seed, a constructor with entries, a raise of
// the counter after the loop, and an append at a slot the count does not name
// all leave the relation unproven.
func TestDensityCarrierWithheldOutsideTheCycle(t *testing.T) {
	for _, item := range []struct{ name, source string }{
		{"counter seeded above zero", `
local src = { "a" }
local n = 1
local buf = {}
for _, s in ipairs(src) do
    n = n + 1
    buf[n] = s
end
if n >= 1 then
    local first = buf[1]
end
`},
		{"constructor seeded with an entry", `
local src = { "a" }
local n = 0
local buf = { "seed" }
for _, s in ipairs(src) do
    n = n + 1
    buf[n] = s
end
if n >= 1 then
    local first = buf[1]
end
`},
		{"counter raised after the loop", `
local src = { "a" }
local n = 0
local buf = {}
for _, s in ipairs(src) do
    n = n + 1
    buf[n] = s
end
n = n + 1
if n >= 1 then
    local first = buf[1]
end
`},
		{"second append after the loop", `
local src = { "a" }
local n = 0
local buf = {}
local at = 7
for _, s in ipairs(src) do
    n = n + 1
    buf[n] = s
end
buf[at] = "extra"
if n >= 1 then
    local first = buf[1]
end
`},
		{"counter handed to a closure", `
local src = { "a" }
local n = 0
local buf = {}
local bump = function() n = n + 5 end
for _, s in ipairs(src) do
    n = n + 1
    buf[n] = s
end
bump()
if n >= 1 then
    local first = buf[1]
end
`},
	} {
		t.Run(item.name, func(t *testing.T) {
			if densityRelationStated(t, item.source) {
				t.Fatal("a write outside the paired fragment still stated a density relation")
			}
		})
	}
}

// TestDensityCarrierWithheldBeforeItsSeeds pins the relation's reach. A branch
// the counter's seed does not precede reaches a half this body has not yet
// bound, so the pair states nothing there.
func TestDensityCarrierWithheldBeforeItsSeeds(t *testing.T) {
	artifact, err := front.CompileBody(`
local src = { "a" }
local flag = true
local buf = {}
if flag then
    local early = buf
end
local n = 0
for _, s in ipairs(src) do
    n = n + 1
    buf[n] = s
end
if n >= 1 then
    local first = buf[1]
end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	stated, withheld := 0, 0
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "branch-relations" {
			continue
		}
		carried := false
		for _, operand := range operation.Operands {
			carried = carried || operand.Role.InFamily(equation.RoleFamilyDensityRelation)
		}
		if carried {
			stated++
		} else {
			withheld++
		}
	}
	if stated == 0 || withheld == 0 {
		t.Fatalf("expected the pair to reach the branches past its seeds and no earlier one: stated=%d withheld=%d", stated, withheld)
	}
}
