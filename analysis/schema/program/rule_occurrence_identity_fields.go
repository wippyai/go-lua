package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// WriteRuleOccurrenceIdentityFields replays the historical rule-placement
// portion of the Artifact identity from the sealed Program family.
func (row Program) WriteRuleOccurrenceIdentityFields(writer identity.StringIdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	count, published := RuleOccurrenceFamily().Count(&row.Frozen, catalog)
	if !published || !writer.WriteUint(uint64(count)) {
		return false
	}
	for index := 0; index < count; index++ {
		rule, held := RuleOccurrenceFamily().At(&row.Frozen, catalog, index)
		occurrence, occurrenceOK := rule.Occurrence()
		input, inputOK := rule.InputPoint()
		route, routeOK := rule.PredecessorRouteID()
		key, writes := rule.Key(), rule.Writes()
		if !held || !rule.Available() || !occurrenceOK ||
			(!inputOK && rule.InputKind() != RuleInputNone) ||
			(!routeOK && rule.InputKind() == RuleInputPredecessor) ||
			!key.Available() || !writes.Available() ||
			!writer.WriteString(string(key)) || !writer.WriteString(string(writes)) ||
			!writer.WriteUint(uint64(occurrence)) || !writer.WriteContentID(rule.PointID()) ||
			!writer.WriteContentID(input) || !writer.WriteUint(uint64(rule.Stage())) ||
			!writer.WriteUint(uint64(rule.InputKind())) || !writer.WriteContentID(route) {
			return false
		}
	}
	return true
}
