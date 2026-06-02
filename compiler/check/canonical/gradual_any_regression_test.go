package canonical_test

import "testing"

func TestGradualAnyCompatibilityRegression(t *testing.T) {
	cases := map[string]string{
		"field-of-unannotated-param": `
local function decode(s: string): any
    return {}
end
local function parse(resp)
    return decode(resp.body)
end
return parse
`,
		"resolved-callee-unannotated-arg": `
local json = {}
function json.decode(source: string): (any, string?)
    return {}, nil
end
local function parse(http_response)
    return json.decode(http_response.body)
end
return parse
`,
		"assign-field-any-to-string": `
local function get(page)
    local name: string = page.data_func
    return name
end
return get
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			requireCanonicalClean(t, src)
		})
	}
}
