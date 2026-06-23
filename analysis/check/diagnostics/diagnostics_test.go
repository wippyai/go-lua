package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFormatTypeUsesBoundedDiagnosticFormatter(t *testing.T) {
	fields := make([]typ.Field, 0, typeformat.DefaultOptions.MaxRecordFields+16)
	for i := 0; i < cap(fields); i++ {
		fields = append(fields, typ.Field{
			Name: strings.Repeat("deep_field_", 8) + string(rune('a'+i)),
			Type: typ.NewMap(typ.String, &typ.Record{Fields: []typ.Field{
				{Name: "nested", Type: typ.String},
			}}),
		})
	}

	unionMembers := make([]typ.Type, 0, typeformat.DefaultOptions.MaxUnionMembers+8)
	for i := 0; i < cap(unionMembers); i++ {
		unionMembers = append(unionMembers, &typ.Record{Fields: []typ.Field{
			{Name: "kind", Type: typ.LiteralString("case_" + string(rune('a'+i)))},
			{Name: "payload", Type: &typ.Record{Fields: fields[:2]}},
		}})
	}

	fn := typ.Func()
	for i := 0; i < typeformat.DefaultOptions.MaxParams+8; i++ {
		fn.Param("param_"+string(rune('a'+i)), &typ.Record{Fields: fields[:2]})
	}
	returns := make([]typ.Type, 0, typeformat.DefaultOptions.MaxReturns+8)
	for i := 0; i < cap(returns); i++ {
		returns = append(returns, &typ.Record{Fields: fields[:2]})
	}
	fn.Returns(returns...)

	for name, tp := range map[string]typ.Type{
		"record":   &typ.Record{Fields: fields},
		"union":    typ.MaterializeUnion(unionMembers),
		"function": fn.Build(),
	} {
		got := formatType(tp)
		if len(got) > typeformat.DefaultOptions.MaxBytes {
			t.Fatalf("%s formatted diagnostic type length = %d, want <= %d: %q", name, len(got), typeformat.DefaultOptions.MaxBytes, got)
		}
		if !strings.Contains(got, "...") {
			t.Fatalf("%s formatted diagnostic type = %q, want truncation marker", name, got)
		}
	}
}

func TestAnnotationAssignabilityReportsLiteralMismatch(t *testing.T) {
	diags := runDiagnostics(t, `local x: number = "no"`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot assign") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want source and annotation evidence", d.Explanation.Evidence())
	}
}

func TestProduceWithConfigAppliesDiagnosticPolicy(t *testing.T) {
	result := runDiagnosticsResult(t, `local x: number = "no"`)
	disabled := ProduceWithConfig(result, Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeAssignmentType: diagnostic.Disable(),
	}}})
	if len(disabled) != 0 {
		t.Fatalf("disabled diagnostics = %#v, want none", disabled)
	}

	remapped := ProduceWithConfig(result, Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeAssignmentType: diagnostic.OverrideSeverity(diagnostic.SeverityHint),
	}}})
	if len(remapped) != 1 {
		t.Fatalf("remapped diagnostics = %#v, want one diagnostic", remapped)
	}
	if remapped[0].Code != CodeAssignmentType || remapped[0].Severity != diagnostic.SeverityHint {
		t.Fatalf("remapped diagnostic = %#v, want assignment diagnostic with hint severity", remapped[0])
	}
}

func TestDiagnosticProducerRegistryDeclaresPolicyDefaults(t *testing.T) {
	optInCodes := map[diagnostic.Code]struct{}{
		CodeUnusedLocal:                  {},
		CodeDeadAssignment:               {},
		CodeRedundantCondition:           {},
		CodeDiscriminatedUnionExhaustive: {},
		CodeFrozenTableMutation:          {},
		CodeResourceUnreleased:           {},
	}
	allCodes := []diagnostic.Code{
		CodeAssignmentType,
		CodeMissingMember,
		CodeOptionalMethodCall,
		CodeNotCallable,
		CodeDirectCallNotCallable,
		CodeDirectCallTooFewArgs,
		CodeDirectCallTooManyArgs,
		CodeDirectCallArgType,
		CodeReturnContractType,
		CodeDirectCallResultAssignment,
		CodeOptionalAssignmentTarget,
		CodeConcatOperand,
		CodeNumericForOperand,
		CodeChannelSelectExhaustive,
		CodeUnresolvedTypeReference,
		CodeUnresolvedValueReference,
		CodeUnusedLocal,
		CodeDeadAssignment,
		CodeRedundantCondition,
		CodeDiscriminatedUnionExhaustive,
		CodeFrozenTableMutation,
		CodeResourceUnreleased,
	}

	declared := make(map[diagnostic.Code]struct{})
	for i, producer := range diagnosticProducers() {
		if producer.produce == nil {
			t.Fatalf("producer %d has nil produce function", i)
		}
		if len(producer.codes) == 0 {
			t.Fatalf("producer %d must declare emitted diagnostic codes", i)
		}
		for _, code := range producer.codes {
			declared[code] = struct{}{}
			_, isOptIn := optInCodes[code]
			if !producer.defaultEnabled && !isOptIn {
				t.Fatalf("producer %d marks default-enabled code %s as opt-in", i, code)
			}
		}
	}
	for _, code := range allCodes {
		if _, ok := declared[code]; !ok {
			t.Fatalf("diagnostic code %s is not declared by any producer", code)
		}
	}

	required := diagnosticProducer{codes: []diagnostic.Code{CodeAssignmentType}, defaultEnabled: true}
	if !required.shouldRun(diagnostic.Policy{}) {
		t.Fatalf("default-enabled producer should run without an explicit policy")
	}
	if required.shouldRun(diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{CodeAssignmentType: diagnostic.Disable()}}) {
		t.Fatalf("default-enabled producer should stop when all emitted codes are disabled")
	}
	optIn := diagnosticProducer{codes: []diagnostic.Code{CodeUnusedLocal}, defaultEnabled: false}
	if optIn.shouldRun(diagnostic.Policy{}) {
		t.Fatalf("opt-in producer should not run without explicit enablement")
	}
	if optIn.shouldRun(diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{CodeUnusedLocal: diagnostic.OverrideSeverity(diagnostic.SeverityHint)}}) {
		t.Fatalf("severity override alone should not enable an opt-in producer")
	}
	if !optIn.shouldRun(diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{CodeUnusedLocal: diagnostic.Enable()}}) {
		t.Fatalf("opt-in producer should run when explicitly enabled")
	}
}

func TestProduceOrdersDiagnosticsBySourcePositionAcrossProducers(t *testing.T) {
	diags := runDiagnostics(t, `
		local maybe: (() -> string)? = nil
		local from_call = maybe()
		local later: number = "wrong"
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %#v, want direct-call and assignment diagnostics", diags)
	}
	if diags[0].Code != CodeDirectCallNotCallable || diags[0].Position.Line >= diags[1].Position.Line {
		t.Fatalf("diagnostic order = %#v, want earlier direct-call diagnostic before later assignment", diagnosticMessages(diags))
	}
	if diags[1].Code != CodeAssignmentType {
		t.Fatalf("second diagnostic = %#v, want assignment diagnostic", diags[1])
	}
}

func diagnosticEvidenceContains(evidence []diagnostic.Evidence, want string) bool {
	for _, item := range evidence {
		if strings.Contains(item.Message, want) {
			return true
		}
	}
	return false
}

func diagnosticHasLabel(d diagnostic.Diagnostic, want string) bool {
	for _, label := range d.Labels {
		if strings.Contains(label.Message, want) {
			return true
		}
	}
	return false
}

func diagnosticLabelCount(d diagnostic.Diagnostic, want string) int {
	count := 0
	for _, label := range d.Labels {
		if strings.Contains(label.Message, want) {
			count++
		}
	}
	return count
}

func TestAnnotationAssignabilityAcceptsSubtypeLiteral(t *testing.T) {
	diags := runDiagnostics(t, `local x: number = 42`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestExplicitNilFieldFreshAssignableSuppressesMismatch(t *testing.T) {
	got := typetable.NewRecord().
		Field("id", typ.String).
		Field("error", typ.Nil).
		Build()
	want := typetable.NewRecord().
		Field("id", typ.String).
		Field("error", typeexpr.Optional(typ.String)).
		Build()

	if !explicitNilFieldFreshAssignable(got, typeexpr.Optional(want)) {
		t.Fatal("explicit nil field should be fresh-assignable to nilable field contract")
	}
}

func TestAnnotationAssignabilityReportsNominalGenericArgumentMismatch(t *testing.T) {
	diags := runDiagnostics(t, `
function f(ch: Channel<string>)
    local bad: Channel<number> = ch
end
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", diags[0].Code, CodeAssignmentType)
	}
}

