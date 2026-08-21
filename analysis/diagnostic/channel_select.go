package diagnostic

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/selectapply"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/type/channelselect"
)

// SelectSite is the compile-time key directory of one published select.
// Bound is exclusive over table ordinals so lookalike holes stay misses.
type SelectSite struct {
	Site  identity.ContentID
	Bound int
}

// LoadCaseSet reconstructs accepted CaseFacts from the composition snapshot.
// Facts live only in the published column; sites name the keys to read.
func LoadCaseSet(published *snapshot.Snapshot, column snapshot.Axis[identity.ContentID, channelselect.CaseFact], sites []SelectSite) (channelselect.CaseSet, bool) {
	var set channelselect.CaseSet
	if published == nil || !published.Published() || !column.Available() {
		return channelselect.CaseSet{}, false
	}
	for _, site := range sites {
		if !site.Site.Available() || site.Bound < 0 {
			return channelselect.CaseSet{}, false
		}
		for ordinal := 0; ordinal < site.Bound; ordinal++ {
			id, idOK := channelselect.CaseFactID(channelselect.CaseFact{Site: site.Site, Ordinal: ordinal})
			if !idOK {
				return channelselect.CaseSet{}, false
			}
			fact, status := snapshot.Read(published, column, id)
			switch status {
			case snapshot.ReadMiss:
				continue
			case snapshot.ReadHit:
				if fact.Site != site.Site || fact.Ordinal != ordinal || !set.Admit(fact) {
					return channelselect.CaseSet{}, false
				}
			default:
				return channelselect.CaseSet{}, false
			}
		}
	}
	return set, true
}

// ChannelSelectInput is the composition snapshot plus the compile-time
// directory CollectChannelSelect reads. Facts stay in the snapshot.
type ChannelSelectInput struct {
	Published *snapshot.Snapshot
	Column    snapshot.Axis[identity.ContentID, channelselect.CaseFact]
	Sites     []SelectSite
	Handlers  []selectapply.Handler
}

const channelSelectFindingDomain = "analysis/diagnostic-finding/channel-select-exhaustiveness/v1"

// CollectChannelSelect emits one finding for each select whose if-chain
// leaves an accepted Snapshot arm unnamed and uncovered.
func CollectChannelSelect(report *DiagnosticReport, input ChannelSelectInput, severity FindingSeverity) bool {
	if report == nil || !severity.Available() {
		return false
	}
	if input.Published == nil || !input.Published.Published() {
		return false
	}
	set, loaded := LoadCaseSet(input.Published, input.Column, input.Sites)
	if !loaded {
		return false
	}
	handlers := make(map[identity.ContentID]selectapply.Handler, len(input.Handlers))
	for _, handler := range input.Handlers {
		if !handler.Site.Available() {
			return false
		}
		if _, duplicate := handlers[handler.Site]; duplicate {
			return false
		}
		handlers[handler.Site] = handler
	}
	for _, site := range input.Sites {
		handler, found := handlers[site.Site]
		if !found {
			continue
		}
		missing := set.MissingArms(site.Site, handler.Handled, handler.SelectDefault || handler.ElseDefault)
		if len(missing) == 0 {
			continue
		}
		if !appendChannelSelectFinding(report, handler, missing, severity) {
			return false
		}
	}
	return true
}

func appendChannelSelectFinding(report *DiagnosticReport, handler selectapply.Handler, missing []channelselect.CaseFact, severity FindingSeverity) bool {
	if report == nil || handler.Result == "" || len(missing) == 0 || !severity.Available() {
		return false
	}
	subject, subjectOK := NewSemanticName(handler.Result)
	location, locationOK := NewLocation(handler.Location.File, handler.Location.StartLine, handler.Location.StartCol, handler.Location.EndLine, handler.Location.EndCol)
	handledNames := make([]string, 0, len(handler.Handled))
	for _, ordinal := range handler.Handled {
		name, named := handler.Names[ordinal]
		if !named || name == "" {
			return false
		}
		handledNames = append(handledNames, name)
	}
	missingNames := make([]string, 0, len(missing))
	for _, fact := range missing {
		name, named := handler.Names[fact.Ordinal]
		if !named || name == "" {
			return false
		}
		missingNames = append(missingNames, name)
	}
	handled, handledOK := NewNameList(handledNames)
	missingList, missingOK := NewNameList(missingNames)
	findingID, findingOK := identity.DeriveContentID(channelSelectFindingDomain, handler.Site[:])
	if !subjectOK || !locationOK || !handledOK || !missingOK || !findingOK {
		return false
	}
	data := NewCaseTemplateData(subject, handled, missingList)
	entry, declared := Declaration(report.declarations, DiagnosticCodeChannelSelectExhaustiveness)
	if !declared || !data.ValidFor(entry, 0) {
		return false
	}
	report.AppendFinding(NewFindingRow(findingID, handler.Site, DiagnosticCodeChannelSelectExhaustiveness, severity, location, data))
	return true
}
