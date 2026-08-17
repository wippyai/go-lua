package target

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

// sourceRowWriter encodes the typed row returned by a public owner query.
// It is not a row model: the digest is the detached rows of that query's
// exact scalar/identity projection.
type sourceRowWriter struct {
	h     hash.Hash
	valid bool
	owner identity.ContentID
	token semanticsource.Token
}

func newSourceRowWriter(owner identity.ContentID, token semanticsource.Token) *sourceRowWriter {
	w := &sourceRowWriter{h: sha256.New(), valid: owner.Available() && token.Origin() != 0 && token.Revision() != 0 && token.Digest() != 0, owner: owner, token: token}
	if !w.valid {
		return w
	}
	_, _ = w.h.Write([]byte("wippy.target/semantic-source-row/v2"))
	_, _ = w.h.Write([]byte{0})
	_, _ = w.h.Write(owner[:])
	var frame [16]byte
	binary.BigEndian.PutUint32(frame[0:4], uint32(token.Origin()))
	binary.BigEndian.PutUint16(frame[4:6], uint16(token.Facet()))
	binary.BigEndian.PutUint16(frame[6:8], uint16(token.Revision()))
	binary.BigEndian.PutUint64(frame[8:16], token.Digest())
	_, _ = w.h.Write(frame[:])
	return w
}

func (w *sourceRowWriter) u64(value uint64) {
	if w == nil || !w.valid {
		return
	}
	var frame [8]byte
	binary.BigEndian.PutUint64(frame[:], value)
	_, _ = w.h.Write(frame[:])
}

func (w *sourceRowWriter) id(value identity.ContentID) {
	if w == nil || !w.valid || !value.Available() {
		if w != nil {
			w.valid = false
		}
		return
	}
	_, _ = w.h.Write(value[:])
}

func (w *sourceRowWriter) text(value string) {
	if w == nil || !w.valid {
		return
	}
	w.u64(uint64(len(value)))
	_, _ = w.h.Write([]byte(value))
}

func (w *sourceRowWriter) input(value InputSource) {
	if w == nil || !w.valid {
		return
	}
	w.u64(uint64(value.Kind))
	w.u64(uint64(value.Ordinal))
}

func (w *sourceRowWriter) values(value Values) {
	w.u64(uint64(value))
}

func (w *sourceRowWriter) finish() (identity.ContentID, bool) {
	if w == nil || !w.valid {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], w.h.Sum(nil))
	return id, id.Available()
}

type targetRowEmitter struct {
	owner  identity.ContentID
	token  semanticsource.Token
	rows   []identity.ContentID
	failed bool
}

func writePublicationEffectDescriptor(writer *sourceRowWriter, descriptor PublicationEffectDescriptor) {
	if writer == nil || !descriptor.validConsequences() {
		if writer != nil {
			writer.valid = false
		}
		return
	}
	writer.u64(uint64(descriptor.Kind()))
	writer.u64(uint64(descriptor.Subject()))
	writer.u64(uint64(descriptor.DestinationRole()))
	writer.u64(uint64(descriptor.Context()))
	writer.u64(uint64(descriptor.Escape()))
	writer.u64(uint64(descriptor.Mutability()))
	writer.u64(uint64(descriptor.Lifetime()))
}

func (emitter *targetRowEmitter) row(write func(*sourceRowWriter)) {
	if emitter == nil || emitter.failed || write == nil {
		return
	}
	writer := newSourceRowWriter(emitter.owner, emitter.token)
	write(writer)
	digest, ok := writer.finish()
	if !ok {
		emitter.failed = true
		return
	}
	emitter.rows = append(emitter.rows, digest)
}

func sourceRows(c *Contract, token semanticsource.Token, emit func(*targetRowEmitter)) (semanticsource.DigestView, bool) {
	if c == nil || !c.semanticSourceReady() || emit == nil {
		return semanticsource.DigestView{}, false
	}
	emitter := &targetRowEmitter{owner: c.ContentID(), token: token}
	emit(emitter)
	if emitter.failed {
		return semanticsource.DigestView{}, false
	}
	return semanticsource.SealDigestView(emitter.owner, emitter.rows)
}

func (c *Contract) operationRows(token semanticsource.Token, write func(*sourceRowWriter, Operation) bool) (semanticsource.DigestView, bool) {
	return sourceRows(c, token, func(emitter *targetRowEmitter) {
		for index := 0; index < c.OperationCount(); index++ {
			op, ok := c.OperationAt(index)
			if !ok {
				emitter.failed = true
				return
			}
			emitter.row(func(writer *sourceRowWriter) {
				ok = write(writer, op)
			})
			if !ok {
				emitter.failed = true
				return
			}
		}
	})
}

