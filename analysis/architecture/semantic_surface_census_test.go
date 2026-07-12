package architecture

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// censusRegenerateEnv rewrites the checked-in census from the current tree
// while preserving assigned classification columns:
//
//	REGENERATE_SEMANTIC_SURFACE_CENSUS=1 go test ./analysis/architecture -run Census
const censusRegenerateEnv = "REGENERATE_SEMANTIC_SURFACE_CENSUS"

const censusDiffLimit = 50

func TestSemanticSurfaceCensusMatchesRepositorySurface(t *testing.T) {
	root := repoRoot(t)
	generated, err := generateSemanticSurfaceCensus(root)
	if err != nil {
		t.Fatalf("generate semantic surface census: %v", err)
	}
	generatedByKey := make(map[string]censusRow, len(generated))
	for _, row := range generated {
		if prev, ok := generatedByKey[row.key()]; ok {
			t.Fatalf("generator produced duplicate census key %s and %s", prev.keyString(), row.keyString())
		}
		generatedByKey[row.key()] = row
	}

	path := semanticSurfaceCensusPath(root)
	if os.Getenv(censusRegenerateEnv) == "1" {
		existing, err := readSemanticSurfaceCensus(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("read census before regeneration: %v", err)
		}
		if err := writeSemanticSurfaceCensus(path, mergeSemanticSurfaceCensus(generated, existing)); err != nil {
			t.Fatalf("write regenerated census: %v", err)
		}
	}

	checked, err := readSemanticSurfaceCensus(path)
	if err != nil {
		t.Fatalf("read checked-in census (set %s=1 to regenerate): %v", censusRegenerateEnv, err)
	}

	var duplicates []string
	var stale []string
	var drifted []string
	seen := make(map[string]struct{}, len(checked))
	for i, row := range checked {
		if i > 0 && !censusRowLess(checked[i-1], row) {
			t.Fatalf("census row %d %s is not sorted by (package, file, symbol, kind) after %s", i+2, row.keyString(), checked[i-1].keyString())
		}
		if _, ok := seen[row.key()]; ok {
			duplicates = append(duplicates, row.keyString())
			continue
		}
		seen[row.key()] = struct{}{}
		want, ok := generatedByKey[row.key()]
		if !ok {
			stale = append(stale, row.keyString())
			continue
		}
		if row.CurrentOwner != want.CurrentOwner {
			drifted = append(drifted, fmt.Sprintf("%s: current_owner %q, generator emits %q", row.keyString(), row.CurrentOwner, want.CurrentOwner))
		}
	}
	var missing []string
	for _, row := range generated {
		if _, ok := seen[row.key()]; !ok {
			missing = append(missing, row.keyString())
		}
	}
	if len(duplicates) > 0 || len(stale) > 0 || len(missing) > 0 || len(drifted) > 0 {
		var report strings.Builder
		fmt.Fprintf(&report, "checked-in census diverges from the repository surface (set %s=1 to regenerate):\n", censusRegenerateEnv)
		appendCensusDiffSection(&report, "duplicate (package, file, symbol, kind) key", duplicates)
		appendCensusDiffSection(&report, "stale row (symbol no longer exists)", stale)
		appendCensusDiffSection(&report, "missing row (symbol not censused)", missing)
		appendCensusDiffSection(&report, "drifted generated column", drifted)
		t.Fatal(report.String())
	}

	for _, row := range checked {
		if parts := splitCensusClassifications(row.Classification); len(parts) > 1 {
			t.Fatalf("census row %s carries multiple classifications %q; exactly one final classification is allowed", row.keyString(), row.Classification)
		}
		if _, ok := censusClassifications[row.Classification]; !ok {
			t.Fatalf("census row %s has classification %q outside the allowed set %v", row.keyString(), row.Classification, censusClassificationList())
		}
		if err := validateCensusDisposition(row); err != nil {
			t.Fatalf("census row %s: %v", row.keyString(), err)
		}
	}

	unclassified := make(map[string]int, len(censusFamilies))
	totals := make(map[string]int, len(censusFamilies))
	for _, row := range checked {
		family, ok := censusFamilyFor(row.Package)
		if !ok {
			t.Fatalf("census row %s belongs to no census family", row.keyString())
		}
		totals[family.Name]++
		if row.Classification == censusUnclassified {
			unclassified[family.Name]++
		}
	}
	remaining := 0
	for _, family := range censusFamilies {
		remaining += unclassified[family.Name]
		t.Logf("census family %-26s unclassified %4d of %4d", family.Name, unclassified[family.Name], totals[family.Name])
	}
	t.Logf("census total %d rows, %d unclassified", len(checked), remaining)

	budgets, err := readCensusBudgets(filepath.Join(root, "analysis", "architecture", "semantic_surface_census_budget.csv"))
	if err != nil {
		t.Fatalf("read semantic surface ownership budget: %v", err)
	}
	if err := validateCensusBudgets(checked, budgets); err != nil {
		t.Fatal(err)
	}
}

type censusBudget struct {
	Family          string
	MaxUnclassified int
}

