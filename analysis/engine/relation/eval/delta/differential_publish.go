package delta

import (
	applydifferential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// publishDifferentials redeems signed Apply extents in their sealed authored
// order.  Differential transport remains private to the evaluator: unlike an
// ordinary Apply result it is never appended to Result.Applications.  Each
// entry reaches the same publication Door as ordinary applications, with the
// predecessor root returned by the prior settlement.
//
// Recurrence widening is derived from the actual After application.  A
// Before-only entry has no current write side and therefore carries the zero
// permit, even when the schedule itself is a widening head.  The Door remains
// the sole transaction path; this helper only sequences its calls and retains
// the resulting settlements/root.
func (session Session) publishDifferentials(
	entry arrangement.ScheduleEntry,
	binding arrangement.PublishBinding,
	values []applydifferential.Results,
	base database.Version,
) ([]publish.Settlement, database.Version, bool) {
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
			value, valueOK := results.At(index)
			if !valueOK || !value.Available() {
				return nil, database.Version{}, false
			}

			// The Publish binding still owns the destination envelope. Check the
			// actual retained side(s) before asking the Door for a permit; this
			// keeps a foreign Before from becoming a valid removal and ensures the
			// widening query is based on the exact After lease.
			before, beforeOK := value.Before()
			after, afterOK := value.After()
			if !beforeOK && !afterOK {
				return nil, database.Version{}, false
			}
			if beforeOK && !publicationApplicationMatches(binding, before) {
				return nil, database.Version{}, false
			}
			if afterOK && !publicationApplicationMatches(binding, after) {
				return nil, database.Version{}, false
			}

			widening := witness.WideningPermit{}
			if afterOK {
				var wideningOK bool
				widening, wideningOK = session.door.WideningFor(entry, destination, after)
				if !wideningOK {
					return nil, database.Version{}, false
				}
			}
			settlement := session.door.PublishDifferential(current, session.scratch, value, widening)
			if !settlement.Available() {
				return nil, database.Version{}, false
			}
			settlements = append(settlements, settlement)
			current = settlement.Next()
		}
	}
	return settlements, current, true
}
