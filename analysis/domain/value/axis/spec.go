package axis

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// BoundaryPolicy declares how one sparse value axis crosses a function-summary
// boundary. Every registered axis must choose a policy explicitly: boundary
// projection is a semantic operation, so silently treating a new axis as
// portable would be unsound.
type BoundaryPolicy uint8

const (
	BoundaryUnspecified BoundaryPolicy = iota
	// LocalOnly removes the axis constraint at a function boundary by projecting
	// it to the axis top.
	LocalOnly
	// PortableIdentity preserves the axis value exactly.
	PortableIdentity
	// Projected applies BoundaryProject to produce the portable axis value.
	Projected
)

// Spec describes one axis lattice and its stable hashing function.
type Spec[T any] struct {
	Key      Key[T]
	Bottom   func() T
	Top      func() T
	Equal    func(a, b T) bool
	LessOrEq func(a, b T) bool
	Join     func(a, b T) T
	Meet     func(a, b T) T
	Widen    func(prev, next T) T
	Hash     func(T) uint64
	// Retention is mandatory for every registered sparse axis. It proves whether
	// values may cross an immutable artifact boundary.
	Retention RetentionPolicy[T]
	// Canonical is mandatory and explicitly Ready or Pending. Pending axes remain
	// valid solver axes but prevent their registry from publishing canonical
	// authority.
	Canonical CanonicalDescriptor[T]
	// Boundary is mandatory for registered product axes. Projected additionally
	// requires BoundaryProject; the other policies reject a projector so stale
	// configuration cannot be ignored accidentally.
	Boundary        BoundaryPolicy
	BoundaryProject func(T) T
	Reducer         Reducer
	// ReducerReads lists the axis ids the Reducer depends on. Product reduction
	// uses it as a cheap gate: when a value's slots do not carry every listed
	// axis, the reducer cannot fire, so the whole reduce pass is skipped without
	// allocating a reduce editor. Empty means the reducer is always considered.
	ReducerReads []string
}

// Lattice adapts this axis spec to the generic lattice contract.
func (s Spec[T]) Lattice() lattice.Lattice[T] {
	s = s.normalized()
	mustValidate(s)
	l := lattice.Lattice[T]{
		Bottom:   s.Bottom,
		Top:      s.Top,
		Equal:    s.Equal,
		LessOrEq: s.LessOrEq,
		Join:     s.Join,
		Widen:    s.Widen,
	}
	if s.Meet != nil {
		l.Meet = s.Meet
	}
	return l
}

// Erase adapts this typed spec to the erased operations used by the product
// carrier.
func (s Spec[T]) Erase() ErasedSpec {
	s = s.normalized()
	mustValidate(s)
	return erasedSpec[T]{spec: s}
}

func (s Spec[T]) normalized() Spec[T] {
	if s.LessOrEq == nil && s.Equal != nil && s.Join != nil {
		s.LessOrEq = func(a, b T) bool {
			return s.Equal(s.Join(a, b), b)
		}
	}
	if s.Widen == nil {
		s.Widen = s.Join
	}
	return s
}

func mustValidate[T any](s Spec[T]) {
	if err := validate(s); err != nil {
		panic(err)
	}
}

func validate[T any](s Spec[T]) error {
	if s.Key.ID() == "" {
		return fmt.Errorf("axis: spec has empty key")
	}
	if s.Bottom == nil {
		return fmt.Errorf("axis %q: Bottom is nil", s.Key.ID())
	}
	if s.Top == nil {
		return fmt.Errorf("axis %q: Top is nil", s.Key.ID())
	}
	if s.Equal == nil {
		return fmt.Errorf("axis %q: Equal is nil", s.Key.ID())
	}
	if s.LessOrEq == nil {
		return fmt.Errorf("axis %q: LessOrEq is nil", s.Key.ID())
	}
	if s.Join == nil {
		return fmt.Errorf("axis %q: Join is nil", s.Key.ID())
	}
	if s.Widen == nil {
		return fmt.Errorf("axis %q: Widen is nil", s.Key.ID())
	}
	if s.Hash == nil {
		return fmt.Errorf("axis %q: Hash is nil", s.Key.ID())
	}
	if err := validateRetentionPolicy(s.Key.ID(), s.Retention); err != nil {
		return err
	}
	if err := validateCanonicalDescriptor(s.Key.ID(), s.Canonical); err != nil {
		return err
	}
	switch s.Boundary {
	case LocalOnly, PortableIdentity:
		if s.BoundaryProject != nil {
			return fmt.Errorf("axis %q: boundary projector requires Projected policy", s.Key.ID())
		}
	case Projected:
		if s.BoundaryProject == nil {
			return fmt.Errorf("axis %q: Projected boundary policy requires a projector", s.Key.ID())
		}
	default:
		return fmt.Errorf("axis %q: boundary policy is unspecified", s.Key.ID())
	}
	return nil
}

