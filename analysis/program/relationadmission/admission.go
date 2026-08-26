package relationadmission

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// InventoryFactory binds one checked certificate to the owner-issued logical
// inventory used by witness.Specialize.  The certificate parameter is
// intentional: the inventory fence must certify the exact checked schema;
// admission must not manufacture or guess that fence itself.
type InventoryFactory interface {
	Bind(certificate.Certificate) (witness.Inventory, bool)
}

// GeometryFactory binds one sealed mount to an already-sealed physical scope
// authority.  It is a capability factory instead of a Region callback so this
// layer cannot reopen cofiber translation or build a parallel geometry.
type GeometryFactory interface {
	Bind(witness.Mounted) (geometry.Geometry, bool)
}

// Input is the complete owner-supplied admission request. Declaration data is
// already resolved; semantic and physical authorities are injected at their
// native boundaries rather than being reconstructed here.
type Input struct {
	Declaration relcompile.Declaration
	Inventory   InventoryFactory
	Bindings    binding.Factory
	Algebras    binding.AlgebraRegistry
	Lineage     lineage.Factory
	Geometry    GeometryFactory
}

// Ready is the opaque handoff for a relation solve.  Its accessors expose the
// same mounted, geometry, and immutable database authorities created by the
// existing owners; it stores no copied plan, state, or snapshot projection.
type Ready struct {
	mounted  witness.Mounted
	geometry geometry.Geometry
	base     database.Version
	sealed   bool
}

// Available reports whether this handoff retains one mutually authenticated
// mount, geometry view, and committed base root.
func (ready Ready) Available() bool {
	return ready.sealed && ready.mounted.Available() && ready.geometry.ValidFor(ready.mounted) && ready.base.Available() && ready.base.Mounted().Same(ready.mounted) && ready.base.Fence().Same(ready.mounted.RuntimeFence())
}

// Mounted returns the exact mounted runtime capability.
func (ready Ready) Mounted() witness.Mounted {
	if !ready.Available() {
		return witness.Mounted{}
	}
	return ready.mounted
}

// Geometry returns the exact physical scope authority bound to Mounted.
func (ready Ready) Geometry() geometry.Geometry {
	if !ready.Available() {
		return geometry.Geometry{}
	}
	return ready.geometry
}

// Base returns the immutable database root after all initial publications.
func (ready Ready) Base() database.Version {
	if !ready.Available() {
		return database.Version{}
	}
	return ready.base
}

// RefusalCode names the admission boundary that refused a request.  It is
// distinct from certificate issues and semantic outcomes, both of which keep
// their native typed representations on Refusal.
type RefusalCode uint8

const (
	RefusalInvalidInput RefusalCode = iota + 1
	RefusalCompile
	RefusalCertificate
	RefusalInventory
	RefusalMount
	RefusalGeometry
	RefusalBootstrap
	RefusalInitialPublication
	RefusalInitialInput
	RefusalInitialScope
	RefusalInitialProvenance
	RefusalInitialApplication
	RefusalInitialSettlement
	RefusalInitialOutcome
)

func (code RefusalCode) String() string {
	switch code {
	case RefusalInvalidInput:
		return "invalid input"
	case RefusalCompile:
		return "declaration compilation"
	case RefusalCertificate:
		return "certificate"
	case RefusalInventory:
		return "inventory"
	case RefusalMount:
		return "mount"
	case RefusalGeometry:
		return "geometry"
	case RefusalBootstrap:
		return "database bootstrap"
	case RefusalInitialPublication:
		return "initial publication"
	case RefusalInitialInput:
		return "initial publication requires a zero-input signature"
	case RefusalInitialScope:
		return "initial publication scope"
	case RefusalInitialProvenance:
		return "initial publication provenance"
	case RefusalInitialApplication:
		return "initial publication application"
	case RefusalInitialSettlement:
		return "initial publication settlement"
	case RefusalInitialOutcome:
		return "initial publication outcome"
	default:
		return "unknown admission"
	}
}

