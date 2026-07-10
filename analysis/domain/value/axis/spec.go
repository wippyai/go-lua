package axis

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
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
	Reducer  Reducer
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
	return nil
}

// ErasedSpec is the type-erased adapter consumed by product operations.
type ErasedSpec interface {
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
	ReducerHook() Reducer
	ReducerReadsHook() []string
}

type erasedSpec[T any] struct {
	spec Spec[T]
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
