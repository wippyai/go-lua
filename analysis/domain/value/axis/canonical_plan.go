package axis

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const (
	canonicalRegistryDomain  = "analysis.value-axis.registry-schema"
	canonicalRegistryVersion = 1
	canonicalPlanRecord      = 1
	canonicalAxisRecord      = 2

	// CanonicalCorePresenceID is the schema identity of the product's mandatory,
	// always-present presence lane. Keeping it in axis avoids duplicating the
	// core-inventory contract between the registry and presence package.
	CanonicalCorePresenceID = "presence"
)

// SchemaIdentity is the portable identity of a declared registry schema.
type SchemaIdentity [sha256.Size]byte

// CanonicalPlanEntry is one immutable axis row in canonical AxisID order.
type CanonicalPlanEntry struct {
	AxisID        string
	Retention     RetentionMode
	Boundary      BoundaryPolicy
	Status        CanonicalStatus
	CodecID       string
	CodecVersion  uint64
	PendingReason string
}

// CanonicalPlan is the deterministic registry-level codec plan. Its schema
// identity exists for comparison while Pending; canonical authority does not.
type CanonicalPlan struct {
	entries         []CanonicalPlanEntry
	identity        SchemaIdentity
	ready           bool
	inventorySealed bool
	pending         []string
}

// SealCanonicalInventory proves that the registry contains the complete
// always-present product-axis inventory. Sparse-axis registration remains
// extensible, but no axis may be added after this completeness boundary.
//
// Presence is currently the sole always-present axis and must occur exactly
// once. Expanding the core inventory is an explicit schema migration here,
// never an inference from whichever entries happen to be registered.
func (r *Registry) SealCanonicalInventory() error {
	if r == nil {
		return fmt.Errorf("axis: nil registry")
	}
	if r.frozen {
		return fmt.Errorf("axis: cannot seal canonical inventory after registry freeze")
	}
	if r.canonicalInventorySealed {
		return fmt.Errorf("axis: canonical inventory is already sealed")
	}
	if len(r.canonicalCore) != 1 || len(r.canonicalCoreOrder) != 1 {
		return fmt.Errorf("axis: canonical core inventory must contain presence exactly once")
	}
	if _, ok := r.canonicalCore[CanonicalCorePresenceID]; !ok {
		return fmt.Errorf("axis: canonical core inventory is missing presence")
	}
	if r.canonicalCoreOrder[0].ID() != CanonicalCorePresenceID {
		return fmt.Errorf("axis: canonical core inventory contains a non-presence entry")
	}
	r.canonicalInventorySealed = true
	return nil
}

// CanonicalPlan returns the canonical metadata plan of an immutable registry.
// Freezing solver registration does not imply canonical readiness.
func (r *Registry) CanonicalPlan() (CanonicalPlan, error) {
	if r == nil {
		return CanonicalPlan{}, fmt.Errorf("axis: nil registry")
	}
	if !r.frozen {
		return CanonicalPlan{}, fmt.Errorf("axis: canonical plan requires a frozen registry")
	}
	all := make([]ErasedSpec, 0, len(r.canonicalCoreOrder)+len(r.order))
	all = append(all, r.canonicalCoreOrder...)
	all = append(all, r.order...)
	entries := make([]CanonicalPlanEntry, len(all))
	for i, spec := range all {
		entries[i] = CanonicalPlanEntry{
			AxisID:        spec.ID(),
			Retention:     spec.RetentionMode(),
			Boundary:      spec.BoundaryPolicy(),
			Status:        spec.CanonicalStatus(),
			CodecID:       spec.CanonicalCodecID(),
			CodecVersion:  spec.CanonicalCodecVersion(),
			PendingReason: spec.CanonicalPendingReason(),
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].AxisID < entries[j].AxisID })

	identity, err := encodeCanonicalPlanIdentity(entries)
	if err != nil {
		return CanonicalPlan{}, err
	}
	plan := CanonicalPlan{
		entries: entries, identity: identity, ready: true,
		inventorySealed: r.canonicalInventorySealed,
	}
	for _, entry := range entries {
		if entry.Status != CanonicalReady {
			plan.ready = false
			plan.pending = append(plan.pending, entry.AxisID)
		}
	}
	return plan, nil
}

func encodeCanonicalPlanIdentity(entries []CanonicalPlanEntry) (SchemaIdentity, error) {
	digest := sha256.New()
	var writer canonical.Writer
	if err := writer.Reset(context.Background(), digest, canonicalRegistryDomain, canonicalRegistryVersion); err != nil {
		return SchemaIdentity{}, err
	}
	if err := writer.Record(canonicalPlanRecord); err != nil {
		return SchemaIdentity{}, err
	}
	if err := writer.Count(uint64(len(entries))); err != nil {
		return SchemaIdentity{}, err
	}
	for _, entry := range entries {
		if err := writer.Record(canonicalAxisRecord); err != nil {
			return SchemaIdentity{}, err
		}
		if err := writer.String(entry.AxisID); err != nil {
			return SchemaIdentity{}, err
		}
		if err := writer.Uint(uint64(entry.Retention)); err != nil {
			return SchemaIdentity{}, err
		}
		if err := writer.Uint(uint64(entry.Boundary)); err != nil {
			return SchemaIdentity{}, err
		}
		if err := writer.Uint(uint64(entry.Status)); err != nil {
			return SchemaIdentity{}, err
		}
		if err := writer.String(entry.CodecID); err != nil {
			return SchemaIdentity{}, err
		}
		if err := writer.Uint(entry.CodecVersion); err != nil {
			return SchemaIdentity{}, err
		}
	}
	if err := writer.Finish(); err != nil {
		return SchemaIdentity{}, err
	}
	var identity SchemaIdentity
	copy(identity[:], digest.Sum(nil))
	return identity, nil
}

// Entries returns a defensive copy of the canonical-order plan rows.
func (p CanonicalPlan) Entries() []CanonicalPlanEntry {
	if len(p.entries) == 0 {
		return nil
	}
	out := make([]CanonicalPlanEntry, len(p.entries))
	copy(out, p.entries)
	return out
}

func (p CanonicalPlan) SchemaIdentity() SchemaIdentity { return p.identity }

// InventorySealed reports whether the registry proved its mandatory core
// inventory complete before freezing.
func (p CanonicalPlan) InventorySealed() bool { return p.inventorySealed }

// AuthorityIdentity is available only when the mandatory core inventory was
// explicitly sealed and every declared axis is Ready.
func (p CanonicalPlan) AuthorityIdentity() (SchemaIdentity, bool) {
	if !p.inventorySealed || !p.ready {
		return SchemaIdentity{}, false
	}
	return p.identity, true
}

// PendingAxes returns the canonical-order axes withholding authority.
func (p CanonicalPlan) PendingAxes() []string {
	if len(p.pending) == 0 {
		return nil
	}
	out := make([]string, len(p.pending))
	copy(out, p.pending)
	return out
}
