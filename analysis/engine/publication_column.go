// publication_column.go owns the write capability over one published column:
// the admitted (column, writer) pairs a binding seals, the unforgeable token
// the engine mints for each of them, and the builder wrapper that token
// unlocks.

package engine

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// ErrUnauthorizedColumnWrite reports a write offered without the capability
// the engine mints for that column. It is what every wrapper below answers a
// zero capability with, so a caller that never obtained one edits nothing
// rather than editing a column it was not admitted to.
var ErrUnauthorizedColumnWrite = errors.New("engine: column write without a minted capability")

// NewColumnBinding opens a publication binding that admits columns and
// nothing else. Seal succeeds only after AdmitColumns states a nonempty set.
func NewColumnBinding() *SchemaBinding {
	return &SchemaBinding{state: &schemaBindingState{
		phase: schemaBindingOpen, authority: &schemaBindingAuthority{},
	}}
}

func sealColumnBindingLocked(state *schemaBindingState) bool {
	if state == nil || len(state.factors) != 0 || len(state.rules) != 0 || len(state.queries) != 0 || len(state.activation) != 0 || len(state.columns) == 0 || !completeAdmittedColumnsLocked(state) {
		if state != nil {
			state.poisonLocked()
		}
		return false
	}
	state.phase = schemaBindingSealed
	return true
}

// ColumnAdmission is one published column the sealed declaration table admits
// a writer for: the schema that sealed the pair, the column, the principal the
// table named as its writer, and the dense slot the column occupies.
//
// It is the composition's request and not a capability. The table states which
// pairs exist; the engine holds a publisher to that statement by admitting the
// set once, before the binding seals, and minting at most one capability per
// column afterwards. The seal's one-writer law and the runtime token are the
// same law at two ends, and this record is what carries it between them.
type ColumnAdmission struct {
	Schema identity.ContentID
	Output schema.Key
	Writer schema.Key
	Slot   uint32
}

// Available reports whether this admission names a column and a writer of one
// sealed table. An admission missing either half admits nothing: a column with
// no writer is one nothing may fill, and a writer with no column is a
// capability over nothing.
func (admission ColumnAdmission) Available() bool {
	return admission.Schema.Available() && admission.Output.Available() && admission.Writer.Available()
}

// admittedColumn is one admitted pair and whether its capability was minted.
// The mint flag is what makes a second mint of one column a refusal rather
// than a second writer.
type admittedColumn struct {
	admission ColumnAdmission
	minted    bool
}

// AdmitColumns records the published columns this binding's publication may
// write, and the principal admitted to write each of them. It runs while the
// binding is open and runs once: the admitted set is the table's statement, so
// a binding that restated it would hold two answers to the same question, and
// the restatement poisons it exactly as any other incomplete declaration does.
//
// The set is stated whole. Every admission names one table, every column
// appears once, and every slot is claimed once, so a capability minted from it
// can never be the second writer of a column.
func AdmitColumns(binding *SchemaBinding, admissions []ColumnAdmission) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || len(admissions) == 0 || state.columns != nil {
		state.poisonLocked()
		return false
	}
	columns := make(map[schema.Key]*admittedColumn, len(admissions))
	slots := make(map[uint32]schema.Key, len(admissions))
	table := admissions[0].Schema
	for _, admission := range admissions {
		if !admission.Available() || admission.Schema != table {
			state.poisonLocked()
			return false
		}
		if _, duplicate := columns[admission.Output]; duplicate {
			state.poisonLocked()
			return false
		}
		if _, claimed := slots[admission.Slot]; claimed {
			state.poisonLocked()
			return false
		}
		columns[admission.Output] = &admittedColumn{admission: admission}
		slots[admission.Slot] = admission.Output
	}
	state.columns, state.columnSlots = columns, slots
	return true
}

// completeAdmittedColumnsLocked restates the admitted set's own law at the
// seal. A binding that admits no column publishes none and is complete; one
// that admits columns publishes them under one table, one writer per column
// and one column per slot.
func completeAdmittedColumnsLocked(state *schemaBindingState) bool {
	if state == nil {
		return false
	}
	if len(state.columns) == 0 {
		return state.columns == nil && state.columnSlots == nil
	}
	if len(state.columns) != len(state.columnSlots) {
		return false
	}
	var table identity.ContentID
	for output, admitted := range state.columns {
		if admitted == nil || admitted.minted || !admitted.admission.Available() || admitted.admission.Output != output {
			return false
		}
		if !table.Available() {
			table = admitted.admission.Schema
		}
		if admitted.admission.Schema != table {
			return false
		}
		if claimed, ok := state.columnSlots[admitted.admission.Slot]; !ok || claimed != output {
			return false
		}
	}
	return true
}

// columnGrant is the engine's proof that one publisher holds the write
// capability over one published column. It is unexported and has no exported
// constructor, so no value of it exists outside a mint, and it retains the
// exact binding state and authority the mint was issued against, so a
// capability whose binding never sealed, was poisoned, or belongs to another
// publication unlocks nothing.
type columnGrant struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	admission ColumnAdmission
}