// Refusal is one typed admission refusal.  Certificate issues and a rejected
// initial publication's semantic outcome remain owned by their existing
// layers; this wrapper never translates them into strings or replacement
// state.
type Refusal struct {
	code       RefusalCode
	issues     []certificate.Issue
	initial    plan.Initial
	hasInitial bool
	outcome    outcome.Result
}

func newRefusal(code RefusalCode) *Refusal {
	return &Refusal{code: code}
}

func certificateRefusal(issues []certificate.Issue) *Refusal {
	return &Refusal{code: RefusalCertificate, issues: append([]certificate.Issue(nil), issues...)}
}

func initialRefusal(code RefusalCode, initial plan.Initial) *Refusal {
	return &Refusal{code: code, initial: initial, hasInitial: true}
}

func outcomeRefusal(initial plan.Initial, result outcome.Result) *Refusal {
	return &Refusal{code: RefusalInitialOutcome, initial: initial, hasInitial: true, outcome: result}
}

// Code identifies the exact admission boundary that refused the request.
func (refusal *Refusal) Code() RefusalCode {
	if refusal == nil {
		return 0
	}
	return refusal.code
}

// CertificateIssues returns the underlying checker findings, in their native
// deterministic order.  It is non-nil only for RefusalCertificate.
func (refusal *Refusal) CertificateIssues() []certificate.Issue {
	if refusal == nil {
		return nil
	}
	return append([]certificate.Issue(nil), refusal.issues...)
}

// Initial returns the schema-sealed initial declaration that failed after
// admission reached the initial-publication phase.
func (refusal *Refusal) Initial() (plan.Initial, bool) {
	if refusal == nil || !refusal.hasInitial {
		return plan.Initial{}, false
	}
	return refusal.initial, true
}

// Outcome returns the exact semantic terminal result only when an otherwise
// valid initial invocation did not produce a publishable result.
func (refusal *Refusal) Outcome() outcome.Result {
	if refusal == nil || !refusal.outcome.Available() {
		return outcome.Result{}
	}
	return refusal.outcome
}

// Error implements error while preserving typed details through the accessors
// above.  A nil refusal is success and therefore has no error text.
func (refusal *Refusal) Error() string {
	if refusal == nil {
		return ""
	}
	return "relation admission: " + refusal.code.String()
}

// Admit composes declaration compilation, independent certificate checking,
// mount specialization, geometry construction, database bootstrap, and any
// zero-input owner seeds.  Every substantive validation remains at its native
// owner; this function refuses at a boundary rather than adding a fallback,
// copied identity table, or alternate state representation.
func Admit(input Input) (Ready, *Refusal) {
	if input.Inventory == nil || input.Bindings == nil || input.Algebras == nil || input.Lineage == nil || input.Geometry == nil {
		return Ready{}, newRefusal(RefusalInvalidInput)
	}

	compiled, compileErr := relcompile.Compile(input.Declaration)
	if compileErr != nil || !compiled.Available() {
		return Ready{}, newRefusal(RefusalCompile)
	}
	cert, checked := certificate.Check(compiled)
	if checked != nil || !cert.Available() {
		if checked == nil {
			return Ready{}, newRefusal(RefusalCertificate)
		}
		return Ready{}, certificateRefusal(checked.Issues())
	}

	inventory, inventoryOK := input.Inventory.Bind(cert)
	if !inventoryOK || inventory == nil {
		return Ready{}, newRefusal(RefusalInventory)
	}
	mounted, mountedOK := witness.Specialize(cert, inventory, input.Bindings, input.Algebras, input.Lineage)
	if !mountedOK || !mounted.Available() {
		return Ready{}, newRefusal(RefusalMount)
	}
	view, geometryOK := input.Geometry.Bind(mounted)
	if !geometryOK || !view.ValidFor(mounted) {
		return Ready{}, newRefusal(RefusalGeometry)
	}
	base, baseOK := database.Bootstrap(mounted, view)
	if !baseOK || !base.Available() {
		return Ready{}, newRefusal(RefusalBootstrap)
	}
	seeded, refused := publishInitials(mounted, view, base, mounted.Initials())
	if refused != nil {
		return Ready{}, refused
	}
	ready := Ready{mounted: mounted, geometry: view, base: seeded, sealed: true}
	if !ready.Available() {
		return Ready{}, newRefusal(RefusalBootstrap)
	}
	return ready, nil
}

