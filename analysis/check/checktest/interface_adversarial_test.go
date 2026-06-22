package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
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
	result := Check(`
interface Reader
	function read(self: self): string
end

local impl: Reader = {}
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "Reader") || !strings.Contains(diag.Message, "read") {
		t.Fatalf("message = %q, want interface contract and missing method name", diag.Message)
	}
}

func TestSourceInterfaceMissingMethodArgumentDiagnosticNamesContract(t *testing.T) {
	result := Check(`
interface Reader
	function read(self: self): string
end

local function use(reader: Reader): string
	return reader:read()
end

local value: string = use({})
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	if !strings.Contains(diag.Message, "Reader") || !strings.Contains(diag.Message, "read") {
		t.Fatalf("message = %q, want interface contract and missing method name", diag.Message)
	}
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
