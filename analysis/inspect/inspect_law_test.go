package inspect

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// inspectLawFixture is the one fixture every law here reads. One fixture is
// enough: the inspector is a reading of the public product surface, so a law
// that holds for one compiled-and-solved session holds for every session that
// surface can produce.
const inspectLawFixture = "bench/fibonacci"

// inspectLawCommands are the commands whose renderings are laws. diff is
// stated separately because it takes a second session.
var inspectLawCommands = []string{"target", "rows", "publish", "why"}

// TestInspectCommandsAreNonemptyAndStable is the rendering contract: every
// command answers, and answers the same way twice. Stability is what makes the
// diff below a reading of two solves rather than a reading of two renderings.
func TestInspectCommandsAreNonemptyAndStable(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	for _, name := range inspectLawCommands {
		first, err := session.Command(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.TrimSpace(first) == "" {
			t.Fatalf("%s produced no output", name)
		}
		second, err := session.Command(name)
		if err != nil {
			t.Fatalf("%s again: %v", name, err)
		}
		if first != second {
			t.Fatalf("%s output is not stable", name)
		}
	}
}

// TestInspectEveryLineNamesItsAccessor holds every rendered fact to the rule
// that it names the accessor it was read from. A line a reader cannot
// reproduce from the public surface is a line this inspector invented.
func TestInspectEveryLineNamesItsAccessor(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	for _, name := range inspectLawCommands {
		rendered, err := session.Command(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, line := range strings.Split(rendered, "\n") {
			if line == "" {
				continue
			}
			accessor, _, cut := strings.Cut(line, "=")
			if !cut || accessor == "" {
				t.Fatalf("%s line names no accessor: %q", name, line)
			}
		}
	}
}

// TestInspectRowNamesTheSolvedStateItHolds is the row contract: an identity
// the session indexes renders under the accessor that produced it.
func TestInspectRowNamesTheSolvedStateItHolds(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	target, err := session.Command("target")
	if err != nil {
		t.Fatal(err)
	}
	id := firstPrefixedValue(t, target, "contract.OperationContentID(")
	rendered, err := session.Command("row", id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "contract.OperationContentID=") {
		t.Fatalf("row did not name contract.OperationContentID:\n%s", rendered)
	}
	if !strings.Contains(rendered, "session.Lookup("+id+").Kind=operation") {
		t.Fatalf("row did not name the indexed kind:\n%s", rendered)
	}
}

// TestInspectWhyWalksTheDeclaredProgram is the provenance contract. why must
// reach a cell's value through the Program declaration that produced it: the
// candidate relation, the ordered joins with the read form each declares, and
// the fold. A why that only restated diagnostics would answer nothing about
// where a value came from.
func TestInspectWhyWalksTheDeclaredProgram(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	rendered, err := session.Command("why")
	if err != nil {
		t.Fatal(err)
	}
	required := []string{
		"composite.QueryIssuance.Count=",
		".QueryRegistration.SubjectAt(",
		".Program.Candidate.Axis=",
		".Program.Candidate.Member=",
		".Program.Joins[0].Sources[0]=Candidate",
		".Program.Joins[0].Read.Form=",
		".Program.Fold.Reducer.Member=",
		".DeclaringRuleCount=",
	}
	for _, fragment := range required {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("why did not walk %q", fragment)
		}
	}
	// The read form vocabulary is the declaration's, not this package's.
	if !strings.Contains(rendered, ".Read.Form=Exact") &&
		!strings.Contains(rendered, ".Read.Form=Selected") &&
		!strings.Contains(rendered, ".Read.Form=Summary") &&
		!strings.Contains(rendered, ".Read.Form=Complete") {
		t.Fatalf("why named no declared read form")
	}
}

// TestInspectWhyAnswersForAFactor states that why also walks a factor named by
// the axis key the schema declares it under, not only a row identity.
func TestInspectWhyAnswersForAFactor(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	rows, err := session.Command("rows")
	if err != nil {
		t.Fatal(err)
	}
	axis := firstPrefixedValue(t, rows, "compilation.RulePlans.AxisAt(0).Key=")
	rendered, err := session.Command("why", axis)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "why.Key="+axis) {
		t.Fatalf("why did not name the factor it was asked about:\n%s", rendered)
	}
	if !strings.Contains(rendered, "why["+axis+"].ReadBy=") {
		t.Fatalf("why did not name the families that read the factor:\n%s", rendered)
	}
	if !strings.Contains(rendered, "why["+axis+"].DeclaringRuleCount=") {
		t.Fatalf("why did not count the rules publishing into the factor:\n%s", rendered)
	}
}

