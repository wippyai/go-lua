package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
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
// sealed family. The admission callback, detachment, and canonical contract
// are one unit: admitting a query without all three would create a mounted row
// that has no sound Result publication boundary.
//
// The layout a family's answers are detached under is not one of the family's
// own declarations. What the family states is the row state vocabulary and the
// columns it publishes; the family the payload is interpreted under and
// whether its rows are keyed come from the registration's shape, and the
// vocabularies come from the sealed structural table. sealPublicationLayout
// seals those together once the declaration table has sealed and closes the
// detachment over the result, so the encoder the Result boundary calls writes
// under the one layout the seal issued.
type queryResultPublication struct {
	admit    func(*engine.SchemaBinding, query.Cell, identity.ContentID, identity.ContentID, identity.ContentID, executioncontext.Context) (engine.ProgramQueryAdmission, bool)
	detach   func(*plane.Sealed, engine.Answer) (bool, uint64, []byte, bool)
	states   structure.Category
	columns  []plane.Column
	layout   *plane.Sealed
	encode   func(engine.Answer) (bool, uint64, []byte, bool)
	contract engine.CanonicalResultContract
}

func (publication queryResultPublication) complete() bool {
	return publication.admit != nil && publication.detach != nil && publication.contract.Available()
}

// planed reports whether this family publishes on the schema plane. A family
// that declares no columns detaches its answers with a codec of its own and is
// handed no sealed layout.
func (publication queryResultPublication) planed() bool {
	return publication.states.Available() && len(publication.columns) > 0
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
	switch registration.Population() {
	case query.PopulationSelectedPoint:
		return contributor.resultComplete()
	case query.PopulationObservation:
		return true
	default:
		return false
	}
}

func wireQuery[F, R any](
	spec query.Spec,
	roles vocabulary.Roles,
	declare func(*engine.SchemaBuilder, query.Declaration) (F, bool),
	bind func(*engine.SchemaBinding, query.Binding[F]) bool,
	recover func(*engine.SchemaBinding, query.Sealed[F]) (R, bool),
	admit func(R, identity.ContentID, identity.ContentID, identity.ContentID, executioncontext.Context) (engine.ProgramQueryAdmission, bool),
	detach func(*plane.Sealed, engine.Answer) (bool, uint64, []byte, bool),
	states structure.Category,
	columns []plane.Column,
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
				fragment, declared := declare(builder, query.Declaration{
					Semantic: semantic, Freezer: freezer,
					Population: registration.PopulationKind(), Subjects: narrowed,
				})
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
	if admit != nil || detach != nil {
		if admit == nil || detach == nil {
			return nil, queryContributor{}, false
		}
		contract, contractOK := engine.NewCanonicalResultContract(identity.ContentID(registration.EntryID()), registration.Freezer())
		if !contractOK {
			return nil, queryContributor{}, false
		}
		contributor.queryResultPublication = queryResultPublication{
			states:  states,
			columns: append([]plane.Column(nil), columns...),
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
			detach:   detach,
			contract: contract,
		}
	}
	if !contributor.registrable(registration) {
		return nil, queryContributor{}, false
	}
	return registration, contributor, true
}

// sealPublicationLayout seals every publishing family's answer layout against
// the vocabulary the declaration table sealed, and closes each family's
// detachment over the layout it published under.
//
// This is where the layout is held. A family authors the columns it publishes
// and names the row state vocabulary; the family key and the keying come from
// the registration's shape, which derives them from the fold the registration
// already declares. Nothing below this function re-derives any of it, and no
// domain seals a layout of its own.
func sealPublicationLayout(registrations []*query.Registration, contributors []queryContributor, vocabulary structure.Table) bool {
	if len(registrations) != len(contributors) {
		return false
	}
	for index := range contributors {
		publication := &contributors[index].queryResultPublication
		if publication.detach == nil {
			continue
		}
		detach := publication.detach
		if !publication.planed() {
			// A family still detaching its answers with a codec of its own
			// publishes no plane layout, so it is handed none rather than a
			// fabricated one.
			publication.encode = func(answer engine.Answer) (bool, uint64, []byte, bool) {
				return detach(nil, answer)
			}
			continue
		}
		registration := registrations[index]
		shape, shapeOK := registration.Shape()
		if !shapeOK {
			return false
		}
		layout, sealed := plane.Seal(shape, vocabulary, publication.states, publication.columns)
		if !sealed || !layout.Available() {
			return false
		}
		publication.layout = layout
		publication.encode = func(answer engine.Answer) (bool, uint64, []byte, bool) {
			return detach(layout, answer)
		}
	}
	return true
}
