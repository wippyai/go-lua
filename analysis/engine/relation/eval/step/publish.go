package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// executePublish redeems any sealed relational child through the one mounted
// publication door. Apply remains the origin of proposal leases, but Publish
// does not care whether that sidecar reached it directly or through a closed
// Merge/ColumnProject composition. Its writable vector is redeemed by
// proposal position, never by a runtime ColumnID search.
//
// Settlements advance a local immutable predecessor in authored order; the
// caller receives those exact transitions and decides when the successor root
// becomes the next solve root. A child node has no dependency identity, so
// only the root ScheduleEntry may authorize an exact widening permit.
func (session Session) executePublish(entry arrangement.ScheduleEntry, node arrangement.Node) (nodeValue, bool) {
	binding, ok := node.Publish()
	if !ok || !binding.Available() {
		return nodeValue{}, false
	}
	children := node.Children()
	if len(children) != 1 {
		return nodeValue{}, false
	}
	child, ok := session.executeNode(children[0])
	if !ok || !child.available() {
		return nodeValue{}, false
	}
	destination := binding.Destination().Access().Relation()
	if !destination.Available() {
		return nodeValue{}, false
	}
	base := session.root
	settlements := make([]publish.Settlement, 0)
	applications := append([]apply.Results{}, child.applications...)
	for _, values := range child.applications {
		if !values.Available() {
			return nodeValue{}, false
		}
		for index := 0; index < values.Len(); index++ {
			application, applicationOK := values.At(index)
			if !applicationOK || !application.Available() {
				return nodeValue{}, false
			}
			if !publicationApplicationMatches(binding, application) {
				return nodeValue{}, false
			}
			widening, wideningOK := session.door.WideningFor(entry, destination, application)
			if !wideningOK {
				return nodeValue{}, false
			}
			settlement := session.door.Publish(base, session.scratch, application, widening)
			if !settlement.Available() {
				return nodeValue{}, false
			}
			settlements = append(settlements, settlement)
			base = settlement.Next()
		}
	}
	return publishNode(node.Digest(), settlements, applications)
}

// publicationApplicationMatches proves that every staged proposal follows
// PublishBinding.Columns' closed repeating row vector. Proposal buffers may
// contain several output rows, but each row must use every writable position
// exactly once and in the sealed order. We intentionally do not call a
// nominal lookup helper here: the positional contract is the runtime
// authority for the published output layout.
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
