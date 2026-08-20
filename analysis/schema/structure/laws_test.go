package structure_test

import (
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"testing"

	"github.com/wippyai/go-lua/domain/composite"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// The structural vocabulary is spelled once here and projected everywhere
// else. These laws state that agreement ordinal for ordinal: every spelling
// that survives the cut is a projection of the sealed table's dense ordinals,
// so a member added, removed, or reordered in one spelling and not in the
// declaration is a rejected build rather than a silent mistranslation.
//
// A verdict names the drifted spelling, because that is the only thing a
// reader has to change: the sealed table is the authority. The artifact
// ordinals are compiled from the Program at load and reach no byte stream of
// their own, so what these laws hold is the agreement between the spellings
// rather than a wire commitment - and agreement is what every projection
// across the boundary rests on.

func sealedVocabulary(t *testing.T) structure.Table {
	t.Helper()
	sealed, failure := composite.Table()
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

// pinned states that one foreign spelling's ordinal resolves to the sealed
// member it claims to spell.
func pinned(t *testing.T, table structure.Table, category structure.Category, ordinal uint16, key schema.Key, spelling string) {
	t.Helper()
	entry, ok := table.At(category, ordinal)
	if !ok {
		t.Fatalf("%s = %d names no member of the sealed vocabulary", spelling, ordinal)
	}
	if entry.Key() != key {
		t.Fatalf("%s = %d is the sealed member %q, not %q", spelling, ordinal, entry.Key(), key)
	}
}

// spelled states that one sealed member renders under the name its consumers
// are written against. A spelling is declared data, so a renamed member is a
// rejected build rather than a silently relabelled catalog.
func spelled(t *testing.T, table structure.Table, category structure.Category, ordinal uint16, spelling string) {
	t.Helper()
	entry, ok := table.At(category, ordinal)
	if !ok {
		t.Fatalf("ordinal %d names no member of the sealed vocabulary", ordinal)
	}
	if entry.Spelling() != spelling {
		t.Fatalf("sealed member %q renders as %q, not %q", entry.Key(), entry.Spelling(), spelling)
	}
}

// counted states that a spelling's last member is the vocabulary's last
// ordinal, so a member added to one spelling alone cannot hide above the
// declared catalog.
func counted(t *testing.T, table structure.Table, category structure.Category, last uint16, spelling string) {
	t.Helper()
	if int(last) != table.Count(category) {
		t.Fatalf("%s = %d, but the sealed vocabulary declares %d members", spelling, last, table.Count(category))
	}
}

// TestProgramVocabularyIsTheSealedTable pins the Program-owned spellings.
// The Program compiles these ordinals at load, so this law is what keeps its
// numbering and the declaration's the same numbering.
func TestProgramVocabularyIsTheSealedTable(t *testing.T) {
	table := sealedVocabulary(t)
	for _, member := range []struct {
		key      schema.Key
		ordinal  uint8
		spelling string
	}{
		{"event/enter", programschema.WTOEventEnter, "programschema.WTOEventEnter"},
		{"event/point", programschema.WTOEventPoint, "programschema.WTOEventPoint"},
		{"event/exit", programschema.WTOEventExit, "programschema.WTOEventExit"},
	} {
		pinned(t, table, structure.CategoryEvent, uint16(member.ordinal), member.key, member.spelling)
	}
	counted(t, table, structure.CategoryEvent, uint16(programschema.WTOEventExit), "programschema.WTOEventExit")

	for _, member := range []struct {
		key      schema.Key
		ordinal  programschema.OutcomeKind
		spelling string
	}{
		{"outcome/normal", programschema.OutcomeNormal, "programschema.OutcomeNormal"},
		{"outcome/return", programschema.OutcomeReturn, "programschema.OutcomeReturn"},
		{"outcome/throw", programschema.OutcomeThrow, "programschema.OutcomeThrow"},
		{"outcome/break", programschema.OutcomeBreak, "programschema.OutcomeBreak"},
		{"outcome/goto", programschema.OutcomeGoto, "programschema.OutcomeGoto"},
		{"outcome/yield", programschema.OutcomeYield, "programschema.OutcomeYield"},
		{"outcome/cancel", programschema.OutcomeCancel, "programschema.OutcomeCancel"},
	} {
		pinned(t, table, structure.CategoryOutcome, uint16(member.ordinal), member.key, member.spelling)
	}
	counted(t, table, structure.CategoryOutcome, uint16(programschema.OutcomeCancel), "programschema.OutcomeCancel")

	for _, member := range []struct {
		key      schema.Key
		spelling string
		ordinal  programschema.RuleStage
		foreign  string
	}{
		{"stage/base", "base", programschema.RuleStageBase, "programschema.RuleStageBase"},
		{"stage/local", "local", programschema.RuleStageLocal, "programschema.RuleStageLocal"},
		{"stage/call-dispatch", "call-dispatch", programschema.RuleStageCallDispatch, "programschema.RuleStageCallDispatch"},
		{"stage/call-summary", "call-summary", programschema.RuleStageCallSummary, "programschema.RuleStageCallSummary"},
		{"stage/call-effect", "call-effect", programschema.RuleStageCallEffect, "programschema.RuleStageCallEffect"},
	} {
		pinned(t, table, structure.CategoryIssuanceStage, uint16(member.ordinal), member.key, member.foreign)
		spelled(t, table, structure.CategoryIssuanceStage, uint16(member.ordinal), member.spelling)
	}
	counted(t, table, structure.CategoryIssuanceStage, uint16(programschema.RuleStageCallEffect), "programschema.RuleStageCallEffect")
}

// TestEngineArtifactVocabularyIsTheSealedTable pins the engine boundary
// spellings. They are the far end of the ingress projection, and the engine
// scalar template is written by ordinal, so a drift here is a mistranslated
// artifact rather than a rejected one.
func TestEngineArtifactVocabularyIsTheSealedTable(t *testing.T) {
	table := sealedVocabulary(t)
	for _, member := range []struct {
		key      schema.Key
		ordinal  rows.ArtifactStructuralArm
		spelling string
	}{
		{"arm/local", rows.ArtifactStructuralArmLocal, "rows.ArtifactStructuralArmLocal"},
		{"arm/resume", rows.ArtifactStructuralArmResume, "rows.ArtifactStructuralArmResume"},
		{"arm/select-true", rows.ArtifactStructuralArmTrue, "rows.ArtifactStructuralArmTrue"},
		{"arm/select-false", rows.ArtifactStructuralArmFalse, "rows.ArtifactStructuralArmFalse"},
		{"arm/tail", rows.ArtifactStructuralArmTail, "rows.ArtifactStructuralArmTail"},
		{"arm/throw", rows.ArtifactStructuralArmThrow, "rows.ArtifactStructuralArmThrow"},
		{"arm/yield", rows.ArtifactStructuralArmYield, "rows.ArtifactStructuralArmYield"},
		{"arm/cancel", rows.ArtifactStructuralArmCancel, "rows.ArtifactStructuralArmCancel"},
	} {
		pinned(t, table, structure.CategoryArm, uint16(member.ordinal), member.key, member.spelling)
	}
	counted(t, table, structure.CategoryArm, uint16(rows.ArtifactStructuralArmCancel), "rows.ArtifactStructuralArmCancel")

	for _, member := range []struct {
		key      schema.Key
		ordinal  rows.ArtifactEventKind
		spelling string
	}{
		{"event/enter", rows.ArtifactEventEnter, "rows.ArtifactEventEnter"},
		{"event/point", rows.ArtifactEventPoint, "rows.ArtifactEventPoint"},
		{"event/exit", rows.ArtifactEventExit, "rows.ArtifactEventExit"},
	} {
		pinned(t, table, structure.CategoryEvent, uint16(member.ordinal), member.key, member.spelling)
	}
	counted(t, table, structure.CategoryEvent, uint16(rows.ArtifactEventExit), "rows.ArtifactEventExit")

	for _, member := range []struct {
		key      schema.Key
		spelling string
		ordinal  rows.ArtifactRuleStage
		foreign  string
	}{
		{"stage/base", "base", rows.ArtifactRuleStageBase, "rows.ArtifactRuleStageBase"},
		{"stage/local", "local", rows.ArtifactRuleStageLocal, "rows.ArtifactRuleStageLocal"},
		{"stage/call-dispatch", "call-dispatch", rows.ArtifactRuleStageIssued3, "rows.ArtifactRuleStageIssued3"},
		{"stage/call-summary", "call-summary", rows.ArtifactRuleStageIssued4, "rows.ArtifactRuleStageIssued4"},
		{"stage/call-effect", "call-effect", rows.ArtifactRuleStageIssued5, "rows.ArtifactRuleStageIssued5"},
	} {
		pinned(t, table, structure.CategoryIssuanceStage, uint16(member.ordinal), member.key, member.foreign)
		spelled(t, table, structure.CategoryIssuanceStage, uint16(member.ordinal), member.spelling)
	}
	counted(t, table, structure.CategoryIssuanceStage, uint16(rows.ArtifactRuleStageIssued5), "rows.ArtifactRuleStageIssued5")
}

// TestIssuanceStagePredecessorIsTheSealedTable pins the native-call
// predecessor chain as declared structure data. Engine admission reads this
// relation rather than naming CallDispatch, CallSummary, and CallEffect.
func TestIssuanceStagePredecessorIsTheSealedTable(t *testing.T) {
	table := sealedVocabulary(t)
	stage := func(ordinal rows.ArtifactRuleStage) *structure.Entry {
		t.Helper()
		entry, ok := table.At(structure.CategoryIssuanceStage, uint16(ordinal))
		if !ok {
			t.Fatalf("issuance stage %d is not sealed", ordinal)
		}
		return entry
	}
	base, local := stage(rows.ArtifactRuleStageBase), stage(rows.ArtifactRuleStageLocal)
	dispatch, summary, effect := stage(rows.ArtifactRuleStageIssued3), stage(rows.ArtifactRuleStageIssued4), stage(rows.ArtifactRuleStageIssued5)
	if base.Native() || base.Predecessor().Available() {
		t.Fatalf("stage/base native=%v predecessor=%q", base.Native(), base.Predecessor())
	}
	if local.Native() || local.Predecessor().Available() {
		t.Fatalf("stage/local native=%v predecessor=%q", local.Native(), local.Predecessor())
	}
	if !dispatch.Native() || dispatch.Predecessor().Available() {
		t.Fatalf("stage/call-dispatch native=%v predecessor=%q", dispatch.Native(), dispatch.Predecessor())
	}
	if !summary.Native() || summary.Predecessor() != dispatch.Key() {
		t.Fatalf("stage/call-summary predecessor %q, want %q", summary.Predecessor(), dispatch.Key())
	}
	if !effect.Native() || effect.Predecessor() != summary.Key() {
		t.Fatalf("stage/call-effect predecessor %q, want %q", effect.Predecessor(), summary.Key())
	}
}

// TestDiagnosticSeverityVocabularyIsTheSealedTable pins the policy severity
// projection. Diagnostic observation identities are canonical structure
// declarations and therefore need no cross-package pin law.
func TestDiagnosticSeverityVocabularyIsTheSealedTable(t *testing.T) {
	table := sealedVocabulary(t)
	for _, member := range []struct {
		key      schema.Key
		spelling string
		ordinal  diagnostic.Severity
		foreign  string
	}{
		{"severity/error", "error", diagnostic.SeverityError, "diagnostic.SeverityError"},
		{"severity/warning", "warning", diagnostic.SeverityWarning, "diagnostic.SeverityWarning"},
		{"severity/hint", "hint", diagnostic.SeverityHint, "diagnostic.SeverityHint"},
	} {
		pinned(t, table, structure.CategoryDiagnosticSeverity, member.ordinal.Ordinal(), member.key, member.foreign)
		spelled(t, table, structure.CategoryDiagnosticSeverity, member.ordinal.Ordinal(), member.spelling)
	}
	counted(t, table, structure.CategoryDiagnosticSeverity, diagnostic.SeverityHint.Ordinal(), "diagnostic.SeverityHint")
}

// TestDiagnosticFamiliesAreResolvedByName states the other half: the family
// vocabulary is numbered by declaration order and no foreign spelling numbers
// it, so what a consumer holds is the member's name. Every published code's
// first segment is the declared spelling of the family its row names, which is
// what makes publishing under a new family one more declared row.
func TestDiagnosticFamiliesAreResolvedByName(t *testing.T) {
	table := sealedVocabulary(t)
	diagnostics, diagnosticsOK := composite.Diagnostics()
	if !diagnosticsOK {
		t.Fatal("sealed diagnostic table unavailable")
	}
	families := make(map[schema.Key]string, table.Count(structure.CategoryDiagnosticFamily))
	for ordinal := uint16(1); ordinal <= uint16(table.Count(structure.CategoryDiagnosticFamily)); ordinal++ {
		entry, ok := table.At(structure.CategoryDiagnosticFamily, ordinal)
		if !ok || entry.Spelling() == "" {
			t.Fatalf("family ordinal %d names no declared member", ordinal)
		}
		families[entry.Key()] = entry.Spelling()
	}
	for position := 0; position < diagnostics.Count(); position++ {
		row, rowOK := diagnostics.At(position)
		if !rowOK {
			t.Fatalf("diagnostic row %d is unavailable", position)
		}
		reference := row.Family()
		if reference.Surface != schema.SurfaceKindStructure {
			t.Fatalf("published code %q names its family on surface %d", row.Code(), reference.Surface)
		}
		spelling, declared := families[reference.Key]
		if !declared {
			t.Fatalf("published code %q names the undeclared family %q", row.Code(), reference.Key)
		}
		family, familyOK := row.Code().Family()
		if !familyOK || family != spelling {
			t.Fatalf("published code %q reads as family %q, but its row declares %q", row.Code(), family, spelling)
		}
	}
}
