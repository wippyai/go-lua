package axis_test

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

func TestCanonicalDescriptorFailsClosedOnMissingAndStaleMetadata(t *testing.T) {
	tests := []struct {
		name string
		edit func(*axis.Spec[int])
	}{
		{"missing", func(spec *axis.Spec[int]) { spec.Canonical = axis.CanonicalDescriptor[int]{} }},
		{"ready-without-encoder", func(spec *axis.Spec[int]) {
			spec.Canonical = axis.ReadyCanonical[int]("test.axis.int", 1, nil)
		}},
		{"ready-without-codec-id", func(spec *axis.Spec[int]) {
			spec.Canonical = axis.ReadyCanonical[int]("", 1, encodeCanonicalInt)
		}},
		{"ready-with-zero-version", func(spec *axis.Spec[int]) {
			spec.Canonical = axis.ReadyCanonical[int]("test.axis.int", 0, encodeCanonicalInt)
		}},
		{"pending-without-reason", func(spec *axis.Spec[int]) {
			spec.Canonical = axis.PendingCanonical[int]("")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := canonicalPlanTestSpec("test.canonical.invalid."+test.name, axis.RetentionImmutable)
			test.edit(&spec)
			defer func() {
				if recover() == nil {
					t.Fatal("invalid canonical metadata did not fail closed")
				}
			}()
			_ = spec.Erase()
		})
	}
}

func TestCanonicalPlanIsRegistrationOrderDeterministic(t *testing.T) {
	first := frozenCanonicalRegistry(t,
		canonicalPlanTestSpec("test.canonical.b", axis.RetentionImmutable),
		canonicalPlanTestSpec("test.canonical.a", axis.RetentionImmutable),
	)
	second := frozenCanonicalRegistry(t,
		canonicalPlanTestSpec("test.canonical.a", axis.RetentionImmutable),
		canonicalPlanTestSpec("test.canonical.b", axis.RetentionImmutable),
	)

	firstPlan, err := first.CanonicalPlan()
	if err != nil {
		t.Fatalf("first CanonicalPlan error = %v", err)
	}
	secondPlan, err := second.CanonicalPlan()
	if err != nil {
		t.Fatalf("second CanonicalPlan error = %v", err)
	}
	if firstPlan.SchemaIdentity() != secondPlan.SchemaIdentity() {
		t.Fatalf("registration order changed schema identity: %x != %x", firstPlan.SchemaIdentity(), secondPlan.SchemaIdentity())
	}
	if got := canonicalPlanIDs(firstPlan); !slices.Equal(got, []string{"test.canonical.a", "test.canonical.b"}) {
		t.Fatalf("canonical plan IDs = %v, want canonical order", got)
	}
	if identity, ok := firstPlan.AuthorityIdentity(); !ok || identity != firstPlan.SchemaIdentity() {
		t.Fatal("all-Ready plan did not publish its schema as canonical authority")
	}
}

func TestCanonicalPlanIdentityIncludesRetentionAndAxisInventory(t *testing.T) {
	immutable := canonicalPlanIdentity(t, canonicalPlanTestSpec("test.canonical.axis", axis.RetentionImmutable))
	validated := canonicalPlanIdentity(t, canonicalPlanTestSpec("test.canonical.axis", axis.RetentionValidated))
	if immutable == validated {
		t.Fatal("retention-mode change did not change canonical schema identity")
	}
	local := canonicalPlanTestSpec("test.canonical.axis", axis.RetentionImmutable)
	local.Boundary = axis.LocalOnly
	if got := canonicalPlanIdentity(t, local); got == immutable {
		t.Fatal("boundary-policy change did not change canonical schema identity")
	}

	one := canonicalPlanIdentity(t, canonicalPlanTestSpec("test.canonical.one", axis.RetentionImmutable))
	two := canonicalPlanIdentity(t,
		canonicalPlanTestSpec("test.canonical.one", axis.RetentionImmutable),
		canonicalPlanTestSpec("test.canonical.two", axis.RetentionImmutable),
	)
	if one == two {
		t.Fatal("adding an axis did not change canonical schema identity")
	}

	changedCodec := canonicalPlanTestSpec("test.canonical.one", axis.RetentionImmutable)
	changedCodec.Canonical = axis.ReadyCanonical[int]("test.axis.int", 2, encodeCanonicalInt)
	if got := canonicalPlanIdentity(t, changedCodec); got == one {
		t.Fatal("stale codec version did not change canonical schema identity")
	}
	changedCodecID := canonicalPlanTestSpec("test.canonical.one", axis.RetentionImmutable)
	changedCodecID.Canonical = axis.ReadyCanonical[int]("test.axis.int.renamed", 1, encodeCanonicalInt)
	if got := canonicalPlanIdentity(t, changedCodecID); got == one {
		t.Fatal("codec ID change did not change canonical schema identity")
	}
}

