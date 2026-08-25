package arrangement

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Plan is the immutable arrangement plan for one checked certificate and one
// mounted fence. It records logical requirements and opaque,
// generation-fenced physical layouts; the local coordinate inside each
// layout's handle is intentionally inaccessible.
type Plan struct{ data *planData }

type planData struct {
	fence         address.Fence
	digest        identity.ContentID
	logicalDigest identity.ContentID
	layouts       []Layout
	deliveries    []DeliveryRequirement
}

// Available reports whether Derive produced a complete plan.
func (plan Plan) Available() bool {
	return plan.data != nil && plan.data.fence.Available() && plan.data.digest.Available() && plan.data.logicalDigest.Available()
}

// Digest is the deterministic mounted plan identity. It includes the
// resolved physical layouts and changes when physical assignment changes.
func (plan Plan) Digest() identity.ContentID {
	if plan.data == nil {
		return identity.ContentID{}
	}
	return plan.data.digest
}

// LogicalDigest is the deterministic logical requirement identity. It is
// stable across physical inventory permutations.
func (plan Plan) LogicalDigest() identity.ContentID {
	if plan.data == nil {
		return identity.ContentID{}
	}
	return plan.data.logicalDigest
}

// Fence returns the mount validity fence captured by Derive.
func (plan Plan) Fence() address.Fence {
	if plan.data == nil {
		return address.Fence{}
	}
	return plan.data.fence
}

// Accesses returns canonical logical access requirements.  Nested vectors are
// copied so callers cannot mutate Plan.
func (plan Plan) Accesses() []Access {
	if !plan.Available() {
		return nil
	}
	result := make([]Access, len(plan.data.layouts))
	for index, layout := range plan.data.layouts {
		result[index] = layout.Access()
	}
	return result
}

// Layouts returns canonical physical layouts in the same order as Accesses.
// Every nested vector is copied so callers cannot mutate Plan.
func (plan Plan) Layouts() []Layout {
	if !plan.Available() {
		return nil
	}
	result := make([]Layout, len(plan.data.layouts))
	for index, layout := range plan.data.layouts {
		result[index] = cloneLayout(layout)
	}
	return result
}

// HasAccess reports logical membership without revealing a physical handle.
func (plan Plan) HasAccess(required Access) bool {
	if !plan.Available() || !required.Available() {
		return false
	}
	for _, layout := range plan.data.layouts {
		if layout.Access().Equal(required) {
			return true
		}
	}
	return false
}

// Resolve returns the immutable physical layout for one logical access.
func (plan Plan) Resolve(required Access) (Layout, bool) {
	if !plan.Available() || !required.Available() {
		return Layout{}, false
	}
	for _, layout := range plan.data.layouts {
		if layout.Access().Equal(required) {
			return layout, layout.ValidFor(plan.data.fence)
		}
	}
	return Layout{}, false
}

// DeliveryRequirements returns canonical semantic delivery requirements.
func (plan Plan) DeliveryRequirements() []DeliveryRequirement {
	if !plan.Available() {
		return nil
	}
	return append([]DeliveryRequirement(nil), plan.data.deliveries...)
}

// ValidFor reports whether this plan and book share the exact mounted fence
// and every logical requirement still resolves in that book.  The check is
// deliberately exact: stale and foreign books are refused.
func (plan Plan) ValidFor(book address.Book) bool {
	if !plan.Available() || !book.Available() || !plan.data.fence.Same(book.Fence()) {
		return false
	}
	for _, layout := range plan.data.layouts {
		access := layout.Access()
		if !layout.ValidFor(book.Fence()) {
			return false
		}
		if relation, ok := book.Relation(access.relation); !ok || !relation.ValidFor(book.Fence()) {
			return false
		}
		if access.key.Available() {
			if key, ok := book.Key(access.key); !ok || !key.ValidFor(book.Fence()) {
				return false
			}
		}
		for _, column := range access.columns {
			if value, ok := book.Column(column); !ok || !value.ValidFor(book.Fence()) {
				return false
			}
		}
	}
	for _, requirement := range plan.data.deliveries {
		if !requirement.Available() {
			return false
		}
		physical, ok := requirement.Access()
		if !ok || !plan.HasAccess(physical) {
			return false
		}
	}
	return true
}

func cloneAccess(access Access) Access {
	access.columns = append([]model.ColumnID(nil), access.columns...)
	return access
}

func cloneLayout(layout Layout) Layout {
	layout.access = cloneAccess(layout.access)
	layout.keyColumns = append([]model.ColumnID(nil), layout.keyColumns...)
	return layout
}

func canonicalizeAccesses(values []Access) []Access {
	canonical := make([]Access, 0, len(values))
	for _, value := range values {
		if !value.Available() {
			continue
		}
		duplicate := false
		for _, prior := range canonical {
			if prior.Equal(value) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			canonical = append(canonical, cloneAccess(value))
		}
	}
	sort.SliceStable(canonical, func(left, right int) bool { return accessLess(canonical[left], canonical[right]) })
	return canonical
}

func canonicalizeDeliveries(values []DeliveryRequirement) []DeliveryRequirement {
	canonical := make([]DeliveryRequirement, 0, len(values))
	for _, value := range values {
		if !value.Available() {
			continue
		}
		duplicate := false
		for _, prior := range canonical {
			if prior.equal(value) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			canonical = append(canonical, value)
		}
	}
	sort.SliceStable(canonical, func(left, right int) bool { return deliveryRequirementLess(canonical[left], canonical[right]) })
	return canonical
}

func appendPlanDigestParts(parts *[][]byte, plan planData) {
	for _, layout := range plan.layouts {
		*parts = append(*parts, accessDigest(layout.access))
	}
	for _, delivery := range plan.deliveries {
		*parts = append(*parts, deliveryRequirementDigest(delivery))
	}
}

func accessDigest(value Access) []byte {
	parts := make([][]byte, 0, 2+len(value.columns))
	parts = append(parts, nominalBytes(value.relation.Owner().Content(), value.relation.Content()))
	if value.key.Available() {
		parts = append(parts, nominalBytes(value.key.Relation().Owner().Content(), value.key.Content()))
	} else {
		parts = append(parts, nil)
	}
	for _, column := range value.columns {
		parts = append(parts, nominalBytes(column.Relation().Owner().Content(), column.Content()))
	}
	result, _ := identity.DeriveContentID("analysis/relation/mount/arrangement/access/v1", parts...)
	return contentBytes(result)
}

func nominalBytes(owner, content identity.ContentID) []byte {
	result := make([]byte, 0, len(owner)+len(content))
	result = append(result, owner[:]...)
	return append(result, content[:]...)
}

func contentBytes(value identity.ContentID) []byte {
	result := make([]byte, len(value))
	copy(result, value[:])
	return result
}
