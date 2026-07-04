package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	internalreadmodel "github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/typ"
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

func TestProduceSuppressesAssignmentCascadeFromUnresolvedRoot(t *testing.T) {
	diags := runDiagnosticsFull(t, `local n: number = provider.meta()`, nil, signaturelookup.Source{})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want only unresolved provider", diags)
	}
	if diags[0].Code != CodeUnresolvedValueReference {
		t.Fatalf("diagnostic code = %s, want %s", diags[0].Code, CodeUnresolvedValueReference)
	}
}

func TestProduceSuppressesConcatCascadeFromInvalidLocalAssignment(t *testing.T) {
	diags := runDiagnostics(t, `
type Session = { user_id: string? }
local ctx: { session: Session } = { session = { user_id = nil } }
local user_id: string = ctx.session.user_id
local body = user_id .. ":suffix"
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want only assignment cause: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s: %#v", diags[0].Code, CodeAssignmentType, diags)
	}
}

func TestProduceKeepsUntrustedAnyAssignmentWithoutUnresolvedRoot(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `local n: number = provider.meta()`, []string{"provider"})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want untrusted any assignment", diags)
	}
	if diags[0].Code != CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", diags[0].Code, CodeAssignmentType)
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

func TestDiagnosticProducerRegistryHasDirectCallJudgmentOwner(t *testing.T) {
	count := 0
	for _, producer := range diagnosticProducers() {
		if diagnosticProducerOwnsCode(producer, CodeDirectCallNotCallable) &&
			diagnosticProducerOwnsCode(producer, CodeDirectCallTooFewArgs) &&
			diagnosticProducerOwnsCode(producer, CodeDirectCallTooManyArgs) &&
			diagnosticProducerOwnsCode(producer, CodeDirectCallArgType) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("direct-call judgment producer entries = %d, want 1", count)
	}
	for _, producer := range diagnosticProducers() {
		if len(producer.codes) == 1 && producer.codes[0] == CodeDirectCallArgType {
			t.Fatalf("direct-call argument-only migration producer should be deleted")
		}
	}
}

func diagnosticProducerOwnsCode(producer diagnosticProducer, code diagnostic.Code) bool {
	for _, got := range producer.codes {
		if got == code {
			return true
		}
	}
	return false
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

func TestAnnotationAssignabilityAcceptsUnannotatedArrayLiteralConstantIndex(t *testing.T) {
	for _, src := range []string{
		`local arr = {1, 2, 3}
local n: number = arr[1]`,
		`local arr = {1, 2, 3}
local n: number = arr[3]`,
	} {
		diags := runDiagnostics(t, src)
		if len(diags) != 0 {
			t.Fatalf("diagnostics = %#v, want none for %q", diags, src)
		}
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
	if !strings.Contains(d.Message, "cannot assign arr[3]") || !strings.Contains(d.Message, "may be nil") || !strings.Contains(d.Explanation.String(), "arr[3] is an indexed read that can miss or read nil") {
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
	if !strings.Contains(d.Message, "cannot assign arr[0]") || !strings.Contains(d.Message, "may be nil") || !strings.Contains(d.Explanation.String(), "arr[0] is an indexed read that can miss or read nil") {
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
		!strings.Contains(d.Explanation.String(), "returned value 1 (xs[1]) is an indexed read that can miss or read nil") {
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
	if !strings.Contains(d.Message, "cannot assign arr[2]") || !strings.Contains(d.Message, "may be nil") || !strings.Contains(d.Explanation.String(), "arr[2] is an indexed read that can miss or read nil") {
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
	if !strings.Contains(d.Message, "cannot assign lookup[2]") || !strings.Contains(d.Message, "may be nil") || !strings.Contains(d.Explanation.String(), "lookup[2] is an indexed read that can miss or read nil") {
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
		`maybe.tags["source"] is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared type here`,
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

func TestAnnotationAssignabilityReportsOptionalCallResultMemberReadWithoutGuard(t *testing.T) {
	src := `
type Config = {host: string, port: number}
local function parse_config(ok: boolean): (Config?, string?)
	if not ok then
		return nil, "failed"
	end
	return {host = "localhost", port = 8080}, nil
end
local function use(ok: boolean)
	local cfg, err = parse_config(ok)
	local host: string = cfg.host
end
`
	root := runDiagnosticsResult(t, src)
	diags := Produce(root)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one assignment diagnostic", diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cfg.host") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("diagnostic = %#v, want optional call-result member assignment", d)
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
  3. missing proof: maybe.tags["source"] is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared type here

help: Guard ` + "`maybe.tags[\"source\"]`" + ` with a nil check, provide a default value, or change the target type to accept nil.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestAnnotationAssignabilityReportsOptionalAliasMapIndex(t *testing.T) {
	diags := runDiagnostics(t, `
type RequestMeta = {
	tags: {[string]: string}?,
}
type Message = {
	meta: RequestMeta,
}
local msg: Message = {
	meta = {
		tags = nil,
	},
}
local tags = msg.meta.tags
local bad_source: string = tags["source"]
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one optional alias map-index assignment", diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || !strings.Contains(d.Message, `tags["source"]`) || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("diagnostic = %#v, want optional alias map-index assignment", d)
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
		!strings.Contains(got, "raw comes from any/unknown") ||
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
		!diagnosticEvidenceContains(evidence, "need_id parameter 1.id expects {id: string}") ||
		!diagnosticEvidenceContains(evidence, `object literal does not provide field "id"`) {
		t.Fatalf("evidence = %#v, want actual type, constraint, and missing-field missing proof", evidence)
	}
	missing := evidence[len(evidence)-1]
	if missing.Kind != diagnostic.EvidenceMissingProof {
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
		return ""
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

func TestAnnotationAssignabilityUsesDeclaredMapContractForDynamicWriteAfterLiteralInitializer(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(header_name: string, header_value: string): ()
			local headers: {[string]: string} = {
				["content-type"] = "application/json",
				["accept"] = "application/json",
			}
			headers[header_name] = header_value
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want declared string-map write contract instead of literal initializer slots", diags)
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
		call = call || strings.Contains(msg, "argument 1 (raw) comes from any/unknown") && strings.Contains(msg, "no proof shows it is")
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

func TestAnnotationAssignabilityReportsUntrustedIdentifierSource(t *testing.T) {
	diags := runDiagnostics(t, `
		local y = value
		local x: number = y
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want untrusted identifier assignment error", diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign y") || !strings.Contains(d.Message, "any") {
		t.Fatalf("diagnostic = %#v, want untrusted identifier assignment error", d)
	}
}

func TestAnnotationAssignabilityReportsAnnotatedIdentifierOptionalWithoutNarrowing(t *testing.T) {
	diags := runDiagnostics(t, `
		local x: string? = nil
		local s: string = x
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want optional identifier assignment error", diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign x") || !strings.Contains(d.Message, "nil, not string") {
		t.Fatalf("diagnostic = %#v, want exact nil identifier assignment error", d)
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

func TestAnnotationAssignabilityUsesElementRuntimeTypeGuardWithoutProvingArray(t *testing.T) {
	diags := runDiagnostics(t, `
local raw: any = {
	items = {"ok", 42},
}

if type(raw.items) == "table" and type(raw.items[1]) == "string" then
	local first: string = raw.items[1]
	local all_items: {string} = raw.items
end
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want only array-shape assignment diagnostic", diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "raw.items") || strings.Contains(d.Message, "raw.items[1]") {
		t.Fatalf("diagnostic = %#v, want raw.items array-shape assignment only", d)
	}
}

func TestAnnotationAssignabilityUsesTypeIsErrorBranchState(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {x: number, y: number}
		function validate(data: any)
			local val, err = Point:is(data)
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
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none: concrete local initializer proves departments present", diags)
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
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none: guard calls preserve the proven departments table", diags)
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
	if len(diags) != 3 {
		t.Fatalf("diagnostics = %#v, want strict-any assignment errors after type-cast postcondition", diags)
	}
	for _, d := range diags {
		if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "data") || !strings.Contains(d.Message, "any") {
			t.Fatalf("diagnostic = %#v, want strict-any assignment error", d)
		}
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

func TestProduceSuppressesLoopDynamicAssignmentCascade(t *testing.T) {
	diags := runDiagnosticsFull(t, `
local item = {
	count = 1,
	name = "ready",
}

for key, value in pairs(item) do
	item[key] = tostring(value)
end

local count: number = item.count
`, []string{"pairs", "tostring"}, signaturelookup.Source{IncludeStdlib: true})
	var assignments []diagnostic.Diagnostic
	for _, diag := range diags {
		if diag.Code == CodeAssignmentType {
			assignments = append(assignments, diag)
		}
	}
	if len(assignments) != 1 {
		t.Fatalf("assignment diagnostics = %d, want only dynamic write cause: %#v", len(assignments), assignments)
	}
	if !strings.Contains(assignments[0].Message, "tostring(...)") || strings.Contains(assignments[0].Message, "item.count") {
		t.Fatalf("assignment message = %q, want dynamic write cause", assignments[0].Message)
	}
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
	fact, declarationPoint, ok := result.DominatingRootLocalAssignment(point, p.Symbol)
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
	if fact, declarationPoint, ok := result.DominatingRootLocalAssignment(point, p.Symbol); ok {
		t.Fatalf("dominating declaration = (%#v, %d), want blocked by root write", fact, declarationPoint)
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
	branchPoint, aliasPath := requireBranchCheckPath(t, result, branchcond.CheckNotNil, "alias")
	_, aliasExpr := requireLocalAssignmentExprByName(t, result, "alias")
	maybePath, ok := result.ExpressionPath(aliasExpr)
	if !ok || maybePath.IsEmpty() {
		t.Fatalf("alias source path = %v, %v; want maybe path", maybePath, ok)
	}
	if !result.PathsAliasAtBoundary(branchPoint, maybePath, aliasPath) {
		t.Fatalf("PathsAliasAtBoundary(%s, %s) = false, want dominating alias equivalence", maybePath, aliasPath)
	}
	var got []string
	internalreadmodel.New(result).ForEachOptionalExhaustiveness(func(item internalreadmodel.OptionalExhaustiveness) bool {
		got = append(got, item.Target)
		return true
	})
	if len(got) != 1 || got[0] != "alias" {
		t.Fatalf("optional exhaustiveness targets = %#v, want alias", got)
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
