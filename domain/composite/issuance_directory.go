package composite

import (
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// ArtifactIssuanceDirectory projects the sealed rule.Issues catalog into the
// cold directory the Program artifact compiler places from. Link-lane rules
// are omitted: they are admitted through the Link table.
//
// Each row carries the operand shape its rule declared, so the compiler places
// the rows an owner can seal an operand for and no others: the placement set
// and the owner-issued operand set are one denominator declared once.
func ArtifactIssuanceDirectory() (artifactcompiler.IssuanceDirectory, bool) {
	sealRegistry()
	if registry.sealed == nil {
		return artifactcompiler.IssuanceDirectory{}, false
	}
	var placements []artifactcompiler.IssuancePlacement
	for _, entry := range registry.templates {
		if entry == nil || entry.Lane() == rule.LaneLink {
			continue
		}
		if !entry.Key().Available() {
			return artifactcompiler.IssuanceDirectory{}, false
		}
		for index := 0; index < entry.IssuanceCount(); index++ {
			issued, ok := entry.IssuanceAt(index)
			if !ok {
				return artifactcompiler.IssuanceDirectory{}, false
			}
			placement, ok := issuancePlacement(issued, entry.Key(), entry.Writes(), entry.Lane() == rule.LaneMounted)
			if !ok {
				return artifactcompiler.IssuanceDirectory{}, false
			}
			placements = append(placements, placement)
		}
	}
	forms, formsOK := structureFramings(structure.CategoryIssuanceForm)
	stages, stagesOK := structureFramings(structure.CategoryIssuanceStage)
	if !formsOK || !stagesOK {
		return artifactcompiler.IssuanceDirectory{}, false
	}
	formFraming := make(map[artifactcompiler.IssuanceForm]string, len(forms))
	for ordinal, framing := range forms {
		formFraming[artifactcompiler.IssuanceForm(ordinal)] = framing
	}
	stageFraming := make(map[programschema.RuleStage]string, len(stages))
	for ordinal, framing := range stages {
		stageFraming[programschema.RuleStage(ordinal)] = framing
	}
	return artifactcompiler.NewIssuanceDirectory(placements, formFraming, stageFraming)
}

// structureFramings projects the declared digest framing of every member of one
// category that names a staged execution cut. A member that stages nothing
// declares none and contributes no row.
func structureFramings(category structure.Category) (map[uint16]string, bool) {
	view, viewOK := registry.sealed.Surface(schema.SurfaceKindStructure)
	table, tableOK := structure.NewTable(view)
	if !viewOK || !tableOK {
		return nil, false
	}
	framings := make(map[uint16]string)
	for ordinal := uint16(1); int(ordinal) <= table.Count(category); ordinal++ {
		entry, ok := table.At(category, ordinal)
		if !ok {
			return nil, false
		}
		if framing := entry.Framing(); framing != "" {
			framings[entry.Ordinal()] = framing
		}
	}
	return framings, true
}

func issuancePlacement(issued rule.Issuance, key, writes schema.Key, transport bool) (artifactcompiler.IssuancePlacement, bool) {
	occurrence, occurrenceOK := structureOrdinal(issued.Occurrence, structure.CategoryOccurrenceKind)
	form, formOK := structureOrdinal(issued.Form, structure.CategoryIssuanceForm)
	input, inputOK := structureOrdinal(issued.Input, structure.CategoryIssuanceInput)
	stage, stageOK := structureOrdinal(issued.Stage, structure.CategoryIssuanceStage)
	requirement, requirementOK := structureOrdinal(issued.Requirement, structure.CategoryIssuanceRequirement)
	if !occurrenceOK || !formOK || !inputOK || !stageOK || !requirementOK {
		return artifactcompiler.IssuancePlacement{}, false
	}
	placement := artifactcompiler.IssuancePlacement{
		Occurrence:  programschema.OccurrenceKind(occurrence),
		Form:        artifactcompiler.IssuanceForm(form),
		Input:       programschema.RuleInputKind(input),
		Stage:       programschema.RuleStage(stage),
		Requirement: artifactcompiler.IssuanceRequirement(requirement),
		Code:        issued.Code,
		HasCode:     issued.HasCode,
		Key:         key,
		Writes:      writes,
		Transport:   transport,
	}
	return placement, placement.Available()
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
