package axis

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

// Key identifies one typed axis in a value product.
type Key[T any] struct {
	id string
}

// NewKey creates a typed axis key. The id must be stable across runs because it
// participates in product hashing and canonicalization.
func NewKey[T any](id string) Key[T] {
	if id == "" {
		panic("axis: empty key id")
	}
	return Key[T]{id: id}
}

// ID returns the stable erased key id.
func (k Key[T]) ID() string {
	return k.id
}

func (k Key[T]) String() string {
	return k.id
}

// Reader and Writer are the erased view reducers use to inspect and update a
// value product without depending on the product package.
type Reader interface {
	GetAny(key string) (any, bool)
}

type Writer interface {
	Reader
	SetAny(key string, value any)
}

// Reducer is an optional registry hook for reduced products. It may inspect any
// registered axis and update zero or more axes. Returning true requests another
// reducer pass.
type Reducer func(Writer) bool

// Spec describes one axis lattice and its stable hashing function.
type Spec[T any] struct {
	Key      Key[T]
	Bottom   func() T
	Top      func() T
	Equal    func(a, b T) bool
	LessOrEq func(a, b T) bool
	Join     func(a, b T) T
	Widen    func(prev, next T) T
	Hash     func(T) uint64
	Reducer  Reducer
}

// Lattice adapts this axis spec to the generic lattice contract.
func (s Spec[T]) Lattice() lattice.Lattice[T] {
	s = s.normalized()
	mustValidate(s)
	return lattice.Lattice[T]{
		Bottom:   s.Bottom,
		Top:      s.Top,
		Equal:    s.Equal,
		LessOrEq: s.LessOrEq,
		Join:     s.Join,
		Widen:    s.Widen,
	}
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
	WidenAny(prev, next any) any
	HashAny(any) uint64
	ReducerHook() Reducer
	typedSpec() any
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

func (e erasedSpec[T]) WidenAny(prev, next any) any {
	return e.spec.Widen(e.cast(prev), e.cast(next))
}

func (e erasedSpec[T]) HashAny(v any) uint64 {
	return e.spec.Hash(e.cast(v))
}

func (e erasedSpec[T]) ReducerHook() Reducer {
	return e.spec.Reducer
}

func (e erasedSpec[T]) typedSpec() any {
	return e.spec
}

func (e erasedSpec[T]) cast(v any) T {
	tv, ok := v.(T)
	if !ok {
		panic(fmt.Sprintf("axis %q: value has type %T, want registered axis type", e.ID(), v))
	}
	return tv
}

// Registry owns the ordered set of axes in one value product.
type Registry struct {
	specs    map[string]ErasedSpec
	order    []ErasedSpec
	reducers []Reducer
	frozen   bool
}

func NewRegistry() *Registry {
	return &Registry{specs: make(map[string]ErasedSpec)}
}

func (r *Registry) Clone() *Registry {
	if r == nil {
		return nil
	}
	out := NewRegistry()
	out.order = append(out.order, r.order...)
	out.reducers = append(out.reducers, r.reducers...)
	out.frozen = r.frozen
	for k, v := range r.specs {
		out.specs[k] = v
	}
	return out
}

// Freeze makes the registry immutable and returns it for fluent construction.
func (r *Registry) Freeze() *Registry {
	if r == nil {
		panic("axis: nil registry")
	}
	r.frozen = true
	return r
}

func (r *Registry) Frozen() bool {
	return r != nil && r.frozen
}

// Register adds a typed axis spec to the registry.
func Register[T any](r *Registry, spec Spec[T]) {
	if err := r.RegisterErased(spec.Erase()); err != nil {
		panic(err)
	}
}

func (r *Registry) RegisterErased(spec ErasedSpec) error {
	if r == nil {
		return fmt.Errorf("axis: nil registry")
	}
	if spec == nil {
		return fmt.Errorf("axis: nil spec")
	}
	if r.frozen {
		return fmt.Errorf("axis: registry is frozen")
	}
	id := spec.ID()
	if id == "" {
		return fmt.Errorf("axis: empty spec id")
	}
	if _, exists := r.specs[id]; exists {
		return fmt.Errorf("axis: duplicate spec %q", id)
	}
	r.specs[id] = spec
	r.order = append(r.order, spec)
	if reducer := spec.ReducerHook(); reducer != nil {
		r.reducers = append(r.reducers, reducer)
	}
	return nil
}

func (r *Registry) LookupErased(id string) (ErasedSpec, bool) {
	if r == nil {
		return nil, false
	}
	spec, ok := r.specs[id]
	return spec, ok
}

func Lookup[T any](r *Registry, key Key[T]) (Spec[T], bool) {
	var zero Spec[T]
	spec, ok := r.LookupErased(key.ID())
	if !ok {
		return zero, false
	}
	typed, ok := spec.typedSpec().(Spec[T])
	if !ok {
		panic(fmt.Sprintf("axis %q: lookup with incompatible typed key", key.ID()))
	}
	return typed, true
}

func (r *Registry) Specs() []ErasedSpec {
	if r == nil || len(r.order) == 0 {
		return nil
	}
	out := make([]ErasedSpec, len(r.order))
	copy(out, r.order)
	return out
}

func (r *Registry) Reducers() []Reducer {
	if r == nil || len(r.reducers) == 0 {
		return nil
	}
	out := make([]Reducer, len(r.reducers))
	copy(out, r.reducers)
	return out
}

// Get reads a typed value from a reducer view.
func Get[T any](r Reader, key Key[T]) (T, bool) {
	var zero T
	if r == nil {
		return zero, false
	}
	v, ok := r.GetAny(key.ID())
	if !ok {
		return zero, false
	}
	tv, ok := v.(T)
	if !ok {
		panic(fmt.Sprintf("axis %q: reader returned %T, want typed key value", key.ID(), v))
	}
	return tv, true
}

// Set writes a typed value to a reducer view.
func Set[T any](w Writer, key Key[T], value T) {
	if w == nil {
		return
	}
	w.SetAny(key.ID(), value)
}
