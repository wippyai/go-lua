package identityvalue

import (
	"bytes"
	"context"
	"errors"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// Registry is the private authority for the portable identity factor.  It is
// deliberately a fresh product registry: values from another registry cannot
// be substituted, compared, or replayed into this one.
//
// This is the symbolic identity lane only.  It does not claim to be the
// Link-owned correlated value Schema; that Schema still has no portable atom
// inventory and remains outside this contract.
type Registry struct {
	product *axis.Registry
}

// NewRegistry creates a fresh, sealed identity product authority.
func NewRegistry() (*Registry, error) {
	reg, err := product.RegistryWithAxes(identity.Spec().Erase())
	if err != nil {
		return nil, err
	}
	return &Registry{product: reg}, nil
}

func (registry *Registry) valid() bool {
	return registry != nil && registry.product != nil && registry.product.Frozen()
}

// Value is an immutable symbolic identity factor value.  Its representation
// retains no registry, Link, or runtime pointer in canonical form; the
// registry is only the authority used to admit hot operations.
type Value struct {
	owner *Registry
	data  product.Value
}

func (value Value) valid() bool {
	return value.owner != nil && value.owner.valid() && product.BelongsToRegistry(value.owner.product, value.data)
}

// Valid reports whether value belongs to one sealed symbolic authority.
func (value Value) Valid() bool { return value.valid() }

// Bottom returns the identity-factor bottom value.
func (registry *Registry) Bottom() Value {
	if !registry.valid() {
		return Value{}
	}
	return Value{owner: registry, data: product.Bottom(registry.product)}
}

// Top returns the identity-factor top value.
func (registry *Registry) Top() Value {
	if !registry.valid() {
		return Value{}
	}
	return Value{owner: registry, data: product.Top()}
}

// Formalize lifts one exact formal root into the identity factor.
func (registry *Registry) Formalize(root formal.Root) (Value, bool) {
	if !registry.valid() || !root.Valid() {
		return Value{}, false
	}
	return registry.withTerm(identity.FormalTerm(root))
}

// Allocation lifts one exact allocation template into the identity factor.
// Allocation templates remain symbolic and are not instantiated by this API.
func (registry *Registry) Allocation(template identity.AllocationTemplate) (Value, bool) {
	if !registry.valid() || !template.Valid() {
		return Value{}, false
	}
	return registry.withTerm(identity.AllocationTerm(template))
}

// Concrete lifts one exact concrete identity into the identity factor.
func (registry *Registry) Concrete(id identity.ID) (Value, bool) {
	if !registry.valid() || id == (identity.ID{}) {
		return Value{}, false
	}
	return registry.withTerm(identity.ConcreteTerm(id))
}

func (registry *Registry) withTerm(term identity.Term) (Value, bool) {
	if !term.Valid() {
		return Value{}, false
	}
	return Value{owner: registry, data: PresentTerm(registry.product, term)}, true
}

// Raw is intentionally absent. Callers can observe the exact symbolic atom,
// but cannot obtain the product registry or its erased slots.
func (value Value) Term() (identity.Term, bool) {
	if !value.valid() {
		return identity.Term{}, false
	}
	return ExactTerm(value.owner.product, value.data)
}

// Equal is the canonical identity-factor equality operation.
func Equal(left, right Value) bool {
	return left.valid() && right.valid() && left.owner == right.owner && product.Equal(left.owner.product, left.data, right.data)
}

// LessOrEq is the canonical identity-factor order operation.
func LessOrEq(left, right Value) bool {
	return left.valid() && right.valid() && left.owner == right.owner && product.LessOrEq(left.owner.product, left.data, right.data)
}

// Join is the canonical identity-factor least upper bound.
func Join(left, right Value) (Value, bool) {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return Value{}, false
	}
	return Value{owner: left.owner, data: product.Join(left.owner.product, left.data, right.data)}, true
}

// Widen is the identity-factor widening operation. The identity axis is
// finite, so its widening is the same pointwise join used by the product.
func Widen(previous, next Value) (Value, bool) {
	if !previous.valid() || !next.valid() || previous.owner != next.owner {
		return Value{}, false
	}
	return Value{owner: previous.owner, data: product.Widen(previous.owner.product, previous.data, next.data)}, true
}

// WidenRank returns the exact identity-axis strict-widening rank. The product
// schema owns the other shape/presence lanes; this factor's symbolic contract
// measures only its typed identity component.
func (value Value) WidenRank() (uint64, bool) {
	if !value.valid() {
		return 0, false
	}
	axisValue := product.Get(value.owner.product, value.data, identity.Key)
	return identity.Spec().WidenRank.At(axisValue, 0), true
}

