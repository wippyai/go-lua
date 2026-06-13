package projectsummary

import "github.com/wippyai/go-lua/analysis/check/fixpoint/summary"

func projectReturnPresenceRelations(result ResultReader) []summary.ReturnPresenceRelation {
	reader, ok := result.(returnPresenceRelationReader)
	var out []summary.ReturnPresenceRelation
	if ok {
		for _, point := range result.ReturnPoints() {
			for _, relation := range reader.ReturnPresenceRelations(point) {
				out = append(out, summary.ReturnPresenceRelation{
					TriggerIndex:    relation.TriggerIndex(),
					TriggerPresence: relation.TriggerPresence(),
					TargetIndex:     relation.TargetIndex(),
					TargetPresence:  relation.TargetPresence(),
				})
			}
		}
	}
	return out
}
