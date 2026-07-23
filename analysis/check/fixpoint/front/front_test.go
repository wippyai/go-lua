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
	byKind := equationsByKind(artifact)
	if len(byKind["entry"]) != 1 || operands(byKind["entry"][0])["entry"] != "entry" {
		t.Fatalf("entry lowering = %#v", byKind["entry"])
	}
	if len(byKind["branch-relations"]) != 1 || operands(byKind["branch-relations"][0])["condition"] != "scalar/bool/true" {
		t.Fatalf("branch lowering = %#v", byKind["branch-relations"])
	}
	writes := make(map[string]equation.Equation)
	for _, write := range byKind["environment-write"] {
		writes[operands(write)["display"]] = write
	}
	if len(writes) != 3 {
		t.Fatalf("assignment writes = %#v, want first, second, and third", writes)
	}
	first, second, third := operands(writes["first"]), operands(writes["second"]), operands(writes["third"])
	if first["value"] != "scalar/number/1" || first["absence"] != "front/absence/error" || second["value"] != first["target"] || third["value"] != second["target"] {
		t.Fatalf("assignment value flow = first=%#v second=%#v third=%#v", first, second, third)
	}
	if guards := writes["third"].Guards; len(guards) != 1 || !strings.HasSuffix(string(guards[0].Encoding), "/true") {
		t.Fatalf("branch-local third write guards = %#v", guards)
	}
}

func TestCompileBodyLowersClaimAsCheckedRefinement(t *testing.T) {
	artifact, err := front.CompileBody(`
local source = nil
local value = source!
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "claim" {
			continue
		}
		got := operands(operation)
		if got["target"] == "" || got["value"] == "" || got["kind"] != "claim-kind/2" || got["type"] != "claim-type/non-nil" || got["display"] != "value" {
			t.Fatalf("claim operands = %#v", got)
		}
		return
	}
	t.Fatal("claim occurrence missing")
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

func TestCompileFreezesCyclesContainingClaims(t *testing.T) {
	for name, source := range map[string]string{
		"typed local": `
local i = 0
while i < 1 do
    local value: number = i
    i = i + 1
end`,
		"non-nil assertion": `
local i = 0
while i < 1 do
    local value = i!
    i = i + 1
end`,
		"typed table": `
local i = 0
while i < 1 do
    type Counter = {value: number}
    local value: Counter = {value = i}
    i = i + 1
end`,
	} {
		t.Run(name, func(t *testing.T) {
			compilation, err := front.Compile(source)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if compilation.Cyclic == nil || compilation.Cyclic.Plan == nil {
				t.Fatalf("cyclic claim body has no frozen certificate: %#v", compilation)
			}
		})
	}
}

func TestCompileBodyLowersNumericForBoundsAndBinding(t *testing.T) {
	for name, test := range map[string]struct {
		source       string
		wantState    string
		wantControl  string
		wantPrewrite bool
	}{
		"implicit step":   {source: `for i = 1, 3 do local value = i end`, wantState: "scalar/number/3", wantControl: "scalar/number/1"},
		"explicit step":   {source: `for i = 1, 6, 2 do local value = i end`, wantState: "scalar/number/6", wantControl: "scalar/number/2"},
		"computed bounds": {source: `local limit = 3; for i = 1, limit do local value = i end`, wantControl: "scalar/number/1", wantPrewrite: true},
	} {
		t.Run(name, func(t *testing.T) {
			artifact, err := front.CompileBody(test.source)
			if err != nil {
				t.Fatalf("CompileBody: %v", err)
			}
			byKind := equationsByKind(artifact)
			if len(byKind["generic-for"]) != 1 {
				t.Fatalf("numeric for lowering = %#v", byKind["generic-for"])
			}
			loop := operands(byKind["generic-for"][0])
			if loop["iterator"] != "scalar/number/1" || loop["control"] != test.wantControl || loop["display-00000000"] != "i" || !strings.HasPrefix(loop["result-00000000"], "path/") {
				t.Fatalf("numeric for operands = %#v", loop)
			}
			if test.wantPrewrite {
				if len(byKind["environment-write"]) != 2 {
					t.Fatalf("computed numeric-for writes = %#v", byKind["environment-write"])
				}
				limit := operands(byKind["environment-write"][0])
				if limit["display"] != "limit" || limit["value"] != "scalar/number/3" || loop["state"] != limit["target"] {
					t.Fatalf("computed bound lowering = loop=%#v limit=%#v", loop, limit)
				}
			} else if loop["state"] != test.wantState {
				t.Fatalf("numeric for state = %q, want %q", loop["state"], test.wantState)
			}
			value := operands(byKind["environment-write"][len(byKind["environment-write"])-1])
			if value["display"] != "value" || value["value"] != loop["result-00000000"] {
				t.Fatalf("numeric for binding = loop=%#v write=%#v", loop, value)
			}
		})
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
	byKind := equationsByKind(artifact)
	if len(byKind["apply"]) != 1 || len(byKind["call-results"]) != 1 || len(byKind["environment-write"]) != 1 {
		t.Fatalf("call lowering = %#v", byKind)
	}
	apply, results, write := operands(byKind["apply"][0]), operands(byKind["call-results"][0]), operands(byKind["environment-write"][0])
	if !strings.HasPrefix(apply["callee"], "path/") || apply["context"] != "call-context/2" || apply["expanded"] != "scalar/bool/true" || results["result-00000000"] != "temp/0" || results["target-00000000"] == "" || write["display"] != "value" || write["value"] != results["result-00000000"] {
		t.Fatalf("call result carrier lowering = apply=%#v results=%#v write=%#v", apply, results, write)
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

func TestCompileBodyKeepsTemporaryBindingForStaticMemberWrite(t *testing.T) {
	artifact, err := front.CompileBody(`
function module.answer()
    return 7
end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "path-replacement" {
			continue
		}
		if got := operands(operation)["value"]; got != "temp/0" {
			t.Fatalf("static member temporary = %q, want first temporary binding temp/0", got)
		}
		return
	}
	t.Fatal("static member write occurrence missing")
}

