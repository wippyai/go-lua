package composite

import (
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
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
func ArtifactIssuanceDirectory(compilation Compilation) (issuance.Directory, bool) {
	state := compilation.catalog
	if state == nil || state.sealed == nil {
		return issuance.Directory{}, false
	}
	var placements []issuance.Placement
	for _, entry := range state.templates {
		if entry == nil || entry.Lane() == rule.LaneLink {
			continue
		}
		if !entry.Key().Available() {
			return issuance.Directory{}, false
		}
		for index := 0; index < entry.IssuanceCount(); index++ {
			issued, ok := entry.IssuanceAt(index)
			if !ok {
				return issuance.Directory{}, false
			}
			placement, ok := issuancePlacement(state, issued, entry.Key(), entry.Writes(), entry.Lane() == rule.LaneMounted)
			if !ok {
				return issuance.Directory{}, false
			}
			placements = append(placements, placement)
		}
	}
	forms, formsOK := structureFramings(state, structure.CategoryIssuanceForm)
	stages, stagesOK := structureFramings(state, structure.CategoryIssuanceStage)
	if !formsOK || !stagesOK {
		return issuance.Directory{}, false
	}
	formFraming := make(map[issuance.Form]string, len(forms))
	for ordinal, framing := range forms {
		formFraming[issuance.Form(ordinal)] = framing
	}
	stageFraming := make(map[programschema.RuleStage]string, len(stages))
	for ordinal, framing := range stages {
		stageFraming[programschema.RuleStage(ordinal)] = framing
	}
	return issuance.NewDirectory(placements, formFraming, stageFraming)
}

// structureFramings projects the declared digest framing of every member of one
// category that names a staged execution cut. A member that stages nothing
// declares none and contributes no row.
func structureFramings(state *catalog, category structure.Category) (map[uint16]string, bool) {
	if state == nil || state.sealed == nil {
		return nil, false
	}
	view, viewOK := state.sealed.Surface(schema.SurfaceKindStructure)
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

func issuancePlacement(state *catalog, issued rule.Issuance, key, writes schema.Key, transport bool) (issuance.Placement, bool) {
	occurrence, occurrenceOK := structureOrdinal(state, issued.Occurrence, structure.CategoryOccurrenceKind)
	form, formOK := structureOrdinal(state, issued.Form, structure.CategoryIssuanceForm)
	input, inputOK := structureOrdinal(state, issued.Input, structure.CategoryIssuanceInput)
	stage, stageOK := structureOrdinal(state, issued.Stage, structure.CategoryIssuanceStage)
	requirement, requirementOK := structureOrdinal(state, issued.Requirement, structure.CategoryIssuanceRequirement)
	if !occurrenceOK || !formOK || !inputOK || !stageOK || !requirementOK {
		return issuance.Placement{}, false
	}
	placement := issuance.Placement{
		Occurrence:  programschema.OccurrenceKind(occurrence),
		Form:        issuance.Form(form),
		Input:       programschema.RuleInputKind(input),
		Stage:       programschema.RuleStage(stage),
		Requirement: issuance.Requirement(requirement),
		Code:        issued.Code,
		HasCode:     issued.HasCode,
		Key:         key,
		Writes:      writes,
		Transport:   transport,
	}
	return placement, placement.Available()
}

func structureOrdinal(state *catalog, key schema.Key, category structure.Category) (uint16, bool) {
	if state == nil || state.sealed == nil {
		return 0, false
	}
	view, viewOK := state.sealed.Surface(schema.SurfaceKindStructure)
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
