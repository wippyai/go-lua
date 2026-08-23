package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// ActivationRule is the typed mounted activation admission surface retained
// by the composed binding. Ordinary operand rules never implement it.
//
// A refusal carries the rule's own evidence erased, exactly as an axis mount
// hook does: the composition names which rule refused and hands that evidence
// on unread, so no activation vocabulary crosses this interface.
type ActivationRule interface {
	MountedAdmit(mount, point, occurrence identity.ContentID, contexts executioncontext.Directory) (engine.MountedActivationAdmit, axis.Cell, bool)
}

// RuleContributor is the compose-owned typed pass for one catalog row. The
// catalog retains only rule.Template values; these hooks live in the schema
// composition root and are never carried by a catalog declaration.
type RuleContributor[P, A any] struct {
	// generated names the lane of the compact schema-program arm, and
	// rule.LaneInvalid names a hand-wired row. It intentionally carries no
	// domain callback or typed hot payload: the composition derives and binds
	// the engine's GeneratedRuleSlot directly from the sealed plan catalog, and
	// the lane is the only thing that arm still has to be told.
	generated         rule.Lane
	declare           func(*engine.SchemaBuilder, vocabulary.Roles, P) (rule.Cell, bool)
	register          func(*engine.SchemaBinding, rule.Cell) (engine.RuleSlotCapability, bool)
	pair              func(*engine.SchemaBinding, rule.Cell, func(schema.Key) (engine.RuleSlotCapability, bool)) bool
	bind              func(*engine.SchemaBinding, A, rule.Cell) (rule.Cell, ActivationRule, bool)
	finalize          func(A, rule.Cell) bool
	occurrenceCatalog func(rule.Cell) (rule.OccurrenceCatalog, bool)
}

// generatedLaneHandoff is the one table naming which lanes have a generated
// registration. Both the wiring check and the registration pass read it, so a
// lane cannot be admitted by one and refused by the other.
func generatedLaneHandoff(lane rule.Lane) (func(*engine.SchemaBinding, *engine.GeneratedRuleSlot) (engine.RuleSlotCapability, bool), bool) {
	switch lane {
	case rule.LaneMounted:
		return engine.RegisterMountedGeneratedSlot, true
	case rule.LaneLink:
		return engine.RegisterLinkGeneratedSlot, true
	default:
		return nil, false
	}
}

func (contributor RuleContributor[P, A]) Declare(builder *engine.SchemaBuilder, roles vocabulary.Roles, principals P) (rule.Cell, bool) {
	if contributor.generated.Available() {
		return rule.Cell{}, false
	}
	if contributor.declare == nil {
		return rule.Cell{}, false
	}
	return contributor.declare(builder, roles, principals)
}

// DeclareGenerated derives one generated Rule slot from the one sealed plan
// catalog owned by the composition declaration transaction. The caller passes
// the canonical sealed Rule ordinal; no domain package supplies geometry,
// semantic identities, or an alternate plan catalog.
func (contributor RuleContributor[P, A]) DeclareGenerated(builder *engine.SchemaBuilder, plans ruleplan.Catalog, ordinal uint32) (rule.Cell, bool) {
	if !contributor.generated.Available() {
		return rule.Cell{}, false
	}
	slot, ok := engine.DeclareGeneratedRuleSlot(builder, plans, ordinal)
	if !ok || slot == nil || !slot.Available() {
		return rule.Cell{}, false
	}
	return rule.NewCell(slot), true
}

func (contributor RuleContributor[P, A]) Register(binding *engine.SchemaBinding, holder rule.Cell) (engine.RuleSlotCapability, bool) {
	if contributor.generated.Available() {
		register, laneOK := generatedLaneHandoff(contributor.generated)
		slot, ok := rule.Payload[*engine.GeneratedRuleSlot](holder)
		if !laneOK || !ok || slot == nil {
			return engine.RuleSlotCapability{}, false
		}
		return register(binding, slot)
	}
	if contributor.register == nil {
		return engine.RuleSlotCapability{}, false
	}
	return contributor.register(binding, holder)
}

func (contributor RuleContributor[P, A]) Pair(binding *engine.SchemaBinding, holder rule.Cell, resolve func(schema.Key) (engine.RuleSlotCapability, bool)) bool {
	return contributor.pair == nil || contributor.pair(binding, holder, resolve)
}

func (contributor RuleContributor[P, A]) Bind(binding *engine.SchemaBinding, authorities A, holder rule.Cell) (rule.Cell, ActivationRule, bool) {
	if contributor.generated.Available() {
		slot, ok := rule.Payload[*engine.GeneratedRuleSlot](holder)
		if !ok || slot == nil || !engine.BindGeneratedRule(binding, slot) {
			return rule.Cell{}, nil, false
		}
		// The generated slot is the complete hot identity. Keeping this same
		// opaque slot cell lets the table retain no HotRule/HotOwner payload.
		return holder, nil, true
	}
	if contributor.bind == nil {
		return rule.Cell{}, nil, false
	}
	return contributor.bind(binding, authorities, holder)
}

