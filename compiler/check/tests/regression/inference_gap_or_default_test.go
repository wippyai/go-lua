package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
)

// TestInferenceGap_GradualParamValueOperatorsInferBodyPreconditions verifies
// that value-operator use on an unannotated parameter emits a function-entry
// obligation instead of failing inside the function body. The caller boundary
// remains responsible for proving the obligation.
func TestInferenceGap_GradualParamValueOperatorsInferBodyPreconditions(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{
			name: "length operator on bare param",
			code: `
local function f(entries)
	return #entries
end
return { f = f }
`,
		},
		{
			name: "concat on or-default of bare param",
			code: `
local function eq(msg)
	return (msg or "fallback") .. "x"
end
eq()
`,
		},
		{
			name: "concat directly on bare param",
			code: `
local function f(msg)
	return msg .. "x"
end
return { f = f }
`,
		},
		{
			name: "comparison on bare param",
			code: `
local function f(n)
	return n < 10
end
return { f = f }
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := testutil.Check(tc.code, testutil.WithStdlib())
			if result.HasError() {
				t.Fatalf("operator body demand should infer a callable precondition, got: %v", testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

func TestInferenceGap_GradualParamValueOperatorPreconditionsRejectIncompatibleCallers(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{
			name: "length operator rejects non-lengthable caller",
			code: `
local function f(entries)
	return #entries
end
f(10)
`,
		},
		{
			name: "concat or-default rejects table caller",
			code: `
local function eq(msg)
	return (msg or "fallback") .. "x"
end
eq({})
`,
		},
		{
			name: "direct concat rejects table caller",
			code: `
local function f(msg)
	return msg .. "x"
end
f({})
`,
		},
		{
			name: "numeric comparison rejects string caller",
			code: `
local function f(n)
	return n < 10
end
f("x")
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := testutil.Check(tc.code, testutil.WithStdlib())
			if !result.HasError() {
				t.Fatalf("expected incompatible caller to fail the inferred operator precondition")
			}
		})
	}
}

// TestInferenceGap_GradualParamReadStaysPreciseWhenConstrained guards the
// soundness boundary: a parameter refined from a typed call keeps its derived
// type and is not widened back to gradual `any`.
func TestInferenceGap_GradualParamReadStaysPreciseWhenConstrained(t *testing.T) {
	source := `
local function send(target: string)
end
local function worker(pid)
	send(pid)
	return #pid
end
return { worker = worker }
`
	result := testutil.Check(source, testutil.WithStdlib())
	// pid is refined to string from send(pid), so #pid (length on string) is valid.
	if result.HasError() {
		t.Fatalf("call-refined parameter must keep its derived type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestInferenceGap_CalleeFlowIntoDoesNotAliasCallerParamByIndex verifies that a
// callee's parameter-to-return flow effect is not propagated into a caller by
// raw parameter index. A forwarding helper whose own same-indexed parameter is
// unrelated to the flowed payload must not inject that argument into the return.
func TestInferenceGap_CalleeFlowIntoDoesNotAliasCallerParamByIndex(t *testing.T) {
	source := `
local DT = { EXECUTE_NODES = "e", NO_WORK = "n" }
local function create_decision(decision_type, payload)
	return { type = decision_type, payload = payload or {} }
end
local function create_nodes_execution(nodes)
	return create_decision(DT.EXECUTE_NODES, { nodes = nodes })
end
local function decide_strategy(ready_nodes, allow)
	if #ready_nodes == 0 then
		return nil
	elseif #ready_nodes == 1 then
		return create_nodes_execution(ready_nodes)
	end
	return create_nodes_execution({ ready_nodes[1] })
end
local function find_input()
	return decide_strategy({}, true)
end
local d = find_input()
if d then
	local p = d.payload
	local _ = p.nodes
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("a boolean argument to a forwarding helper must not leak into the returned payload record, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestInferenceGap_ImportedOrDefaultFieldRemainderSurvivesAbsentField verifies
// that an imported function whose return field is `param.field or default`
// keeps its default-string floor when the caller omits that field, instead of
// collapsing the field to unknown.
func TestInferenceGap_ImportedOrDefaultFieldRemainderSurvivesAbsentField(t *testing.T) {
	mod := testutil.CheckAndExport(`
local mapper = {}
function mapper.map_error_response(info)
	local error_message = info.message or "fallback"
	return { error_message = error_message }
end
return mapper
`, "mapper_mod", testutil.WithStdlib())
	if mod.HasError() {
		t.Fatalf("module should export cleanly, got: %v", testutil.ErrorMessages(mod.Errors))
	}

	encoded, err := io.EncodeManifest(mod.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	result := testutil.Check(`
local mapper = require("mapper_mod")
local present = mapper.map_error_response({ message = "timeout" })
local absent = mapper.map_error_response({ status_code = 400 })
local _: string = present.error_message
local _: string = absent.error_message
`, testutil.WithStdlib(), testutil.WithManifest("mapper_mod", decoded))
	if result.HasError() {
		t.Fatalf("an or-default return field must remain string when the caller omits the source field, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
