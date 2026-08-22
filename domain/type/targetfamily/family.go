// Package targetfamily owns the sealed class vocabulary of a Target
// contract: every declared type the operation family reaches, in one
// canonical order, already decoded and canonically encoded.
//
// It is a column of the sealed contract rather than a memo. The declaration
// denominator is a property of the target's content, so it is derived once
// where that content is sealed. Every Link that mounts the target reads this
// column; none re-derives it, and no second derivation exists to fall back to.
//
// The package sits in the type domain because a declared type vocabulary is a
// type-domain value, and it takes the reading of a declaration as a Decoder
// value rather than an import, so it names no peer domain and stays below the
// domains that classify with it.
package targetfamily

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// Family is the sealed class vocabulary of one Target contract: every
// declared type reachable from the operation family, in one canonical order,
// already decoded and canonically encoded.
//
// It is a column of the sealed contract, not a memo. The whole declaration
// denominator is a property of the target's content, so it is derived once
// where that content is sealed. A Link that mounts the target reads this
// column; it never re-derives it, and there is no second path that could.
type Family struct {
	id       identity.ContentID
	rows     []row
	concrete *typeauthority.Family
}

// row is one declared Target type. A row is either a concrete
// member of the sealed Runtime vocabulary or an opaque residual: a scoped
// endpoint carrying operation formals, which is a finite opaque class rather
// than a decodable structural type.
type row struct {
	value  vocabulary.Type
	member int32 // index into concrete, or -1 for an opaque residual
}

// Count is the number of declared Target types this family classifies.
func (family *Family) Count() int {
	if family == nil {
		return 0
	}
	return len(family.rows)
}

// ContentID is the portable identity of the whole sealed family.
func (family *Family) ContentID() identity.ContentID {
	if family == nil {
		return identity.ContentID{}
	}
	return family.id
}

// Decoder is the type domain's reading of one neutral declaration. Target
// owns the declaration denominator and this package owns its order; what a
// declaration means stays with the domain that authored the envelope, which
// is why the meaning arrives as a value rather than as an import.
type Decoder func(declaration schematype.Type) (typ.Type, bool)

// SealColumn seals one Target's class vocabulary as its semantic column. The
// caller supplies the domain reading; the order and the identity are this
// package's.
func SealColumn(operations operationvalue.Core, decode Decoder) (contract.SealedColumn, error) {
	family, err := sealFamily(operations, decode)
	if err != nil {
		return contract.SealedColumn{}, err
	}
	return contract.SealedColumn{ID: family.id, Value: family}, nil
}

// Of reads the sealed class vocabulary of one Target contract.
// A contract that carries no such column cannot be mounted: there is no
// second derivation to fall back to.
func Of(target *contract.Contract) (*Family, bool) {
	if target == nil {
		return nil, false
	}
	column := target.Column()
	if !column.Available() {
		return nil, false
	}
	family, ok := column.Value.(*Family)
	return family, ok && family != nil && family.id.Available()
}

// sealFamily walks the complete operation family once, in Target handle
// order, and admits every declared type it reaches. Target owns the complete
// declaration denominator: every callable endpoint is classified directly,
// and Link application selection is a later Rule concern that is deliberately
// absent here.
func sealFamily(operations operationvalue.Core, decode Decoder) (*Family, error) {
	if decode == nil {
		return nil, errors.New("targetfamily: no declaration reading")
	}
	walk := walkState{
		operations: operations,
		decode:     decode,
		seen:       make(map[vocabulary.Type]struct{}, operations.OperationCount()),
	}
	for index := 0; index < operations.OperationCount(); index++ {
		operation, valid := operations.OperationAt(index)
		if !valid {
			return nil, errors.New("targetfamily: malformed operation family")
		}
		if err := walk.operation(operation); err != nil {
			return nil, err
		}
	}
	concrete, err := typeauthority.SealFamily("wippy.domain.static/target-class-family", walk.decoded)
	if err != nil {
		return nil, err
	}
	family := &Family{rows: walk.rows, concrete: concrete}
	family.id = familyID(family)
	if !family.id.Available() {
		return nil, errors.New("targetfamily: unavailable Target family identity")
	}
	return family, nil
}

func familyID(family *Family) (id identity.ContentID) {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.domain.static/target-class-family/v1"))
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(len(family.rows)))
	_, _ = h.Write(word[:])
	for _, row := range family.rows {
		binary.BigEndian.PutUint64(word[:], uint64(row.value))
		_, _ = h.Write(word[:])
		binary.BigEndian.PutUint64(word[:], uint64(uint32(row.member)))
		_, _ = h.Write(word[:])
	}
	concreteID := family.concrete.ContentID()
	_, _ = h.Write(concreteID[:])
	copy(id[:], h.Sum(nil))
	return id
}

// walkState is the one-shot traversal that turns the operation family
// into the ordered declared-type vocabulary.
type walkState struct {
	operations operationvalue.Core
	decode     Decoder
	seen       map[vocabulary.Type]struct{}
	rows       []row
	decoded    []typ.Type
}