func (contributor RuleContributor[P, A]) Finalize(authorities A, holder rule.Cell) bool {
	return contributor.finalize == nil || contributor.finalize(authorities, holder)
}

// OccurrenceCatalog is the inventory of occurrences one rule's lane admits.
// A hand-wired rule states it through the owner callback its wiring supplied;
// a generated rule has no callback and needs none - the engine derives the
// inventory from the candidate relation its sealed plan already names.
func (contributor RuleContributor[P, A]) OccurrenceCatalog(binding *engine.SchemaBinding, holder rule.Cell) (rule.OccurrenceCatalog, bool) {
	if contributor.generated.Available() {
		slot, ok := rule.Payload[*engine.GeneratedRuleSlot](holder)
		if !ok || slot == nil {
			return nil, false
		}
		inventory, inventoryOK := engine.GeneratedOccurrenceCatalog(binding, slot)
		return inventory, inventoryOK
	}
	if contributor.occurrenceCatalog == nil {
		return nil, false
	}
	return contributor.occurrenceCatalog(holder)
}

// WiringRefusal names why one catalog row's contributor and its template do
// not describe the same rule. It is a closed vocabulary rather than a bare
// false, because the two halves of a row are authored in different packages:
// a mismatch that surfaces as an anonymous failure at a slot ordinal costs the
// reader the whole walk to find which rule and which half.
//
// The zero value is the admitted verdict.
type WiringRefusal uint8

const (
	WiringAdmitted WiringRefusal = iota
	// WiringTemplateAbsent is a row whose declaration itself was rejected.
	WiringTemplateAbsent
	// WiringProgramWiredByHand is a template that carries a callback-free
	// Program wired through WireRule. The composition owns slot declaration and
	// binding for a Program, so a hand-wired hook set would declare the slot a
	// second time; the declaration says generated and the wiring says hand, and
	// only one of them can be executed.
	WiringProgramWiredByHand
	// WiringGeneratedWithoutProgram is the converse: a row wired generated whose
	// template declares no Program, so the composition has nothing to lower.
	WiringGeneratedWithoutProgram
	// WiringGeneratedLaneUnsupported is a generated row on a lane that has no
	// generated registration handoff.
	WiringGeneratedLaneUnsupported
	// WiringHooksMissing is a hand-wired row lacking one of the three passes
	// every hand-wired rule must supply.
	WiringHooksMissing
	// WiringOccurrenceCatalogMissing is a Link or mounted-point row that
	// supplies no owner-issued occurrence inventory.
	WiringOccurrenceCatalogMissing
	// WiringOccurrenceCatalogUnexpected is an artifact-materialized row that
	// supplies one anyway.
	WiringOccurrenceCatalogUnexpected
)

func (refusal WiringRefusal) String() string {
	switch refusal {
	case WiringTemplateAbsent:
		return "template-absent"
	case WiringProgramWiredByHand:
		return "program-wired-by-hand"
	case WiringGeneratedWithoutProgram:
		return "generated-without-program"
	case WiringGeneratedLaneUnsupported:
		return "generated-lane-unsupported"
	case WiringHooksMissing:
		return "hooks-missing"
	case WiringOccurrenceCatalogMissing:
		return "occurrence-catalog-missing"
	case WiringOccurrenceCatalogUnexpected:
		return "occurrence-catalog-unexpected"
	default:
		return "admitted"
	}
}

// RuleWiringFailure is the named verdict of one refused catalog row.
type RuleWiringFailure struct {
	Rule    schema.Key
	Refusal WiringRefusal
}

func (failure RuleWiringFailure) Available() bool { return failure.Refusal != WiringAdmitted }

func (failure RuleWiringFailure) String() string {
	if !failure.Available() {
		return "admitted"
	}
	return string(failure.Rule) + "/" + failure.Refusal.String()
}

func (contributor RuleContributor[P, A]) complete(template *rule.Template) WiringRefusal {
	if template == nil {
		return WiringTemplateAbsent
	}
	if contributor.generated.Available() {
		if !template.Program().Available() {
			return WiringGeneratedWithoutProgram
		}
		if _, laneOK := generatedLaneHandoff(template.Lane()); !laneOK || template.Lane() != contributor.generated {
			return WiringGeneratedLaneUnsupported
		}
		return WiringAdmitted
	}
	if template.Program().Available() {
		return WiringProgramWiredByHand
	}
	if contributor.declare == nil || contributor.register == nil || contributor.bind == nil {
		return WiringHooksMissing
	}
	if template.Lane() == rule.LaneLink || template.Lane() == rule.LaneMountedPoint {
		if contributor.occurrenceCatalog == nil {
			return WiringOccurrenceCatalogMissing
		}
		return WiringAdmitted
	}
	if contributor.occurrenceCatalog != nil {
		return WiringOccurrenceCatalogUnexpected
	}
	return WiringAdmitted
}

