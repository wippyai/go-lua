package delta

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// publish redeems applications in authored/path order through the one
// mounted publication door. No application is suppressed by an evaluator
// ordinal: semi-naive disjointness is established by the stable-side rules
// before this function is reached.
func (session Session) publish(entry arrangement.ScheduleEntry, binding arrangement.PublishBinding, values []apply.Results, base database.Version) ([]publish.Settlement, database.Version, bool) {
	if !session.Available() || !entry.Available() || !binding.Available() || values == nil || !base.Available() {
		return nil, database.Version{}, false
	}
	destination := binding.Destination().Access().Relation()
	if !destination.Available() {
		return nil, database.Version{}, false
	}
	settlements := make([]publish.Settlement, 0)
	current := base
	for _, results := range values {
		if !results.Available() {
			return nil, database.Version{}, false
		}
		for index := 0; index < results.Len(); index++ {
			application, ok := results.At(index)
			if !ok || !application.Available() || !publicationApplicationMatches(binding, application) {
				return nil, database.Version{}, false
			}
			widening, wideningOK := session.door.WideningFor(entry, destination, application)
			if !wideningOK {
				return nil, database.Version{}, false
			}
			settlement := session.door.Publish(current, session.scratch, application, widening)
			if !settlement.Available() {
				return nil, database.Version{}, false
			}
			settlements = append(settlements, settlement)
			current = settlement.Next()
		}
	}
	return settlements, current, true
}

func publicationApplicationMatches(binding arrangement.PublishBinding, application apply.Application) bool {
	if !binding.Available() || !application.Available() {
		return false
	}
	proposals, hasProposals := application.Proposals()
	if !hasProposals {
		return application.Outcome().Code == outcome.Refused
	}
	if !proposals.Available() || proposals.Len() == 0 {
		return proposals.Available()
	}
	destination := binding.Destination().Access().Relation()
	columns := binding.Columns().Columns()
	if !destination.Available() || len(columns) == 0 || proposals.Len()%len(columns) != 0 {
		return false
	}
	for index := 0; index < proposals.Len(); index++ {
		proposal, ok := proposals.At(index)
		if !ok || !proposal.Available() {
			return false
		}
		cell := proposal.Destination()
		if !cell.Available() || cell.Relation() != destination || cell.Column() != columns[index%len(columns)] {
			return false
		}
	}
	return true
}
