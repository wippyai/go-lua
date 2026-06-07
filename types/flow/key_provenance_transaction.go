package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

type KeyProvenanceKind uint8

const (
	KeyProvenanceKeyedIteration KeyProvenanceKind = iota + 1
	KeyProvenanceKeyArrayAssignment
	KeyProvenanceIndexedKeyArrayIteration
	KeyProvenanceGuardedIndex
	KeyProvenanceDynamicIndexWrite
)

// KeyProvenancePathTransaction is the source-facing publication form for facts
// of the form "this value is a key of that table". Flow owns the conversion to
// stable addresses and the cross-reductions among key-presence, key-array, and
// delayed index-write facts.
type KeyProvenancePathTransaction struct {
	Kind      KeyProvenanceKind
	TablePath constraint.Path
	ArrayPath constraint.Path
	KeyPath   constraint.Path
	ValuePath constraint.Path
}

// KeyProvenanceTransaction is the normalized address-space form of
// KeyProvenancePathTransaction. Path validation and lowering happen once at the
// source boundary; reduced-product application consumes only stable identities.
type KeyProvenanceTransaction struct {
	Kind         KeyProvenanceKind
	Table        StableAddress
	Array        StableAddress
	Key          StableAddress
	ValuePath    StableAddress
	HasValuePath bool
}

// KeyProvenanceResult reports non-flow refinements derived while applying a
// key-provenance transaction. Producers may consume these through their own policy
// layer; flow only computes the evidence.
type KeyProvenanceResult struct {
	KeyRefinementPath  constraint.Path
	KeyRefinementValue product.AbstractValue
}

// KeyProvenanceTransactionResult is the address-space result of applying a
// normalized key-provenance transaction.
type KeyProvenanceTransactionResult struct {
	KeyRefinementAddress StableAddress
	KeyRefinementValue   product.AbstractValue
}

// ApplyKeyProvenancePathTransaction lowers and applies a source-level
// key-provenance transaction.
func ApplyKeyProvenancePathTransaction(out *PointState, txPath KeyProvenancePathTransaction) (KeyProvenanceResult, bool) {
	if out == nil {
		return KeyProvenanceResult{}, false
	}
	tx, ok := KeyProvenanceTransactionOfPath(txPath)
	if !ok {
		return KeyProvenanceResult{}, false
	}
	result, changed := ApplyKeyProvenanceTransaction(out, tx)
	return KeyProvenanceResult{
		KeyRefinementPath:  txPath.KeyPath,
		KeyRefinementValue: result.KeyRefinementValue,
	}, changed
}

// KeyProvenanceTransactionOfPath lowers a source-level path transaction into the
// canonical address transaction consumed by flow.
func KeyProvenanceTransactionOfPath(txPath KeyProvenancePathTransaction) (KeyProvenanceTransaction, bool) {
	tx := KeyProvenanceTransaction{Kind: txPath.Kind}
	switch txPath.Kind {
	case KeyProvenanceKeyedIteration, KeyProvenanceGuardedIndex, KeyProvenanceDynamicIndexWrite:
		if txPath.TablePath.IsEmpty() || txPath.KeyPath.IsEmpty() {
			return KeyProvenanceTransaction{}, false
		}
		table, tableOK := StableAddressOfPath(txPath.TablePath)
		key, keyOK := StableAddressOfPath(txPath.KeyPath)
		if !tableOK || !keyOK {
			return KeyProvenanceTransaction{}, false
		}
		tx.Table = table
		tx.Key = key
		if !txPath.ValuePath.IsEmpty() {
			if value, ok := StableAddressOfPath(txPath.ValuePath); ok {
				tx.ValuePath = value
				tx.HasValuePath = true
			}
		}
		return tx, true
	case KeyProvenanceKeyArrayAssignment:
		if txPath.ArrayPath.IsEmpty() || txPath.TablePath.IsEmpty() {
			return KeyProvenanceTransaction{}, false
		}
		array, arrayOK := StableAddressOfPath(txPath.ArrayPath)
		table, tableOK := StableAddressOfPath(txPath.TablePath)
		if !arrayOK || !tableOK {
			return KeyProvenanceTransaction{}, false
		}
		tx.Array = array
		tx.Table = table
		return tx, true
	case KeyProvenanceIndexedKeyArrayIteration:
		if txPath.ArrayPath.IsEmpty() || txPath.KeyPath.IsEmpty() {
			return KeyProvenanceTransaction{}, false
		}
		array, arrayOK := StableAddressOfPath(txPath.ArrayPath)
		key, keyOK := StableAddressOfPath(txPath.KeyPath)
		if !arrayOK || !keyOK {
			return KeyProvenanceTransaction{}, false
		}
		tx.Array = array
		tx.Key = key
		return tx, true
	default:
		return KeyProvenanceTransaction{}, false
	}
}

// ApplyKeyProvenanceTransaction applies a normalized key-provenance transaction
// to the point-state reduced products.
func ApplyKeyProvenanceTransaction(out *PointState, tx KeyProvenanceTransaction) (KeyProvenanceTransactionResult, bool) {
	if out == nil {
		return KeyProvenanceTransactionResult{}, false
	}
	switch tx.Kind {
	case KeyProvenanceKeyedIteration, KeyProvenanceGuardedIndex, KeyProvenanceDynamicIndexWrite:
		if tx.Table.Key() == "" || tx.Key.Key() == "" {
			return KeyProvenanceTransactionResult{}, false
		}
		return KeyProvenanceTransactionResult{}, ApplyKeyPresenceProof(out, KeyPresenceProof{
			Table:        tx.Table,
			Key:          tx.Key,
			ValuePath:    tx.ValuePath,
			HasValuePath: tx.HasValuePath,
		})
	case KeyProvenanceKeyArrayAssignment:
		if tx.Array.Key() == "" || tx.Table.Key() == "" {
			return KeyProvenanceTransactionResult{}, false
		}
		changed := ApplyKeyArrayProof(out, KeyArrayProof{
			Array: tx.Array,
			Table: tx.Table,
		})
		return KeyProvenanceTransactionResult{}, changed
	case KeyProvenanceIndexedKeyArrayIteration:
		if tx.Array.Key() == "" || tx.Key.Key() == "" {
			return KeyProvenanceTransactionResult{}, false
		}
		keyValue, _ := PointFactsOf(*out).AddressValue(tx.Key)
		iteration, changed := ApplyIndexedKeyArrayIterationProof(out, IndexedKeyArrayIterationProof{
			Array:    tx.Array,
			Key:      tx.Key,
			KeyValue: keyValue,
		})
		return KeyProvenanceTransactionResult{
			KeyRefinementAddress: tx.Key,
			KeyRefinementValue:   keyDomainFromIterationTables(out, iteration.Tables),
		}, changed
	default:
		return KeyProvenanceTransactionResult{}, false
	}
}

func keyDomainFromIterationTables(out *PointState, tables []StableAddress) product.AbstractValue {
	if out == nil {
		return product.AbstractValue{}
	}
	var keyDomain product.AbstractValue
	for _, tableAddr := range tables {
		if keyType, ok := PointFactsOf(*out).KeyDomainAtAddress(tableAddr); ok && !typ.IsAbsentOrUnknown(keyType) {
			av := product.FromType(keyType)
			if keyDomain.IsZero() {
				keyDomain = av
			} else {
				keyDomain = product.Join(keyDomain, av)
			}
		}
	}
	return keyDomain
}