func TestAnnotationAssignabilityReportsRecursiveUnionArrayElementMismatch(t *testing.T) {
	diags := runDiagnostics(t, `
type TextNode = { kind: "text", value: string }
type GroupNode = { kind: "group", children: {TreeNode} }
type TreeNode = TextNode | GroupNode

function f(tree: TreeNode)
    if tree.kind == "group" then
        local first = tree.children[1]
        if first and first.kind == "text" then
            local value: string = first.value
            local bad_value: number = first.value
        end
    end
    if tree.kind == "text" then
        local children = tree.children
    end
end
`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	messages := make([]string, 0, len(diags))
	for _, diag := range diags {
		messages = append(messages, diag.Message)
	}
	if !containsDiagnosticMessage(messages, "cannot assign first.value because it is string, not number") ||
		!containsDiagnosticMessage(messages, `has no member "children"`) {
		t.Fatalf("diagnostics = %#v, want first.value mismatch and text.children missing-member", messages)
	}
}

func TestAnnotationAssignabilityReportsArrayLiteralElementMismatch(t *testing.T) {
	diags := runDiagnostics(t, `local arr: {number} = {1, "two", 3}`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, `"two"`) || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestAnnotationAssignabilityAcceptsHomogeneousArrayLiteral(t *testing.T) {
	diags := runDiagnostics(t, `local arr: {number} = {1, 2, 3}`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestAnnotationAssignabilityAcceptsTableInsertLengthFloorArrayIndex(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
local arr: {number} = {}
table.insert(arr, 1)
table.insert(arr, 2)
local n: number = arr[1]
`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestAnnotationAssignabilityAdvancesLengthFloorAcrossTableInserts(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
local arr: {number} = {}
table.insert(arr, 1)
table.insert(arr, 2)
table.insert(arr, 3)
local n: number = arr[3]
`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after three positive length deltas", diags)
	}
}

func TestAnnotationAssignabilityKeepsOptionalityPastTableInsertLengthFloor(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
local arr: {number} = {}
table.insert(arr, 1)
table.insert(arr, 2)
local n: number = arr[3]
`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want out-of-floor array index error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot assign arr[3]") || !strings.Contains(d.Message, "may be nil") || !strings.Contains(d.Explanation.String(), "no guard on this path proves arr[3] is non-nil") {
		t.Fatalf("diagnostic = %#v, want optional array index with missing-proof evidence", d)
	}
}

func TestAnnotationAssignabilityKeepsOptionalityForZeroArrayIndex(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
local arr: {number} = {}
table.insert(arr, 1)
table.insert(arr, 2)
local n: number = arr[0]
`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want zero-index array error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot assign arr[0]") || !strings.Contains(d.Message, "may be nil") || !strings.Contains(d.Explanation.String(), "no guard on this path proves arr[0] is non-nil") {
		t.Fatalf("diagnostic = %#v, want optional zero-index read with missing-proof evidence", d)
	}
}

func TestAnnotationAssignabilityReportsRootLiteralArrayReadWithoutLengthProof(t *testing.T) {
	diags := runDiagnostics(t, `
local function bad(): number
	local xs: {number} = {}
	return xs[1]
end
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want root literal array read error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeReturnContractType && d.Code != CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want return or assignment type diagnostic: %#v", d.Code, d)
	}
	if !strings.Contains(d.Message, "cannot return xs[1] as returned value 1") ||
		!strings.Contains(d.Message, "may be nil") ||
		!strings.Contains(d.Explanation.String(), "no proof on this path shows returned value 1 (xs[1]) satisfies the declared return type") {
		t.Fatalf("diagnostic = %#v, want optional root array read with missing-proof evidence", d)
	}
}

func TestAnnotationAssignabilityInvalidatesLengthFloorAfterTableRemove(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
local arr: {number} = {}
table.insert(arr, 1)
table.insert(arr, 2)
table.remove(arr)
local n: number = arr[2]
`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want post-remove stale length-floor error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot assign arr[2]") || !strings.Contains(d.Message, "may be nil") || !strings.Contains(d.Explanation.String(), "no guard on this path proves arr[2] is non-nil") {
		t.Fatalf("diagnostic = %#v, want optional post-remove read with missing-proof evidence", d)
	}
}

func TestAnnotationAssignabilityDoesNotUseLengthFloorForIntegerMapIndex(t *testing.T) {
	diags := runDiagnostics(t, `
local lookup: {[integer]: string} = {}
if #lookup >= 2 then
    local s: string = lookup[2]
end
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want integer-map optional index error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot assign lookup[2]") || !strings.Contains(d.Message, "may be nil") || !strings.Contains(d.Explanation.String(), "no guard on this path proves lookup[2] is non-nil") {
		t.Fatalf("diagnostic = %#v, want optional map index with missing-proof evidence", d)
	}
}

func TestAnnotationAssignabilityRejectsStaleIndexRangeProofs(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "index reassigned after guard",
			src: `
local function bad(xs: {number}): ()
	local i: number = 1
	if i <= #xs then
		i = i + 1
		local n: number = xs[i]
	end
end
`,
		},
		{
			name: "computed sibling index not guarded",
			src: `
local function bad(xs: {number}): ()
	local i: number = 1
	if i <= #xs then
		local j = i + 1
		local n: number = xs[j]
	end
end
`,
		},
		{
			name: "array reassigned after guard",
			src: `
local function bad(xs: {number}, ys: {number}): ()
	local i: number = 1
	if i <= #xs then
		xs = ys
		local n: number = xs[i]
	end
end
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want stale range-proof assignment error: %#v", len(diags), diags)
			}
			d := diags[0]
			if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
				t.Fatalf("diagnostic = %#v, want assignment error", d)
			}
			if !strings.Contains(d.Message, "cannot assign xs[") ||
				!strings.Contains(d.Message, "may be nil") ||
				!strings.Contains(d.Explanation.String(), "is an indexed read that can miss or read nil") ||
				!strings.Contains(d.Explanation.String(), "no proof shows the selected slot satisfies the declared type here") {
				t.Fatalf("diagnostic = %#v, want optional array read with missing-proof evidence", d)
			}
		})
	}
}

func TestAnnotationAssignabilityDoesNotTrustCastEscape(t *testing.T) {
	diags := runDiagnostics(t, `local x: number = "no" as any`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if got := diags[0].Explanation.String(); !strings.Contains(got, "assigned value") ||
		!strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "assigned value comes from any/unknown") ||
		!strings.Contains(got, "no proof on this path shows assigned value is number") {
		t.Fatalf("explanation = %q, want source, explicit-any claim, and missing-proof evidence", got)
	}
}

func TestAnnotationAssignabilityExplainsOptionalReceiverInNestedIndexedRead(t *testing.T) {
	diags := runDiagnostics(t, `
type Tags = {[string]: string}
type Policy = { tags: Tags }
local maybe: Policy? = nil
local source: string = maybe.tags["source"]
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one assignment error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s: %#v", d.Code, CodeAssignmentType, d)
	}
	evidence := d.Explanation.Evidence()
	wantEvidence := []string{
		`maybe.tags["source"] can be string or nil here`,
		"source is declared as string",
		"maybe may be nil before reading .tags",
		`maybe.tags may be nil before indexing ["source"]`,
		`no guard on this path proves maybe.tags["source"] is non-nil`,
	}
	if len(evidence) != len(wantEvidence) {
		t.Fatalf("evidence = %#v, want %d ordered items", evidence, len(wantEvidence))
	}
	for i, want := range wantEvidence {
		if evidence[i].Message != want {
			t.Fatalf("evidence[%d] = %q, want %q; full evidence = %#v", i, evidence[i].Message, want, evidence)
		}
	}
}

func TestAnnotationAssignabilityRendersNestedOptionalIndexedReadTrace(t *testing.T) {
	src := strings.TrimLeft(`
type Tags = {[string]: string}
type Policy = { tags: Tags }
local maybe: Policy? = nil
local source: string = maybe.tags["source"]
`, "\n")
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one assignment error: %#v", len(diags), diags)
	}
	rendered := diagnostic.Render(diags[0], diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.assignment]: cannot assign maybe.tags["source"] because it may be nil
 --> diagnostics_test.lua:4:24
  |
  |               ↓ declared type
4 | local source: string = maybe.tags["source"]
  |                        ↑ assigned value

because:
  1. proven: maybe.tags["source"] can be string or nil here
  2. claimed: source is declared as string
  3. proven: maybe may be nil before reading .tags
  4. proven: maybe.tags may be nil before indexing ["source"]
  5. missing proof: no guard on this path proves maybe.tags["source"] is non-nil

help: Guard ` + "`maybe.tags[\"source\"]`" + ` with a nil check, provide a default value, or change the target type to accept nil.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestAnnotationAssignabilityDoesNotTrustExplicitAnyStructuralWitness(t *testing.T) {
	diags := runDiagnostics(t, `
local raw = ({ id = "ok" } :: any)
local req: { id: string } = raw
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1 explicit-any structural witness error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "id") {
		t.Fatalf("diagnostic = %#v, want assignment mismatch for record id contract", d)
	}
}

func TestDirectCallDoesNotTrustExplicitAnyStructuralWitness(t *testing.T) {
	diags := runDiagnostics(t, `
local function accept(req: { id: string })
end
local raw = ({ id = "ok" } :: any)
accept(raw)
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1 explicit-any call error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeDirectCallArgType || !strings.Contains(d.Message, "id") {
		t.Fatalf("diagnostic = %#v, want direct call mismatch for record id contract", d)
	}
	if got := diags[0].Explanation.String(); !strings.Contains(got, "argument 1 (raw) has type any") ||
		!strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "no proof on this path shows raw satisfies the parameter type") {
		t.Fatalf("explanation = %q, want explicit-any claim and missing-proof evidence", got)
	}
}

func TestDirectCallGenericConstraintReportsMissingObjectFieldEvidence(t *testing.T) {
	diags := runDiagnostics(t, `
type HasId = { id: string }
local function need_id<T: HasId>(x: T): string return x.id end
return need_id({ name = "no-id-here" })
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1 generic constraint argument error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || !strings.Contains(d.Message, "not {id: string}") {
		t.Fatalf("diagnostic = %#v, want direct call generic constraint mismatch", d)
	}
	evidence := d.Explanation.Evidence()
	if !diagnosticEvidenceContains(evidence, `argument 1 has type {name: "no-id-here"}`) ||
		!diagnosticEvidenceContains(evidence, "need_id parameter 1 expects {id: string}") ||
		!diagnosticEvidenceContains(evidence, `object literal does not provide field "id"`) {
		t.Fatalf("evidence = %#v, want actual type, constraint, and missing-field missing proof", evidence)
	}
	missing := evidence[len(evidence)-1]
	if missing.Kind != diagnostic.EvidenceMissingProof || missing.Trust != diagnostic.TrustUnknown {
		t.Fatalf("missing-field evidence = %#v, want missing-proof evidence", missing)
	}
}

func TestAnnotationAssignabilityReportsScalarOperatorRHS(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "arithmetic", src: `local bad: string = 1 + 2`},
		{name: "relational", src: `local bad: string = 1 < 2`},
		{name: "concat", src: `local bad: number = "a" .. "b"`},
		{name: "logical", src: `local bad: number = true and false`},
		{name: "unary minus", src: `local bad: string = -1`},
		{name: "unary not", src: `local bad: number = not false`},
		{name: "unary len", src: `local bad: string = #"abc"`},
		{name: "unary bitnot", src: `local bad: string = ~1`},
		{name: "cast wrapper", src: `local bad: string = (1 + 2) as number`},
		{name: "non-nil wrapper", src: `local bad: string = (1 + 2)!`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
			}
			d := diags[0]
			if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
				t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
			}
			if !strings.Contains(d.Message, "cannot assign") {
				t.Fatalf("message = %q, want assignment mismatch", d.Message)
			}
		})
	}
}

