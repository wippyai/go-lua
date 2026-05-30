package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZDeadlockMinProbe isolates the false-positive value-domain patterns from the
// deadlock-* regression fixtures into minimal forms so the canonical flow's
// unknown-vs-gradual decision is unambiguous. Each case is curated-clean (the
// fixtures expect errors:0); any DIAG logged here is a canonical false positive.
// Diagnostic probe for the cutover-gate investigation.
func TestZZDeadlockMinProbe(t *testing.T) {
	cases := map[string]string{
		// ipairs index over an unannotated param: i should be integer, concat clean.
		"ipairs-index-concat": `
local function f(definitions)
    for i, def in ipairs(definitions) do
        return "index " .. i
    end
end
return f
`,
		// pairs key over an annotated map: field_name should be string.
		"pairs-key-concat-annotated-map": `
local function f(cfg: {[string]: string})
    for field_name, expr in pairs(cfg) do
        return "fail " .. field_name
    end
end
return f
`,
		// pairs key over an unannotated param: key should be gradual, concat clean.
		"pairs-key-concat-untyped": `
local function f(raw)
    for key, v in pairs(raw) do
        return "k " .. key
    end
end
return f
`,
		// length over a value from an :: any method chain result.
		"length-of-any-chain": `
local function f(reader: any)
    local data = reader:with(1):all()
    return table.create(0, #data)
end
return f
`,
		// concat of an unannotated-field access (self deps chain).
		"field-concat-untyped-self": `
local methods = {}
function methods:g(target)
    return "t[" .. target.idx .. "]"
end
return methods
`,
		// CONTROL: explicit `any` annotation on the iterated param. If this is clean
		// but the unannotated form errors, the gap is the unannotated->any fallback in
		// the iterator-source exprType closure, NOT IterVars/ElementType on any.
		"ipairs-index-concat-any-annotated": `
local function f(definitions: any)
    for i, def in ipairs(definitions) do
        return "index " .. i
    end
end
return f
`,
		"pairs-key-concat-any-annotated": `
local function f(raw: any)
    for key, v in pairs(raw) do
        return "k " .. key
    end
end
return f
`,
		// field read off an unannotated method self, then concat: self.id is gradual.
		"self-field-concat": `
local methods = {}
function methods:g()
    return "node[" .. self.id .. "]"
end
return methods
`,
		// `or "default"` over an unresolved-call local: err is unknown from an
		// unresolved require; curated truth treats it as gradual.
		"or-default-over-unresolved-call": `
local ext = require("ext")
local function f()
    local ok, err = ext.submit()
    return "fail: " .. (err or "unknown")
end
return f
`,
		// ipairs index over a field of unannotated self (target_idx pattern).
		"ipairs-index-over-self-field": `
local methods = {}
function methods:g()
    for idx, t in ipairs(self.targets) do
        return "t[" .. idx .. "]"
    end
end
return methods
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
			n := 0
			for _, m := range testutil.ErrorMessages(res.Diagnostics) {
				t.Logf("DIAG: %s", m)
				n++
			}
			if n == 0 {
				t.Logf("CLEAN (no false positive)")
			}
		})
	}
}
