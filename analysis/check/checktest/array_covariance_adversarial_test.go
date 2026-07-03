package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

func TestHeapTrackedCovariantArrayReplacementReportsMissingFieldAsNilOnly(t *testing.T) {
	result := Check(`
type Animal = { name: string }
type Dog = { name: string, breed: string }
local dogs: {Dog} = { { name = "rex", breed = "lab" } }
local animals: {Animal} = dogs
animals[1] = { name = "cat" }
local b: string = dogs[1].breed
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "is nil, not string") {
		t.Fatalf("message = %q, want exact nil assignment", diag.Message)
	}
	requireEvidenceMessage(t, diag, "dogs[1].breed has type nil")
	for _, evidence := range diag.Explanation.Evidence() {
		if strings.Contains(evidence.Message, "can be string or nil") {
			t.Fatalf("evidence = %#v, want replacement heap object to prove breed is nil-only", diag.Explanation.Evidence())
		}
	}
}

func TestAsyncClosureInitializedMemberDoesNotBecomeSynchronousProof(t *testing.T) {
	result := Check(`
local function make_async()
    local obj = {}
    coroutine.spawn(function()
        obj.get_value = function(self): number
            return 42
        end
    end)
    return obj
end

local async_obj = make_async()
local v: number = async_obj:get_value()
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "async_obj:get_value(...)") {
		t.Fatalf("message = %q, want async method call assignment diagnostic", diag.Message)
	}
}

func TestReturnedLocalTableInitializedMemberStaysCallable(t *testing.T) {
	result := Check(`
local function make()
    local obj = { x = 1 }
    local function init()
        obj.get_x = function(self): number
            return self.x
        end
    end
    init()
    return obj
end

local built = make()
local n: number = built:get_x()
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want returned local table member fact to keep get_x callable", result.Diagnostics)
	}
}
