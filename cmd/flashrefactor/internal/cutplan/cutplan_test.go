package cutplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIntentV3IsStrictAndCanonical(t *testing.T) {
	left := testIntent()
	right := testIntent()
	right.Operations[0].Edits[0].Relocate.Subjects = []Relocation{
		{From: object("old", "Flow.Count"), To: object("new", "Flow.Count")},
		{From: object("old", "Flow"), To: object("new", "Flow")},
	}
	right.Operations[0].Verify.Gates = []Gate{GateResidue, GateDiagnostics, GateImportDAG}
	leftDigest, err := IntentDigest(left)
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := IntentDigest(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != rightDigest {
		t.Fatalf("declarative order changed digest: %s != %s", leftDigest, rightDigest)
	}
	encoded, err := json.Marshal(left)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeIntent(encoded); err != nil {
		t.Fatalf("strict intent roundtrip: %v", err)
	}
	if _, err := DecodeIntent([]byte(`{"schema":3,"name":"cut","operations":[],"shell":"no"}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestLawPackageRejectsPatterns(t *testing.T) {
	for _, candidate := range []string{"./...", "./pkg/*", "./pkg,./other", "./pkg?", "./pkg/[x]"} {
		intent := testIntent()
		intent.Operations[0].Verify.Laws[0].Package = candidate
		if err := ValidateIntent(intent); err == nil {
			t.Fatalf("law package pattern accepted: %q", candidate)
		}
	}
}

func TestResolutionRequirementsAreExactAndCanonical(t *testing.T) {
	got, err := ResolutionRequirements(testIntent())
	if err != nil {
		t.Fatal(err)
	}
	want := []ResolutionRequirement{
		{Object: object("new", "Flow"), Role: ObjectTarget, Path: "internal/flow.go", Package: "internal"},
		{Object: object("new", "Flow.Count"), Role: ObjectTarget, Path: "internal/flow.go", Package: "internal"},
		{Object: object("old", "Flow"), Role: ObjectSource, Path: "internal/old.go"},
		{Object: object("old", "Flow.Count"), Role: ObjectSource, Path: "internal/old.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("requirements mismatch:\n got: %#v\nwant: %#v", got, want)
	}
	for index := 1; index < len(got); index++ {
		if got[index-1].Object.Object >= got[index].Object.Object {
			t.Fatalf("requirements are not in canonical object order: %#v", got)
		}
	}
}

func TestImpactObjectsExcludeContainment(t *testing.T) {
	intent := testIntent()
	intent.Operations[0].Edits[0].Relocate.Containment = &Containment{
		Parent: object("old", "Link"), Child: object("new", "State"), Through: object("new", "Link.state"),
	}
	impact, err := ImpactObjects(intent)
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range impact {
		if object.Object == "example.invalid/old#package:Link" || object.Object == "example.invalid/new#package:State" || object.Object == "example.invalid/new#type:Link/field:state" {
			t.Fatalf("containment object entered impact denominator: %#v", impact)
		}
	}
	if len(impact) != 4 {
		t.Fatalf("impact denominator=%#v", impact)
	}
}

func TestResolutionRequirementsLocateContainmentAndRejectCollisions(t *testing.T) {
	intent := testIntent()
	intent.Operations[0].Edits[0].Relocate.Containment = &Containment{
		Parent: object("old", "Link"), Child: object("new", "State"), Through: object("new", "Link.state"),
	}
	requirements, err := ResolutionRequirements(intent)
	if err != nil {
		t.Fatal(err)
	}
	byObject := map[string]ResolutionRequirement{}
	for _, requirement := range requirements {
		byObject[requirement.Object.Object] = requirement
	}
	for _, want := range []ResolutionRequirement{
		{Object: object("old", "Link"), Role: ObjectSource, Path: "internal/old.go"},
		{Object: object("new", "State"), Role: ObjectTarget, Path: "internal/flow.go", Package: "internal"},
		{Object: object("new", "Link.state"), Role: ObjectTarget, Path: "internal/old.go"},
	} {
		if got := byObject[want.Object.Object]; !reflect.DeepEqual(got, want) {
			t.Fatalf("containment requirement for %s = %#v, want %#v", want.Object.Object, got, want)
		}
	}

	intent = testIntent()
	intent.Operations[0].Edits[0].Relocate.Containment = &Containment{
		Parent: object("old", "Link"), Child: object("new", "State"), Through: object("new", "Flow.Count"),
	}
	if _, err := ResolutionRequirements(intent); err == nil || !strings.Contains(err.Error(), "resolution location collision") {
		t.Fatalf("containment/source-subject location collision accepted: %v", err)
	}
}

func TestEditSumAndRegisteredGeneratorFailClosed(t *testing.T) {
	intent := testIntent()
	edit := &intent.Operations[0].Edits[0]
	edit.Generate = &Generate{Provider: Provider("fixture-generator"), Destination: "internal/flow.go"}
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "exactly one payload") {
		t.Fatalf("multi-payload edit accepted: %v", err)
	}
	intent = testIntent()
	intent.Operations[0].Edits = []Edit{{Kind: EditGenerate, Generate: &Generate{Provider: Provider("go test; rm -rf"), Destination: "internal/generated.go"}}}
	intent.Operations[0].Footprint = Footprint{Read: []string{"internal/old.go"}, Write: []string{"internal/generated.go"}}
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "provider key") {
		t.Fatalf("unregistered/shell provider accepted: %v", err)
	}
	intent.Operations[0].Edits[0].Generate.Provider = Provider("fixture`run")
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "provider key") {
		t.Fatalf("shell-bearing provider accepted: %v", err)
	}
	intent = testIntent()
	intent.Operations[0].Edits = []Edit{{Kind: EditRetire, Retire: &Retire{Source: "internal/old.go", Symbols: []SymbolRef{object("old", "Flow")}}}}
	intent.Operations[0].Footprint = Footprint{Read: []string{"internal/old.go"}, Write: []string{"internal/old.go"}}
	intent.Operations[0].Bindings = nil
	intent.Operations[0].Imports = nil
	if err := ValidateIntent(intent); err != nil {
		t.Fatalf("valid retire rejected: %v", err)
	}
}

func TestExactBindingsImportsAndObjectAbsence(t *testing.T) {
	intent := testIntent()
	intent.Operations[0].Bindings[0].Consumer = "elsewhere.go"
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "binding consumer") {
		t.Fatalf("unfootprinted binding accepted: %v", err)
	}
	intent = testIntent()
	intent.Operations[0].Imports[0].From = nil
	intent.Operations[0].Imports[0].To = nil
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "at least one endpoint") {
		t.Fatalf("empty import endpoint accepted: %v", err)
	}
	intent = testIntent()
	intent.Operations[0].Imports[0].To.Name = ""
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "destination import") {
		t.Fatalf("import without declared package name accepted: %v", err)
	}
	intent = testIntent()
	intent.Operations[0].Bindings = nil
	intent.Operations[0].Imports = nil
	if err := ValidateIntent(intent); err != nil {
		t.Fatalf("optional exact lists require an obsolete empty marker: %v", err)
	}
	intent = testIntent()
	intent.Operations[0].Footprint.Write = append(intent.Operations[0].Footprint.Write, "internal/unclaimed.go")
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "not claimed") {
		t.Fatalf("unclaimed write footprint accepted: %v", err)
	}
}

