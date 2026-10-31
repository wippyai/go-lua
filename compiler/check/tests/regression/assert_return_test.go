package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
)

func TestAssertReturnNarrowsOptionalRecord(t *testing.T) {
	source := `
type Projection = {content: string}
function gateway(): Projection?
	return {content = "ok"}
end
function takes_string(value: string)
end
local projection = assert(gateway())
takes_string(projection.content)
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("assert should preserve the record after removing nil, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestAssertReturnNarrowsOptionalRecordForExplicitAssignment(t *testing.T) {
	source := `
type Projection = {content: string}
function gateway(): Projection?
	return {content = "ok"}
end
local projection: Projection = assert(gateway())
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("assert result should satisfy an explicit record annotation, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestAssertReturnPreservesWrongFieldDiagnostics(t *testing.T) {
	source := `
type Projection = {content: string}
function gateway(): Projection?
	return {content = "ok"}
end
local projection = assert(gateway())
local number_value: number = projection.content
`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("assert should preserve the field type so an invalid assignment is diagnosed")
	}
}

func TestAssertReturnTruthinessAndMessageCompatibility(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "truthy primitive values retain their types",
			Code: `
local n: number = assert(0)
local s: string = assert("")
local b: boolean = assert(true)
local nil_value: string = assert(nil)
local false_value: string = assert(false)
`,
			Stdlib: true,
		},
		{
			Name: "message remains optional string",
			Code: `
local s: string = assert("ok", "message")
`,
			Stdlib: true,
		},
		{
			Name: "assert remains assignable at its declared function boundary",
			Code: `
local assertion: (any, string) -> any = assert
`,
			Stdlib: true,
		},
		{
			Name: "non-string message remains rejected",
			Code: `
assert("ok", 42)
`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestAssertReturnDoesNotNarrowUnrelatedGenericIdentity(t *testing.T) {
	source := `
type Projection = {content: string}
function identity<T>(value: T): T
	return value
end
function takes_string(value: string)
end
local projection = identity((nil :: Projection?))
takes_string(projection.content)
`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("an unrelated generic identity must leave an optional value optional")
	}
}

func TestShadowedAssertRetainsLocalFunctionSemantics(t *testing.T) {
	source := `
type Projection = {content: string}
function gateway(): Projection?
	return {content = "ok"}
end
local assert = function(value: Projection?): Projection?
	return value
end
function takes_string(value: string)
end
local projection = assert(gateway())
takes_string(projection.content)
`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("a shadowed local assert must retain its declared optional return")
	}
}

func TestAssertReturnNarrowsOptionalResultFromManifestProducer(t *testing.T) {
	producer := testutil.CheckAndExport(`
type Projection = {content: string}
local M = {}
function M.gateway(): Projection?
	return {content = "ok"}
end
return M
`, "assert_return_producer", testutil.WithStdlib())
	if producer.HasError() {
		t.Fatalf("unexpected producer errors: %v", testutil.ErrorMessages(producer.Errors))
	}
	encoded, err := io.EncodeManifest(producer.Manifest)
	if err != nil {
		t.Fatalf("failed to encode producer manifest: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("failed to decode producer manifest: %v", err)
	}

	consumer := testutil.Check(`
local producer = require("assert_return_producer")
function takes_string(value: string)
end
local projection = assert(producer.gateway())
takes_string(projection.content)
`, testutil.WithStdlib(), testutil.WithManifest("assert_return_producer", decoded))
	if consumer.HasError() {
		t.Fatalf("assert should narrow an optional result imported through a manifest, got: %v", testutil.ErrorMessages(consumer.Diagnostics))
	}
}
