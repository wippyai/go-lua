package oracle

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/result"
)

// TestCorpusSemanticNativeUnimplementedFamilyIsFencedLaw is the law behind the
// native half of the unsupported ledger. A native selector names a fact family,
// and a family the analyzer does not issue rows under answers no selector at
// all - so an upper bound on that family is satisfied by the absence of the
// producer rather than by the analyzed program. That is a vacuous pass, and it
// reads exactly like a proved bound.
//
// The law states the distinction the harness must draw: a bound on an
// unimplemented family is fenced by name before any comparison, and a bound on
// an implemented family is not fenced at all, whatever it counts.
func TestCorpusSemanticNativeUnimplementedFamilyIsFencedLaw(t *testing.T) {
	zero := 0
	one := 1

	declared := result.NativeFamiliesDeclaredNotImplemented()
	if len(declared) == 0 {
		t.Fatal("declared-not-implemented native family register is empty")
	}
	dead := declared[0]

	// A max:0 bound on a declared family is the vacuous pass in its purest
	// form: nothing can ever match it. It must be fenced, and the fence must
	// name both the family and the surface that owes it.
	vacuous := &corpusDiagnosticProjectExpectations{manifest: &corpusDiagnosticManifest{
		Check: &corpusDiagnosticManifestCheck{Native: &corpusNativeContract{
			Facts: []corpusNativeFactContract{{Name: "dead family upper bound", Family: dead.Family, Max: &zero}},
		}},
	}}
	rows := corpusSemanticFixtureInputUnsupported(vacuous)
	if len(rows) != 1 || !strings.Contains(rows[0], strconv.Quote(dead.Family)) || !strings.Contains(rows[0], dead.Owner) {
		t.Fatalf("max:0 on the unimplemented family %q fenced as %v, want exactly one row naming the family and owner %q", dead.Family, rows, dead.Owner)
	}

	// The same fence applies to an invalidation selector: it reads the same
	// family column and would otherwise pass vacuously the same way.
	invalidated := &corpusDiagnosticProjectExpectations{manifest: &corpusDiagnosticManifest{
		Check: &corpusDiagnosticManifestCheck{Native: &corpusNativeContract{
			Invalidation: []corpusNativeInvalidationContract{{Name: "dead family invalidation", Family: dead.Family, Max: &zero}},
		}},
	}}
	if rows := corpusSemanticFixtureInputUnsupported(invalidated); len(rows) != 1 || !strings.Contains(rows[0], strconv.Quote(dead.Family)) {
		t.Fatalf("invalidation on the unimplemented family %q fenced as %v, want exactly one row naming it", dead.Family, rows)
	}

	// A family in neither half is a name nothing in the analyzer answers. It is
	// fenced too, and it is rendered as unregistered rather than as owed.
	unknown := corpusUnregisteredNativeFamily(t)
	rows = corpusSemanticFixtureInputUnsupported(&corpusDiagnosticProjectExpectations{manifest: &corpusDiagnosticManifest{
		Check: &corpusDiagnosticManifestCheck{Native: &corpusNativeContract{
			Facts: []corpusNativeFactContract{{Family: unknown, Max: &zero}},
		}},
	}})
	if len(rows) != 1 || !strings.Contains(rows[0], strconv.Quote(unknown)) || !strings.Contains(rows[0], corpusNativeFamilyUnregisteredOwner) {
		t.Fatalf("unregistered family %q fenced as %v, want exactly one row naming it as %s", unknown, rows, corpusNativeFamilyUnregisteredOwner)
	}

	// An implemented family is never fenced, whatever bound it carries. A zero
	// upper bound on an implemented family is an ordinary assertion the solve
	// decides, and a fence here would turn a judgeable contract into an
	// unsupported one.
	implemented := result.NativeFamiliesImplemented()
	if len(implemented) == 0 {
		t.Fatal("implemented native family vocabulary is empty")
	}
	live := &corpusDiagnosticProjectExpectations{manifest: &corpusDiagnosticManifest{
		Check: &corpusDiagnosticManifestCheck{Native: &corpusNativeContract{
			Facts: []corpusNativeFactContract{
				{Name: "live family upper bound", Family: implemented[0], Max: &zero},
				{Name: "live family lower bound", Family: implemented[0], Trust: "proven", Min: &one},
			},
			Invalidation: []corpusNativeInvalidationContract{{Name: "live family invalidation", Family: implemented[0], Max: &zero}},
		}},
	}}
	if rows := corpusSemanticFixtureInputUnsupported(live); len(rows) != 0 {
		t.Fatalf("implemented family %q fenced as unsupported: %v", implemented[0], rows)
	}

	// A selector with no family column names no family. It selects by other
	// columns and is judged by the solve, so it is not fenced either.
	familyless := &corpusDiagnosticProjectExpectations{manifest: &corpusDiagnosticManifest{
		Check: &corpusDiagnosticManifestCheck{Native: &corpusNativeContract{
			Facts: []corpusNativeFactContract{{Name: "no family column", Subject: "x", Max: &zero}},
		}},
	}}
	if rows := corpusSemanticFixtureInputUnsupported(familyless); len(rows) != 0 {
		t.Fatalf("family-less native selector fenced as unsupported: %v", rows)
	}
}

