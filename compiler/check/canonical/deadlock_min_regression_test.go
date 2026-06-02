package canonical_test

import "testing"

func TestDeadlockMinFalsePositiveRegression(t *testing.T) {
	cases := map[string]string{
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
		"pairs-key-concat-untyped": `
local function f(raw)
    for key, v in pairs(raw) do
        return "k " .. key
    end
end
return f
`,
		"length-of-any-chain": `
local function f(reader: any)
    local data = reader:with(1):all()
    return table.create(0, #data)
end
return f
`,
		"field-concat-untyped-self": `
local methods = {}
function methods:g(target)
    return "t[" .. target.idx .. "]"
end
return methods
`,
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
		"self-field-concat": `
local methods = {}
function methods:g()
    return "node[" .. self.id .. "]"
end
return methods
`,
		"or-default-over-unresolved-call": `
local ext = require("ext")
local function f()
    local ok, err = ext.submit()
    return "fail: " .. (err or "unknown")
end
return f
`,
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
			requireCanonicalClean(t, src)
		})
	}
}
