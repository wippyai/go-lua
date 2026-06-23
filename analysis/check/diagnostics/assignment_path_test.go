package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestAssignmentReportsNestedDynamicVariantWriteInvalidatedGuardWithEvidence(t *testing.T) {
	diags := runDiagnostics(t, `
		type FileSlot = {
			kind: "file",
			path: string,
		}
		type TimerSlot = {
			kind: "timer",
			seconds: number,
		}
		type Slot = {
			value: FileSlot | TimerSlot,
		}
		type Slots = {[string]: Slot}

		local slots: Slots = {
			active = {
				value = {kind = "file", path = "/tmp/active"},
			},
		}
		local key = "active"

		if slots.active.value.kind == "file" then
			slots[key].value = {kind = "timer", seconds = 20}
			local stale_path: string = slots.active.value.path
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want stale dynamic path assignment error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, "cannot assign slots.active.value.path") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q, want string assignment mismatch", d.Message)
	}
	if got := d.Explanation.Evidence(); len(got) < 2 {
		t.Fatalf("explanation evidence = %#v, want source and annotation evidence", got)
	}
	if len(d.Labels) < 2 || d.Labels[0].Message != "assigned value" || d.Labels[1].Message != "declared type" {
		t.Fatalf("labels = %#v, want assigned value and declared type", d.Labels)
	}
}

func TestAssignmentReportsNestedDynamicVariantWriteInvalidatedAliasWithEvidence(t *testing.T) {
	result := runDiagnosticsResult(t, `
		type FileSlot = {
			kind: "file",
			path: string,
		}
		type TimerSlot = {
			kind: "timer",
			seconds: number,
		}
		type Slot = {
			value: FileSlot | TimerSlot,
		}
		type Slots = {[string]: Slot}

		local slots: Slots = {
			active = {
				value = {kind = "file", path = "/tmp/active"},
			},
		}
		local active = slots.active
		local key = "active"

		if active.value.kind == "file" then
			slots[key].value = {kind = "timer", seconds = 20}
			local stale_path: string = active.value.path
		end
	`)
	diags := Produce(result)
	if len(diags) != 1 {
		point, expr := requireLocalAssignmentExprByName(t, result, "stale_path")
		rootType := "<unavailable>"
		if path, ok := result.ExpressionPath(expr); ok {
			if root, ok := dominatingRootDeclarationType(result, newResultResolver(result, nil), nil, point, path.Symbol); ok {
				rootType = formatType(root)
			}
		}
		if got, ok := dominatingDeclarationProjectionType(result, newResultResolver(result, nil), point, expr); ok {
			t.Fatalf("diagnostics = %d, want stale aliased dynamic path assignment error; declaration root = %s; declaration projection = %s; diags = %#v", len(diags), rootType, formatType(got), diags)
		}
		t.Fatalf("diagnostics = %d, want stale aliased dynamic path assignment error; declaration root = %s; declaration projection unavailable; diags = %#v", len(diags), rootType, diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, "cannot assign active.value.path") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q, want string assignment mismatch", d.Message)
	}
	if got := d.Explanation.Evidence(); len(got) < 2 {
		t.Fatalf("explanation evidence = %#v, want source and annotation evidence", got)
	}
	if len(d.Labels) < 2 || d.Labels[0].Message != "assigned value" || d.Labels[1].Message != "declared type" {
		t.Fatalf("labels = %#v, want assigned value and declared type", d.Labels)
	}
}

func TestAssignmentReportsDynamicIndexWriteInvalidatedGuardWithEvidence(t *testing.T) {
	diags := runDiagnostics(t, `
		type Box = {
			value: string?,
		}

		local box: Box = {value = "ready"}
		local alias = box
		local key = "value"

		if box.value then
			alias[key] = nil
			local after: string = box.value
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want dynamic-index invalidated guard assignment error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, "cannot assign box.value") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q, want string assignment mismatch", d.Message)
	}
	if got := d.Explanation.Evidence(); len(got) < 2 {
		t.Fatalf("explanation evidence = %#v, want source and annotation evidence", got)
	}
	if len(d.Labels) < 2 || d.Labels[0].Message != "assigned value" || d.Labels[1].Message != "declared type" {
		t.Fatalf("labels = %#v, want assigned value and declared type", d.Labels)
	}
}

func TestAssignmentDoesNotUseBroadDynamicKeyAsStaticMemberProof(t *testing.T) {
	diags := runDiagnostics(t, `
		type Box = {
			value: string?,
		}

		local function f(k: string): ()
			local box: Box = {}
			box[k] = "ready"
			local after: string = box.value
		end
	`)
	found := false
	for _, d := range diags {
		if d.Code == CodeAssignmentType &&
			strings.Contains(d.Message, "cannot assign box.value") &&
			strings.Contains(d.Message, "may be nil") &&
			strings.Contains(d.Explanation.String(), "no guard on this path proves box.value is non-nil") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v, want broad dynamic key to leave box.value optional with missing-proof evidence", diags)
	}
}

func TestAssignmentDoesNotUseAliasBroadDynamicKeyAsStaticMemberProof(t *testing.T) {
	diags := runDiagnostics(t, `
		type Box = {
			value: string?,
		}

		local function f(k: string): ()
			local box: Box = {}
			local alias = box
			alias[k] = "ready"
			local after: string = box.value
		end
	`)
	found := false
	for _, d := range diags {
		if d.Code == CodeAssignmentType &&
			strings.Contains(d.Message, "cannot assign box.value") &&
			strings.Contains(d.Message, "may be nil") &&
			strings.Contains(d.Explanation.String(), "no guard on this path proves box.value is non-nil") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v, want alias broad dynamic key to leave box.value optional with missing-proof evidence", diags)
	}
}

