package constraint_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/constraint"
	"github.com/wippyai/go-lua/domain/constraint/expr"
)

// The symbolic expression form vocabulary is declared once and projected
// everywhere else. These laws state that the sealed table is the grammar's own
// closed enumeration, form for form: a form the grammar builds and the table
// does not declare, or a row whose ordinal names a different form than the one
// it declares, is a rejected build rather than a silent mistranslation.

func sealedForms(t *testing.T) structure.Table {
	t.Helper()
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("sealed compilation unavailable")
	}
	sealed, failure := composite.Table(compilation)
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("sealed table holds no structural vocabulary")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("sealed structural vocabulary did not project")
	}
	return table
}

// TestSealedVocabularyIsTheClosedGrammar states the declaration is the whole
// grammar and nothing besides it: every form the enumeration admits is declared
// at its own ordinal, and the sealed catalog holds no member the grammar does
// not build.
func TestSealedVocabularyIsTheClosedGrammar(t *testing.T) {
	table := sealedForms(t)
	declared := table.Count(structure.CategoryConstraintForm)
	if declared != expr.FormCount {
		t.Fatalf("sealed expression form vocabulary declares %d members, but the closed grammar has %d", declared, expr.FormCount)
	}
	for _, form := range expr.Forms() {
		entry, ok := table.At(structure.CategoryConstraintForm, uint16(form))
		if !ok {
			t.Fatalf("grammar form %d names no member of the sealed vocabulary", form)
		}
		declaredForm, formOK := constraint.FormFor(entry)
		if !formOK || declaredForm != form {
			t.Fatalf("sealed member %q at ordinal %d declares form %d, not %d", entry.Key(), entry.Ordinal(), declaredForm, form)
		}
		if !entry.Accepted() {
			t.Fatalf("sealed member %q is held back from the projection its vocabulary feeds", entry.Key())
		}
	}
}

// TestDeclaredFormNamesAreTheOneSpelling states that a row's key is the form's
// name on this surface, so a consumer that needs the name of a form reads it
// from the sealed table rather than from a list of its own.
func TestDeclaredFormNamesAreTheOneSpelling(t *testing.T) {
	table := sealedForms(t)
	seen := make(map[schema.Key]expr.Form, table.Count(structure.CategoryConstraintForm))
	for _, form := range expr.Forms() {
		entry, ok := table.At(structure.CategoryConstraintForm, uint16(form))
		if !ok {
			t.Fatalf("grammar form %d names no member of the sealed vocabulary", form)
		}
		if !entry.Key().Available() || entry.Key() != constraint.FormKey(form) {
			t.Fatalf("grammar form %d is sealed as %q, but the domain names it %q", form, entry.Key(), constraint.FormKey(form))
		}
		if prior, duplicate := seen[entry.Key()]; duplicate {
			t.Fatalf("forms %d and %d are both declared as %q", prior, form, entry.Key())
		}
		seen[entry.Key()] = form
	}
}

// TestForeignRowsAreNotConstraintForms states the projection is exact in the
// other direction: a member of another structural vocabulary does not answer as
// a grammar form, so a consumer cannot read one catalog through the other.
func TestForeignRowsAreNotConstraintForms(t *testing.T) {
	table := sealedForms(t)
	for _, category := range []structure.Category{
		structure.CategoryArm, structure.CategoryEvent,
		structure.CategoryOutcome, structure.CategoryRuntimeKind,
	} {
		for ordinal := 1; ordinal <= table.Count(category); ordinal++ {
			entry, ok := table.At(category, uint16(ordinal))
			if !ok {
				t.Fatalf("structural vocabulary %d holds no member at ordinal %d", category, ordinal)
			}
			if form, formOK := constraint.FormFor(entry); formOK {
				t.Fatalf("sealed member %q of vocabulary %d answered as grammar form %d", entry.Key(), category, form)
			}
		}
	}
}

// TestAuthoredSpecsCarryTheGrammarOrdinals states the declaration adopts the
// grammar's numbering rather than inventing one beside it: the authored row set
// is dense from the grammar's first form and carries no member the enumeration
// does not hold.
func TestAuthoredSpecsCarryTheGrammarOrdinals(t *testing.T) {
	specs := constraint.StructureSpecs()
	if len(specs) != expr.FormCount {
		t.Fatalf("the domain declares %d rows for a grammar of %d forms", len(specs), expr.FormCount)
	}
	for position, spec := range specs {
		form := expr.Forms()[position]
		if spec.Category != structure.CategoryConstraintForm {
			t.Fatalf("row %q is declared under vocabulary %d", spec.Key, spec.Category)
		}
		if spec.Ordinal != uint16(form) {
			t.Fatalf("row %q carries ordinal %d, but its grammar form is %d", spec.Key, spec.Ordinal, form)
		}
		if !spec.Accepted || !spec.Key.Available() {
			t.Fatalf("row for grammar form %d is declared incompletely", form)
		}
	}
}
