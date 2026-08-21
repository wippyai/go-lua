package imports

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// Bump only when the authored Module schema or canonical encoding changes.
// Derived Key resolution and Entry rows are intentionally outside this digest.
const contentVersion = 2

var (
	errInvalidArtifactComponent = errors.New("program/imports: invalid artifact component")
)

func authoredContent(imports []Import) (id identity.ContentID) {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/imports", contentVersion) != nil ||
		writeAuthoredRows(&writer, imports) != nil {
		return identity.ContentID{}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func validArtifactImportRow(row Import, index int) bool {
	return row.Term == keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1)) &&
		row.Call != 0 && keyspace.TermFamily(row.Call) == keyspace.FamilyCall &&
		keyspace.TermOrdinal(row.Call) != 0 &&
		(row.Alias == 0 || (keyspace.TermFamily(row.Alias) == keyspace.FamilyCell && keyspace.TermOrdinal(row.Alias) != 0)) &&
		row.Request != 0 && keyspace.TermFamily(row.Request) == keyspace.FamilyString && keyspace.TermOrdinal(row.Request) != 0
}

// writeAuthoredRows is the canonical identity encoder for authored imports.
func writeAuthoredRows(writer *framing.Writer, imports []Import) error {
	if writer == nil {
		return framing.ErrNilDestination
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

func writeAuthoredImport(writer *framing.Writer, row Import) error {
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
