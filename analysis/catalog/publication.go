// Package catalog owns neutral projections compiled from a sealed declaration
// catalog. It imports no analyzer domain and retains no process global: a
// publication plan belongs to the compilation that produced it.
package catalog

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Publication is the immutable dense publication plan derived from one sealed
// declaration catalog. The declaration schema is the sole source of output,
// query, writer, coverage, and slot identity; no caller supplies or rebuilds
// any of those coordinates.
type Publication struct {
	schemaID identity.ContentID
	slots    map[schema.Key]uint32
	outputs  []publishedColumn
	families map[schema.Key]identity.ContentID
	queries  []publishedQuery
	ok       bool
}

type publishedColumn struct {
	Output   schema.Key
	Writer   schema.Key
	Slot     uint32
	Coverage axis.Coverage
}

type publishedQuery struct {
	Family schema.Key
	ID     identity.ContentID
	Slot   uint32
}

// CompilePublication derives the publication plan from a sealed schema. It is
// deliberately explicit and allocation-local: separate checker and LSP
// environments can compile different catalogs without sharing a process
// singleton or a stale plan.
func CompilePublication(sealed *schema.Schema) (Publication, bool) {
	if !sealed.Available() {
		return Publication{}, false
	}
	schemaID := identity.ContentID(sealed.Digest())
	if !schemaID.Available() {
		return Publication{}, false
	}

	slots := make(map[schema.Key]uint32)
	outputs := make([]publishedColumn, 0)
	appendOutputs := func(enginePublished bool) bool {
		view, ok := sealed.Surface(schema.SurfaceKindAxis)
		if !ok {
			return false
		}
		for position := 0; position < view.Count(); position++ {
			entry, entryOK := view.At(position)
			axisEntry, axisOK := entry.(interface {
				schema.Entry
				Storage() axis.Storage
				OutputCount() int
				OutputAt(int) (axis.Output, bool)
				Coverage() axis.Coverage
			})
			if !entryOK || !axisOK || !axisEntry.EntryAvailable() {
				return false
			}
			if (axisEntry.Storage() == axis.StorageEngine) != enginePublished {
				continue
			}
			coverage := axisEntry.Coverage()
			if !coverage.Available() {
				return false
			}
			for index := 0; index < axisEntry.OutputCount(); index++ {
				output, outputOK := axisEntry.OutputAt(index)
				if !outputOK || !output.Available() {
					return false
				}
				if _, duplicate := slots[output.Key]; duplicate {
					return false
				}
				slot := uint32(len(outputs))
				slots[output.Key] = slot
				outputs = append(outputs, publishedColumn{
					Output:   output.Key,
					Writer:   output.Writer,
					Slot:     slot,
					Coverage: coverage,
				})
			}
		}
		return true
	}
	if !appendOutputs(true) || !appendOutputs(false) {
		return Publication{}, false
	}

	view, viewOK := sealed.Surface(schema.SurfaceKindQuery)
	if !viewOK {
		return Publication{}, false
	}
	families := make(map[schema.Key]identity.ContentID, view.Count())
	queries := make([]publishedQuery, 0, view.Count())
	for position := 0; position < view.Count(); position++ {
		entry, entryOK := view.At(position)
		registration, registrationOK := entry.(*query.Registration)
		if !entryOK || !registrationOK || !registration.EntryAvailable() {
			return Publication{}, false
		}
		if registration.PopulationKind() != query.PopulationKindSelectedPoint {
			continue
		}
		family := registration.Key()
		if _, duplicate := families[family]; duplicate {
			return Publication{}, false
		}
		id := identity.ContentID(registration.EntryID())
		if !id.Available() {
			return Publication{}, false
		}
		querySlot := uint32(len(outputs) + len(queries))
		families[family] = id
		queries = append(queries, publishedQuery{Family: family, ID: id, Slot: querySlot})
	}
	return Publication{
		schemaID: schemaID,
		slots:    slots,
		outputs:  outputs,
		families: families,
		queries:  queries,
		ok:       true,
	}, true
}

