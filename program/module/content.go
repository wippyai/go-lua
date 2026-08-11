package module

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Bump only when the authored Module schema or canonical encoding changes.
// Derived Key resolution and Entry rows are intentionally outside this digest.
const contentVersion = 2

var (
	errInvalidArtifactComponent = errors.New("program/module: invalid artifact component")
	errInvalidArtifactSection   = errors.New("program/module: invalid artifact section")
)

func authoredContent(imports []Import) (id keyspace.ContentID) {
	hash := sha256.New()
	var writer canonical.Writer
	if writer.Reset(hash, "program/module", contentVersion) != nil ||
		writeAuthoredRows(&writer, imports) != nil {
		return keyspace.ContentID{}
	}
	if writer.Finish() != nil {
		return keyspace.ContentID{}
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}

// WriteArtifactSection writes only Module's authored payload. The caller owns
// the surrounding stream framing: this method deliberately emits no domain,
// version, or terminal marker. Derived Key resolution and Entry projections
// are not part of the Module artifact authority. The direct immutable View is
// the only accepted owner capability; unavailable views fail closed.
func WriteArtifactSection(writer *canonical.Writer, view View) error {
	if writer == nil {
		return canonical.ErrNilDestination
	}
	if view.component != nil {
		if !view.component.content.Available() {
			return errInvalidArtifactComponent
		}
		return writeAuthoredRows(writer, view.component.imports)
	}
	if view.state == nil {
		return errInvalidArtifactComponent
	}
	state := view.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.terminal || !state.claimed || state.authored == nil || !state.authored.content.Available() {
		return errInvalidArtifactComponent
	}
	return writeAuthoredRows(writer, state.authored.imports)
}

func validArtifactImportRow(row Import, index int) bool {
	return row.Term == keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1)) &&
		row.Call != 0 && keyspace.TermFamily(row.Call) == keyspace.FamilyCall &&
		keyspace.TermOrdinal(row.Call) != 0 &&
		(row.Alias == 0 || (keyspace.TermFamily(row.Alias) == keyspace.FamilyCell && keyspace.TermOrdinal(row.Alias) != 0)) &&
		row.Request != 0 && keyspace.TermFamily(row.Request) == keyspace.FamilyString && keyspace.TermOrdinal(row.Request) != 0
}

// ReadArtifactSection reads only Module's authored payload. Header and stream
// completion belong to the enclosing artifact codec. The returned Input keeps
// authored Request and leaves only derived Key at its zero value.
func ReadArtifactSection(reader *canonical.Reader) (Input, error) {
	if reader == nil {
		return Input{}, canonical.ErrMalformed
	}
	count, err := reader.Count()
	if err != nil {
		return Input{}, err
	}
	if count > uint64(keyspace.MaxTermOrdinal) || count > maxIntValue() {
		return Input{}, errInvalidArtifactSection
	}
	// A Uint event has a three-byte minimum (tag, one-byte length, one-byte
	// payload), and every authored row contains exactly four such events.
	// Check this before allocating the result slice so a hostile count cannot
	// turn a short payload into a large reservation.
	const rowWireMinimum = uint64(4 * 3)
	if count > uint64(reader.Remaining())/rowWireMinimum {
		return Input{}, errInvalidArtifactSection
	}

	// Probe the complete section on a value copy before reserving the result
	// slice. The reader is intentionally small and copyable; semantic failure
	// at either end of a hostile dense payload must not allocate count rows.
	probe := *reader
	for index := uint64(0); index < count; index++ {
		if _, err := readArtifactImport(&probe, index); err != nil {
			return Input{}, err
		}
	}

	imports := make([]Import, int(count))
	for index := uint64(0); index < count; index++ {
		row, err := readArtifactImport(reader, index)
		if err != nil {
			return Input{}, err
		}
		imports[index] = row
	}
	return Input{Imports: imports}, nil
}

// writeAuthoredRows is the one canonical encoder for the authored Import
// relation. Both ContentID and the payload-only artifact section call this
// function, so they cannot drift in row framing or field order.
func writeAuthoredRows(writer *canonical.Writer, imports []Import) error {
	if writer == nil {
		return canonical.ErrNilDestination
	}
	if uint64(len(imports)) > uint64(keyspace.MaxTermOrdinal) {
		return errInvalidArtifactComponent
	}
	if err := writer.Count(uint64(len(imports))); err != nil {
		return err
	}
	for index, row := range imports {
		if !validArtifactImportRow(row, index) {
			return errInvalidArtifactComponent
		}
		if err := writeAuthoredImport(writer, row); err != nil {
			return err
		}
	}
	return nil
}

func writeAuthoredImport(writer *canonical.Writer, row Import) error {
	if err := writer.Uint(uint64(row.Term)); err != nil {
		return err
	}
	if err := writer.Uint(uint64(row.Call)); err != nil {
		return err
	}
	if err := writer.Uint(uint64(row.Alias)); err != nil {
		return err
	}
	return writer.Uint(uint64(row.Request))
}

func readArtifactTerm(reader *canonical.Reader) (keyspace.Term, error) {
	value, err := reader.Uint()
	if err != nil {
		return 0, err
	}
	if value > uint64(^uint32(0)) {
		return 0, errInvalidArtifactSection
	}
	term := keyspace.Term(value)
	if value != 0 && (keyspace.TermFamily(term) == keyspace.FamilyInvalid || keyspace.TermOrdinal(term) == 0 || keyspace.TermOrdinal(term) > keyspace.MaxTermOrdinal) {
		return 0, errInvalidArtifactSection
	}
	return term, nil
}

func readArtifactImport(reader *canonical.Reader, index uint64) (Import, error) {
	term, err := readArtifactTerm(reader)
	if err != nil {
		return Import{}, err
	}
	expected := keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1))
	if term != expected {
		return Import{}, errInvalidArtifactSection
	}
	call, err := readArtifactTerm(reader)
	if err != nil {
		return Import{}, err
	}
	if keyspace.TermFamily(call) != keyspace.FamilyCall || keyspace.TermOrdinal(call) == 0 {
		return Import{}, errInvalidArtifactSection
	}
	alias, err := readArtifactTerm(reader)
	if err != nil {
		return Import{}, err
	}
	if alias != 0 && (keyspace.TermFamily(alias) != keyspace.FamilyCell || keyspace.TermOrdinal(alias) == 0) {
		return Import{}, errInvalidArtifactSection
	}
	request, err := readArtifactTerm(reader)
	if err != nil {
		return Import{}, err
	}
	if keyspace.TermFamily(request) != keyspace.FamilyString || keyspace.TermOrdinal(request) == 0 {
		return Import{}, errInvalidArtifactSection
	}
	return Import{Term: term, Call: call, Alias: alias, Request: request}, nil
}

func validArtifactImports(imports []Import) bool {
	if uint64(len(imports)) > uint64(keyspace.MaxTermOrdinal) {
		return false
	}
	for index, row := range imports {
		if row.Term != keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1)) ||
			row.Call == 0 || keyspace.TermFamily(row.Call) != keyspace.FamilyCall ||
			keyspace.TermOrdinal(row.Call) == 0 ||
			(row.Alias != 0 && (keyspace.TermFamily(row.Alias) != keyspace.FamilyCell || keyspace.TermOrdinal(row.Alias) == 0)) ||
			row.Request == 0 || keyspace.TermFamily(row.Request) != keyspace.FamilyString || keyspace.TermOrdinal(row.Request) == 0 {
			return false
		}
	}
	return true
}

func maxIntValue() uint64 { return uint64(^uint(0) >> 1) }
