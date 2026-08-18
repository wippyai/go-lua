package composite

import (
	"github.com/wippyai/go-lua/analysis/engine/rows"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// ArtifactIssuanceDirectory projects the sealed rule.Issues catalog into the
// cold directory the Program artifact compiler places from. Link-lane rules
// are omitted: they are admitted through the Link table.
func ArtifactIssuanceDirectory() (programartifact.IssuanceDirectory, bool) {
	sealRegistry()
	if registry.sealed == nil {
		return nil, false
	}
	var directory programartifact.IssuanceDirectory
	for _, entry := range registry.templates {
		if entry == nil || entry.Lane() == rule.LaneLink {
			continue
		}
		if !entry.Key().Available() {
			return nil, false
		}
		for index := 0; index < entry.IssuanceCount(); index++ {
			issued, ok := entry.IssuanceAt(index)
			if !ok {
				return nil, false
			}
			placement, ok := issuancePlacement(issued, entry.Key())
			if !ok {
				return nil, false
			}
			directory = append(directory, placement)
		}
	}
	return directory, true
}

func issuancePlacement(issued rule.Issuance, key schema.Key) (programartifact.IssuancePlacement, bool) {
	occurrence, occurrenceOK := structureOrdinal(issued.Occurrence, structure.CategoryOccurrenceKind)
	form, formOK := structureOrdinal(issued.Form, structure.CategoryIssuanceForm)
	input, inputOK := structureOrdinal(issued.Input, structure.CategoryIssuanceInput)
	stage, stageOK := structureOrdinal(issued.Stage, structure.CategoryIssuanceStage)
	if !occurrenceOK || !formOK || !inputOK || !stageOK {
		return programartifact.IssuancePlacement{}, false
	}
	placement := programartifact.IssuancePlacement{
		Occurrence: programartifact.OccurrenceKind(occurrence),
		Form:       programartifact.IssuanceForm(form),
		Input:      programartifact.RuleInputKind(input),
		Stage:      programartifact.RuleStage(stage),
		Code:       issued.Code,
		HasCode:    issued.HasCode,
		Key:        key,
	}
	return placement, placement.Available()
}

// IssuanceStageLaws is the sealed native-call predecessor relation projected
// onto the engine scalar stage ordinals.
func IssuanceStageLaws() ([]rows.ArtifactStageLaw, bool) {
	sealRegistry()
	if registry.sealed == nil {
		return nil, false
	}
	view, viewOK := registry.sealed.Surface(schema.SurfaceKindStructure)
	table, tableOK := structure.NewTable(view)
	if !viewOK || !tableOK {
		return nil, false
	}
	count := table.Count(structure.CategoryIssuanceStage)
	byKey := make(map[schema.Key]rows.ArtifactRuleStage, count)
	for ordinal := uint16(1); int(ordinal) <= count; ordinal++ {
		entry, ok := table.At(structure.CategoryIssuanceStage, ordinal)
		if !ok || !entry.Key().Available() {
			return nil, false
		}
		byKey[entry.Key()] = rows.ArtifactRuleStage(entry.Ordinal())
	}
	var laws []rows.ArtifactStageLaw
	for ordinal := uint16(1); int(ordinal) <= count; ordinal++ {
		entry, ok := table.At(structure.CategoryIssuanceStage, ordinal)
		if !ok {
			return nil, false
		}
		if !entry.Native() && !entry.Predecessor().Available() {
			continue
		}
		law := rows.ArtifactStageLaw{Stage: rows.ArtifactRuleStage(entry.Ordinal()), Native: entry.Native()}
		if entry.Predecessor().Available() {
			predecessor, predecessorOK := byKey[entry.Predecessor()]
			if !predecessorOK {
				return nil, false
			}
			law.Predecessor = predecessor
		}
		if !law.Valid() {
			return nil, false
		}
		laws = append(laws, law)
	}
	return laws, len(laws) > 0
}

func structureOrdinal(key schema.Key, category structure.Category) (uint16, bool) {
	view, viewOK := registry.sealed.Surface(schema.SurfaceKindStructure)
	table, tableOK := structure.NewTable(view)
	if !viewOK || !tableOK {
		return 0, false
	}
	for ordinal := uint16(1); int(ordinal) <= table.Count(category); ordinal++ {
		entry, ok := table.At(category, ordinal)
		if ok && entry.Key() == key {
			return entry.Ordinal(), true
		}
	}
	return 0, false
}
