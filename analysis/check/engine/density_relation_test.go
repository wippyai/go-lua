package engine_test

import (
	"strings"
	"testing"
)

// densityCollectorSource wraps one counter/sequence body so the loop shape and
// the statements around it are the only difference between cases. The read is
// always buf[1] under the same n >= 1 guard.
func densityCollectorSource(body string) string {
	return "local function collect(src: {string}): string\n" + body + `
    if n >= 1 then
        local first: string = buf[1]
        return first
    end
    return ""
end
return collect
`
}

func densityReadRefuted(t *testing.T, body string) (string, bool) {
	t.Helper()
	summary := diagnosticSummaries(checkSource(t, densityCollectorSource(body)))
	return summary, strings.Contains(summary, "may be nil")
}

// TestPairedCounterAndAppendProveTheWrittenPrefixIsDense pins the relation the
// front discharges inductively: the counter enters the loop at zero beside an
// empty constructor, and every trip raises it by one and writes exactly the
// slot it names. A post-loop n >= 1 therefore proves buf holds slot 1.
func TestPairedCounterAndAppendProveTheWrittenPrefixIsDense(t *testing.T) {
	summary, refuted := densityReadRefuted(t, `    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end`)
	if refuted {
		t.Fatalf("a read inside the counted prefix of a densely filled sequence was refused:\n%s", summary)
	}
}

// TestDensityRelationWithdrawnByAnUnpairedWrite pins the withdrawal side. The
// proof is over the body's complete write set, so any write that breaks the
// step of one, the seed of zero, or the pairing of increment and append leaves
// the count stating nothing about the sequence's length.
func TestDensityRelationWithdrawnByAnUnpairedWrite(t *testing.T) {
	for _, item := range []struct{ name, body string }{
		{"conditional append", `    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        if s ~= "" then
            buf[n] = s
        end
    end`},
		{"step of two", `    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 2
        buf[n] = s
    end`},
		{"counter seeded above zero", `    local n: integer = 1
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end`},
		{"counter raised outside the pair", `    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end
    n = n + 1`},
		{"second append outside the pair", `    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end
    buf[7] = "extra"`},
		{"append of nil", `    local n: integer = 0
    local buf: {string?} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = nil
    end`},
		{"counter handed to a closure", `    local n: integer = 0
    local buf: {string} = {}
    local bump = function() n = n + 5 end
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end
    bump()`},
	} {
		t.Run(item.name, func(t *testing.T) {
			summary, refuted := densityReadRefuted(t, item.body)
			if !refuted {
				t.Fatalf("a read was admitted against a count that no longer states the sequence's length:\n%s", summary)
			}
		})
	}
}

// TestDensityRelationStopsAtTheLastWrittenSlot pins the relation's reach. The
// count equals the length, so the slot one past it is the slot the next trip
// would have filled and no proof reaches it.
func TestDensityRelationStopsAtTheLastWrittenSlot(t *testing.T) {
	summary := diagnosticSummaries(checkSource(t, `local function past_last(src: {string}): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end

    if n >= 1 then
        local beyond: string = buf[n + 1]
        return beyond
    end
    return ""
end
return past_last
`))
	if !strings.Contains(summary, "may be nil") {
		t.Fatalf("a read one past the counted prefix was admitted:\n%s", summary)
	}
}

// TestDeclaredSequenceIndexKeepsTheMissingSlotNil pins the occupancy split. A
// declared array types every occupied slot, and a read whose occupancy nothing
// proves keeps Lua's missing-slot nil rather than the bare element.
func TestDeclaredSequenceIndexKeepsTheMissingSlotNil(t *testing.T) {
	summary := diagnosticSummaries(checkSource(t, `local function pick(src: {string}): string
    local buf: {string} = {}
    for _, s in ipairs(src) do
        buf[#buf + 1] = s
    end
    local first: string = buf[1]
    return first
end
return pick
`))
	if !strings.Contains(summary, "may be nil") {
		t.Fatalf("a declared array slot with no occupancy proof was read as its bare element:\n%s", summary)
	}
}