func TestAnnotationAssignabilityReportsChannelSelectBranchPayloadMismatch(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
type Event = { kind: "event", id: string, attempt: number }
type Timer = { kind: "timer", elapsed: number }
type Stop = { kind: "stop", reason: string }
type Source = { primary: Channel<Event>, timers: Channel<Timer>, stops: Channel<Stop> }
function consume(source: Source)
	local result = channel.select {
		source.primary:case_receive(),
		source.timers:case_receive(),
		source.stops:case_receive(),
	}
	if result.channel == source.primary then
		local event = result.value
		local wrong: number = event.id
	end
	if result.channel == source.timers then
		local timer = result.value
		local wrong: string = timer.elapsed
	end
end
`, []string{"channel"})
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	messages := diagnosticMessages(diags)
	if !containsDiagnosticMessage(messages, "cannot assign event.id because it is string, not number") ||
		!containsDiagnosticMessage(messages, "cannot assign timer.elapsed because it is number, not string") {
		t.Fatalf("diagnostics = %#v, want string->number and number->string channel payload mismatches", messages)
	}
}

func TestChannelSelectCaseIndexPreservesDuplicateAndReversedMatches(t *testing.T) {
	selected := path.Path{Root: "selected"}
	result := selected.Field("result")
	resultChannel := result.Field(channelselect.ResultChannelField)
	primary := path.Path{Root: "primary"}
	timers := path.Path{Root: "timers"}
	otherResult := path.Path{Root: "other"}.Field("result")

	index := newChannelSelectCaseIndex([]selectInfo{
		{
			result: result,
			cases: []selectCase{
				{path: primary, name: "primary receive"},
				{path: primary, name: "primary send"},
				{path: timers, name: "timers"},
			},
		},
		{
			result: otherResult,
			cases:  []selectCase{{path: primary, name: "later primary"}},
		},
	})

	matches := index.matchesForCheck(branchcond.Check{
		Kind:      branchcond.CheckPathEqual,
		Path:      primary,
		OtherPath: resultChannel,
	})
	if len(matches) != 2 ||
		matches[0].selectIndex != 0 || matches[0].caseIndex != 0 ||
		matches[1].selectIndex != 0 || matches[1].caseIndex != 1 {
		t.Fatalf("reversed primary matches = %#v, want first select duplicate cases [0 1]", matches)
	}

	matches = index.matchesForCheck(branchcond.Check{
		Kind:      branchcond.CheckPathEqual,
		Path:      resultChannel,
		OtherPath: timers,
	})
	if len(matches) != 1 || matches[0].selectIndex != 0 || matches[0].caseIndex != 2 {
		t.Fatalf("direct timers matches = %#v, want select 0 case [2]", matches)
	}
}

func TestDirectCallSiteUsesMemberAccessStructurally(t *testing.T) {
	directSite := factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: 1,
		CalleePath:   path.Path{Root: "math.max"},
	})
	directFact := semantics.CallFact{
		Call: &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "math_max"}},
	}
	if directCallSiteUsesMemberAccess(directSite, directFact) {
		t.Fatalf("punctuated direct callee root was classified as member access")
	}

	memberPathSite := factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: 2,
		CalleePath:   path.Path{Root: "api"}.Field("make"),
	})
	if !directCallSiteUsesMemberAccess(memberPathSite, directFact) {
		t.Fatalf("callee path with a field segment was not classified as member access")
	}

	attrFact := semantics.CallFact{
		Call: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object:    &ast.IdentExpr{Value: "api"},
				Key:       &ast.IdentExpr{Value: "make"},
				KeySyntax: ast.AttrKeyDot,
			},
		},
	}
	if !directCallSiteUsesMemberAccess(directSite, attrFact) {
		t.Fatalf("attribute callee expression was not classified as member access")
	}
}

func TestAnnotationAssignabilityChannelSelectDirectParameterBranches(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
type Event = { id: string }
type Timer = { elapsed: number }
function consume(primary: Channel<Event>, timers: Channel<Timer>): string
	local result = channel.select {
		primary:case_receive(),
		timers:case_receive(),
	}
	if result.channel == primary then
		local event = result.value
		local id: string = event.id
		local wrong: number = event.id
		return id
	end
	if result.channel == timers then
		local timer = result.value
		local elapsed: number = timer.elapsed
		local wrong: string = timer.elapsed
		return tostring(elapsed)
	end
	return ""
end
`, []string{"channel"})
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	messages := diagnosticMessages(diags)
	if !containsDiagnosticMessage(messages, "cannot assign event.id because it is string, not number") ||
		!containsDiagnosticMessage(messages, "cannot assign timer.elapsed because it is number, not string") {
		t.Fatalf("diagnostics = %#v, want direct-param channel payload mismatches only", messages)
	}
}

func TestAnnotationAssignabilityRejectsGradualUntypedDynamicMapWriteWithoutProof(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(raw, key: string)
			local map: {[string]: string} = {}
			map[key] = raw
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1 for unproven dynamic source: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want typed map assignment error", d)
	}
}

func TestAnnotationAssignabilityChecksClosedRecordDynamicWriteAgainstEveryPossibleField(t *testing.T) {
	diags := runDiagnostics(t, `
		type Row = {
			id: string,
			meta: any,
		}

		function f(key: string, value: number): ()
			local row: Row = {id = "ok", meta = {}}
			row[key] = value
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want closed-record dynamic write mismatch: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, "number") || !strings.Contains(d.Message, "string") {
		t.Fatalf("message = %q, want number-to-string dynamic write mismatch", d.Message)
	}
	if evidence := d.Explanation.Evidence(); !diagnosticEvidenceContains(evidence, "value has type number") ||
		!diagnosticEvidenceContains(evidence, "assignment target row[key] requires") {
		t.Fatalf("evidence = %#v, want value and target path evidence", evidence)
	}
}

func TestAnnotationAssignabilityRejectsClosedRecordDynamicWriteUnionValueForMixedFields(t *testing.T) {
	diags := runDiagnostics(t, `
		type Row = {
			id: string,
			count: number,
		}

		function f(key: string, value: string | number): ()
			local row: Row = {id = "ok", count = 0}
			row[key] = value
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want mixed-field dynamic write mismatch: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, "string") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q, want union value and mixed-field target mismatch", d.Message)
	}
	if evidence := d.Explanation.Evidence(); !diagnosticEvidenceContains(evidence, "value has type number | string") ||
		!diagnosticEvidenceContains(evidence, "assignment target row[key] requires") {
		t.Fatalf("evidence = %#v, want value and target path evidence", evidence)
	}
}

func TestAnnotationAssignabilityRejectsExplicitAnyForClosedRecordDynamicWrite(t *testing.T) {
	diags := runDiagnostics(t, `
		type Row = {
			id: string,
			meta: any,
		}

		function f(key: string): ()
			local row: Row = {id = "ok", meta = {}}
			local raw = (1 :: any)
			row[key] = raw
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want explicit-any closed-record dynamic write mismatch: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	explanation := d.Explanation.String()
	if !strings.Contains(explanation, "user asserted any") ||
		!strings.Contains(explanation, "no proof on this path shows raw satisfies the declared type") {
		t.Fatalf("explanation = %q, want explicit-any missing-proof evidence for string field", explanation)
	}
}

func TestAnnotationAssignabilityAcceptsClosedRecordDynamicWriteWhenValueFitsEveryField(t *testing.T) {
	diags := runDiagnostics(t, `
		type Row = {
			id: string,
			label: string,
		}

		function f(key: string, value: string): ()
			local row: Row = {id = "ok", label = "ready"}
			row[key] = value
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want string value accepted for all closed-record dynamic fields", diags)
	}
}

func TestAnnotationAssignabilityRejectsExplicitAnyAsProof(t *testing.T) {
	diags := runDiagnostics(t, `
		type Payload = {id: string, count: number}
		local raw: any = {id = "cfg", count = 2}
		local payload: Payload = raw
		if raw.id then
			local id: string = raw.id
		end
		local function consume(payload: Payload): number
			return payload.count + 1
		end
		local count = consume(raw)
	`)
	if len(diags) != 3 {
		t.Fatalf("diagnostics = %d, want 3: %#v", len(diags), diags)
	}
	var assignment, field, call bool
	for _, d := range diags {
		msg := d.Message
		assignment = assignment || strings.Contains(msg, "cannot assign raw because it is any, not") && strings.Contains(msg, "id: string")
		field = field || strings.Contains(msg, "cannot assign raw.id because it is any, not string")
		call = call || strings.Contains(msg, "argument 1 (raw) is any") && strings.Contains(msg, "not Payload")
	}
	if !assignment || !field || !call {
		t.Fatalf("diagnostics = %#v, want explicit-any assignment, field, and call errors", diags)
	}
}

func TestAnnotationAssignabilityRejectsExplicitAnyFieldThroughIPairs(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
local raw: any = nil
local pages = {
	{ id = raw, route = "/ok" },
}
local accessible: {[string]: string} = {}
for _, page in ipairs(pages) do
	accessible[page.route] = page.id
end
`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want typed map assignment error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want string map assignment error", d)
	}
}

func TestAnnotationAssignabilityRejectsExplicitAnyFieldAfterEqualityGuard(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
local raw: any = nil
local pages = {
	{ id = raw, route = "/ok" },
}
local routes: {[string]: string} = { ["/ok"] = "page:ok" }
local accessible: {[string]: string} = {}
for _, page in ipairs(pages) do
	local route = page.route
	if route and routes[route] == page.id then
		accessible[route] = page.id
	end
end
`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want typed map assignment error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want string map assignment error", d)
	}
}

func TestAnnotationAssignabilityRejectsExplicitAnyFieldThroughFixtureGuardShape(t *testing.T) {
	diags := runDiagnosticsFull(t, `
local unknown_id: any = nil
local all_pages = {
	{ id = unknown_id, mount_route = "/ok/:part(.*)*", secure = false },
}
local routes_map: {[string]: string} = {
	["/ok/:part(.*)*"] = "page:ok",
}
local accessible: {[string]: string} = {}

for _, page in ipairs(all_pages) do
	local mr = page.mount_route
	if mr and routes_map[mr] == page.id and (not page.secure or can_access(page)) then
		accessible[mr] = page.id
	end
end
`, []string{"test", "type", "value", "can_access"}, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want typed map assignment error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want string map assignment error", d)
	}
}

func TestDirectCallRejectsGradualTopThroughOrDefault(t *testing.T) {
	diags := runDiagnostics(t, `
local http = {
	get = function(url: string, options: table)
		return { url = url, options = options }, nil
	end,
}

local function main(args)
	local url = (args and args.url) or "http://localhost:8085/hello"
	return http.get(url, { timeout = "2s" })
end
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want gradual-top string argument error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeDirectCallArgType || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want direct call string argument error", d)
	}
}

func TestAnnotationAssignabilitySkipsUnannotatedIdentifierSources(t *testing.T) {
	diags := runDiagnostics(t, `
		local y = value
		local x: number = y
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unannotated identifier source", diags)
	}
}

func TestAnnotationAssignabilitySkipsAnnotatedIdentifierWithoutPointProof(t *testing.T) {
	diags := runDiagnostics(t, `
		local x: string? = value
		local s: string = x
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none without point-local source proof", diags)
	}
}

func TestAnnotationAssignabilityReportsMaybeParameterWithoutNarrowing(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string?)
			local y: string = x
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign x") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("diagnostic = %#v, want optional parameter assignment error", d)
	}
}

func TestAnnotationAssignabilityReportsMissingUnionMapEntry(t *testing.T) {
	diags := runDiagnostics(t, `
		type Allow = {kind: "allow", reason: string}
		type Deny = {kind: "deny", reason: string}
		type Decision = Allow | Deny
		local cache: {[string]: Decision} = {}
		local missing: Decision = cache["missing"]
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want missing-key optionality error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign cache[\"missing\"]") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("diagnostic = %#v, want nilable union assignment error", d)
	}
}