// TestInspectWhyNamesItsSolveOutcome holds why to reporting the analyzer's own
// verdict verbatim. A session whose solve refused must say so under the
// diagnostic's own spelling rather than render an empty provenance.
func TestInspectWhyNamesItsSolveOutcome(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	rendered, err := session.Command("why")
	if err != nil {
		t.Fatal(err)
	}
	reason := "solve.AnalyzeDiagnostics.Reason=" + session.SolveDiagnostics().Reason.String()
	if !strings.Contains(rendered, reason) {
		t.Fatalf("why did not name %q", reason)
	}
	phase := "solve.AnalyzeDiagnostics.Phase=" + session.SolveDiagnostics().Phase.String()
	if !strings.Contains(rendered, phase) {
		t.Fatalf("why did not name %q", phase)
	}
}

// TestInspectRowsNamesTheConstructTopology is the topology contract: the
// factor axes, the rule rows, each row's activation candidate, and the owner
// fence each published output is held to.
func TestInspectRowsNamesTheConstructTopology(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	rendered, err := session.Command("rows")
	if err != nil {
		t.Fatal(err)
	}
	required := []string{
		"compilation.RulePlans.AxisCount=",
		"compilation.RulePlans.AxisAt(0).Key=",
		"composite.AxisStorage(",
		"composite.Table.RuleCount=",
		".Program.Candidate.Member=",
		".Program.Fold.Outputs[0].OwnerFenceHeld=",
		".Program.Activation.TransportCount=",
	}
	for _, fragment := range required {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rows did not name %q", fragment)
		}
	}
	// The owner fence is a verdict, not a decoration: a rule that publishes
	// outside the axis frame its Template owns is a defect the topology view
	// must surface rather than round off.
	if strings.Contains(rendered, ".OwnerFenceHeld=false") {
		t.Fatalf("a declared output escapes its rule's owner fence:\n%s", rendered)
	}
}

// TestInspectSelfDiffIsEmpty states that a fixture diffed against itself
// differs nowhere. It is the diff's soundness floor: any per-run identity,
// map order, or timestamp leaking into a rendering shows up here.
func TestInspectSelfDiffIsEmpty(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	diff, err := session.Command("diff", inspectLawFixture)
	if err != nil {
		t.Fatal(err)
	}
	if diff != "" {
		t.Fatalf("self diff = %q", diff)
	}
}

// TestInspectIndexRowReadAllocatesNothing states that reading one indexed row
// after the solve allocates nothing. The index holds sealed scalars, so a
// lookup copies a value and never materializes a row.
func TestInspectIndexRowReadAllocatesNothing(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	target, err := session.Command("target")
	if err != nil {
		t.Fatal(err)
	}
	id, ok := ParseContentID(firstPrefixedValue(t, target, "contract.OperationContentID("))
	if !ok {
		t.Fatal("operation identity")
	}
	if _, found := session.Lookup(id); !found {
		t.Fatal("indexed identity is not addressable")
	}
	allocs := testing.AllocsPerRun(100, func() {
		if _, found := session.Lookup(id); !found {
			t.Fatal("indexed identity is not addressable")
		}
	})
	if allocs != 0 {
		t.Fatalf("Session.Lookup allocs = %v, want 0", allocs)
	}
}

// TestInspectSolvedCellRowReadAllocatesNothing states the same property for
// the solved plane: once a cell has been admitted, reading one coordinate row
// and every column of it allocates nothing. The view is offsets into the
// payload the solve detached, so a row read materializes no row.
//
// The law is quantified over the cells this session publishes. A session whose
// solve refused publishes none, and the law then reports that it read none
// rather than asserting against a state the analyzer did not reach.
func TestInspectSolvedCellRowReadAllocatesNothing(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	solved := session.Result()
	read := 0
	if solved != nil {
		for familyIndex := 0; familyIndex < solved.FamilyCount(); familyIndex++ {
			family, familyOK := solved.FamilyAt(familyIndex)
			if !familyOK {
				continue
			}
			layout, layoutOK := session.plan.QueryResultLayout(family.Key())
			if !layoutOK {
				continue
			}
			for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
				view, refusal, viewOK := session.CellView(familyIndex, queryIndex)
				if !viewOK {
					continue
				}
				if refusal.Available() {
					t.Fatalf("family %s query %d payload was refused: %s", family.Key(), queryIndex, refusal)
				}
				if view.RowCount() == 0 {
					continue
				}
				first, firstOK := view.At(0)
				if !firstOK {
					t.Fatalf("family %s query %d row 0 is not addressable", family.Key(), queryIndex)
				}
				coordinate := first.ID()
				allocs := testing.AllocsPerRun(100, func() {
					readEveryColumn(t, layout, view, coordinate)
				})
				if allocs != 0 {
					t.Fatalf("family %s query %d row read allocs = %v, want 0", family.Key(), queryIndex, allocs)
				}
				read++
			}
		}
	}
	if session.SolveStatus() == analysis.AnalyzeComplete && read == 0 {
		// A completed solve publishes cells. Reading none of them would make
		// this law vacuous without saying so, which is how a zero-allocation
		// claim survives the path it stopped covering.
		t.Fatal("solve completed and published no readable cell")
	}
	t.Logf("solve status=%v; zero-allocation row reads proved on %d published cells", session.SolveStatus(), read)
}

