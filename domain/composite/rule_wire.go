package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// ActivationRule is the typed mounted activation admission surface retained
// by the composed binding. Ordinary operand rules never implement it.
type ActivationRule interface {
	MountedAdmit(mount, point, occurrence identity.ContentID) (engine.MountedActivationAdmit, bool)
}

// RuleContributor is the compose-owned typed pass for one catalog row. The
// catalog retains only rule.Template values; these hooks live in the schema
// composition root and are never carried by a catalog declaration.
type RuleContributor[P, A any] struct {
	declare     func(*engine.SchemaBuilder, vocabulary.Roles, P) (rule.Cell, bool)
	register    func(*engine.SchemaBinding, rule.Cell) (engine.RuleSlotCapability, bool)
	pair        func(*engine.SchemaBinding, rule.Cell, func(schema.Key) (engine.RuleSlotCapability, bool)) bool
	bind        func(*engine.SchemaBinding, A, rule.Cell) (rule.Cell, ActivationRule, bool)
	finalize    func(A, rule.Cell) bool
	linkCatalog func(rule.Cell) (rule.LinkCatalog, bool)
}

func (contributor RuleContributor[P, A]) Declare(builder *engine.SchemaBuilder, roles vocabulary.Roles, principals P) (rule.Cell, bool) {
	if contributor.declare == nil {
		return rule.Cell{}, false
	}
	return contributor.declare(builder, roles, principals)
}

func (contributor RuleContributor[P, A]) Register(binding *engine.SchemaBinding, holder rule.Cell) (engine.RuleSlotCapability, bool) {
	if contributor.register == nil {
		return engine.RuleSlotCapability{}, false
	}
	return contributor.register(binding, holder)
}

func (contributor RuleContributor[P, A]) Pair(binding *engine.SchemaBinding, holder rule.Cell, resolve func(schema.Key) (engine.RuleSlotCapability, bool)) bool {
	return contributor.pair == nil || contributor.pair(binding, holder, resolve)
}

func (contributor RuleContributor[P, A]) Bind(binding *engine.SchemaBinding, authorities A, holder rule.Cell) (rule.Cell, ActivationRule, bool) {
	if contributor.bind == nil {
		return rule.Cell{}, nil, false
	}
	return contributor.bind(binding, authorities, holder)
}

func (contributor RuleContributor[P, A]) Finalize(authorities A, holder rule.Cell) bool {
	return contributor.finalize == nil || contributor.finalize(authorities, holder)
}

func (contributor RuleContributor[P, A]) LinkCatalog(holder rule.Cell) (rule.LinkCatalog, bool) {
	if contributor.linkCatalog == nil {
		return nil, false
	}
	return contributor.linkCatalog(holder)
}

func (contributor RuleContributor[P, A]) complete(template *rule.Template) bool {
	if contributor.declare == nil || contributor.register == nil || contributor.bind == nil {
		return false
	}
	if template == nil {
		return false
	}
	if template.Lane() == rule.LaneLink {
		return contributor.linkCatalog != nil
	}
	return contributor.linkCatalog == nil
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
	catalog func(H) (rule.LinkCatalog, bool),
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
		contributor.linkCatalog = func(holder rule.Cell) (rule.LinkCatalog, bool) {
			hot, hotOK := rule.Payload[H](holder)
			if !hotOK {
				return nil, false
			}
			inventory, ok := catalog(hot)
			return inventory, ok && inventory != nil
		}
	}
	return template, contributor, contributor.complete(template)
}