func TestAnnotationAssignabilityReportsMissingUnionMapEntryUnderInferredReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		type Task = {kind: "task", id: string}
		type Timer = {kind: "timer", id: string}
		type Envelope = Task | Timer
		type State = {processed: {[string]: Envelope}, counters: {[string]: number}}
		type Actor = {state: State}
		local function new_actor(): Actor
			return {state = {processed = {}, counters = {}}}
		end
		local actor = new_actor()
		actor.state.processed["m1"] = {kind = "task", id = "m1"}
		actor.state.counters["task"] = 1
		local missing_processed: Envelope = actor.state.processed["missing"]
		local missing_counter: number = actor.state.counters["missing"]
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want processed and counter missing-key errors: %#v", len(diags), diags)
	}
	messages := diagnosticMessages(diags)
	if !containsDiagnosticMessage(messages, "cannot assign actor.state.processed[\"missing\"]") ||
		!containsDiagnosticMessage(messages, "cannot assign actor.state.counters[\"missing\"]") {
		t.Fatalf("diagnostics = %#v, want both missing-key assignment errors", messages)
	}
}

func TestAnnotationAssignabilityUsesSolvedTypeTestState(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string | number)
			if type(x) == "string" then
				local n: number = x
			else
				local s: string = x
			end
		end
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	for _, d := range diags {
		if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign") {
			t.Fatalf("diagnostic = %#v, want assignment mismatch", d)
		}
	}
}

func TestMemberCallPreservesRootPresenceGuardAcrossUnrelatedDynamicIndexWrite(t *testing.T) {
	diags := runDiagnostics(t, `
		type Obj = {m: () -> ()}
		function f(x: Obj?, t: {[string]: number}, key: string)
			if x then
				t[key] = 1
				x.m()
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want unrelated dynamic index write to preserve root presence guard", diags)
	}
}

func TestAnnotationAssignabilityUsesTypeIsWrapperErrorBranchState(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {x: number, y: number}
		local function isPoint(x)
			return Point:is(x)
		end
		function validate(data: any)
			local val, err = isPoint(data)
			if err ~= nil then
				local p: Point = val
			end
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "nil") {
		t.Fatalf("diagnostic = %#v, want nil-to-Point assignment error", d)
	}
}

func TestAnnotationAssignabilityUsesDeclaredLocalValueForTypeTestState(t *testing.T) {
	diags := runDiagnostics(t, `
		local y: string | number = 42
		if type(y) == "string" then
			local n: number = y
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string") || !strings.Contains(d.Message, "number") {
		t.Fatalf("diagnostic = %#v, want string-to-number assignment mismatch", d)
	}
}

func TestAnnotationAssignabilityUsesSolvedTypeNotState(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string | number)
			if type(x) ~= "string" then
				local s: string = x
			else
				local n: number = x
			end
		end
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	for _, d := range diags {
		if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign") {
			t.Fatalf("diagnostic = %#v, want assignment mismatch", d)
		}
	}
}

func TestAnnotationAssignabilityAcceptsAssertedMaybeParameter(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
		function f(x: string?)
			assert(x)
			local y: string = x
		end
	`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after assert", diags)
	}
}

func TestAnnotationAssignabilitySkipsRootLiteralIndexProjection(t *testing.T) {
	diags := runDiagnostics(t, `
		local xs: {number} = {1, 2}
		local x: number = xs[1]
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestAnnotationAssignabilityAcceptsRootOptionalLiteralIndexWithPresentElementProof(t *testing.T) {
	diags := runDiagnostics(t, `
		local xs: {number}? = {1, 2}
		local x: number = xs[1]
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for exact root-local element proof", diags)
	}
}

func TestAnnotationAssignabilityAcceptsWhileIndexReadProvenInRange(t *testing.T) {
	diags := runDiagnostics(t, `
		function first(xs: {number}): number
			local i: number = 1
			while i <= #xs do
				local v: number = xs[i]
				if v > 0 then
					return v
				end
				i = i + 1
			end
			return 0
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for proven in-range positive index", diags)
	}
}

func TestAnnotationAssignabilityAcceptsInferredFunctionFieldAliasReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		type Res = { answer: string }
		local M = {
			dep = {
				get = function()
					return nil
				end,
			},
		}
		function M.run()
			return M.dep.get()
		end
		M.dep = {
			get = function()
				return { answer = "ok" }
			end,
		}
		local f: fun(): Res = M.run
		local res = f()
		local answer: string = res.answer
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for current function-field alias return", diags)
	}
}

func TestAnnotationAssignabilityAcceptsDominatingFieldDefinedWrapperReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		local M = {
			dep = {
				get = function()
					return nil
				end,
			},
		}
		function M.run()
			return M.dep.get()
		end
		M.dep = {
			get = function()
				return { answer = "ok" }
			end,
		}
		local res = M.run()
		local answer: string = res.answer
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none when provider replacement dominates wrapper call", diags)
	}
}

func TestAnnotationAssignabilityRejectsNonDominatingFieldDefinedWrapperReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		local function run(flag: boolean)
			local M = {
				dep = {
					get = function()
						return nil
					end,
				},
			}
			function M.run()
				return M.dep.get()
			end
			if flag then
				M.dep = {
					get = function()
						return { answer = "ok" }
					end,
				}
			end
			local res = M.run()
			local answer: string = res.answer
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one non-dominating wrapper return error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign res.answer") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("diagnostic = %#v, want optional wrapper return assignment error", d)
	}
}

func TestAnnotationAssignabilityRejectsBranchReassignedCallResultField(t *testing.T) {
	diags := runDiagnostics(t, `
		local function make(): { answer: string }
			return { answer = "ok" }
		end

		local function run(flag: boolean)
			local res = make()
			if flag then
				res = { answer = 1 }
			end
			local answer: string? = res.answer
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one branch reassigned call-result field error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string | 1") || !strings.Contains(d.Message, "string?") {
		t.Fatalf("diagnostic = %#v, want string-or-1-to-string? assignment error", d)
	}
	if evidence := d.Explanation.Evidence(); !diagnosticEvidenceContains(evidence, "res is reassigned before the read; after that assignment, res.answer has literal value 1") {
		t.Fatalf("evidence = %#v, want reassignment invalidation evidence", evidence)
	}
}

func TestAnnotationAssignabilityRendersBranchReassignedCallResultFieldTrace(t *testing.T) {
	src := strings.TrimLeft(`
local function make(): { answer: string }
    return { answer = "ok" }
end

local function run(flag: boolean)
    local res = make()
    if flag then
        res = { answer = 1 }
    end
    local answer: string? = res.answer
end
`, "\n")
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one branch reassigned call-result field error: %#v", len(diags), diags)
	}
	rendered := diagnostic.Render(diags[0], diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.assignment]: cannot assign res.answer because it is string | 1, not string?
 --> diagnostics_test.lua:10:29
   |
10 |     local answer: string? = res.answer
   |                             ↑ assigned value

because:
  1. proven: res.answer has type string | 1
  2. claimed: answer is declared as string?
  3. proven: res is reassigned before the read; after that assignment, res.answer has literal value 1
 --> diagnostics_test.lua:8:15
  |
8 |         res = { answer = 1 }
  |               ^
  4. missing proof: no proof on this path shows res.answer satisfies the declared type

help: Use a value compatible with the expected type, or change the target type if ` + "`res.answer`" + ` is valid.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestAnnotationAssignabilityKeepsOriginalCallArmAfterBranchReassignedCallResultField(t *testing.T) {
	diags := runDiagnostics(t, `
		local function make(): { answer: string }
			return { answer = "ok" }
		end

		local function run(flag: boolean)
			local res = make()
			if flag then
				res = { answer = 1 }
			end
			local answer: number = res.answer
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one original call arm assignment error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string | 1") || !strings.Contains(d.Message, "number") {
		t.Fatalf("diagnostic = %#v, want string-or-1-to-number assignment error", d)
	}
	if evidence := d.Explanation.Evidence(); !diagnosticEvidenceContains(evidence, "res is reassigned before the read; after that assignment, res.answer has literal value 1") {
		t.Fatalf("evidence = %#v, want reassignment invalidation evidence", evidence)
	}
}

func TestAnnotationAssignabilityAcceptsBranchReassignedCallResultFieldUnion(t *testing.T) {
	diags := runDiagnostics(t, `
		local function make(): { answer: string }
			return { answer = "ok" }
		end

		local function run(flag: boolean)
			local res = make()
			if flag then
				res = { answer = 1 }
			end
			local answer: string | number = res.answer
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want union annotation accepted", diags)
	}
}

func TestAnnotationAssignabilityRejectsReassignedFunctionFieldAliasReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		type Res = { answer: string }
		local M = {
			dep = {
				get = function()
					return nil
				end,
			},
		}
		function M.run()
			return M.dep.get()
		end
		M.run = function()
			return nil
		end
		local f: fun(): Res = M.run
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType {
		t.Fatalf("diagnostic = %#v, want assignment mismatch for reassigned wrapper", d)
	}
}

func TestAnnotationAssignabilityReportsNestedOptionalIndexProjection(t *testing.T) {
	diags := runDiagnostics(t, `
		type Response = {
			result: {
				data: {
					departments: {string}?,
				},
			},
		}
		local response: Response = {
			result = {
				data = {
					departments = {"engineering"},
				},
			},
		}
		local first: string = response.result.data.departments[1]
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign response.result.data.departments[1]") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("diagnostic = %#v, want optional nested index assignment error", d)
	}
}

