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
	if verdict := translationLaw(table, structure.CategoryIssuanceStage, "engineArtifactRuleStage", engineArtifactRuleStage); verdict != "" {
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

	swappedStage := func(stage programartifact.RuleStage) (rows.ArtifactRuleStage, bool) {
		translated, admitted := engineArtifactRuleStage(stage)
		switch translated {
		case rows.ArtifactRuleStageCallDispatch:
			return rows.ArtifactRuleStageCallSummary, admitted
		case rows.ArtifactRuleStageCallSummary:
			return rows.ArtifactRuleStageCallDispatch, admitted
		default:
			return translated, admitted
		}
	}
	verdict = translationLaw(table, structure.CategoryIssuanceStage, "engineArtifactRuleStage", swappedStage)
	if !strings.Contains(verdict, "stage/call-dispatch") || !strings.Contains(verdict, "stage/call-summary") {
		t.Fatalf("an exchanged execution cut mapping reads as %q, which names neither exchanged member", verdict)
	}

	droppedStage := func(stage programartifact.RuleStage) (rows.ArtifactRuleStage, bool) {
		if stage == programartifact.RuleStageCallEffect {
			return rows.ArtifactRuleStageInvalid, false
		}
		return engineArtifactRuleStage(stage)
	}
	verdict = translationLaw(table, structure.CategoryIssuanceStage, "engineArtifactRuleStage", droppedStage)
	if !strings.Contains(verdict, "stage/call-effect") {
		t.Fatalf("a mapping that drops one execution cut reads as %q, which does not name the sealed member left uncrossed", verdict)
	}
}