func TestAssignmentRejectsNilThroughPossiblyPresentDynamicIndexKey(t *testing.T) {
	diags := runDiagnostics(t, `
		type Key = "name" | "missing"
		type Bag = {name: string}

		local function f(bag: Bag, key: Key): ()
			bag[key] = nil
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one dynamic-index write mismatch", diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign nil") || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want nil rejected against possible name:string slot", d)
	}
}

func TestAssignmentMeetsDynamicIndexWriteContractsAcrossPossibleFields(t *testing.T) {
	diags := runDiagnostics(t, `
		type Key = "name" | "count"
		type Bag = {name: string, count: number}

		local function f(bag: Bag, key: Key): ()
			bag[key] = "bad"
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one dynamic-index write mismatch", diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign") || !strings.Contains(d.Message, "number") {
		t.Fatalf("diagnostic = %#v, want string rejected because key may select count:number", d)
	}
}

func TestAssignmentAllowsNilThroughDynamicIndexWhenOnlyOptionalSlotCanMatch(t *testing.T) {
	diags := runDiagnostics(t, `
		type Key = "value" | "missing"
		type Box = {value: string?}

		local function f(box: Box, key: Key): ()
			box[key] = nil
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want nil accepted when every possible declared slot is optional", diags)
	}
}

func TestAssignmentAllowsNilDynamicIndexDeletionForArraySlots(t *testing.T) {
	diags := runDiagnostics(t, `
		local xs: {number} = {1, 2}
		xs[#xs] = nil
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want nil dynamic-index write accepted as array slot deletion", diags)
	}
}

func TestAssignmentUsesStaticBracketStringKeyAsStaticMemberProof(t *testing.T) {
	diags := runDiagnostics(t, `
		type Box = {
			value: string?,
		}

		local function f(): ()
			local box: Box = {}
			box["value"] = "ready"
			local after: string = box.value
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want static bracket string write to prove the static member", diags)
	}
}

func TestAssignmentReportsSummaryPathInvalidatedGuardWithEvidence(t *testing.T) {
	diags := runDiagnostics(t, `
		type Box = {
			value: string?,
		}

		local function clear(box: Box, key: string): ()
			box[key] = nil
		end

		local box: Box = {value = "ready"}
		if box.value then
			clear(box, "value")
			local after: string = box.value
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want summary invalidated guard assignment error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, "cannot assign box.value") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q, want path-specific optional assignment mismatch", d.Message)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) < 2 {
		t.Fatalf("explanation evidence = %#v, want source and annotation evidence", evidence)
	}
	if !strings.Contains(evidence[0].Message, "box.value can be string or nil here") ||
		!strings.Contains(evidence[1].Message, "after is declared as string") ||
		!strings.Contains(d.Explanation.String(), "no guard on this path proves box.value is non-nil") {
		t.Fatalf("evidence = %#v, want path-specific source, declaration, and guard evidence", evidence)
	}
	if len(d.Labels) < 2 || d.Labels[0].Message != "assigned value" || d.Labels[1].Message != "declared type" {
		t.Fatalf("labels = %#v, want assigned value and declared type", d.Labels)
	}
}

func TestAssignmentRejectsReassignedFunctionFieldStalePathType(t *testing.T) {
	diags := runDiagnostics(t, `
		local M = {}
		function M.f(): string
			return "ok"
		end

		M.f = 42
		local g: () -> string = M.f
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want stale function-field assignment error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, "cannot assign") || !strings.Contains(d.Message, "fun() -> string") {
		t.Fatalf("message = %q, want function assignment mismatch", d.Message)
	}
	if got := d.Explanation.Evidence(); len(got) < 2 {
		t.Fatalf("explanation evidence = %#v, want source and annotation evidence", got)
	}
}

func TestAssignmentReportsOptionalDynamicIndexTarget(t *testing.T) {
	diags := runDiagnostics(t, `
		type Bag = {name: string}
		function f(bag: Bag?)
			bag["name"] = "ok"
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeOptionalAssignmentTarget || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot assign through optional bag without nil check") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("evidence = %#v, want container and write-requirement proof", d.Explanation.Evidence())
	}
	if !strings.Contains(d.Help, "Guard `bag` with a nil check") {
		t.Fatalf("help = %q, want nil-check repair", d.Help)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "bag can be") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "writing bag[\"name\"] requires its container to be non-nil") {
		t.Fatalf("evidence = %#v, want container type and write requirement", d.Explanation.Evidence())
	}
}

func TestAssignmentReportsOptionalStaticMemberTarget(t *testing.T) {
	diags := runDiagnostics(t, `
		type Bag = {name: string}
		function f(bag: Bag?)
			bag.name = "ok"
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeOptionalAssignmentTarget || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot assign through optional bag without nil check") {
		t.Fatalf("message = %q", d.Message)
	}
	if !strings.Contains(d.Help, "Guard `bag` with a nil check") {
		t.Fatalf("help = %q, want nil-check repair", d.Help)
	}
}

func TestAssignmentAcceptsGuardedOptionalDynamicIndexTarget(t *testing.T) {
	diags := runDiagnostics(t, `
		type Bag = {name: string}
		function f(bag: Bag?)
			if bag ~= nil then
				bag["name"] = "ok"
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after nil check", diags)
	}
}
