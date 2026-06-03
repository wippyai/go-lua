package canonical_test

import "testing"

func TestCanonicalInferredBodyPreconditionKeepsLocalUseClean(t *testing.T) {
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
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			requireCanonicalClean(t, src)
		})
	}
}

func TestCanonicalStrictAnyRequiresProofAtConcreteBoundaries(t *testing.T) {
	cases := map[string]struct {
		src  string
		want string
	}{
		"assign-field-any-to-string": {
			src: `
local function get(page)
    local name: string = page.data_func
    return name
end
return get
`,
			want: "cannot assign any to string",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			requireCanonicalDiagnosticContains(t, tc.src, tc.want)
		})
	}
}

func TestCanonicalInferredBodyPreconditionRejectsUnprovedCaller(t *testing.T) {
	cases := map[string]struct {
		src  string
		want string
	}{
		"field-of-unannotated-param": {
			src: `
local function decode(s: string): any
    return {}
end
local function parse(resp)
    return decode(resp.body)
end
return parse({body = 1})
`,
			want: "argument 1",
		},
		"resolved-callee-unannotated-arg": {
			src: `
local json = {}
function json.decode(source: string): (any, string?)
    return {}, nil
end
local function parse(http_response)
    return json.decode(http_response.body)
end
return parse({body = 1})
`,
			want: "argument 1",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			requireCanonicalDiagnosticContains(t, tc.src, tc.want)
		})
	}
}
