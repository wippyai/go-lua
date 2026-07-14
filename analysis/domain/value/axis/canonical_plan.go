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
	entries  []CanonicalPlanEntry
	identity SchemaIdentity
	ready    bool
	pending  []string
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
	plan := CanonicalPlan{entries: entries, identity: identity, ready: true}
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

// AuthorityIdentity is available only when every axis is explicitly Ready.
func (p CanonicalPlan) AuthorityIdentity() (SchemaIdentity, bool) {
	if !p.ready {
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