func (walk *walkState) target(value vocabulary.Type) error {
	if _, exists := walk.seen[value]; exists {
		return nil
	}
	walk.seen[value] = struct{}{}
	declaration, ok := walk.operations.TypeDeclaration(value)
	if !ok {
		return errors.New("targetfamily: Target type declaration unavailable")
	}
	member := int32(-1)
	decoded, decodedOK := walk.decode(declaration)
	if decodedOK && decoded != nil {
		unwrapped := typ.UnwrapStructuralWrappers(decoded)
		if unwrapped == nil {
			return errors.New("targetfamily: nil concrete class")
		}
		if len(walk.decoded) > int(^uint32(0)>>1) {
			return errors.New("targetfamily: Target family overflow")
		}
		member = int32(len(walk.decoded))
		walk.decoded = append(walk.decoded, unwrapped)
	}
	walk.rows = append(walk.rows, row{value: value, member: member})
	return nil
}

func (walk *walkState) values(values vocabulary.Values) error {
	for index := 0; index < walk.operations.ValuesCount(values); index++ {
		value, ok := walk.operations.ValuesAt(values, index)
		if !ok {
			return errors.New("targetfamily: malformed Target Values")
		}
		if err := walk.target(value); err != nil {
			return err
		}
	}
	for index := 0; index < walk.operations.ValuesSuffixCount(values); index++ {
		value, ok := walk.operations.ValuesSuffixAt(values, index)
		if !ok {
			return errors.New("targetfamily: malformed Target Values suffix")
		}
		if err := walk.target(value); err != nil {
			return err
		}
	}
	if value, ok := walk.operations.ValuesTailType(values); ok {
		return walk.target(value)
	}
	return nil
}

func (walk *walkState) operation(operation vocabulary.Operation) error {
	core := walk.operations
	input, ok := core.Input(operation)
	if !ok {
		return errors.New("targetfamily: operation input unavailable")
	}
	if err := walk.values(input); err != nil {
		return err
	}
	for index := 0; index < core.TypeFormalCount(operation); index++ {
		if value, ok := core.TypeFormalConstraint(operation, vocabulary.TypeFormal(index)); ok {
			if err := walk.target(value); err != nil {
				return err
			}
		}
	}
	for index := 0; index < core.ValuesVarCount(operation); index++ {
		value, ok := core.ValuesVarType(operation, vocabulary.ValuesVar(index))
		if !ok {
			return errors.New("targetfamily: ValuesVar type unavailable")
		}
		if err := walk.target(value); err != nil {
			return err
		}
	}
	for index := 0; index < core.OutcomeCount(operation); index++ {
		_, values, ok := core.OutcomeAt(operation, index)
		if !ok {
			return errors.New("targetfamily: malformed outcome")
		}
		if err := walk.values(values); err != nil {
			return err
		}
	}
	kinds := [...]flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel}
	for index := 0; index < core.CallbackCount(operation); index++ {
		callback, ok := core.CallbackAt(operation, index)
		if !ok {
			return errors.New("targetfamily: malformed callback")
		}
		values, ok := core.CallbackArguments(callback)
		if !ok {
			return errors.New("targetfamily: callback arguments unavailable")
		}
		if err := walk.values(values); err != nil {
			return err
		}
		for _, kind := range kinds {
			if values, found := core.CallbackOutcome(callback, kind); found {
				if err := walk.values(values); err != nil {
					return err
				}
			}
		}
	}
	for index := 0; index < core.SubedgeCount(operation); index++ {
		edge, ok := core.SubedgeAt(operation, index)
		if !ok {
			return errors.New("targetfamily: malformed subedge")
		}
		values, ok := core.SubedgeArguments(edge)
		if !ok {
			return errors.New("targetfamily: subedge arguments unavailable")
		}
		if err := walk.values(values); err != nil {
			return err
		}
		for _, kind := range kinds {
			if values, found := core.SubedgeTerminal(edge, kind); found {
				if err := walk.values(values); err != nil {
					return err
				}
			}
		}
		if values, found := core.SubedgeAdmissionFailure(edge); found {
			if err := walk.values(values); err != nil {
				return err
			}
		}
	}
	for index := 0; index < core.ResumeCount(operation); index++ {
		resume, ok := core.ResumeIDAt(operation, index)
		if !ok {
			return errors.New("targetfamily: malformed resume")
		}
		_, _, _, values, ok := core.Resume(resume)
		if !ok {
			return errors.New("targetfamily: resume arguments unavailable")
		}
		if err := walk.values(values); err != nil {
			return err
		}
	}
	return nil
}

// At reports one declared Target type in canonical family order. member is
// the concrete vocabulary position, or -1 for an opaque residual: a scoped
// endpoint retaining operation formals, which is a finite opaque class rather
// than a decodable structural type.
func (family *Family) At(index int) (value vocabulary.Type, member int, available bool) {
	if family == nil || index < 0 || index >= len(family.rows) {
		return 0, 0, false
	}
	return family.rows[index].value, int(family.rows[index].member), true
}

// Member binds one already-encoded concrete row to the Authority that will
// seal it. It performs no decode, no clone, and no canonical encoding: the
// family's owner paid all three exactly once.
func (family *Family) Member(member int, types *typeauthority.Authority) (identity.ContentID, typeauthority.RuntimeInput, bool) {
	if family == nil {
		return identity.ContentID{}, typeauthority.RuntimeInput{}, false
	}
	canonicalID, identityOK := family.concrete.CanonicalIdentity(member)
	input, inputOK := family.concrete.Input(member, types)
	if !identityOK || !inputOK {
		return identity.ContentID{}, typeauthority.RuntimeInput{}, false
	}
	return canonicalID, input, true
}
