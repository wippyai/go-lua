package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

// HeapTableIdentitySkeletonDynamicIndexFacts returns the exact finite dynamic
// index observation of one present heap object without reading its independent
// root or static-member value coordinates. Heap/object Bottom, heap Top, and a
// Top dynamic-index map have no finite projection and report false, matching
// HeapObjectContainerType's concrete observation.
func (d ProductDomain) HeapTableIdentitySkeletonDynamicIndexFacts(
	skeleton HeapTableIdentitySkeletonFactor,
	id identity.ID,
) (map[dynamicindex.Key]dynamicindex.Fact, bool, error) {
	if _, err := d.validateHeapTableIdentitySkeleton(skeleton, skeleton.keys); err != nil {
		return nil, false, err
	}
	if id == (identity.ID{}) || skeleton.top {
		return nil, false, nil
	}
	object, present := skeleton.objects[identity.ConcreteTerm(id)]
	if !present || object.bottom || object.dynamicIndexFactsTop || len(object.dynamicIndexFacts) == 0 {
		return nil, false, nil
	}
	out := make(map[dynamicindex.Key]dynamicindex.Fact, len(object.dynamicIndexFacts))
	for key, fact := range object.dynamicIndexFacts {
		out[key] = fact
	}
	return out, true, nil
}