// readEveryColumn is one whole row read: the addressing the plane declares -
// the coordinate bisection on a keyed answer, the row's position on a
// general-fold answer that publishes no coordinate - the row's class, and
// every declared column under its carrier.
func readEveryColumn(t *testing.T, layout *plane.Sealed, view plane.View, coordinate identity.ContentID) {
	t.Helper()
	row, ok := view.Lookup(coordinate)
	if !ok {
		if coordinate.Available() {
			t.Fatal("coordinate is not addressable")
		}
		// A general fold answers its whole subject at one point whose identity
		// is the query site's own, so its row carries no coordinate of its own
		// and is addressed by position.
		row, ok = view.At(0)
		if !ok {
			t.Fatal("the unkeyed answer's row is not addressable")
		}
	}
	row.Class()
	for column := 0; column < layout.ColumnCount(); column++ {
		declared, declaredOK := layout.ColumnAt(column)
		if !declaredOK {
			continue
		}
		switch declared.Carrier {
		case plane.CarrierMember:
			row.Member(column)
		case plane.CarrierEvidence:
			row.Evidence(column)
		case plane.CarrierFlag:
			row.Flag(column)
		case plane.CarrierOrdinal:
			row.Ordinal(column)
		case plane.CarrierIdentity:
			row.Identity(column)
		case plane.CarrierWords:
			for index := 0; index < row.Count(); index++ {
				row.WordAt(index)
			}
		case plane.CarrierAtoms:
			for index := 0; index < row.Count(); index++ {
				row.AtomAt(index)
			}
		}
	}
}

// TestInspectNamesEveryUnexposedAccessor states that the inspector reports the
// facts the public surface does not publish instead of reaching past it. The
// construct and solved lists are rendered by rows; the transition list by
// publish.
func TestInspectNamesEveryUnexposedAccessor(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	rows, err := session.Command("rows")
	if err != nil {
		t.Fatal(err)
	}
	publish, err := session.Command("publish")
	if err != nil {
		t.Fatal(err)
	}
	if len(Gaps()) == 0 {
		t.Fatal("the gap list is empty")
	}
	for _, gap := range Gaps() {
		line := "unexposed." + gap.Layer + "=" + gap.Accessor
		if !strings.Contains(rows, line) {
			t.Fatalf("rows does not name %q", line)
		}
	}
	for _, gap := range transitionGaps() {
		line := "unexposed." + gap.Layer + "=" + gap.Accessor
		if !strings.Contains(publish, line) {
			t.Fatalf("publish does not name %q", line)
		}
	}
}

// TestInspectPublishNamesEverySummaryColumn states that publish renders each
// family's declared publication columns. They are a declaration, so they are
// readable whether or not the fixture reached a solve.
func TestInspectPublishNamesEverySummaryColumn(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	rendered, err := session.Command("publish")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, ".ColumnCount=") || !strings.Contains(rendered, ".ColumnAt(0).Carrier=") {
		t.Fatalf("publish named no summary column:\n%s", rendered)
	}
	if !strings.Contains(rendered, "transition.DeclaredTransportRows=") {
		t.Fatalf("publish named no transition row count:\n%s", rendered)
	}
}

// TestInspectRefusesUnknownCommands states the command surface is closed.
func TestInspectRefusesUnknownCommands(t *testing.T) {
	session := openFixture(t, inspectLawFixture)
	if _, err := session.Command("elaborate"); err == nil {
		t.Fatal("unknown command was admitted")
	}
	if _, err := session.Command("row"); err == nil {
		t.Fatal("row without an identity was admitted")
	}
	if _, err := session.Command("row", "not-an-identity"); err == nil {
		t.Fatal("row with a malformed identity was admitted")
	}
}

func openFixture(t *testing.T, name string) *Session {
	t.Helper()
	root, err := testfixture.RepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	session, err := Open(root, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !session.Close() {
			t.Error("close session")
		}
	})
	return session
}

func firstPrefixedValue(t *testing.T, text, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		_, value, ok := strings.Cut(line, "=")
		if ok && value != "" {
			return value
		}
	}
	t.Fatalf("no line prefixed %q", prefix)
	return ""
}