// WireGeneratedRule admits a callback-free Rule program into the generic
// composition path. Its contributor carries only the generated marker; the
// composition owns all engine slot declaration, binding, and capability
// issuance, so the owning domain retains no engine fragment or hot callback.
func WireGeneratedRule[P, A any](spec rule.Spec) (*rule.Template, RuleContributor[P, A], bool) {
	template, ok := rule.New(spec)
	if !ok || template == nil || !template.Program().Available() {
		return nil, RuleContributor[P, A]{}, false
	}
	if _, laneOK := generatedLaneHandoff(template.Lane()); !laneOK {
		return nil, RuleContributor[P, A]{}, false
	}
	return template, RuleContributor[P, A]{generated: template.Lane()}, true
}

// WireRule binds a domain's typed declaration, owner registration, and hot
// implementation registration to one catalog row. Construction later resolves
// the sealed canonical schema cell by its parent-issued capability; no erased
// callback-bearing issuer is retained here.
func WireRule[P, A, F, H any](
	spec rule.Spec,
	declare func(*engine.SchemaBuilder, rule.Declaration[P]) (F, bool),
	register func(*engine.SchemaBinding, rule.Registration[F]) (engine.RuleSlotCapability, bool),
	pair func(*engine.SchemaBinding, rule.Pairing[F], func(schema.Key) (engine.RuleSlotCapability, bool)) bool,
	bind func(*engine.SchemaBinding, rule.Binding[A, F]) (H, bool),
	finalize func(rule.Finalization[A, H]) bool,
	catalog func(H) (rule.OccurrenceCatalog, bool),
	activation func(H) ActivationRule,
) (*rule.Template, RuleContributor[P, A], bool) {
	if declare == nil || register == nil || bind == nil {
		return nil, RuleContributor[P, A]{}, false
	}
	template, ok := rule.New(spec)
	if !ok {
		return nil, RuleContributor[P, A]{}, false
	}
	roles := template.DeclaredRoles()
	contributor := RuleContributor[P, A]{
		declare: func(builder *engine.SchemaBuilder, view vocabulary.Roles, owners P) (rule.Cell, bool) {
			if builder == nil {
				return rule.Cell{}, false
			}
			restricted, rolesOK := view.Restrict(roles...)
			if !rolesOK {
				return rule.Cell{}, false
			}
			fragment, declared := declare(builder, rule.Declaration[P]{Roles: restricted, Principals: owners})
			if !declared {
				return rule.Cell{}, false
			}
			return rule.NewCell(fragment), true
		},
		register: func(binding *engine.SchemaBinding, holder rule.Cell) (engine.RuleSlotCapability, bool) {
			fragment, ok := rule.Payload[F](holder)
			if !ok {
				return engine.RuleSlotCapability{}, false
			}
			return register(binding, rule.Registration[F]{Fragment: fragment})
		},
		bind: func(binding *engine.SchemaBinding, set A, holder rule.Cell) (rule.Cell, ActivationRule, bool) {
			fragment, fragmentOK := rule.Payload[F](holder)
			if !fragmentOK {
				return rule.Cell{}, nil, false
			}
			hot, bound := bind(binding, rule.Binding[A, F]{Fragment: fragment, Authorities: set})
			if !bound {
				return rule.Cell{}, nil, false
			}
			var activationRule ActivationRule
			if activation != nil {
				activationRule = activation(hot)
				if activationRule == nil {
					return rule.Cell{}, nil, false
				}
			}
			return rule.NewCell(hot), activationRule, true
		},
	}
	if pair != nil {
		contributor.pair = func(binding *engine.SchemaBinding, holder rule.Cell, resolve func(schema.Key) (engine.RuleSlotCapability, bool)) bool {
			fragment, ok := rule.Payload[F](holder)
			return ok && pair(binding, rule.Pairing[F]{Fragment: fragment}, resolve)
		}
	}
	if finalize != nil {
		contributor.finalize = func(set A, holder rule.Cell) bool {
			hot, ok := rule.Payload[H](holder)
			return ok && finalize(rule.Finalization[A, H]{Rule: hot, Authorities: set})
		}
	}
	if catalog != nil {
		contributor.occurrenceCatalog = func(holder rule.Cell) (rule.OccurrenceCatalog, bool) {
			hot, hotOK := rule.Payload[H](holder)
			if !hotOK {
				return nil, false
			}
			inventory, ok := catalog(hot)
			return inventory, ok && inventory != nil
		}
	}
	return template, contributor, contributor.complete(template) == WiringAdmitted
}