func TestRelocationAuthorsEveryTargetAndContainmentObject(t *testing.T) {
	intent := testIntent()
	relocate := intent.Operations[0].Edits[0].Relocate
	relocate.Destination.Package = ""
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "source and destination") {
		t.Fatalf("relocation without target package clause accepted: %v", err)
	}
	intent = testIntent()
	relocate = intent.Operations[0].Edits[0].Relocate
	relocate.Subjects[1].To = relocate.Subjects[0].To
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "exactly once") {
		t.Fatalf("two sources targeting one declaration accepted: %v", err)
	}
	intent = testIntent()
	relocate = intent.Operations[0].Edits[0].Relocate
	relocate.Containment = &Containment{
		Parent: object("old", "Link"), Child: object("new", "Flow"), Through: object("new", "Link.flow"),
	}
	if err := ValidateIntent(intent); err != nil {
		t.Fatalf("fully named containment rejected: %v", err)
	}
	relocate.Containment.Through = SymbolRef{}
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "containment") {
		t.Fatalf("containment without inserted field accepted: %v", err)
	}
	legacy := `{"schema":2,"name":"legacy","operations":[{"id":"x","symbols":[]}]}`
	if _, err := DecodeIntent([]byte(legacy)); err == nil {
		t.Fatal("source-only legacy relocation spelling accepted")
	}
}

