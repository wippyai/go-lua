package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

// A module table built from an empty literal sees every write, so a field it
// never receives reads as nil and `t.f or default` is the default.
func TestAbsentFieldOfLocalTableIsNil(t *testing.T) {
	result := testutil.Check(`
local impl = { resolve = function(key: string): string? return key end }
local writer = {}

local function delivery()
	return writer._delivery or impl
end

function writer.resolve(key: string): string?
	return delivery().resolve(key)
end

return writer
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
}

// An absent field of a table literal reads as nil; returning it against a
// declared string is reported as nil, not as an unknown value.
func TestAbsentFieldOfInferredTableReportsNil(t *testing.T) {
	cases := map[string]string{
		"closed literal": `
local function f(): string
	local obj = { x = "a" }
	return obj.y
end
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			result := testutil.Check(src, testutil.WithStdlib())
			msgs := testutil.ErrorMessages(result.Errors)
			found := false
			for _, m := range msgs {
				if strings.Contains(m, "cannot return nil") {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected a nil return error, got: %v", msgs)
			}
		})
	}
}

// A parameter typed from call-site hints is known only partially: a field the
// hint lacks stays unknown, and flowing it into a declared type is reported as
// an implicit unknown, not an error.
func TestAbsentFieldOfHintedParamIsImplicitUnknown(t *testing.T) {
	result := testutil.Check(`
local function read(o)
	local missing: number = o.missing
	return missing
end
read({ present = 1 })
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
	if !hasDiagnostic(result.Diagnostics, diag.SeverityHint, "implicit unknown flows into declared number") {
		t.Fatalf("expected an implicit unknown hint, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// When every table the module sets a class's metatable on is a literal with
// no dynamic-key writes, the class's instances hold only the fields the class
// and its constructor give them, so a field neither writes reads nil on self.
func TestAbsentFieldOfCompleteReceiverIsNil(t *testing.T) {
	result := testutil.Check(`
local session_writer = {}
session_writer.__index = session_writer

function session_writer.new(session_id: string)
    local self = setmetatable({}, session_writer)
    self.session_id = session_id
    return self
end

function session_writer:get_user_id(): string
    return self.user_id
end
`, testutil.WithStdlib())
	msgs := testutil.ErrorMessages(result.Errors)
	if len(msgs) != 1 || !strings.Contains(msgs[0], "cannot return nil") {
		t.Fatalf("expected the receiver's absent field to read nil, got: %v", msgs)
	}
}

// A receiver whose instances are copied through dynamic keys may hold fields
// the class never names, so an absent field stays unknown on self.
func TestAbsentFieldOfCopiedReceiverStaysUnknown(t *testing.T) {
	result := testutil.Check(`
local methods = {}
local mt = { __index = methods }
function methods:copy()
    local new = {}
    for k, v in pairs(self) do new[k] = v end
    return setmetatable(new, mt)
end
function methods:label(): string
    local n: number = self.count
    return "x"
end
return methods
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
	if !hasDiagnostic(result.Diagnostics, diag.SeverityHint, "implicit unknown flows into declared number") {
		t.Fatalf("expected an implicit unknown hint, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func hasDiagnostic(diags []diag.Diagnostic, severity diag.Severity, contains string) bool {
	for _, d := range diags {
		if d.Severity == severity && strings.Contains(d.Message, contains) {
			return true
		}
	}
	return false
}
