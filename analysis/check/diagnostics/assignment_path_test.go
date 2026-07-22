package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

func TestAssignmentReportsNestedDynamicVariantWriteInvalidatedGuardWithEvidence(t *testing.T) {
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
		local key = "active"

		if slots.active.value.kind == "file" then
			slots[key].value = {kind = "timer", seconds = 20}
			local stale_path: string = slots.active.value.path
		end
	`)
	for _, point := range result.Graph().RPO() {
		if st, ok := result.StateAt(point); ok {
			st.ForEachDynamicIndexFact(func(key dynamicindex.Key, fact dynamicindex.Fact) bool {
				keyType, _ := typevalue.TypeOf(result.Registry(), fact.KeyValue)
				valueType, _ := typevalue.TypeOf(result.Registry(), fact.Value)
				t.Logf("point=%d dynamic table=%s key=%v value=%v admission=%d", point, result.KeySpace().FormatReadOnly(key.Table), keyType, valueType, fact.Admission)
				return true
			})
			st.ForEachPathStaticMember(result.KeySpace(), func(key keyspace.Key, value product.Value) bool {
				valueType, _ := typevalue.TypeOf(result.Registry(), value)
				t.Logf("point=%d static=%s value=%v", point, result.KeySpace().FormatReadOnly(key), valueType)
				return true
			})
		}
	}
	diags := Produce(result)
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
	assertAssignmentPathEvidence(t, d,
		"slots.active.value.path can be string or nil here",
		"stale_path is declared as string",
		"no guard on this path proves slots.active.value.path is non-nil",
		"Guard `slots.active.value.path` with a nil check",
	)
	if len(d.Labels) < 2 || d.Labels[0].Message != "assigned value" || d.Labels[1].Message != "declared type" {
		t.Fatalf("labels = %#v, want assigned value and declared type", d.Labels)
	}
}

func TestAssignmentReportsMissingStaticMapSlotAgainstUnionAliasAsMayNil(t *testing.T) {
	diags := runDiagnostics(t, `
		type AllowDecision = { kind: "allow", reason: string }
		type DenyDecision = { kind: "deny", reason: string }
		type Decision = AllowDecision | DenyDecision
		type Store = {
			cached: {[string]: Decision},
		}

		local store: Store = { cached = {} }
		local missing: Decision = store.cached["missing"]
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one missing indexed-read assignment: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, `cannot assign store.cached["missing"]`) || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q, want missing-slot nil assignment", d.Message)
	}
	if strings.Contains(d.Message, "not {") {
		t.Fatalf("message = %q, should not render same-type mismatch", d.Message)
	}
	assertAssignmentPathEvidence(t, d,
		`store.cached["missing"] can be`,
		"missing is declared as Decision",
		`store.cached["missing"] is an indexed read that can miss or read nil`,
		"Guard `store.cached[\"missing\"]` with a nil check",
	)
}

