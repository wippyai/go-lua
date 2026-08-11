package authored

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Derived causal ports deliberately do not affect this authored identity.
const contentVersion = 7

var (
	errInvalidArtifactComponent = errors.New("program/flow: invalid artifact component")
	errInvalidArtifactSection   = errors.New("program/flow: invalid artifact section")
)

func contentID(component *component) (id keyspace.ContentID) {
	if component == nil {
		return keyspace.ContentID{}
	}
	hash := sha256.New()
	var writer canonical.Writer
	if writer.Reset(hash, "program/flow", contentVersion) != nil ||
		writeContent(&writer, component) != nil || writer.Finish() != nil {
		return keyspace.ContentID{}
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}

// WriteArtifactSection writes the authored Flow content without a domain,
// version, or terminal frame. The enclosing artifact owns those frames.
// Derived proofs, projections, and owner provenance are intentionally absent.
func WriteArtifactSection(writer *canonical.Writer, view View) error {
	if writer == nil {
		return canonical.ErrNilDestination
	}
	if !view.active() || view.component == nil || !view.component.contentID.Available() {
		return errInvalidArtifactComponent
	}
	return writeContent(writer, view.component)
}

// ReadArtifactSection reads exactly the nine authored Flow content records.
// Header and stream completion belong to the enclosing artifact codec. Counts
// are deliberately left zero: the root artifact injects the dense universes
// before calling Build.
func ReadArtifactSection(reader *canonical.Reader) (Input, error) {
	if reader == nil {
		return Input{}, canonical.ErrMalformed
	}
	decoder := artifactDecoder{reader: reader}
	if err := decoder.record(1); err != nil {
		return Input{}, err
	}
	values, err := decoder.values()
	if err != nil {
		return Input{}, err
	}
	if err := decoder.record(2); err != nil {
		return Input{}, err
	}
	access, err := decoder.access()
	if err != nil {
		return Input{}, err
	}
	if err := decoder.record(3); err != nil {
		return Input{}, err
	}
	storage, err := decoder.storage()
	if err != nil {
		return Input{}, err
	}
	if err := decoder.record(4); err != nil {
		return Input{}, err
	}
	tables, err := decoder.tables()
	if err != nil {
		return Input{}, err
	}
	if err := decoder.record(5); err != nil {
		return Input{}, err
	}
	functions, err := decoder.functions()
	if err != nil {
		return Input{}, err
	}
	if err := decoder.record(6); err != nil {
		return Input{}, err
	}
	operators, err := decoder.operators()
	if err != nil {
		return Input{}, err
	}
	if err := decoder.record(7); err != nil {
		return Input{}, err
	}
	calls, err := decoder.calls()
	if err != nil {
		return Input{}, err
	}
	if err := decoder.record(8); err != nil {
		return Input{}, err
	}
	control, err := decoder.control()
	if err != nil {
		return Input{}, err
	}
	if err := decoder.record(9); err != nil {
		return Input{}, err
	}
	claims, typeValues, err := decoder.claims()
	if err != nil {
		return Input{}, err
	}
	return Input{
		Values:     values,
		Access:     access,
		Storage:    storage,
		Tables:     tables,
		Functions:  functions,
		Operators:  operators,
		Calls:      calls,
		Control:    control,
		Claims:     claims,
		TypeValues: typeValues,
	}, nil
}

// writeContent is the one authored row writer shared by ContentID and the
// payload-only artifact section. Its record and pool ordering is identity:
// Table order precedes Table rows, function captures precede Function rows,
// Control cells precede loops, and Operators precede Calls.
func writeContent(writer *canonical.Writer, component *component) error {
	if writer == nil {
		return canonical.ErrNilDestination
	}
	if component == nil {
		return errInvalidArtifactComponent
	}
	if err := writer.Record(1); err != nil {
		return err
	}
	if err := writeValues(writer, component.values); err != nil {
		return err
	}
	if err := writer.Record(2); err != nil {
		return err
	}
	if err := writeAccess(writer, component.access); err != nil {
		return err
	}
	if err := writer.Record(3); err != nil {
		return err
	}
	if err := writeStorage(writer, component.storage); err != nil {
		return err
	}
	if err := writer.Record(4); err != nil {
		return err
	}
	if err := writeTables(writer, component.tables); err != nil {
		return err
	}
	if err := writer.Record(5); err != nil {
		return err
	}
	if err := writeFunctions(writer, component.functions); err != nil {
		return err
	}
	if err := writer.Record(6); err != nil {
		return err
	}
	if err := writeOperators(writer, component.operators); err != nil {
		return err
	}
	if err := writer.Record(7); err != nil {
		return err
	}
	if err := writeCalls(writer, component.calls); err != nil {
		return err
	}
	if err := writer.Record(8); err != nil {
		return err
	}
	if err := writeAuthoredControl(writer, component.authoredControl); err != nil {
		return err
	}
	if err := writer.Record(9); err != nil {
		return err
	}
	return writeClaims(writer, component.claims)
}

func writeClaims(writer *canonical.Writer, claims claimStore) error {
	if err := writer.Count(uint64(len(claims.claims))); err != nil {
		return err
	}
	for _, row := range claims.claims {
		for _, term := range [...]keyspace.Term{row.Owner, row.Operand} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
		if err := writer.Uint(uint64(row.Kind)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(claims.typeValues))); err != nil {
		return err
	}
	for _, row := range claims.typeValues {
		if err := writeTerm(writer, row.Owner); err != nil {
			return err
		}
	}
	return nil
}

func writeOperators(writer *canonical.Writer, operators operatorStore) error {
	if err := writer.Count(uint64(len(operators.unaries))); err != nil {
		return err
	}
	for _, row := range operators.unaries {
		if err := writeTerm(writer, row.Owner); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Op)); err != nil {
			return err
		}
		if err := writeTerm(writer, row.Operand); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(operators.binaries))); err != nil {
		return err
	}
	for _, row := range operators.binaries {
		if err := writeTerm(writer, row.Owner); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Op)); err != nil {
			return err
		}
		for _, term := range [...]keyspace.Term{row.Left, row.Right} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	if err := writer.Count(uint64(len(operators.selects))); err != nil {
		return err
	}
	for _, row := range operators.selects {
		if err := writeTerm(writer, row.Owner); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Op)); err != nil {
			return err
		}
		for _, term := range [...]keyspace.Term{row.Left, row.Right} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeAuthoredControl(writer *canonical.Writer, control authoredControlStore) error {
	if err := writer.Count(uint64(len(control.returns))); err != nil {
		return err
	}
	for _, row := range control.returns {
		for _, term := range [...]keyspace.Term{row.Owner, row.Values} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	if err := writer.Count(uint64(len(control.breaks))); err != nil {
		return err
	}
	for _, row := range control.breaks {
		if err := writeTerm(writer, row.Owner); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(control.labels))); err != nil {
		return err
	}
	for _, row := range control.labels {
		if err := writeTerm(writer, row.Owner); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(control.gotos))); err != nil {
		return err
	}
	for _, row := range control.gotos {
		for _, term := range [...]keyspace.Term{row.Owner, row.Target} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	if err := writer.Count(uint64(len(control.branches))); err != nil {
		return err
	}
	for _, row := range control.branches {
		for _, term := range [...]keyspace.Term{row.Owner, row.Condition, row.WhenTrue, row.WhenFalse} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	if err := writer.Count(uint64(len(control.cells))); err != nil {
		return err
	}
	for _, cell := range control.cells {
		if err := writeTerm(writer, cell); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(control.loops))); err != nil {
		return err
	}
	for _, row := range control.loops {
		for _, term := range [...]keyspace.Term{row.Owner, row.Body, row.Control} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
		if err := writer.Uint(uint64(row.Kind)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Cells.Start)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Cells.End)); err != nil {
			return err
		}
	}
	return nil
}