func TestCompileBodyLowersAdjustedOpenReturnTailSlots(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   map[string]string
	}{
		{name: "open tail", source: `return provider()`, want: map[string]string{"return-value-00000000": "temp/0"}},
		{name: "prefix and open tail", source: `return "prefix", provider()`, want: map[string]string{"return-value-00000000": `scalar/string/"prefix"`, "return-value-00000001": "temp/0"}},
		{name: "parenthesized tail is adjusted", source: `return (provider())`, want: map[string]string{"return-value-00000000": "temp/0"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifact, err := front.CompileBody(test.source)
			if err != nil {
				t.Fatalf("CompileBody: %v", err)
			}
			for _, operation := range artifact.Equations {
				if operation.Occurrence.Kind != "publication" {
					continue
				}
				if got := operands(operation); !mapsEqual(got, test.want) {
					t.Fatalf("publication slots = %#v, want %#v", got, test.want)
				}
				return
			}
			t.Fatal("publication occurrence missing")
		})
	}
}

func mapsEqual(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

func TestCompileBodyLowersCompleteTableAllocationGraph(t *testing.T) {
	artifact, err := front.CompileBody(`
local seed = 1
local object = { seed, enabled = false, child = { answer = 42 } }
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	byKind := equationsByKind(artifact)
	if len(byKind["allocation-template"]) != 2 || len(byKind["object-materialization"]) != 2 {
		t.Fatalf("allocation lowering = %#v", byKind)
	}
	var outerTemplate, outerMaterialization map[string]string
	for _, lowered := range byKind["allocation-template"] {
		roles := operands(lowered)
		if roles["value-00000001"] == "scalar/bool/false" {
			outerTemplate = roles
		}
	}
	for _, lowered := range byKind["object-materialization"] {
		roles := operands(lowered)
		if roles["member-00000001"] == "member/.enabled/scalar/bool/false" {
			outerMaterialization = roles
		}
	}
	if outerTemplate == nil || outerMaterialization == nil || outerTemplate["site"] != outerMaterialization["site"] || outerTemplate["result"] != outerMaterialization["result"] || outerMaterialization["list-floor"] != "list-floor/1" || outerMaterialization["member-00000000"] == "" || outerMaterialization["member-00000002"] == "" || outerMaterialization["member-00000003"] != "member/.child.answer/scalar/number/42" {
		t.Fatalf("complete outer table graph = template=%#v materialization=%#v", outerTemplate, outerMaterialization)
	}
}

func TestCompileBodyAdmitsOpenTableTailWithoutClaimingClosedShape(t *testing.T) {
	artifact, err := front.CompileBody(`local values = { provider() }`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	byKind := equationsByKind(artifact)
	for _, kind := range []string{"allocation-template", "object-materialization"} {
		if len(byKind[kind]) != 1 {
			t.Fatalf("%s lowering = %#v", kind, byKind[kind])
		}
		roles := operands(byKind[kind][0])
		if roles["open-tail"] != "scalar/bool/true" || roles["tail"] == "" {
			t.Fatalf("%s open-tail operands = %#v", kind, roles)
		}
	}
	for _, write := range byKind["environment-write"] {
		roles := operands(write)
		if roles["target"] == "" || roles["value"] != "scalar/table" {
			continue
		}
		return
	}
	t.Fatalf("open-tail table was given a finite shape: %#v", byKind["environment-write"])
}

func TestCompileAdmitsNestedVarargTableBody(t *testing.T) {
	compilation, err := front.Compile(`
local function collect(...)
    local values = { ... }
    return values
end
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(compilation.Nested) != 1 {
		t.Fatalf("nested compilations = %d, want one", len(compilation.Nested))
	}
	byKind := equationsByKind(compilation.Nested[0].Artifact)
	if len(byKind["allocation-template"]) != 1 || len(byKind["object-materialization"]) != 1 {
		t.Fatalf("nested allocation lowering = %#v", byKind)
	}
	for _, kind := range []string{"allocation-template", "object-materialization"} {
		roles := operands(byKind[kind][0])
		if roles["open-tail"] != "scalar/bool/true" || roles["tail"] != "vararg" {
			t.Fatalf("nested %s tail = %#v", kind, roles)
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

func equationsByKind(artifact equation.Artifact) map[string][]equation.Equation {
	result := make(map[string][]equation.Equation)
	for _, lowered := range artifact.Equations {
		result[lowered.Occurrence.Kind] = append(result[lowered.Occurrence.Kind], lowered)
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
	for name, source := range map[string]string{
		"path destination":      `local key = "missing"; local result = record[key]`,
		"temporary destination": `local key = "missing"; local result = record[key].field`,
		"nested dynamic key":    `local first = "one"; local second = "two"; local result = record[first][second]`,
	} {
		t.Run(name, func(t *testing.T) {
			artifact, err := front.CompileBody(source)
			if err != nil {
				t.Fatalf("CompileBody: %v", err)
			}
			for _, lowered := range artifact.Equations {
				if lowered.Occurrence.Kind != "dynamic-index-read" {
					continue
				}
				got := operands(lowered)
				if got["container"] == "" || got["key"] == "" || got["target"] == "" {
					t.Fatalf("dynamic read omitted an operand: %#v", got)
				}
				if _, hasValue := got["value"]; hasValue {
					t.Fatalf("dynamic read invented a concrete value: %#v", got)
				}
				return
			}
			t.Fatal("missing dynamic-index-read occurrence")
		})
	}
}

func TestCompileBodyKeepsUntargetedCallResultsAsWholeTuples(t *testing.T) {
	for name, source := range map[string]string{
		"pairs iterator":   `for key, value in pairs(record) do end`,
		"ipairs iterator":  `for key, value in ipairs(record) do end`,
		"generic iterator": `for value in iterator() do end`,
	} {
		t.Run(name, func(t *testing.T) {
			artifact, err := front.CompileBody(source)
			if err != nil {
				t.Fatalf("CompileBody: %v", err)
			}
			for _, lowered := range artifact.Equations {
				if lowered.Occurrence.Kind != "call-results" {
					continue
				}
				got := operands(lowered)
				if got["application"] == "" || got["result-00000000"] == "" {
					t.Fatalf("untargeted call lost result tuple: %#v", got)
				}
				if _, partialTarget := got["target-00000000"]; partialTarget {
					t.Fatalf("untargeted call emitted partial target metadata: %#v", got)
				}
				return
			}
			t.Fatal("missing call-results occurrence")
		})
	}
}

func TestCompileBodyAdmitsTemporaryAssignmentDestinations(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		reads  int
	}{
		"call member chain":    {source: `local result = provider().field`, reads: 1},
		"nested dynamic read":  {source: `local key = "field"; local result = record[key].field`, reads: 2},
		"logical member chain": {source: `local result = (primary or fallback).field`, reads: 1},
	} {
		t.Run(name, func(t *testing.T) {
			artifact, err := front.CompileBody(test.source)
			if err != nil {
				t.Fatalf("CompileBody: %v", err)
			}
			reads := equationsByKind(artifact)["dynamic-index-read"]
			if len(reads) != test.reads {
				t.Fatalf("temporary destination reads = %#v, want %d", reads, test.reads)
			}
			last := operands(reads[len(reads)-1])
			if !strings.HasPrefix(last["target"], "path/") || last["key"] != `scalar/string/"field"` {
				t.Fatalf("temporary destination lowering = %#v", last)
			}
			if name == "call member chain" && last["container"] != "temp/0" {
				t.Fatalf("call member did not index its call-result temporary: %#v", last)
			}
			if name == "logical member chain" && last["container"] != "temp/0" {
				t.Fatalf("logical member did not index its expression temporary: %#v", last)
			}
		})
	}
}

func TestCompileBodyAdmitsUnknownDynamicIndexWriteContainers(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		key    string
	}{
		"call result":         {source: `factory()["key"] = 1`, key: `scalar/string/"key"`},
		"call member":         {source: `factory().items["key"] = 1`, key: `scalar/string/"key"`},
		"dynamic then member": {source: `factory()["key"].field = 1`, key: `scalar/string/"field"`},
	} {
		t.Run(name, func(t *testing.T) {
			artifact, err := front.CompileBody(test.source)
			if err != nil {
				t.Fatalf("CompileBody: %v", err)
			}
			byKind := equationsByKind(artifact)
			if len(byKind["path-invalidation"]) != 1 || len(byKind["index-mutation"]) != 1 {
				t.Fatalf("unknown container write lowering = %#v", byKind)
			}
			invalidation, mutation := operands(byKind["path-invalidation"][0]), operands(byKind["index-mutation"][0])
			if invalidation["container"] != "scalar/top" || invalidation["key"] != test.key || invalidation["suffix"] != "suffix/" || mutation["container"] != invalidation["container"] || mutation["key"] != invalidation["key"] || mutation["suffix"] != invalidation["suffix"] || mutation["value"] != "scalar/number/1" {
				t.Fatalf("unknown container write operands = invalidation=%#v mutation=%#v", invalidation, mutation)
			}
		})
	}
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
	byKind := equationsByKind(artifact)
	if len(byKind["apply"]) != 1 || len(byKind["call-results"]) != 1 || len(byKind["branch-relations"]) != 1 || len(byKind["environment-write"]) != 2 {
		t.Fatalf("call selector lowering = %#v", byKind)
	}
	apply, results, branch := operands(byKind["apply"][0]), operands(byKind["call-results"][0]), operands(byKind["branch-relations"][0])
	if apply["callee"] == "" || apply["context"] != "call-context/5" || apply["adjusted"] != "scalar/bool/true" || results["result-00000000"] != "temp/0" || branch["condition"] != results["result-00000000"] {
		t.Fatalf("call-selector value flow = apply=%#v results=%#v branch=%#v", apply, results, branch)
	}
	selected := operands(byKind["environment-write"][1])
	if selected["display"] != "selected" || selected["value"] != "scalar/bool/true" || len(byKind["environment-write"][1].Guards) != 1 {
		t.Fatalf("selected-arm write = %#v guards=%#v", selected, byKind["environment-write"][1].Guards)
	}
}
