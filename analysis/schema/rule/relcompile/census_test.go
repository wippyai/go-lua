package relcompile_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"

	callactivation "github.com/wippyai/go-lua/domain/call/activation/program"
	calldispatch "github.com/wippyai/go-lua/domain/call/dispatch/program"
	effectcallsitebody "github.com/wippyai/go-lua/domain/effect/callsite/body/program"
	effectcallsiteopaque "github.com/wippyai/go-lua/domain/effect/callsite/opaque/program"
	effectcallsiteselected "github.com/wippyai/go-lua/domain/effect/callsite/selected/program"
	heapallocationempty "github.com/wippyai/go-lua/domain/heap/allocation/empty/program"
	heapallocationingress "github.com/wippyai/go-lua/domain/heap/allocation/ingress"
	heapbootstrap "github.com/wippyai/go-lua/domain/heap/bootstrap"
	heapformalfreeze "github.com/wippyai/go-lua/domain/heap/formalfreeze/program"
	heappublicationfreeze "github.com/wippyai/go-lua/domain/heap/publicationfreeze/program"
	packsource "github.com/wippyai/go-lua/domain/pack/source"
	placementcapture "github.com/wippyai/go-lua/domain/placement/capture/program"
	placementcontainment "github.com/wippyai/go-lua/domain/placement/containment/program"
	placementformal "github.com/wippyai/go-lua/domain/placement/formal/program"
	placementpublicationescape "github.com/wippyai/go-lua/domain/placement/publicationescape/program"
	placementreturnescape "github.com/wippyai/go-lua/domain/placement/returnescape/program"
	placementstore "github.com/wippyai/go-lua/domain/placement/store/program"
	placementsuspension "github.com/wippyai/go-lua/domain/placement/suspension/program"
	placementtransfer "github.com/wippyai/go-lua/domain/placement/transfer/program"
	statictransfer "github.com/wippyai/go-lua/domain/static/transfer"
	valueallocation "github.com/wippyai/go-lua/domain/value/allocation/program"
	valuearithmetic "github.com/wippyai/go-lua/domain/value/arithmetic/program"
	valuebodyresult "github.com/wippyai/go-lua/domain/value/bodyresult/program"
	valuebootstrap "github.com/wippyai/go-lua/domain/value/bootstrap"
	valueequality "github.com/wippyai/go-lua/domain/value/equality/program"
	valuefreshresult "github.com/wippyai/go-lua/domain/value/freshresult/program"
	valuemoduleload "github.com/wippyai/go-lua/domain/value/moduleload/program"
	valueorder "github.com/wippyai/go-lua/domain/value/order/program"
	valuerefinement "github.com/wippyai/go-lua/domain/value/refinement/program"
	valueresultalias "github.com/wippyai/go-lua/domain/value/resultalias/program"
	valueruntimekind "github.com/wippyai/go-lua/domain/value/runtimekind/program"
	valuesource "github.com/wippyai/go-lua/domain/value/source"
	valuetransfer "github.com/wippyai/go-lua/domain/value/transfer"
)

// specimen is one census row: the authored declaration of one analysis family
// and the plane it belongs to.
type specimen struct {
	Family string
	Plane  string
	Spec   rule.Spec
}

