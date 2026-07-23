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
			Name:   "adversarial/coercion-arithmetic-string-number-is-conservative",
			Source: `local result = "12" + 3`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "unknown")}},
		},
		{
			Name:   "adversarial/coercion-concat-number-string-is-lua-string",
			Source: `local result = 12 .. "3"`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"123"`)}},
		},
		{
			Name:   "adversarial/coercion-concat-string-number-is-lua-string",
			Source: `local result = "id-" .. 4`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"id-4"`)}},
		},
		{
			Name:   "adversarial/coercion-subtraction-string-number-is-conservative",
			Source: `local result = "12" - 3`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "unknown")}},
		},
		{
			Name: "adversarial/metatable-index-chain-read-is-conservative",
			Source: `
local fallback = { answer = 42 }
local middle = setmetatable({}, { __index = fallback })
local object = setmetatable({}, { __index = middle })
local result = object.answer
`,
			Expect: Expectation{Published: []PublishedOutcome{value("middle", "unknown"), value("object", "unknown"), value("result", "unknown")}},
		},
		{
			Name: "adversarial/metatable-index-read-is-conservative",
			Source: `
local fallback = { answer = 42 }
local object = setmetatable({}, { __index = fallback })
local result = object.answer
`,
			Expect: Expectation{Published: []PublishedOutcome{value("object", "unknown"), value("result", "unknown")}},
		},
		{
			Name: "adversarial/metatable-newindex-write-routing-is-conservative",
			Source: `
local sink = {}
local object = setmetatable({}, { __newindex = sink })
object.answer = 42
local result = sink.answer
`,
			Expect: Expectation{Published: []PublishedOutcome{value("object", "unknown"), value("result", "unknown")}},
		},
		{
			Name: "adversarial/multi-return-last-call-expands-assignment-tail-conservatively",
			Source: `
local function f() return 1, 2 end
local first, second, third = 0, f()
`,
			Expect: Expectation{Published: []PublishedOutcome{value("first", "0"), value("second", "unknown"), value("third", "unknown")}},
		},
		{
			Name:   "adversarial/multi-return-last-provider-call-expands-assignment-tail",
			Source: `local first, second, third = 0, string.find("abc", "b")`,
			Expect: Expectation{Published: []PublishedOutcome{value("first", "0"), value("second", "unknown"), value("third", "unknown")}},
		},
		{
			Name: "adversarial/multi-return-nonfinal-call-truncates-assignment-list-conservatively",
			Source: `
local function f() return 1, 2 end
local first, second = f(), 3
`,
			Expect: Expectation{Published: []PublishedOutcome{value("first", "unknown"), value("second", "3")}},
		},
		{
			Name:   "adversarial/multi-return-nonfinal-provider-call-truncates-assignment-list",
			Source: `local first, second = string.find("abc", "b"), 3`,
			Expect: Expectation{Published: []PublishedOutcome{value("first", "unknown"), value("second", "3")}},
		},
		{
			Name: "adversarial/multi-return-parenthesized-call-truncates-to-one-conservatively",
			Source: `
local function f() return 1, 2 end
local first, second = (f()), 3
`,
			Expect: Expectation{Published: []PublishedOutcome{value("first", "unknown"), value("second", "3")}},
		},
		{
			Name:   "adversarial/multi-return-parenthesized-provider-call-truncates-to-one",
			Source: `local first, second = (string.find("abc", "b")), 3`,
			Expect: Expectation{Published: []PublishedOutcome{value("first", "unknown"), value("second", "3")}},
		},
		{
			Name: "adversarial/vararg-boundary-counts-nil-and-false-conservatively",
			Source: `
local function count(...) return select("#", ...) end
local result = count(nil, false, 3)
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "unknown")}},
		},
		{
			Name: "adversarial/vararg-boundary-empty-list-is-conservative",
			Source: `
local function count(...) return select("#", ...) end
local result = count()
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "unknown")}},
		},
		{
			Name: "adversarial/vararg-forwarding-through-function-boundary-is-conservative",
			Source: `
local function count(...) return select("#", ...) end
local function forward(...) return count(...) end
local result = forward(nil, false, 3)
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "unknown")}},
		},
		{
			Name: "adversarial/vararg-parenthesized-forwarding-truncates-conservatively",
			Source: `
local function first(...) return (...) end
local result = first(nil, false, 3)
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "unknown")}},
		},
		{
			Name: "adversarial/vararg-select-preserves-lua-arity-through-boundary-conservatively",
			Source: `
local function relay(...) return select("#", ...) end
local result = relay("one", nil, "three")
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "unknown")}},
		},
		{
			Name: "allocation/array-constructor-has-contiguous-prefix",
			Source: `
local values = { "first", "second" }
local result = values[2]
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"second"`)}},
		},
		{
			Name: "allocation/closure-captures-the-current-binding",
			Source: `
local captured = "before"
local read = function() return captured end
captured = "after"
local result = read()
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"after"`)}},
		},
		{
			Name: "allocation/constructor-closure-is-truthy",
			Source: `
local callback = function() end
local result = "before"
if callback then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"then"`)}},
		},
		{
			Name: "allocation/constructor-nested-member-has-completed-write",
			Source: `
local object = { child = { answer = 42 } }
local result = object.child.answer
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "42")}},
		},
		{
			Name: "allocation/constructor-static-member-has-completed-write",
			Source: `
local object = { answer = 42 }
local result = object.answer
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "42")}},
		},
		{
			Name: "allocation/constructor-table-is-truthy",
			Source: `
local object = {}
local result = "before"
if object then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"then"`)}},
		},
		{
			Name: "allocation/dynamic-key-materializes-the-runtime-key",
			Source: `
local key = "selected"
local object = { [key] = 9 }
local result = object.selected
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "9")}},
		},
		{
			Name: "allocation/empty-table-member-is-absent",
			Source: `
local object = {}
local result = object.missing
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "nil")}},
		},
		{
			Name: "allocation/false-member-remains-present",
			Source: `
local object = { enabled = false }
local result = object.enabled
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "false")}},
		},
		{
			Name: "allocation/fresh-table-literals-are-distinct",
			Source: `
local left = {}
local right = {}
local result = left == right
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "false")}},
		},
		{
			Name: "allocation/nested-table-materializes-its-own-object",
			Source: `
local outer = { child = { answer = 42 } }
local result = outer.child.answer
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "42")}},
		},
		{
			Name: "allocation/nil-field-is-absent-not-bottom",
			Source: `
local object = { missing = nil }
local result = object.missing
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "nil")}},
		},
		{
			Name: "assignment/empty-declaration-is-nil",
			Source: `
local first, second
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("first", "nil"), value("second", "nil"),
			}},
		},
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
			Name: "assignment/explicit-nil-is-a-value",
			Source: `
local value = nil
`,
			Expect: Expectation{Published: []PublishedOutcome{value("value", "nil")}},
		},
		{
			Name: "assignment/false-is-a-value",
			Source: `
local disabled = false
`,
			Expect: Expectation{Published: []PublishedOutcome{value("disabled", "false")}},
		},
		{
			Name: "assignment/implicit-global-read-is-nil",
			Source: `
local value = missing_global
`,
			Expect: Expectation{Published: []PublishedOutcome{value("value", "nil")}},
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
			Name: "assignment/parallel-assignment-keeps-all-old-values",
			Source: `
local first, second, third = 1, 2, 3
first, second, third = third, first, second
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("first", "3"), value("second", "1"), value("third", "2"),
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
			Name: "assignment/parallel-assignment-snapshots-path-before-literal-write",
			Source: `
local left, right = 1, 2
left, right = 9, left
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("left", "9"), value("right", "1"),
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
			Name: "assignment/reassignment-reads-prior-write",
			Source: `
local original = 1
original = 2
local copied = original
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("original", "2"), value("copied", "2"),
			}},
		},
		{
			Name: "assignment/string-values-use-lua-spelling",
			Source: `
local greeting = "hello"
`,
			Expect: Expectation{Published: []PublishedOutcome{value("greeting", `"hello"`)}},
		},
		{
			Name: "assignment/unmaterialized-member-read-is-unknown",
			Source: `
local record = provider()
local name = record.name
`,
			Expect: Expectation{Published: []PublishedOutcome{value("name", "unknown"), value("record", "unknown")}},
		},
		{
			Name: "assignment/unmaterialized-member-read-through-alias-is-unknown",
			Source: `
local record = provider()
local alias = record
local name = alias.name
`,
			Expect: Expectation{Published: []PublishedOutcome{value("alias", "unknown"), value("name", "unknown"), value("record", "unknown")}},
		},
		{
			Name: "assignment/unmaterialized-nested-member-read-is-unknown",
			Source: `
local record = provider()
local name = record.profile.name
`,
			Expect: Expectation{Published: []PublishedOutcome{value("name", "unknown"), value("record", "unknown")}},
		},
		{
			Name: "expression/unmaterialized-member-arithmetic-is-unknown",
			Source: `
local record = provider()
local count = record.count + 1
`,
			Expect: Expectation{Published: []PublishedOutcome{value("count", "unknown"), value("record", "unknown")}},
		},
		{
			Name: "expression/unmaterialized-member-concat-is-unknown",
			Source: `
local record = provider()
local label = record.label .. "!"
`,
			Expect: Expectation{Published: []PublishedOutcome{value("label", "unknown"), value("record", "unknown")}},
		},
		{
			Name: "outcome/unmaterialized-member-return-is-unknown",
			Source: `
local record = provider()
return record.enabled
`,
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "1"), outcome("return/0", "unknown"), value("record", "unknown")}},
		},
		{
			Name:   "expression/arithmetic-precedence-publishes-number",
			Source: `local result = 1 + 2 * 3`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "7")}},
		},
		{
			Name:   "expression/concat-coerces-numbers-left-to-right",
			Source: `local result = "id-" .. 4 .. "-ok"`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"id-4-ok"`)}},
		},
		{
			Name:   "expression/logical-uses-lua-truth-and-value-flow",
			Source: `local left = false and "no"; local right = nil or 0`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("left", "false"), value("right", "0"),
			}},
		},
		{
			Name:   "expression/unary-not-and-length-use-lua-values",
			Source: `local negated = not 0; local length = #"hello"`,
			Expect: Expectation{Published: []PublishedOutcome{value("negated", "false"), value("length", "5")}},
		},
		{
			Name: "branch/absent-global-is-nil-at-runtime",
			Source: `
local result
if definitely_absent_global then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"else"`)}},
		},
		{
			Name: "branch/unknown-member-truthiness-selects-no-arm",
			Source: `
local record = provider()
local result
if record.enabled then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("record", "unknown"), value("result", "nil")}},
		},
		{
			Name: "branch/unknown-member-type-test-selects-no-arm",
			Source: `
local record = provider()
local result
if type(record.enabled) == "string" then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("record", "unknown"), value("result", "nil")}},
		},
		{
			Name: "branch/unproven-claim-truthiness-selects-no-arm",
			Source: `
local raw = provider()
local value = raw :: string
local result
if value then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{
				Published:   []PublishedOutcome{value("raw", "unknown"), value("result", "nil"), value("value", "unknown")},
				Diagnostics: []DiagnosticCandidate{{Code: "claim/unproven", Detail: `claim "string" is not proven`}},
			},
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
			Name: "branch/literal-equality-selects-matching-arm",
			Source: `
local status = "ready"
local result
if status == "ready" then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"then"`)}},
		},
		{
			Name: "branch/literal-inequality-selects-matching-arm",
			Source: `
local status = "ready"
local result
if status ~= "closed" then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"then"`)}},
		},
		{
			Name: "branch/nested-guards-require-every-selected-edge",
			Source: `
local outer = true
local inner = false
local result = "before"
if outer then
    if inner then
        result = "wrong"
    else
        result = "nested-else"
    end
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"nested-else"`)}},
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
			Name: "branch/not-flips-truthiness",
			Source: `
local result
if not false then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"then"`)}},
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
			Name: "branch/path-equality-selects-matching-arm",
			Source: `
local left = 7
local right = 7
local result
if left == right then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"then"`)}},
		},
		{
			Name: "branch/path-inequality-selects-matching-arm",
			Source: `
local left = 7
local right = 8
local result
if left ~= right then
    result = "then"
else
    result = "else"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"then"`)}},
		},
		{
			Name: "branch/uninitialized-local-is-nil",
			Source: `
local absent
local result
if absent == nil then
    result = "nil"
else
    result = "not-nil"
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"nil"`)}},
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
			Name: "call/empty-argument-list-is-valid",
			Source: `
local function answer()
    return 42
end
local result = answer()
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "42")}},
		},
		{
			Name: "call/false-result-is-not-absent",
			Source: `
local function disabled()
    return false
end
local result = disabled()
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "false")}},
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
			Name: "call/missing-result-slot-is-nil",
			Source: `
local function one()
    return "only"
end
local first, second = one()
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("first", `"only"`), value("second", "nil"),
			}},
		},
		{
			Name: "call/multret-tail-forwards-through-wrapper",
			Source: `
local function pair()
    return "left", "right"
end
local function forward(...)
    return ...
end
local first, second = forward(pair())
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("first", `"left"`), value("second", `"right"`),
			}},
		},
		{
			Name: "call/nested-call-argument-evaluates-first",
			Source: `
local function addOne(value)
    return value + 1
end
local function double(value)
    return value * 2
end
local result = double(addOne(3))
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "8")}},
		},
		{
			Name: "call/nil-result-is-not-absent",
			Source: `
local function none()
    return nil
end
local result = none()
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "nil")}},
		},
		{
			Name: "call/nonfinal-call-is-adjusted-to-one-result",
			Source: `
local function pair()
    return "first", "second"
end
local first, second = pair(), "tail"
`,
			Expect: Expectation{Published: []PublishedOutcome{
				value("first", `"first"`), value("second", `"tail"`),
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
			Name: "call/statement-call-discards-results",
			Source: `
local observed = "before"
local function writeAndReturn()
    observed = "written"
    return "discarded"
end
writeAndReturn()
`,
			Expect: Expectation{Published: []PublishedOutcome{value("observed", `"written"`)}},
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
			Name: "channel-select/default-has-no-receive-payload",
			Source: `
local inbox: Channel<string>
local selected = channel.select { inbox:case_receive(), default = true }
local payload = selected.value
`,
			// A default selection is not a receive: its value field is Lua nil.
			// This is a concrete nil, not an absent channel-select payload fact.
			Expect: Expectation{Published: []PublishedOutcome{value("payload", "nil")}},
		},
		{
			Name: "channel-select/default-is-explicitly-marked",
			Source: `
local inbox: Channel<string>
local selected = channel.select { inbox:case_receive(), default = true }
local is_default = selected.default
`,
			Expect: Expectation{Published: []PublishedOutcome{value("is_default", "true")}},
		},
		{
			Name: "channel-select/default-keeps-case-channel-absent",
			Source: `
local inbox: Channel<string>
local selected = channel.select { inbox:case_receive(), default = true }
local no_payload = selected.value == nil
`,
			// No receive case was selected. The observable result carries Lua nil
			// rather than a fabricated Bottom or unknown payload.
			Expect: Expectation{Published: []PublishedOutcome{value("no_payload", "true")}},
		},
		{
			Name: "channel-select/multiple-cases-retain-source-order",
			Source: `
local first: Channel<string>
local second: Channel<string>
local selected = channel.select { first:case_receive(), second:case_receive() }
`,
			// Readiness is unknown, so Lua permits either case; this corpus does
			// not fabricate a selected payload.
			Expect: Expectation{},
		},
		{
			Name:   "claim/annotation-mismatch-is-diagnostic-refinement",
			Source: `local value: string = 1`,
			Expect: Expectation{
				Published:   []PublishedOutcome{value("value", "unknown")},
				Diagnostics: []DiagnosticCandidate{{Code: "claim/unproven", Subject: "", Detail: `claim "string" is not proven`}},
			},
		},
		{
			Name: "claim/distinct-annotation-mismatches-retain-both-diagnostics",
			Source: `
local text: string = 1
local count: number = "one"
`,
			Expect: Expectation{
				Published: []PublishedOutcome{value("count", "unknown"), value("text", "unknown")},
				Diagnostics: []DiagnosticCandidate{
					{Code: "claim/unproven", Detail: `claim "number" is not proven`},
					{Code: "claim/unproven", Detail: `claim "string" is not proven`},
				},
			},
		},
		{
			Name: "claim/distinct-casts-of-unknown-retain-both-diagnostics",
			Source: `
local text = provider() :: string
local count = provider() :: number
`,
			Expect: Expectation{
				Published: []PublishedOutcome{value("count", "unknown"), value("text", "unknown")},
				Diagnostics: []DiagnosticCandidate{
					{Code: "claim/unproven", Detail: `claim "number" is not proven`},
					{Code: "claim/unproven", Detail: `claim "string" is not proven`},
				},
			},
		},
		{
			Name: "claim/non-nil-and-annotation-mismatches-retain-both-diagnostics",
			Source: `
local source = nil
local required = source!
local label: string = 1
`,
			Expect: Expectation{
				Published: []PublishedOutcome{value("label", "unknown"), value("required", "unknown"), value("source", "nil")},
				Diagnostics: []DiagnosticCandidate{
					{Code: "claim/unproven", Detail: `claim "string" is not proven`},
					{Code: "claim/unproven", Detail: "claim non-nil is not proven"},
				},
			},
		},
		{
			Name:   "claim/annotation-proven-literal-keeps-lua-value",
			Source: `local value: string = "ready"`,
			Expect: Expectation{Published: []PublishedOutcome{value("value", `"ready"`)}},
		},
		{
			Name:   "claim/cast-of-unknown-is-diagnostic-refinement",
			Source: `local value = provider() :: string`,
			Expect: Expectation{
				Published:   []PublishedOutcome{value("value", "unknown")},
				Diagnostics: []DiagnosticCandidate{{Code: "claim/unproven", Subject: "", Detail: `claim "string" is not proven`}},
			},
		},
		{
			Name: "claim/non-nil-assertion-of-nil-is-diagnostic-refinement",
			Source: `
local source = nil
local value = source!
`,
			Expect: Expectation{
				Published:   []PublishedOutcome{value("source", "nil"), value("value", "unknown")},
				Diagnostics: []DiagnosticCandidate{{Code: "claim/unproven", Subject: "", Detail: "claim non-nil is not proven"}},
			},
		},
		{
			Name: "channel-select/no-default-may-block-without-publication",
			Source: `
local inbox: Channel<string>
local selected = channel.select { inbox:case_receive() }
`,
			Expect: Expectation{},
		},
		{
			Name: "channel-select/receive-payload-is-unknown-until-a-case-wins",
			Source: `
local inbox: Channel<string>
local selected = channel.select { inbox:case_receive() }
`,
			// Unknown readiness is neither a nil payload nor Bottom.  It produces
			// no concrete observation before a case is chosen.
			Expect: Expectation{},
		},
		{
			Name: "channel-select/receive-result-keeps-ok-distinct-from-value",
			Source: `
local inbox: Channel<string>
local selected = channel.select { inbox:case_receive() }
`,
			Expect: Expectation{},
		},
		{
			Name: "channel-select/zero-cases-with-default-is-nonblocking",
			Source: `
local selected = channel.select { default = true }
local payload = selected.value
`,
			Expect: Expectation{Published: []PublishedOutcome{value("payload", "nil")}},
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
			Name: "loop/generic-for-break-stops-after-first-result",
			Source: `
local function count(_, control)
    if control == nil then return 1 end
    return control + 1
end
local result = 0
for index in count, nil, nil do
    result = index
    break
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "1")}},
		},
		{
			Name: "loop/generic-for-control-uses-first-result",
			Source: `
local function count(_, control)
    if control == nil then return 1 end
    if control < 3 then return control + 1 end
end
local result = 0
for index in count, nil, nil do
    result = index
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "3")}},
		},
		{
			Name: "loop/generic-for-extra-sources-are-discarded",
			Source: `
local function once(state, control)
    if state == "state" and control == "control" then return 1 end
end
local result = 0
for index in once, "state", "control", "ignored" do
    result = index
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "1")}},
		},
		{
			Name: "loop/generic-for-false-first-result-continues",
			Source: `
local function once(_, control)
    if control == nil then return false, "value" end
end
local result = "before"
for key, item in once, nil, nil do
    result = item
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"value"`)}},
		},
		{
			Name: "loop/generic-for-missing-state-and-control-are-nil",
			Source: `
local function once(state, control)
    if state == nil and control == nil then return "key", "value" end
end
local result = "before"
for key, item in once do
    result = item
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"value"`)}},
		},
		{
			Name: "loop/generic-for-missing-secondary-results-become-nil",
			Source: `
local function once(_, control)
    if control == nil then return "key" end
end
local result = "before"
for key, item in once, nil, nil do
    result = item
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "nil")}},
		},
		{
			Name: "loop/generic-for-nil-first-result-stops",
			Source: `
local function none()
    return nil, "ignored"
end
local result = "before"
for key, item in none, nil, nil do
    result = item
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"before"`)}},
		},
		{
			Name: "loop/generic-for-nil-secondary-result-is-bound",
			Source: `
local function once(_, control)
    if control == nil then return "key", nil end
end
local result = "before"
for key, item in once, nil, nil do
    result = item
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "nil")}},
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
		{
			Name: "loop/while-accumulates-until-its-bound",
			Source: `
local total = 0
local current = 0
while current < 3 do
    total = total + current
    current = current + 1
end
`,
			Expect: Expectation{Published: []PublishedOutcome{value("current", "3"), value("total", "3")}},
		},
		{
			Name: "loop/repeat-accumulates-before-its-condition",
			Source: `
local total = 0
local current = 0
repeat
    total = total + current
    current = current + 1
until current == 3
`,
			Expect: Expectation{Published: []PublishedOutcome{value("current", "3"), value("total", "3")}},
		},
		{
			Name: "pathstore/absent-index-read-is-nil",
			Source: `
local record = {}
local result = record["missing"]
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "nil")}},
		},
		{
			Name: "pathstore/dynamic-index-write-preserves-unrelated-member",
			Source: `
local record = { fixed = "kept" }
local key = "dynamic"
record[key] = "stored"
local result = record.fixed
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"kept"`)}},
		},
		{
			Name: "pathstore/dynamic-parameter-key-preserves-known-member",
			Source: `
local function write(record, key)
    record[key] = "dynamic"
    return record.fixed
end
local result = write({ fixed = "kept" }, "other")
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"kept"`)}},
		},
		{
			Name: "pathstore/false-value-is-not-absent",
			Source: `
local record = {}
record["present"] = false
local result = record.present
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "false")}},
		},
		{
			Name: "pathstore/nil-write-removes-member",
			Source: `
local record = { present = "value" }
record.present = nil
local result = record.present
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "nil")}},
		},
		{
			Name: "pathstore/numeric-index-is-distinct-from-string-index",
			Source: `
local record = {}
record[1] = "number"
record["1"] = "string"
local result = record[1]
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"number"`)}},
		},
		{
			Name: "pathstore/static-member-write-overwrites-prior-value",
			Source: `
local record = { status = "old" }
record.status = "new"
local result = record.status
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"new"`)}},
		},
		{
			Name: "pathstore/static-string-index-aliases-dot-member",
			Source: `
local record = {}
record["status"] = "ready"
local result = record.status
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"ready"`)}},
		},
		{
			Name:   "provider/global-call-with-false",
			Source: `local result = type(false)`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"boolean"`)}},
		},
		{
			Name:   "provider/global-call-with-nil",
			Source: `local result = type(nil)`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"nil"`)}},
		},
		{
			Name:   "provider/global-call-with-zero",
			Source: `local result = type(0)`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", `"number"`)}},
		},
		{
			Name:   "provider/global-statement-results-are-absent",
			Source: `collectgarbage("collect")`,
			Expect: Expectation{},
		},
		{
			Name:   "provider/select-counts-nil-values",
			Source: `local result = select("#", nil, false)`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "2")}},
		},
		{
			Name:   "provider/select-without-values-is-zero",
			Source: `local result = select("#")`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "0")}},
		},
		{
			Name:   "provider/string-find-returns-two-results",
			Source: `local first, second = string.find("abc", "b")`,
			Expect: Expectation{Published: []PublishedOutcome{value("first", "2"), value("second", "2")}},
		},
		{
			Name:   "provider/tonumber-nil-remains-nil",
			Source: `local result = tonumber(nil)`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "nil")}},
		},
		{
			Name: "recursion/mutual-functions-resolve-in-one-file",
			Source: `
local even, odd
even = function(value)
    if value == 0 then return true end
    return odd(value - 1)
end
odd = function(value)
    if value == 0 then return false end
    return even(value - 1)
end
local result = even(6)
`,
			Expect: Expectation{Published: []PublishedOutcome{value("result", "true")}},
		},
		{
			Name:   "outcome/bare-return-has-zero-results",
			Source: `return`,
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "0")}},
		},
		{
			Name:   "outcome/empty-string-is-a-result",
			Source: `return ""`,
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "1"), outcome("return/0", `""`)}},
		},
		{
			Name:   "outcome/false-is-a-result",
			Source: `return false`,
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "1"), outcome("return/0", "false")}},
		},
		{
			Name:   "outcome/local-value-is-published",
			Source: "local result = \"ready\"\nreturn result",
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "1"), outcome("return/0", `"ready"`)}},
		},
		{
			Name:   "outcome/missing-local-is-explicit-nil",
			Source: "local missing\nreturn missing",
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "1"), outcome("return/0", "nil")}},
		},
		{
			Name:   "outcome/multiple-results-preserve-order",
			Source: `return 7, nil, "tail"`,
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "3"), outcome("return/0", "7"), outcome("return/1", "nil"), outcome("return/2", `"tail"`)}},
		},
		{
			Name:   "outcome/nil-guard-selects-else-return",
			Source: "if nil then return \"then\" else return \"else\" end",
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "1"), outcome("return/0", `"else"`)}},
		},
		{
			Name:   "outcome/nil-is-distinct-from-no-result",
			Source: `return nil`,
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "1"), outcome("return/0", "nil")}},
		},
		{
			Name:   "outcome/selected-true-arm-publishes-only-its-return",
			Source: "if true then return \"selected\" else return \"not-selected\" end",
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "1"), outcome("return/0", `"selected"`)}},
		},
		{
			Name:   "outcome/zero-is-a-result",
			Source: `return 0`,
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "1"), outcome("return/0", "0")}},
		},
		{
			Name:   "outcome/open-return-tail-publishes-head-result",
			Source: `return provider()`,
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "1"), outcome("return/0", "unknown")}},
		},
		{
			Name:   "outcome/open-return-tail-keeps-preceding-slot",
			Source: `return "prefix", provider()`,
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "2"), outcome("return/0", `"prefix"`), outcome("return/1", "unknown")}},
		},
		{
			Name:   "outcome/parenthesized-return-call-is-one-adjusted-slot",
			Source: `return (provider())`,
			Expect: Expectation{Published: []PublishedOutcome{outcome("return/arity", "1"), outcome("return/0", "unknown")}},
		},
		{
			Name: "pathstore/member-write-from-first-temporary-is-admitted",
			Source: `
function module.answer()
    return 7
end
`,
			Expect: Expectation{},
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

func outcome(subject, result string) PublishedOutcome {
	return PublishedOutcome{Channel: "outcome", Subject: subject, Value: result}
}