func TestAnnotationAssignabilityReportsNestedOptionalIndexAfterGuardCalls(t *testing.T) {
	diags := runDiagnostics(t, `
		type Response = {
			result: {
				data: {
					departments: {string}?,
				},
			},
		}
		local response: Response = {
			result = {
				data = {
					departments = {"engineering"},
				},
			},
		}
		test.not_nil(response.result.data.departments, "departments required")
		test.eq(type(response.result.data.departments), "table", "departments should be a table")
		local count: number = #response.result.data.departments
		local first: string = response.result.data.departments[1]
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign response.result.data.departments[1]") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("diagnostic = %#v, want optional nested index assignment error", d)
	}
}

func TestAnnotationAssignabilityReportsMissingRequiredField(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {x: number, y: number}
		local p: Point = {x = 10}
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "y") {
		t.Fatalf("diagnostic = %#v, want missing required field y", d)
	}
	if d := diags[0]; !diagnosticEvidenceContains(d.Explanation.Evidence(), "object literal has type {x: 10}") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "p is declared as Point") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "required field p.y has type number, but the object literal does not provide it") {
		t.Fatalf("evidence = %#v, want provided shape, declared type, and missing required path", d.Explanation.Evidence())
	}
	if d := diags[0]; !strings.Contains(d.Help, "Add field `y`") {
		t.Fatalf("help = %q, want missing-field repair", d.Help)
	}
}

func TestLexicalTypeShadowingResolvesNearestVisibleAlias(t *testing.T) {
	diags := runDiagnostics(t, `
		type Value = number
		local a: Value = 10
		if true then
			type Value = string
			local b: Value = "hello"
		end
		local c: Value = 20
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestUnresolvedLexicalTypeReferences(t *testing.T) {
	cases := []struct {
		name string
		src  string
		line int
		col  int
		end  int
	}{
		{
			name: "not visible outside block",
			src: strings.TrimLeft(`
if true then
	type LocalPoint = {x: number, y: number}
end
local p: LocalPoint = {x = 1, y = 2}
`, "\n"),
			line: 4,
			col:  10,
			end:  19,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
			}
			d := diags[0]
			if d.Code != CodeUnresolvedTypeReference || d.Severity != diagnostic.SeverityError {
				t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
			}
			if d.Message != "unknown type LocalPoint" {
				t.Fatalf("message = %q, want exact unresolved type message", d.Message)
			}
			if d.Position.Line != tc.line || d.Position.Column != tc.col || d.Position.EndColumn != tc.end {
				t.Fatalf("position = %#v, want exact span of LocalPoint", d.Position)
			}
			if d.Span.StartLine != tc.line || d.Span.StartCol != tc.col || d.Span.EndCol != tc.end {
				t.Fatalf("span = %#v, want exact span for only the unresolved type", d.Span)
			}
			evidence := d.Explanation.Evidence()
			if len(evidence) != 1 || evidence[0].Message != "no type named LocalPoint is declared in this scope, a parent scope, or an imported module" {
				t.Fatalf("evidence = %#v, want one missing-declaration proof", evidence)
			}
			if evidence[0].Kind != diagnostic.EvidenceAbstractFact || evidence[0].Trust != diagnostic.TrustProven {
				t.Fatalf("evidence kind/trust = %s/%s, want proven lookup fact", evidence[0].Kind, evidence[0].Trust)
			}
			if !diagnosticHasLabel(d, labelUnknownType) {
				t.Fatalf("labels = %#v, want unknown-type focus label", d.Labels)
			}
			if !strings.Contains(d.Help, "Declare the type") || !strings.Contains(d.Help, "fully qualified") {
				t.Fatalf("help = %q, want actionable type-resolution help", d.Help)
			}
			rendered := diagnostic.Render(d, diagnostic.RenderOptions{
				Sources:             diagnostic.SourceMap{"diagnostics_test.lua": tc.src},
				ShowSourceLabelRows: true,
			})
			want := `error[type.reference.unresolved]: unknown type LocalPoint
 --> diagnostics_test.lua:4:10
  |
4 | local p: LocalPoint = {x = 1, y = 2}
  |          ↑ unknown type

because:
  1. proven: no type named LocalPoint is declared in this scope, a parent scope, or an imported module

help: Declare the type in scope, import the module that exports it, or use the fully qualified exported type name.`
			if rendered != want {
				t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
			}
		})
	}
}

func TestForwardTypeReferenceResolves(t *testing.T) {
	// Type declarations are not order-dependent within a scope: a sibling
	// alias may reference one declared later, including through a recursive
	// cycle. The forward reference must resolve without an unresolved-type
	// diagnostic.
	diags := runDiagnostics(t, `
type Group = {kind: "group", children: {Node}}
type Node = {kind: "leaf"} | Group
local p: Node = {kind = "leaf"}
`)
	for _, d := range diags {
		if d.Code == CodeUnresolvedTypeReference {
			t.Fatalf("forward type reference reported unresolved: %#v", d)
		}
	}
}

