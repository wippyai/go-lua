package pathevidence

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// ApplyCoordinatePresenceImplications publishes must implications directly in
// the coupled path-evidence coordinate family. It is the factorwise form of
// Lane.AddPathPresenceImplication: any publication establishes reachability
// for the complete coupled carrier, and membership is idempotent.
func ApplyCoordinatePresenceImplications(
	skeleton CoordinateSkeleton,
	entries []CoordinateEntry,
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	implications []PathPresenceImplication,
) (CoordinateSkeleton, []CoordinateEntry, bool) {
	if reg == nil || ks == nil || !ks.Valid() {
		return CoordinateSkeleton{}, nil, false
	}
	if len(implications) == 0 {
		return skeleton, append([]CoordinateEntry(nil), entries...), true
	}
	for index, implication := range implications {
		key, scalar := implicationCoordinateParts(implication)
		if !CoordinateKeyValid(key, ks, reg) || !CoordinateScalarValid(key, scalar, reg) ||
			index != 0 && !pathPresenceImplicationLess(ks, implications[index-1], implication) {
			return CoordinateSkeleton{}, nil, false
		}
	}
	byKey := make(map[CoordinateKey]CoordinateScalar, len(entries)+len(implications))
	for index, entry := range entries {
		if !CoordinateKeyValid(entry.Key, ks, reg) || !CoordinateScalarValid(entry.Key, entry.Scalar, reg) || !entry.Scalar.present ||
			index != 0 && !CoordinateKeyLess(entries[index-1].Key, entry.Key, ks) {
			return CoordinateSkeleton{}, nil, false
		}
		byKey[entry.Key] = entry.Scalar
	}
	changed := false
	for _, implication := range implications {
		key, scalar := implicationCoordinateParts(implication)
		if current, present := byKey[key]; present && !skeleton.pathPresenceImplicationsBottom {
			next := CoordinateScalarMeet(reg, current, scalar)
			if !CoordinateScalarEqual(reg, current, next) {
				byKey[key], changed = next, true
			}
		} else {
			byKey[key], changed = scalar, true
		}
	}
	if changed {
		skeleton = skeleton.Reachable()
	}
	out := make([]CoordinateEntry, 0, len(byKey))
	for key, scalar := range byKey {
		out = append(out, CoordinateEntry{Key: key, Scalar: scalar})
	}
	sort.Slice(out, func(i, j int) bool { return CoordinateKeyLess(out[i].Key, out[j].Key, ks) })
	return skeleton, out, true
}
