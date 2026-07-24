package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/lint"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

// checkSource runs the ordinary project check over one module.
func checkSource(t *testing.T, source string) []diag.Diagnostic {
	t.Helper()
	result, err := lint.CheckProject(context.Background(), lint.ProjectInput{
		Entries: []lint.Entry{{Path: "main.lua", ModulePath: "main", Source: source}},
		Targets: []string{"main"},
	})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	return result.Diagnostics
}

func diagnosticSummaries(diagnostics []diag.Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, item := range diagnostics {
		parts = append(parts, item.Position.String()+" ["+string(item.Code)+"] "+item.Message)
	}
	return strings.Join(parts, "\n")
}

// TestMemberWrittenAfterConstructionSurvivesReturn pins the heap member
// authority at a return boundary. The constructor shape of obj does not contain
// get_x, so a caller that consumed the allocation-time value would prove the
// member absent and refute the call.
func TestMemberWrittenAfterConstructionSurvivesReturn(t *testing.T) {
	diagnostics := checkSource(t, `local function make()
    local obj = { x = 1 }
    obj.get_x = function(self): number
        return self.x
    end
    return obj
end

local built = make()
local n: number = built:get_x()
return n
`)
	if len(diagnostics) != 0 {
		t.Fatalf("member written after construction was lost at the return boundary:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestMemberWrittenThroughCaptureSurvivesCall pins the same authority across a
// capture writeback: the mutation happens inside a called local function that
// captures the table.
func TestMemberWrittenThroughCaptureSurvivesCall(t *testing.T) {
	diagnostics := checkSource(t, `local obj = { x = 1 }

local function init()
    obj.get_x = function(self): number
        return self.x
    end
end

init()

local n: number = obj:get_x()
return n
`)
	if len(diagnostics) != 0 {
		t.Fatalf("member written through a capture was lost at the call boundary:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestMemberWrittenThroughCaptureSurvivesNestedReturn composes both boundaries:
// the write happens in a nested capture-mutating body and the table then leaves
// its allocating frame as a return value.
func TestMemberWrittenThroughCaptureSurvivesNestedReturn(t *testing.T) {
	diagnostics := checkSource(t, `local function make()
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
return n
`)
	if len(diagnostics) != 0 {
		t.Fatalf("member written through a nested capture was lost:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestAbsentMemberRemainsRefuted keeps the fix from becoming optimism: a member
// that no write establishes is still proven absent and still refutes the call.
func TestAbsentMemberRemainsRefuted(t *testing.T) {
	diagnostics := checkSource(t, `local function make()
    local obj = { x = 1 }
    obj.get_x = function(self): number
        return self.x
    end
    return obj
end

local built = make()
local n: number = built:get_y()
return n
`)
	refuted := false
	for _, item := range diagnostics {
		if item.Code == "type.call.direct.not_callable" && strings.Contains(item.Message, "built.get_y") {
			refuted = true
		}
	}
	if !refuted {
		t.Fatalf("an unwritten member must remain refuted:\n%s", diagnosticSummaries(diagnostics))
	}
}
