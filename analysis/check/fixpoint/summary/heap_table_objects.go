package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func normalizeHeapTableObjects(reg *axis.Registry, in map[identity.ID]heapidentity.TableObject) map[identity.ID]heapidentity.TableObject {
	if len(in) == 0 {
		return nil
	}
	objectDomain := heapidentity.ObjectDomain(reg)
	out := make(map[identity.ID]heapidentity.TableObject, len(in))
	for id, object := range in {
		if id == (identity.ID{}) || objectDomain.Equal(object, objectDomain.Bottom()) {
			continue
		}
		out[id] = heapidentity.CloneObject(object)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneHeapTableObjects(in map[identity.ID]heapidentity.TableObject) map[identity.ID]heapidentity.TableObject {
	return heapidentity.CloneMap(in)
}

// CloneHeapTableObjects returns a defensive copy of summary heap table objects.
func CloneHeapTableObjects(in map[identity.ID]heapidentity.TableObject) map[identity.ID]heapidentity.TableObject {
	return cloneHeapTableObjects(in)
}

func heapTableObjectsEqual(reg *axis.Registry, a, b map[identity.ID]heapidentity.TableObject) bool {
	return heapidentity.MapDomain(reg).Equal(normalizeHeapTableObjects(reg, a), normalizeHeapTableObjects(reg, b))
}

func heapTableObjectsLessOrEq(reg *axis.Registry, a, b map[identity.ID]heapidentity.TableObject) bool {
	return heapidentity.MapDomain(reg).LessOrEq(normalizeHeapTableObjects(reg, a), normalizeHeapTableObjects(reg, b))
}

func joinHeapTableObjects(reg *axis.Registry, a, b map[identity.ID]heapidentity.TableObject) map[identity.ID]heapidentity.TableObject {
	return normalizeHeapTableObjects(reg, heapidentity.MapDomain(reg).Join(
		normalizeHeapTableObjects(reg, a),
		normalizeHeapTableObjects(reg, b),
	))
}

func widenHeapTableObjects(reg *axis.Registry, prev, next map[identity.ID]heapidentity.TableObject) map[identity.ID]heapidentity.TableObject {
	return normalizeHeapTableObjects(reg, heapidentity.MapDomain(reg).Widen(
		normalizeHeapTableObjects(reg, prev),
		normalizeHeapTableObjects(reg, next),
	))
}