func TestLockBindsExactFootprintResolutionAndExecution(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "internal/old.go", "old")
	intent := testIntent()
	inputs, err := Fingerprint(root, ReadPaths(intent), []string{"internal/flow.go"})
	if err != nil {
		t.Fatal(err)
	}
	evidence := testEvidence(intent, inputs)
	lock, err := BuildLock(intent, testToolchain(), evidence)
	if err != nil {
		t.Fatalf("build lock: %v", err)
	}
	if err := VerifyLock(root, lock); err != nil {
		t.Fatalf("verify lock: %v", err)
	}
	writeTestFile(t, root, "internal/old.go", "new-old")
	writeTestFile(t, root, "internal/flow.go", "new-flow")
	if err := VerifyOutputs(root, lock); err != nil {
		t.Fatalf("verify outputs: %v", err)
	}
	if err := VerifyDiff([]byte("exact diff"), lock); err != nil {
		t.Fatalf("verify diff: %v", err)
	}
	if err := VerifyDiff([]byte("different diff"), lock); err == nil || !strings.Contains(err.Error(), "diff changed") {
		t.Fatalf("changed diff accepted: %v", err)
	}
	evidence = testEvidence(intent, inputs)
	evidence.Resolution.Objects = evidence.Resolution.Objects[:1]
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "missing object") {
		t.Fatalf("missing object evidence accepted: %v", err)
	}
	evidence = testEvidence(intent, inputs)
	evidence.Execution.Touched = []string{"internal/old.go"}
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "touched files") {
		t.Fatalf("partial output footprint accepted: %v", err)
	}
	evidence = testEvidence(intent, inputs)
	for index := range evidence.Resolution.Objects {
		if evidence.Resolution.Objects[index].Role == ObjectSource {
			evidence.Resolution.Objects[index].Role = ObjectTarget
			break
		}
	}
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "classification") {
		t.Fatalf("misclassified source object accepted: %v", err)
	}
	evidence = testEvidence(intent, inputs)
	for index := range evidence.Resolution.Objects {
		if evidence.Resolution.Objects[index].Role == ObjectTarget {
			evidence.Resolution.Objects[index].Package = "wrong"
			break
		}
	}
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "declared destination") {
		t.Fatalf("target evidence outside declared destination accepted: %v", err)
	}
	evidence = testEvidence(intent, inputs)
	for index := range evidence.Resolution.Objects {
		if evidence.Resolution.Objects[index].Role == ObjectSource {
			evidence.Resolution.Objects[index].Definition.Path = "internal/wrong.go"
			break
		}
	}
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "declared source") {
		t.Fatalf("source evidence outside declared source accepted: %v", err)
	}
}

func TestLockRejectsHazardsAndInputDrift(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "internal/old.go", "old")
	intent := testIntent()
	inputs, err := Fingerprint(root, ReadPaths(intent), []string{"internal/flow.go"})
	if err != nil {
		t.Fatal(err)
	}
	evidence := testEvidence(intent, inputs)
	evidence.Hazards = []Hazard{{Code: "cycle", Severity: "error", Detail: "cycle", Paths: []string{"internal/old.go"}}}
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "blocking hazard") {
		t.Fatalf("blocking hazard accepted: %v", err)
	}
	lock, err := BuildLock(intent, testToolchain(), testEvidence(intent, inputs))
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "internal/old.go", "drift")
	if err := VerifyLock(root, lock); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("input drift accepted: %v", err)
	}
}

