package composite

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// The published column plan is the projection of the sealed axis frames and the
// sealed query families onto the published value's addressing. A snapshot
// addresses a column by the identity of the schema that sealed it and a dense
// slot; the declaration table is that schema, and the slot is the column's
// position in the sealed catalog order, so an address is a function of the
// sealed table alone and no publisher assigns one of its own.
//
// A query family's answers are held in a result column, and a result column is
// a column: it is stored, addressed and read through the storage an axis column
// is, so the two share one dense slot range rather than two numbering schemes.
// The axis columns open the range in axis catalog order and the result columns
// continue it in query catalog order.
//
// The plan is derived once from the sealed table and is immutable afterwards,
// like the table itself.
var publication struct {
	once     sync.Once
	schemaID identity.ContentID
	slots    map[schema.Key]uint32
	outputs  []publishedColumn
	families map[schema.Key]identity.ContentID
	queries  []publishedQuery
	ok       bool
}

// publishedColumn is one output of the sealed plan: the column's name, the
// principal the table admitted as its writer, the dense slot it occupies, and
// what a reader concludes from a key it holds no row for.
type publishedColumn struct {
	Output   schema.Key
	Writer   schema.Key
	Slot     uint32
	Coverage axis.Coverage
}

// publishedQuery is one sealed query family of the plan: the family's authored
// key, the identity its answers are published and opened under, and the dense
// slot its result column occupies.
type publishedQuery struct {
	Family schema.Key
	ID     identity.ContentID
	Slot   uint32
}

func sealPublication() {
	publication.once.Do(func() {
		sealRegistry()
		if registry.sealed == nil {
			return
		}
		digest := registry.sealed.Digest()
		if !digest.Available() {
			return
		}
		slots := make(map[schema.Key]uint32)
		var columns []publishedColumn
		for _, entry := range registry.axes {
			coverage := entry.Coverage()
			for index := 0; index < entry.OutputCount(); index++ {
				output, ok := entry.OutputAt(index)
				if !ok || !coverage.Available() {
					return
				}
				slot := uint32(len(columns))
				slots[output.Key] = slot
				columns = append(columns, publishedColumn{
					Output:   output.Key,
					Writer:   output.Writer,
					Slot:     slot,
					Coverage: coverage,
				})
			}
		}
		// The query surface is read from the sealed table rather than from an
		// inventory kept beside it, so the families the projection answers are
		// exactly the families the table sealed and the two cannot drift.
		view, viewOK := registry.sealed.Surface(schema.SurfaceKindQuery)
		if !viewOK {
			return
		}
		families := make(map[schema.Key]identity.ContentID, view.Count())
		answered := make([]publishedQuery, 0, view.Count())
		for position := 0; position < view.Count(); position++ {
			entry, entryOK := view.At(position)
			registration, registrationOK := entry.(*query.Registration)
			if !entryOK || !registrationOK || !registration.EntryAvailable() {
				return
			}
			// A family is answered under the identity the table sealed it as an
			// entry with. It is derived from the surface and the authored key, so a
			// consumer opens the family the declaration names and never an identity
			// a publisher minted, and the codec identity stays what it is declared
			// as: the contract the answers are frozen under, not the family.
			id := identity.ContentID(registration.ID())
			if !id.Available() {
				return
			}
			answered = append(answered, publishedQuery{
				Family: registration.Key(),
				ID:     id,
				Slot:   uint32(len(columns) + len(answered)),
			})
			families[registration.Key()] = id
		}
		publication.schemaID, publication.slots, publication.outputs = digest, slots, columns
		publication.families, publication.queries, publication.ok = families, answered, true
	})
}

// PublicationSchema is the sealing schema identity every projected address
// carries. A snapshot published under another schema answers none of these
// addresses, which is what keeps a consumer from reading a column of a table it
// was not compiled against.
func PublicationSchema() (identity.ContentID, bool) {
	sealPublication()
	return publication.schemaID, publication.ok
}

// PublicationColumns is the number of dense column slots the sealed table
// publishes: one per declared axis output, then one result column per sealed
// query family. It is the bound every projected slot falls inside.
func PublicationColumns() int {
	sealPublication()
	return len(publication.outputs) + len(publication.queries)
}

