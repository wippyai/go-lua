package source

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/lua/semantics/exactkey"
)

// keyForm is private because Source preserves spelling metadata only: name
// and positional-list keys. Flow owns the evaluated field-operation forms.
type keyForm uint8

const (
	keyFormInvalid keyForm = iota
	keyFormName
	keyFormList
)

// KeyInput is one authored Source spelling. Construct it with NameKey or
// ListKey; it deliberately is not a second public FieldKind vocabulary.
type KeyInput struct {
	owner keyspace.Term
	exact keyspace.LiteralValue
	form  keyForm
}

func NameKey(owner keyspace.Term, text string) KeyInput {
	return KeyInput{owner: owner, exact: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}, form: keyFormName}
}

func ListKey(owner keyspace.Term, ordinal int64) KeyInput {
	return KeyInput{owner: owner, exact: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: ordinal}, form: keyFormList}
}

// ControlFaultKind is closed binder-rejected source evidence. It has no Flow
// transfer or outcome interpretation.
type ControlFaultKind uint8

const (
	ControlFaultDuplicateLabel ControlFaultKind = iota + 1
	ControlFaultUndefinedGoto
	ControlFaultGotoEntersLocal
	ControlFaultBreakOutsideLoop
)

func (kind ControlFaultKind) valid() bool {
	return kind >= ControlFaultDuplicateLabel && kind <= ControlFaultBreakOutsideLoop
}

type ControlFault struct {
	Owner   keyspace.Term
	Kind    ControlFaultKind
	Label   keyspace.Term
	Blocker keyspace.Term
}

type familyKey struct {
	owner keyspace.Term
	form  keyForm
	exact keyspace.Key
}

// exactStore is Source-owned immutable exact-key storage. Keyspace provides
// only the Key/LiteralValue identities; it owns no rows, builders, codecs, or
// semantic predicates.
type exactStore struct {
	atoms []keyspace.LiteralValue
}

type keyFaultStore struct {
	exact  exactStore
	keys   []familyKey
	faults []ControlFault
}

func buildKeyFault(a *authority, input Input) error {
	if a == nil || len(input.Keys) != a.count(keyspace.FamilyKey) ||
		len(input.Faults) != a.count(keyspace.FamilyControlFault) {
		return errors.New("program/source: key/fault family cardinality mismatch")
	}
	lookup, err := buildExactAtoms(&a.keys.exact, input.ExactAtoms)
	if err != nil {
		return err
	}
	a.keys.keys = make([]familyKey, len(input.Keys))
	for index, row := range input.Keys {
		if !a.validFamilyTerm(row.owner, keyspace.FamilyBody) {
			return errors.New("program/source: invalid source-key owner")
		}
		exact, ok := exactkey.Normalize(row.exact)
		if !ok || !validSourceKey(row.form, exact) {
			return errors.New("program/source: invalid source key")
		}
		key, ok := lookup[exact]
		if !ok {
			return errors.New("program/source: source key missing exact-atom denominator")
		}
		a.keys.keys[index] = familyKey{owner: row.owner, form: row.form, exact: key}
	}
	for _, fault := range input.Faults {
		if !validControlFault(a, fault) {
			return errors.New("program/source: invalid control fault")
		}
	}
	a.keys.faults = append([]ControlFault(nil), input.Faults...)
	return nil
}

// buildExactAtoms seals the complete input denominator before Source rows
// reference it. The lookup map is construction scratch only: published views
// expose dense handles and slices, never a mutable key map.
func buildExactAtoms(store *exactStore, rows []keyspace.LiteralValue) (map[keyspace.LiteralValue]keyspace.Key, error) {
	if store == nil || uint64(len(rows)) > uint64(^uint32(0)) {
		return nil, errors.New("program/source: invalid exact-atom denominator")
	}
	unique := make(map[keyspace.LiteralValue]struct{}, len(rows))
	store.atoms = make([]keyspace.LiteralValue, 0, len(rows))
	for _, raw := range rows {
		atom, ok := exactkey.Normalize(raw)
		if !ok {
			return nil, errors.New("program/source: invalid exact key atom")
		}
		if _, duplicate := unique[atom]; duplicate {
			continue
		}
		unique[atom] = struct{}{}
		store.atoms = append(store.atoms, atom)
	}
	sort.Slice(store.atoms, func(left, right int) bool {
		return exactkey.CompareCanonical(exactkey.FromLiteral(store.atoms[left]), exactkey.FromLiteral(store.atoms[right])) < 0
	})
	lookup := make(map[keyspace.LiteralValue]keyspace.Key, len(store.atoms))
	for index, atom := range store.atoms {
		lookup[atom] = keyspace.Key(index + 1)
	}
	return lookup, nil
}

func validSourceKey(form keyForm, exact keyspace.LiteralValue) bool {
	switch form {
	case keyFormName:
		return exact.Kind == keyspace.LiteralString
	case keyFormList:
		return exact.Kind == keyspace.LiteralInteger && exact.Integer > 0
	default:
		return false
	}
}

func validControlFault(a *authority, fault ControlFault) bool {
	if a == nil || !a.validFamilyTerm(fault.Owner, keyspace.FamilyBody) || !fault.Kind.valid() ||
		(fault.Label != 0 && !a.validFamilyTerm(fault.Label, keyspace.FamilyLabel)) ||
		(fault.Blocker != 0 && !a.validFamilyTerm(fault.Blocker, keyspace.FamilyCell)) {
		return false
	}
	switch fault.Kind {
	case ControlFaultDuplicateLabel:
		return fault.Label != 0 && fault.Blocker == 0
	case ControlFaultUndefinedGoto, ControlFaultBreakOutsideLoop:
		return fault.Label == 0 && fault.Blocker == 0
	case ControlFaultGotoEntersLocal:
		return fault.Label != 0 && fault.Blocker != 0
	default:
		return false
	}
}