func TestUnresolvedValueReferencesReportsImplicitGlobalReads(t *testing.T) {
	src := strings.TrimLeft(`
local x = missing + known
missing = 42
print(known)
`, "\n")
	diags := runDiagnosticsWithGlobals(t, src, []string{"known", "print"})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeUnresolvedValueReference || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if d.Message != "unknown value missing" {
		t.Fatalf("message = %q, want exact unresolved value message", d.Message)
	}
	if d.Position.Line != 1 || d.Position.Column != 11 || d.Position.EndColumn != 17 {
		t.Fatalf("position = %#v, want exact span of missing read", d.Position)
	}
	if d.Span.StartLine != 1 || d.Span.StartCol != 11 || d.Span.EndCol != 17 {
		t.Fatalf("span = %#v, want exact span for only the missing identifier", d.Span)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) != 1 || evidence[0].Message != "no value named missing is declared, predeclared, imported, or configured global in this scope" {
		t.Fatalf("evidence = %#v, want one missing-declaration proof", evidence)
	}
	if evidence[0].Kind != diagnostic.EvidenceAbstractFact || evidence[0].Trust != diagnostic.TrustProven {
		t.Fatalf("evidence kind/trust = %s/%s, want proven lookup fact", evidence[0].Kind, evidence[0].Trust)
	}
	if !diagnosticHasLabel(d, labelUnknownValue) {
		t.Fatalf("labels = %#v, want unknown-value focus label", d.Labels)
	}
	if !strings.Contains(d.Help, "Declare the value") || !strings.Contains(d.Help, "configured globals") {
		t.Fatalf("help = %q, want actionable value-resolution help", d.Help)
	}
	rendered := diagnostic.Render(d, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[value.reference.unresolved]: unknown value missing
 --> diagnostics_test.lua:1:11
  |
1 | local x = missing + known
  |           ↑ unknown value

because:
  1. proven: no value named missing is declared, predeclared, imported, or configured global in this scope

help: Declare the value, import it through require, or add it to the configured globals when it is intentionally ambient.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestUnresolvedValueReferencesReportsNestedReads(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		local t = {[key] = {value = source}}
		sink[t[other]] = value
	`, []string{"sink", "value"})
	if len(diags) != 3 {
		t.Fatalf("diagnostics = %d, want 3: %#v", len(diags), diags)
	}
	for _, d := range diags {
		if d.Code != CodeUnresolvedValueReference {
			t.Fatalf("diagnostic code = %s, want %s: %#v", d.Code, CodeUnresolvedValueReference, diags)
		}
	}
}

func TestMemberCallReportsMissingMethodAfterDiscriminantNarrowing(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: () -> ()}
		type Cat = {kind: "cat", meow: () -> ()}
		type Animal = Dog | Cat

		local function speak(a: Animal)
			if a.kind == "dog" then
				a.meow()
			end
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeMissingMember || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "meow") || !strings.Contains(d.Message, "dog") {
		t.Fatalf("message = %q, want missing meow on narrowed dog variant", d.Message)
	}
	if len(d.Explanation.Evidence()) == 0 || d.Explanation.String() == "" {
		t.Fatalf("explanation = %#v, want non-empty evidence", d.Explanation)
	}
}

func TestMemberCallAcceptsMatchingDiscriminantMethod(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: () -> ()}
		type Cat = {kind: "cat", meow: () -> ()}
		type Animal = Dog | Cat

		local function speak(a: Animal)
			if a.kind == "dog" then
				a.bark()
			else
				a.meow()
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestMemberReadReportsStaticBracketMissingFieldAfterDiscriminantNarrowing(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: string}
		type Cat = {kind: "cat", meow: string}
		type Animal = Dog | Cat

		local function speak(a: Animal)
			if a.kind == "dog" then
				local bad = a["meow"]
			end
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want static bracket missing-member read: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeMissingMember || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, `"meow"`) || !strings.Contains(d.Message, "dog") {
		t.Fatalf("message = %q, want missing meow on narrowed dog variant", d.Message)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) != 1 ||
		!diagnosticEvidenceContains(evidence, `a["meow"] reads member "meow" from receiver type`) ||
		d.Explanation.String() == "" {
		t.Fatalf("explanation evidence = %#v, want path-specific member-read receiver evidence", evidence)
	}
	if !diagnosticHasLabel(d, labelMemberRead) {
		t.Fatalf("labels = %#v, want member-read focus label", d.Labels)
	}
	if !strings.Contains(d.Help, "Narrow the receiver before reading `meow`") {
		t.Fatalf("help = %q, want actionable missing-member help", d.Help)
	}
}

func TestMemberReadSkipsDynamicBracketKeyAfterDiscriminantNarrowing(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: string}
		type Cat = {kind: "cat", meow: string}
		type Animal = Dog | Cat

		local function speak(a: Animal, key: string)
			if a.kind == "dog" then
				local unknown = a[key]
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want no static member-read diagnostic for dynamic key", diags)
	}
}

func TestMemberReadReportsAliasVariantWriteInvalidatedGuardWithEvidence(t *testing.T) {
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

		local alias = slots.active

		if alias.value.kind == "file" then
			local before: string = alias.value.path
			alias.value = {kind = "timer", seconds = 5}
			local stale_path: string = slots.active.value.path
			local stale_seconds: number = before
		end
	`)
	assertStalePathMissingMemberEvidence(t, result)
}

func TestMemberReadReportsBracketVariantWriteInvalidatedGuardWithEvidence(t *testing.T) {
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

		if slots.active.value.kind == "file" then
			local before: string = slots["active"].value.path
			slots["active"].value = {kind = "timer", seconds = 10}
			local stale_path: string = slots.active.value.path
			local stale_seconds: number = before
		end
	`)
	assertStalePathMissingMemberEvidence(t, result)
}

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

func assertStalePathMissingMemberEvidence(t *testing.T, result *body.Result) {
	t.Helper()
	point, expr := requireLocalAssignmentExprByName(t, result, "stale_path")
	read, ok := expr.(*ast.AttrGetExpr)
	if !ok {
		t.Fatalf("stale_path expr = %T, want AttrGetExpr", expr)
	}
	envs := guardEnvironments(result)
	context := producerContext{resolver: newResultResolver(result, nil)}
	typers := memberReadTypers{
		narrowed: newStructuralFlowExpressionTyper(result, context.resolver, point, envs[point]),
		base:     newStructuralFlowExpressionTyper(result, context.resolver, point, guardEnv{}),
		result:   result,
		point:    point,
	}
	receiver, ok := typers.receiverType(read.Object)
	if !ok {
		t.Fatal("member-read receiver type unavailable")
	}
	if !fieldProvablyAbsent(receiver, "path") {
		t.Fatalf("receiver type = %s, want path provably absent", formatType(receiver))
	}
	broad, broadOK := typers.base.broadType(read.Object)
	if !broadOK || !isMultiArmUnion(broad) {
		t.Fatalf("broad receiver type = %s/%v, want original union", formatType(broad), ok)
	}
	fieldBroad := broad
	if withoutNil := projectionWithoutNil(broad); withoutNil != nil && !typ.IsNever(withoutNil) {
		fieldBroad = withoutNil
	}
	if field, ok := access.Field(fieldBroad, "path"); !ok {
		t.Fatalf("broad receiver type = %s, does not admit path after nil stripping; field=%s/%v", formatType(broad), formatType(field), ok)
	}
	produced, ok := memberRead(context).read(read, typers)
	if !ok || produced.Code != CodeMissingMember {
		t.Fatalf("memberRead.read = %#v/%v, want missing-member diagnostic", produced, ok)
	}

	diags := Produce(result)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d %#v, want stale member read and stale literal assignment", len(diags), diags)
	}
	var missing diagnostic.Diagnostic
	for _, d := range diags {
		if d.Code == CodeMissingMember {
			missing = d
			break
		}
	}
	if missing.Code != CodeMissingMember || missing.Severity != diagnostic.SeverityError {
		t.Fatalf("missing-member diagnostic = %#v, want error; all diagnostics = %#v", missing, diags)
	}
	if !strings.Contains(missing.Message, `"path"`) || !strings.Contains(missing.Message, "timer") {
		t.Fatalf("message = %q, want missing path on timer variant", missing.Message)
	}
	evidence := missing.Explanation.Evidence()
	if len(evidence) != 1 ||
		!diagnosticEvidenceContains(evidence, `slots.active.value.path reads member "path" from receiver type`) ||
		missing.Explanation.String() == "" {
		t.Fatalf("explanation evidence = %#v, want path-specific stale member-read evidence", evidence)
	}
	if !diagnosticHasLabel(missing, labelMemberRead) {
		t.Fatalf("labels = %#v, want member-read focus label", missing.Labels)
	}
}

func TestMemberCallReportsUnionReceiverMissingMethod(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string | number)
			x:upper()
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeMissingMember || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "upper") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q, want missing upper on string|number receiver", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "x.upper has receiver type") {
		t.Fatalf("explanation evidence = %#v, want member receiver evidence", d.Explanation.Evidence())
	}
	if !diagnosticHasLabel(d, labelMemberCall) {
		t.Fatalf("labels = %#v, want member-call focus label", d.Labels)
	}
	if !strings.Contains(d.Help, "Narrow the receiver before reading `upper`") {
		t.Fatalf("help = %q, want actionable missing-member help", d.Help)
	}
}

func TestMemberCallEvidenceKeepsNestedCalleePath(t *testing.T) {
	cases := []struct {
		name         string
		src          string
		code         diagnostic.Code
		wantEvidence string
	}{
		{
			name: "missing nested member",
			src: `
type ReadyClient = {kind: "ready", ready: () -> ()}
type IdleClient = {kind: "idle", wait: () -> ()}
type Client = ReadyClient | IdleClient
type Box = {client: Client}
function f(box: Box)
    if box.client.kind == "ready" then
        box.client:run()
    end
end
`,
			code:         CodeMissingMember,
			wantEvidence: "box.client.run has receiver type",
		},
		{
			name: "non-callable nested member",
			src: `
type BadClient = {kind: "bad", run: number}
type GoodClient = {kind: "good", run: () -> ()}
type Client = BadClient | GoodClient
type Box = {client: Client}
function f(box: Box)
    if box.client.kind == "bad" then
        box.client:run()
    end
end
`,
			code:         CodeNotCallable,
			wantEvidence: "box.client.run has type number at call",
		},
		{
			name: "nested member call contract",
			src: `
type Client = {invoke: (id: string) -> ()}
type Box = {client: Client}
function f(box: Box)
    box.client.invoke(42)
end
`,
			code:         CodeDirectCallArgType,
			wantEvidence: "box.client.invoke parameter 1 expects string",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want one nested member-call diagnostic: %#v", len(diags), diags)
			}
			d := diags[0]
			if d.Code != tc.code || d.Severity != diagnostic.SeverityError {
				t.Fatalf("diagnostic = %#v, want %s error", d, tc.code)
			}
			if !diagnosticEvidenceContains(d.Explanation.Evidence(), tc.wantEvidence) {
				t.Fatalf("evidence = %#v, want %q", d.Explanation.Evidence(), tc.wantEvidence)
			}
		})
	}
}

func TestMemberCallAcceptsUnionReceiverWhenAllAlternativesCallable(t *testing.T) {
	diags := runDiagnostics(t, `
		type Left = {run: () -> string}
		type Right = {run: () -> number}

		function f(x: Left | Right)
			x:run()
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestMemberCallReportsOptionalSymbolReceiver(t *testing.T) {
	diags := runDiagnostics(t, `
		type Message = {topic: (self: Message) -> string}
		function f(m: Message?)
			m:topic()
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeOptionalMethodCall || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot call method on an optional value without a nil check") {
		t.Fatalf("message = %q", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "receiver m is optional at call to m.topic") {
		t.Fatalf("evidence = %#v, want path-specific optional receiver evidence", d.Explanation.Evidence())
	}
}

func TestMemberCallReportsOptionalExpressionReceiver(t *testing.T) {
	diags := runDiagnostics(t, `
		type Message = {topic: (self: Message) -> string}
		local function make(): {Message}
			return {}
		end
		local _: string = make()[1]:topic()
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeOptionalMethodCall || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot call method on an optional value without a nil check") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestMemberCallAcceptsNarrowedOptionalReceiver(t *testing.T) {
	diags := runDiagnostics(t, `
		type Message = {topic: (self: Message) -> string}
		function f(m: Message?)
			if m then
				m:topic()
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after nil check", diags)
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

func TestMemberCallReportsWrongArgumentType(t *testing.T) {
	diags := runDiagnostics(t, `
		type Client = {invoke: (model_id: string, payload: any) -> ()}
		function f(c: Client)
			c.invoke(42, {})
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 1") || !strings.Contains(d.Message, "string") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestMemberCallReportsTooFewArgs(t *testing.T) {
	diags := runDiagnostics(t, `
		type Client = {invoke: (model_id: string, payload: number) -> ()}
		function f(c: Client)
			c.invoke("model")
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallTooFewArgs || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "expects 2 arguments") || !strings.Contains(d.Message, "got 1") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestMemberCallReportsTooManyArgs(t *testing.T) {
	diags := runDiagnostics(t, `
		type Client = {invoke: (model_id: string, payload: number) -> ()}
		function f(c: Client)
			c.invoke("model", 1, true)
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallTooManyArgs || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "expects 2 arguments") || !strings.Contains(d.Message, "got 3") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Labels) != 1 || d.Labels[0].Message != "extra argument" {
		t.Fatalf("labels = %#v, want extra argument label", d.Labels)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want call and declaration evidence", d.Explanation.Evidence())
	}
}

func TestColonMemberCallConsumesReceiverParameter(t *testing.T) {
	diags := runDiagnostics(t, `
		type ClientSelf = {id: string}
		type Client = {id: string, invoke: (self: ClientSelf, model_id: string) -> ()}
		function f(c: Client)
			c:invoke(42)
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 1") || !strings.Contains(d.Message, "string") {
		t.Fatalf("message = %q", d.Message)
	}

	ok := runDiagnostics(t, `
		type ClientSelf = {id: string}
		type Client = {id: string, invoke: (self: ClientSelf, model_id: string) -> ()}
		function f(c: Client)
			c:invoke("model")
		end
	`)
	if len(ok) != 0 {
		t.Fatalf("diagnostics = %#v, want none for matching colon call", ok)
	}
}

func TestMemberCallSkipsUnreachableDiscriminantBranch(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: () -> ()}

		local function speak(a: Dog)
			if a.kind == "cat" then
				a.meow()
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unreachable branch body", diags)
	}
}

func TestMemberCallAcceptsUnnarrowedPrimitiveMethod(t *testing.T) {
	diags := runDiagnostics(t, `
		local value: string = "abc"
		value:upper()
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestInlineConcreteCastMemberAccessIsTyped(t *testing.T) {
	// A cast to a CONCRETE type is trusted by the linter for inference, so
	// `(result :: { id: string }).id` types as string -- no spurious "any" error.
	diags := runDiagnostics(t, `
		local function g(result: any): string
			return (result :: { id: string }).id
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("inline concrete cast member access should type-check cleanly, got %d: %#v", len(diags), diags)
	}
}

func TestDirectCallReportsNonCallableTarget(t *testing.T) {
	diags := runDiagnostics(t, `
		local x: number = 42
		x()
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallNotCallable || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "not callable") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "x has type number") {
		t.Fatalf("explanation evidence = %#v, want callee type evidence", d.Explanation.Evidence())
	}
	if !diagnosticHasLabel(d, labelCallTarget) {
		t.Fatalf("labels = %#v, want call-target focus label", d.Labels)
	}
	if !strings.Contains(d.Help, "replace `x` with a callable expression") {
		t.Fatalf("help = %q, want actionable non-callable target help", d.Help)
	}
}

func TestDirectCallReportsPossiblyNilTargetWithDirectCallCode(t *testing.T) {
	diags := runDiagnostics(t, `
		local maybe: (() -> string)? = nil
		maybe()
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallNotCallable || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot call maybe") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "maybe has a callable type, but may also be nil") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "no guard on this path proves maybe is non-nil before this call") {
		t.Fatalf("explanation evidence = %#v, want optional callable and missing guard proof", d.Explanation.Evidence())
	}
	if !diagnosticHasLabel(d, labelCallTarget) {
		t.Fatalf("labels = %#v, want call-target focus label", d.Labels)
	}
	if !strings.Contains(d.Help, "Guard `maybe` with a nil check") {
		t.Fatalf("help = %q, want actionable possibly-nil target help", d.Help)
	}
}

func TestDirectCallRendersPossiblyNilTargetTrace(t *testing.T) {
	src := strings.TrimLeft(`
local maybe: (() -> string)? = nil
maybe()
`, "\n")
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	rendered := diagnostic.Render(diags[0], diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.call.direct.not_callable]: cannot call maybe because it may be nil
 --> diagnostics_test.lua:2:1
  |
2 | maybe()
  | ↑ call target

because:
  1. proven: maybe has a callable type, but may also be nil
  2. missing proof: no guard on this path proves maybe is non-nil before this call

help: Guard ` + "`maybe`" + ` with a nil check before calling it.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestDirectCallReportsTooFewArgs(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a
		end
		add(1)
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallTooFewArgs || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "expects 2 arguments") || !strings.Contains(d.Message, "got 1") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want call and declaration evidence", d.Explanation.Evidence())
	}
	if !diagnosticHasLabel(d, labelCallExpression) {
		t.Fatalf("labels = %#v, want call-expression focus label", d.Labels)
	}
}

func TestDirectCallReportsTooManyArgs(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a
		end
		add(1, 2, 3)
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallTooManyArgs || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "expects 2 arguments") || !strings.Contains(d.Message, "got 3") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Labels) != 1 || d.Labels[0].Message != "extra argument" {
		t.Fatalf("labels = %#v, want extra argument label", d.Labels)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want call and declaration evidence", d.Explanation.Evidence())
	}
}

func TestDirectCallTooManyArgsSuppressesResultAssignmentDiagnostic(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a
		end
		local x: string = add(1, 2, 3)
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want only too-many-args: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeDirectCallTooManyArgs || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
}

func TestDirectCallReportsWrongArgumentType(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a
		end
		add(1, "wrong")
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 2") || !strings.Contains(d.Message, `"wrong"`) || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want call and parameter evidence", d.Explanation.Evidence())
	}
}

func TestCallParamObligationReportsStricterThanDirectUnionContract(t *testing.T) {
	diags := runDiagnostics(t, `
		local function scale(x: number | string): number
			return x * 2
		end
		scale("not-number")
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want stricter callee obligation only: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 1") || !strings.Contains(d.Message, `"not-number"`) || !strings.Contains(d.Message, "not number") {
		t.Fatalf("message = %q", d.Message)
	}
	evidence := d.Explanation.Evidence()
	if !diagnosticEvidenceContains(evidence, "inside scale, argument 1 must satisfy number") {
		t.Fatalf("explanation = %q, want callee-use obligation evidence", d.Explanation.String())
	}
	if len(evidence) < 2 || !spanEqual(evidence[1].Span, d.Labels[0].Span) {
		t.Fatalf("obligation evidence span = %#v, label span = %#v; want argument-focused evidence", evidence, d.Labels)
	}
}

func TestDirectCallUsesGenericResultFalseEdgeBoundaryProof(t *testing.T) {
	diags := runDiagnostics(t, `
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }

		local function err<T>(message: string): Result<T>
			return { ok = false, error = message }
		end

		local function map_result<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return { ok = true, value = fn(result.value) }
			end
			return err(result.error)
		end

		local r = map_result({ ok = true, value = "x" }, function(value: string): number
			return #value
		end)
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want generic false-edge result.error accepted", diags)
	}
}

func TestDirectCallUsesLoopLocalMethodReturnBoundaryProof(t *testing.T) {
	diags := runDiagnostics(t, `
		type Message = {
			from: fun(self: Message): string,
			payload: fun(self: Message): any,
		}

		type Channel = {
			receive: fun(self: Channel): (Message, boolean),
		}

		local process = {}
		function process.listen(): Channel
			error("stub")
		end
		function process.send(pid: string, topic: string): boolean
			return true
		end

		local done = false
		coroutine.spawn(function()
			local ch = process.listen()
			while not done do
				local msg, ok = ch:receive()
				if not ok then
					break
				end
				local p = msg:payload()
				local data = p and p:data() or nil
				local reply_to = msg:from()
				if type(data) ~= "table" or type(data.amount) ~= "number" then
					process.send(reply_to, "nak")
				else
					process.send(reply_to, "ack")
				end
			end
		end)
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want loop-local method return accepted", diags)
	}
}

func TestDirectCallReportsWrongArgumentTypeInNestedReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a + b
		end
		local function f(): number
			return add("bad", 2)
		end
		local x = f()
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 1") || !strings.Contains(d.Message, `"bad"`) || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestDirectCallSkipsGenericIdentityArgumentClaims(t *testing.T) {
	diags := runDiagnostics(t, `
		local function identity<T>(x: T): T
			return x
		end
		local n: number = identity(42)
		local s: string = identity("hello")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for generic identity call", diags)
	}
}

func TestAnnotationAssignabilityUsesBoundaryProofAfterAssignedTypeCast(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		type Point = {x: number, y: number}
		local function validate(data: any)
			Point(data)
			local p: {x: number, y: number} = data
			return p
		end
		local function validate_assign(data: any)
			local v = Point(data)
			local p: {x: number, y: number} = data
			return p
		end
		local function expect_point(x)
			return Point(x)
		end
		local function validate_wrapped(data: any)
			expect_point(data)
			local p: {x: number, y: number} = data
			return p
		end
		return validate, validate_assign, validate_wrapped
	`, nil)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after type-cast postcondition", diags)
	}
}

func TestDirectCallAcceptsTypedOptionalParam(t *testing.T) {
	diags := runDiagnostics(t, `
		local function log(msg: string, level: string?)
		end
		log("hello")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for typed optional param", diags)
	}
}

func TestDirectCallAcceptsUntypedDefaultOptional(t *testing.T) {
	diags := runDiagnostics(t, `
		local function greet(name, greeting)
			local message = greeting or "Hello"
			return message
		end
		greet("World")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for untyped default optional", diags)
	}
}

func TestDirectCallAcceptsMultipleOrDefaults(t *testing.T) {
	diags := runDiagnostics(t, `
		local function pick(a, b, c)
			local left = b or "left"
			local right = c or "right"
			return left, right
		end
		pick("head")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for trailing defaults", diags)
	}
}

func TestDirectCallAcceptsExplicitNilCheckOptional(t *testing.T) {
	diags := runDiagnostics(t, `
		local function maybe(value: string?)
			if value == nil then
				return
			end
			return value
		end
		maybe()
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for optional nil-checked param", diags)
	}
}

func TestDirectCallAcceptsVariadicExtraArgs(t *testing.T) {
	diags := runDiagnostics(t, `
		local function log(prefix: string, ...: number)
		end
		log("n", 1, 2, 3)
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for typed variadic extra args", diags)
	}
}

func TestDirectCallResultAssignmentReportsAnnotatedLocalMismatch(t *testing.T) {
	src := `
local function add(a: number, b: number): number
	return a + b
end
local x: string = add(1, 2)
`
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallResultAssignment || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "call result") || !strings.Contains(d.Message, "string") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	stmts := mustStmts(t, src)
	fn := stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	assign := stmts[1].(*ast.LocalAssignStmt)
	if got := d.Explanation.Evidence(); len(got) != 2 {
		t.Fatalf("explanation evidence = %#v, want 2 items", got)
	} else {
		if !strings.Contains(got[0].Message, "add declares call result 1 as number") {
			t.Fatalf("return evidence message = %q", got[0].Message)
		}
		if got[0].Span != ast.SpanOf(fn.ReturnTypes[0]) {
			t.Fatalf("return evidence span = %#v, want %#v", got[0].Span, ast.SpanOf(fn.ReturnTypes[0]))
		}
		if got[1].Span != ast.SpanOf(assign.Types[0]) {
			t.Fatalf("declared type evidence span = %#v, want %#v", got[1].Span, ast.SpanOf(assign.Types[0]))
		}
	}
}

func TestDirectCallResultAssignmentReportsTypedMemberCalleeWithoutManifestSignature(t *testing.T) {
	src := strings.TrimLeft(`
type API = { make: () -> number }
local api: API = {
    make = function(): number
        return 1
    end,
}
local x: string = api.make()
`, "\n")
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one direct-call result assignment diagnostic: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallResultAssignment || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want direct-call result assignment", d)
	}
	if !strings.Contains(d.Message, "call result") || !strings.Contains(d.Message, "number") ||
		!strings.Contains(d.Message, "string") {
		t.Fatalf("message = %q, want number result to string target mismatch", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "returns number") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "assignment target x requires string") {
		t.Fatalf("evidence = %#v, want member-call result and target annotation evidence", d.Explanation.Evidence())
	}
	if len(d.Labels) < 2 || d.Labels[0].Message != "call result" || d.Labels[1].Message != "declared type" ||
		d.Labels[0].Span != d.Span {
		t.Fatalf("labels/span = %#v/%#v, want call-result span and declared type labels", d.Labels, d.Span)
	}
	rendered := diagnostic.Render(d, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.call.direct.result_assignment]: call result 1 is number, not string
 --> diagnostics_test.lua:7:19
  |
  |          ↓ declared type
7 | local x: string = api.make()
  |                   ↑ call result

because:
  1. proven: api.make returns number
  2. claimed: assignment target x requires string

help: Assign the call result to a compatible target type, or change the callee return type if this result is valid.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestDirectCallMemberArgumentProofFailureTakesPrecedenceOverResultAssignment(t *testing.T) {
	diags := runDiagnostics(t, `
type API = { make: (name: string) -> number }
local api: API = {
	make = function(name: string): number
		return 1
	end,
}
local x: string = api.make(42)
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one direct-call argument diagnostic: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType {
		t.Fatalf("diagnostic = %#v, want direct-call argument diagnostic", d)
	}
	if !diagnosticHasLabel(d, "argument value") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "argument 1 has literal value 42") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "parameter 1 expects string") {
		t.Fatalf("diagnostic = %#v, want member-call argument value and parameter evidence", d)
	}
	for _, diag := range diags {
		if diag.Code == CodeDirectCallResultAssignment {
			t.Fatalf("diagnostics include result-assignment diagnostic despite member-call argument proof failure: %#v", diags)
		}
	}
}

func TestDirectCallArgumentProofFailureTakesPrecedenceOverResultAssignment(t *testing.T) {
	diags := runDiagnostics(t, `
local function f(x: { id: string }): number
	return 1
end

local raw = ({ id = "ok" } :: any)
local y: string = f(raw)
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one direct-call argument diagnostic: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType {
		t.Fatalf("diagnostic = %#v, want direct-call argument diagnostic", d)
	}
	if got := d.Explanation.String(); !strings.Contains(got, "f parameter 1 expects") ||
		!strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "no proof on this path shows raw satisfies the parameter type") {
		t.Fatalf("explanation = %q, want parameter declaration and explicit-any missing-proof evidence", got)
	}
	for _, diag := range diags {
		if diag.Code == CodeDirectCallResultAssignment {
			t.Fatalf("diagnostics include result-assignment diagnostic despite argument proof failure: %#v", diags)
		}
	}
}

func TestDirectCallResultAssignmentSkipsGenericReturnContracts(t *testing.T) {
	diags := runDiagnostics(t, `
		local function id<T>(x: T): T
			return x
		end
		local s: string = id("hello")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for generic return contract", diags)
	}
}

func runDiagnostics(t *testing.T, src string) []diagnostic.Diagnostic {
	t.Helper()
	return runDiagnosticsWithGlobals(t, src, []string{"test", "type", "value"})
}

func renderLineDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	n := len(wantLines)
	if len(gotLines) > n {
		n = len(gotLines)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		var wantLine, gotLine string
		if i < len(wantLines) {
			wantLine = wantLines[i]
		}
		if i < len(gotLines) {
			gotLine = gotLines[i]
		}
		if wantLine == gotLine {
			continue
		}
		b.WriteString("- ")
		b.WriteString(wantLine)
		b.WriteByte('\n')
		b.WriteString("+ ")
		b.WriteString(gotLine)
		b.WriteByte('\n')
	}
	return b.String()
}

