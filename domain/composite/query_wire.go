package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// queryContributor is the composition-owned wiring for one sealed family.
// Schema registrations are declarative; these hooks are selected by family
// key when the table is composed.
type queryContributor struct {
	declare func(*engine.SchemaBuilder, query.Subjects) (query.Cell, bool)
	bind    func(*engine.SchemaBinding, query.Cell, query.Subjects) bool
	recover func(*engine.SchemaBinding, query.Cell) (query.Cell, bool)
	admit   func(*engine.SchemaBinding, query.Cell, identity.ContentID, identity.ContentID, identity.ContentID) (engine.ProgramQueryAdmission, bool)
	encode  func(engine.Answer) (bool, uint64, []byte, bool)
	contract engine.CanonicalResultContract
}

func (contributor queryContributor) complete() bool {
	return contributor.declare != nil && contributor.bind != nil && contributor.recover != nil && contributor.admit != nil && contributor.encode != nil && contributor.contract.Available()
}

func wireQuery[F, R any](
	spec query.Spec,
	roles vocabulary.Roles,
	declare func(*engine.SchemaBuilder, query.Declaration) (F, bool),
	bind func(*engine.SchemaBinding, query.Binding[F]) bool,
	recover func(*engine.SchemaBinding, query.Sealed[F]) (R, bool),
	admit func(R, identity.ContentID, identity.ContentID, identity.ContentID) (engine.ProgramQueryAdmission, bool),
	encode func(engine.Answer) (bool, uint64, []byte, bool),
) (*query.Registration, queryContributor, bool) {
	if declare == nil || bind == nil || recover == nil || admit == nil || encode == nil {
		return nil, queryContributor{}, false
	}
	registration, ok := query.New(spec, roles)
	if !ok {
		return nil, queryContributor{}, false
	}
	contract, contractOK := engine.NewCanonicalResultContract(identity.ContentID(registration.ID()), registration.Freezer())
	if !contractOK {
		return nil, queryContributor{}, false
	}
	subjects := registration.Subjects()
	semantic := registration.Semantic()
	freezer := registration.Freezer()
	contributor := queryContributor{
		declare: func(builder *engine.SchemaBuilder, view query.Subjects) (query.Cell, bool) {
			narrowed, narrowedOK := query.RestrictSubjects(view, subjects)
			if !narrowedOK {
				return query.Cell{}, false
			}
			fragment, declared := declare(builder, query.Declaration{Semantic: semantic, Freezer: freezer, Subjects: narrowed})
			if !declared {
				return query.Cell{}, false
			}
			return query.NewCell(fragment), true
		},
		bind: func(binding *engine.SchemaBinding, holder query.Cell, view query.Subjects) bool {
			fragment, fragmentOK := query.Payload[F](holder)
			narrowed, narrowedOK := query.RestrictSubjects(view, subjects)
			return fragmentOK && narrowedOK && bind(binding, query.Binding[F]{Fragment: fragment, Subjects: narrowed})
		},
		recover: func(binding *engine.SchemaBinding, holder query.Cell) (query.Cell, bool) {
			fragment, fragmentOK := query.Payload[F](holder)
			if !fragmentOK {
				return query.Cell{}, false
			}
			implementation, recovered := recover(binding, query.Sealed[F]{Fragment: fragment})
			if !recovered {
				return query.Cell{}, false
			}
			return query.NewCell(implementation), true
		},
		admit: func(binding *engine.SchemaBinding, holder query.Cell, id, mount, point identity.ContentID) (engine.ProgramQueryAdmission, bool) {
			fragment, ok := query.Payload[F](holder)
			if !ok {
				return engine.ProgramQueryAdmission{}, false
			}
			implementation, ok := recover(binding, query.Sealed[F]{Fragment: fragment})
			if !ok {
				return engine.ProgramQueryAdmission{}, false
			}
			return admit(implementation, id, mount, point)
		},
		encode: encode,
		contract: contract,
	}
	return registration, contributor, contributor.complete()
}
