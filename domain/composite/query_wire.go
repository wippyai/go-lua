package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// queryProducer is the typed producer capability for one sealed family.
// Schema registrations are declarative; these hooks are selected by family
// key when the table is composed. A producer declares and binds the typed
// query state and recovers its sealed implementation. It does not imply that
// the implementation can be admitted as a selected-point Result row.
type queryProducer struct {
	declare func(*engine.SchemaBuilder, query.Subjects) (query.Cell, bool)
	bind    func(*engine.SchemaBinding, query.Cell, query.Subjects) bool
	recover func(*engine.SchemaBinding, query.Cell) (query.Cell, bool)
}

func (producer queryProducer) complete() bool {
	return producer.declare != nil && producer.bind != nil && producer.recover != nil
}

// queryResultPublication is the selected-point Result capability for one
// sealed family. The admission callback, encoder, and canonical contract are
// one unit: admitting a query without all three would create a mounted row
// that has no sound Result publication boundary.
type queryResultPublication struct {
	admit    func(*engine.SchemaBinding, query.Cell, identity.ContentID, identity.ContentID, identity.ContentID, executioncontext.Context) (engine.ProgramQueryAdmission, bool)
	encode   func(engine.Answer) (bool, uint64, []byte, bool)
	contract engine.CanonicalResultContract
}

func (publication queryResultPublication) complete() bool {
	return publication.admit != nil && publication.encode != nil && publication.contract.Available()
}

// queryContributor is the composition-owned wiring for one sealed family.
// The producer capability is sufficient for an observation population. A
// selected-point family additionally needs the complete Result publication
// capability above.
type queryContributor struct {
	queryProducer
	queryResultPublication
}

func (contributor queryContributor) producerComplete() bool {
	return contributor.queryProducer.complete()
}

func (contributor queryContributor) resultComplete() bool {
	return contributor.queryResultPublication.complete()
}

func (contributor queryContributor) complete() bool {
	return contributor.producerComplete() && contributor.resultComplete()
}

// admit is the binding-facing Result gate. It deliberately lives on the
// contributor rather than manufacturing a no-op callback for producer-only
// observations: a family with no complete Result capability simply refuses the
// selected-point admission request.
func (contributor queryContributor) admit(binding *engine.SchemaBinding, holder query.Cell, id, mount, point identity.ContentID, context executioncontext.Context) (engine.ProgramQueryAdmission, bool) {
	if !contributor.resultComplete() {
		return engine.ProgramQueryAdmission{}, false
	}
	return contributor.queryResultPublication.admit(binding, holder, id, mount, point, context)
}

// registrable states the population-sensitive admission law. Every family
// must carry a complete typed producer. Only the selected-point population is
// a Result publication lane and therefore requires the complete Result
// capability; observation producers may intentionally leave it absent.
func (contributor queryContributor) registrable(registration *query.Registration) bool {
	if registration == nil || !contributor.producerComplete() {
		return false
	}
	return registration.Population() != query.PopulationSelectedPoint || contributor.resultComplete()
}

func wireQuery[F, R any](
	spec query.Spec,
	roles vocabulary.Roles,
	declare func(*engine.SchemaBuilder, query.Declaration) (F, bool),
	bind func(*engine.SchemaBinding, query.Binding[F]) bool,
	recover func(*engine.SchemaBinding, query.Sealed[F]) (R, bool),
	admit func(R, identity.ContentID, identity.ContentID, identity.ContentID, executioncontext.Context) (engine.ProgramQueryAdmission, bool),
	encode func(engine.Answer) (bool, uint64, []byte, bool),
) (*query.Registration, queryContributor, bool) {
	if declare == nil || bind == nil || recover == nil {
		return nil, queryContributor{}, false
	}
	registration, ok := query.New(spec, roles)
	if !ok {
		return nil, queryContributor{}, false
	}
	subjects := registration.Subjects()
	semantic := registration.Semantic()
	freezer := registration.Freezer()
	contributor := queryContributor{
		queryProducer: queryProducer{
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
		},
	}
	// Result callbacks are optional for an observation producer, but they are
	// all-or-nothing whenever supplied. In particular, an observation family
	// never gets a fabricated encoder or admission just to satisfy the table.
	if admit != nil || encode != nil {
		if admit == nil || encode == nil {
			return nil, queryContributor{}, false
		}
		contract, contractOK := engine.NewCanonicalResultContract(identity.ContentID(registration.ID()), registration.Freezer())
		if !contractOK {
			return nil, queryContributor{}, false
		}
		contributor.queryResultPublication = queryResultPublication{
			admit: func(binding *engine.SchemaBinding, holder query.Cell, id, mount, point identity.ContentID, context executioncontext.Context) (engine.ProgramQueryAdmission, bool) {
				fragment, ok := query.Payload[F](holder)
				if !ok {
					return engine.ProgramQueryAdmission{}, false
				}
				implementation, ok := recover(binding, query.Sealed[F]{Fragment: fragment})
				if !ok {
					return engine.ProgramQueryAdmission{}, false
				}
				return admit(implementation, id, mount, point, context)
			},
			encode:   encode,
			contract: contract,
		}
	}
	if !contributor.registrable(registration) {
		return nil, queryContributor{}, false
	}
	return registration, contributor, true
}
