package regression

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestCapturedDecodedOutcomeKeepsDeclaredBound(t *testing.T) {
	const source = `
type Outcome = "succeeded" | "failed"
type Message = {message_id: string, outcome: Outcome?}
type Payload = {kind: "message", body: Message} | {kind: "other", body: string}
local function outcome(value: unknown): Outcome?
 if value == "succeeded" or value == "failed" then return value end
 return nil
end
local function decode(value: unknown): Message?
 local result: Message = {message_id = "id"}
 if value ~= nil then
  local decoded = outcome(value)
  if not decoded then return nil end
  result.outcome = decoded
 end
 return result
end
local function commit(payload: Payload) end
local function run(callback: () -> ()) callback() end
local function submit(value: unknown)
 local decoded = decode(value)
 if not decoded then return end
 run(function() commit({kind = "message", body = decoded}) end)
end
return submit
`
	cases := []struct {
		name   string
		source string
		bad    string
	}{
		{"decoded_capture", source, ""},
		{"inferred_alias_field", strings.Replace(source, "local decoded = decode(value)", `local decoded = {message_id = "id", outcome = outcome(value)}`, 1), ""},
		{"nested_inferred_alias_field", strings.Replace(strings.Replace(source, "local decoded = decode(value)", `local decoded = {message_id = "id", outcome = outcome(value)}`, 1), `run(function() commit({kind = "message", body = decoded}) end)`, `run(function() run(function() commit({kind = "message", body = decoded}) end) end)`, 1), ""},
		{"valid_captured_write", strings.Replace(source, `commit({kind = "message", body = decoded})`, `decoded.outcome = "failed"; commit({kind = "message", body = decoded})`, 1), ""},
		{"annotated_parent_write", strings.Replace(strings.Replace(source, "local decoded = decode(value)", "local decoded: Message? = decode(value)", 1), `run(function() commit({kind = "message", body = decoded}) end)`, `run(function() commit({kind = "message", body = decoded}) end)
 decoded.outcome = "failed"`, 1), ""},
		{"guarded_alias_field", strings.Replace(source, "if not decoded then return end", `if not decoded or decoded.outcome ~= "succeeded" then return end`, 1), ""},
		{"invalid_decoded_initializer", strings.Replace(source, `{message_id = "id"}`, `{message_id = "id", outcome = "invalid"}`, 1), "invalid"},
		{"invalid_captured_write", strings.Replace(source, `commit({kind = "message", body = decoded})`, `decoded.outcome = "invalid"; commit({kind = "message", body = decoded})`, 1), "invalid"},
		{"unbounded_string", strings.Replace(source, "local decoded = decode(value)", `local decoded = {message_id = "id", outcome = tostring(value)}`, 1), "expected Payload"},
		{"mutable_singleton", `
local decoded = {outcome = "succeeded"}
local function get(): "succeeded" return decoded.outcome end
decoded.outcome = "failed"
return get
`, `expected "succeeded"`},
		{"mutable_alias_singleton", `
type Outcome = "succeeded" | "failed"
local function initial(): Outcome return "succeeded" end
local decoded = {outcome = initial()}
if decoded.outcome ~= "succeeded" then return end
local function get(): "succeeded" return decoded.outcome end
local function set() decoded.outcome = "failed" end
return get, set
`, `expected "succeeded"`},
	}
	for _, strict := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("strict=%v/%s", strict, tc.name), func(t *testing.T) {
				result := testutil.Check(tc.source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
				messages := strings.Join(testutil.ErrorMessages(result.Errors), "; ")
				if tc.bad == "" {
					if result.HasError() {
						t.Fatal(messages)
					}
				} else if !strings.Contains(messages, tc.bad) {
					t.Fatalf("want diagnostic containing %q, got %q", tc.bad, messages)
				}
			})
		}
	}
}