func TestAssignmentReportsUnwrittenStaticMapSlotAgainstUnionAliasAsMayNil(t *testing.T) {
	diags := runDiagnostics(t, `
		type AllowDecision = { kind: "allow", reason: string }
		type DenyDecision = { kind: "deny", reason: string }
		type Decision = AllowDecision | DenyDecision
		type Store = {
			cached: {[string]: Decision},
		}

		local store: Store = { cached = {} }
		store.cached["present"] = { kind = "allow", reason = "ok" }
		local missing: Decision = store.cached["missing"]
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one missing indexed-read assignment: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, `cannot assign store.cached["missing"]`) || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q, want missing-slot nil assignment", d.Message)
	}
	if strings.Contains(d.Message, "not {") {
		t.Fatalf("message = %q, should not render same-type mismatch", d.Message)
	}
	assertAssignmentPathEvidence(t, d,
		`store.cached["missing"] can be`,
		"missing is declared as Decision",
		`store.cached["missing"] is an indexed read that can miss or read nil`,
		"Guard `store.cached[\"missing\"]` with a nil check",
	)
}

func TestAssignmentReportsFunctionMutatedMissingMapSlotAsMayNil(t *testing.T) {
	diags := runDiagnostics(t, `
		type AllowDecision = { kind: "allow", reason: string }
		type DenyDecision = { kind: "deny", reason: string }
		type Decision = AllowDecision | DenyDecision
		type Store = {
			cached: {[string]: Decision},
		}

		local store: Store = { cached = {} }
		local function cache_decision(s: Store, key: string, decision: Decision): ()
			s.cached[key] = decision
		end
		cache_decision(store, "present", { kind = "allow", reason = "ok" })
		local missing: Decision = store.cached["missing"]
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one missing indexed-read assignment: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, `cannot assign store.cached["missing"]`) || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q, want missing-slot nil assignment", d.Message)
	}
	if strings.Contains(d.Message, "not {") {
		t.Fatalf("message = %q, should not render same-type mismatch", d.Message)
	}
	assertAssignmentPathEvidence(t, d,
		`store.cached["missing"] can be`,
		"missing is declared as Decision",
		`store.cached["missing"] is an indexed read that can miss or read nil`,
		"Guard `store.cached[\"missing\"]` with a nil check",
	)
}

func TestAssignmentKeepsExactNilMemberAfterIndexedAliasWrite(t *testing.T) {
	diags := runDiagnostics(t, `
		type Animal = { name: string }
		type Dog = { name: string, breed: string }
		local dogs: {Dog} = { { name = "rex", breed = "lab" } }
		local animals: {Animal} = dogs
		animals[1] = { name = "cat" }
		local b: string = dogs[1].breed
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one exact nil assignment: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, `cannot assign dogs[1].breed`) || !strings.Contains(d.Message, "nil, not string") {
		t.Fatalf("message = %q, want exact nil member assignment", d.Message)
	}
	if strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q, should preserve exact nil member evidence", d.Message)
	}
	assertAssignmentPathEvidence(t, d,
		`dogs[1].breed has type nil`,
		"b is declared as string",
		"",
		"Use a value compatible with the expected type",
	)
}

func TestAssignmentAcceptsNestedUnionFieldAfterDiscriminantGuard(t *testing.T) {
	src := `
		type RenderOutput = {
			kind: "rendered",
			body: string,
			label: string?,
		}
		type IndexOutput = {
			kind: "indexed",
			count: integer,
		}
		type AuditOutput = {
			kind: "audited",
			note: string,
		}
		type Output = RenderOutput | IndexOutput | AuditOutput
		type Receipt = {
			plugin: string,
			output: Output,
		}

		local receipt: Receipt = {
			plugin = "render",
			output = {
				kind = "rendered",
				body = "ok",
				label = nil,
			},
		}

		if receipt.output.kind == "rendered" then
			local rendered: RenderOutput = receipt.output
			local body: string = rendered.body
		end
	`
	diags := runDiagnostics(t, src)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want guarded nested discriminant assignment accepted", diags)
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
		t.Fatalf("diagnostics = %d, want stale aliased dynamic path assignment error; diags = %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, "cannot assign active.value.path") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q, want string assignment mismatch", d.Message)
	}
	assertAssignmentPathEvidence(t, d,
		"active.value.path can be string or nil here",
		"stale_path is declared as string",
		"no guard on this path proves active.value.path is non-nil",
		"Guard `active.value.path` with a nil check",
	)
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
	assertAssignmentPathEvidence(t, d,
		"box.value can be string or nil here",
		"after is declared as string",
		"no guard on this path proves box.value is non-nil",
		"Guard `box.value` with a nil check",
	)
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

func TestAssignmentMeetsStaticBracketWriteContractsAcrossUnionRecordArms(t *testing.T) {
	diags := runDiagnostics(t, `
		type Box = {value: number} | {value: string}

		local function f(box: Box): ()
			box["value"] = "bad"
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one static bracket write mismatch", diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign") || !strings.Contains(d.Message, "number") {
		t.Fatalf("diagnostic = %#v, want string rejected because box may require value:number", d)
	}
}

func TestAssignmentMeetsStaticDotWriteContractsAcrossUnionRecordArms(t *testing.T) {
	diags := runDiagnostics(t, `
		type Box = {value: number} | {value: string}

		local function f(box: Box): ()
			box.value = "bad"
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one static dot write mismatch", diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign") || !strings.Contains(d.Message, "number") {
		t.Fatalf("diagnostic = %#v, want string rejected because box may require value:number", d)
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
	result := runDiagnosticsResult(t, `
		type Box = {
			value: string?,
		}

		local function f(): ()
			local box: Box = {}
			box["value"] = "ready"
			local after: string = box.value
		end
	`)
	diags := Produce(result)
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
	if !strings.Contains(d.Message, "cannot assign box.value") || !strings.Contains(d.Message, "is nil, not string") {
		t.Fatalf("message = %q, want path-specific optional assignment mismatch", d.Message)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) < 2 {
		t.Fatalf("explanation evidence = %#v, want source and annotation evidence", evidence)
	}
	if !strings.Contains(evidence[0].Message, "box.value has type nil") ||
		!strings.Contains(evidence[1].Message, "after is declared as string") ||
		!strings.Contains(d.Explanation.String(), "no proof on this path shows box.value is string") {
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

func TestAssignmentAcceptsDeclaredMapSlotMemberWrite(t *testing.T) {
	diags := runDiagnostics(t, `
		type FileSlot = {kind: "file", path: string}
		type TimerSlot = {kind: "timer", seconds: number}
		type Slot = {value: FileSlot | TimerSlot}
		type Slots = {[string]: Slot}

		local slots: Slots = {
			active = {
				value = {kind = "file", path = "/tmp/active"},
			},
		}
		local key = "active"
		slots[key].value = {kind = "timer", seconds = 20}
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want no optional-target error for declared non-optional map base", diags)
	}
}

func TestAssignmentReportsOptionalMapBaseBeforeSlotMemberWrite(t *testing.T) {
	diags := runDiagnostics(t, `
		type Slot = {value: string}
		type Slots = {[string]: Slot}
		function f(slots: Slots?)
			slots["active"].value = "ready"
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want optional map-base assignment target error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeOptionalAssignmentTarget || !strings.Contains(d.Message, "cannot assign through optional slots") {
		t.Fatalf("diagnostic = %#v, want optional slots container error", d)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "writing slots[\"active\"].value requires its container to be non-nil") {
		t.Fatalf("evidence = %#v, want write requirement for full target", d.Explanation.Evidence())
	}
}

func TestAssignmentAcceptsDeclaredNestedMapWrite(t *testing.T) {
	diags := runDiagnostics(t, `
		type State = {plugin_counts: {[string]: integer}}
		function f(state: State, key: string)
			local current = state.plugin_counts[key] or 0
			state.plugin_counts[key] = current + 1
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want declared non-optional nested map write accepted", diags)
	}
}

func assertAssignmentPathEvidence(t *testing.T, d diagnostic.Diagnostic, source, target, missingProof, help string) {
	t.Helper()
	evidence := d.Explanation.Evidence()
	if !diagnosticEvidenceContains(evidence, source) ||
		!diagnosticEvidenceContains(evidence, target) ||
		!diagnosticEvidenceContains(evidence, missingProof) {
		t.Fatalf("evidence = %#v, want source %q, target %q, and missing-proof %q", evidence, source, target, missingProof)
	}
	if !strings.Contains(d.Help, help) {
		t.Fatalf("help = %q, want %q", d.Help, help)
	}
}
