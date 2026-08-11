package role

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestRolePredicatesRejectForeignAndBoundaryOrdinals(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyNil] = 1
	counts[keyspace.FamilyCall] = 1
	counts[keyspace.FamilyCell] = 1
	counts[keyspace.FamilyKey] = 1
	counts[keyspace.FamilyValues] = 1
	for _, test := range []struct {
		name                                 string
		term                                 keyspace.Term
		wantValue, wantOpen, wantAddressable bool
	}{
		{"nil", keyspace.MakeTerm(keyspace.FamilyNil, 1), true, false, false},
		{"call", keyspace.MakeTerm(keyspace.FamilyCall, 1), true, true, false},
		{"cell", keyspace.MakeTerm(keyspace.FamilyCell, 1), false, false, true},
		{"foreign ordinal", keyspace.MakeTerm(keyspace.FamilyNil, 2), false, false, false},
		{"foreign family", keyspace.MakeTerm(keyspace.FamilyString, 1), false, false, false},
		{"zero", 0, false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ValueOccurrence(counts, test.term); got != test.wantValue {
				t.Fatalf("ValueOccurrence(%v) = %v, want %v", test.term, got, test.wantValue)
			}
			if got := OpenOccurrence(counts, test.term); got != test.wantOpen {
				t.Fatalf("OpenOccurrence(%v) = %v, want %v", test.term, got, test.wantOpen)
			}
			if got := Addressable(counts, test.term); got != test.wantAddressable {
				t.Fatalf("Addressable(%v) = %v, want %v", test.term, got, test.wantAddressable)
			}
		})
	}
}

func TestFieldSourceAndLoopControlFamilyMatrix(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyKey] = 1
	counts[keyspace.FamilyNil] = 1
	counts[keyspace.FamilyUnary] = 1
	counts[keyspace.FamilyValues] = 1
	for _, test := range []struct {
		name string
		kind kind.FieldKind
		term keyspace.Term
		want bool
	}{
		{"list key", kind.FieldList, keyspace.MakeTerm(keyspace.FamilyKey, 1), true},
		{"name key", kind.FieldName, keyspace.MakeTerm(keyspace.FamilyKey, 1), true},
		{"exact nil", kind.FieldExact, keyspace.MakeTerm(keyspace.FamilyNil, 1), true},
		{"exact unary candidate", kind.FieldExact, keyspace.MakeTerm(keyspace.FamilyUnary, 1), true},
		{"key value", kind.FieldKey, keyspace.MakeTerm(keyspace.FamilyNil, 1), true},
		{"name non-key", kind.FieldName, keyspace.MakeTerm(keyspace.FamilyNil, 1), false},
		{"invalid kind", kind.FieldKind(99), keyspace.MakeTerm(keyspace.FamilyKey, 1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := FieldSourceFamily(counts, test.term, test.kind); got != test.want {
				t.Fatalf("FieldSourceFamily = %v, want %v", got, test.want)
			}
		})
	}
	for _, test := range []struct {
		name string
		kind kind.LoopKind
		term keyspace.Term
		want bool
	}{
		{"while scalar", kind.LoopWhile, keyspace.MakeTerm(keyspace.FamilyNil, 1), true},
		{"repeat scalar", kind.LoopRepeat, keyspace.MakeTerm(keyspace.FamilyNil, 1), true},
		{"numeric values", kind.LoopNumericFor, keyspace.MakeTerm(keyspace.FamilyValues, 1), true},
		{"generic values", kind.LoopGenericFor, keyspace.MakeTerm(keyspace.FamilyValues, 1), true},
		{"numeric scalar", kind.LoopNumericFor, keyspace.MakeTerm(keyspace.FamilyNil, 1), false},
		{"invalid loop", kind.LoopKind(99), keyspace.MakeTerm(keyspace.FamilyValues, 1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := LoopControlFamily(counts, test.term, test.kind); got != test.want {
				t.Fatalf("LoopControlFamily = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRoleFamilyOnlyMatrix(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		counts[family] = 1
		term := keyspace.MakeTerm(family, 1)
		if got := ValueOccurrenceFamily(family); got != ValueOccurrence(counts, term) {
			t.Fatalf("ValueOccurrenceFamily(%d) = %v, counted term = %v", family, got, ValueOccurrence(counts, term))
		}
		if got := OpenOccurrenceFamily(family); got != OpenOccurrence(counts, term) {
			t.Fatalf("OpenOccurrenceFamily(%d) = %v, counted term = %v", family, got, OpenOccurrence(counts, term))
		}
		if got := AddressableFamily(family); got != Addressable(counts, term) {
			t.Fatalf("AddressableFamily(%d) = %v, counted term = %v", family, got, Addressable(counts, term))
		}
		counts[family] = 0
	}
}
