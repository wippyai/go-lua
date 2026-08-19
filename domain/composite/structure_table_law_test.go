package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// TestStructureTableSeals states that the authored structural vocabulary is
// admitted and sealed by the one declaration root, and that it projects back at
// the ordinals it was declared under: the projection a consumer switches on is
// the authored table itself, never a restatement of it.
func TestStructureTableSeals(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("sealed table holds no structural surface")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("sealed structural vocabulary did not project")
	}
	// The sizes are the vocabularies the analyzer consolidates here: the eight
	// structural arms, the three bracket events, the seven body outcomes, the
	// eight Lua runtime families, the ten symbolic expression forms, the four
	// diagnostic observation populations, the five publication families, the
	// three severities, the thirty-three compiled occurrence families, the five
	// placement forms, the four operand polarities, the two declared operand
	// shapes, the five execution cuts, and
	// the ninety-two global semantic roles (including the runtime-kind result
	// and predicate behavior relations, observation geometry,
	// evidence-anchor, query population, and query projection roles).
	// They are stated independently of the authored inventory,
	// so a member added or dropped on either side is a verdict rather than a
	// table that agrees with itself.
	sizes := map[structure.Category]int{
		structure.CategoryArm:                   8,
		structure.CategoryEvent:                 3,
		structure.CategoryOutcome:               7,
		structure.CategoryRuntimeKind:           8,
		structure.CategoryConstraintForm:        10,
		structure.CategoryDiagnosticObservation: 4,
		structure.CategoryDiagnosticFamily:      5,
		structure.CategoryDiagnosticSeverity:    3,
		structure.CategoryOccurrenceKind:        30,
		structure.CategoryIssuanceForm:          5,
		structure.CategoryIssuanceInput:         4,
		structure.CategoryIssuanceRequirement:   2,
		structure.CategoryIssuanceStage:         5,
		structure.CategorySemanticRole:          94,
		// The native publication column vocabularies: three numeric carriers,
		// five exact scalar carriers, seven primitive arithmetic operators,
		// four unary operators, three divisor proofs, four truthiness classes,
		// four branch partitions, two conditional arms, and three overflow
		// disciplines.
		structure.CategoryNativeNumericRepresentation: 3,
		structure.CategoryNativeScalarRepresentation:  5,
		structure.CategoryNativeArithmeticOperator:    7,
		structure.CategoryNativeUnaryOperator:         4,
		structure.CategoryNativeDivisorProperty:       3,
		structure.CategoryNativeTruthinessClass:       4,
		structure.CategoryNativeBranchPartition:       4,
		structure.CategoryNativeBranchArm:             2,
		structure.CategoryNativeNumericOverflow:       3,
	}
	declared := 0
	for category, size := range sizes {
		if table.Count(category) != size {
			t.Fatalf("category %d projected %d of %d members", category, table.Count(category), size)
		}
		declared += size
	}
	if view.Count() != declared {
		t.Fatalf("sealed structural surface holds %d rows for %d vocabulary members", view.Count(), declared)
	}
	// A member is reachable at the position the aggregation numbers it, and a
	// contributor that authors an ordinal is naming that same position: a
	// category whose ordinals a foreign spelling is pinned to declares them, and
	// a category resolved by name alone leaves them to the aggregation.
	positions := make(map[structure.Category]uint16, len(sizes))
	for _, spec := range structureSpecs() {
		positions[spec.Category]++
		position := positions[spec.Category]
		if spec.Ordinal != 0 && spec.Ordinal != position {
			t.Fatalf("member %q authors ordinal %d at position %d of its category", spec.Key, spec.Ordinal, position)
		}
		member, memberOK := table.At(spec.Category, position)
		if !memberOK || member.Key() != spec.Key {
			t.Fatalf("member %q is not reachable at its declared ordinal", spec.Key)
		}
		if member.Spelling() != spec.Spelling {
			t.Fatalf("member %q renders as %q, declared %q", spec.Key, member.Spelling(), spec.Spelling)
		}
		if member.Native() != spec.Native || member.Predecessor() != spec.Predecessor {
			t.Fatalf("member %q native=%v predecessor=%q, declared native=%v predecessor=%q", spec.Key, member.Native(), member.Predecessor(), spec.Native, spec.Predecessor)
		}
	}
}

// TestStructureSurfaceSealsFirstInTheCatalog states the phase law for this
// surface: it hosts the closed identity vocabularies the rest of the catalog
// names members of, and it names no surface itself, so no surface precedes it
// and every surface above it may resolve against it.
func TestStructureSurfaceSealsFirstInTheCatalog(t *testing.T) {
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind < schema.SurfaceKindStructure {
			t.Fatalf("surface ordinal %d precedes the structural ordinal %d", kind, schema.SurfaceKindStructure)
		}
	}
}

// TestStructureTableIsHostedFromItsContributions states that the analyzer's
// vocabulary is an aggregate rather than one authored list: each contribution
// declares its own rows, the surface numbers the result densely per category,
// and the sealed table holds exactly what the contributions declared.
func TestStructureTableIsHostedFromItsContributions(t *testing.T) {
	contributions := structureContributions()
	if len(contributions) < 2 {
		t.Fatal("the structural vocabulary is hosted from a single contribution")
	}
	declared := 0
	for _, contribution := range contributions {
		if len(contribution) == 0 {
			t.Fatal("a contribution declares no rows")
		}
		declared += len(contribution)
	}
	entries, ok := structureEntries()
	if !ok || len(entries) != declared {
		t.Fatalf("the contributions collected %d rows of %d, ok=%t", len(entries), declared, ok)
	}
	counts := make(map[structure.Category]uint16, len(entries))
	for _, entry := range entries {
		counts[entry.Category()]++
		if entry.Ordinal() != counts[entry.Category()] {
			t.Fatalf("member %q holds ordinal %d at position %d of its category", entry.Key(), entry.Ordinal(), counts[entry.Category()])
		}
		if entry.Spelling() == "" {
			t.Fatalf("member %q was collected without a rendered name", entry.Key())
		}
	}
}

// TestStagedCutMembersDeclareOneFraming states the framing column over the
// authored table: a member that names a staged execution cut declares the
// digest framing that cut's points are derived under, and no other member
// declares one. The cut inventory is stated here independently of the table, so
// a framing added to a member that stages nothing, or withdrawn from one that
// does, is a verdict rather than a table that agrees with itself.
func TestStagedCutMembersDeclareOneFraming(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d", failure.Contributor, failure.Law)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("sealed table holds no structural surface")
	}
	staged := map[schema.Key]struct{}{
		"issuance/local": {}, "issuance/computation": {}, "issuance/local-predecessor": {},
		"stage/call-dispatch": {}, "stage/call-summary": {}, "stage/call-effect": {},
	}
	declared := make(map[string]schema.Key, len(staged))
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*structure.Entry)
		if !rowOK || !entryOK || entry == nil {
			t.Fatalf("structural row %d is not a vocabulary member", position)
		}
		_, stages := staged[entry.Key()]
		if stages != (entry.Framing() != "") {
			t.Fatalf("member %q stages a cut = %v but declares framing %q", entry.Key(), stages, entry.Framing())
		}
		if !stages {
			continue
		}
		if prior, duplicate := declared[entry.Framing()]; duplicate {
			t.Fatalf("members %q and %q declare one framing %q", prior, entry.Key(), entry.Framing())
		}
		declared[entry.Framing()] = entry.Key()
	}
	if len(declared) != len(staged) {
		t.Fatalf("the table declares %d staged-cut framings, the cut inventory holds %d", len(declared), len(staged))
	}
}
