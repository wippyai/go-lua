package discriminant

import "github.com/wippyai/go-lua/analysis/domain/type/typ"

// RecordsConflict reports whether two records are discriminated variants kept
// distinct rather than coalesced.
//
// A required literal field shared by both records is either a variant tag or
// incidental literal data. The structural signal of a tag is that it is the
// single literal axis on which the records disagree: exactly one shared required
// literal field has differing values, and no shared required literal field has
// an equal value acting as a constant data key. When the records disagree on
// several literal fields, or share an equal-valued literal alongside the
// difference, the literals are incidental data and the records coalesce.
// Records whose literal-erased residuals do not merge cleanly also stay
// distinct, since merging them would lose structure rather than widen a scalar.
func (d *Detector) RecordsConflict(a, b *typ.Record) bool {
	if d == nil {
		d = NewDetector()
	}
	differing, equal := d.sharedRequiredLiteralAxes(a, b)
	if differing == 1 && equal == 0 {
		return true
	}
	if differing == 0 {
		return false
	}
	return !d.literalErasedResidualsCleanlyMergeable(a, b)
}

func (d *Detector) sharedRequiredLiteralAxes(a, b *typ.Record) (differing, equal int) {
	if d == nil {
		d = NewDetector()
	}
	left := d.requiredTags(a)
	right := d.requiredTags(b)
	left.forEach(func(path string, leftHash uint64) bool {
		rightHash, ok := right.lookup(path)
		if !ok {
			return true
		}
		if leftHash == rightHash {
			equal++
		} else {
			differing++
		}
		return true
	})
	return differing, equal
}
