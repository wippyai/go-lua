package axis

import "fmt"

// Registry owns the ordered set of axes in one value product.
type Registry struct {
	specs                    map[string]ErasedSpec
	order                    []ErasedSpec
	canonicalCore            map[string]ErasedSpec
	canonicalCoreOrder       []ErasedSpec
	canonicalInventorySealed bool
	reducers                 []Reducer
	reducerOwners            []string
	reducerReads             [][]string
	reducerWrites            [][]string
	frozen                   bool
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
	owners   []string
	reads    [][]string
	writes   [][]string
}

func (v ReducersView) Len() int {
	return len(v.reducers)
}

func (v ReducersView) At(i int) Reducer {
	return v.reducers[i]
}

// OwnerAt returns the axis id whose spec registered the i-th reducer.
func (v ReducersView) OwnerAt(i int) string {
	return v.owners[i]
}

// ReadsAt returns the axis ids the i-th reducer depends on.
func (v ReducersView) ReadsAt(i int) []string {
	return v.reads[i]
}

// WritesAt returns the axis ids the i-th reducer may restrict.
func (v ReducersView) WritesAt(i int) []string {
	return v.writes[i]
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

// RegisterCanonicalCore adds an always-present product lane to canonical
// registry metadata without registering it as a sparse product slot.
func RegisterCanonicalCore[T any](r *Registry, spec Spec[T]) {
	if err := r.registerCanonicalCoreErased(spec.Erase()); err != nil {
		panic(err)
	}
}

func (r *Registry) registerCanonicalCoreErased(spec ErasedSpec) error {
	if err := r.validateRegistration(spec); err != nil {
		return err
	}
	id := spec.ID()
	if _, exists := r.specs[id]; exists {
		return fmt.Errorf("axis: duplicate spec %q", id)
	}
	if r.canonicalCore == nil {
		r.canonicalCore = make(map[string]ErasedSpec)
	}
	if _, exists := r.canonicalCore[id]; exists {
		return fmt.Errorf("axis: duplicate canonical core spec %q", id)
	}
	r.canonicalCore[id] = spec
	r.canonicalCoreOrder = append(r.canonicalCoreOrder, spec)
	return nil
}

func (r *Registry) RegisterErased(spec ErasedSpec) error {
	if err := r.validateRegistration(spec); err != nil {
		return err
	}
	id := spec.ID()
	if _, exists := r.canonicalCore[id]; exists {
		return fmt.Errorf("axis: duplicate spec %q", id)
	}
	if _, exists := r.specs[id]; exists {
		return fmt.Errorf("axis: duplicate spec %q", id)
	}
	r.specs[id] = spec
	r.order = append(r.order, spec)
	if reducer := spec.ReducerHook(); reducer != nil {
		r.reducers = append(r.reducers, reducer)
		r.reducerOwners = append(r.reducerOwners, id)
		r.reducerReads = append(r.reducerReads, reducerAxes(spec.ReducerReadsHook(), id))
		r.reducerWrites = append(r.reducerWrites, reducerAxes(spec.ReducerWritesHook(), id))
	}
	return nil
}

func (r *Registry) validateRegistration(spec ErasedSpec) error {
	if r == nil {
		return fmt.Errorf("axis: nil registry")
	}
	if spec == nil {
		return fmt.Errorf("axis: nil spec")
	}
	if !spec.erasedSpecSeal().validated {
		return fmt.Errorf("axis: erased spec did not originate from validated typed Spec.Erase")
	}
	if r.frozen {
		return fmt.Errorf("axis: registry is frozen")
	}
	if r.canonicalInventorySealed {
		return fmt.Errorf("axis: canonical inventory is sealed")
	}
	id := spec.ID()
	if id == "" {
		return fmt.Errorf("axis: empty spec id")
	}
	switch spec.RetentionMode() {
	case RetentionImmutable, RetentionValidated:
	default:
		return fmt.Errorf("axis %q: erased artifact retention policy is unspecified", id)
	}
	switch spec.BoundaryPolicy() {
	case LocalOnly, PortableIdentity, Projected:
	default:
		return fmt.Errorf("axis %q: erased boundary policy is unspecified", id)
	}
	switch spec.CanonicalStatus() {
	case CanonicalReady:
		if spec.CanonicalCodecID() == "" {
			return fmt.Errorf("axis %q: erased ready canonical codec id is empty", id)
		}
		if spec.CanonicalCodecVersion() == 0 {
			return fmt.Errorf("axis %q: erased ready canonical codec version is zero", id)
		}
	case CanonicalPending:
		if spec.CanonicalPendingReason() == "" {
			return fmt.Errorf("axis %q: erased pending canonical reason is empty", id)
		}
	default:
		return fmt.Errorf("axis %q: erased canonical descriptor is unspecified", id)
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
	return ReducersView{
		reducers: r.reducers,
		owners:   r.reducerOwners,
		reads:    r.reducerReads,
		writes:   r.reducerWrites,
	}
}

func reducerAxes(configured []string, owner string) []string {
	if configured == nil {
		return []string{owner}
	}
	result := make([]string, len(configured))
	copy(result, configured)
	return result
}
