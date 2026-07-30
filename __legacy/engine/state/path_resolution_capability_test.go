package state

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

func TestPathResolutionLanesFollowRegisteredEnabledInventory(t *testing.T) {
	reg := standard.Registry()
	tests := []struct {
		name   string
		domain ProductDomain
		want   []LaneID
	}{
		{
			name:   "full",
			domain: RegisteredProductDomain(reg),
			want:   []LaneID{LaneDynamicIndex, LaneHeapTableIdentity},
		},
		{
			name: "dynamic subset",
			domain: mustProductDomainWithLanesForPathResolutionTest(t, reg, []LaneID{
				LaneNumCeils, LaneDynamicIndex, LaneValues,
			}),
			want: []LaneID{LaneDynamicIndex},
		},
		{
			name: "heap subset",
			domain: mustProductDomainWithLanesForPathResolutionTest(t, reg, []LaneID{
				LaneLenFloors, LaneHeapTableIdentity, LanePathEvidence,
			}),
			want: []LaneID{LaneHeapTableIdentity},
		},
		{
			name: "independent subset",
			domain: mustProductDomainWithLanesForPathResolutionTest(t, reg, []LaneID{
				LaneValues, LanePathEvidence, LaneLenFloors,
			}),
			want: []LaneID{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lanes := test.domain.PathResolutionLanes()
			got := make([]LaneID, len(lanes))
			for index := range lanes {
				got[index] = lanes[index].ID()
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("PathResolutionLanes() = %v, want %v", got, test.want)
			}
		})
	}
}

func mustProductDomainWithLanesForPathResolutionTest(t *testing.T, reg *axis.Registry, lanes []LaneID) ProductDomain {
	t.Helper()
	domain, err := TryRegisteredProductDomainWithLanes(reg, lanes)
	if err != nil {
		t.Fatalf("build product domain: %v", err)
	}
	return domain
}
