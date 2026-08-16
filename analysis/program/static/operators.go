package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// compactOperators owns the exact authored operator denominator.  It does
// not classify operands, evaluate an operator, or construct a generic node
// representation: those would either duplicate Flow or leak analysis into
// Program.
func compactOperators(component *Component, counts [keyspace.FamilyCount]uint32, input OperatorsInput) error {
	store := &component.operators
	for _, row := range input.TypeOf {
		// TypeOf.Operand is a cross-owner Flow value handle, not a Static
		// node; keep its ordinary counted-Term check distinct from Node.
		if !staticrole.ScopeHandle(counts, row.Scope) || !flowrole.ValueOccurrence(counts, row.Operand) {
			return errors.New("program/static: invalid typeof scope or operand")
		}
		store.typeOf = append(store.typeOf, row)
	}
	for _, row := range input.KeyOf {
		if !staticrole.Node(counts, row.Inner) {
			return errors.New("program/static: invalid keyof child")
		}
		store.keyOf = append(store.keyOf, row)
	}
	for _, row := range input.IndexAccess {
		if !staticrole.Node(counts, row.Object) || !staticrole.Node(counts, row.Index) {
			return errors.New("program/static: invalid indexed-access child")
		}
		store.indexAccess = append(store.indexAccess, row)
	}
	for _, row := range input.Conditional {
		if !staticrole.Node(counts, row.Check) || !staticrole.Node(counts, row.Extends) ||
			!staticrole.Node(counts, row.Then) || !staticrole.Node(counts, row.Else) {
			return errors.New("program/static: invalid conditional child")
		}
		store.conditional = append(store.conditional, row)
	}
	return nil
}

// emitOperatorsContainment owns only the concrete type children of the three
// purely static operators. TypeOf intentionally contributes none: both of its
// endpoints are cross-owner handles closed with Source/Flow.
func emitOperatorsContainment(component *Component, check *containment) bool {
	store := &component.operators
	for index, row := range store.keyOf {
		if !check.attach(keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, uint32(index+1)), row.Inner) {
			return false
		}
	}
	for index, row := range store.indexAccess {
		parent := keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, uint32(index+1))
		if !check.attach(parent, row.Object) || !check.attach(parent, row.Index) {
			return false
		}
	}
	for index, row := range store.conditional {
		parent := keyspace.MakeTerm(keyspace.FamilyTypeConditional, uint32(index+1))
		if !check.attach(parent, row.Check) || !check.attach(parent, row.Extends) ||
			!check.attach(parent, row.Then) || !check.attach(parent, row.Else) {
			return false
		}
	}
	return true
}

// writeOperatorsContent owns all four exact authored static operator rows.
func writeOperatorsContent(writer *framing.Writer, store operatorsStore) error {
	if err := writer.Count(uint64(len(store.typeOf))); err != nil {
		return err
	}
	for _, row := range store.typeOf {
		if err := writer.Uint(uint64(row.Scope)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Operand)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.keyOf))); err != nil {
		return err
	}
	for _, row := range store.keyOf {
		if err := writer.Uint(uint64(row.Inner)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.indexAccess))); err != nil {
		return err
	}
	for _, row := range store.indexAccess {
		if err := writer.Uint(uint64(row.Object)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Index)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.conditional))); err != nil {
		return err
	}
	for _, row := range store.conditional {
		if err := writer.Uint(uint64(row.Check)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Extends)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Then)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Else)); err != nil {
			return err
		}
	}
	return nil
}
