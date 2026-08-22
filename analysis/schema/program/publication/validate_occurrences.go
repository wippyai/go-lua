package publication

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// occurrenceRow is one directory entry: the sealed occurrence and the dense
// parent ordinal that names its child spans. The ordinal is carried because
// occurrence child columns are addressed by parent position, so a consumer
// that resolved a row by identity would otherwise have to recover the
// position by scanning the family again.
type occurrenceRow struct {
	row     programschema.Occurrence
	ordinal int
}

// validateSealOccurrences builds the seal pass's one occurrence directory.
//
// The published Program deliberately retains no inverse index: Program's
// resolve-by-identity accessors are documented cold scans, because a retained
// occurrence directory belongs to a consumer's workspace rather than beside
// the publication. This validator is such a consumer, and this is that
// workspace. Building the directory once here is what keeps seal validation
// linear in the occurrence family; resolving identities through the cold
// scan from inside a per-row loop makes it quadratic in program size.
//
// The phase runs immediately after the foundation phase because it depends
// only on the body and point directories that phase publishes, and because
// every later phase resolves occurrences through it.
func (validator *validator) validateSealOccurrences(state *validationState) bool {
	if validator == nil || state == nil {
		return false
	}
	program := validator.program
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		return false
	}
	state.occurrenceRows = make(map[programschema.OccurrenceKind]map[identity.ContentID]occurrenceRow)
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if !rowOK || !row.Available() {
			return false
		}
		body, hasBody := row.BodyID()
		if hasBody {
			if _, exists := state.bodyRows[body]; !exists {
				return false
			}
		}
		pointOffset, pointCount, pointSpanOK := row.PointSpan()
		inputOffset, inputCount, inputSpanOK := row.InputSpan()
		if !pointSpanOK || !inputSpanOK {
			return false
		}
		for pointIndex := uint32(0); pointIndex < pointCount; pointIndex++ {
			point, pointOK := program.OccurrencePointAt(int(pointOffset + pointIndex))
			if !pointOK || !point.Available() {
				return false
			}
			if _, exists := state.pointRows[point.PointID()]; !exists {
				return false
			}
		}
		for inputIndex := uint32(0); inputIndex < inputCount; inputIndex++ {
			input, inputOK := program.OccurrenceInputAt(int(inputOffset + inputIndex))
			if !inputOK || !input.Available() {
				return false
			}
		}
		kind := row.Kind()
		rows := state.occurrenceRows[kind]
		if rows == nil {
			rows = make(map[identity.ContentID]occurrenceRow)
			state.occurrenceRows[kind] = rows
		}
		if _, duplicate := rows[row.ID()]; duplicate {
			return false
		}
		rows[row.ID()] = occurrenceRow{row: row, ordinal: index}
	}
	return true
}

// occurrence resolves one sealed occurrence of a declared kind through the
// directory. It is the only occurrence-by-identity route inside the seal
// pass.
func (state *validationState) occurrence(kind programschema.OccurrenceKind, id identity.ContentID) (occurrenceRow, bool) {
	if state == nil {
		return occurrenceRow{}, false
	}
	entry, held := state.occurrenceRows[kind][id]
	return entry, held
}
