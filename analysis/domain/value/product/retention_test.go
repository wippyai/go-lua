package product

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

type retentionTestValue struct {
	rank int
	safe bool
}

func TestRetentionSafeIsAxisOwnedAndRegistryFenced(t *testing.T) {
	key := axis.NewKey[retentionTestValue]("test.retention")
	newRegistry := func() *axis.Registry {
		reg := axis.NewRegistry()
		axis.Register(reg, axis.Spec[retentionTestValue]{
			Key:      key,
			Bottom:   func() retentionTestValue { return retentionTestValue{} },
			Top:      func() retentionTestValue { return retentionTestValue{rank: 2, safe: true} },
			Equal:    func(a, b retentionTestValue) bool { return a == b },
			LessOrEq: func(a, b retentionTestValue) bool { return a.rank <= b.rank },
			Join: func(a, b retentionTestValue) retentionTestValue {
				if a.rank > b.rank {
					return a
				}
				return b
			},
			Meet: func(a, b retentionTestValue) retentionTestValue {
				if a.rank < b.rank {
					return a
				}
				return b
			},
			Hash:      func(v retentionTestValue) uint64 { return uint64(v.rank)<<1 | boolBit(v.safe) },
			Boundary:  axis.PortableIdentity,
			Retention: axis.ValidatedRetention(func(value retentionTestValue) bool { return value.safe }),
		})
		return reg.Freeze()
	}
	reg := newRegistry()
	foreign := newRegistry()
	safe := Set(reg, Top(), key, retentionTestValue{rank: 1, safe: true})
	unsafe := Set(reg, Top(), key, retentionTestValue{rank: 1, safe: false})
	if !RetentionSafe(reg, safe) {
		t.Fatal("axis-owned safe value was rejected")
	}
	if RetentionSafe(reg, unsafe) {
		t.Fatal("axis-owned unsafe value crossed artifact boundary")
	}
	if RetentionSafe(foreign, safe) {
		t.Fatal("foreign-registry value crossed artifact boundary")
	}
}

func TestRetentionSafeUsesCanonicalSlotOrdinals(t *testing.T) {
	immutableKey := axis.NewKey[int]("z.immutable")
	validatedKey := axis.NewKey[string]("a.validated")
	reg := axis.NewRegistry()
	// Registration order is intentionally the reverse of canonical axis order,
	// and the payload types differ. This catches treating a product slot ordinal
	// as an index into Registry.SpecsView (which preserves registration order).
	axis.Register(reg, axis.Spec[int]{
		Key:       immutableKey,
		Bottom:    func() int { return 0 },
		Top:       func() int { return 2 },
		Equal:     func(a, b int) bool { return a == b },
		LessOrEq:  func(a, b int) bool { return a <= b },
		Join:      func(a, b int) int { return max(a, b) },
		Meet:      func(a, b int) int { return min(a, b) },
		Hash:      func(v int) uint64 { return uint64(v) },
		Boundary:  axis.PortableIdentity,
		Retention: axis.ImmutableRetention[int](),
	})
	rank := func(v string) int {
		switch v {
		case "":
			return 0
		case "safe", "unsafe":
			return 1
		default:
			return 2
		}
	}
	axis.Register(reg, axis.Spec[string]{
		Key:      validatedKey,
		Bottom:   func() string { return "" },
		Top:      func() string { return "top" },
		Equal:    func(a, b string) bool { return a == b },
		LessOrEq: func(a, b string) bool { return rank(a) <= rank(b) },
		Join: func(a, b string) string {
			if rank(a) >= rank(b) {
				return a
			}
			return b
		},
		Meet: func(a, b string) string {
			if rank(a) <= rank(b) {
				return a
			}
			return b
		},
		Hash: func(v string) uint64 {
			if v == "unsafe" {
				return 3
			}
			return uint64(rank(v))
		},
		Boundary:  axis.PortableIdentity,
		Retention: axis.ValidatedRetention(func(v string) bool { return v != "unsafe" }),
	})
	reg.Freeze()

	if !RetentionSafe(reg, Bottom(reg)) {
		t.Fatal("safe bottom payloads across every canonical slot were rejected")
	}
	allSlots := Set(reg, Set(reg, Top(), immutableKey, 1), validatedKey, "safe")
	if !RetentionSafe(reg, allSlots) {
		t.Fatal("safe mixed-policy payloads in every canonical slot were rejected")
	}
	unsafe := Set(reg, allSlots, validatedKey, "unsafe")
	if got := Get(reg, unsafe, validatedKey); got != "unsafe" {
		t.Fatalf("unsafe payload was not stored: got %q", got)
	}
	if RetentionSafe(reg, unsafe) {
		t.Fatal("validated unsafe payload crossed artifact boundary")
	}
}

type mutableTopRetentionValue struct {
	rank int
	safe bool
}

func TestRetentionSafeRejectsRegistryWithUnsafeImplicitTop(t *testing.T) {
	key := axis.NewKey[mutableTopRetentionValue]("test.mutable-top")
	reg, err := RegistryWithAxes(axis.Spec[mutableTopRetentionValue]{
		Key:    key,
		Bottom: func() mutableTopRetentionValue { return mutableTopRetentionValue{safe: true} },
		Top:    func() mutableTopRetentionValue { return mutableTopRetentionValue{rank: 2} },
		Equal:  func(a, b mutableTopRetentionValue) bool { return a == b },
		LessOrEq: func(a, b mutableTopRetentionValue) bool {
			return a.rank <= b.rank
		},
		Join: func(a, b mutableTopRetentionValue) mutableTopRetentionValue {
			if a.rank >= b.rank {
				return a
			}
			return b
		},
		Meet: func(a, b mutableTopRetentionValue) mutableTopRetentionValue {
			if a.rank <= b.rank {
				return a
			}
			return b
		},
		Hash: func(v mutableTopRetentionValue) uint64 {
			return uint64(v.rank)<<1 | boolBit(v.safe)
		},
		Boundary: axis.PortableIdentity,
		Retention: axis.ValidatedRetention(func(v mutableTopRetentionValue) bool {
			return v.safe
		}),
	}.Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes: %v", err)
	}
	if RetentionSafe(reg, Top()) {
		t.Fatal("registry-neutral product Top hid unsafe implicit axis Top")
	}
	explicitSafe := Set(reg, Top(), key, mutableTopRetentionValue{rank: 1, safe: true})
	if RetentionSafe(reg, explicitSafe) {
		t.Fatal("registry with unsafe implicit Top gained artifact authority")
	}
}

func boolBit(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}