// declared is every family whose rule declaration carries an authored Program.
// The wired registration arms of placement capture, containment, suspension
// and publication escape are the same families through the legacy protocol, so
// the declarative sibling is the census row and the wired arm is a deletion
// entry rather than a second family.
func declared() []specimen {
	return []specimen{
		{Family: "call/activation", Plane: "family", Spec: callactivation.RuleEntry()},
		{Family: "call/dispatch", Plane: "family", Spec: calldispatch.RuleEntry()},
		{Family: "effect/callsite/body", Plane: "family", Spec: effectcallsitebody.RuleEntry()},
		{Family: "effect/callsite/opaque", Plane: "family", Spec: effectcallsiteopaque.RuleEntry()},
		{Family: "effect/callsite/selected", Plane: "family", Spec: effectcallsiteselected.RuleEntry()},
		{Family: "heap/allocation/empty", Plane: "family", Spec: heapallocationempty.RuleEntry()},
		{Family: "heap/allocation/ingress", Plane: "seed", Spec: heapallocationingress.RuleEntry()},
		{Family: "heap/bootstrap", Plane: "seed", Spec: heapbootstrap.RuleEntry()},
		{Family: "heap/formalfreeze", Plane: "family", Spec: heapformalfreeze.RuleEntry()},
		{Family: "heap/publicationfreeze", Plane: "family", Spec: heappublicationfreeze.RuleEntry()},
		{Family: "pack/source", Plane: "seed", Spec: packsource.RuleEntry()},
		{Family: "placement/capture", Plane: "family", Spec: placementcapture.RuleEntry()},
		{Family: "placement/containment", Plane: "family", Spec: placementcontainment.RuleEntry()},
		{Family: "placement/formal", Plane: "family", Spec: placementformal.RuleEntry()},
		{Family: "placement/publicationescape", Plane: "family", Spec: placementpublicationescape.RuleEntry()},
		{Family: "placement/returnescape", Plane: "family", Spec: placementreturnescape.RuleEntry()},
		{Family: "placement/store", Plane: "family", Spec: placementstore.RuleEntry()},
		{Family: "placement/suspension", Plane: "family", Spec: placementsuspension.RuleEntry()},
		{Family: "placement/suspension-evidence", Plane: "family", Spec: placementsuspension.EvidenceRuleEntry()},
		{Family: "placement/transfer", Plane: "family", Spec: placementtransfer.RuleEntry()},
		{Family: "static/transfer", Plane: "family", Spec: statictransfer.RuleEntry()},
		{Family: "value/allocation", Plane: "seed", Spec: valueallocation.RuleEntry()},
		{Family: "value/arithmetic", Plane: "family", Spec: valuearithmetic.RuleEntry()},
		{Family: "value/bodyresult", Plane: "family", Spec: valuebodyresult.RuleEntry()},
		{Family: "value/bootstrap", Plane: "seed", Spec: valuebootstrap.RuleEntry()},
		{Family: "value/equality", Plane: "family", Spec: valueequality.RuleEntry()},
		{Family: "value/freshresult", Plane: "family", Spec: valuefreshresult.RuleEntry()},
		{Family: "value/moduleload", Plane: "family", Spec: valuemoduleload.RuleEntry()},
		{Family: "value/order", Plane: "family", Spec: valueorder.RuleEntry()},
		{Family: "value/refinement", Plane: "family", Spec: valuerefinement.RuleEntry()},
		{Family: "value/resultalias", Plane: "family", Spec: valueresultalias.RuleEntry()},
		{Family: "value/runtimekind", Plane: "family", Spec: valueruntimekind.RuleEntry()},
		{Family: "value/source", Plane: "seed", Spec: valuesource.RuleEntry()},
		{Family: "value/transfer", Plane: "family", Spec: valuetransfer.RuleEntry()},
	}
}

// uncommitted is every family whose declarative rule declaration is not part
// of this revision. The family exists and still runs on its wired
// registration arm, so it is a row like any other: the corpus is not covered
// while a declaration the census cannot read is left out of the matrix.
func uncommitted() []entry {
	rows := []entry{
		{Family: "placement/allocationbirth", Rule: "placement-allocation-birth"},
		{Family: "placement/freshbirth", Rule: "placement-fresh-birth"},
		{Family: "typestate", Rule: "typestate"},
	}
	for index := range rows {
		rows[index].Plane = "family"
		rows[index].Status = statusCoupling
		rows[index].Site = "program"
		rows[index].Missing = "declaration"
		rows[index].Reason = "the declarative rule declaration is not committed at this revision; the family runs on its wired registration arm"
	}
	return rows
}

