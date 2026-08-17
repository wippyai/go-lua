package generator

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogRevisionAndFreshness(t *testing.T) {
	entries, err := parse(filepath.Join("..", "catalog.schema"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validate(entries); err != nil {
		t.Fatal(err)
	}
	relations, err := emitRelations(entries)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	relationPath := filepath.Join(directory, "relations.go")
	if err := os.WriteFile(relationPath, relations, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fresh(relationPath, relations); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(relationPath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fresh(relationPath, relations); err == nil {
		t.Fatal("stale relation output accepted")
	}
}

func TestCheckedInOutputsAreFresh(t *testing.T) {
	entries, err := parse(filepath.Join("..", "catalog.schema"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validate(entries); err != nil {
		t.Fatal(err)
	}
	historyPath := filepath.Join("..", "catalog.history")
	baseline, err := parseHistory(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	retired, err := parseRetired(filepath.Join("..", "catalog.retired"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRetirements(entries, baseline, retired); err != nil {
		t.Fatal(err)
	}
	if err := validateCompatibility(entries, baseline, retired); err != nil {
		t.Fatal(err)
	}
	if err := validateCurrentHistory(entries, baseline, retired); err != nil {
		t.Fatal(err)
	}
	history, err := emitHistory(baseline)
	if err != nil {
		t.Fatal(err)
	}
	checkedInHistory, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(checkedInHistory, history) {
		t.Fatalf("checked-in semantic revision history is stale: %s", historyPath)
	}
	relations, err := emitRelations(entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range []struct {
		path string
		want []byte
	}{
		{path: filepath.Join("..", "generated.go"), want: relations},
	} {
		got, err := os.ReadFile(output.path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, output.want) {
			t.Fatalf("checked-in generated output is stale: %s", output.path)
		}
	}
}

func TestCompatibilityBaselineRequiresRevisionBump(t *testing.T) {
	entries, err := parse(filepath.Join("..", "catalog.schema"))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := parseHistory(filepath.Join("..", "catalog.history"))
	if err != nil {
		t.Fatal(err)
	}
	retired := repositoryRetirements(t)
	entryIndex := -1
	for index := range entries {
		if entries[index].origin == "ProgramSourceControlFault" && entries[index].facet == "-" {
			entryIndex = index
			break
		}
	}
	if entryIndex < 0 {
		t.Fatal("missing compatibility fixture entry")
	}
	for _, test := range []struct {
		name   string
		mutate func(*entry)
	}{
		{name: "owner", mutate: func(item *entry) { item.owner = "ProgramStatic" }},
		{name: "form", mutate: func(item *entry) { item.form = "SealDerived" }},
		{name: "parents", mutate: func(item *entry) { item.parents = []ref{{origin: "ProgramSourceOrder", facet: "-"}} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := append([]entry(nil), entries...)
			test.mutate(&changed[entryIndex])
			if err := validateCompatibility(changed, baseline, retired); err == nil {
				t.Fatal("same-revision semantic mutation accepted")
			}
			updated := make(map[historyKey]historyEntry, len(baseline))
			for key, value := range baseline {
				updated[key] = value
			}
			if err := appendCurrentHistory(changed, updated, retired); err == nil {
				t.Fatal("history rewrite accepted same-revision semantic mutation")
			}
		})
	}
	changed := append([]entry(nil), entries...)
	changed[entryIndex].parents = []ref{{origin: "ProgramSourceOrder", facet: "-"}}
	changed[entryIndex].revision++
	if err := validateCompatibility(changed, baseline, retired); err == nil {
		t.Fatal("revision-bumped mutation without a new history row accepted")
	}
	if err := appendCurrentHistory(changed, baseline, retired); err != nil {
		t.Fatalf("append revision history: %v", err)
	}
	if err := validateCompatibility(changed, baseline, retired); err != nil {
		t.Fatalf("revision-bumped semantic mutation with a new history row rejected: %v", err)
	}
}

func TestHistoryReservesNumericIdentityAndParentRevisionClosure(t *testing.T) {
	entries, err := parse(filepath.Join("..", "catalog.schema"))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := parseHistory(filepath.Join("..", "catalog.history"))
	if err != nil {
		t.Fatal(err)
	}
	retired := repositoryRetirements(t)

	renamed := append([]entry(nil), entries...)
	for index := range renamed {
		if renamed[index].origin == "ProgramSourceControlFault" {
			renamed[index].origin = "RenamedProgramSourceControlFault"
		}
	}
	if err := validate(renamed); err != nil {
		t.Fatalf("renamed numeric fixture is invalid: %v", err)
	}
	if err := validateCompatibility(renamed, baseline, retired); err == nil {
		t.Fatal("numeric history identity accepted a renamed relation")
	}
	updated := make(map[historyKey]historyEntry, len(baseline))
	for key, value := range baseline {
		updated[key] = value
	}
	if err := appendCurrentHistory(renamed, updated, retired); err == nil {
		t.Fatal("numeric history identity accepted a renamed relation during update")
	}
	renamedRevision := append([]entry(nil), renamed...)
	for index := range renamedRevision {
		if renamedRevision[index].origin == "RenamedProgramSourceControlFault" {
			renamedRevision[index].revision++
		}
	}
	if err := validate(renamedRevision); err != nil {
		t.Fatalf("revision-bumped renamed numeric fixture is invalid: %v", err)
	}
	if err := appendCurrentHistory(renamedRevision, updated, retired); err == nil {
		t.Fatal("numeric history identity accepted a renamed relation after a revision bump")
	}
	relocated := append([]entry(nil), entries...)
	for index := range relocated {
		if relocated[index].origin == "ProgramSourceControlFault" {
			relocated[index].originValue = 0x00010006
		}
	}
	if err := validate(relocated); err != nil {
		t.Fatalf("relocated historical-name fixture is invalid: %v", err)
	}
	relocatedHistory := make(map[historyKey]historyEntry, len(baseline))
	for key, value := range baseline {
		relocatedHistory[key] = value
	}
	if err := appendCurrentHistory(relocated, relocatedHistory, retired); err == nil {
		t.Fatal("history accepted a removed relation name reused at a new numeric slot")
	}

	changed := append([]entry(nil), entries...)
	for index := range changed {
		if changed[index].origin == "ProgramFlowLiterals" {
			changed[index].revision++
		}
	}
	if err := validate(changed); err != nil {
		t.Fatalf("parent-revision fixture is invalid: %v", err)
	}
	closure := make(map[historyKey]historyEntry, len(baseline)+1)
	for key, value := range baseline {
		closure[key] = value
	}
	definitions := entryIndex(changed)
	for _, item := range changed {
		if item.origin != "ProgramFlowLiterals" {
			continue
		}
		digest, err := semanticDigest(item, definitions)
		if err != nil {
			t.Fatal(err)
		}
		closure[historyKeyFor(item)] = historyEntry{originName: item.origin, facetName: item.facet, digest: digest}
	}
	if err := validateCompatibility(changed, closure, retired); err == nil {
		t.Fatal("parent revision changed without requiring a child revision bump")
	}
}

func TestHistoryRejectsDisappearanceAndWholeCatalogRollback(t *testing.T) {
	entries, err := parse(filepath.Join("..", "catalog.schema"))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := parseHistory(filepath.Join("..", "catalog.history"))
	if err != nil {
		t.Fatal(err)
	}
	retired := repositoryRetirements(t)
	removed := make([]entry, 0, len(entries)-1)
	for _, item := range entries {
		if item.origin == "ProgramSourceControlFault" && item.facet == "-" {
			continue
		}
		removed = append(removed, item)
	}
	if len(removed) != len(entries)-1 {
		t.Fatal("remove-current-row fixture did not remove exactly one relation")
	}
	if err := validate(removed); err != nil {
		t.Fatalf("remove-current-row fixture is invalid: %v", err)
	}
	if err := validateCompatibility(removed, baseline, retired); err == nil {
		t.Fatal("history accepted a removed current relation")
	}
	updated := make(map[historyKey]historyEntry, len(baseline))
	for key, value := range baseline {
		updated[key] = value
	}
	if err := appendCurrentHistory(removed, updated, retired); err == nil {
		t.Fatal("history update accepted a removed current relation")
	}

	rollback := []entry{{origin: "One", originValue: 1, facet: "-", owner: "ProgramSource", form: "Authored", revision: 1}}
	if err := validate(rollback); err != nil {
		t.Fatalf("rollback fixture is invalid: %v", err)
	}
	digest, err := semanticDigest(rollback[0], entryIndex(rollback))
	if err != nil {
		t.Fatal(err)
	}
	rolledHistory := map[historyKey]historyEntry{
		historyKey{origin: 1, facet: 0, revision: 1}: historyEntry{originName: "One", facetName: "-", digest: digest},
		historyKey{origin: 1, facet: 0, revision: 2}: historyEntry{originName: "One", facetName: "-", digest: digest},
	}
	if err := validateCompatibility(rollback, rolledHistory, nil); err == nil {
		t.Fatal("history accepted a whole-catalog rollback to an older matching digest")
	}
	if err := appendCurrentHistory(rollback, rolledHistory, nil); err == nil {
		t.Fatal("history update accepted a whole-catalog rollback to an older matching digest")
	}
}

func TestRetirementIsAnIrreversibleExactHistoryCut(t *testing.T) {
	entries, err := parse(filepath.Join("..", "catalog.schema"))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := parseHistory(filepath.Join("..", "catalog.history"))
	if err != nil {
		t.Fatal(err)
	}
	retired := mergeRetirements(t, repositoryRetirements(t), retirementFor(t, baseline, "ProgramSourceControlFault", "-"))
	removed := withoutEntry(t, entries, "ProgramSourceControlFault", "-")
	if err := validate(removed); err != nil {
		t.Fatalf("legal retirement schema invalid: %v", err)
	}
	if err := validateCompatibility(removed, baseline, retired); err != nil {
		t.Fatalf("legal retirement rejected: %v", err)
	}
	if err := validateCurrentHistory(removed, baseline, retired); err != nil {
		t.Fatalf("retired current history rejected: %v", err)
	}
	relations, err := emitRelations(removed)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(relations, []byte("ProgramSourceControlFault")) {
		t.Fatal("generated active catalog retained a retired relation")
	}

	// The old numeric identity cannot be activated again, even under a new name.
	numericRevival := append(append([]entry(nil), removed...), entry{
		origin: "Replacement", originValue: 0x00010005, facet: "-", owner: "ProgramSource", form: "Authored", revision: 1,
	})
	if err := validate(numericRevival); err != nil {
		t.Fatalf("numeric-revival fixture invalid: %v", err)
	}
	if err := validateCompatibility(numericRevival, baseline, retired); err == nil {
		t.Fatal("retired numeric identity revived")
	}

	// The old generated name cannot migrate to a new numeric identity either.
	nameRevival := append(append([]entry(nil), removed...), entry{
		origin: "ProgramSourceControlFault", originValue: 0x00010006, facet: "-", owner: "ProgramSource", form: "Authored", revision: 1,
	})
	if err := validate(nameRevival); err != nil {
		t.Fatalf("name-revival fixture invalid: %v", err)
	}
	if err := validateCompatibility(nameRevival, baseline, retired); err == nil {
		t.Fatal("retired generated name revived")
	}

	// Active and retired identity is never a tolerated overlap.
	if err := validateCompatibility(entries, baseline, retired); err == nil {
		t.Fatal("active/retired collision accepted")
	}
}

func TestRetirementRequiresExactFinalHistoryEvidence(t *testing.T) {
	entries, err := parse(filepath.Join("..", "catalog.schema"))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := parseHistory(filepath.Join("..", "catalog.history"))
	if err != nil {
		t.Fatal(err)
	}
	removed := withoutEntry(t, entries, "ProgramSourceControlFault", "-")
	fixture := retirementFor(t, baseline, "ProgramSourceControlFault", "-")
	for token, row := range fixture {
		row.entry.digest = strings.Repeat("0", sha256.Size*2)
		fixture[token] = row
	}
	retired := mergeRetirements(t, repositoryRetirements(t), fixture)
	if err := validateCompatibility(removed, baseline, retired); err == nil {
		t.Fatal("tampered retirement evidence accepted")
	}
	if err := validateCompatibility(removed, baseline, repositoryRetirements(t)); err == nil {
		t.Fatal("historical disappearance without retirement accepted")
	}
}

func repositoryRetirements(t *testing.T) map[historyTokenKey]retirement {
	t.Helper()
	retired, err := parseRetired(filepath.Join("..", "catalog.retired"))
	if err != nil {
		t.Fatal(err)
	}
	return retired
}

func mergeRetirements(t *testing.T, sets ...map[historyTokenKey]retirement) map[historyTokenKey]retirement {
	t.Helper()
	merged := make(map[historyTokenKey]retirement)
	for _, set := range sets {
		for token, row := range set {
			if _, duplicate := merged[token]; duplicate {
				t.Fatalf("duplicate retirement fixture origin=0x%08X facet=%d", token.origin, token.facet)
			}
			merged[token] = row
		}
	}
	return merged
}

func retirementFor(t *testing.T, baseline map[historyKey]historyEntry, origin, facet string) map[historyTokenKey]retirement {
	t.Helper()
	var selected historyKey
	var identity historyEntry
	found := false
	for key, row := range baseline {
		if row.originName != origin || row.facetName != facet || found && key.revision <= selected.revision {
			continue
		}
		selected, identity, found = key, row, true
	}
	if !found {
		t.Fatalf("history has no retirement fixture %s@%s", origin, facet)
	}
	return map[historyTokenKey]retirement{{origin: selected.origin, facet: selected.facet}: {key: selected, entry: identity}}
}

func withoutEntry(t *testing.T, entries []entry, origin, facet string) []entry {
	t.Helper()
	result := make([]entry, 0, len(entries)-1)
	for _, item := range entries {
		if item.origin == origin && item.facet == facet {
			continue
		}
		result = append(result, item)
	}
	if len(result) != len(entries)-1 {
		t.Fatalf("remove fixture %s@%s did not remove exactly one relation", origin, facet)
	}
	return result
}

func TestValidateRejectsNumericIdentityCollisions(t *testing.T) {
	base := entry{origin: "One", originValue: 1, facet: "-", owner: "ProgramSource", form: "Authored", revision: 1}
	if err := validate([]entry{base, {origin: "Two", originValue: 1, facet: "-", owner: "ProgramSource", form: "Authored", revision: 1}}); err == nil {
		t.Fatal("numeric origin collision accepted")
	}
	if err := validate([]entry{base, {origin: "Two", originValue: 1, facet: "-", owner: "ProgramSource", form: "Authored", revision: 2}}); err == nil {
		t.Fatal("cross-revision numeric origin collision accepted")
	}
	primary := entry{origin: "One", originValue: 1, facet: "-", owner: "ProgramSource", form: "Authored", revision: 1}
	first := entry{origin: "One", originValue: 1, facet: "FacetOne", facetValue: 1, owner: "ProgramSource", form: "Authored", revision: 1}
	second := entry{origin: "One", originValue: 1, facet: "FacetTwo", facetValue: 1, owner: "ProgramSource", form: "Authored", revision: 1}
	if err := validate([]entry{primary, first, second}); err == nil {
		t.Fatal("numeric token collision accepted")
	}
	if err := validate([]entry{
		{origin: "One", originValue: 1, facet: "-", owner: "ProgramSource", form: "Authored", revision: 1},
		{origin: "One", originValue: 1, facet: "Facet", facetValue: 1, owner: "ProgramSource", form: "Authored", revision: 2},
	}); err == nil {
		t.Fatal("per-origin revision conflict accepted")
	}
}

func TestRunCheckRejectsStaleCatalogFiles(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, files paths)
	}{
		{
			name: "schema",
			mutate: func(t *testing.T, files paths) {
				content, err := os.ReadFile(files.schema)
				if err != nil {
					t.Fatal(err)
				}
				changed := strings.Replace(string(content), "ProgramSourceControlFault 0x00010005 - 0 ProgramSource Authored", "ProgramSourceControlFault 0x00010005 - 0 ProgramSource SealDerived", 1)
				if changed == string(content) {
					t.Fatal("schema stale fixture did not change")
				}
				if err := os.WriteFile(files.schema, []byte(changed), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "history",
			mutate: func(t *testing.T, files paths) {
				content, err := os.ReadFile(files.history)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(files.history, append(content, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "retired",
			mutate: func(t *testing.T, files paths) {
				if err := os.WriteFile(files.retired, []byte("ProgramSourceControlFault 0x00010005 - 0 1 "+strings.Repeat("0", sha256.Size*2)+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "relation source",
			mutate: func(t *testing.T, files paths) {
				if err := os.WriteFile(files.relations, []byte("stale"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := checkFixture(t)
			test.mutate(t, files)
			if err := Run(files.schema, files.history, files.retired, files.relations, true, false); err == nil {
				t.Fatal("Run(check) accepted a stale catalog file")
			}
		})
	}
}

func TestRunCheckAcceptsOnlyTheCurrentCatalog(t *testing.T) {
	files := checkFixture(t)
	if err := Run(files.schema, files.history, files.retired, files.relations, true, false); err != nil {
		t.Fatalf("Run(check) = %v", err)
	}
	if err := os.WriteFile(files.relations, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(files.schema, files.history, files.retired, files.relations, true, false); err == nil {
		t.Fatal("Run(check) accepted stale relation output")
	}
}

func checkFixture(t *testing.T) paths {
	t.Helper()
	root := t.TempDir()
	files := paths{
		schema:    filepath.Join(root, "catalog.schema"),
		history:   filepath.Join(root, "catalog.history"),
		retired:   filepath.Join(root, "catalog.retired"),
		relations: filepath.Join(root, "generated.go"),
	}
	for destination, source := range map[string]string{
		files.schema:    filepath.Join("..", "catalog.schema"),
		files.history:   filepath.Join("..", "catalog.history"),
		files.retired:   filepath.Join("..", "catalog.retired"),
		files.relations: filepath.Join("..", "generated.go"),
	} {
		content, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return files
}