func readCensusBudgets(path string) ([]censusBudget, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	wantHeader := []string{"family", "max_unclassified"}
	if len(records) == 0 {
		return nil, fmt.Errorf("%s is empty, want header row %v", path, wantHeader)
	}
	if len(records[0]) != len(wantHeader) {
		return nil, fmt.Errorf("%s header has %d columns, want %d", path, len(records[0]), len(wantHeader))
	}
	for i := range wantHeader {
		if records[0][i] != wantHeader[i] {
			return nil, fmt.Errorf("%s header column %d is %q, want %q", path, i+1, records[0][i], wantHeader[i])
		}
	}

	budgets := make([]censusBudget, 0, len(records)-1)
	for i, record := range records[1:] {
		if len(record) != len(wantHeader) {
			return nil, fmt.Errorf("%s row %d has %d columns, want %d", path, i+2, len(record), len(wantHeader))
		}
		limit, err := strconv.Atoi(record[1])
		if err != nil || limit < 0 {
			return nil, fmt.Errorf("%s row %d has invalid max_unclassified %q", path, i+2, record[1])
		}
		budgets = append(budgets, censusBudget{Family: record[0], MaxUnclassified: limit})
	}
	return budgets, nil
}

func validateCensusBudgets(rows []censusRow, budgets []censusBudget) error {
	if len(budgets) != len(censusFamilies) {
		return fmt.Errorf("semantic surface ownership budget has %d families, want %d", len(budgets), len(censusFamilies))
	}
	limits := make(map[string]int, len(budgets))
	for i, budget := range budgets {
		if i >= len(censusFamilies) || budget.Family != censusFamilies[i].Name {
			want := "<none>"
			if i < len(censusFamilies) {
				want = censusFamilies[i].Name
			}
			return fmt.Errorf("semantic surface ownership budget row %d names family %q, want %q in census-family order", i+2, budget.Family, want)
		}
		if _, duplicate := limits[budget.Family]; duplicate {
			return fmt.Errorf("semantic surface ownership budget repeats family %q", budget.Family)
		}
		limits[budget.Family] = budget.MaxUnclassified
	}

	actual := make(map[string]int, len(censusFamilies))
	for _, row := range rows {
		family, ok := censusFamilyFor(row.Package)
		if !ok {
			return fmt.Errorf("census row %s belongs to no census family", row.keyString())
		}
		if row.Classification == censusUnclassified {
			actual[family.Name]++
		}
	}
	for _, family := range censusFamilies {
		if actual[family.Name] != limits[family.Name] {
			return fmt.Errorf("semantic surface ownership budget is stale in family %s: %d unclassified, budget %d; update the family-local ratchet in the same reviewed change", family.Name, actual[family.Name], limits[family.Name])
		}
	}
	return nil
}

func validateCensusDisposition(row censusRow) error {
	if row.Classification == censusUnclassified {
		return nil
	}
	require := func(field, value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("classification %q requires %s", row.Classification, field)
		}
		return nil
	}
	requireEvidence := func() error {
		if err := require("differential", row.Differential); err != nil {
			return err
		}
		if row.Differential == "pending" {
			return fmt.Errorf("classification %q requires completed differential evidence, got %q", row.Classification, row.Differential)
		}
		return nil
	}
	requireSemanticPrefix := func(prefixes ...string) error {
		if err := require("semantic_id", row.SemanticID); err != nil {
			return err
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(row.SemanticID, prefix) {
				return nil
			}
		}
		return fmt.Errorf("classification %q requires semantic_id with prefix %s, got %q", row.Classification, strings.Join(prefixes, " or "), row.SemanticID)
	}

	switch row.Classification {
	case "retained-lexical":
		if err := requireSemanticPrefix("lexical:"); err != nil {
			return err
		}
	case "moved-primitive":
		if err := requireSemanticPrefix("primitive:"); err != nil {
			return err
		}
	case "observation-compiler":
		if err := requireSemanticPrefix("observation:", "product:"); err != nil {
			return err
		}
	case "boundary-provider":
		if err := requireSemanticPrefix("boundary:"); err != nil {
			return err
		}
	case "deleted":
		if row.FinalOwner != "" {
			return fmt.Errorf("classification %q requires empty final_owner, got %q", row.Classification, row.FinalOwner)
		}
		if row.Schema != "" {
			return fmt.Errorf("classification %q requires empty schema, got %q", row.Classification, row.Schema)
		}
		if err := requireSemanticPrefix("deleted:"); err != nil {
			return err
		}
		return requireEvidence()
	default:
		return fmt.Errorf("unknown classification %q", row.Classification)
	}

	if err := require("final_owner", row.FinalOwner); err != nil {
		return err
	}
	if err := require("schema", row.Schema); err != nil {
		return err
	}
	return requireEvidence()
}

func appendCensusDiffSection(report *strings.Builder, label string, entries []string) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(report, "%s (%d):\n", label, len(entries))
	for i, entry := range entries {
		if i == censusDiffLimit {
			fmt.Fprintf(report, "  ... and %d more\n", len(entries)-censusDiffLimit)
			break
		}
		fmt.Fprintf(report, "  %s\n", entry)
	}
}

// splitCensusClassifications splits a classification cell on list separators
// so a row carrying several values fails as multiple ownership, not as one
// unknown word.
func splitCensusClassifications(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', ';', '|', '+', ' ', '\t':
			return true
		}
		return false
	})
}

func censusClassificationList() []string {
	return []string{
		"retained-lexical",
		"moved-primitive",
		"observation-compiler",
		"boundary-provider",
		"deleted",
		censusUnclassified,
	}
}