// entry is one machine-readable census row.
type entry struct {
	Family      string `json:"family"`
	Plane       string `json:"plane"`
	Rule        string `json:"rule"`
	Status      string `json:"status"`
	Sketch      string `json:"sketch,omitempty"`
	Site        string `json:"site,omitempty"`
	Missing     string `json:"missing,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Expressions int    `json:"expressions,omitempty"`
}

const (
	statusCompiles = "COMPILES"
	statusCoupling = "COUPLING-FINDING"
	statusABIGap   = "ABI-GAP"
)

// survey resolves one authored declaration and lowers it, and reports what the
// row proves: a complete logical plan, or the exact declaration site whose
// relational counterpart no owner states.
func survey(t *testing.T, row specimen) entry {
	t.Helper()
	result := entry{Family: row.Family, Plane: row.Plane, Rule: string(row.Spec.Key)}
	surfaces := newOwners(t)
	placement := surfaces.install(row.Spec)
	rules, err := relcompile.Resolve(surfaces.registry, row.Spec, placement)
	if err != nil {
		refusal := refusalOf(t, err)
		result.Status = statusCoupling
		result.Site = refusal.Site.Path
		result.Missing = refusal.Kind.String()
		result.Reason = refusal.Reason.String()
		return result
	}
	compiled := lower(t, surfaces, row.Spec, rules)
	result.Status = statusCompiles
	result.Expressions = len(compiled.Expressions())
	result.Sketch = sketch(compiled)
	return result
}

func lower(t *testing.T, surfaces *owners, spec rule.Spec, rules []relcompile.Rule) plan.ExecutionSchema {
	t.Helper()
	owner, err := surfaces.registry.Owner(relcompile.Site{Path: "census"}, schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: spec.Writes})
	if err != nil {
		t.Fatalf("resolve writing owner of %s: %v", spec.Key, err)
	}
	schemaID, ok := model.IssueSchemaID(owner, surfaces.token("schema", relcompile.EntryName(schema.SurfaceKindRule, spec.Key)))
	if !ok {
		t.Fatalf("issue schema identity for %s", spec.Key)
	}
	declaration := surfaces.registry.Declaration(schemaID)
	declaration.Rules = rules
	compiled, err := relcompile.Compile(declaration)
	if err != nil {
		t.Fatalf("compile %s: %v", spec.Key, err)
	}
	return compiled
}

// sketch renders the logical plan of one compiled schema in the closed
// grammar, so the matrix carries the shape a row lowers to and not only that
// it lowered.
func sketch(compiled plan.ExecutionSchema) string {
	shapes := make([]string, 0, len(compiled.Expressions()))
	for _, expression := range compiled.Expressions() {
		shapes = append(shapes, render(expression.Expression()))
	}
	sort.Strings(shapes)
	rendered := ""
	for index, shape := range shapes {
		if index != 0 {
			rendered += " | "
		}
		rendered += shape
	}
	return rendered
}

func render(expression algebra.Expression) string {
	switch value := expression.(type) {
	case algebra.Input:
		return "Input"
	case algebra.Select:
		return "Select(" + render(value.Child()) + ")"
	case algebra.Project:
		return "Project(" + render(value.Child()) + ")"
	case algebra.Complete:
		return "Complete(" + render(value.Child()) + ")"
	case algebra.Join:
		return "Join(" + render(value.Left()) + "," + render(value.Right()) + ")"
	case algebra.Merge:
		return "Merge(" + renderAll(value.Inputs()) + ")"
	case algebra.Group:
		return "Group(" + render(value.Child()) + ")"
	case algebra.Apply:
		return "Apply(" + renderAll(value.Inputs()) + ")"
	case algebra.Publish:
		return "Publish(" + render(value.Child()) + ")"
	default:
		return "?"
	}
}

func renderAll(expressions []algebra.Expression) string {
	rendered := ""
	for index, expression := range expressions {
		if index != 0 {
			rendered += ","
		}
		rendered += render(expression)
	}
	return rendered
}

// TestDeclarationCensus is the machine-readable coverage matrix. Every row is
// one authored declaration surveyed through the same lowering, and the matrix
// is pinned so a change in what the corpus proves is a reviewed change.
func TestDeclarationCensus(t *testing.T) {
	rows := make([]entry, 0, 64)
	for _, row := range declared() {
		rows = append(rows, survey(t, row))
	}
	rows = append(rows, rawAccessCensus(t)...)
	rows = append(rows, uncommitted()...)
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Family != rows[right].Family {
			return rows[left].Family < rows[right].Family
		}
		return rows[left].Rule < rows[right].Rule
	})

	rendered, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatalf("render census: %v", err)
	}
	rendered = append(rendered, '\n')
	path := filepath.Join("testdata", "census.json")
	if os.Getenv("RELCOMPILE_CENSUS_UPDATE") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(path, rendered, 0o644); err != nil {
			t.Fatalf("write census: %v", err)
		}
	}
	pinned, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pinned census: %v", err)
	}
	if string(pinned) != string(rendered) {
		t.Fatalf("the census matrix moved; rerun with RELCOMPILE_CENSUS_UPDATE=1 and review\n%s", rendered)
	}
}

// TestEveryCensusFindingNamesItsDeclarationSite states the census reports
// findings rather than compensating for them: a row that does not lower names
// the authored site and the owner statement that is missing, so the remedy is
// a declaration change and never a lowering that guesses the other side.
func TestEveryCensusFindingNamesItsDeclarationSite(t *testing.T) {
	for _, row := range declared() {
		result := survey(t, row)
		if result.Status == statusCompiles {
			if result.Sketch == "" || result.Expressions == 0 {
				t.Fatalf("%s compiles to no logical plan", result.Family)
			}
			continue
		}
		if result.Site == "" {
			t.Fatalf("%s reports a finding with no declaration site", result.Family)
		}
		if result.Missing == "" || result.Reason == "" {
			t.Fatalf("%s reports a finding without naming the missing owner statement", result.Family)
		}
	}
}

// TestEveryDeclaredFamilyHasOneCensusRow states the matrix is total over the
// corpus: every authored family, every seed, the two raw indexed access plans
// and every declaration that does not build are one row each.
func TestEveryDeclaredFamilyHasOneCensusRow(t *testing.T) {
	seen := map[string]bool{}
	count := 0
	for _, row := range declared() {
		count++
		if seen[string(row.Spec.Key)] {
			t.Fatalf("rule %s is censused twice", row.Spec.Key)
		}
		seen[string(row.Spec.Key)] = true
	}
	count += len(rawAccessCensus(t)) + len(uncommitted())

	pinned, err := os.ReadFile(filepath.Join("testdata", "census.json"))
	if err != nil {
		t.Fatalf("read pinned census: %v", err)
	}
	var rows []entry
	if err := json.Unmarshal(pinned, &rows); err != nil {
		t.Fatalf("decode pinned census: %v", err)
	}
	if len(rows) != count {
		t.Fatalf("pinned census rows = %d, want %d", len(rows), count)
	}
}

// TestEveryResidualIsAnUndeclaredOwnerStatement states the shape of what is
// left. A row that does not lower is waiting on a statement its owner has not
// made, never on a lowering this compiler has not written: no residual reports
// an unlowered authored fact, so the remaining work is declaration work.
func TestEveryResidualIsAnUndeclaredOwnerStatement(t *testing.T) {
	for _, row := range declared() {
		result := survey(t, row)
		if result.Status == statusCompiles {
			continue
		}
		if result.Reason != "undeclared" {
			t.Fatalf("%s reports residual %q; every residual is an undeclared owner statement", result.Family, result.Reason)
		}
	}
}
