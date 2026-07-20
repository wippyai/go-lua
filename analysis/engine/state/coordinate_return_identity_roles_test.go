package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestReturnIdentityRolesSealIndependentOfLaneRegistrationOrder(t *testing.T) {
	for _, specs := range [][]laneSpec{
		{heapTableIdentityLaneSpec, placementLaneSpec},
		{placementLaneSpec, heapTableIdentityLaneSpec},
	} {
		domain := newLaneCatalog(specs).ProductDomain(standard.Registry())
		owner, ok := domain.ReturnIdentityContainerFamily()
		if !ok || owner.ID() != heapCoordinateFamilyID {
			t.Fatalf("container owner = %q/%t", owner.ID(), ok)
		}
		heapRoles, err := domain.CoordinateReturnIdentityRoles(owner)
		if err != nil {
			t.Fatal(err)
		}
		for _, role := range []CoordinateReturnIdentityRole{
			CoordinateReturnIdentitySeed,
			CoordinateReturnIdentitySkeletonEdge,
			CoordinateReturnIdentityScalarEdge,
			CoordinateReturnIdentityContainer,
		} {
			if !heapRoles.Has(role) {
				t.Fatalf("heap owner omitted role %d", role)
			}
		}
		placementLane, ok := domain.ProductLane(LanePlacement)
		if !ok {
			t.Fatal("placement lane missing")
		}
		families, err := domain.CoordinateFamilies(placementLane)
		if err != nil || len(families) != 1 {
			t.Fatalf("placement families = %d/%v", len(families), err)
		}
		placementRoles, err := domain.CoordinateReturnIdentityRoles(families[0])
		if err != nil || !placementRoles.Has(CoordinateReturnIdentityPublisher) || placementRoles.Has(CoordinateReturnIdentityContainer) {
			t.Fatalf("placement roles malformed: publisher=%t container=%t err=%v", placementRoles.Has(CoordinateReturnIdentityPublisher), placementRoles.Has(CoordinateReturnIdentityContainer), err)
		}
	}
}

func TestProductDomainRejectsMalformedReturnIdentityRoleInventory(t *testing.T) {
	baseBuild := heapCoordinateFamilySpec.build
	tests := []struct {
		name  string
		roles coordinateReturnIdentityRoleBits
		two   bool
	}{
		{name: "closure-without-container", roles: coordinateReturnIdentityRoles(CoordinateReturnIdentitySeed)},
		{name: "container-without-closure", roles: coordinateReturnIdentityRoles(CoordinateReturnIdentityContainer)},
		{name: "duplicate-container-owner", roles: coordinateReturnIdentityRoles(CoordinateReturnIdentitySeed, CoordinateReturnIdentityContainer), two: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			family := heapCoordinateFamilySpec
			family.build = func(reg *axis.Registry, options DomainOptions) coordinateFamilyOps {
				ops := baseBuild(reg, options)
				ops.returnIdentity.roles = test.roles
				return ops
			}
			spec := heapTableIdentityLaneSpec
			spec.coordinateFamilies = []coordinateFamilySpec{family}
			if test.two {
				second := family
				second.id = "duplicate-container-test"
				spec.coordinateFamilies = append(spec.coordinateFamilies, second)
			}
			catalog := newLaneCatalog([]laneSpec{spec})
			defer func() {
				if recover() == nil {
					t.Fatal("ProductDomain admitted malformed return-identity roles")
				}
			}()
			_ = catalog.ProductDomain(standard.Registry())
		})
	}
}
