package regression_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestCrossModuleGenericExportPreservesFunctionTypeParams(t *testing.T) {
	json := testutil.CheckAndExport(`
type Type<T> = {
    decode: (any) -> T,
}

local M = {}

function M.decode_map<T, U>(data: string, witness: Type<T>, fn: (T) -> U): U
    return fn(witness.decode(data))
end

return M
`, "json")
	if len(json.Errors) > 0 {
		t.Fatalf("json module errors: %v", json.Errors)
	}

	rec := unwrap.Record(json.Manifest.Export)
	if rec == nil {
		t.Fatalf("json export = %T (%v), want record", json.Manifest.Export, json.Manifest.Export)
	}
	field := rec.GetField("decode_map")
	if field == nil {
		t.Fatalf("missing decode_map export in %v", rec)
	}
	fn := unwrap.Function(field.Type)
	if fn == nil {
		t.Fatalf("decode_map export = %T (%v), want function", field.Type, field.Type)
	}
	if len(fn.TypeParams) != 2 {
		t.Fatalf("decode_map type params = %d, want 2; fn=%v", len(fn.TypeParams), fn)
	}
	if len(fn.Returns) != 1 {
		t.Fatalf("decode_map returns = %v, want one return", fn.Returns)
	}
	if _, ok := unwrap.Alias(fn.Returns[0]).(*typ.TypeParam); !ok {
		t.Fatalf("decode_map return = %T (%v), want type param U", fn.Returns[0], fn.Returns[0])
	}
}

func TestCrossModuleImportedGenericFunctionInfersInstantiatedArg(t *testing.T) {
	process := testutil.CheckAndExport(`
type Box<T> = {
    value: T,
}

local M = {}

function M.id<T>(value: Box<T>): Box<T>
    return value
end

return M
`, "process")
	if len(process.Errors) > 0 {
		t.Fatalf("process module errors: %v", process.Errors)
	}

	result := testutil.Check(`
local process = require("process")

type Box<T> = {
    value: T,
}

type Node = {
    id: string,
}

local box: Box<Node> = {value = {id = "n1"}}
local out = process.id(box)
local id: string = out.value.id
local bad: number = out.value.id -- expect-error
`, testutil.WithStdlib(), testutil.WithModule("process", process))

	var errors []diag.Diagnostic
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			errors = append(errors, d)
		}
	}
	if len(errors) != 1 {
		t.Fatalf("errors = %v, want exactly annotated bad assignment", errors)
	}
}

func TestCrossModuleImportedGenericFunctionInfersPackageGenericArg(t *testing.T) {
	process := testutil.CheckAndExport(`
local M = {}

function M.receive_map<T, U>(channel: Channel<T>, fn: (T) -> U): U?
    local value, ok = channel:receive()
    if ok then
        return fn(value)
    end
    return nil
end

return M
`, "process", testutil.WithManifest("channel", testutil.ChannelManifest()))
	if len(process.Errors) > 0 {
		t.Fatalf("process module errors: %v", process.Errors)
	}
	rec := unwrap.Record(process.Manifest.Export)
	if rec == nil {
		t.Fatalf("process export = %T (%v), want record", process.Manifest.Export, process.Manifest.Export)
	}
	field := rec.GetField("receive_map")
	if field == nil {
		t.Fatal("missing receive_map export")
	}
	fn := unwrap.Function(field.Type)
	if fn == nil || len(fn.TypeParams) != 2 || len(fn.Params) != 2 || len(fn.Returns) != 1 {
		t.Fatalf("receive_map export = %v, want generic fn<T,U>(Channel<T>, (T)->U): U?", field.Type)
	}
	callback := unwrap.Function(fn.Params[1].Type)
	if callback == nil || len(callback.Params) != 1 || len(callback.Returns) != 1 {
		t.Fatalf("receive_map callback param = %v, want unary callback", fn.Params[1].Type)
	}
	if _, ok := unwrap.Alias(callback.Returns[0]).(*typ.TypeParam); !ok {
		t.Fatalf("receive_map callback return = %T (%v), want type param U", callback.Returns[0], callback.Returns[0])
	}

	result := testutil.Check(`
local process = require("process")

type Node = {
    id: string,
}

local function run(ch: Channel<Node>)
    local out = process.receive_map(ch, function(node)
        return node.id
    end)
    if out then
        local id: string = out
        local bad: number = out -- expect-error
    end
end
`, testutil.WithStdlib(), testutil.WithManifest("channel", testutil.ChannelManifest()), testutil.WithModule("process", process))

	var errors []diag.Diagnostic
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			errors = append(errors, d)
		}
	}
	if len(errors) != 1 {
		t.Fatalf("errors = %v, want exactly annotated bad assignment", errors)
	}
}