func writeFunctions(writer *canonical.Writer, functions functionStore) error {
	if err := writer.Count(uint64(len(functions.captures))); err != nil {
		return err
	}
	for _, capture := range functions.captures {
		for _, term := range [...]keyspace.Term{capture.Inner, capture.Outer} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	if err := writer.Count(uint64(len(functions.rows))); err != nil {
		return err
	}
	for _, row := range functions.rows {
		for _, term := range [...]keyspace.Term{row.Owner, row.Body, row.Vararg} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
		if err := writer.Uint(uint64(row.Captures.Start)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Captures.End)); err != nil {
			return err
		}
	}
	return nil
}

func writeCalls(writer *canonical.Writer, calls callStore) error {
	if err := writer.Count(uint64(len(calls.rows))); err != nil {
		return err
	}
	for _, row := range calls.rows {
		for _, term := range [...]keyspace.Term{row.Owner, row.Callee, row.Receiver, row.Actuals} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeAccess(writer *canonical.Writer, access accessStore) error {
	if err := writer.Count(uint64(len(access.exact))); err != nil {
		return err
	}
	for _, row := range access.exact {
		for _, term := range [...]keyspace.Term{row.Owner, row.Base, row.Source} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
		if err := writer.Uint(uint64(row.Kind)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(access.dynamic))); err != nil {
		return err
	}
	for _, row := range access.dynamic {
		for _, term := range [...]keyspace.Term{row.Owner, row.Base, row.Key} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeStorage(writer *canonical.Writer, storage storageStore) error {
	if err := writer.Count(uint64(len(storage.cells))); err != nil {
		return err
	}
	for _, row := range storage.cells {
		if err := writer.Uint(uint64(row.Kind)); err != nil {
			return err
		}
		if err := writeTerm(writer, row.Body); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Key)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(storage.reads))); err != nil {
		return err
	}
	for _, row := range storage.reads {
		for _, term := range [...]keyspace.Term{row.Owner, row.Source} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
		if err := writer.Bool(row.Implicit); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(storage.varargs))); err != nil {
		return err
	}
	for _, row := range storage.varargs {
		for _, term := range [...]keyspace.Term{row.Owner, row.Cell} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	if err := writer.Count(uint64(len(storage.binds))); err != nil {
		return err
	}
	for _, row := range storage.binds {
		for _, term := range [...]keyspace.Term{row.Owner, row.Values} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	if err := writer.Count(uint64(len(storage.assigns))); err != nil {
		return err
	}
	for _, row := range storage.assigns {
		for _, term := range [...]keyspace.Term{row.Owner, row.Values} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	if err := writer.Count(uint64(len(storage.writes))); err != nil {
		return err
	}
	for _, row := range storage.writes {
		for _, term := range [...]keyspace.Term{row.Assign, row.Target} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeValues(writer *canonical.Writer, values valueStore) error {
	if err := writer.Count(uint64(len(values.terms))); err != nil {
		return err
	}
	for _, term := range values.terms {
		if err := writeTerm(writer, term); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(values.rows))); err != nil {
		return err
	}
	for _, row := range values.rows {
		for _, term := range [...]keyspace.Term{row.Owner, row.Tail} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
		if err := writer.Uint(uint64(row.Fixed.Start)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Fixed.End)); err != nil {
			return err
		}
	}
	return nil
}

func writeTables(writer *canonical.Writer, tables tableStore) error {
	if err := writer.Count(uint64(len(tables.order))); err != nil {
		return err
	}
	for _, term := range tables.order {
		if err := writeTerm(writer, term); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(tables.rows))); err != nil {
		return err
	}
	for _, row := range tables.rows {
		if err := writeTerm(writer, row.Owner); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Fields.Start)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Fields.End)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(tables.fields))); err != nil {
		return err
	}
	for _, row := range tables.fields {
		for _, term := range [...]keyspace.Term{row.Table, row.Key, row.Values} {
			if err := writeTerm(writer, term); err != nil {
				return err
			}
		}
		if err := writer.Uint(uint64(row.Kind)); err != nil {
			return err
		}
	}
	return nil
}

func writeTerm(writer *canonical.Writer, term keyspace.Term) error {
	if term == 0 {
		if err := writer.Uint(0); err != nil {
			return err
		}
		return writer.Uint(0)
	}
	if err := writer.Uint(uint64(keyspace.TermFamily(term))); err != nil {
		return err
	}
	return writer.Uint(uint64(keyspace.TermOrdinal(term)))
}
