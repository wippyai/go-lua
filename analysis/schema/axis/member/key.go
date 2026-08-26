package member

import "github.com/wippyai/go-lua/analysis/schema"

// KeyVector names one ordered column vector the rows of a relation are published
// under.
//
// A publication addresses a row by a key, and the relation is the only
// authority on which of its columns form one and in what order. Naming it here
// makes the key ordinary declared data: a reader resolves it like any other
// member instead of reconstructing a vector from a role it had to know.
//
// The declaration owed this statement already. Every generated catalog reads
// key vocabulary back out - a relation's publication key is resolved by name
// wherever a rule publishes - so the vector was determined by the declaration
// and simply never published by it. This closes that debt; it opens no new
// surface, and it states nothing a publication does not already consume.
//
// Order is the whole content of the vector, so Columns is a sequence and not a
// set: two keys over the same columns in different orders address rows
// differently.
type KeyVector struct {
	// Name is the key's own identity within its axis. It shares the one
	// member-name namespace, so a key cannot take the name of a relation, a
	// projection, or another key.
	Name schema.Key
	// Columns are the relation's own columns, in the order the key holds them.
	Columns []schema.Key
}

// Declared reports whether this key states anything at all.
func (key KeyVector) Declared() bool { return key.Name.Available() || len(key.Columns) != 0 }

// Available reports whether the key names itself and one ordered vector of
// distinct columns. A key with no column addresses nothing, and a column
// repeated within one key would give a row two positions in its own address.
func (key KeyVector) Available() bool {
	if !key.Name.Available() || len(key.Columns) == 0 {
		return false
	}
	seen := make(map[schema.Key]struct{}, len(key.Columns))
	for _, column := range key.Columns {
		if !column.Available() {
			return false
		}
		if _, duplicate := seen[column]; duplicate {
			return false
		}
		seen[column] = struct{}{}
	}
	return true
}

// keysConsistent reports whether one relation's declared keys are each usable
// and no two of them take the same name.
func keyVectorsConsistent(keys []KeyVector) bool {
	names := make(map[schema.Key]struct{}, len(keys))
	for _, key := range keys {
		if !key.Available() {
			return false
		}
		if _, duplicate := names[key.Name]; duplicate {
			return false
		}
		names[key.Name] = struct{}{}
	}
	return true
}

func cloneKeyVectors(keys []KeyVector) []KeyVector {
	if keys == nil {
		return nil
	}
	clone := make([]KeyVector, len(keys))
	for index, key := range keys {
		clone[index] = KeyVector{Name: key.Name, Columns: append([]schema.Key(nil), key.Columns...)}
	}
	return clone
}
