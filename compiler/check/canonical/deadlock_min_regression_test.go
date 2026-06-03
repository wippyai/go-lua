package canonical_test

import "testing"

func TestDeadlockMinFalsePositiveRegression(t *testing.T) {
	clean := map[string]string{
		"ipairs-index-concat": `
local function f(definitions)
    for i, def in ipairs(definitions) do
        return "index " .. i
    end
end
return f
`,
		"pairs-key-concat-annotated-map": `
local function f(cfg: {[string]: string})
    for field_name, expr in pairs(cfg) do
        return "fail " .. field_name
    end
end
return f
`,
		"ipairs-index-concat-any-annotated": `
local function f(definitions: any)
    for i, def in ipairs(definitions) do
        return "index " .. i
    end
end
return f
`,
		"or-default-over-unresolved-call": `
	local ext = require("ext")
	local function f()
	    local ok, err = ext.submit()
	    return "fail: " .. (err or "unknown")
	end
	return f
	`,
		"method-target-field-concat-infers-contract": `
	local methods = {}
	function methods:g(target)
	    return "t[" .. target.idx .. "]"
	end
	return methods
	`,
		"method-self-field-concat-infers-contract": `
	local methods = {}
	function methods:g()
	    return "node[" .. self.id .. "]"
	end
	return methods
	`,
	}
	for name, src := range clean {
		t.Run(name, func(t *testing.T) {
			requireCanonicalClean(t, src)
		})
	}
}

func TestCanonicalStrictAnyRejectsUnprovedDeadlockMinCases(t *testing.T) {
	cases := map[string]struct {
		src  string
		want string
	}{
		"pairs-key-concat-untyped": {
			src: `
local function f(raw)
    for key, v in pairs(raw) do
        return "k " .. key
    end
end
return f
`,
			want: "cannot concatenate any",
		},
		"length-of-any-chain": {
			src: `
local function f(reader: any)
    local data = reader:with(1):all()
    return table.create(0, #data)
end
return f
`,
			want: "cannot apply length operator to any",
		},
		"pairs-key-concat-any-annotated": {
			src: `
	local function f(raw: any)
	    for key, v in pairs(raw) do
        return "k " .. key
    end
end
return f
`,
			want: "cannot concatenate any",
		},
		"ipairs-index-over-self-field": {
			src: `
	local methods = {}
function methods:g()
    for idx, t in ipairs(self.targets) do
        return "t[" .. idx .. "]"
    end
end
return methods
`,
			want: "cannot concatenate any",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			requireCanonicalDiagnosticContains(t, tc.src, tc.want)
		})
	}
}
