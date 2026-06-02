package canonical_test

import "testing"

func TestParameterNarrowingRegression(t *testing.T) {
	cases := map[string]string{
		"not-nil-param": `
local function not_nil(val: any)
    if val == nil then
        error("nil")
    end
end
local function f(x: string?): string
    not_nil(x)
    return x
end
return f
`,
		"is-nil-param": `
local function is_nil(val: any)
    if val ~= nil then
        error("expected nil")
    end
end
local function f(x: string?): nil
    is_nil(x)
    return x
end
return f
`,
		"is-nil-msg-param": `
local function is_nil(val: any, msg: string?)
    if val ~= nil then
        error(msg or "expected nil", 2)
    end
end
local function f(x: string?): nil
    is_nil(x, "m")
    return x
end
return f
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			requireCanonicalClean(t, src)
		})
	}
}
