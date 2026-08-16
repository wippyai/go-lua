package target

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// targetReceiptWriter encodes the typed row returned by a public owner query.
// It is not a row model: the digest is the detached receipt of that query's
// exact scalar/identity projection.
type targetReceiptWriter struct {
	h     hash.Hash
	valid bool
	owner keyspace.ContentID
	token semanticsource.Token
}

func newTargetReceiptWriter(owner keyspace.ContentID, token semanticsource.Token) *targetReceiptWriter {
	w := &targetReceiptWriter{h: sha256.New(), valid: owner.Available() && token.Origin() != 0 && token.Revision() != 0 && token.Digest() != 0, owner: owner, token: token}
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

func (w *targetReceiptWriter) u64(value uint64) {
	if w == nil || !w.valid {
		return
	}
	var frame [8]byte
	binary.BigEndian.PutUint64(frame[:], value)
	_, _ = w.h.Write(frame[:])
}

func (w *targetReceiptWriter) id(value keyspace.ContentID) {
	if w == nil || !w.valid || !value.Available() {
		if w != nil {
			w.valid = false
		}
		return
	}
	_, _ = w.h.Write(value[:])
}

func (w *targetReceiptWriter) text(value string) {
	if w == nil || !w.valid {
		return
	}
	w.u64(uint64(len(value)))
	_, _ = w.h.Write([]byte(value))
}

func (w *targetReceiptWriter) input(value InputSource) {
	if w == nil || !w.valid {
		return
	}
	w.u64(uint64(value.Kind))
	w.u64(uint64(value.Ordinal))
}

func (w *targetReceiptWriter) values(value Values) {
	w.u64(uint64(value))
}

func (w *targetReceiptWriter) finish() (keyspace.ContentID, bool) {
	if w == nil || !w.valid {
		return keyspace.ContentID{}, false
	}
	var id keyspace.ContentID
	copy(id[:], w.h.Sum(nil))
	return id, id.Available()
}

type targetRowEmitter struct {
	owner  keyspace.ContentID
	token  semanticsource.Token
	rows   []keyspace.ContentID
	failed bool
}

func writePublicationEffectDescriptor(writer *targetReceiptWriter, descriptor PublicationEffectDescriptor) {
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

func (emitter *targetRowEmitter) row(write func(*targetReceiptWriter)) {
	if emitter == nil || emitter.failed || write == nil {
		return
	}
	writer := newTargetReceiptWriter(emitter.owner, emitter.token)
	write(writer)
	digest, ok := writer.finish()
	if !ok {
		emitter.failed = true
		return
	}
	emitter.rows = append(emitter.rows, digest)
}

func targetTypedRows(c *Contract, token semanticsource.Token, emit func(*targetRowEmitter)) (SemanticSourceView, bool) {
	if c == nil || !c.semanticSourceReady() || emit == nil {
		return SemanticSourceView{}, false
	}
	emitter := &targetRowEmitter{owner: c.ContentID(), token: token}
	emit(emitter)
	if emitter.failed {
		return SemanticSourceView{}, false
	}
	return SemanticSourceView{owner: emitter.owner, digests: emitter.rows}, true
}

func (c *Contract) operationRows(token semanticsource.Token, write func(*targetReceiptWriter, Operation) bool) (SemanticSourceView, bool) {
	return targetTypedRows(c, token, func(emitter *targetRowEmitter) {
		for index := 0; index < c.OperationCount(); index++ {
			op, ok := c.OperationAt(index)
			if !ok {
				emitter.failed = true
				return
			}
			emitter.row(func(writer *targetReceiptWriter) {
				ok = write(writer, op)
			})
			if !ok {
				emitter.failed = true
				return
			}
		}
	})
}

func (c *Contract) operationIdentity(op Operation, writer *targetReceiptWriter) bool {
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

func (c *Contract) buildTargetViews() (SemanticSourceViews, bool) {
	if !c.semanticSourceReady() {
		return SemanticSourceViews{}, false
	}
	owner := c.ContentID()
	views := SemanticSourceViews{owner: owner}
	var ok bool
	views.contract, ok = targetTypedRows(c, tokenTarget(semanticsource.OriginTargetContract, 0), func(emitter *targetRowEmitter) {
		emitter.row(func(writer *targetReceiptWriter) { writer.id(owner) })
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.operation, ok = c.operationRows(tokenTarget(semanticsource.OriginTargetOperation, 0), func(writer *targetReceiptWriter, op Operation) bool {
		return c.operationIdentity(op, writer)
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.abi, ok = c.operationRows(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI), func(writer *targetReceiptWriter, op Operation) bool {
		return c.operationIdentity(op, writer)
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.subedge, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for index := 0; index < c.SubedgeCount(op); index++ {
			edge, found := c.SubedgeAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			emitter.row(func(writer *targetReceiptWriter) {
				writer.id(opID)
				writer.u64(uint64(edge))
				role, roleOK := c.SubedgeRole(edge)
				family, familyOK := c.SubedgeFamily(edge)
				callee, calleeOK := c.SubedgeCallee(edge)
				admission, admissionOK := c.SubedgeAdmission(edge)
				writer.u64(uint64(role))
				writer.u64(uint64(family))
				writer.u64(uint64(callee))
				writer.u64(uint64(admission))
				if !roleOK || !familyOK || !calleeOK || !admissionOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.callback, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback), func(emitter *targetRowEmitter, op Operation, _ keyspace.ContentID) {
		for index := 0; index < c.CallbackCount(op); index++ {
			callback, found := c.CallbackAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			emitter.row(func(writer *targetReceiptWriter) {
				id, idOK := c.CallbackContentID(op, callback)
				writer.id(id)
				writer.u64(uint64(callback))
				writer.u64(uint64(op))
				if !idOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.binding, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for index := 0; index < c.BindingCount(op); index++ {
			emitter.row(func(writer *targetReceiptWriter) {
				writer.id(opID)
				writer.u64(uint64(index))
				namespace, namespaceOK := c.BindingNamespaceAt(op, index)
				writer.u64(uint64(namespace))
				for part := 0; part < c.BindingOwnerCountAt(op, index); part++ {
					key, keyOK := c.BindingOwnerKeyAt(op, index, part)
					writer.u64(uint64(key))
					if !keyOK {
						writer.valid = false
					}
				}
				for part := 0; part < c.BindingMemberCountAt(op, index); part++ {
					key, keyOK := c.BindingMemberKeyAt(op, index, part)
					writer.u64(uint64(key))
					if !keyOK {
						writer.valid = false
					}
				}
				if !namespaceOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.resume, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume), func(emitter *targetRowEmitter, op Operation, _ keyspace.ContentID) {
		for index := 0; index < c.ResumeCount(op); index++ {
			resume, found := c.ResumeIDAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			emitter.row(func(writer *targetReceiptWriter) {
				id, idOK := c.ResumeContentID(op, resume)
				writer.id(id)
				writer.u64(uint64(resume))
				if !idOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.spawn, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for index := 0; index < c.SpawnCount(op); index++ {
			spawn, found := c.SpawnIDAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			emitter.row(func(writer *targetReceiptWriter) {
				ownerOp, function, child, yield, resume, childEntry, values, spawnOK := c.Spawn(spawn)
				writer.id(opID)
				writer.u64(uint64(spawn))
				writer.u64(uint64(ownerOp))
				writer.input(function)
				writer.u64(uint64(child))
				writer.u64(uint64(yield))
				writer.u64(uint64(resume))
				writer.values(childEntry)
				writer.values(values)
				if !spawnOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.opaque, ok = targetTypedRows(c, tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque), func(emitter *targetRowEmitter) {
		op, found := c.Opaque()
		if !found {
			emitter.failed = true
			return
		}
		emitter.row(func(writer *targetReceiptWriter) {
			if !c.operationIdentity(op, writer) {
				writer.valid = false
			}
		})
	})
	if !ok {
		return SemanticSourceViews{}, false
	}

	views.operationEffect, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for index := 0; index < c.EffectCount(op); index++ {
			emitter.row(func(writer *targetReceiptWriter) {
				id, found := c.EffectOccurrenceID(op, index)
				writer.id(id)
				writer.id(opID)
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.callbackEffect, ok = c.callbackRows(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect), func(emitter *targetRowEmitter, callback CallbackID) {
		for index := 0; index < c.CallbackEffectCount(callback); index++ {
			emitter.row(func(writer *targetReceiptWriter) {
				id, found := c.CallbackEffectOccurrenceID(callback, index)
				writer.id(id)
				writer.u64(uint64(callback))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.publicationEffect, ok = targetTypedRows(c, tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetPublicationEffect), func(emitter *targetRowEmitter) {
		for operationIndex := 0; operationIndex < c.OperationCount(); operationIndex++ {
			op, found := c.OperationAt(operationIndex)
			if !found {
				emitter.failed = true
				return
			}
			ownerID, ownerOK := c.EffectOperationID(op)
			if !ownerOK {
				emitter.failed = true
				return
			}
			for effectIndex := 0; effectIndex < c.EffectCount(op); effectIndex++ {
				descriptor, present := c.PublicationEffectDescriptor(op, effectIndex)
				if !present {
					continue
				}
				descriptorID, descriptorOK := c.PublicationEffectDescriptorID(op, effectIndex)
				occurrenceID, occurrenceOK := c.PublicationEffectOccurrenceID(op, effectIndex)
				emitter.row(func(writer *targetReceiptWriter) {
					writer.id(ownerID)
					writer.id(descriptorID)
					writer.id(occurrenceID)
					writePublicationEffectDescriptor(writer, descriptor)
					if !descriptorOK || !occurrenceOK {
						writer.valid = false
					}
				})
			}
			for callbackIndex := 0; callbackIndex < c.CallbackCount(op); callbackIndex++ {
				callback, callbackOK := c.CallbackAt(op, callbackIndex)
				if !callbackOK {
					emitter.failed = true
					return
				}
				callbackID, callbackIDOK := c.CallbackContentID(op, callback)
				if !callbackIDOK {
					emitter.failed = true
					return
				}
				for effectIndex := 0; effectIndex < c.CallbackEffectCount(callback); effectIndex++ {
					descriptor, present := c.CallbackPublicationEffectDescriptor(callback, effectIndex)
					if !present {
						continue
					}
					descriptorID, descriptorOK := c.CallbackPublicationEffectDescriptorID(callback, effectIndex)
					occurrenceID, occurrenceOK := c.CallbackPublicationEffectOccurrenceID(callback, effectIndex)
					emitter.row(func(writer *targetReceiptWriter) {
						writer.id(ownerID)
						writer.id(callbackID)
						writer.id(descriptorID)
						writer.id(occurrenceID)
						writePublicationEffectDescriptor(writer, descriptor)
						if !descriptorOK || !occurrenceOK {
							writer.valid = false
						}
					})
				}
			}
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.callbackRelease, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackRelease), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for index := 0; index < c.CallbackReleaseCount(op); index++ {
			emitter.row(func(writer *targetReceiptWriter) {
				callback, input, outcome, mode, found := c.CallbackReleaseAt(op, index)
				writer.id(opID)
				writer.u64(uint64(callback))
				writer.u64(uint64(input))
				writer.u64(uint64(outcome))
				writer.u64(uint64(mode))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.outcome, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for index := 0; index < c.OutcomeCount(op); index++ {
			emitter.row(func(writer *targetReceiptWriter) {
				kind, values, found := c.OutcomeAt(op, index)
				id, idOK := c.OutcomeContentID(op, index)
				writer.id(opID)
				writer.u64(uint64(kind))
				writer.values(values)
				writer.id(id)
				if !found || !idOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.transfer, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for index := 0; index < c.TransferCount(op); index++ {
			emitter.row(func(writer *targetReceiptWriter) {
				transfer, found := c.TransferIDAt(op, index)
				id, idOK := c.TransferContentID(op, transfer)
				writer.id(opID)
				writer.u64(uint64(transfer))
				writer.id(id)
				if !found || !idOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.transferOutcome, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransferOutcome), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for index := 0; index < c.TransferCount(op); index++ {
			transfer, found := c.TransferIDAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			for outcome := 0; outcome < c.TransferOutcomeCount(op, index); outcome++ {
				emitter.row(func(writer *targetReceiptWriter) {
					ordinal, possibility, rowOK := c.TransferOutcomeAt(op, index, outcome)
					id, _, idOK := c.TransferOutcomeContentID(op, transfer, outcome)
					writer.id(opID)
					writer.u64(uint64(transfer))
					writer.u64(uint64(ordinal))
					writer.u64(uint64(possibility))
					writer.id(id)
					if !rowOK || !idOK {
						writer.valid = false
					}
				})
			}
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.suspension, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSuspension), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for index := 0; index < c.SuspensionCount(op); index++ {
			emitter.row(func(writer *targetReceiptWriter) {
				yield, reentry, source, multiplicity, found := c.SuspensionAt(op, index)
				writer.id(opID)
				writer.u64(uint64(yield))
				writer.u64(uint64(reentry))
				writer.u64(uint64(source))
				writer.u64(uint64(multiplicity))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.resumeOutcome, ok = c.resumeRows(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResumeOutcome), func(emitter *targetRowEmitter, resume ResumeID) {
		for index := 0; index < c.ResumeOutcomeCount(resume); index++ {
			emitter.row(func(writer *targetReceiptWriter) {
				kind, outcome, found := c.ResumeOutcomeAt(resume, index)
				id, idOK := c.ResumeContentIDForResume(resume)
				writer.u64(uint64(resume))
				writer.u64(uint64(kind))
				writer.u64(uint64(outcome))
				writer.id(id)
				if !found || !idOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.spawnSibling, ok = c.spawnRows(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawnSibling), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for index := 0; index < c.SpawnCount(op); index++ {
			spawn, found := c.SpawnIDAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			for sibling := 0; sibling < c.SpawnSiblingCount(spawn); sibling++ {
				emitter.row(func(writer *targetReceiptWriter) {
					value, rowOK := c.SpawnSiblingAt(spawn, sibling)
					writer.id(opID)
					writer.u64(uint64(spawn))
					writer.u64(uint64(value))
					if !rowOK {
						writer.valid = false
					}
				})
			}
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.subedgeArgumentOrigin, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedgeArgumentOrigin), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for index := 0; index < c.SubedgeCount(op); index++ {
			edge, found := c.SubedgeAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			for argument := 0; argument < c.ArgumentOriginCount(edge); argument++ {
				emitter.row(func(writer *targetReceiptWriter) {
					segment, ordinal, source, input, rowOK := c.ArgumentOriginAt(edge, argument)
					writer.id(opID)
					writer.u64(uint64(edge))
					writer.u64(uint64(segment))
					writer.u64(uint64(ordinal))
					writer.u64(uint64(source))
					writer.input(input)
					if !rowOK {
						writer.valid = false
					}
				})
			}
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.callbackResult, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackResult), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for outcome := 0; outcome < c.OutcomeCount(op); outcome++ {
			for index := 0; index < c.CallbackResultCount(op, outcome); index++ {
				emitter.row(func(writer *targetReceiptWriter) {
					result, callback, found := c.CallbackResultAt(op, outcome, index)
					writer.id(opID)
					writer.u64(uint64(outcome))
					writer.u64(uint64(result))
					writer.u64(uint64(callback))
					if !found {
						writer.valid = false
					}
				})
			}
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.resultAlias, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResultAlias), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for outcome := 0; outcome < c.OutcomeCount(op); outcome++ {
			for index := 0; index < c.ResultAliasCount(op, outcome); index++ {
				emitter.row(func(writer *targetReceiptWriter) {
					result, kind, source, found := c.ResultAliasAt(op, outcome, index)
					writer.id(opID)
					writer.u64(uint64(outcome))
					writer.u64(uint64(result))
					writer.u64(uint64(kind))
					writer.u64(uint64(source))
					if !found {
						writer.valid = false
					}
				})
			}
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.produced, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProduced), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for outcome := 0; outcome < c.OutcomeCount(op); outcome++ {
			for index := 0; index < c.ProducedCount(op, outcome); index++ {
				emitter.row(func(writer *targetReceiptWriter) {
					result, target, found := c.ProducedAt(op, outcome, index)
					writer.id(opID)
					writer.u64(uint64(outcome))
					writer.u64(uint64(result))
					targetID, targetOK := c.OperationContentID(target)
					writer.id(targetID)
					if !found || !targetOK {
						writer.valid = false
					}
				})
			}
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.producedCapture, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProducedCapture), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for outcome := 0; outcome < c.OutcomeCount(op); outcome++ {
			for produced := 0; produced < c.ProducedCount(op, outcome); produced++ {
				for capture := 0; capture < c.ProducedCaptureCount(op, outcome, produced); capture++ {
					emitter.row(func(writer *targetReceiptWriter) {
						kind, ordinal, found := c.ProducedCaptureAt(op, outcome, produced, capture)
						writer.id(opID)
						writer.u64(uint64(outcome))
						writer.u64(uint64(produced))
						writer.u64(uint64(capture))
						writer.u64(uint64(kind))
						writer.u64(uint64(ordinal))
						if !found {
							writer.valid = false
						}
					})
				}
			}
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.freshResult, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetFreshResult), func(emitter *targetRowEmitter, op Operation, opID keyspace.ContentID) {
		for outcome := 0; outcome < c.OutcomeCount(op); outcome++ {
			for index := 0; index < c.FreshResultCount(op, outcome); index++ {
				emitter.row(func(writer *targetReceiptWriter) {
					result, ordinal, kind, found := c.FreshResultAt(op, outcome, index)
					writer.id(opID)
					writer.u64(uint64(outcome))
					writer.u64(uint64(result))
					writer.u64(uint64(ordinal))
					writer.u64(uint64(kind))
					if !found {
						writer.valid = false
					}
				})
			}
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}

	views.protocol, ok = targetTypedRows(c, tokenTarget(semanticsource.OriginTargetProtocol, 0), func(emitter *targetRowEmitter) {
		for index := 0; index < c.ProtocolCount(); index++ {
			protocol, found := c.ProtocolAt(index)
			emitter.row(func(writer *targetReceiptWriter) {
				writer.u64(uint64(protocol))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.protocolState, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolState), func(emitter *targetRowEmitter, protocol Protocol) {
		for index := 0; index < c.StateCount(protocol); index++ {
			state, found := c.StateAt(protocol, index)
			name, nameOK := c.StateName(protocol, state)
			final, finalOK := c.StateFinal(protocol, state)
			emitter.row(func(writer *targetReceiptWriter) {
				writer.u64(uint64(protocol))
				writer.u64(uint64(state))
				writer.text(name)
				writer.u64(boolWord(final))
				if !found || !nameOK || !finalOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.protocolAcquisition, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolAcquisition), func(emitter *targetRowEmitter, protocol Protocol) {
		for index := 0; index < c.ProtocolAcquisitionCount(protocol); index++ {
			op, outcome, result, state, found := c.ProtocolAcquisitionAt(protocol, index)
			emitter.row(func(writer *targetReceiptWriter) {
				writer.u64(uint64(protocol))
				writer.u64(uint64(op))
				writer.u64(uint64(outcome))
				writer.u64(uint64(result))
				writer.u64(uint64(state))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.protocolTransition, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransition), func(emitter *targetRowEmitter, protocol Protocol) {
		for index := 0; index < c.TransitionCount(protocol); index++ {
			op, input, ordinal, from, found := c.TransitionAt(protocol, index)
			emitter.row(func(writer *targetReceiptWriter) {
				writer.u64(uint64(protocol))
				writer.u64(uint64(op))
				writer.u64(uint64(input))
				writer.u64(uint64(ordinal))
				writer.u64(uint64(from))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.protocolTransitionOutcome, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransitionOutcome), func(emitter *targetRowEmitter, protocol Protocol) {
		for transition := 0; transition < c.TransitionCount(protocol); transition++ {
			for index := 0; index < c.TransitionOutcomeCount(protocol, transition); index++ {
				emitter.row(func(writer *targetReceiptWriter) {
					outcome, state, found := c.TransitionOutcomeAt(protocol, transition, index)
					writer.u64(uint64(protocol))
					writer.u64(uint64(transition))
					writer.u64(uint64(index))
					writer.u64(uint64(outcome))
					writer.u64(uint64(state))
					if !found {
						writer.valid = false
					}
				})
			}
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.protocolEscape, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolEscape), func(emitter *targetRowEmitter, protocol Protocol) {
		for index := 0; index < c.EscapeCount(protocol); index++ {
			op, input, ordinal, found := c.EscapeAt(protocol, index)
			emitter.row(func(writer *targetReceiptWriter) {
				writer.u64(uint64(protocol))
				writer.u64(uint64(op))
				writer.u64(uint64(input))
				writer.u64(uint64(ordinal))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.protocolCallbackHolder, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolCallbackHolder), func(emitter *targetRowEmitter, protocol Protocol) {
		for index := 0; index < c.ProtocolCallbackHolderCount(protocol); index++ {
			op, input, callback, found := c.ProtocolCallbackHolderAt(protocol, index)
			emitter.row(func(writer *targetReceiptWriter) {
				writer.u64(uint64(protocol))
				writer.u64(uint64(op))
				writer.input(input)
				writer.u64(uint64(callback))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}

	views.boot, ok = targetTypedRows(c, tokenTarget(semanticsource.OriginTargetBoot, 0), func(emitter *targetRowEmitter) {
		for index := 0; index < c.InitialRootCount(); index++ {
			root, found := c.InitialRootAt(index)
			identity, identityOK := c.InitialRootIdentity(root)
			emitter.row(func(writer *targetReceiptWriter) {
				writer.u64(uint64(root))
				writer.text(identity)
				if !found || !identityOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.bootEntry, ok = targetTypedRows(c, tokenTarget(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootEntry), func(emitter *targetRowEmitter) {
		for index := 0; index < c.InitialEntryCount(); index++ {
			root, key, value, mutability, found := c.InitialEntryAt(index)
			valueID, valueOK := c.InitialValueContentID(value)
			emitter.row(func(writer *targetReceiptWriter) {
				writer.u64(uint64(root))
				writer.u64(uint64(key))
				writer.id(valueID)
				writer.u64(uint64(mutability))
				if !found || !valueOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.bootMetatableAttachment, ok = targetTypedRows(c, tokenTarget(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootMetatableAttachment), func(emitter *targetRowEmitter) {
		for index := 0; index < c.InitialMetatableAttachmentCount(); index++ {
			base, metatable, found := c.InitialMetatableAttachmentAt(index)
			writer := func(w *targetReceiptWriter) {
				w.u64(uint64(base))
				w.u64(uint64(metatable))
				if !found {
					w.valid = false
				}
			}
			emitter.row(writer)
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.bootBinding, ok = targetTypedRows(c, tokenTarget(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootBinding), func(emitter *targetRowEmitter) {
		for index := 0; index < c.InitialBindingCount(); index++ {
			name, class, value, root, key, found := c.InitialBindingAt(index)
			valueID, valueOK := c.InitialValueContentID(value)
			emitter.row(func(writer *targetReceiptWriter) {
				writer.text(name)
				writer.u64(uint64(class))
				writer.id(valueID)
				writer.u64(uint64(root))
				writer.u64(uint64(key))
				if !found || !valueOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.gsub, ok = targetTypedRows(c, tokenTarget(semanticsource.OriginTargetGsub, 0), func(emitter *targetRowEmitter) {
		for index := 0; index < c.OperationCount(); index++ {
			op, found := c.OperationAt(index)
			if !found {
				emitter.failed = true
				return
			}
			replacement, key, access, resultOutcome, result, branch := c.GsubTableReplacement(op)
			if !branch {
				continue
			}
			emitter.row(func(writer *targetReceiptWriter) {
				writer.id(writerOwnerID(c, op))
				writer.u64(uint64(replacement))
				writer.u64(uint64(key))
				writer.u64(uint64(access))
				writer.u64(uint64(resultOutcome))
				writer.u64(uint64(result))
				for aliasIndex := 0; aliasIndex < c.GsubTableReplacementEffectAliasCount(op); aliasIndex++ {
					alias, aliasOK := c.GsubTableReplacementEffectAliasAt(op, aliasIndex)
					writer.u64(uint64(alias))
					if !aliasOK {
						writer.valid = false
					}
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}
	return views, views.valid()
}

func buildTargetSemanticSourceReceipt(c *Contract) (SemanticSourceReceipt, bool) {
	views, ok := c.buildTargetViews()
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	receipt := SemanticSourceReceipt{owner: c.ContentID(), views: views}
	return receipt, receipt.Valid()
}

func boolWord(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}

func tokenTarget(origin semanticsource.Origin, facet semanticsource.Facet) semanticsource.Token {
	definition, _ := semanticsource.Definition(origin, facet)
	return definition.Token()
}

func writerOwnerID(c *Contract, op Operation) keyspace.ContentID {
	id, _ := c.OperationContentID(op)
	return id
}

func (c *Contract) operationRowsNested(token semanticsource.Token, emit func(*targetRowEmitter, Operation, keyspace.ContentID)) (SemanticSourceView, bool) {
	return targetTypedRows(c, token, func(emitter *targetRowEmitter) {
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

func (c *Contract) callbackRows(token semanticsource.Token, emit func(*targetRowEmitter, CallbackID)) (SemanticSourceView, bool) {
	return targetTypedRows(c, token, func(emitter *targetRowEmitter) {
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

func (c *Contract) resumeRows(token semanticsource.Token, emit func(*targetRowEmitter, ResumeID)) (SemanticSourceView, bool) {
	return targetTypedRows(c, token, func(emitter *targetRowEmitter) {
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

func (c *Contract) spawnRows(token semanticsource.Token, emit func(*targetRowEmitter, Operation, keyspace.ContentID)) (SemanticSourceView, bool) {
	return c.operationRowsNested(token, emit)
}

func (c *Contract) protocolRows(token semanticsource.Token, emit func(*targetRowEmitter, Protocol)) (SemanticSourceView, bool) {
	return targetTypedRows(c, token, func(emitter *targetRowEmitter) {
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
func (c *Contract) ResumeContentIDForResume(resume ResumeID) (keyspace.ContentID, bool) {
	for index := 0; index < c.OperationCount(); index++ {
		op, ok := c.OperationAt(index)
		if !ok {
			return keyspace.ContentID{}, false
		}
		for row := 0; row < c.ResumeCount(op); row++ {
			candidate, found := c.ResumeIDAt(op, row)
			if found && candidate == resume {
				return c.ResumeContentID(op, resume)
			}
		}
	}
	return keyspace.ContentID{}, false
}
