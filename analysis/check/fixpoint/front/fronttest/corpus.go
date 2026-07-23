package fronttest

import "sort"

// Case is one self-contained Lua program and its language-semantic contract.
// Name is also the stable identity used in failure reports.
type Case struct {
	Name   string
	Source string
	Expect Expectation
}

// Expectation is the complete set of results a case publishes.  An omitted
// slice means that channel must be empty; it never means "don't care".
type Expectation struct {
	Published   []PublishedOutcome
	Diagnostics []DiagnosticCandidate
}

// PublishedOutcome is a normalized observation of a Lua program.  Channel is
// normally "value" or "outcome", Subject identifies the source-level thing
// observed, and Value uses Lua spelling (for example nil, false, 3, or "hi").
type PublishedOutcome struct {
	Channel string
	Subject string
	Value   string
}

// DiagnosticCandidate identifies an expected diagnostic without committing the
// corpus to a renderer or a legacy diagnostic-file schema.
type DiagnosticCandidate struct {
	Code    string
	Subject string
	Detail  string
}

// StarterCorpus is a compact language-semantic corpus.  Each program is valid
// Lua except runtime-nil-arithmetic, which intentionally reaches Lua's
// arithmetic type error.  The corpus names are sorted so additions have a
// stable home in failure reports and review diffs.
func StarterCorpus() []Case {
	cases := []Case{
		{
			Name: "assignment/extra-results-are-discarded",
			Source: `
local first, second = 1, 2, 3
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("first", "1"), value("second", "2"),
			}},
		},
		{
			Name: "assignment/local-literal",
			Source: `
local answer = 42
`,
			Expect: Expectation{Published: []PublishedOutcome{value("answer", "42")}},
		},
		{
			Name: "assignment/missing-results-become-nil",
			Source: `
local first, second = "present"
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("first", `"present"`), value("second", "nil"),
			}},
		},
		{
			Name: "assignment/parallel-assignment-reads-old-values",
			Source: `
local left, right = "left", "right"
left, right = right, left
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("left", `"right"`), value("right", `"left"`),
			}},
		},
		{
			Name: "assignment/reassignment-overwrites-value",
			Source: `
local count = 1
count = 2
`,
			Expect: Expectation{Published: []PublishedOutcome{value("count", "2")}},
		},
		{
			Name: "branch/false-is-falsy",
			Source: `
local result
if false then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"else"`)}},
		},
		{
			Name: "branch/nil-is-falsy",
			Source: `
local result
if nil then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"else"`)}},
		},
		{
			Name: "branch/number-zero-is-truthy",
			Source: `
local result
if 0 then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"then"`)}},
		},
		{
			Name: "branch/only-selected-arm-writes",
			Source: `
local result = "before"
if true then
    result = "selected"
else
    result = "not-selected"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"selected"`)}},
		},
		{
			Name: "branch/empty-string-is-truthy",
			Source: `
local result
if "" then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"then"`)}},
		},
		{
			Name: "call/final-call-expands-results",
			Source: `
local function pair()
    return "left", "right"
end
local first, second, third = 1, pair()
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("first", "1"), value("second", `"left"`), value("third", `"right"`),
			}},
		},
		{
			Name: "call/function-argument-and-return",
			Source: `
local function identity(item)
    return item
end
local result = identity(7)
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "7")}},
		},
		{
			Name: "call/parenthesized-call-is-one-result",
			Source: `
local function pair()
    return "left", "right"
end
local first, second, third = 1, (pair())
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("first", "1"), value("second", `"left"`), value("third", "nil"),
			}},
		},
		{
			Name: "call/return-without-values-is-nil",
			Source: `
local function empty()
    return
end
local result = empty()
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "nil")}},
		},
		{
			Name: "call/table-method-receives-self",
			Source: `
local object = { base = 4 }
function object:add(amount)
    return self.base + amount
end
local result = object:add(3)
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "7")}},
		},
		{
			Name: "diagnostic/nil-arithmetic-raises-error",
			Source: `
local absent
local result = absent + 1
`,
			Expect: Expectation{Diagnostics: []DiagnosticCandidate{{
				Code:    "runtime/arithmetic-non-number",
				Subject: "absent + 1",
				Detail:  "attempt to perform arithmetic on nil",
			}}},
		},
		{
			Name: "loop/generic-for-uses-iterator-results",
			Source: `
local function once(_, control)
    if control == nil then
        return 1, "only"
    end
end
local result
for index, item in once, nil, nil do
    result = item
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"only"`)}},
		},
		{
			Name: "loop/numeric-for-has-inclusive-limit",
			Source: `
local result
for index = 1, 3 do
    result = index
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "3")}},
		},
		{
			Name: "loop/numeric-for-respects-step",
			Source: `
local result
for index = 5, 1, -2 do
    result = index
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "1")}},
		},
		{
			Name: "loop/repeat-runs-before-condition",
			Source: `
local count = 0
repeat
    count = count + 1
until true
`,
			Expect: Expectation{Published: []PublishedOutcome{value("count", "1")}},
		},
		{
			Name: "loop/while-false-skips-body",
			Source: `
local result = "before"
while false do
    result = "body"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"before"`)}},
		},
	}
	for index := range cases {
		cases[index].Expect.Published = canonicalPublished(cases[index].Expect.Published)
		cases[index].Expect.Diagnostics = canonicalDiagnostics(cases[index].Expect.Diagnostics)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
	return cases
}

func value(subject, result string) PublishedOutcome {
	return PublishedOutcome{Channel: "value", Subject: subject, Value: result}
}