// valid re-asks the binding whether this grant is still the one capability its
// column was minted for. It is asked on every write rather than once at the
// mint, because the authority a grant names is the live publication's and not
// a copy of it.
func (grant *columnGrant) valid() bool {
	if grant == nil || grant.state == nil || grant.authority == nil || !grant.admission.Available() {
		return false
	}
	grant.state.mu.Lock()
	defer grant.state.mu.Unlock()
	if grant.state.phase != schemaBindingSealed || grant.state.authority != grant.authority {
		return false
	}
	admitted, held := grant.state.columns[grant.admission.Output]
	return held && admitted != nil && admitted.minted && admitted.admission == grant.admission
}

// ColumnWrite is the minted capability to write one published column at the
// key and value types it was minted for. It holds one unexported grant and
// nothing else: a package outside the engine can name the type and copy a
// value of it, and can construct only the zero one, which unlocks no column.
//
// A copy of a capability is the same capability. There is at most one grant per
// column, because a column mints once.
type ColumnWrite[K comparable, V any] struct{ grant *columnGrant }

// Available reports whether this capability was minted and its publication is
// still the sealed one it was minted against.
func (write ColumnWrite[K, V]) Available() bool { return write.grant.valid() }

// Output names the column this capability writes. The zero capability names
// none.
func (write ColumnWrite[K, V]) Output() schema.Key {
	if write.grant == nil {
		return ""
	}
	return write.grant.admission.Output
}

// column recovers the address this capability unlocks. It stays inside the
// engine: handing the address out would make the capability a formality, since
// the storage layer knows nothing of principals and edits any address it is
// given.
func (write ColumnWrite[K, V]) column() (snapshot.Axis[K, V], bool) {
	if !write.grant.valid() {
		return snapshot.Axis[K, V]{}, false
	}
	return snapshot.Axis[K, V]{SchemaID: write.grant.admission.Schema, Slot: write.grant.admission.Slot}, true
}

// MintColumnWrite issues the write capability for one admitted column at the
// key and value types the publisher claims for it. It runs only after the
// binding seals, so a capability exists only once the publication it belongs to
// is complete, and it issues once per column: a second mint is refused whatever
// types it claims, which is the runtime end of the table's one-writer law.
//
// A mint naming a writer the table did not admit for the column, or a column
// the table never declared, is refused. A refused mint leaves the sealed
// binding as it was: it withholds a capability rather than breaking a
// publication.
func MintColumnWrite[K comparable, V any](binding *SchemaBinding, output, writer schema.Key) (ColumnWrite[K, V], bool) {
	state := bindingState(binding)
	if state == nil {
		return ColumnWrite[K, V]{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || state.authority == nil || !output.Available() || !writer.Available() {
		return ColumnWrite[K, V]{}, false
	}
	admitted, held := state.columns[output]
	if !held || admitted == nil || admitted.minted || admitted.admission.Writer != writer {
		return ColumnWrite[K, V]{}, false
	}
	admitted.minted = true
	return ColumnWrite[K, V]{grant: &columnGrant{
		state: state, authority: state.authority, admission: admitted.admission,
	}}, true
}

// PublishColumn seals content into the column this capability was minted for.
// It is the wholesale write: it states the column's rows and the key universe
// it is total over in one act.
func PublishColumn[K comparable, V any](write ColumnWrite[K, V], builder *snapshot.Builder, content snapshot.Content[K, V]) error {
	column, unlocked := write.column()
	if !unlocked {
		return ErrUnauthorizedColumnWrite
	}
	return snapshot.PutColumn(builder, column, content)
}

// PublishQueryColumn seals content as the result column of one query family
// and registers the family answerable, at the slot this capability was minted
// for. A result column is a column: it is written through the same capability
// an axis column is, and the family identity is what a consumer opens it by.
func PublishQueryColumn[K comparable, O any](write ColumnWrite[K, O], builder *snapshot.Builder, family identity.ContentID, content snapshot.Content[K, O]) (snapshot.QueryPlan[K, O], error) {
	column, unlocked := write.column()
	if !unlocked {
		return snapshot.QueryPlan[K, O]{}, ErrUnauthorizedColumnWrite
	}
	return snapshot.DeclareQuery(builder, family, column.Slot, content)
}

// PublishRow writes one row into the column this capability was minted for.
func PublishRow[K comparable, V any](write ColumnWrite[K, V], builder *snapshot.Builder, key K, value V) error {
	column, unlocked := write.column()
	if !unlocked {
		return ErrUnauthorizedColumnWrite
	}
	return snapshot.SetRow(builder, column, key, value)
}

// WithdrawRow removes one row from the column this capability was minted for.
// A key the column's denominator covers reads as proven absent afterwards.
func WithdrawRow[K comparable, V any](write ColumnWrite[K, V], builder *snapshot.Builder, key K) error {
	column, unlocked := write.column()
	if !unlocked {
		return ErrUnauthorizedColumnWrite
	}
	return snapshot.RemoveRow(builder, column, key)
}
