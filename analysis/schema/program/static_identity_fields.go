package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// WriteStaticTypeValueIdentityFields replays the authored static type-value
// rows that historically precede the static-node graph in the Artifact
// identity stream. Their row order and field ownership stay with Program.
func (row Program) WriteStaticTypeValueIdentityFields(writer identity.StringIdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	typeValueCount, typeValuesPublished := StaticTypeValueFamily().Count(&row.Frozen, catalog)
	if !typeValuesPublished || !writer.WriteUint(uint64(typeValueCount)) {
		return false
	}
	for index := 0; index < typeValueCount; index++ {
		value, held := StaticTypeValueFamily().At(&row.Frozen, catalog, index)
		if !held || !writer.WriteContentID(value.ID()) || !writer.WriteContentID(value.BodyPathID()) ||
			!writer.WriteContentID(value.ReferenceID()) || !writer.WriteContentID(value.RootID()) || !writer.WriteString(value.Name()) {
			return false
		}
	}
	return true
}

// WriteStaticExpressionInputIdentityFields replays the authored expression
// and input rows that historically follow the static-node graph in the
// Artifact identity stream. Dense family positions are retained by replay
// order; no storage offset enters the identity.
func (row Program) WriteStaticExpressionInputIdentityFields(writer identity.StringIdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	expressionCount, expressionsPublished := StaticExpressionFamily().Count(&row.Frozen, catalog)
	if !expressionsPublished || !writer.WriteUint(uint64(expressionCount)) {
		return false
	}
	for index := 0; index < expressionCount; index++ {
		expression, held := StaticExpressionFamily().At(&row.Frozen, catalog, index)
		if !held || !writer.WriteContentID(expression.ID()) || !writer.WriteContentID(expression.ReferenceID()) || !writer.WriteContentID(expression.Owner()) {
			return false
		}
	}
	inputCount, inputsPublished := StaticInputFamily().Count(&row.Frozen, catalog)
	if !inputsPublished || !writer.WriteUint(uint64(inputCount)) {
		return false
	}
	for index := 0; index < inputCount; index++ {
		input, held := StaticInputFamily().At(&row.Frozen, catalog, index)
		if !held || !input.Available() {
			return false
		}
		exact := input.OperandLiteral()
		if !writer.WriteContentID(input.ID()) || !writer.WriteContentID(input.Owner()) || !writer.WriteUint(uint64(input.Kind())) ||
			!writer.WriteUint(uint64(input.OperandKind())) || !writer.WriteContentID(input.ExpressionID()) ||
			!writer.WriteContentID(input.SourceID()) || !writer.WriteContentID(input.TargetID()) || !writer.WriteContentID(input.OperandID()) ||
			!writer.WriteContentID(input.FrontierID()) || !writer.WriteContentID(input.OperandReferenceID()) ||
			!writer.WriteContentID(input.OperandSubjectID()) || !writer.WriteContentID(input.OperandBodyPathID()) ||
			!writer.WriteUint(uint64(exact.Kind)) || !writer.WriteBool(exact.Bool) || !writer.WriteUint(uint64(exact.Integer)) ||
			!writer.WriteUint(exact.FloatBits) || !writer.WriteString(exact.String) || !writer.WriteUint(uint64(input.Cursor())) {
			return false
		}
	}
	return true
}
