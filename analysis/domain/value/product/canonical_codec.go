package product

import (
	"context"
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const (
	canonicalProductDomain  = "analysis.value-product"
	canonicalProductVersion = 1

	canonicalProductRecord  uint64 = 1
	canonicalPresenceRecord uint64 = 2
	canonicalSparseRecord   uint64 = 3
)

// EncodeCanonical returns the portable canonical encoding of value together
// with the exact registry schema authority governing those bytes.
//
// Authority is fail-closed: reg must be frozen, its mandatory inventory must
// be sealed, and every axis codec must be Ready. The returned byte slice and
// schema identity are both zero on every error, including cancellation or an
// axis encoder failure, so a partial stream can never be published.
func EncodeCanonical(ctx context.Context, reg *axis.Registry, value Value) ([]byte, axis.SchemaIdentity, error) {
	rt := runtimeFor(reg)
	if rt.err != nil {
		return nil, axis.SchemaIdentity{}, rt.err
	}
	codec := rt.canonicalValueCodec()
	if codec.err != nil {
		return nil, axis.SchemaIdentity{}, codec.err
	}
	if err := validateCanonicalProductValue(rt, value); err != nil {
		return nil, axis.SchemaIdentity{}, err
	}

	var writer canonical.Writer
	if err := writer.ResetBuffer(ctx, canonicalProductDomain, canonicalProductVersion); err != nil {
		return nil, axis.SchemaIdentity{}, err
	}
	if err := writer.Record(canonicalProductRecord); err != nil {
		return nil, axis.SchemaIdentity{}, err
	}
	if err := writer.Bytes(codec.authority[:]); err != nil {
		return nil, axis.SchemaIdentity{}, err
	}
	if err := writer.Uint(uint64(ShapeOf(value))); err != nil {
		return nil, axis.SchemaIdentity{}, err
	}
	if err := writer.Record(canonicalPresenceRecord); err != nil {
		return nil, axis.SchemaIdentity{}, err
	}
	if err := codec.presence.EncodeCanonicalAny(&writer, PresenceOf(value)); err != nil {
		return nil, axis.SchemaIdentity{}, fmt.Errorf("product: encode canonical presence: %w", err)
	}
	if err := writer.Count(uint64(len(codec.sparse))); err != nil {
		return nil, axis.SchemaIdentity{}, err
	}
	for _, info := range codec.sparse {
		if err := writer.Record(canonicalSparseRecord); err != nil {
			return nil, axis.SchemaIdentity{}, err
		}
		if err := writer.String(info.id); err != nil {
			return nil, axis.SchemaIdentity{}, err
		}
		payload, present := lookupSlot(value, info.ordinal)
		if err := writer.Bool(present); err != nil {
			return nil, axis.SchemaIdentity{}, err
		}
		if present {
			if err := info.spec.EncodeCanonicalAny(&writer, payload); err != nil {
				return nil, axis.SchemaIdentity{}, fmt.Errorf("product: encode canonical axis %q: %w", info.id, err)
			}
		}
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		return nil, axis.SchemaIdentity{}, err
	}
	return encoded, codec.authority, nil
}

type canonicalProductCodec struct {
	authority axis.SchemaIdentity
	presence  axis.ErasedSpec
	sparse    []axisRuntimeAxis
	err       error
}

func (rt *registryRuntime) canonicalValueCodec() canonicalProductCodec {
	rt.canonicalCodecOnce.Do(func() {
		rt.canonicalCodec = buildCanonicalProductCodec(rt)
	})
	return rt.canonicalCodec
}

func buildCanonicalProductCodec(rt *registryRuntime) canonicalProductCodec {
	plan, err := rt.reg.CanonicalPlan()
	if err != nil {
		return canonicalProductCodec{err: fmt.Errorf("product: canonical registry plan: %w", err)}
	}
	authority, ok := plan.AuthorityIdentity()
	if !ok {
		return canonicalProductCodec{err: fmt.Errorf(
			"product: canonical registry authority unavailable (inventory sealed=%t, pending axes=%v)",
			plan.InventorySealed(), plan.PendingAxes(),
		)}
	}
	presenceSpec, sparse, err := canonicalProductInventory(rt, plan)
	if err != nil {
		return canonicalProductCodec{err: err}
	}
	return canonicalProductCodec{authority: authority, presence: presenceSpec, sparse: sparse}
}

func canonicalProductInventory(rt *registryRuntime, plan axis.CanonicalPlan) (axis.ErasedSpec, []axisRuntimeAxis, error) {
	presenceSpec := presence.Spec().Erase()
	entries := plan.Entries()
	if len(entries) != len(rt.axes)+1 {
		return nil, nil, fmt.Errorf("product: canonical registry inventory has %d entries, want %d", len(entries), len(rt.axes)+1)
	}
	sparse := make([]axisRuntimeAxis, 0, len(rt.axes))
	seenPresence := false
	previousID := ""
	for i, entry := range entries {
		if i > 0 && previousID >= entry.AxisID {
			return nil, nil, fmt.Errorf("product: canonical registry plan is not in strict AxisID order")
		}
		previousID = entry.AxisID
		if entry.AxisID == presence.Key.ID() {
			if seenPresence || !canonicalEntryMatchesSpec(entry, presenceSpec) {
				return nil, nil, fmt.Errorf("product: canonical presence inventory does not match the product presence codec")
			}
			seenPresence = true
			continue
		}
		info, ok := rt.axis(entry.AxisID)
		if !ok || !canonicalEntryMatchesSpec(entry, info.spec) {
			return nil, nil, fmt.Errorf("product: canonical sparse inventory mismatch for axis %q", entry.AxisID)
		}
		sparse = append(sparse, info)
	}
	if !seenPresence || len(sparse) != len(rt.axes) {
		return nil, nil, fmt.Errorf("product: canonical registry inventory is incomplete")
	}
	return presenceSpec, sparse, nil
}

func canonicalEntryMatchesSpec(entry axis.CanonicalPlanEntry, spec axis.ErasedSpec) bool {
	return spec != nil &&
		entry.AxisID == spec.ID() &&
		entry.Retention == spec.RetentionMode() &&
		entry.Boundary == spec.BoundaryPolicy() &&
		entry.Status == spec.CanonicalStatus() &&
		entry.CodecID == spec.CanonicalCodecID() &&
		entry.CodecVersion == spec.CanonicalCodecVersion()
}

func validateCanonicalProductValue(rt *registryRuntime, value Value) error {
	if value.n == nil {
		return nil
	}
	if value.n.reg != rt.reg {
		return fmt.Errorf("product: value belongs to a different registry")
	}
	if value.n.shape != ShapeBottom && value.n.shape != ShapeTop {
		return fmt.Errorf("product: invalid shape %d", value.n.shape)
	}
	if !validPresence(value.n.presence) {
		return fmt.Errorf("product: invalid presence %d", value.n.presence)
	}
	normalShape, normalPresence, _ := reducePresenceShape(value.n.shape, value.n.presence)
	if normalShape != value.n.shape || !presence.Equal(normalPresence, value.n.presence) {
		return fmt.Errorf("product: shape and presence are not in canonical reduced form")
	}
	if value.n.shape == ShapeTop && presence.Equal(value.n.presence, presence.Top()) && len(value.n.slots) == 0 {
		return fmt.Errorf("product: explicit node represents registry-neutral Top")
	}
	var previous uint16
	for i, current := range value.n.slots {
		if int(current.ordinal) >= len(rt.axes) {
			return fmt.Errorf("product: sparse slot ordinal %d is outside the registry", current.ordinal)
		}
		if i > 0 && previous >= current.ordinal {
			return fmt.Errorf("product: sparse slots are not in strict canonical order")
		}
		previous = current.ordinal
		info := rt.axisOrdinal(current.ordinal)
		if reflect.TypeOf(current.value) != info.topType {
			return fmt.Errorf("product: axis %q has payload type %T, want %v", info.id, current.value, info.topType)
		}
		if info.spec.IsTopAny(current.value) {
			return fmt.Errorf("product: axis %q stores a noncanonical explicit Top slot", info.id)
		}
	}
	if rt.isProductBottom(value.n.presence, value.n.slots) &&
		!rt.sameNode(value.n, ShapeBottom, presence.Bottom(), rt.bottomSlots) {
		return fmt.Errorf("product: value is not in canonical product-Bottom form")
	}
	return nil
}

func validPresence(value presence.Value) bool {
	return presence.Equal(value, presence.Bottom()) ||
		presence.Equal(value, presence.Present()) ||
		presence.Equal(value, presence.Absent()) ||
		presence.Equal(value, presence.Maybe())
}
