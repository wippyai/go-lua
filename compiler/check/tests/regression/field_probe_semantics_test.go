package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFieldProbeSemantics_InterfaceMissingFieldIsNilOnlyInProbe(t *testing.T) {
	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(typ.NewRecord().
		Field("now", typ.Func().Returns(typ.NewInterface("time.Time", []typ.Method{
			{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
		})).Build()).
		Build())

	result := testutil.Check(`
local time = require("time")
local t = time.now()
if t.from == "pid" then
	return false
end
if t.kind then
	return false
end
return true
`, testutil.WithStdlib(), testutil.WithManifest("time", timeManifest))
	if result.HasError() {
		t.Fatalf("expected missing interface fields to be nil-producing probes, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	result = testutil.Check(`
local time = require("time")
local t = time.now()
local from = t.from
return from
`, testutil.WithStdlib(), testutil.WithManifest("time", timeManifest))
	if !result.HasError() {
		t.Fatal("expected direct missing interface field value read to remain an error")
	}
	if !strings.Contains(strings.Join(testutil.ErrorMessages(result.Diagnostics), "\n"), "field 'from' does not exist") {
		t.Fatalf("expected missing field diagnostic, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFieldProbeSemantics_UntypedTableGuardAllowsExistenceProbe(t *testing.T) {
	result := testutil.Check(`
local context = {}

function context:add_tools(tool_specs): any
	if not tool_specs then
		return self
	end

	if type(tool_specs) == "string" or (type(tool_specs) == "table" and tool_specs.id) then
		tool_specs = { tool_specs }
	end

	return self
end

function context:add_delegates(delegate_specs): any
	if not delegate_specs then
		return self
	end

	if delegate_specs.id then
		delegate_specs = { delegate_specs }
	end

	return self
end
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected untyped table-style field probes to be nil-producing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
