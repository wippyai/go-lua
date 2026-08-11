// Package assemble lowers Target's static transfer outcomes through the one
// activation façade. It deliberately owns no alternate Call/Target index:
// Link, Call, Pack, and Target remain the respective authorities.
package assemble

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	escapeowner "github.com/wippyai/go-lua/analysis/domain/escape/owner"
	escaperule "github.com/wippyai/go-lua/analysis/domain/escape/rule"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

// Spec supplies the already-declared domain owners and authored semantic
// identities for Escape's structural source rule.  IDs for applications,
// operations, and outcomes are obtained directly from Link/Target below.
type Spec struct {
	Link        *link.Link
	Calls       *callowner.Owner
	Packs       *packowner.Owner
	Escapes     *escapeowner.Owner
	Transfer    *escaperule.Rule
	Semantic    engine.SemanticKey
	Family      engine.SemanticKey
	Admission   engine.SemanticKey
	PayloadRole engine.SemanticKey
	PayloadSlot engine.SemanticKey
}

// Source is the narrow source compiler. endpoints is the one static
// Target-owned outcome list; it is never crossed with applications.
type Source struct {
	composition *engine.Composition
	link        *link.Link
	calls       *callowner.Owner
	packs       *packowner.Owner
	transfer    *escaperule.Rule
	family      engine.ActivationFamily
	rule        *engine.ActivationRule
	callRead    engine.Read[engine.OrderedCells[calldomain.Value]]
	catalog     staticCatalog
	prepared    *engine.PreparedActivationPlan
	plan        *engine.ActivationPlan
	payloadRole engine.SemanticKey
	payloadSlot engine.SemanticKey
}

// Declare records one exact Call read and the activation decision.  Static
// transfer enumeration is deferred until the composition has sealed.
func Declare(composition *engine.Composition, spec Spec) (*Source, bool) {
	if composition == nil || spec.Link == nil || spec.Calls == nil || spec.Packs == nil || spec.Escapes == nil || spec.Transfer == nil ||
		!spec.Semantic.Available() || !spec.Family.Available() || !spec.Admission.Available() || !spec.PayloadRole.Available() || !spec.PayloadSlot.Available() ||
		spec.PayloadSlot == spec.PayloadRole || spec.PayloadSlot == spec.Semantic || spec.PayloadSlot == spec.Family || spec.PayloadSlot == spec.Admission ||
		spec.Calls.Algebra() == nil || spec.Packs.Schema() == nil || !spec.Escapes.Schema().Valid() ||
		!spec.Transfer.MatchesLink(spec.Link) || spec.Calls.Link() != spec.Link || spec.Packs.Schema().Link() != spec.Link || spec.Escapes.Schema().Link() != spec.Link {
		return nil, false
	}
	contract, ok := spec.Link.Boundary().Target()
	if !ok || contract == nil {
		return nil, false
	}
	source := &Source{composition: composition, link: spec.Link, calls: spec.Calls, packs: spec.Packs, transfer: spec.Transfer, payloadRole: spec.PayloadRole, payloadSlot: spec.PayloadSlot}
	family, familyOK := engine.DeclareActivationFamily(composition, spec.Family)
	if !familyOK {
		return nil, false
	}
	rule, ruleOK := engine.DeclareActivationRule(composition, engine.ActivationRuleSpec{
		Semantic: spec.Semantic, Family: family, Inputs: 1, Admission: engine.AdmitActivationByTrustedTheorem(spec.Admission),
		Declare: func(rule *engine.ActivationRule) bool {
			input, inputOK := rule.InputAt(0)
			read, readOK := engine.ReadFrom(rule, input, source.calls.ExactRead())
			source.callRead = read
			return inputOK && readOK
		},
		Run: source.run,
	})
	if !ruleOK || rule == nil {
		return nil, false
	}
	source.family, source.rule = family, rule
	return source, true
}

func (source *Source) run(activation engine.Activation) bool {
	application, applicationOK := engine.ActivationApplication(activation)
	cells, readOK := engine.ActivationReadValue(activation, source.callRead)
	if !applicationOK || !readOK || cells.Count() != 1 {
		return false
	}
	value, present, available := cells.At(0)
	if !available || !present {
		return true
	}
	if value.HasOpaqueAlternative() {
		for _, item := range source.catalog.transfers {
			if !engine.Activate(activation, application, item.target, item.endpoint) {
				return false
			}
		}
		return true
	}
	for index := 0; index < value.KnownTargetCount(); index++ {
		known, knownOK := value.KnownTargetAt(index)
		if !knownOK {
			return false
		}
		operation, operationOK := known.Operation()
		// A known Lua function target is a precise Call alternative but has no
		// Target operation. It contributes no native transfer endpoint; only a
		// malformed capability is a structural failure.
		if !operationOK {
			continue
		}
		if !source.activateOperation(activation, application, operation) {
			return false
		}
	}
	return true
}