// Available reports whether this plan was compiled from a complete sealed
// schema.
func (publication Publication) Available() bool { return publication.ok }

// SchemaID is the declaration digest carried by every projected address.
func (publication Publication) SchemaID() (identity.ContentID, bool) {
	return publication.schemaID, publication.ok && publication.schemaID.Available()
}

// Columns is the total dense slot count, including axis outputs and query
// result columns.
func (publication Publication) Columns() int {
	if !publication.ok {
		return 0
	}
	return len(publication.outputs) + len(publication.queries)
}

// ProjectAxis returns the address of one declared output.
func ProjectAxis[K comparable, V any](publication Publication, output schema.Key) (snapshot.Axis[K, V], bool) {
	if !publication.ok {
		return snapshot.Axis[K, V]{}, false
	}
	slot, declared := publication.slots[output]
	if !declared {
		return snapshot.Axis[K, V]{}, false
	}
	return snapshot.Axis[K, V]{SchemaID: publication.schemaID, Slot: slot}, true
}

// Coverage returns the declared absence semantics of one output column.
func (publication Publication) Coverage(output schema.Key) (axis.Coverage, bool) {
	if !publication.ok {
		return axis.CoverageInvalid, false
	}
	slot, declared := publication.slots[output]
	if !declared || int(slot) >= len(publication.outputs) {
		return axis.CoverageInvalid, false
	}
	return publication.outputs[slot].Coverage, true
}

// WriteRequests returns all axis output admissions in dense slot order.
func (publication Publication) WriteRequests() ([]engine.ColumnAdmission, bool) {
	if !publication.ok {
		return nil, false
	}
	requests := make([]engine.ColumnAdmission, 0, len(publication.outputs))
	for _, column := range publication.outputs {
		requests = append(requests, engine.ColumnAdmission{
			Schema: publication.schemaID,
			Output: column.Output,
			Writer: column.Writer,
			Slot:   column.Slot,
		})
	}
	return requests, true
}

// QueryRequest is the materialization request for one sealed query family.
type QueryRequest struct {
	Schema identity.ContentID
	Family schema.Key
	ID     identity.ContentID
	Slot   uint32
}

// QueryRequests returns all query-family requests in dense slot order.
func (publication Publication) QueryRequests() ([]QueryRequest, bool) {
	if !publication.ok {
		return nil, false
	}
	requests := make([]QueryRequest, 0, len(publication.queries))
	for _, query := range publication.queries {
		requests = append(requests, QueryRequest{
			Schema: publication.schemaID,
			Family: query.Family,
			ID:     query.ID,
			Slot:   query.Slot,
		})
	}
	return requests, true
}

// ProjectQuery returns the identity under which one declared family is read.
func (publication Publication) ProjectQuery(family schema.Key) (identity.ContentID, bool) {
	if !publication.ok {
		return identity.ContentID{}, false
	}
	id, declared := publication.families[family]
	return id, declared
}

// Admissions returns every declared axis and query column admission in slot
// order. It is the complete open-side publication contract.
func (publication Publication) Admissions() ([]engine.ColumnAdmission, bool) {
	columns, columnsOK := publication.WriteRequests()
	queries, queriesOK := publication.QueryRequests()
	if !columnsOK || !queriesOK || len(columns)+len(queries) != publication.Columns() {
		return nil, false
	}
	admissions := make([]engine.ColumnAdmission, 0, len(columns)+len(queries))
	admissions = append(admissions, columns...)
	for _, request := range queries {
		admissions = append(admissions, engine.ColumnAdmission{
			Schema: request.Schema,
			Output: request.Family,
			Writer: request.Family,
			Slot:   request.Slot,
		})
	}
	return admissions, true
}

// AdmitColumns records the complete publication contract on an open binding.
func (publication Publication) AdmitColumns(binding *engine.SchemaBinding) bool {
	if !publication.ok {
		return false
	}
	admissions, ok := publication.Admissions()
	return ok && engine.AdmitColumns(binding, admissions)
}
