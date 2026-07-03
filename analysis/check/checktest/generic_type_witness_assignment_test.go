package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestGenericTypeWitnessWholeValueAssignmentMismatch(t *testing.T) {
	result := Check(`
type Type<T> = { decode: (any) -> T }
type Payload = { id: string, count: number }
type Timer = { elapsed: number }

local TimerType: Type<Timer> = {
	decode = function(raw: any): Timer
		return { elapsed = 1 }
	end,
}

local json = {}
function json.decode<T>(data: string, witness: Type<T>): T
	return witness.decode(data)
end

local wrong_payload: Payload = json.decode("{}", TimerType)
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallResultAssignment,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            17,
		MessageContains: []string{
			"call result 1",
			"{elapsed: number}",
			"{count: number, id: string}",
		},
		EvidenceContains: []string{
			"json.decode",
			"wrong_payload requires",
			"{count: number, id: string}",
		},
	})
}

func TestGenericTypeWitnessArrayAssignmentMismatch(t *testing.T) {
	result := Check(`
type Type<T> = { decode: (any) -> T }
type Payload = { id: string, count: number }
type Timer = { elapsed: number }

local PayloadArrayType: Type<{Payload}> = {
	decode = function(raw: any): {Payload}
		return { { id = "a", count = 1 } }
	end,
}

local json = {}
function json.decode<T>(data: string, witness: Type<T>): T
	return witness.decode(data)
end

local rows = json.decode("[]", PayloadArrayType)
local wrong_rows: {Timer} = rows
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            18,
		MessageContains: []string{
			"rows",
			"{count: number, id: string}[]",
			"{elapsed: number}[]",
		},
		EvidenceContains: []string{
			"wrong_rows is declared as {Timer}",
		},
	})
}