func publishInitials(mounted witness.Mounted, view geometry.Geometry, base database.Version, initials []plan.Initial) (database.Version, *Refusal) {
	if len(initials) == 0 {
		return base, nil
	}
	door, doorOK := publish.New(mounted, view)
	if !doorOK || !door.Available() {
		return database.Version{}, newRefusal(RefusalInitialSettlement)
	}
	scratch := store.NewReadScratch(view.Manager())
	if scratch == nil || !scratch.Available() {
		return database.Version{}, newRefusal(RefusalInitialSettlement)
	}

	current := base
	for _, initial := range initials {
		bound, boundOK := mounted.Binding(initial.Operation())
		if !boundOK || bound == nil {
			return database.Version{}, initialRefusal(RefusalInitialPublication, initial)
		}
		operation := bound.Signature()
		if !operation.Available() || operation.Identity() != initial.Operation() {
			return database.Version{}, initialRefusal(RefusalInitialPublication, initial)
		}
		if operation.InputLen() != 0 {
			return database.Version{}, initialRefusal(RefusalInitialInput, initial)
		}
		scope, scopeOK := mounted.Scope(initial.Scope())
		if !scopeOK {
			return database.Version{}, initialRefusal(RefusalInitialScope, initial)
		}
		provenance, provenanceOK := initialProvenance(mounted, operation)
		if !provenanceOK {
			return database.Version{}, initialRefusal(RefusalInitialProvenance, initial)
		}
		application, applicationOK := apply.Apply(mounted, initial.Operation(), scope, provenance, binding.NewOwnerNamedDestination(operation.Outputs()[0].Relation))
		if !applicationOK || !application.Available() {
			return database.Version{}, initialRefusal(RefusalInitialApplication, initial)
		}
		settlement := door.Publish(current, scratch, application, witness.WideningPermit{})
		if !settlement.Available() {
			return database.Version{}, initialRefusal(RefusalInitialSettlement, initial)
		}
		result := settlement.Outcome()
		if !result.Available() {
			return database.Version{}, outcomeRefusal(initial, result)
		}
		if result.Code != outcome.Produced {
			return database.Version{}, outcomeRefusal(initial, result)
		}
		current = settlement.Next()
		if !current.Available() {
			return database.Version{}, initialRefusal(RefusalInitialSettlement, initial)
		}
	}
	return current, nil
}

// initialProvenance redeems every explicit output denominator of a zero-input
// operation. There is no operation-wide destination authority: when an
// initial operation has several output denominators, their owner-issued
// lineage witnesses are joined by the mounted lineage authority.
func initialProvenance(mounted witness.Mounted, operation signature.Signature) (model.LineageRef, bool) {
	if !mounted.Available() || !operation.Available() || operation.OutputLen() == 0 {
		return model.LineageRef{}, false
	}
	authority, ok := mounted.Lineage()
	if !ok || authority == nil {
		return model.LineageRef{}, false
	}
	var result model.LineageRef
	for _, output := range operation.Outputs() {
		if !output.Denominator.Available() {
			return model.LineageRef{}, false
		}
		ref, refOK := mounted.DenominatorLineage(output.Denominator)
		if !refOK {
			return model.LineageRef{}, false
		}
		if !result.Available() {
			result = ref
			continue
		}
		result, ok = authority.Join(result, ref)
		if !ok {
			return model.LineageRef{}, false
		}
	}
	return result, result.Available()
}