// TestCorpusSemanticNativeImplementedFamilyAnsweringZeroRowsPassesLaw is the
// other side of the same distinction, held against a real solve. A published
// family that answers zero rows for a selector is a proved absence: the issuer
// ran, published its rows, and none of them carries the selected member. That
// must remain a pass, or the fence above would have bought its precision by
// making every negative assertion unusable.
func TestCorpusSemanticNativeImplementedFamilyAnsweringZeroRowsPassesLaw(t *testing.T) {
	zero := 0
	run := nativePublicationCorpusResult(t, "native/const-folded-through-local")
	if run.result == nil || !run.result.NativePublicationAvailable() {
		t.Fatal("solved fixture exposes no native publication")
	}
	published := false
	for index := 0; index < run.result.NativePublicationCount(); index++ {
		if row, ok := run.result.NativePublicationAt(index); ok && row.Family() == "representation" {
			published = true
			break
		}
	}
	if !published {
		t.Fatal("fixture publishes no representation row; the zero-answer assertion below would be vacuous")
	}
	expectation := &corpusDiagnosticProjectExpectations{manifest: &corpusDiagnosticManifest{
		Check: &corpusDiagnosticManifestCheck{Native: &corpusNativeContract{
			Facts: []corpusNativeFactContract{{Name: "no float carrier", Family: "representation", Representation: "float", Max: &zero}},
		}},
	}}
	if rows := corpusSemanticFixtureInputUnsupported(expectation); len(rows) != 0 {
		t.Fatalf("implemented family with a zero upper bound fenced as unsupported: %v", rows)
	}
	if mismatches := corpusSemanticNativeMismatches(run.compilation, expectation, run.result); len(mismatches) != 0 {
		t.Fatalf("implemented family answering zero rows reported %v, want a clean upper bound", mismatches)
	}
}

// TestCorpusFixtureNativeFamiliesAreImplementedOrDeclared is the registry law
// applied to the corpus. Every native fact family any fixture names is either
// implemented by the closed publication enum or carried by the
// declared-not-implemented register with an owner. A family in neither is a
// name nothing in the analyzer answers, which is the silence this law exists to
// make impossible: a fixture may declare a fact the analyzer does not yet
// publish, but never one nobody owes.
//
// The register is not an allowlist for the acceptance gate. An entry here still
// fences its fixture and still fails the unimplemented sub-test; what it adds
// is the owner, so the failing list is addressed to someone.
func TestCorpusFixtureNativeFamiliesAreImplementedOrDeclared(t *testing.T) {
	projects := corpusHarnessProjects(t)
	referenced := make(map[string]int)
	for _, project := range projects {
		for _, family := range corpusNativeContractFamilies(project.expectation) {
			referenced[family]++
		}
	}
	names := make([]string, 0, len(referenced))
	for family := range referenced {
		names = append(names, family)
	}
	sort.Strings(names)
	implemented, declared := 0, 0
	for _, family := range names {
		status, row := result.NativeFamilyAnswer(family)
		switch status {
		case result.NativeFamilyStatusImplemented:
			implemented++
		case result.NativeFamilyStatusDeclared:
			declared++
			if row.Owner == "" || row.Reason == "" {
				t.Errorf("fixture native family %q is registered without an owner or reason", family)
			}
		default:
			t.Errorf("fixture native family %q is neither implemented by the closed publication enum nor carried by the declared-not-implemented register. Issue rows for it, or register it with the surface that owes the fact.", family)
		}
	}
	t.Logf("fixture native families: referenced=%d implemented=%d declared-not-implemented=%d", len(names), implemented, declared)
}

