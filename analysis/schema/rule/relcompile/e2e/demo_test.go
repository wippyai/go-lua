package e2e

import (
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	valuearithmetic "github.com/wippyai/go-lua/domain/value/arithmetic/program"
	valuebootstrap "github.com/wippyai/go-lua/domain/value/bootstrap"
	valuesource "github.com/wippyai/go-lua/domain/value/source"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// fixture is the Lua program the demo carries end to end. Its two annotated
// functions and three call sites are the shortest program in the corpus whose
// answer depends on the whole value chain: a source seed, the global
// bootstrap, and the binary arithmetic reduction over both.
const fixture = "basic/arithmetic"

// family is one authored declaration of the chain under demonstration.
type family struct {
	Family string
	Spec   rule.Spec
}

// chain is the value families the fixture's answer is derived by, in the order
// their rows are produced.
func chain() []family {
	return []family{
		{"value/source", valuesource.RuleEntry()},
		{"value/bootstrap", valuebootstrap.RuleEntry()},
		{"value/arithmetic", valuearithmetic.RuleEntry()},
	}
}

// TestFixtureCompilesToItsArtifact is stage one: the fixture the demo answers
// for is a real corpus project that seals and compiles.
func TestFixtureCompilesToItsArtifact(t *testing.T) {
	artifact := compileFixture(t)
	if artifact == nil {
		t.Fatal("the fixture produced no artifact")
	}
}

// TestValueChainLowersToOneExecutionSchema is stage two: the three authored
// declarations the fixture's answer depends on resolve against one declaration
// surface and lower into one immutable logical schema. One schema and not
// three, because the arithmetic reduction reads the rows the seeds publish.
func TestValueChainLowersToOneExecutionSchema(t *testing.T) {
	compiled, _ := lowerValueChain(t)
	if len(compiled.Expressions()) != len(chain()) {
		t.Fatalf("value chain lowered to %d expressions, want %d", len(compiled.Expressions()), len(chain()))
	}
}

// TestEveryFamilyOfTheChainIsAdmittedByTheChecker is stage three read one
// family at a time, so the gate names the exact declaration whose lowered plan
// the checker will not admit rather than one refusal set over three families.
func TestEveryFamilyOfTheChainIsAdmittedByTheChecker(t *testing.T) {
	for _, family := range chain() {
		compiled, _ := lowerFamilies(t, family)
		if _, refusal := certificate.Check(compiled); refusal != nil {
			t.Errorf("the checker refused %s: %v", family.Family, refusal.Error())
		}
	}
}

// TestValueChainIsAdmittedByTheChecker is stage three: the lowered schema is
// admitted by the independent certificate checker, which is the only authority
// mount accepts.
func TestValueChainIsAdmittedByTheChecker(t *testing.T) {
	compiled, _ := lowerValueChain(t)
	cert, refusal := certificate.Check(compiled)
	if refusal != nil {
		t.Fatalf("the checker refused the lowered value chain: %v", refusal.Error())
	}
	if !cert.Available() {
		t.Fatal("the checker admitted the value chain without a certificate")
	}
}

// compileFixture seals the corpus project and compiles it through a private
// workspace, exactly as cmd/solvedump does before it solves.
func compileFixture(t *testing.T) *analysis.Plan {
	t.Helper()
	root, err := testfixture.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}
	corpus, err := testfixture.LoadCorpus(root)
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	project, err := corpus.Project(fixture)
	if err != nil {
		t.Fatalf("open fixture %s: %v", fixture, err)
	}
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("seal standard library target: %v", err)
	}
	linked, err := testfixture.SealCorpusProject(target, project)
	if err != nil {
		t.Fatalf("seal fixture %s: %v", fixture, err)
	}
	compiled, status, diagnostics := analysis.NewWorkspace().CompileWithDiagnostics(linked)
	if status != analysis.CompileComplete {
		t.Fatalf("compile fixture %s: status %v, diagnostics %v", fixture, status, diagnostics)
	}
	return compiled
}

// lowerValueChain resolves every family of the chain against one declaration
// surface and lowers them into one execution schema.
func lowerValueChain(t *testing.T) (plan.ExecutionSchema, *surface) {
	t.Helper()
	return lowerFamilies(t, chain()...)
}

// lowerFamilies resolves the named families against one declaration surface
// and lowers them into one execution schema.
func lowerFamilies(t *testing.T, families ...family) (plan.ExecutionSchema, *surface) {
	t.Helper()
	axis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: families[0].Spec.Writes}
	surfaces := newSurface(t, axis)
	placements := make([]relcompile.Placement, 0, len(families))
	for _, row := range families {
		placements = append(placements, surfaces.declare(row.Spec))
	}
	for _, row := range families {
		surfaces.seal(row.Spec)
	}
	rules := make([]relcompile.Rule, 0, len(families))
	for index, row := range families {
		lowered, err := relcompile.Resolve(surfaces.registry, row.Spec, placements[index])
		if err != nil {
			t.Fatalf("resolve %s: %v", row.Family, err)
		}
		rules = append(rules, lowered.Rules...)
	}
	declaration := surfaces.registry.Declaration(surfaces.schemaID)
	declaration.Rules = rules
	compiled, err := relcompile.Compile(declaration)
	if err != nil {
		t.Fatalf("compile the value chain: %v", err)
	}
	return compiled, surfaces
}
