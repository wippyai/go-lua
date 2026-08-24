package oracle

import (
	"context"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// A module's construction rows are O(its source). Every plane the seal builds
// - the Points a body declares, the Rules that read them, the Groups those
// Rules contribute to - names something the source wrote, so twice the source
// is twice the rows. A plane that grows faster than the source is a product
// over a pair the source never wrote, and the whole seal inherits it: each row
// mints identities, becomes a graph member, and is retained through the sealed
// tables.
//
// The measurement is a growth ratio rather than an absolute count, because the
// absolute count is a property of the schema and the fixture while the growth
// is a property of the construction. Points are the linear reference: they are
// one per authored program point, so rows per point is the constant that must
// stay a constant.
//
// rowPopulationLawTolerance is how much rows-per-point may drift between two
// prefixes of one fixture before the growth is superlinear rather than a
// constant factor. Two is generous: a linear plane holds this ratio at 1.
const rowPopulationLawTolerance = 2.0

// rowPopulationLawPrefixes are two prefixes of the same fixture. The larger is
// the smaller plus more of the same, so every plane should grow with it and no
// faster.
//
// They are 100 and 150 rather than 40 and 100 because the first 146 lines of
// this fixture are almost entirely type declarations: prefix40 declares 280
// points but only 8 yield boundaries, while prefix100 declares 606 points and
// 49. The two are not the same kind of program, so rows per point moves
// between them for a reason that is not growth. Prefix100 and prefix150 are
// both in the executable-code regime and are the honest pair to compare.
// rowPopulationLawReferencePrefix is measured and logged alongside them so the
// whole curve stays visible to a reader.
var rowPopulationLawPrefixes = [2]int{100, 150}

const rowPopulationLawReferencePrefix = 40

type rowPopulationSample struct {
	cases int
	rows  engine.DbgProgramRowCounts
	heap  uint64
}

// TestConstructionRowsAreLinearInSource states the L8 population law over the
// edge-matrix prefix ladder.
func TestConstructionRowsAreLinearInSource(t *testing.T) {
	reference := measureConstructionRowPopulation(t, rowPopulationLawReferencePrefix)
	t.Logf("reference prefix%d points=%d rules=%d ruleReads=%d heapAllocMB=%d (logged, not asserted: this prefix is mostly type declarations)",
		rowPopulationLawReferencePrefix, reference.rows.Points, reference.rows.Rules, reference.rows.RuleReads, reference.heap>>20)

	samples := [2]rowPopulationSample{}
	for index, cases := range rowPopulationLawPrefixes {
		samples[index] = measureConstructionRowPopulation(t, cases)
		t.Logf("prefix%d points=%d rules=%d groups=%d queries=%d ruleReads=%d heapAllocMB=%d",
			cases, samples[index].rows.Points, samples[index].rows.Rules, samples[index].rows.Groups,
			samples[index].rows.Queries, samples[index].rows.RuleReads, samples[index].heap>>20)
	}
	small, large := samples[0], samples[1]
	if small.rows.Points == 0 || large.rows.Points == 0 || small.rows.Rules == 0 {
		t.Fatal("the prefix ladder declared no point or rule rows")
	}
	if large.rows.Points <= small.rows.Points {
		t.Fatal("the larger prefix declared no more points than the smaller one")
	}
	for _, plane := range []struct {
		name         string
		small, large uint64
	}{
		{"rules", small.rows.Rules, large.rows.Rules},
		{"groups", small.rows.Groups, large.rows.Groups},
		{"ruleReads", small.rows.RuleReads, large.rows.RuleReads},
		{"groupInputs", small.rows.GroupInputs, large.rows.GroupInputs},
	} {
		perPointSmall := float64(plane.small) / float64(small.rows.Points)
		perPointLarge := float64(plane.large) / float64(large.rows.Points)
		if perPointLarge > perPointSmall*rowPopulationLawTolerance {
			t.Errorf("%s per point grew from %.2f at prefix%d to %.2f at prefix%d: the plane is a product, not a row per source coordinate",
				plane.name, perPointSmall, small.cases, perPointLarge, large.cases)
		}
	}
}

// measureConstructionRowPopulation seals one prefix and reports the row
// population its construction declared, with the live heap it left behind.
func measureConstructionRowPopulation(t *testing.T, cases int) rowPopulationSample {
	t.Helper()
	project := corpusHarnessFixture(t, "semantic/type-engine-edge-matrix")
	source := corpusHarnessSourceText(t, project, "main.lua")
	end := edgeMatrixPrefixEnd(t, source, cases)
	linked, err := testfixture.SealSource(corpusHarnessContract(t), "main.lua", source[:end])
	if err != nil {
		t.Fatal(err)
	}
	engine.DbgProgramRowsReset()
	plan, status, diagnostics := analysis.CompileWithDiagnostics(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("prefix%d compile = %v plan=%t diagnostics=%+v", cases, status, plan != nil, diagnostics)
	}
	defer plan.Close()
	// The construction runs inside the first solve. Its answer is not this
	// law's subject: a refused solve has already declared the rows this law
	// counts, and counting them is the point.
	_, _, _ = plan.SolveWithDiagnostics(context.Background(), corpusHarnessSolveOptions())
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return rowPopulationSample{cases: cases, rows: engine.DbgProgramRows(), heap: stats.HeapAlloc}
}
