package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// WriteSummaryIdentityFields replays the historical exact, arithmetic, and
// unary summary portions of the Artifact identity from their Program-owned
// sealed families.
func (row Program) WriteSummaryIdentityFields(writer identity.IdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	exactCount, exactPublished := ExactScalarSummaryFamily().Count(&row.Frozen, catalog)
	if !exactPublished || !writer.WriteUint(uint64(exactCount)) {
		return false
	}
	for index := 0; index < exactCount; index++ {
		exact, held := ExactScalarSummaryFamily().At(&row.Frozen, catalog, index)
		literal, literalOK := exact.Literal()
		if !held || !exact.Available() || !literalOK ||
			!writer.WriteContentID(exact.ID()) || !writer.WriteContentID(exact.OccurrenceID()) ||
			!writer.WriteContentID(exact.SubjectID()) || !writer.WriteContentID(exact.BodyPathID()) ||
			!writer.WriteUint(uint64(exact.Role())) || !writer.WriteUint(uint64(literal.Kind)) ||
			!writer.WriteUint(uint64(literal.Integer)) || !writer.WriteUint(literal.FloatBits) {
			return false
		}
	}

	arithmeticCount, arithmeticPublished := ArithmeticSummaryFamily().Count(&row.Frozen, catalog)
	if !arithmeticPublished || !writer.WriteUint(uint64(arithmeticCount)) {
		return false
	}
	for index := 0; index < arithmeticCount; index++ {
		arithmetic, held := ArithmeticSummaryFamily().At(&row.Frozen, catalog, index)
		left, right, result, representationsOK := arithmetic.Representations()
		if !held || !arithmetic.Available() || !representationsOK ||
			!writer.WriteContentID(arithmetic.ID()) || !writer.WriteContentID(arithmetic.OccurrenceID()) ||
			!writer.WriteContentID(arithmetic.BodyPathID()) || !writer.WriteUint(uint64(arithmetic.Operator())) ||
			!writer.WriteUint(uint64(left)) || !writer.WriteUint(uint64(right)) ||
			!writer.WriteUint(uint64(result)) || !writer.WriteUint(uint64(arithmetic.DivisorProperty())) {
			return false
		}
	}

	unaryCount, unaryPublished := UnarySummaryFamily().Count(&row.Frozen, catalog)
	if !unaryPublished || !writer.WriteUint(uint64(unaryCount)) {
		return false
	}
	for index := 0; index < unaryCount; index++ {
		unary, held := UnarySummaryFamily().At(&row.Frozen, catalog, index)
		operand, result, representationsOK := unary.Representations()
		if !held || !unary.Available() || !representationsOK ||
			!writer.WriteContentID(unary.ID()) || !writer.WriteContentID(unary.OccurrenceID()) ||
			!writer.WriteContentID(unary.BodyPathID()) || !writer.WriteContentID(unary.OutputPointID()) ||
			!writer.WriteUint(uint64(unary.Operator())) || !writer.WriteUint(uint64(operand)) ||
			!writer.WriteUint(uint64(result)) {
			return false
		}
	}
	return true
}
