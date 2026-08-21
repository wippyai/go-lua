package static

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/internal/framing"
)

// contentVersion changes only when the fixed, owned Static semantic codec
// changes. Query indexes, containment scratch, and cross-owner geometry are
// deliberately outside this authored identity.
const contentVersion = 4

const (
	staticContentRecordTypes        uint64 = 1
	staticContentRecordReferences   uint64 = 2
	staticContentRecordDeclarations uint64 = 3
	staticContentRecordSignatures   uint64 = 4
	staticContentRecordContracts    uint64 = 5
	staticContentRecordOperators    uint64 = 6
	staticContentRecordOperands     uint64 = 7
	staticContentRecordPublications uint64 = 8
)

// ContentID returns the sealed authored Static identity.
func (component *Component) ContentID() identity.ContentID {
	if component == nil {
		return identity.ContentID{}
	}
	return component.snapshot.ContentID()
}

// contentID coordinates exactly the eight typed authored verticals while the
// build-only assembly is still live. Each vertical owns its own scalar order
// and never hashes query derivatives.
func contentID(component *assembly) (id identity.ContentID) {
	if component == nil {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/static", contentVersion) != nil ||
		writeContent(&writer,
			component.types, component.references, component.declarations,
			component.signatures, component.contracts, component.operators,
			component.operands, component.publications) != nil ||
		writer.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

// writeContent encodes the eight authored Static relations in their canonical
// identity order. Derived indexes and cross-owner geometry are excluded.
func writeContent(
	writer *framing.Writer,
	types statictypes.Table,
	references staticrefs.Table,
	declarations staticdecl.Table,
	signatures staticsig.Table,
	contracts staticcontracts.Table,
	operators staticoperators.Table,
	operands staticoperands.Table,
	publications staticpubs.Table,
) error {
	if err := writer.Record(staticContentRecordTypes); err != nil {
		return err
	}
	if err := statictypes.WriteContent(writer, types); err != nil {
		return err
	}
	if err := writer.Record(staticContentRecordReferences); err != nil {
		return err
	}
	if err := staticrefs.WriteContent(writer, references); err != nil {
		return err
	}
	if err := writer.Record(staticContentRecordDeclarations); err != nil {
		return err
	}
	if err := staticdecl.WriteContent(writer, declarations); err != nil {
		return err
	}
	if err := writer.Record(staticContentRecordSignatures); err != nil {
		return err
	}
	if err := staticsig.WriteContent(writer, signatures); err != nil {
		return err
	}
	if err := writer.Record(staticContentRecordContracts); err != nil {
		return err
	}
	if err := staticcontracts.WriteContent(writer, contracts); err != nil {
		return err
	}
	if err := writer.Record(staticContentRecordOperators); err != nil {
		return err
	}
	if err := staticoperators.WriteContent(writer, operators); err != nil {
		return err
	}
	if err := writer.Record(staticContentRecordOperands); err != nil {
		return err
	}
	if err := staticoperands.WriteContent(writer, operands); err != nil {
		return err
	}
	if err := writer.Record(staticContentRecordPublications); err != nil {
		return err
	}
	return staticpubs.WriteContent(writer, publications)
}