func (source *Source) activateOperation(activation engine.Activation, application engine.SemanticKey, operation target.Operation) bool {
	for _, item := range source.catalog.byOperation[operation] {
		if item.operation == operation && !engine.Activate(activation, application, item.target, item.endpoint) {
			return false
		}
	}
	return true
}

// Stage enumerates Target's deliverable transfer outcomes once and stages
// their exact prototypes in the caller-owned shared SourceAssembly. Every
// endpoint has the same one-Pack import ABI, including scalar, tail, and
// whole-Pack declarations. This source never owns a private Batch or seal.
func (source *Source) Stage(assembly *engine.SourceAssembly) bool {
	if source == nil || source.composition == nil || !source.composition.Sealed() || source.plan != nil || source.prepared != nil || assembly == nil || !source.link.ContentID().Available() {
		return false
	}
	catalog, enumerated := buildStaticCatalog(source.link)
	if !enumerated || len(catalog.transfers) == 0 {
		return false
	}
	entries := make([]engine.ActivationPlanEntry, 0, len(catalog.transfers))
	for _, item := range catalog.transfers {
		instance, instanceOK := source.transfer.Prototype(item.operand, source.payloadRole, source.payloadSlot)
		admission, admissionOK := engine.ActivationPrototypeAdmissionFor(item.endpoint, instance)
		if !instanceOK || instance == nil || !admissionOK {
			return false
		}
		entries = append(entries, engine.ActivationPlanEntry{
			Target: item.target, Endpoint: item.endpoint,
			PortRole: source.payloadRole, Provenance: item.target, Prototype: admission,
		})
	}
	prepared, preparedOK := engine.StageActivationPlan(assembly, source.composition, source.family, entries)
	if !preparedOK || prepared == nil {
		return false
	}
	source.catalog, source.prepared = catalog, prepared
	return true
}

// Finalize materializes this source's staged immutable activation catalog only
// after the shared SourceAssembly has crossed its global seal barrier.
func (source *Source) Finalize(assembly *engine.SourceAssembly) (*engine.ActivationPlan, bool) {
	if source == nil || source.plan != nil || source.prepared == nil || len(source.catalog.transfers) == 0 || assembly == nil {
		return nil, false
	}
	plan, planOK := engine.FinalizeActivationPlan(assembly, source.prepared)
	if !planOK || plan == nil {
		return nil, false
	}
	source.plan = plan
	return plan, true
}

// Trigger creates the one application-specific structural instance. The
// shared plan remains static; only its typed Call read and one exact Call
// Pack root are bound to this Link Call application.
func (source *Source) Trigger(application linkproject.Application, base engine.ActivationBase) (*engine.StructuralInstance, bool) {
	if source == nil || source.plan == nil || source.rule == nil || source.calls == nil || source.packs == nil {
		return nil, false
	}
	project := source.link.Project()
	if project == nil {
		return nil, false
	}
	applicationID, applicationIDOK := project.ApplicationID(application)
	applicationKey, applicationKeyOK := engine.NewSemanticKey([32]byte(applicationID), staticSemanticVersion)
	callKey, callKeyOK := source.calls.Algebra().KeyForApplication(application)
	callRef, callRefOK := source.calls.Locate(callKey)
	root, rootOK := source.packs.Schema().CallRoot(application)
	packRef, packRefOK := source.packs.Locate(root)
	if !applicationIDOK || !applicationID.Available() || !applicationKeyOK || !callKeyOK || !callRefOK || !rootOK || !packRefOK {
		return nil, false
	}
	port, portOK := engine.NewActivationPort(source.payloadRole, base)
	if !portOK {
		return nil, false
	}
	if !source.packs.AddActivationPortRead(port, source.payloadSlot, packRef) {
		return nil, false
	}
	instance, instanceOK := engine.NewActivationTrigger(source.rule, applicationKey, source.plan, []*engine.ActivationPort{port}, func(binding *engine.StructuralBinding) bool {
		return engine.StructuralRead(binding, source.callRead, callRef)
	})
	return instance, instanceOK
}
