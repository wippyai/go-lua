package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// WriteOccurrenceIdentityFields replays the historical occurrence parent and
// child-plane portion of the Artifact identity from the sealed Program
// publication. Offsets are storage layout; only the ordered child rows and
// their widths enter the preimage.
func (row Program) WriteOccurrenceIdentityFields(writer identity.StringIdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	occurrenceCount, occurrencesPublished := OccurrenceFamily().Count(&row.Frozen, catalog)
	pointCount, pointsPublished := OccurrencePointFamily().Count(&row.Frozen, catalog)
	inputCount, inputsPublished := OccurrenceInputFamily().Count(&row.Frozen, catalog)
	if !occurrencesPublished || !pointsPublished || !inputsPublished || !writer.WriteUint(uint64(occurrenceCount)) {
		return false
	}
	for index := 0; index < occurrenceCount; index++ {
		occurrence, held := OccurrenceFamily().At(&row.Frozen, catalog, index)
		pointOffset, points, pointsOK := occurrence.PointSpan()
		inputOffset, inputs, inputsOK := occurrence.InputSpan()
		literalFamily, literal, literalOK := occurrence.Literal()
		if !held || !occurrence.Available() || !pointsOK || !inputsOK ||
			uint64(pointOffset)+uint64(points) > uint64(pointCount) ||
			uint64(inputOffset)+uint64(inputs) > uint64(inputCount) {
			return false
		}
		body, _ := occurrence.BodyID()
		if !writer.WriteUint(uint64(occurrence.Kind())) || !writer.WriteContentID(occurrence.ID()) ||
			!writer.WriteContentID(body) || !writer.WriteUint(occurrence.Code()) || !writer.WriteUint(uint64(points)) {
			return false
		}
		for position := uint32(0); position < points; position++ {
			point, pointHeld := OccurrencePointFamily().At(&row.Frozen, catalog, int(pointOffset+position))
			if !pointHeld || !point.Available() || !writer.WriteContentID(point.PointID()) {
				return false
			}
		}
		if !writer.WriteUint(uint64(inputs)) {
			return false
		}
		for position := uint32(0); position < inputs; position++ {
			input, inputHeld := OccurrenceInputFamily().At(&row.Frozen, catalog, int(inputOffset+position))
			if !inputHeld || !input.Available() || !writer.WriteContentID(input.InputID()) {
				return false
			}
		}
		if !writer.WriteUint(uint64(literalFamily)) || !writer.WriteBool(literalOK) ||
			!writer.WriteUint(uint64(literal.Kind)) || !writer.WriteBool(literal.Bool) ||
			!writer.WriteUint(uint64(literal.Integer)) || !writer.WriteUint(literal.FloatBits) || !writer.WriteString(literal.String) {
			return false
		}
	}
	return true
}