// ErasedSpec is the type-erased adapter consumed by product operations.
type ErasedSpec interface {
	// erasedSpecSeal keeps erased specs provenance-checked: every accepted
	// implementation must come from a fully validated typed Spec.Erase call.
	// The private result also lets Registry reject forged implementations from
	// tests inside this package instead of trusting their metadata methods.
	erasedSpecSeal() erasedSpecToken
	ID() string
	BottomAny() any
	TopAny() any
	IsTopAny(any) bool
	EqualAny(a, b any) bool
	LessOrEqAny(a, b any) bool
	JoinAny(a, b any) any
	HasMeet() bool
	MeetAny(a, b any) any
	WidenAny(prev, next any) any
	HashAny(any) uint64
	RetentionMode() RetentionMode
	RetentionSafeAny(any) bool
	CanonicalStatus() CanonicalStatus
	CanonicalCodecID() string
	CanonicalCodecVersion() uint64
	CanonicalPendingReason() string
	EncodeCanonicalAny(*canonical.Writer, any) error
	BoundaryPolicy() BoundaryPolicy
	ProjectBoundaryAny(any) any
	ReducerHook() Reducer
	ReducerReadsHook() []string
}

type erasedSpec[T any] struct {
	spec Spec[T]
}

type erasedSpecToken struct{ validated bool }

func (erasedSpec[T]) erasedSpecSeal() erasedSpecToken {
	return erasedSpecToken{validated: true}
}

func (e erasedSpec[T]) ID() string {
	return e.spec.Key.ID()
}

func (e erasedSpec[T]) BottomAny() any {
	return e.spec.Bottom()
}

func (e erasedSpec[T]) TopAny() any {
	return e.spec.Top()
}

func (e erasedSpec[T]) IsTopAny(v any) bool {
	return e.spec.Equal(e.cast(v), e.spec.Top())
}

func (e erasedSpec[T]) EqualAny(a, b any) bool {
	return e.spec.Equal(e.cast(a), e.cast(b))
}

func (e erasedSpec[T]) LessOrEqAny(a, b any) bool {
	return e.spec.LessOrEq(e.cast(a), e.cast(b))
}

func (e erasedSpec[T]) JoinAny(a, b any) any {
	return e.spec.Join(e.cast(a), e.cast(b))
}

func (e erasedSpec[T]) HasMeet() bool {
	return e.spec.Meet != nil
}

func (e erasedSpec[T]) MeetAny(a, b any) any {
	if e.spec.Meet == nil {
		panic(fmt.Sprintf("axis %q: Meet is nil", e.ID()))
	}
	return e.spec.Meet(e.cast(a), e.cast(b))
}

func (e erasedSpec[T]) WidenAny(prev, next any) any {
	return e.spec.Widen(e.cast(prev), e.cast(next))
}

func (e erasedSpec[T]) HashAny(v any) uint64 {
	return e.spec.Hash(e.cast(v))
}

func (e erasedSpec[T]) RetentionMode() RetentionMode { return e.spec.Retention.Mode }

func (e erasedSpec[T]) CanonicalStatus() CanonicalStatus { return e.spec.Canonical.status }

func (e erasedSpec[T]) CanonicalCodecID() string { return e.spec.Canonical.codecID }

func (e erasedSpec[T]) CanonicalCodecVersion() uint64 { return e.spec.Canonical.version }

func (e erasedSpec[T]) CanonicalPendingReason() string { return e.spec.Canonical.pendingReason }

func (e erasedSpec[T]) EncodeCanonicalAny(writer *canonical.Writer, value any) error {
	if e.spec.Canonical.status != CanonicalReady || e.spec.Canonical.encode == nil {
		return fmt.Errorf("axis %q: canonical codec is not ready", e.ID())
	}
	return e.spec.Canonical.encode(writer, e.cast(value))
}

func (e erasedSpec[T]) RetentionSafeAny(value any) bool {
	typed, ok := value.(T)
	if !ok {
		return false
	}
	switch e.spec.Retention.Mode {
	case RetentionImmutable:
		return true
	case RetentionValidated:
		return e.spec.Retention.Validate(typed)
	default:
		return false
	}
}

func (e erasedSpec[T]) BoundaryPolicy() BoundaryPolicy {
	return e.spec.Boundary
}

func (e erasedSpec[T]) ProjectBoundaryAny(v any) any {
	typed := e.cast(v)
	switch e.spec.Boundary {
	case LocalOnly:
		return e.spec.Top()
	case PortableIdentity:
		return typed
	case Projected:
		return e.spec.BoundaryProject(typed)
	default:
		panic(fmt.Sprintf("axis %q: boundary policy is unspecified", e.ID()))
	}
}

func (e erasedSpec[T]) ReducerHook() Reducer {
	return e.spec.Reducer
}

func (e erasedSpec[T]) ReducerReadsHook() []string {
	return e.spec.ReducerReads
}

func (e erasedSpec[T]) cast(v any) T {
	tv, ok := v.(T)
	if !ok {
		panic(fmt.Sprintf("axis %q: value has type %T, want registered axis type", e.ID(), v))
	}
	return tv
}