// ProjectAxis mints the published address of one declared output at the key and
// value types the caller claims for it. The address is the sealing schema and
// the output's dense slot; the claim is carried to the snapshot, which either
// recovers a column built for exactly those types or fails closed, so a wrong
// claim reads as no read rather than as a wrong answer.
//
// It is the whole projection from a declaration to a published address: a
// consumer names the column the table declares and never a slot of its own.
func ProjectAxis[K comparable, V any](output schema.Key) (snapshot.Axis[K, V], bool) {
	sealPublication()
	if !publication.ok {
		return snapshot.Axis[K, V]{}, false
	}
	slot, declared := publication.slots[output]
	if !declared {
		return snapshot.Axis[K, V]{}, false
	}
	return snapshot.Axis[K, V]{SchemaID: publication.schemaID, Slot: slot}, true
}

// PublicationCoverage returns what a reader of one declared output concludes
// from a key its column holds no row for. A publisher reads it to decide
// whether the column is sealed with the key universe it is total over: the
// answer is derived from the axis's declared cardinality, so publication and
// declaration cannot disagree about what an absence means.
func PublicationCoverage(output schema.Key) (axis.Coverage, bool) {
	sealPublication()
	if !publication.ok {
		return axis.CoverageInvalid, false
	}
	slot, declared := publication.slots[output]
	if !declared {
		return axis.CoverageInvalid, false
	}
	return publication.outputs[slot].Coverage, true
}

// WriteRequest is the composition's issuance request for one published column:
// the column, the principal the table sealed as its writer, the schema that
// sealed the pair, and the slot the column occupies. It is a request and not a
// capability: the engine owns the write token, mints one only for a pair a
// sealed table admits, and mints at most one per column, so the one-writer law
// this table states at seal is the same law the engine holds a publisher to at
// runtime.
//
// The record is the engine's own admission shape rather than a copy of it. What
// this table requests and what the engine admits is one statement, so the two
// ends of the one-writer law cannot drift into two shapes that agree by
// convention.
type WriteRequest = engine.ColumnAdmission

// WriteRequests returns the issuance request for every column the sealed table
// publishes, in slot order. The set is total: every declared output is
// requested, so a column the engine never issues a token for is a column no
// publisher can fill rather than one an unadmitted writer could.
func WriteRequests() ([]WriteRequest, bool) {
	sealPublication()
	if !publication.ok {
		return nil, false
	}
	requests := make([]WriteRequest, 0, len(publication.outputs))
	for _, column := range publication.outputs {
		requests = append(requests, WriteRequest{
			Schema: publication.schemaID,
			Output: column.Output,
			Writer: column.Writer,
			Slot:   column.Slot,
		})
	}
	return requests, true
}

// ProjectQuery mints the published identity one sealed query family is answered
// under. It is the reading half of the query projection: a consumer holds the
// family the declaration names, opens it on a snapshot under this identity, and
// reads the result column that snapshot published for it, so a family a
// publication never answered opens nothing rather than reading some other
// column.
func ProjectQuery(family schema.Key) (identity.ContentID, bool) {
	sealPublication()
	if !publication.ok {
		return identity.ContentID{}, false
	}
	id, declared := publication.families[family]
	return id, declared
}

// QueryRequest is the composition's materialization request for one sealed
// query family: the family, the identity its answers are published and opened
// under, the schema that sealed it, and the slot its result column occupies. It
// is a request and not a capability, exactly as a column's issuance request is:
// the family is declared here and the answers are materialized by the pass that
// holds them.
type QueryRequest struct {
	Schema identity.ContentID
	Family schema.Key
	ID     identity.ContentID
	Slot   uint32
}

// QueryRequests returns the materialization request for every query family the
// sealed table answers, in slot order. The set is total: every sealed family is
// requested, so a family a publication never materializes is a family that
// opens on no snapshot rather than one a consumer could answer from an
// unpublished column.
func QueryRequests() ([]QueryRequest, bool) {
	sealPublication()
	if !publication.ok {
		return nil, false
	}
	requests := make([]QueryRequest, 0, len(publication.queries))
	for _, answered := range publication.queries {
		requests = append(requests, QueryRequest{
			Schema: publication.schemaID,
			Family: answered.Family,
			ID:     answered.ID,
			Slot:   answered.Slot,
		})
	}
	return requests, true
}