func TestPendingReasonIsDiagnosticNotSchemaIdentity(t *testing.T) {
	first := canonicalPlanTestSpec("test.canonical.pending", axis.RetentionImmutable)
	first.Canonical = axis.PendingCanonical[int]("first diagnostic explanation")
	second := canonicalPlanTestSpec("test.canonical.pending", axis.RetentionImmutable)
	second.Canonical = axis.PendingCanonical[int]("refined diagnostic explanation")
	if canonicalPlanIdentity(t, first) != canonicalPlanIdentity(t, second) {
		t.Fatal("free-text pending reason leaked into canonical schema identity")
	}
}

func TestCanonicalPlanWithPendingAxisCannotPublishAuthority(t *testing.T) {
	ready := canonicalPlanTestSpec("test.canonical.ready", axis.RetentionImmutable)
	pending := canonicalPlanTestSpec("test.canonical.pending", axis.RetentionImmutable)
	pending.Canonical = axis.PendingCanonical[int]("portable identity is not implemented")

	reg := frozenCanonicalRegistry(t, ready, pending)
	plan, err := reg.CanonicalPlan()
	if err != nil {
		t.Fatalf("CanonicalPlan error = %v", err)
	}
	if _, ok := plan.AuthorityIdentity(); ok {
		t.Fatal("pending plan published canonical authority")
	}
	if got := plan.PendingAxes(); !slices.Equal(got, []string{"test.canonical.pending"}) {
		t.Fatalf("PendingAxes = %v", got)
	}
}

func TestCanonicalPlanRequiresFrozenRegistry(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, canonicalPlanTestSpec("test.canonical.mutable", axis.RetentionImmutable))
	if _, err := reg.CanonicalPlan(); err == nil {
		t.Fatal("mutable registry published a canonical plan")
	}
}

func canonicalPlanTestSpec(id string, retention axis.RetentionMode) axis.Spec[int] {
	policy := axis.ImmutableRetention[int]()
	if retention == axis.RetentionValidated {
		policy = axis.ValidatedRetention(func(int) bool { return true })
	}
	return axis.Spec[int]{
		Key:       axis.NewKey[int](id),
		Bottom:    func() int { return 0 },
		Top:       func() int { return 2 },
		Equal:     func(a, b int) bool { return a == b },
		LessOrEq:  func(a, b int) bool { return a <= b },
		Join:      func(a, b int) int { return max(a, b) },
		Meet:      func(a, b int) int { return min(a, b) },
		Hash:      func(v int) uint64 { return uint64(v) },
		Retention: policy,
		Boundary:  axis.PortableIdentity,
		Canonical: axis.ReadyCanonical("test.axis.int", 1, encodeCanonicalInt),
	}
}

func encodeCanonicalInt(writer *canonical.Writer, value int) error {
	return writer.Int(int64(value))
}

func frozenCanonicalRegistry(t *testing.T, specs ...axis.Spec[int]) *axis.Registry {
	t.Helper()
	reg := axis.NewRegistry()
	for _, spec := range specs {
		axis.Register(reg, spec)
	}
	return reg.Freeze()
}

func canonicalPlanIdentity(t *testing.T, specs ...axis.Spec[int]) axis.SchemaIdentity {
	t.Helper()
	plan, err := frozenCanonicalRegistry(t, specs...).CanonicalPlan()
	if err != nil {
		t.Fatalf("CanonicalPlan error = %v", err)
	}
	return plan.SchemaIdentity()
}

func canonicalPlanIDs(plan axis.CanonicalPlan) []string {
	entries := plan.Entries()
	ids := make([]string, len(entries))
	for i, entry := range entries {
		ids[i] = entry.AxisID
	}
	return ids
}
