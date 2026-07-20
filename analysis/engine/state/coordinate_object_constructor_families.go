package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func heapCoordinateObjectMutation(reg *axis.Registry) coordinateObjectMutationOps {
	return coordinateObjectMutationOps{
		participant: true,
		active:      func(plan ObjectGraphMutationPlan) bool { return len(plan.objects) != 0 },
		applySkeleton: func(source coordinateSkeletonPayload, plan ObjectGraphMutationPlan) (coordinateSkeletonPayload, []coordinateObjectMutationPublication, bool, error) {
			skeleton := heapCoordinateSkeletonValue(source)
			if skeleton.keys != nil && skeleton.keys != plan.keys {
				return nil, nil, false, fmt.Errorf("%w: heap constructor keyspace", ErrInvalidLaneFactor)
			}
			if skeleton.top {
				return source, nil, true, nil
			}
			out := cloneHeapCoordinateSkeleton(skeleton)
			out.keys = plan.keys
			if out.objects == nil {
				out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton, len(plan.objects))
			}
			pubs := make([]coordinateObjectMutationPublication, 0)
			for _, object := range plan.objects {
				incoming := heapObjectSkeletonFromObject(plan.keys, object.object)
				if current, present := out.objects[object.id]; plan.mode == objectGraphJoin && present && !current.bottom {
					out.objects[object.id] = joinHeapCoordinateObject(reg, plan.keys, current, incoming, false)
				} else {
					out.objects[object.id] = incoming
				}
				pubs = append(pubs, coordinateObjectMutationPublication{
					key:   wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateRoot, id: object.id}),
					value: object.object.Root(), mode: plan.mode,
				})
				keys := make([]keyspace.Key, 0)
				object.object.VisitStaticMembers(func(key keyspace.Key, _ product.Value) bool {
					keys = append(keys, key)
					return true
				})
				sort.Slice(keys, func(i, j int) bool { return plan.keys.Less(keys[i], keys[j]) })
				for _, key := range keys {
					value, _ := object.object.StaticMember(key)
					pubs = append(pubs, coordinateObjectMutationPublication{
						key:   wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateMember, id: object.id, key: key}),
						value: value, mode: plan.mode,
					})
				}
			}
			return wrapHeapCoordinateSkeleton(out), pubs, true, nil
		},
		affectsKey: func(plan ObjectGraphMutationPlan, source coordinateKeyPayload) bool {
			key := heapCoordinateKeyValue(source)
			for _, object := range plan.objects {
				if object.id != key.id {
					continue
				}
				if plan.mode == objectGraphReplace || key.kind == heapCoordinateRoot {
					return true
				}
				_, present := object.object.StaticMember(key.key)
				return present
			}
			return false
		},
		applyScalar: func(publication coordinateObjectMutationPublication, current coordinateScalarPayload) (coordinateScalarPayload, error) {
			if publication.mode == objectGraphReplace {
				return wrapHeapCoordinateScalar(publication.value), nil
			}
			if current == nil {
				return nil, ErrInvalidLaneFactor
			}
			return wrapHeapCoordinateScalar(product.Join(reg, heapCoordinateScalarValue(current).value, publication.value)), nil
		},
	}
}

func placementCoordinateObjectMutation() coordinateObjectMutationOps {
	return coordinateObjectMutationOps{
		participant: true,
		active: func(plan ObjectGraphMutationPlan) bool {
			for _, object := range plan.objects {
				if object.placement > placement.Bottom {
					return true
				}
			}
			return false
		},
		applySkeleton: func(source coordinateSkeletonPayload, plan ObjectGraphMutationPlan) (coordinateSkeletonPayload, []coordinateObjectMutationPublication, bool, error) {
			if placementCoordinateSkeletonValue(source).top {
				return source, nil, true, nil
			}
			pubs := make([]coordinateObjectMutationPublication, 0, len(plan.objects))
			for _, object := range plan.objects {
				if object.placement > placement.Bottom {
					pubs = append(pubs, coordinateObjectMutationPublication{key: wrapPlacementCoordinateKey(object.id), placement: object.placement, mode: plan.mode})
				}
			}
			return source, pubs, true, nil
		},
		affectsKey: func(plan ObjectGraphMutationPlan, source coordinateKeyPayload) bool {
			id := placementCoordinateKeyValue(source).id
			for _, object := range plan.objects {
				if object.id == id && object.placement > placement.Bottom {
					return true
				}
			}
			return false
		},
		applyScalar: func(publication coordinateObjectMutationPublication, current coordinateScalarPayload) (coordinateScalarPayload, error) {
			if current == nil {
				current = wrapPlacementCoordinateScalar(placement.Bottom)
			}
			return wrapPlacementCoordinateScalar(placement.Join(placementCoordinateScalarValue(current).value, publication.placement)), nil
		},
	}
}