func TestLockRequiresExactRelocationRoutesAndGateUnion(t *testing.T) {
	intent := testIntent()
	inputs := InputFingerprint{
		Files:  []HashPath{{Path: "internal/old.go", SHA256: digestText("old")}},
		Absent: []string{"internal/flow.go"},
	}
	evidence := testEvidence(intent, inputs)
	if len(evidence.Routes) != 2 || len(evidence.Gates) != 3 {
		t.Fatalf("fixture lost generated denominator: routes=%d gates=%d", len(evidence.Routes), len(evidence.Gates))
	}
	evidence.Routes = evidence.Routes[:1]
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "missing reference route") {
		t.Fatalf("missing relocation route accepted: %v", err)
	}

	evidence = testEvidence(intent, inputs)
	evidence.Routes[0].Sites = evidence.Routes[0].Sites[:1]
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "does not cover every source site") {
		t.Fatalf("incomplete exact source route accepted: %v", err)
	}

	evidence = testEvidence(intent, inputs)
	evidence.Routes[0].Sites[1].Target = evidence.Routes[0].Sites[0].Target
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "target site is not unique") {
		t.Fatalf("duplicate target route accepted: %v", err)
	}

	evidence = testEvidence(intent, inputs)
	evidence.Gates = evidence.Gates[:2]
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "missing gate evidence") {
		t.Fatalf("missing requested gate accepted: %v", err)
	}

	evidence = testEvidence(intent, inputs)
	evidence.Gates = append(evidence.Gates, GateEvidence{Gate: GateDiagnostics, ResultSHA256: digestText("duplicate")})
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "duplicate gate evidence") {
		t.Fatalf("duplicate gate evidence accepted: %v", err)
	}
}

func TestLockToolchainBindsAllExecutableInputs(t *testing.T) {
	intent := testIntent()
	inputs := InputFingerprint{
		Files:  []HashPath{{Path: "internal/old.go", SHA256: digestText("old")}},
		Absent: []string{"internal/flow.go"},
	}
	for _, mutate := range []func(*Toolchain){
		func(value *Toolchain) { value.HelperBuild = "" },
		func(value *Toolchain) { value.HelperSHA256 = "invalid" },
		func(value *Toolchain) { value.GoVersion = "" },
		func(value *Toolchain) { value.GoExecutableSHA256 = "invalid" },
		func(value *Toolchain) { value.Resolver = "" },
		func(value *Toolchain) { value.BuildEnvSHA256 = "invalid" },
		func(value *Toolchain) { value.ModuleGraphSHA256 = "invalid" },
	} {
		toolchain := testToolchain()
		mutate(&toolchain)
		if _, err := BuildLock(intent, toolchain, testEvidence(intent, inputs)); err == nil || !strings.Contains(err.Error(), "toolchain must bind") {
			t.Fatalf("incomplete toolchain accepted: %+v", toolchain)
		}
	}
}

