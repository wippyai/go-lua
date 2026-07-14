package axis

import "fmt"

// Registry owns the ordered set of axes in one value product.
type Registry struct {
	specs        map[string]ErasedSpec
	order        []ErasedSpec
	reducers     []Reducer
	reducerReads [][]string
	extensions   map[string]map[string]Extension
	extensionSeq map[string][]Extension
	frozen       bool
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
	reads    [][]string
}

func (v ReducersView) Len() int {
	return len(v.reducers)
}

func (v ReducersView) At(i int) Reducer {
	return v.reducers[i]
}

// ReadsAt returns the axis ids the i-th reducer depends on.
func (v ReducersView) ReadsAt(i int) []string {
	return v.reads[i]
}

// Extension is an immutable descriptor registered alongside value-product axes.
// Extensions live in their own kind/id namespace and are ignored by product
// value operations. Engine layers use them to hang verified descriptor tables
// off the same frozen registry instance that already keys domain caches.
type Extension interface {
	ExtensionKind() string
	ExtensionID() string
}

// ExtensionsView is a read-only, allocation-free view of registry extensions
// for one kind.
type ExtensionsView struct {
	extensions []Extension
}

func (v ExtensionsView) Len() int {
	return len(v.extensions)
}

func (v ExtensionsView) At(i int) Extension {
	return v.extensions[i]
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
	if !spec.erasedSpecSeal().validated {
		return fmt.Errorf("axis: erased spec did not originate from validated typed Spec.Erase")
	}
	if r.frozen {
		return fmt.Errorf("axis: registry is frozen")
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
	if _, exists := r.specs[id]; exists {
		return fmt.Errorf("axis: duplicate spec %q", id)
	}
	r.specs[id] = spec
	r.order = append(r.order, spec)
	if reducer := spec.ReducerHook(); reducer != nil {
		r.reducers = append(r.reducers, reducer)
		r.reducerReads = append(r.reducerReads, spec.ReducerReadsHook())
	}
	return nil
}

// RegisterExtension adds a non-product descriptor to the registry.
func (r *Registry) RegisterExtension(ext Extension) error {
	if r == nil {
		return fmt.Errorf("axis: nil registry")
	}
	if ext == nil {
		return fmt.Errorf("axis: nil extension")
	}
	if r.frozen {
		return fmt.Errorf("axis: registry is frozen")
	}
	kind := ext.ExtensionKind()
	if kind == "" {
		return fmt.Errorf("axis: extension has empty kind")
	}
	id := ext.ExtensionID()
	if id == "" {
		return fmt.Errorf("axis: extension %q has empty id", kind)
	}
	if r.extensions == nil {
		r.extensions = make(map[string]map[string]Extension)
	}
	byID := r.extensions[kind]
	if byID == nil {
		byID = make(map[string]Extension)
		r.extensions[kind] = byID
	}
	if _, exists := byID[id]; exists {
		return fmt.Errorf("axis: duplicate extension %q/%q", kind, id)
	}
	byID[id] = ext
	if r.extensionSeq == nil {
		r.extensionSeq = make(map[string][]Extension)
	}
	r.extensionSeq[kind] = append(r.extensionSeq[kind], ext)
	return nil
}

func (r *Registry) LookupErased(id string) (ErasedSpec, bool) {
	if r == nil {
		return nil, false
	}
	spec, ok := r.specs[id]
	return spec, ok
}

func (r *Registry) LookupExtension(kind, id string) (Extension, bool) {
	if r == nil || r.extensions == nil {
		return nil, false
	}
	byID := r.extensions[kind]
	if byID == nil {
		return nil, false
	}
	ext, ok := byID[id]
	return ext, ok
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

func (r *Registry) Extensions(kind string) []Extension {
	if r == nil || len(r.extensionSeq[kind]) == 0 {
		return nil
	}
	out := make([]Extension, len(r.extensionSeq[kind]))
	copy(out, r.extensionSeq[kind])
	return out
}

func (r *Registry) ExtensionsView(kind string) ExtensionsView {
	if r == nil {
		return ExtensionsView{}
	}
	return ExtensionsView{extensions: r.extensionSeq[kind]}
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
	return ReducersView{reducers: r.reducers, reads: r.reducerReads}
}
