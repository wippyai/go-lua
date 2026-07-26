package engine_test

import (
	"strings"
	"testing"
)

// guardedElementReadSource wraps one guarded body so the guard and the
// statements between it and the read are the only difference between cases.
func guardedElementReadSource(body string) string {
	return "local function f(xs: {string}, i: number): string\n" + body + "\n    return \"\"\nend\nreturn f\n"
}

func guardedElementReadRefuted(t *testing.T, body string) (string, bool) {
	t.Helper()
	summary := diagnosticSummaries(checkSource(t, guardedElementReadSource(body)))
	return summary, strings.Contains(summary, "may be nil")
}

// TestStoreRevokesProofsMeasuredAgainstTheBorder pins the revocation the index
// proofs depend on. A store the analysis reaches can empty the slot the
// sequence border rests on, so the floor, the in-range presence proof and the
// border read all stop holding at it.
func TestStoreRevokesProofsMeasuredAgainstTheBorder(t *testing.T) {
	for _, item := range []struct{ name, body string }{
		{"border read after an unresolved-key store", `    if #xs >= 1 then
        local n = #xs
        xs[i] = nil
        local v: string = xs[n]
        return v
    end`},
		{"border read after an exact-key store", `    if #xs >= 1 then
        local n = #xs
        local k = 1
        xs[k] = nil
        local v: string = xs[n]
        return v
    end`},
		{"in-range read after an exact-key store", `    if i >= 1 and i <= #xs then
        local k = 1
        xs[k] = nil
        local v: string = xs[i]
        return v
    end`},
		{"ceiling read after an exact-key store", `    if #xs >= 3 and i >= 1 and i <= 3 then
        local k = 1
        xs[k] = nil
        local v: string = xs[i]
        return v
    end`},
	} {
		t.Run(item.name, func(t *testing.T) {
			summary, refuted := guardedElementReadRefuted(t, item.body)
			if !refuted {
				t.Fatalf("a read measured against a border a store could move was accepted:\n%s", summary)
			}
		})
	}
}

// TestBorderProofsHoldWithoutAnInterveningStore is the control the revocation
// cases are measured against: with nothing between the guard and the read, each
// route keeps the proof its guard established.
func TestBorderProofsHoldWithoutAnInterveningStore(t *testing.T) {
	for _, item := range []struct{ name, body string }{
		{"border read under a proven floor", `    if #xs >= 1 then
        local n = #xs
        local v: string = xs[n]
        return v
    end`},
		{"ceiling within a proven floor", `    if #xs >= 3 and i >= 1 and i <= 3 then
        local v: string = xs[i]
        return v
    end`},
		{"index in range of the container", `    if i >= 1 and i <= #xs then
        local v: string = xs[i]
        return v
    end`},
	} {
		t.Run(item.name, func(t *testing.T) {
			summary, refuted := guardedElementReadRefuted(t, item.body)
			if refuted {
				t.Fatalf("a proven in-range read was refuted:\n%s", summary)
			}
		})
	}
}

// TestBorderReadWithoutAFloorStaysOptional pins the other side of the border
// route: with no proven non-empty length the empty border remains available and
// the read is nil.
func TestBorderReadWithoutAFloorStaysOptional(t *testing.T) {
	summary := diagnosticSummaries(checkSource(t, `local function f(xs: {string}): string
    local n = #xs
    local v: string = xs[n]
    return v
end
return f
`))
	if !strings.Contains(summary, "may be nil") {
		t.Fatalf("a border read with no proven length floor was accepted:\n%s", summary)
	}
}

// TestGuardOverBodyLocalKeepsTheBodyLive pins the admission this lane owes. It
// refuses every occurrence that could import a caller-owned value, so a body
// local holds only what the declaration's own formals and literals produced. A
// branch over such a local is decided by the declaration exactly as a branch
// over the formal itself is, and the arm it selects publishes its obligations.
func TestGuardOverBodyLocalKeepsTheBodyLive(t *testing.T) {
	for _, item := range []struct{ name, source string }{
		{"local seeded from a literal", `local function f(xs: {string}): string
    local i: number = 0
    if i <= #xs then
        local v: string = xs[i]
        return v
    end
    return ""
end
return f
`},
		{"local seeded from a negative literal", `local function f(xs: {string}): string
    local i: number = -5
    if i <= #xs then
        local v: string = xs[i]
        return v
    end
    return ""
end
return f
`},
		{"local seeded from a formal", `local function f(xs: {string}, i: number): string
    local j = i
    if j >= 1 then
        local v: string = xs[j]
        return v
    end
    return ""
end
return f
`},
	} {
		t.Run(item.name, func(t *testing.T) {
			summary := diagnosticSummaries(checkSource(t, item.source))
			if !strings.Contains(summary, "may be nil") {
				t.Fatalf("a body whose guard names a body local published no obligation:\n%s", summary)
			}
		})
	}
}

// TestGuardOverForeignRootStaysDormant pins the limit of that admission. A
// branch over a captured cell rests on an authority the declaration-only entry
// does not establish, so the body waits for a call rather than publishing under
// a seeded guess.
func TestGuardOverForeignRootStaysDormant(t *testing.T) {
	summary := diagnosticSummaries(checkSource(t, `local mode = false
local function f(xs: {string}, i: number): string
    if mode then
        local v: string = xs[i]
        return v
    end
    return ""
end
return f
`))
	if strings.Contains(summary, "may be nil") {
		t.Fatalf("a branch over a capture published a declaration-owned refutation:\n%s", summary)
	}
}
