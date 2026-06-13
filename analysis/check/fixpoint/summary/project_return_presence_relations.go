package summary

func projectReturnPresenceRelations(result ResultReader) []ReturnPresenceRelation {
	reader, ok := result.(returnPresenceRelationReader)
	var out []ReturnPresenceRelation
	if ok {
		for _, point := range result.ReturnPoints() {
			for _, relation := range reader.ReturnPresenceRelations(point) {
				out = append(out, ReturnPresenceRelation{
					TriggerIndex:    relation.TriggerIndex(),
					TriggerPresence: relation.TriggerPresence(),
					TargetIndex:     relation.TargetIndex(),
					TargetPresence:  relation.TargetPresence(),
				})
			}
		}
	}
	return normalizeReturnPresenceRelations(out)
}
