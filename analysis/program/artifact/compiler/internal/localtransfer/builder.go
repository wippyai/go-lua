// Package localtransfer owns the compiler's private local-transport assembly
// state. It derives transport identities from the canonical write set, closes
// the rows in (From, To, ID) order, and transfers the resulting dense schema
// planes exactly once. The parent compiler owns neither the draft vocabulary
// nor the conversion loop.
package localtransfer

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	artifactdigest "github.com/wippyai/go-lua/analysis/program/artifact/digest"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
)

type draft struct {
	id     identity.ContentID
	from   identity.ContentID
	to     identity.ContentID
	full   bool
	writes []schema.Key
}

func (row draft) available() bool {
	if !row.id.Available() || !row.from.Available() || !row.to.Available() || row.from == row.to || row.full == (len(row.writes) != 0) {
		return false
	}
	for index, write := range row.writes {
		if !write.Available() || index != 0 && row.writes[index-1] >= write {
			return false
		}
	}
	return true
}

// Builder is the one-shot mutable owner of local-transfer drafts.
type Builder struct {
	format      uint64
	rows        []draft
	sealed      bool
	faulted     bool
	transferred bool
}

// New creates an empty local-transfer owner for one artifact format.
func New(format uint64) *Builder { return &Builder{format: format} }

// Append admits one transport, canonicalizing its write set before deriving
// the content identity. Refusals are issued against the exact row that was
// being admitted; the parent carries the schema fault without translation.
func (builder *Builder) Append(domain string, from, to identity.ContentID, full bool, writes ...schema.Key) programconstruction.Fault {
	if builder == nil || builder.sealed || builder.transferred {
		return programconstruction.New(programcatalog.LocalTransfer(), programconstruction.IssueLocalTransferUnavailable, -1, -1)
	}
	rowIndex := len(builder.rows)
	ordered, orderedOK := orderedKeys(writes)
	if !orderedOK {
		return programconstruction.New(programcatalog.LocalTransfer(), programconstruction.IssueLocalTransferUnavailable, rowIndex, -1)
	}
	fields := []artifactdigest.Field{
		artifactdigest.ContentID(from), artifactdigest.ContentID(to),
		artifactdigest.Bool(full), artifactdigest.Uint(uint64(len(ordered))),
	}
	for _, write := range ordered {
		fields = append(fields, artifactdigest.Key(write))
	}
	row := draft{
		id:     artifactdigest.Digest(domain, builder.format, fields...),
		from:   from,
		to:     to,
		full:   full,
		writes: ordered,
	}
	if !row.available() {
		return programconstruction.New(programcatalog.LocalTransfer(), programconstruction.IssueLocalTransferUnavailable, rowIndex, -1)
	}
	builder.rows = append(builder.rows, row)
	return programconstruction.Fault{}
}

func orderedKeys(keys []schema.Key) ([]schema.Key, bool) {
	ordered := append([]schema.Key(nil), keys...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	for index, key := range ordered {
		if !key.Available() || index > 0 && ordered[index-1] == key {
			return nil, false
		}
	}
	return ordered, true
}

// Seal canonically orders the drafts and rejects duplicate transport IDs. The
// least-significant key is applied first so the final order is (From, To, ID),
// exactly matching the former compiler finalizer.
func (builder *Builder) Seal() programconstruction.Fault {
	if builder == nil || builder.sealed || builder.transferred {
		return programconstruction.New(programcatalog.LocalTransfer(), programconstruction.IssueLocalTransferUnavailable, -1, -1)
	}
	identity.SortByContentID(builder.rows, func(row draft) identity.ContentID { return row.id })
	identity.SortByContentID(builder.rows, func(row draft) identity.ContentID { return row.to })
	identity.SortByContentID(builder.rows, func(row draft) identity.ContentID { return row.from })
	for index := 1; index < len(builder.rows); index++ {
		if builder.rows[index-1].id == builder.rows[index].id {
			builder.sealed = true
			builder.faulted = true
			return programconstruction.New(programcatalog.LocalTransfer(), programconstruction.IssueLocalTransferDuplicate, index, -1)
		}
	}
	builder.sealed = true
	return programconstruction.Fault{}
}

// TakeCanonicalPlanes validates and transfers the one canonical transport and
// write plane. It succeeds only after Seal and only once.
func (builder *Builder) TakeCanonicalPlanes() ([]programschema.LocalTransfer, []programschema.LocalTransferWrite, programconstruction.Fault) {
	if builder == nil || !builder.sealed || builder.faulted || builder.transferred {
		return nil, nil, programconstruction.New(programcatalog.LocalTransfer(), programconstruction.IssueLocalTransferUnavailable, -1, -1)
	}
	transfers := make([]programschema.LocalTransfer, 0, len(builder.rows))
	writes := make([]programschema.LocalTransferWrite, 0)
	for rowIndex, row := range builder.rows {
		if !row.available() {
			return nil, nil, programconstruction.New(programcatalog.LocalTransfer(), programconstruction.IssueLocalTransferUnavailable, rowIndex, -1)
		}
		if !fitsUint32(len(writes)) || !fitsUint32(len(row.writes)) {
			return nil, nil, programconstruction.New(programcatalog.LocalTransferWrite(), programconstruction.IssueLocalTransferWriteUnavailable, len(writes), -1)
		}
		offset := uint32(len(writes))
		for _, key := range row.writes {
			write, ok := programschema.NewLocalTransferWrite(key)
			if !ok {
				return nil, nil, programconstruction.New(programcatalog.LocalTransferWrite(), programconstruction.IssueLocalTransferWriteUnavailable, len(writes), -1)
			}
			writes = append(writes, write)
		}
		converted, ok := programschema.NewLocalTransfer(row.id, row.from, row.to, row.full, offset, uint32(len(row.writes)))
		if !ok {
			return nil, nil, programconstruction.New(programcatalog.LocalTransfer(), programconstruction.IssueLocalTransferUnavailable, rowIndex, -1)
		}
		transfers = append(transfers, converted)
	}
	builder.rows = nil
	builder.transferred = true
	return transfers, writes, programconstruction.Fault{}
}

func fitsUint32(value int) bool { return value >= 0 && uint64(value) <= uint64(^uint32(0)) }
