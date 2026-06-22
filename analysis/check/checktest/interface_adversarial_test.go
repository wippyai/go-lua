package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestSourceInterfaceAcceptsStructuralImplementation(t *testing.T) {
	result := Check(`
interface Reader
	function read(self: self): string
end

local impl: Reader = {
	read = function(self: any): string
		return "ok"
	end,
}

local value: string = impl:read()
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want source interface implementation to type-check", result.Diagnostics)
	}
}

func TestSourceInterfaceFreshArgumentChecksMethodSet(t *testing.T) {
	result := Check(`
interface Reader
	function read(self: self): string
end

local function use(reader: Reader): string
	return reader:read()
end

local value: string = use({
	read = function(self: any): string
		return "ok"
	end,
})
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want fresh table argument to satisfy Reader structurally", result.Diagnostics)
	}
}

func TestSourceInterfaceExtendsRequiresInheritedMethods(t *testing.T) {
	result := Check(`
interface Reader
	function read(self: self): string
end

interface Closer
	function close(self: self): boolean
end

interface ReadCloser: Reader, Closer
	function reset(self: self): boolean
end

local impl: ReadCloser = {
	read = function(self: any): string
		return "ok"
	end,
	close = function(self: any): boolean
		return true
	end,
	reset = function(self: any): boolean
		return true
	end,
}

local text: string = impl:read()
local closed: boolean = impl:close()
local reset: boolean = impl:reset()
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inherited interface method set to type-check", result.Diagnostics)
	}
}

func TestSourceInterfaceMissingMethodDiagnosticNamesContract(t *testing.T) {
	src := strings.TrimLeft(`
interface Reader
	function read(self: self): string
end

local impl: Reader = {}
`, "\n")
	result := Check(src)
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            5,
		Column:          22,
		MessageContains: []string{
			"object literal",
			"does not implement Reader",
			`missing method "read"`,
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"object literal", "has type {}"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"impl is declared as Reader"},
			},
			{
				Kind:  diagnostic.EvidenceMissingProof,
				Trust: diagnostic.TrustUnknown,
				MessageContains: []string{
					"required method read",
					"fun(self: self) -> string",
					"object literal does not provide it",
				},
			},
		},
		LabelContains: []string{"declared type", "object literal"},
		HelpContains:  []string{"Add method `read`", "change the target interface"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			`error[type.assignment]: object literal does not implement Reader: missing method "read"`,
			`  |             ↓ declared type`,
			`5 | local impl: Reader = {}`,
			`  |                      ↑ object literal`,
			`1. proven: object literal has type {}`,
			`2. claimed: impl is declared as Reader`,
			`3. missing proof: required method read has type fun(self: self) -> string, but the object literal does not provide it`,
			"help: Add method `read`, or change the target interface if this value should not implement it.",
		},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.assignment]: object literal does not implement Reader: missing method "read"
 --> test.lua:5:22
  |
  |             ↓ declared type
5 | local impl: Reader = {}
  |                      ↑ object literal

because:
  1. proven: object literal has type {}
  2. claimed: impl is declared as Reader
  3. missing proof: required method read has type fun(self: self) -> string, but the object literal does not provide it

help: Add method ` + "`read`" + `, or change the target interface if this value should not implement it.`
	assertRenderedEqual(t, rendered, want)
}

func TestSourceInterfaceMissingMethodArgumentDiagnosticNamesContract(t *testing.T) {
	src := strings.TrimLeft(`
interface Reader
	function read(self: self): string
end

local function use(reader: Reader): string
	return reader:read()
end

local value: string = use({})
`, "\n")
	result := Check(src)
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            9,
		Column:          27,
		MessageContains: []string{
			"argument 1",
			"does not implement Reader",
			`missing method "read"`,
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"argument 1", "has type {}"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"use parameter 1 expects Reader"},
			},
			{
				Kind:  diagnostic.EvidenceMissingProof,
				Trust: diagnostic.TrustUnknown,
				MessageContains: []string{
					"required method read",
					"fun(self: self) -> string",
					"object literal does not provide it",
				},
			},
		},
		LabelContains: []string{"argument value"},
		HelpContains:  []string{"Pass a value", "satisfies the parameter type", "change the callee signature"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			`error[type.call.direct.argument_type]: argument 1 does not implement Reader: missing method "read"`,
			`9 | local value: string = use({})`,
			`  |                           ↑ argument value`,
			`1. proven: argument 1 has type {}`,
			`2. claimed: use parameter 1 expects Reader`,
			`5 | local function use(reader: Reader): string`,
			`3. missing proof: required method read has type fun(self: self) -> string, but the object literal does not provide it`,
			`help: Pass a value for argument 1 that satisfies the parameter type, or change the callee signature if that argument is valid.`,
		},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.call.direct.argument_type]: argument 1 does not implement Reader: missing method "read"
 --> test.lua:9:27
  |
9 | local value: string = use({})
  |                           ↑ argument value

because:
  1. proven: argument 1 has type {}
  2. claimed: use parameter 1 expects Reader
 --> test.lua:5:28
  |
5 | local function use(reader: Reader): string
  |                            ^
  3. missing proof: required method read has type fun(self: self) -> string, but the object literal does not provide it

help: Pass a value for argument 1 that satisfies the parameter type, or change the callee signature if that argument is valid.`
	assertRenderedEqual(t, rendered, want)
}

func TestSourceInterfaceRejectsWrongMethodSignature(t *testing.T) {
	result := Check(`
interface Reader
	function read(self: self): string
end

local impl: Reader = {
	read = function(self: any): number
		return 1
	end,
}
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "read") || !strings.Contains(diag.Message, "number") || !strings.Contains(diag.Message, "string") {
		t.Fatalf("message = %q, want method signature mismatch with read/number/string", diag.Message)
	}
}

func TestSourceInterfaceRejectsWrongMethodSignatureInFreshArgument(t *testing.T) {
	result := Check(`
interface Reader
	function read(self: self): string
end

local function use(reader: Reader): string
	return reader:read()
end

local value: string = use({
	read = function(self: any): number
		return 1
	end,
})
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	if !strings.Contains(diag.Message, "read") || !strings.Contains(diag.Message, "number") || !strings.Contains(diag.Message, "string") {
		t.Fatalf("message = %q, want argument method signature mismatch with read/number/string", diag.Message)
	}
}

func TestSourceInterfaceRejectsNarrowerMethodReceiver(t *testing.T) {
	result := Check(`
interface Reader
	function read(self: self): string
end

local impl: Reader = {
	read = function(self: {id: string}): string
		return self.id
	end,
}
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "read") || !strings.Contains(diag.Message, "id") || !strings.Contains(diag.Message, "Reader") {
		t.Fatalf("message = %q, want receiver contract mismatch with read/id/Reader", diag.Message)
	}
}

func TestSourceInterfaceRejectsWrongInheritedMethodSignature(t *testing.T) {
	result := Check(`
interface Reader
	function read(self: self): string
end

interface ReadCloser: Reader
	function close(self: self): boolean
end

local impl: ReadCloser = {
	read = function(self: any): number
		return 1
	end,
	close = function(self: any): boolean
		return true
	end,
}
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "read") || !strings.Contains(diag.Message, "number") || !strings.Contains(diag.Message, "string") {
		t.Fatalf("message = %q, want inherited method signature mismatch with read/number/string", diag.Message)
	}
}
