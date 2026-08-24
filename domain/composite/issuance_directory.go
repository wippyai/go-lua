package composite

import (
	issuanceschema "github.com/wippyai/go-lua/analysis/schema/issuance"
	"github.com/wippyai/go-lua/analysis/schema/rule"
)

// ArtifactIssuanceDirectory binds rule subscriptions directly to their sealed
// issuance declarations. It performs no ordinal casts, framing projection, or
// semantic reconstruction. Link and mounted-point rules are omitted because
// authored Program rows do not issue either lane.
func ArtifactIssuanceDirectory(compilation Compilation) (issuanceschema.Plan, bool) {
	state := compilation.catalog
	if state == nil || state.sealed == nil {
		return issuanceschema.Plan{}, false
	}
	view, viewOK := state.sealed.Surface(issuanceschema.NewSurface(nil).Kind())
	table, tableOK := issuanceschema.NewTable(view)
	if !viewOK || !tableOK {
		return issuanceschema.Plan{}, false
	}
	var subscriptions []issuanceschema.SubscriptionSpec
	for _, entry := range state.templates {
		if entry == nil || entry.Lane() == rule.LaneLink || entry.Lane() == rule.LaneMountedPoint {
			continue
		}
		if !entry.Key().Available() || !entry.Writes().Available() {
			return issuanceschema.Plan{}, false
		}
		// The candidate source is the rule Program's own statement. The
		// subscription transports it so the issuance machine can resolve the
		// row while it owns the join; it is not authored a second time beside
		// the subscription.
		issuedRow := entry.Program().Candidate.IssuedRow
		for index := 0; index < entry.IssuanceCount(); index++ {
			issued, ok := entry.IssuanceAt(index)
			if !ok || !issued.Available() {
				return issuanceschema.Plan{}, false
			}
			subscriptions = append(subscriptions, issuanceschema.SubscriptionSpec{
				Family: issued.Occurrence, Requirement: issued.Requirement, Form: issued.Form,
				Rule: entry.Key(), Writes: entry.Writes(), Source: issuedRow,
			})
		}
	}
	return issuanceschema.NewPlan(table, subscriptions)
}
