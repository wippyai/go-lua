package catalog

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	"github.com/wippyai/go-lua/analysis/schema/rule/relinput"
	"github.com/wippyai/go-lua/analysis/schema/seal"
)

// Declarations is one explicit, environment-owned declaration input. It is
// intentionally just an ordered set of schema surfaces: the concrete owner
// supplies the rows, while this package supplies the one seal boundary. A
// Declarations value is not shared between environments and has no process
// lifetime.
//
// Register preserves the caller's phase order. seal.Builder rejects a
// duplicate, out-of-order, or malformed surface at Seal; this wrapper does
// not sort or reconstruct a second inventory.
type Declarations struct {
	surfaces []seal.Surface
	rejected bool
	sealed   bool
}

// NewDeclarations opens an empty declaration input for one environment.
func NewDeclarations() *Declarations { return &Declarations{} }

// Register adds one concrete owner surface in schema catalog order. A
// declaration input cannot be changed after Seal, so a compiled result is
// immutable and its digest is stable for the exact authored order.
func (declarations *Declarations) Register(surface seal.Surface) bool {
	if declarations == nil || declarations.sealed || declarations.rejected || surface == nil {
		return false
	}
	if surface.Kind() == schema.SurfaceKindInvalid {
		declarations.rejected = true
		return false
	}
	declarations.surfaces = append(declarations.surfaces, surface)
	return true
}

// Compilation is the immutable result of sealing one explicit declaration
// input. It carries the sealed declaration table and its neutral publication
// projection. No domain roster, executable callback, or process-global state
// is retained here.
type Compilation struct {
	schema      *seal.Schema
	rulePlans   ruleplan.Catalog
	publication Publication
	digest      identity.ContentID
	ok          bool
}

// RulePlans returns the dense, rule-ordinal-aligned execution plans derived
// from the complete sealed schema. The plans carry the schema digest as their
// only identity; they are not a second declaration table.
func (compilation Compilation) RulePlans() (ruleplan.Catalog, bool) {
	if !compilation.Available() || !compilation.rulePlans.Available() {
		return ruleplan.Catalog{}, false
	}
	return compilation.rulePlans, true
}

// InputBundle seals the relation input bundle for this compilation's own rule
// catalog. The ordinals a bundle is addressed by are the ordinals this
// compilation numbered its rules with, so the catalog the bundle is fenced to
// is supplied here and never by its caller.
//
// Placement is not a declaration fact and is not recoverable from one: which
// relation-schema conjunction a rule's candidate rows are decided at, and
// which one each declared input port observes, is decided where the rule is
// composed. The composition that placed the rules answers all of it; this
// boundary adds the catalog and seals the two together.
//
// A compilation that did not seal states no rule ordinals, and the seal
// refuses at the catalog boundary rather than publishing an empty table.
func (compilation Compilation) InputBundle(owner model.OwnerID, composition relinput.Composition) (*relinput.Bundle, *relinput.Refusal) {
	plans, _ := compilation.RulePlans()
	return relinput.Seal(plans, owner, composition)
}

// Available reports whether this compilation sealed completely.
func (compilation Compilation) Available() bool {
	return compilation.ok && compilation.schema != nil && compilation.schema.Available() && compilation.digest.Available()
}

// Schema returns the immutable declaration table.
func (compilation Compilation) Schema() *seal.Schema {
	if !compilation.Available() {
		return nil
	}
	return compilation.schema
}

// Publication returns the immutable neutral projection derived from the same
// declaration table. It is not recomputed by consumers.
func (compilation Compilation) Publication() (Publication, bool) {
	if !compilation.Available() || !compilation.publication.Available() {
		return Publication{}, false
	}
	return compilation.publication, true
}

// Digest is the declaration identity carried by the compiled environment.
func (compilation Compilation) Digest() identity.ContentID {
	if !compilation.Available() {
		return identity.ContentID{}
	}
	return compilation.digest
}

// Seal validates and freezes the explicit declaration input. The first
// failure is returned unchanged; no partial Schema or publication escapes.
func (declarations *Declarations) Seal() (Compilation, schema.SealFailure) {
	if declarations == nil || declarations.sealed || declarations.rejected {
		return Compilation{}, schema.SealFailure{Law: seal.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
	}
	declarations.sealed = true
	if len(declarations.surfaces) == 0 {
		return Compilation{}, schema.SealFailure{Law: seal.LawSurfaceCatalog, Disposition: schema.DispositionIncomplete}
	}
	builder := seal.NewBuilder()
	registrationFailed := false
	for _, surface := range declarations.surfaces {
		if !builder.Register(surface) {
			registrationFailed = true
			break
		}
	}
	sealed, failure := builder.Seal()
	if registrationFailed && !failure.Available() {
		return Compilation{}, schema.SealFailure{Law: seal.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
	}
	if failure.Available() || sealed == nil || !sealed.Available() {
		return Compilation{}, failure
	}
	rulePlans, failure := ruleplan.Compile(sealed)
	if failure.Available() || !rulePlans.Available() {
		return Compilation{}, failure
	}
	publication, ok := CompilePublication(sealed)
	if !ok {
		return Compilation{}, schema.SealFailure{Law: seal.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
	}
	digest := identity.ContentID(sealed.Digest())
	if !digest.Available() {
		return Compilation{}, schema.SealFailure{Law: seal.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
	}
	return Compilation{schema: sealed, rulePlans: rulePlans, publication: publication, digest: digest, ok: true}, schema.SealFailure{}
}
