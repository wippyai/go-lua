package structure_test

import (
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// The structural vocabulary is spelled once here and projected everywhere
// else. These laws state that agreement ordinal for ordinal: every spelling
// that survives the cut is a projection of the sealed table's dense ordinals,
// so a member added, removed, or reordered in one spelling and not in the
// declaration is a rejected build rather than a silent mistranslation.
//
// A verdict names the drifted spelling, because that is the only thing a
// reader has to change: the sealed table is the authority, and the artifact
// ordinals it adopts are serialized ABI.

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

// counted states that a spelling's last member is the vocabulary's last
// ordinal, so a member added to one spelling alone cannot hide above the
// declared catalog.
func counted(t *testing.T, table structure.Table, category structure.Category, last uint16, spelling string) {
	t.Helper()
	if int(last) != table.Count(category) {
		t.Fatalf("%s = %d, but the sealed vocabulary declares %d members", spelling, last, table.Count(category))
	}
}

// TestArtifactVocabularyIsTheSealedTable pins the artifact-owned spellings.
// These ordinals are serialized ABI, so the declaration adopts them and this
// law is the proof it still does.
func TestArtifactVocabularyIsTheSealedTable(t *testing.T) {
	table := sealedVocabulary(t)
	for _, member := range []struct {
		key      schema.Key
		ordinal  programartifact.RouteKind
		spelling string
	}{
		{"arm/local", programartifact.RouteLocal, "programartifact.RouteLocal"},
		{"arm/resume", programartifact.RouteResume, "programartifact.RouteResume"},
		{"arm/select-true", programartifact.RouteSelectTrue, "programartifact.RouteSelectTrue"},
		{"arm/select-false", programartifact.RouteSelectFalse, "programartifact.RouteSelectFalse"},
		{"arm/tail", programartifact.RouteTail, "programartifact.RouteTail"},
		{"arm/throw", programartifact.RouteThrow, "programartifact.RouteThrow"},
		{"arm/yield", programartifact.RouteYield, "programartifact.RouteYield"},
		{"arm/cancel", programartifact.RouteCancel, "programartifact.RouteCancel"},
	} {
		pinned(t, table, structure.CategoryArm, uint16(member.ordinal), member.key, member.spelling)
	}
	counted(t, table, structure.CategoryArm, uint16(programartifact.RouteCancel), "programartifact.RouteCancel")

	for _, member := range []struct {
		key      schema.Key
		ordinal  programartifact.WTOEventKind
		spelling string
	}{
		{"event/enter", programartifact.WTOEventEnter, "programartifact.WTOEventEnter"},
		{"event/point", programartifact.WTOEventPoint, "programartifact.WTOEventPoint"},
		{"event/exit", programartifact.WTOEventExit, "programartifact.WTOEventExit"},
	} {
		pinned(t, table, structure.CategoryEvent, uint16(member.ordinal), member.key, member.spelling)
	}
	counted(t, table, structure.CategoryEvent, uint16(programartifact.WTOEventExit), "programartifact.WTOEventExit")

	for _, member := range []struct {
		key      schema.Key
		ordinal  programartifact.OutcomeKind
		spelling string
	}{
		{"outcome/normal", programartifact.OutcomeNormal, "programartifact.OutcomeNormal"},
		{"outcome/return", programartifact.OutcomeReturn, "programartifact.OutcomeReturn"},
		{"outcome/throw", programartifact.OutcomeThrow, "programartifact.OutcomeThrow"},
		{"outcome/break", programartifact.OutcomeBreak, "programartifact.OutcomeBreak"},
		{"outcome/goto", programartifact.OutcomeGoto, "programartifact.OutcomeGoto"},
		{"outcome/yield", programartifact.OutcomeYield, "programartifact.OutcomeYield"},
		{"outcome/cancel", programartifact.OutcomeCancel, "programartifact.OutcomeCancel"},
	} {
		pinned(t, table, structure.CategoryOutcome, uint16(member.ordinal), member.key, member.spelling)
	}
	counted(t, table, structure.CategoryOutcome, uint16(programartifact.OutcomeCancel), "programartifact.OutcomeCancel")
}

// TestIngressVocabularyIsTheSealedTable pins the ingress boundary spellings.
// Ingress projects an artifact row's ordinal through the sealed table, so this
// law is what makes that projection an identity rather than a translation.
func TestIngressVocabularyIsTheSealedTable(t *testing.T) {
	table := sealedVocabulary(t)
	for _, member := range []struct {
		key      schema.Key
		ordinal  ingress.StructuralArm
		spelling string
	}{
		{"arm/local", ingress.StructuralArmLocal, "ingress.StructuralArmLocal"},
		{"arm/resume", ingress.StructuralArmResume, "ingress.StructuralArmResume"},
		{"arm/select-true", ingress.StructuralArmTrue, "ingress.StructuralArmTrue"},
		{"arm/select-false", ingress.StructuralArmFalse, "ingress.StructuralArmFalse"},
		{"arm/tail", ingress.StructuralArmTail, "ingress.StructuralArmTail"},
		{"arm/throw", ingress.StructuralArmThrow, "ingress.StructuralArmThrow"},
		{"arm/yield", ingress.StructuralArmYield, "ingress.StructuralArmYield"},
		{"arm/cancel", ingress.StructuralArmCancel, "ingress.StructuralArmCancel"},
	} {
		pinned(t, table, structure.CategoryArm, uint16(member.ordinal), member.key, member.spelling)
	}
	counted(t, table, structure.CategoryArm, uint16(ingress.StructuralArmCancel), "ingress.StructuralArmCancel")

	for _, member := range []struct {
		key      schema.Key
		ordinal  ingress.EventKind
		spelling string
	}{
		{"event/enter", ingress.EventEnter, "ingress.EventEnter"},
		{"event/point", ingress.EventPoint, "ingress.EventPoint"},
		{"event/exit", ingress.EventExit, "ingress.EventExit"},
	} {
		pinned(t, table, structure.CategoryEvent, uint16(member.ordinal), member.key, member.spelling)
	}
	counted(t, table, structure.CategoryEvent, uint16(ingress.EventExit), "ingress.EventExit")
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
}
