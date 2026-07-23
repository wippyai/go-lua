package front_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

func TestCompileBodyLowersScalarAssignmentAndBranchSlice(t *testing.T) {
	artifact, err := front.CompileBody(`
local first = 1
local second = first
if true then
    local third = second
end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	if artifact.CanonicalBytes() == nil {
		t.Fatal("CompileBody returned a non-canonical artifact")
	}
	got := make(map[string]int)
	for _, equation := range artifact.Equations {
		got[equation.Occurrence.Kind]++
	}
	if got["entry"] != 1 || got["environment-write"] != 3 || got["branch-relations"] != 1 {
		t.Fatalf("lowered occurrence kinds = %#v", got)
	}
}

func TestCompileBodyLowersGenericForWithAdjustedTupleAndBindings(t *testing.T) {
	artifact, err := front.CompileBody(`
for key, value in next do
end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	var loopOperands map[string]string
	for _, lowered := range artifact.Equations {
		if lowered.Occurrence.Kind != "generic-for" {
			continue
		}
		loopOperands = operands(lowered)
	}
	if loopOperands == nil {
		t.Fatalf("generic-for occurrence missing from %#v", artifact.Equations)
	}
	for role, want := range map[string]string{"state": "scalar/nil", "control": "scalar/nil", "display-00000000": "key", "display-00000001": "value"} {
		if got := loopOperands[role]; got != want {
			t.Errorf("generic-for operand %s = %q, want %q", role, got, want)
		}
	}
	for _, role := range []string{"iterator", "result-00000000", "result-00000001"} {
		if !strings.HasPrefix(loopOperands[role], "path/") {
			t.Errorf("generic-for operand %s = %q, want closed path", role, loopOperands[role])
		}
	}
}

func TestCompileFreezesNonGenericCycles(t *testing.T) {
	compilation, err := front.Compile(`
while true do
end
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if compilation.Cyclic == nil || compilation.Cyclic.Plan == nil {
		t.Fatalf("Compile did not retain a cyclic certificate: %#v", compilation)
	}
	if compilation.Cyclic.Plan.ComponentCount() == 0 {
		t.Fatal("cyclic certificate has no WTO component")
	}
}

func TestCompileBodyLowersCallsAndTheirResultCarriers(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		apply       int
		results     int
		role        string
		wantGuard   bool
		wantContext string
	}{
		{name: "direct assignment", source: `local value = invoke("argument")`, apply: 1, results: 1, role: "callee", wantContext: "call-context/2"},
		{name: "method assignment", source: `local value = receiver:invoke(1)`, apply: 1, results: 1, role: "receiver", wantContext: "call-context/2"},
		{name: "statement result discard", source: `invoke()`, apply: 1, results: 1, role: "callee", wantContext: "call-context/1"},
		{name: "nested call", source: `local value = outer(inner(1))`, apply: 2, results: 2, role: "callee", wantContext: "call-context/6"},
		{name: "condition call is guarded by selected branch", source: `
if predicate() then
    local value = invoke()
end
`, apply: 2, results: 2, role: "callee", wantGuard: true, wantContext: "call-context/5"},
		{name: "expanded final result list", source: `local first, second = invoke()`, apply: 1, results: 1, role: "callee", wantContext: "call-context/2"},
		{name: "adjusted parenthesized result", source: `local first, second = (invoke())`, apply: 1, results: 1, role: "callee", wantContext: "call-context/2"},
		{name: "assert predicate call", source: `assert(value)`, apply: 1, results: 1, role: "check", wantContext: "call-context/1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifact, err := front.CompileBody(test.source)
			if err != nil {
				t.Fatalf("CompileBody: %v", err)
			}
			applies, results := 0, 0
			var firstApply, firstResults *struct {
				operands map[string]string
				guards   int
			}
			for _, operation := range artifact.Equations {
				operands := make(map[string]string, len(operation.Operands))
				for _, operand := range operation.Operands {
					operands[operand.Role] = string(operand.Term.Encoding)
				}
				switch operation.Occurrence.Kind {
				case "apply":
					applies++
					if firstApply == nil {
						firstApply = &struct {
							operands map[string]string
							guards   int
						}{operands: operands, guards: len(operation.Guards)}
					}
				case "call-results":
					results++
					if firstResults == nil {
						firstResults = &struct {
							operands map[string]string
							guards   int
						}{operands: operands, guards: len(operation.Guards)}
					}
				}
			}
			if applies != test.apply || results != test.results {
				t.Fatalf("apply/results = %d/%d, want %d/%d", applies, results, test.apply, test.results)
			}
			if firstApply == nil || firstApply.operands[test.role] == "" {
				t.Fatalf("first apply operands = %#v, want role %q", firstApply, test.role)
			}
			if firstResults == nil || firstResults.operands["application"] == "" {
				t.Fatalf("first result operands = %#v, want application carrier", firstResults)
			}
			if test.wantContext != "" && firstApply.operands["context"] != test.wantContext {
				t.Fatalf("first call context = %q, want %q", firstApply.operands["context"], test.wantContext)
			}
			if test.wantGuard {
				guarded := false
				for _, operation := range artifact.Equations {
					if operation.Occurrence.Kind == "apply" && len(operation.Guards) > 0 {
						guarded = true
					}
				}
				if !guarded {
					t.Fatal("selected-arm call has no branch guard")
				}
			}
			if test.name == "expanded final result list" && firstApply.operands["expanded"] != "scalar/bool/true" {
				t.Fatalf("expanded call marker = %q", firstApply.operands["expanded"])
			}
			if test.name == "adjusted parenthesized result" && (firstApply.operands["adjusted"] != "scalar/bool/true" || firstApply.operands["result-spread"] != "scalar/bool/false") {
				t.Fatalf("adjusted call operands = %#v", firstApply.operands)
			}
			if test.name == "method assignment" && firstApply.operands["method"] != `method/"invoke"` {
				t.Fatalf("method operand = %q", firstApply.operands["method"])
			}
			if test.name == "assert predicate call" && !strings.HasPrefix(firstApply.operands["check"], "check/") {
				t.Fatalf("assert check = %q", firstApply.operands["check"])
			}
		})
	}
}

func TestCompileBodyAssignmentReadsShareStatementSnapshot(t *testing.T) {
	artifact, err := front.CompileBody(`
local left, right = 1, 2
left, right = right, left
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	var writes []equation.Equation
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind == "environment-write" {
			writes = append(writes, operation)
		}
	}
	if len(writes) != 4 {
		t.Fatalf("assignment equations = %d, want 4", len(writes))
	}
	readBoundary := func(operation equation.Equation) string {
		t.Helper()
		for _, operand := range operation.Operands {
			if operand.Role == "read-before" {
				return string(operand.Term.Encoding)
			}
		}
		t.Fatalf("assignment %s has no read-before operand", operation.Target.Name)
		return ""
	}
	if got, want := readBoundary(writes[2]), readBoundary(writes[3]); got != want {
		t.Fatalf("parallel assignment read boundaries = %q and %q, want one pre-write snapshot", got, want)
	}
	if got := readBoundary(writes[2]); got == readBoundary(writes[1]) {
		t.Fatalf("second assignment statement reused its prior statement boundary %q", got)
	}
}

func TestCompileBodyLowersCallInsteadOfRejectingIt(t *testing.T) {
	artifact, err := front.CompileBody(`local value = source()`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	got := occurrenceCounts(artifact)
	if got["apply"] != 1 || got["call-results"] != 1 {
		t.Fatalf("call occurrence kinds = %#v", got)
	}
}

func TestCompileBodyExternalCallAddsBoundaryFactorWithoutForkingCallPair(t *testing.T) {
	artifact, err := front.CompileBody(`
local library = require("catalog")
local kind = type(nil)
local row = library:fetch(kind)
collectgarbage("collect")
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	counts := occurrenceCounts(artifact)
	if counts["apply"] != 4 || counts["call-results"] != 4 || counts["external-call"] != 4 {
		t.Fatalf("external call ownership counts = %#v, want four apply/result pairs and four factors", counts)
	}
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "external-call" {
			continue
		}
		roles := operands(operation)
		if !strings.HasPrefix(roles["application"], "call/op-") || !strings.HasPrefix(roles["provider"], "provider/") {
			t.Fatalf("external boundary operands = %#v", roles)
		}
		if _, ownsSlot := roles["result-00000000"]; ownsSlot {
			t.Fatalf("external factor owns a result slot: %#v", roles)
		}
	}
}

func TestCompileBodyLowersExactPublicationSlots(t *testing.T) {
	artifact, err := front.CompileBody(`return 7, nil, false`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "publication" {
			continue
		}
		roles := operands(operation)
		if len(roles) != 3 || roles["return-value-00000000"] != "scalar/number/7" || roles["return-value-00000001"] != "scalar/nil" || roles["return-value-00000002"] != "scalar/bool/false" {
			t.Fatalf("publication slots = %#v", roles)
		}
		return
	}
	t.Fatal("publication occurrence missing")
}

func TestCompileBodyLowersCompleteTableAllocationGraph(t *testing.T) {
	artifact, err := front.CompileBody(`
local seed = 1
local object = { seed, enabled = false, child = { answer = 42 } }
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	got := occurrenceCounts(artifact)
	if got["allocation-template"] != 2 || got["object-materialization"] != 2 {
		t.Fatalf("allocation occurrence kinds = %#v", got)
	}
	for _, equation := range artifact.Equations {
		if equation.Occurrence.Kind != "object-materialization" {
			continue
		}
		if operand(equation.Operands, "site") == nil || operand(equation.Operands, "result") == nil {
			t.Fatalf("object materialization omitted exact site/result: %#v", equation.Operands)
		}
	}
}

func TestTableAllocationKeepsTemplateAndMaterializationOnOneSite(t *testing.T) {
	artifact, err := front.CompileBody(`local object = { answer = 42 }`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	var templateSite, materializedSite string
	for _, equation := range artifact.Equations {
		switch equation.Occurrence.Kind {
		case "allocation-template":
			templateSite = string(operand(equation.Operands, "site").Term.Encoding)
		case "object-materialization":
			materializedSite = string(operand(equation.Operands, "site").Term.Encoding)
		}
	}
	if templateSite == "" || templateSite != materializedSite {
		t.Fatalf("allocation sites = template %q, materialization %q", templateSite, materializedSite)
	}
}

func TestTableAllocationRepresentsNilAsAbsenceAndContiguousFloor(t *testing.T) {
	artifact, err := front.CompileBody(`local values = { 1, nil, 3, missing = nil }`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, equation := range artifact.Equations {
		if equation.Occurrence.Kind != "object-materialization" {
			continue
		}
		if got := string(operand(equation.Operands, "list-floor").Term.Encoding); got != "list-floor/1" {
			t.Fatalf("list floor = %q, want contiguous exact prefix of one", got)
		}
		for _, candidate := range equation.Operands {
			if strings.HasPrefix(candidate.Role, "member-") && strings.Contains(string(candidate.Term.Encoding), ".missing/") {
				t.Fatalf("nil field was materialized as a member: %#v", candidate)
			}
		}
	}
}

func TestCompileBodyGuardsAllocationWithItsBranch(t *testing.T) {
	artifact, err := front.CompileBody(`
if true then
    local object = { answer = 42 }
end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, equation := range artifact.Equations {
		if equation.Occurrence.Kind == "allocation-template" || equation.Occurrence.Kind == "object-materialization" {
			if len(equation.Guards) != 1 || !strings.HasSuffix(string(equation.Guards[0].Encoding), "/true") {
				t.Fatalf("guarded allocation guards = %#v", equation.Guards)
			}
		}
	}
}

func TestCompileBodyLowersClosureAllocationAndCaptures(t *testing.T) {
	artifact, err := front.CompileBody(`
local captured = "value"
local read = function() return captured end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	var materialized bool
	for _, equation := range artifact.Equations {
		if equation.Occurrence.Kind != "object-materialization" || string(operand(equation.Operands, "kind").Term.Encoding) != "object-kind/closure" {
			continue
		}
		materialized = true
		if operand(equation.Operands, "prototype") == nil || operand(equation.Operands, "capture-00000000") == nil {
			t.Fatalf("closure materialization lost its prototype or capture: %#v", equation.Operands)
		}
	}
	if !materialized {
		t.Fatal("missing closure object materialization")
	}
}

func TestCompileBodyRejectsInexactAllocationKeys(t *testing.T) {
	_, err := front.CompileBody(`
local key = "answer"
local object = { [key] = 42 }
`)
	if err == nil || !strings.Contains(err.Error(), "non-exact key") {
		t.Fatalf("CompileBody error = %v, want inexact allocation rejection", err)
	}
}

func occurrenceCounts(artifact equation.Artifact) map[string]int {
	result := make(map[string]int)
	for _, equation := range artifact.Equations {
		result[equation.Occurrence.Kind]++
	}
	return result
}

func operand(operands []equation.Operand, role string) *equation.Operand {
	for index := range operands {
		if operands[index].Role == role {
			return &operands[index]
		}
	}
	return nil
}

func TestCompileBodyLowersPathStoreInvalidationAndIndexMutationTogether(t *testing.T) {
	artifact, err := front.CompileBody(`
local key = "id"
record.label = false
record[key].state = nil
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	got := make(map[string][]equation.Equation)
	for _, equation := range artifact.Equations {
		got[equation.Occurrence.Kind] = append(got[equation.Occurrence.Kind], equation)
	}
	for kind, want := range map[string]int{
		"entry": 1, "environment-write": 1, "path-replacement": 1,
		"path-invalidation": 1, "index-mutation": 1,
	} {
		if count := len(got[kind]); count != want {
			t.Fatalf("%s occurrences = %d, want %d; all = %#v", kind, count, want, got)
		}
	}
	static := operands(got["path-replacement"][0])
	if static["value"] != "scalar/bool/false" {
		t.Fatalf("static replacement value = %q, want false", static["value"])
	}
	invalidation := operands(got["path-invalidation"][0])
	if invalidation["key"] == "" {
		t.Fatalf("invalidation lost the dynamic key: %#v", invalidation)
	}
	if invalidation["suffix"] != "suffix/.state" {
		t.Fatalf("invalidation suffix = %q, want suffix/.state", invalidation["suffix"])
	}
	mutation := operands(got["index-mutation"][0])
	if mutation["value"] != "scalar/nil" {
		t.Fatalf("index mutation value = %q, want scalar/nil", mutation["value"])
	}
	if mutation["suffix"] != invalidation["suffix"] || mutation["key"] != invalidation["key"] {
		t.Fatalf("paired path operations disagree: invalidation=%#v mutation=%#v", invalidation, mutation)
	}
}

func TestCompileBodyGuardsBothHalvesOfDynamicIndexWrite(t *testing.T) {
	artifact, err := front.CompileBody(`
local key = "id"
if true then
    record[key] = 3
end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, equation := range artifact.Equations {
		if equation.Occurrence.Kind != "path-invalidation" && equation.Occurrence.Kind != "index-mutation" {
			continue
		}
		if len(equation.Guards) != 1 {
			t.Fatalf("%s guards = %#v, want exactly the selected branch edge", equation.Occurrence.Kind, equation.Guards)
		}
	}
}

func TestCompileBodyLowersDynamicIndexReadWithoutTreatingNilAsAbsence(t *testing.T) {
	artifact, err := front.CompileBody(`
local key = "missing"
local result = record[key]
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, equation := range artifact.Equations {
		if equation.Occurrence.Kind != "path-replacement" {
			continue
		}
		got := operands(equation)
		if got["container"] == "" || got["key"] == "" || got["target"] == "" {
			t.Fatalf("dynamic read omitted an operand: %#v", got)
		}
		if _, hasValue := got["value"]; hasValue {
			t.Fatalf("dynamic read invented a replacement value instead of preserving lookup semantics: %#v", got)
		}
		return
	}
	t.Fatal("missing path replacement for dynamic index read")
}

func operands(equation equation.Equation) map[string]string {
	result := make(map[string]string, len(equation.Operands))
	for _, operand := range equation.Operands {
		result[operand.Role] = string(operand.Term.Encoding)
	}
	return result
}

func TestCompileBodyLowersNormalizedBranchPredicate(t *testing.T) {
	artifact, err := front.CompileBody(`
local status = "ready"
if status == "ready" then
    local result = "selected"
end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "branch-relations" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "predicate" {
				if !strings.HasPrefix(string(operand.Term.Encoding), "front/branch-predicate/v1/") {
					t.Fatalf("predicate encoding = %q", operand.Term.Encoding)
				}
				return
			}
		}
		t.Fatal("normalized branch had no predicate operand")
	}
	t.Fatal("artifact had no branch relation")
}

func TestCompileBodyLowersBranchWithCallSelector(t *testing.T) {
	artifact, err := front.CompileBody(`
local function predicate()
    return true
end
if predicate() then
    local selected = true
end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	got := occurrenceCounts(artifact)
	if got["apply"] != 1 || got["call-results"] != 1 || got["branch-relations"] != 1 {
		t.Fatalf("call selector occurrence kinds = %#v", got)
	}
}
