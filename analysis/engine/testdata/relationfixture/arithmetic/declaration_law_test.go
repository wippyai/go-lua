package arithmetic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/check/typing"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

func TestDeclarationChecksWithExplicitThreeInputArithmeticABI(t *testing.T) {
	fixture := buildDeclaration(t)
	if !fixture.Schema.Available() {
		t.Fatal("arithmetic schema unavailable")
	}
	if len(fixture.Arithmetic.Inputs()) != 3 {
		t.Fatalf("arithmetic inputs=%d, want candidateAddr/sourceLeft/sourceRight", len(fixture.Arithmetic.Inputs()))
	}
	mapping := algebra.NewApplyContract(fixture.Arithmetic.Identity(), []algebra.SlotSource{
		algebra.NewSlotSource(0, 0),
		algebra.NewSlotSource(0, 2),
		algebra.NewSlotSource(0, 3),
	}, algebra.OwnerNamed()).SlotSource()
	if len(mapping) != 3 || mapping[0].Child() != 0 || mapping[1].Child() != 0 || mapping[2].Child() != 0 || mapping[0].Cell() != 0 || mapping[1].Cell() != 2 || mapping[2].Cell() != 3 {
		t.Fatalf("arithmetic slot mapping=%v, want one child with cells [0 2 3]", mapping)
	}
	cert, refusal := certificate.Check(fixture.Schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("arithmetic certificate: %v", refusal)
	}
}

func TestUnjoinedMultiRelationApplyRefusesWithoutCompositeCorrelation(t *testing.T) {
	fixture := buildDeclaration(t)
	entries := fixture.Schema.Expressions()
	if len(entries) != 1 {
		t.Fatalf("arithmetic expressions=%d", len(entries))
	}
	// Removing the Join leaves only the Candidate child. The three-slot ABI
	// still names Source columns, so the checker must refuse the declaration;
	// there is no worker-side predicate or runtime relation inference that can
	// repair this missing correlation.
	badApply := algebra.NewApply(
		[]algebra.Expression{algebra.NewInput(fixture.IDs.Candidate)},
		algebra.NewApplyContract(fixture.Arithmetic.Identity(), []algebra.SlotSource{
			algebra.NewSlotSource(0, 0),
			algebra.NewSlotSource(0, 2),
			algebra.NewSlotSource(0, 3),
		}, algebra.OwnerNamed()),
	)
	badExpression := algebra.NewPublish(badApply, algebra.NewPublishContract(fixture.IDs.Output, fixture.IDs.OutputKey))
	builder := plan.NewBuilder(fixture.Schema.SchemaID())
	for _, value := range fixture.Schema.Relations() {
		if !builder.AddRelation(value) {
			t.Fatal("copy relation")
		}
	}
	for _, value := range fixture.Schema.Columns() {
		if !builder.AddColumn(value) {
			t.Fatal("copy column")
		}
	}
	for _, value := range fixture.Schema.Keys() {
		if !builder.AddKey(value) {
			t.Fatal("copy key")
		}
	}
	for _, value := range fixture.Schema.Scopes() {
		if !builder.AddScope(value) {
			t.Fatal("copy scope")
		}
	}
	for _, value := range fixture.Schema.Signatures() {
		if !builder.AddSignature(value) {
			t.Fatal("copy signature")
		}
	}
	for _, value := range entries {
		if !builder.AddExpression(plan.DefineExpressionRef(value.ID(), badExpression)) {
			t.Fatal("replace expression")
		}
	}
	for _, value := range fixture.Schema.Dependencies() {
		if !builder.AddDependency(value) {
			t.Fatal("copy dependency")
		}
	}
	for _, value := range fixture.Schema.SCCs() {
		if !builder.AddSCC(value) {
			t.Fatal("copy scc")
		}
	}
	malformed, ok := builder.Build()
	if !ok {
		t.Fatal("build unjoined declaration")
	}
	report := typing.Check(malformed)
	if report.Valid() {
		t.Fatal("unjoined multi-relation Apply was accepted")
	}
	for _, issue := range report.Issues() {
		if issue.Code == typing.CodeMissingReference {
			return
		}
	}
	t.Fatalf("unjoined Apply lacked missing-column refusal: %v", report.Issues())
}
