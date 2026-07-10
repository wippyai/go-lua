package discriminant

import "github.com/wippyai/go-lua/analysis/type/typ"

// Detector owns required-literal tag extraction and conflict detection for
// record variants. It is reusable so callers that join many related records can
// share the tag cache without exposing their own state.
type Detector struct {
	tags   map[typ.Type]requiredTagSet
	active map[typ.Type]bool
}

func NewDetector() *Detector {
	return &Detector{}
}

// ClosedRecordSetConflicts reports whether any pair in a closed record set is
// separated by a required literal discriminant.
func (d *Detector) ClosedRecordSetConflicts(records []*typ.Record) bool {
	if d == nil {
		d = NewDetector()
	}
	hasTags := false
	for _, rec := range records {
		if d.HasRequiredTag(rec) {
			hasTags = true
			break
		}
	}
	if !hasTags {
		return false
	}

	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			if d.RecordsConflict(records[i], records[j]) {
				return true
			}
		}
	}
	return false
}

// ClosedRecordSetPresenceConflicts reports whether a closed record set is
// separated by required-field presence rather than a literal tag. Two arms are
// presence-discriminated when each carries a required, non-literal field the
// other entirely lacks: neither arm is a structural subtype of the other on the
// required-field axis, so a presence or truthiness guard on a distinguishing
// member statically selects between them.
func (d *Detector) ClosedRecordSetPresenceConflicts(records []*typ.Record) bool {
	if d == nil {
		d = NewDetector()
	}
	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			if d.RecordsPresenceConflict(records[i], records[j]) {
				return true
			}
		}
	}
	return false
}