// corpusUnregisteredNativeFamily is a family name the register provably does
// not hold, derived from the register rather than spelled here so that adding a
// row cannot silently turn the probe into a test of nothing.
func corpusUnregisteredNativeFamily(t *testing.T) string {
	t.Helper()
	for ordinal := 0; ordinal <= len(result.NativeFamiliesDeclaredNotImplemented()); ordinal++ {
		candidate := "no-such-native-family-" + strconv.Itoa(ordinal)
		if status, _ := result.NativeFamilyAnswer(candidate); status == result.NativeFamilyUnknown {
			return candidate
		}
	}
	t.Fatal("every derived probe name is a registered native family")
	return ""
}

// corpusNativeFamilyFixture is one fixture's unimplemented native contract: the
// families it names that no issuer publishes rows under. It is carried per
// fixture because the fixture is the unit of repair - a family is implemented
// once and every fixture naming it becomes judgeable together.
type corpusNativeFamilyFixture struct {
	project  string
	families []string
}

// corpusNativeFamilyLedger is the corpus-wide native family answer. It is
// derived from the fixture manifests and the closed publication vocabulary
// alone, so it is exact whether or not a fixture reached its solve.
type corpusNativeFamilyLedger struct {
	fixtures []corpusNativeFamilyFixture
	// families counts how many fixtures name each unimplemented family, and
	// owners carries the register's answer for it.
	families map[string]int
	owners   map[string]string
}

func (ledger corpusNativeFamilyLedger) summary() string {
	return fmt.Sprintf("acceptance unimplemented native families: fixtures=%d families=%d", len(ledger.fixtures), len(ledger.families))
}

// corpusNativeFamilyLedgerOf reads every fixture's native contract without
// running it. A family is answered by the vocabulary and the register together,
// which are both decidable from the manifest, so the whole list is knowable
// here and cannot be shortened by a walk that stopped early.
func corpusNativeFamilyLedgerOf(projects []corpusHarnessProject) corpusNativeFamilyLedger {
	ledger := corpusNativeFamilyLedger{families: make(map[string]int), owners: make(map[string]string)}
	for _, project := range projects {
		named := make([]string, 0, 4)
		for _, family := range corpusNativeContractFamilies(project.expectation) {
			status, declared := result.NativeFamilyAnswer(family)
			if status == result.NativeFamilyStatusImplemented {
				continue
			}
			named = append(named, family)
			ledger.families[family]++
			if status == result.NativeFamilyStatusDeclared {
				ledger.owners[family] = declared.Owner
				continue
			}
			ledger.owners[family] = corpusNativeFamilyUnregisteredOwner
		}
		if len(named) == 0 {
			continue
		}
		sort.Strings(named)
		ledger.fixtures = append(ledger.fixtures, corpusNativeFamilyFixture{project: project.name, families: named})
	}
	return ledger
}

// report is the failing sub-test. It names every fixture whose native contract
// selects a family no issuer publishes, then renders the families themselves
// with the surface each one is owed by. There is deliberately no allowlist: the
// printed list is the native-plane work queue, and it shortens only when an
// issuer publishes the family.
func (ledger corpusNativeFamilyLedger) report(t *testing.T) {
	t.Helper()
	if len(ledger.fixtures) == 0 {
		return
	}
	for _, fixture := range ledger.fixtures {
		t.Errorf("fixture %s selects unimplemented native fact families: %s", fixture.project, strings.Join(fixture.families, " "))
	}
	families := make([]string, 0, len(ledger.families))
	for family := range ledger.families {
		families = append(families, family)
	}
	sort.Strings(families)
	var summary strings.Builder
	fmt.Fprintf(&summary, "%s\nunimplemented native families by owner:", ledger.summary())
	for _, family := range families {
		fmt.Fprintf(&summary, "\n  %-28s fixtures=%-3d owner=%s", family, ledger.families[family], ledger.owners[family])
	}
	t.Error(summary.String())
}

