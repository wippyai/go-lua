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
		native, nativeOK := rule.Native()
		key, writes := rule.Key(), rule.Writes()
		if !held || !rule.Available() || !occurrenceOK || !nativeOK ||
			(inputOK != input.Available()) || (routeOK != route.Available()) ||
			!key.Available() || !writes.Available() ||
			!writer.WriteString(string(key)) || !writer.WriteString(string(writes)) ||
			!writer.WriteUint(uint64(occurrence)) || !writer.WriteContentID(rule.PointID()) ||
			!writer.WriteContentID(input) || !writer.WriteString(string(rule.Stage())) ||
			!writer.WriteString(string(rule.InputSpec())) || !writer.WriteContentID(route) || !writer.WriteBool(native) {
			return false
		}
	}
	return true
}