func (c *Contract) operationIdentity(op Operation, writer *sourceRowWriter) bool {
	if writer == nil {
		return false
	}
	id, ok := c.OperationContentID(op)
	if !ok {
		return false
	}
	writer.id(id)
	writer.u64(uint64(op))
	return true
}

func (c *Contract) buildSourceViews() (SourceViews, bool) {
	if !c.semanticSourceReady() {
		return SourceViews{}, false
	}
	views := SourceViews{owner: c.ContentID()}
	if !c.buildOperationSourceRows(&views) || !c.buildProtocolSourceRows(&views) || !c.buildBootSourceRows(&views) {
		return SourceViews{}, false
	}
	return views, views.valid()
}

func buildTargetSourceViews(c *Contract) (SourceViews, bool) {
	if c == nil {
		return SourceViews{}, false
	}
	return c.buildSourceViews()
}

func boolWord(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}

func tokenTarget(origin semanticsource.Origin, facet semanticsource.Facet) semanticsource.Token {
	definition, _ := semanticsource.Declare(origin, facet)
	return definition.Token()
}

func writerOwnerID(c *Contract, op Operation) identity.ContentID {
	id, _ := c.OperationContentID(op)
	return id
}

func (c *Contract) operationRowsNested(token semanticsource.Token, emit func(*targetRowEmitter, Operation, identity.ContentID)) (semanticsource.DigestView, bool) {
	return sourceRows(c, token, func(emitter *targetRowEmitter) {
		for index := 0; index < c.OperationCount(); index++ {
			op, ok := c.OperationAt(index)
			if !ok {
				emitter.failed = true
				return
			}
			opID, idOK := c.OperationContentID(op)
			if !idOK {
				emitter.failed = true
				return
			}
			emit(emitter, op, opID)
			if emitter.failed {
				return
			}
		}
	})
}

func (c *Contract) callbackRows(token semanticsource.Token, emit func(*targetRowEmitter, CallbackID)) (semanticsource.DigestView, bool) {
	return sourceRows(c, token, func(emitter *targetRowEmitter) {
		for opIndex := 0; opIndex < c.OperationCount(); opIndex++ {
			op, ok := c.OperationAt(opIndex)
			if !ok {
				emitter.failed = true
				return
			}
			for index := 0; index < c.CallbackCount(op); index++ {
				callback, found := c.CallbackAt(op, index)
				if !found {
					emitter.failed = true
					return
				}
				emit(emitter, callback)
				if emitter.failed {
					return
				}
			}
		}
	})
}

func (c *Contract) resumeRows(token semanticsource.Token, emit func(*targetRowEmitter, ResumeID)) (semanticsource.DigestView, bool) {
	return sourceRows(c, token, func(emitter *targetRowEmitter) {
		for opIndex := 0; opIndex < c.OperationCount(); opIndex++ {
			op, ok := c.OperationAt(opIndex)
			if !ok {
				emitter.failed = true
				return
			}
			for index := 0; index < c.ResumeCount(op); index++ {
				resume, found := c.ResumeIDAt(op, index)
				if !found {
					emitter.failed = true
					return
				}
				emit(emitter, resume)
				if emitter.failed {
					return
				}
			}
		}
	})
}

func (c *Contract) spawnRows(token semanticsource.Token, emit func(*targetRowEmitter, Operation, identity.ContentID)) (semanticsource.DigestView, bool) {
	return c.operationRowsNested(token, emit)
}

func (c *Contract) protocolRows(token semanticsource.Token, emit func(*targetRowEmitter, Protocol)) (semanticsource.DigestView, bool) {
	return sourceRows(c, token, func(emitter *targetRowEmitter) {
		for index := 0; index < c.ProtocolCount(); index++ {
			protocol, ok := c.ProtocolAt(index)
			if !ok {
				emitter.failed = true
				return
			}
			emit(emitter, protocol)
			if emitter.failed {
				return
			}
		}
	})
}

// ResumeContentIDForResume keeps row traversal on public typed projections;
// the identity method itself is operation-owned, so resolve its owner through
// the dense Resume query rather than reaching into Contract tables.
func (c *Contract) ResumeContentIDForResume(resume ResumeID) (identity.ContentID, bool) {
	for index := 0; index < c.OperationCount(); index++ {
		op, ok := c.OperationAt(index)
		if !ok {
			return identity.ContentID{}, false
		}
		for row := 0; row < c.ResumeCount(op); row++ {
			candidate, found := c.ResumeIDAt(op, row)
			if found && candidate == resume {
				return c.ResumeContentID(op, resume)
			}
		}
	}
	return identity.ContentID{}, false
}
