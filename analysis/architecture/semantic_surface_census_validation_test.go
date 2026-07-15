package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSemanticSurfaceCensusRejectsShortRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "census.csv")
	contents := strings.Join(censusHeader, ",") + "\nanalysis/check/body,api.go,Result,type\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readSemanticSurfaceCensus(path)
	if err == nil || !strings.Contains(err.Error(), "row 2 has 4 columns, want 10") {
		t.Fatalf("read malformed census error = %v, want exact row-width failure", err)
	}
}

func TestCensusCanonicalKeyIncludesKind(t *testing.T) {
	base := censusRow{Package: "analysis/check/body", File: "api.go", Symbol: "Result"}
	typeRow := base
	typeRow.Kind = "type"
	funcRow := base
	funcRow.Kind = "func"
	if typeRow.key() == funcRow.key() {
		t.Fatal("canonical census key aliases different declaration kinds")
	}
}

func TestCensusBudgetRatchetsEachFamilyIndependently(t *testing.T) {
	budgets := make([]censusBudget, len(censusFamilies))
	for i, family := range censusFamilies {
		budgets[i] = censusBudget{Family: family.Name}
	}
	rows := []censusRow{
		{Package: "analysis/lua/transferfacts", Classification: censusUnclassified},
		{Package: "analysis/engine/factflow", Classification: "retained-lexical"},
	}
	err := validateCensusBudgets(rows, budgets)
	if err == nil || !strings.Contains(err.Error(), "family lua/transferfacts: 1 unclassified, budget 0") {
		t.Fatalf("validate family-local budget error = %v, want lua/transferfacts growth", err)
	}

	budgets[0].MaxUnclassified = 2
	err = validateCensusBudgets(rows, budgets)
	if err == nil || !strings.Contains(err.Error(), "family lua/transferfacts: 1 unclassified, budget 2") {
		t.Fatalf("validate stale family-local budget error = %v, want exact ratchet failure", err)
	}
}

func TestCensusDispositionRequiresTypedOwnershipEvidence(t *testing.T) {
	valid := censusRow{
		Classification: "moved-primitive",
		FinalOwner:     "analysis/engine/operationplan",
		SemanticID:     "primitive:call",
		Schema:         "primitive/v1",
		Differential:   "fixture:call",
	}
	if err := validateCensusDisposition(valid); err != nil {
		t.Fatalf("valid moved-primitive disposition: %v", err)
	}

	missing := valid
	missing.Differential = "pending"
	if err := validateCensusDisposition(missing); err == nil || !strings.Contains(err.Error(), "completed differential evidence") {
		t.Fatalf("pending differential error = %v, want completed evidence failure", err)
	}

	deleted := censusRow{
		Classification: "deleted",
		FinalOwner:     "analysis/engine/operationplan",
		SemanticID:     "deleted:old-call",
		Differential:   "fixture:no-regrowth",
	}
	if err := validateCensusDisposition(deleted); err == nil || !strings.Contains(err.Error(), "empty final_owner") {
		t.Fatalf("deleted owner error = %v, want empty final_owner failure", err)
	}
}
