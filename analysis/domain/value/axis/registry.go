package axis

import "fmt"

// Registry owns the ordered set of axes in one value product.
type Registry struct {
	specs    map[string]ErasedSpec
	order    []ErasedSpec
	reducers []Reducer
	frozen   bool
}

// SpecsView is a read-only, allocation-free view of a registry's ordered specs.
type SpecsView struct {
	specs []ErasedSpec
}

func (v SpecsView) Len() int {
	return len(v.specs)
}

func (v SpecsView) At(i int) ErasedSpec {
	return v.specs[i]
}

// ReducersView is a read-only, allocation-free view of a registry's reducers.
type ReducersView struct {
	reducers []Reducer
}

func (v ReducersView) Len() int {
	return len(v.reducers)
}

func (v ReducersView) At(i int) Reducer {
	return v.reducers[i]
}

func NewRegistry() *Registry {
	return &Registry{specs: make(map[string]ErasedSpec)}
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

func (r *Registry) Specs() []ErasedSpec {
	if r == nil || len(r.order) == 0 {
		return nil
	}
	out := make([]ErasedSpec, len(r.order))
	copy(out, r.order)
	return out
}

func (r *Registry) SpecsView() SpecsView {
	if r == nil {
		return SpecsView{}
	}
	return SpecsView{specs: r.order}
}

func (r *Registry) Reducers() []Reducer {
	if r == nil || len(r.reducers) == 0 {
		return nil
	}
	out := make([]Reducer, len(r.reducers))
	copy(out, r.reducers)
	return out
}

func (r *Registry) ReducersView() ReducersView {
	if r == nil {
		return ReducersView{}
	}
	return ReducersView{reducers: r.reducers}
}