func runDiagnosticsWithGlobals(t *testing.T, src string, globals []string) []diagnostic.Diagnostic {
	t.Helper()
	return runDiagnosticsFull(t, src, globals, signaturelookup.Source{})
}

func runDiagnosticsWithSignatures(t *testing.T, src string, signatures signaturelookup.Source) []diagnostic.Diagnostic {
	t.Helper()
	return runDiagnosticsFull(t, src, []string{"test", "type", "value"}, signatures)
}

func runDiagnosticsFull(t *testing.T, src string, globals []string, signatures signaturelookup.Source) []diagnostic.Diagnostic {
	t.Helper()
	return Produce(runDiagnosticsResultFull(t, src, globals, signatures))
}

func runDiagnosticsResult(t *testing.T, src string) *body.Result {
	t.Helper()
	return runDiagnosticsResultFull(t, src, []string{"test", "type", "value"}, signaturelookup.Source{})
}

func runDiagnosticsResultFull(t *testing.T, src string, globals []string, signatures signaturelookup.Source) *body.Result {
	t.Helper()
	stmts, err := parse.ParseString(src, "diagnostics_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	reg := standard.Registry()
	result, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{
			Registry:   reg,
			Globals:    globals,
			Signatures: signatures,
		},
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	return result.RootResult()
}

func requireLocalAssignmentExprByName(t *testing.T, result *body.Result, name string) (cfg.Point, ast.Expr) {
	t.Helper()
	for _, point := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || fact.Name != name || fact.Expr == nil {
			continue
		}
		return point, fact.Expr
	}
	t.Fatalf("local assignment %q not found", name)
	return 0, nil
}

