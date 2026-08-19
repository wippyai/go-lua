package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// axisContributor is the composition-owned wiring for one bound axis.
// Engine-published axes have none.
type axisContributor struct {
	declare func(*engine.SchemaBuilder, vocabulary.Roles) (axis.Cell, bool)
	bind    func(*engine.SchemaBinding, LinkInputs, axis.Cell) (axis.Cell, bool)
}

func (contributor axisContributor) complete() bool {
	return contributor.declare != nil && contributor.bind != nil
}

func wireAxis[F, H, V any](
	spec axis.Spec[LinkInputs],
	declare func(*engine.SchemaBuilder, axis.Declaration) (F, bool),
	bind func(*engine.SchemaBinding, axis.Binding[LinkInputs, F]) (H, bool),
	algebra func(H) (axis.Algebra[V], bool),
) (*axisTemplate, axisContributor, bool) {
	if declare == nil || bind == nil || algebra == nil {
		return nil, axisContributor{}, false
	}
	template, ok := axis.New(spec)
	if !ok || !template.Storage().Bound() {
		return nil, axisContributor{}, false
	}
	roles := template.DeclaredRoles()
	contributor := axisContributor{
		declare: func(builder *engine.SchemaBuilder, view vocabulary.Roles) (axis.Cell, bool) {
			if builder == nil {
				return axis.Cell{}, false
			}
			restricted, rolesOK := view.Restrict(roles...)
			if !rolesOK {
				return axis.Cell{}, false
			}
			fragment, declared := declare(builder, axis.Declaration{Roles: restricted})
			if !declared {
				return axis.Cell{}, false
			}
			return axis.NewCell(fragment), true
		},
		bind: func(binding *engine.SchemaBinding, inputs LinkInputs, holder axis.Cell) (axis.Cell, bool) {
			fragment, fragmentOK := axis.Payload[F](holder)
			if !fragmentOK {
				return axis.Cell{}, false
			}
			hot, bound := bind(binding, axis.Binding[LinkInputs, F]{Fragment: fragment, Inputs: inputs})
			if !bound {
				return axis.Cell{}, false
			}
			published, algebraOK := algebra(hot)
			if !algebraOK || !published.Available() {
				return axis.Cell{}, false
			}
			return axis.NewBoundCell(hot), true
		},
	}
	return template, contributor, contributor.complete()
}
