package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// ruleContributor is the composition-owned wiring for one sealed rule.
type ruleContributor struct {
	declare     func(*engine.SchemaBuilder, vocabulary.Roles, principals) (rule.Cell, bool)
	register    func(*engine.SchemaBinding, rule.Cell) (engine.RuleSlotCapability, bool)
	pair        func(*engine.SchemaBinding, rule.Cell, func(schema.Key) (engine.RuleSlotCapability, bool)) bool
	bind        func(*engine.SchemaBinding, authorities, rule.Cell) (rule.Cell, bool)
	finalize    func(authorities, rule.Cell) bool
	linkCatalog func(rule.Cell) (rule.LinkCatalog, bool)
}

func (contributor ruleContributor) complete(lane rule.Lane) bool {
	if contributor.declare == nil || contributor.register == nil || contributor.bind == nil {
		return false
	}
	if lane == rule.LaneLink {
		return contributor.linkCatalog != nil
	}
	return contributor.linkCatalog == nil
}

func wireRule[F, H any](
	spec rule.Spec,
	declare func(*engine.SchemaBuilder, rule.Declaration[principals]) (F, bool),
	register func(*engine.SchemaBinding, rule.Registration[F]) (engine.RuleSlotCapability, bool),
	pair func(*engine.SchemaBinding, rule.Pairing[F], func(schema.Key) (engine.RuleSlotCapability, bool)) bool,
	bind func(*engine.SchemaBinding, rule.Binding[authorities, F]) (H, bool),
	finalize func(rule.Finalization[authorities, H]) bool,
	catalog func(H) (rule.LinkCatalog, bool),
) (*rule.Template, ruleContributor, bool) {
	if declare == nil || register == nil || bind == nil {
		return nil, ruleContributor{}, false
	}
	template, ok := rule.New(spec)
	if !ok {
		return nil, ruleContributor{}, false
	}
	roles := template.DeclaredRoles()
	contributor := ruleContributor{
		declare: func(builder *engine.SchemaBuilder, view vocabulary.Roles, owners principals) (rule.Cell, bool) {
			if builder == nil {
				return rule.Cell{}, false
			}
			restricted, rolesOK := view.Restrict(roles...)
			if !rolesOK {
				return rule.Cell{}, false
			}
			fragment, declared := declare(builder, rule.Declaration[principals]{Roles: restricted, Principals: owners})
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
		bind: func(binding *engine.SchemaBinding, set authorities, holder rule.Cell) (rule.Cell, bool) {
			fragment, fragmentOK := rule.Payload[F](holder)
			if !fragmentOK {
				return rule.Cell{}, false
			}
			hot, bound := bind(binding, rule.Binding[authorities, F]{Fragment: fragment, Authorities: set})
			if !bound {
				return rule.Cell{}, false
			}
			return rule.NewCell(hot), true
		},
	}
	if pair != nil {
		contributor.pair = func(binding *engine.SchemaBinding, holder rule.Cell, resolve func(schema.Key) (engine.RuleSlotCapability, bool)) bool {
			fragment, ok := rule.Payload[F](holder)
			return ok && pair(binding, rule.Pairing[F]{Fragment: fragment}, resolve)
		}
	}
	if finalize != nil {
		contributor.finalize = func(set authorities, holder rule.Cell) bool {
			hot, ok := rule.Payload[H](holder)
			return ok && finalize(rule.Finalization[authorities, H]{Rule: hot, Authorities: set})
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
	return template, contributor, contributor.complete(template.Lane())
}