func TestDominatingRootLocalAssignmentFindsDominatingDeclaration(t *testing.T) {
	result := runDiagnosticsResult(t, `
		local source: string = "ok"
		if test then
			local sink: string = source
		end
	`)
	point, expr := requireLocalAssignmentExprByName(t, result, "sink")
	p, ok := result.ExpressionPath(expr)
	if !ok || p.Symbol == 0 {
		t.Fatalf("sink expression path = %v, %v; want source symbol", p, ok)
	}
	fact, declarationPoint, ok := dominatingRootLocalAssignment(result, nil, point, p.Symbol)
	if !ok {
		t.Fatalf("dominatingRootLocalAssignment = false, want source declaration")
	}
	if declarationPoint == 0 || fact.Name != "source" || fact.Type == nil {
		t.Fatalf("dominating declaration = (%#v, %d), want typed source declaration", fact, declarationPoint)
	}
}

func TestDominatingRootLocalAssignmentStopsAtDominatingRootWrite(t *testing.T) {
	result := runDiagnosticsResult(t, `
		local source: string = "ok"
		source = "mutated"
		local sink: string = source
	`)
	point, expr := requireLocalAssignmentExprByName(t, result, "sink")
	p, ok := result.ExpressionPath(expr)
	if !ok || p.Symbol == 0 {
		t.Fatalf("sink expression path = %v, %v; want source symbol", p, ok)
	}
	if fact, declarationPoint, ok := dominatingRootLocalAssignment(result, nil, point, p.Symbol); ok {
		t.Fatalf("dominating declaration = (%#v, %d), want blocked by root write", fact, declarationPoint)
	}
}

func TestDominatingRootLocalAssignmentUsesDiagnosticFlowCache(t *testing.T) {
	result := runDiagnosticsResult(t, `
		local source: string = "ok"
		local sink: string = source
	`)
	point, expr := requireLocalAssignmentExprByName(t, result, "sink")
	p, ok := result.ExpressionPath(expr)
	if !ok || p.Symbol == 0 {
		t.Fatalf("sink expression path = %v, %v; want source symbol", p, ok)
	}
	flow := newDiagnosticFlowCache(result)
	idom := flow.immediateDominators()
	for child := range idom {
		idom[child] = child
	}
	if fact, declarationPoint, ok := dominatingRootLocalAssignment(result, flow, point, p.Symbol); ok {
		t.Fatalf("dominating declaration = (%#v, %d), want cached dominators to block lookup", fact, declarationPoint)
	}
}

func TestOptionalPathEquivalenceUsesDominatingAliasDeclaration(t *testing.T) {
	root := runDiagnosticsResult(t, `
		local function remember(maybe: string?, sink: { seen: string }): string
			local alias = maybe
			if alias ~= nil then
				sink.seen = maybe
			end
			return sink.seen
		end
	`)
	if len(root.FunctionResults()) != 1 {
		t.Fatalf("nested function results = %d, want 1", len(root.FunctionResults()))
	}
	result := root.FunctionResults()[0]
	flow := newDiagnosticFlowCache(result)
	resolver := newResultResolver(result, newResultResolver(root, nil))
	branchPoint, aliasPath := requireBranchCheckPath(t, result, branchcond.CheckNotNil, "alias")
	_, aliasExpr := requireLocalAssignmentExprByName(t, result, "alias")
	maybePath, ok := result.ExpressionPath(aliasExpr)
	if !ok || maybePath.IsEmpty() {
		t.Fatalf("alias source path = %v, %v; want maybe path", maybePath, ok)
	}
	if !optionalPathsEquivalentAt(result, flow, branchPoint, maybePath, aliasPath) {
		t.Fatalf("optionalPathsEquivalentAt(%s, %s) = false, want dominating alias equivalence", maybePath, aliasPath)
	}
	if !optionalPathConsumesTarget(result, flow, branchPoint, maybePath, aliasPath) {
		t.Fatalf("optionalPathConsumesTarget(%s, %s) = false, want alias-guarded source consumption", maybePath, aliasPath)
	}
	aliasType, ok := optionalPathType(result, resolver, flow, branchPoint, aliasPath)
	if !ok || !optionalTypeHasValue(aliasType) {
		t.Fatalf("optionalPathType(%s) = (%v, %v), want optional source type", aliasPath, aliasType, ok)
	}
}

func TestStaticStringExprValueAtUsesBoundaryLiteralType(t *testing.T) {
	result := runDiagnosticsResult(t, `
		local key = "audit"
		local sink = key
	`)
	point, expr := requireLocalAssignmentExprByName(t, result, "sink")
	got, ok := staticStringExprValueAt(result, point, expr)
	if !ok || got != "audit" {
		t.Fatalf("staticStringExprValueAt = (%q, %v), want audit literal", got, ok)
	}
}

func TestDiagnosticFlowCacheCachesImmediateDominators(t *testing.T) {
	result := runDiagnosticsResult(t, `
		local seed = 1
		local next = seed + 1
		local final = next + 1
	`)
	flow := newDiagnosticFlowCache(result)
	first := flow.immediateDominators()
	if len(first) == 0 {
		t.Fatalf("immediate dominators = empty")
	}
	sentinel := cfg.Point(999999)
	first[sentinel] = result.Graph().Entry()
	second := flow.immediateDominators()
	if got, ok := second[sentinel]; !ok || got != result.Graph().Entry() {
		t.Fatalf("immediate dominators were recomputed instead of cached: got (%d, %v)", got, ok)
	}
	delete(second, sentinel)
}

func TestDiagnosticFlowCacheTreatsEntryAsReachablePoint(t *testing.T) {
	result := runDiagnosticsResult(t, `
		local seed = 1
		local next = seed + 1
	`)
	graph := result.Graph()
	flow := newDiagnosticFlowCache(result)
	if !flow.canReach(graph.Entry(), graph.Exit()) {
		t.Fatalf("entry should reach exit through diagnostic flow cache")
	}
	if !flow.canReach(graph.Entry(), graph.Entry()) {
		t.Fatalf("entry should reach itself through diagnostic flow cache")
	}
}

func requireBranchCheckPath(t *testing.T, result *body.Result, kind branchcond.CheckKind, pathText string) (cfg.Point, path.Path) {
	t.Helper()
	var available []string
	for _, point := range result.Graph().RPO() {
		fact, ok := result.BranchCondition(point)
		if ok {
			available = append(available, fact.Check.Path.String())
		}
		if !ok || fact.Check.Kind != kind || fact.Check.Path.String() != pathText {
			continue
		}
		return point, fact.Check.Path
	}
	t.Fatalf("branch check %v for %q not found; available branch paths: %v", kind, pathText, available)
	return 0, path.Path{}
}

func TestTruthyDominatingBranchProofsRecognizeTrueArmPresence(t *testing.T) {
	result := runDiagnosticsResult(t, `
		local owner: string? = value
		if owner then
			local out = ":" .. owner
		end
	`)
	point, expr := requireLocalAssignmentExprByName(t, result, "out")
	concat, ok := expr.(*ast.StringConcatOpExpr)
	if !ok {
		t.Fatalf("out expression = %T, want string concat", expr)
	}
	ownerPath, ok := result.ExpressionPath(concat.Rhs)
	if !ok || ownerPath.IsEmpty() {
		t.Fatalf("owner expression path = %v, %v; want resolved path", ownerPath, ok)
	}
	if !newTruthyDominatingBranchProofs(result).provesPresent(point, ownerPath) {
		t.Fatalf("truthy guard did not prove %s present at concat point", ownerPath.String())
	}
}

func TestTruthyDominatingBranchProofsDoNotLeakPastJoin(t *testing.T) {
	result := runDiagnosticsResult(t, `
		local owner: string? = value
		if owner then
			local inside = owner
		end
		local out = ":" .. owner
	`)
	point, expr := requireLocalAssignmentExprByName(t, result, "out")
	concat, ok := expr.(*ast.StringConcatOpExpr)
	if !ok {
		t.Fatalf("out expression = %T, want string concat", expr)
	}
	ownerPath, ok := result.ExpressionPath(concat.Rhs)
	if !ok || ownerPath.IsEmpty() {
		t.Fatalf("owner expression path = %v, %v; want resolved path", ownerPath, ok)
	}
	if newTruthyDominatingBranchProofs(result).provesPresent(point, ownerPath) {
		t.Fatalf("truthy guard incorrectly proved %s present after the if join", ownerPath.String())
	}
}

func diagnosticMessages(diags []diagnostic.Diagnostic) []string {
	out := make([]string, len(diags))
	for i, diag := range diags {
		out[i] = diag.Message
	}
	return out
}

func containsDiagnosticMessage(messages []string, want string) bool {
	for _, message := range messages {
		if strings.Contains(message, want) {
			return true
		}
	}
	return false
}

func mustStmts(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "diagnostics_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts
}

func mustFunctionExpr(t *testing.T, src string) *ast.FunctionExpr {
	t.Helper()
	stmts := mustStmts(t, src)
	if len(stmts) != 1 {
		t.Fatalf("stmts = %d, want 1", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want *ast.FuncDefStmt with function", stmts[0])
	}
	return def.Func
}