// TestCorpusNativeFamilyLedgerNamesEveryFencedFixture is the harness law behind
// the failing sub-test. It proves the three properties the work queue rests on:
// a fixture naming an unimplemented family is listed with that family named, a
// fixture whose native contract the analyzer can judge is absent, and every
// listed family carries an owner. Without this a quiet ledger would read the
// same as an implemented native plane, which is the exact silence the sub-test
// exists to end.
func TestCorpusNativeFamilyLedgerNamesEveryFencedFixture(t *testing.T) {
	projects := corpusHarnessProjects(t)
	ledger := corpusNativeFamilyLedgerOf(projects)
	listed := make(map[string]corpusNativeFamilyFixture, len(ledger.fixtures))
	for _, fixture := range ledger.fixtures {
		listed[fixture.project] = fixture
	}
	for _, project := range projects {
		unimplemented := make([]string, 0, 4)
		for _, family := range corpusNativeContractFamilies(project.expectation) {
			if status, _ := result.NativeFamilyAnswer(family); status != result.NativeFamilyStatusImplemented {
				unimplemented = append(unimplemented, family)
			}
		}
		fixture, isListed := listed[project.name]
		if len(unimplemented) == 0 {
			if isListed {
				t.Errorf("fixture %s is listed and names no unimplemented native family", project.name)
			}
			// A fixture the ledger passes over must also be one the fence
			// passes over, or a fenced fixture would fail with no entry in the
			// queue that explains it.
			for _, row := range corpusSemanticFixtureInputUnsupported(project.expectation) {
				if strings.Contains(row, "native fact family") {
					t.Errorf("fixture %s is fenced on a native family and absent from the ledger: %s", project.name, row)
				}
			}
			continue
		}
		if !isListed {
			t.Errorf("fixture %s names unimplemented native families and is absent from the ledger: %v", project.name, unimplemented)
			continue
		}
		if len(fixture.families) != len(unimplemented) {
			t.Errorf("fixture %s lists %d unimplemented families, its contract names %d", project.name, len(fixture.families), len(unimplemented))
		}
		for _, family := range fixture.families {
			if result.NativeFamilyImplemented(family) {
				t.Errorf("fixture %s lists the implemented family %q as unimplemented", project.name, family)
			}
		}
	}
	for family, owner := range ledger.owners {
		if owner == "" {
			t.Errorf("unimplemented native family %q is rendered with no owner", family)
		}
	}
}

// TestCorpusAcceptanceFencesTheVacuousNativeFamilyPassLaw is the end-to-end
// statement of the same distinction, taken through the acceptance mode itself
// on the fixture that showed it: a single upper bound of zero rows on a family
// no issuer publishes. Judged as a count it is satisfied by the analyzer having
// nothing to say, which is the silence the fence removes. The fixture must now
// fail before compile, naming the family it selects.
func TestCorpusAcceptanceFencesTheVacuousNativeFamilyPassLaw(t *testing.T) {
	const fixture = "native/hostglobal-untyped-withheld"
	const family = "host_global_binding"
	if result.NativeFamilyImplemented(family) {
		t.Fatalf("family %q is implemented; this probe no longer covers the vacuous pass", family)
	}
	project := corpusHarnessFixture(t, fixture)
	families := corpusNativeContractFamilies(project.expectation)
	if len(families) != 1 || families[0] != family {
		t.Fatalf("fixture %s names families %v, want exactly [%s]", fixture, families, family)
	}
	_, class, err := corpusHarnessExecuteDetached(t, project, corpusSemanticAcceptanceMode())
	if err == nil {
		t.Fatalf("fixture %s passed acceptance while selecting the unimplemented family %q", fixture, family)
	}
	if class != "fixture-contract" || !strings.Contains(err.Error(), strconv.Quote(family)) {
		t.Fatalf("fixture %s failed as class=%q err=%v, want a fixture-contract fence naming %q", fixture, class, err, family)
	}
}