func TestLockCanonicalizesGeneratedRoutesAndGates(t *testing.T) {
	intent := testIntent()
	inputs := InputFingerprint{
		Files:  []HashPath{{Path: "internal/old.go", SHA256: digestText("old")}},
		Absent: []string{"internal/flow.go"},
	}
	evidence := testEvidence(intent, inputs)
	evidence.Routes[0], evidence.Routes[1] = evidence.Routes[1], evidence.Routes[0]
	evidence.Routes[0].Sites[0], evidence.Routes[0].Sites[1] = evidence.Routes[0].Sites[1], evidence.Routes[0].Sites[0]
	lastGate := len(evidence.Gates) - 1
	evidence.Gates[0], evidence.Gates[lastGate] = evidence.Gates[lastGate], evidence.Gates[0]
	lock, err := BuildLock(intent, testToolchain(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if routeKey(lock.Evidence.Routes[0].From, lock.Evidence.Routes[0].To) > routeKey(lock.Evidence.Routes[1].From, lock.Evidence.Routes[1].To) {
		t.Fatalf("routes were not canonicalized: %#v", lock.Evidence.Routes)
	}
	for index := 1; index < len(lock.Evidence.Gates); index++ {
		if lock.Evidence.Gates[index-1].Gate >= lock.Evidence.Gates[index].Gate {
			t.Fatalf("gates were not canonicalized: %#v", lock.Evidence.Gates)
		}
	}
	for index := 1; index < len(lock.Evidence.Routes[0].Sites); index++ {
		if referenceSiteRouteKey(lock.Evidence.Routes[0].Sites[index-1]) >= referenceSiteRouteKey(lock.Evidence.Routes[0].Sites[index]) {
			t.Fatalf("route sites were not canonicalized: %#v", lock.Evidence.Routes[0].Sites)
		}
	}
	evidence.Resolution.Objects[0].Definition.PackageIDs[0] = "mutated-after-lock"
	if lock.Evidence.Resolution.Objects[0].Definition.PackageIDs[0] == "mutated-after-lock" {
		t.Fatal("lock aliases generated package-variant evidence")
	}
}

func TestGenerateRequiresLockProviderRegistryEvidence(t *testing.T) {
	intent := Intent{Schema: Version, Name: "generate", Operations: []Operation{{
		ID: "generate", Authority: Authority{From: "source", To: "generated"},
		Edits: []Edit{{Kind: EditGenerate, Generate: &Generate{
			Provider: "fixture-generator", Inputs: []string{"internal/source.go"}, Destination: "internal/generated.go",
		}}},
		Footprint: Footprint{Read: []string{"internal/source.go"}, Write: []string{"internal/generated.go"}},
		Verify: Verification{
			Laws:  []Law{{ID: "generated", Package: "./internal", Test: "TestGenerated"}},
			Gates: []Gate{GateDiagnostics},
		},
	}}}
	inputs := InputFingerprint{Files: []HashPath{{Path: "internal/source.go", SHA256: digestText("source")}}, Absent: []string{"internal/generated.go"}}
	evidence := LockEvidence{
		Inputs:     inputs,
		Resolution: ResolutionEvidence{},
		Execution: ExecutionEvidence{
			Touched: []string{"internal/generated.go"}, Outputs: []HashPath{{Path: "internal/generated.go", SHA256: digestText("generated")}}, DiffSHA256: digestText("diff"),
		},
		Gates: []GateEvidence{{Gate: GateDiagnostics, ResultSHA256: digestText("diagnostics")}},
	}
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("unregistered provider accepted: %v", err)
	}
	evidence.Resolution.Providers = []ProviderEvidence{{Name: "fixture-generator", Identity: "fixture@1"}}
	if _, err := BuildLock(intent, testToolchain(), evidence); err != nil {
		t.Fatalf("registered provider rejected: %v", err)
	}
}

func TestLockRejectsSemanticSitesWithoutVariantIdentityOrRole(t *testing.T) {
	intent := testIntent()
	inputs := InputFingerprint{Files: []HashPath{{Path: "internal/old.go", SHA256: digestText("old")}}, Absent: []string{"internal/flow.go"}}
	for name, mutate := range map[string]func(*Position){
		"missing-package-variant": func(site *Position) { site.PackageIDs = nil },
		"missing-structural-role": func(site *Position) { site.Role = "" },
	} {
		t.Run(name, func(t *testing.T) {
			evidence := testEvidence(intent, inputs)
			mutate(&evidence.Resolution.Objects[0].Definition)
			if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "semantic site") {
				t.Fatalf("unbound semantic site accepted: %v", err)
			}
		})
	}
}

func TestLockRejectsReferenceRoutesThatChangeSiteRole(t *testing.T) {
	intent := testIntent()
	inputs := InputFingerprint{Files: []HashPath{{Path: "internal/old.go", SHA256: digestText("old")}}, Absent: []string{"internal/flow.go"}}
	evidence := testEvidence(intent, inputs)
	// Keep target membership valid while proving that a use cannot become a
	// selector merely because both resolve to the right reviewed object.
	for index := range evidence.Resolution.Objects {
		if evidence.Resolution.Objects[index].Role == ObjectTarget {
			evidence.Resolution.Objects[index].References[0].Role = SiteSelector
		}
	}
	for index := range evidence.Routes {
		for site := range evidence.Routes[index].Sites {
			if evidence.Routes[index].Sites[site].Source.Role == SiteUse {
				evidence.Routes[index].Sites[site].Target.Role = SiteSelector
			}
		}
	}
	if _, err := BuildLock(intent, testToolchain(), evidence); err == nil || !strings.Contains(err.Error(), "changes semantic site role") {
		t.Fatalf("role-changing reference route accepted: %v", err)
	}
}