// CanonicalArtifact contains only deterministic bytes and the sealed schema
// identity. It cannot retain a registry, Link, or runtime pointer.
type CanonicalArtifact struct {
	artifact product.CanonicalArtifact
}

func (artifact CanonicalArtifact) Valid() bool { return artifact.artifact.Valid() }

func (artifact CanonicalArtifact) Bytes() []byte {
	if !artifact.Valid() {
		return nil
	}
	return artifact.artifact.Bytes()
}

func (artifact CanonicalArtifact) SchemaIdentity() axis.SchemaIdentity {
	if !artifact.Valid() {
		return axis.SchemaIdentity{}
	}
	return artifact.artifact.SchemaIdentity()
}

// CanonicalEqual compares values from different fresh authorities without
// weakening the hot owner fence. It is the cold replay/equality witness for
// canonical transport; Equal remains intentionally exact-authority only.
func CanonicalEqual(ctx context.Context, left, right Value) (bool, error) {
	leftArtifact, err := left.Canonical(ctx)
	if err != nil {
		return false, err
	}
	rightArtifact, err := right.Canonical(ctx)
	if err != nil {
		return false, err
	}
	return leftArtifact.SchemaIdentity() == rightArtifact.SchemaIdentity() && bytes.Equal(leftArtifact.Bytes(), rightArtifact.Bytes()), nil
}

// Canonical seals and round-trips one value before publication. Product's
// codec supplies the canonical replay proof and emits no erased Go value or
// process-local authority into the bytes.
func (value Value) Canonical(ctx context.Context) (CanonicalArtifact, error) {
	if !value.valid() {
		return CanonicalArtifact{}, errors.New("identityvalue: invalid value")
	}
	artifact, err := product.SealCanonical(ctx, value.owner.product, value.data)
	if err != nil {
		return CanonicalArtifact{}, err
	}
	return CanonicalArtifact{artifact: artifact}, nil
}

// Replay materializes an artifact under the exact fresh registry schema. The
// registry pointer is supplied out-of-band and never participates in bytes.
func (artifact CanonicalArtifact) Replay(ctx context.Context, registry *Registry) (Value, error) {
	if !artifact.Valid() || !registry.valid() {
		return Value{}, errors.New("identityvalue: invalid canonical replay authority")
	}
	data, err := artifact.artifact.Materialize(ctx, registry.product)
	if err != nil {
		return Value{}, err
	}
	return Value{owner: registry, data: data}, nil
}

// Freshen replays a value into a newly allocated registry. Equal, order, join,
// widen, and the factor rank are preserved by canonical replay while authority
// pointer identity is intentionally replaced.
func Freshen(ctx context.Context, value Value) (*Registry, Value, error) {
	artifact, err := value.Canonical(ctx)
	if err != nil {
		return nil, Value{}, err
	}
	registry, err := NewRegistry()
	if err != nil {
		return nil, Value{}, err
	}
	fresh, err := artifact.Replay(ctx, registry)
	if err != nil {
		return nil, Value{}, err
	}
	return registry, fresh, nil
}

// Binding is one typed formal-to-identity image. Allocation templates are
// intentionally not accepted as substitution variables.
type Binding struct {
	Variable formal.Root
	Image    identity.Value
}

// Substitution is an immutable formal identity substitution fenced to one
// registry. Its underlying term contract preserves Bottom, Singleton, and Top
// images and refuses allocation-template instantiation.
type Substitution struct {
	owner *Registry
	inner identity.Substitution
}

func NewSubstitution(registry *Registry, bindings []Binding) (Substitution, bool) {
	if !registry.valid() {
		return Substitution{}, false
	}
	innerBindings := make([]identity.Binding, len(bindings))
	for index, binding := range bindings {
		if !binding.Variable.Valid() {
			return Substitution{}, false
		}
		innerBindings[index] = identity.Binding{
			Variable: binding.Variable,
			Image:    binding.Image,
		}
	}
	inner, ok := identity.NewSubstitution(innerBindings)
	if !ok {
		return Substitution{}, false
	}
	return Substitution{owner: registry, inner: inner}, true
}

// Apply substitutes the exact identity atom in value. Bottom and Top remain
// unchanged; an unbound formal and an allocation template fail closed rather
// than silently changing the correlated relation.
func (substitution Substitution) Apply(value Value) (Value, bool) {
	if substitution.owner == nil || !value.valid() || value.owner != substitution.owner {
		return Value{}, false
	}
	axisValue := product.Get(substitution.owner.product, value.data, identity.Key)
	term, singleton := axisValue.Term()
	if !singleton {
		return value, true
	}
	image, ok := substitution.inner.Substitute(term)
	if !ok {
		return Value{}, false
	}
	return Value{owner: substitution.owner, data: product.Set(substitution.owner.product, value.data, identity.Key, image)}, true
}
