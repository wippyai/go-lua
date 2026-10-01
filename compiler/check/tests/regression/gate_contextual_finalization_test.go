package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
)

func TestGateContextualArgumentFinalizesPendingFields(t *testing.T) {
	pending := typ.NewRecord().
		Field("node_id", typ.NewUnion(typ.Any, typ.Unresolved)).
		MapComponent(typ.Any, typ.Any).SetComplete(true).Build()
	checkModes(t, `
local function consume(context: {[string]: any}) end
local function run(context: Pending) consume(context) end
return run`, "", "got {node_id: unknown, [any]: any}",
		testutil.WithTypes(map[string]typ.Type{"Pending": pending}))
}

func TestGateRecordWithAnyValuesFitsStringMap(t *testing.T) {
	checkBothModes(t, `
local function consume(context: {[string]: any}) end
local function run(id: any)
 local context = {node_id = id}
 consume(context)
end
return run`, "")
}

func TestGateDynamicKeysStillRequireStringEvidence(t *testing.T) {
	checkModes(t, `
local function consume(context: {[string]: any}) end
local function run(id: any, additions: any)
 local context = {node_id = id}
 for k, v in pairs(additions) do context[k] = v end
 consume(context)
end
return run`, "", "expected {[string]: any}")
}