func TestLockPermitsDifferentSourceAndTargetVariantSets(t *testing.T) {
	intent := testIntent()
	inputs := InputFingerprint{Files: []HashPath{{Path: "internal/old.go", SHA256: digestText("old")}}, Absent: []string{"internal/flow.go"}}
	evidence := testEvidence(intent, inputs)
	for index := range evidence.Resolution.Objects {
		if evidence.Resolution.Objects[index].Role != ObjectSource {
			continue
		}
		evidence.Resolution.Objects[index].Definition.PackageIDs = []string{"example.invalid/old", "example.invalid/old [example.invalid/old.test]"}
		for site := range evidence.Resolution.Objects[index].References {
			evidence.Resolution.Objects[index].References[site].PackageIDs = []string{"example.invalid/old", "example.invalid/old [example.invalid/old.test]"}
		}
	}
	for index := range evidence.Routes {
		for site := range evidence.Routes[index].Sites {
			evidence.Routes[index].Sites[site].Source.PackageIDs = []string{"example.invalid/old", "example.invalid/old [example.invalid/old.test]"}
		}
	}
	if _, err := BuildLock(intent, testToolchain(), evidence); err != nil {
		t.Fatalf("different source and target variant topology rejected: %v", err)
	}
}

func TestImportRefKeepsDeclaredNameSeparateFromAliasSpelling(t *testing.T) {
	intent := testIntent()
	route := &intent.Operations[0].Imports[0]
	route.From.Alias = ""
	route.To.Alias = ""
	if err := ValidateIntent(intent); err != nil {
		t.Fatalf("implicit source and target imports rejected: %v", err)
	}
	route.From.Alias = "renamed"
	route.To.Alias = "next"
	if err := ValidateIntent(intent); err != nil {
		t.Fatalf("explicit source and target imports rejected: %v", err)
	}
	route.From.Alias = "_"
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "source import") {
		t.Fatalf("blank source alias accepted: %v", err)
	}
	route.From.Alias = "."
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "source import") {
		t.Fatalf("dot source alias accepted: %v", err)
	}
	route.From.Alias = ""
	route.From.Name = "_"
	if err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "source import") {
		t.Fatalf("blank package clause accepted: %v", err)
	}
}

func TestSymbolRefGrammarDistinguishesPackageAndMemberObjects(t *testing.T) {
	validPackage := SymbolRef{Object: "example.invalid/pkg#package:Flow"}
	validField := SymbolRef{Object: "example.invalid/pkg#type:Link/field:Flow"}
	validMethod := SymbolRef{Object: "example.invalid/pkg#type:Link/method:Flow"}
	for _, value := range []SymbolRef{validPackage, validField, validMethod} {
		if !safeSymbolRef(value) {
			t.Fatalf("valid canonical object rejected: %s", value.Object)
		}
	}
	for _, value := range []SymbolRef{
		{Object: "example.invalid/pkg#Link.Flow"},
		{Object: "example.invalid/pkg#type:Link/member:Flow"},
		{Object: "example.invalid/pkg#package:Link.Flow"},
	} {
		if safeSymbolRef(value) {
			t.Fatalf("ambiguous object accepted: %s", value.Object)
		}
	}
}

