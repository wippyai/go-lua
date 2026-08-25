package relcut

import (
	"strings"
	"testing"
)

// The manifest a lane executes from is executable: it decodes, it is internally
// consistent, and every path it expects to delete or restate is in the tree.
func TestManifestIsExecutable(t *testing.T) {
	manifest, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	root, err := RepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	findings := Validate(manifest, root)
	for _, finding := range findings {
		if finding.Severity == SeverityRefused {
			t.Errorf("%s", finding)
			continue
		}
		t.Logf("%s", finding)
	}
	if Refused(findings) {
		t.Fatal("the deletion manifest is not executable from this tree")
	}
}

// Every clause of the design's delete-set has an entry. The manifest is the
// cut's whole statement, so a clause with no entry is a clause nobody owns.
func TestEveryDeleteClauseIsOwned(t *testing.T) {
	manifest, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	clauses := map[string]string{
		"analysis/engine/execution":                   "S2-engine-execution",
		"analysis/engine/internal/executioncatalog":   "S2-executioncatalog",
		"generated family executors":                  "S3-generated-family-workers",
		"installer choreography":                      "S3-installer-choreography",
		"HotRule, HotOwner and BindHot":               "S4-owner-hot-owners",
		"registries derivable from declarations":      "S6-derivable-registries",
		"emitter/render/installer machinery":          "S5-emit-machinery",
		"runtime validation and reconstruction":       "S6-engine-runtime",
		"access paths outside the canonical snapshot": "S8-read-paths",
	}
	present := map[string]Entry{}
	for _, entry := range manifest.Entries {
		present[entry.ID] = entry
	}
	for clause, owner := range clauses {
		if _, held := present[owner]; !held {
			t.Errorf("§12 clause %q has no entry (expected %s)", clause, owner)
		}
	}
}

// A deletion states what happens to the laws it removes. An entry that deletes
// a law surface and names no restatement is how a proof quietly disappears.
func TestDeletionsAccountForTheirLaws(t *testing.T) {
	manifest, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range manifest.Entries {
		if len(entry.LawsDie) == 0 {
			continue
		}
		if len(entry.LawsRestated) == 0 {
			t.Errorf("%s retires %d law(s) and names no restatement", entry.ID, len(entry.LawsDie))
		}
	}
}

// Every retained kernel candidate carries a discharge condition and a stated
// consequence of failing it. A keep with no failure branch is a promise, and
// §12 refuses promises about low-level kernels.
func TestRetainedKernelsCarryADischargeCondition(t *testing.T) {
	manifest, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	kept := 0
	for _, entry := range manifest.Entries {
		if entry.Disposition != DispositionKeepIfGeneric {
			continue
		}
		kept++
		obligation := strings.ToLower(entry.ProofObligation)
		if !strings.Contains(obligation, "delete") && !strings.Contains(obligation, "must not") {
			t.Errorf("%s states no consequence for an undischarged obligation: %q", entry.ID, entry.ProofObligation)
		}
	}
	if kept == 0 {
		t.Fatal("the manifest names no retained kernel candidate")
	}
}

// Reading order is dependency order: an entry never appears before something
// it is blocked by.
func TestReadingOrderRespectsDependencies(t *testing.T) {
	manifest, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	position := map[string]int{}
	for index, entry := range manifest.Order() {
		position[entry.ID] = index
	}
	for _, entry := range manifest.Entries {
		for _, blocker := range entry.BlockedBy {
			if position[blocker] > position[entry.ID] {
				t.Errorf("%s reads before its blocker %s", entry.ID, blocker)
			}
		}
	}
}

// A malformed manifest is refused rather than half-executed.
func TestValidateRefusesMalformedManifests(t *testing.T) {
	cases := map[string]Manifest{
		"duplicate identity": {Entries: []Entry{
			{ID: "a", Paths: []string{"go.mod"}, Disposition: DispositionDelete, Authority: "x"},
			{ID: "a", Paths: []string{"README.md"}, Disposition: DispositionDelete, Authority: "x"},
		}},
		"shared path": {Entries: []Entry{
			{ID: "a", Paths: []string{"go.mod"}, Disposition: DispositionDelete, Authority: "x"},
			{ID: "b", Paths: []string{"go.mod"}, Disposition: DispositionDelete, Authority: "x"},
		}},
		"keep without obligation": {Entries: []Entry{
			{ID: "a", Paths: []string{"go.mod"}, Disposition: DispositionKeepIfGeneric, Authority: "x"},
		}},
		"unknown disposition": {Entries: []Entry{
			{ID: "a", Paths: []string{"go.mod"}, Disposition: Disposition("remove"), Authority: "x"},
		}},
		"unknown blocker": {Entries: []Entry{
			{ID: "a", Paths: []string{"go.mod"}, Disposition: DispositionDelete, Authority: "x", BlockedBy: []string{"z"}},
		}},
		"dependency cycle": {Entries: []Entry{
			{ID: "a", Paths: []string{"go.mod"}, Disposition: DispositionDelete, Authority: "x", BlockedBy: []string{"b"}},
			{ID: "b", Paths: []string{"README.md"}, Disposition: DispositionDelete, Authority: "x", BlockedBy: []string{"a"}},
		}},
		"undeclared layer": {Entries: []Entry{
			{ID: "a", Paths: []string{"go.mod"}, Disposition: DispositionDelete, Authority: "x", ResidueLayer: "L9"},
		}},
		"no authority": {Entries: []Entry{
			{ID: "a", Paths: []string{"go.mod"}, Disposition: DispositionDelete},
		}},
	}
	for name, manifest := range cases {
		if !Refused(Validate(manifest, "")) {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A stale path is a refusal, not a warning: a manifest that names a file the
// tree no longer holds cannot be executed.
func TestStalePathIsRefused(t *testing.T) {
	root, err := RepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Entries: []Entry{{
		ID: "a", Paths: []string{"analysis/engine/does-not-exist.go"},
		Disposition: DispositionDelete, Authority: "x", ExpectPresent: true,
	}}}
	if !Refused(Validate(manifest, root)) {
		t.Fatal("a stale path was accepted")
	}
}
