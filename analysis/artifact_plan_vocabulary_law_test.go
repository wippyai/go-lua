package analysis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/rows"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
)

// The plan hands the engine its three artifact vocabularies through hand
// switches. Every spelling those switches read and write is pinned ordinal for
// ordinal to the sealed structural table by the vocabulary pin laws, so a
// switch that carries an ordinal through unchanged carries the sealed member
// through unchanged. These laws state exactly that: each switch is total over
// the sealed vocabulary, admits nothing outside it, and translates every member
// to itself - the identity on ordinals, which is a bijection over the sealed
// members. A verdict names the sealed member that drifted, because that member
// is the one thing a reader has to look at.
//
// The switches themselves are not read here. The laws take the translation as
// an argument, so the same law body states the agreement for the engine's
// mapping and rejects a deliberately swapped copy of it.

func sealedStructuralTable(t *testing.T) structure.Table {
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

// memberName renders one ordinal as the sealed member it names, so a verdict
// reads as a member rather than as a number.
func memberName(table structure.Table, category structure.Category, ordinal uint16) string {
	entry, ok := table.At(category, ordinal)
	if !ok {
		return fmt.Sprintf("the undeclared ordinal %d", ordinal)
	}
	return fmt.Sprintf("%q", string(entry.Key()))
}

// translationLaw states that one switch is a bijection over a sealed
// vocabulary: it admits an ordinal exactly when the sealed table declares one
// there, and the ordinal it writes is the ordinal it read. It returns the empty
// string when the law holds and the verdict naming the drifted member when it
// does not.
func translationLaw[In ~uint8, Out ~uint8](table structure.Table, category structure.Category, switchName string, translate func(In) (Out, bool)) string {
	count := table.Count(category)
	if count == 0 {
		return fmt.Sprintf("the sealed vocabulary read by %s declares no members", switchName)
	}
	for value := 0; value <= int(^uint8(0)); value++ {
		translated, admitted := translate(In(value))
		member, declared := table.At(category, uint16(value))
		if !declared {
			if admitted {
				return fmt.Sprintf("%s admits the undeclared ordinal %d as %s", switchName, value, memberName(table, category, uint16(translated)))
			}
			continue
		}
		if !admitted {
			return fmt.Sprintf("the sealed member %q does not cross %s", string(member.Key()), switchName)
		}
		if uint16(translated) != uint16(value) {
			return fmt.Sprintf("%s translates the sealed member %q to %s", switchName, string(member.Key()), memberName(table, category, uint16(translated)))
		}
	}
	return ""
}

// stageTranslationLaw states the same agreement for the two-sided execution cut
// switch. The cut a rule is placed at is admitted through its role, so the law
// is over both vocabularies at once: every mounted role crosses at exactly one
// sealed cut and carries it through unchanged, no role the mounted vocabulary
// excludes crosses at all, and every sealed cut is claimed by some mounted
// role.
func stageTranslationLaw(table structure.Table, translate func(programartifact.RuleRole, programartifact.RuleStage) (rows.ArtifactRuleStage, bool)) string {
	const switchName = "engineArtifactRuleStage"
	count := table.Count(structure.CategoryIssuanceStage)
	if count == 0 {
		return "the sealed execution cut vocabulary declares no members"
	}
	mounted := make(map[programartifact.RuleRole]bool, programartifact.MountedRuleRoleCount())
	for index := 0; index < programartifact.MountedRuleRoleCount(); index++ {
		role, ok := programartifact.MountedRuleRoleAt(index)
		if !ok {
			return fmt.Sprintf("the mounted rule role at position %d is unavailable", index)
		}
		mounted[role] = true
	}
	claimed := make(map[uint16]bool, count)
	for value := 0; value <= int(^uint8(0)); value++ {
		role := programartifact.RuleRole(value)
		crossed := uint16(0)
		for ordinal := 0; ordinal <= count+1; ordinal++ {
			translated, admitted := translate(role, programartifact.RuleStage(ordinal))
			if !admitted {
				continue
			}
			member, declared := table.At(structure.CategoryIssuanceStage, uint16(ordinal))
			if !declared {
				return fmt.Sprintf("%s admits the undeclared execution cut %d for rule role %d", switchName, ordinal, value)
			}
			if uint16(translated) != uint16(ordinal) {
				return fmt.Sprintf("%s translates the sealed member %q to %s", switchName, string(member.Key()), memberName(table, structure.CategoryIssuanceStage, uint16(translated)))
			}
			if crossed != 0 {
				return fmt.Sprintf("rule role %d crosses %s at both %s and %q", value, switchName, memberName(table, structure.CategoryIssuanceStage, crossed), string(member.Key()))
			}
			crossed = uint16(ordinal)
		}
		switch {
		case mounted[role] && crossed == 0:
			return fmt.Sprintf("the mounted rule role %d crosses %s at no sealed execution cut", value, switchName)
		case !mounted[role] && crossed != 0:
			return fmt.Sprintf("the unmounted rule role %d crosses %s at %s", value, switchName, memberName(table, structure.CategoryIssuanceStage, crossed))
		}
		if crossed != 0 {
			claimed[crossed] = true
		}
	}
	for ordinal := uint16(1); ordinal <= uint16(count); ordinal++ {
		if !claimed[ordinal] {
			return fmt.Sprintf("no mounted rule role crosses %s at the sealed member %s", switchName, memberName(table, structure.CategoryIssuanceStage, ordinal))
		}
	}
	return ""
}

// TestEngineArtifactVocabularySwitchesAreSealedBijections states the agreement
// the engine hand-off rests on: the plan's three switches carry the sealed
// vocabularies across unchanged.
func TestEngineArtifactVocabularySwitchesAreSealedBijections(t *testing.T) {
	table := sealedStructuralTable(t)
	if verdict := translationLaw(table, structure.CategoryArm, "engineStructuralArm", engineStructuralArm); verdict != "" {
		t.Fatal(verdict)
	}
	if verdict := translationLaw(table, structure.CategoryEvent, "engineEventKind", engineEventKind); verdict != "" {
		t.Fatal(verdict)
	}
	if verdict := stageTranslationLaw(table, engineArtifactRuleStage); verdict != "" {
		t.Fatal(verdict)
	}
}

// TestEngineArtifactVocabularyLawsNameADriftedMember states that the laws are
// the drift verdict they claim to be. Each probe is a copy of one mapping with
// two arms exchanged, and the law has to name the sealed member the copy
// mistranslates.
func TestEngineArtifactVocabularyLawsNameADriftedMember(t *testing.T) {
	table := sealedStructuralTable(t)

	swappedArm := func(arm ingress.StructuralArm) (rows.ArtifactStructuralArm, bool) {
		switch arm {
		case ingress.StructuralArmTrue:
			return rows.ArtifactStructuralArmFalse, true
		case ingress.StructuralArmFalse:
			return rows.ArtifactStructuralArmTrue, true
		default:
			return engineStructuralArm(arm)
		}
	}
	verdict := translationLaw(table, structure.CategoryArm, "engineStructuralArm", swappedArm)
	if !strings.Contains(verdict, "arm/select-true") || !strings.Contains(verdict, "arm/select-false") {
		t.Fatalf("an exchanged arm mapping reads as %q, which names neither exchanged member", verdict)
	}

	swappedEvent := func(kind ingress.EventKind) (rows.ArtifactEventKind, bool) {
		switch kind {
		case ingress.EventEnter:
			return rows.ArtifactEventExit, true
		case ingress.EventExit:
			return rows.ArtifactEventEnter, true
		default:
			return engineEventKind(kind)
		}
	}
	verdict = translationLaw(table, structure.CategoryEvent, "engineEventKind", swappedEvent)
	if !strings.Contains(verdict, "event/enter") || !strings.Contains(verdict, "event/exit") {
		t.Fatalf("an exchanged event mapping reads as %q, which names neither exchanged member", verdict)
	}

	swappedStage := func(role programartifact.RuleRole, stage programartifact.RuleStage) (rows.ArtifactRuleStage, bool) {
		translated, admitted := engineArtifactRuleStage(role, stage)
		switch translated {
		case rows.ArtifactRuleStageCallDispatch:
			return rows.ArtifactRuleStageCallSummary, admitted
		case rows.ArtifactRuleStageCallSummary:
			return rows.ArtifactRuleStageCallDispatch, admitted
		default:
			return translated, admitted
		}
	}
	verdict = stageTranslationLaw(table, swappedStage)
	if !strings.Contains(verdict, "stage/call-dispatch") || !strings.Contains(verdict, "stage/call-summary") {
		t.Fatalf("an exchanged execution cut mapping reads as %q, which names neither exchanged member", verdict)
	}

	droppedStage := func(role programartifact.RuleRole, stage programartifact.RuleStage) (rows.ArtifactRuleStage, bool) {
		if stage == programartifact.RuleStageCallEffect {
			return rows.ArtifactRuleStageInvalid, false
		}
		return engineArtifactRuleStage(role, stage)
	}
	verdict = stageTranslationLaw(table, droppedStage)
	stalled := fmt.Sprintf("the mounted rule role %d crosses engineArtifactRuleStage at no sealed execution cut", programartifact.RuleRoleEffectSelected)
	if verdict != stalled {
		t.Fatalf("a mapping that drops one execution cut reads as %q, which does not name the role left carrying none", verdict)
	}
}