func testIntent() Intent {
	oldFlow := object("old", "Flow")
	oldCount := object("old", "Flow.Count")
	newFlow := object("new", "Flow")
	newCount := object("new", "Flow.Count")
	return Intent{Schema: Version, Name: "flow-cut", Operations: []Operation{{
		ID: "flow", Authority: Authority{From: "old", To: "new"},
		Edits: []Edit{{Kind: EditRelocate, Relocate: &Relocate{
			Source: "internal/old.go", Destination: Destination{Path: "internal/flow.go", Package: "internal"}, Subjects: []Relocation{
				{From: oldFlow, To: newFlow}, {From: oldCount, To: newCount},
			},
		}}},
		Bindings: []Binding{{
			Consumer: "internal/old.go", From: oldCount, To: newCount, Form: BindingField,
			Receiver: []ReceiverPathStep{{Kind: ReceiverField, Object: newFlow}},
		}},
		Imports: []Import{{
			Consumer: "internal/old.go",
			From:     &ImportRef{Path: "example.invalid/old", Name: "old", Alias: "oldpkg"},
			To:       &ImportRef{Path: "example.invalid/new", Name: "new", Alias: "newpkg"},
			Symbols:  []SymbolRef{newFlow},
		}},
		Footprint: Footprint{Read: []string{"internal/old.go"}, Write: []string{"internal/old.go", "internal/flow.go"}},
		Verify: Verification{
			Laws:  []Law{{ID: "flow", Package: "./internal", Test: "TestFlow"}},
			Gates: []Gate{GateDiagnostics, GateImportDAG, GateResidue},
		},
	}}}
}

func object(pkg, name string) SymbolRef {
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		return SymbolRef{Object: "example.invalid/" + pkg + "#type:" + parts[0] + "/field:" + parts[1]}
	}
	return SymbolRef{Object: "example.invalid/" + pkg + "#package:" + name}
}

func testEvidence(intent Intent, inputs InputFingerprint) LockEvidence {
	outputs := ExecutionEvidence{
		Touched: []string{"internal/old.go", "internal/flow.go"},
		Outputs: []HashPath{
			{Path: "internal/old.go", SHA256: digestText("new-old")},
			{Path: "internal/flow.go", SHA256: digestText("new-flow")},
		},
		DiffSHA256: digestText("exact diff"),
	}
	requirements, err := ResolutionRequirements(intent)
	if err != nil {
		panic(err)
	}
	resolution := ResolutionEvidence{}
	for index, requirement := range requirements {
		path, pkg := "internal/old.go", "internal"
		if requirement.Path != "" {
			path = requirement.Path
		}
		if requirement.Package != "" {
			pkg = requirement.Package
		}
		resolution.Objects = append(resolution.Objects, ObjectEvidence{
			Object: requirement.Object, Role: requirement.Role, Package: pkg, Definition: testSite(path, index, 1, index+1, SiteDeclaration),
			References: []Position{testSite(path, index+100, 2, index+1, SiteUse)},
		})
	}
	byObject := map[string]ObjectEvidence{}
	for _, object := range resolution.Objects {
		byObject[object.Object.Object] = object
	}
	routes, err := ReferenceRouteRequirements(intent)
	if err != nil {
		panic(err)
	}
	for index := range routes {
		source, target := byObject[routes[index].From.Object], byObject[routes[index].To.Object]
		routes[index].Sites = []ReferenceSiteRoute{
			{Source: source.Definition, Target: target.Definition},
			{Source: source.References[0], Target: target.References[0]},
		}
	}
	gates, err := GateRequirements(intent)
	if err != nil {
		panic(err)
	}
	gateEvidence := make([]GateEvidence, 0, len(gates))
	for _, gate := range gates {
		gateEvidence = append(gateEvidence, GateEvidence{Gate: gate, ResultSHA256: digestText(string(gate))})
	}
	return LockEvidence{Inputs: inputs, Resolution: resolution, Routes: routes, Gates: gateEvidence, Execution: outputs}
}

func testToolchain() Toolchain {
	return Toolchain{
		HelperBuild: "v2", HelperSHA256: digestText("flashrefactor-v2"),
		GoVersion: "go1.23.3", GoExecutableSHA256: digestText("go"),
		Resolver: "x-tools-go-packages@v0.35.0", BuildEnvSHA256: digestText("build-env"), ModuleGraphSHA256: digestText("module-graph"),
	}
}
func digestText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func testSite(path string, offset, line, column int, role SiteRole) Position {
	return Position{PackageIDs: []string{"example.invalid/test [example.invalid/test.test]"}, Path: path, Offset: offset, Line: line, Column: column, Role: role}
}
func writeTestFile(t *testing.T, root, path, value string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
