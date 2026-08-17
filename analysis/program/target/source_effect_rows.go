package target

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

func (c *Contract) buildOperationEffectRows(views *SourceViews) bool {
	if c == nil || views == nil {
		return false
	}
	var ok bool
	views.operationEffect, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for index := 0; index < c.EffectCount(op); index++ {
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.callbackEffect, ok = c.callbackRows(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect), func(emitter *targetRowEmitter, callback CallbackID) {
		for index := 0; index < c.CallbackEffectCount(callback); index++ {
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.publicationEffect, ok = sourceRows(c, tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetPublicationEffect), func(emitter *targetRowEmitter) {
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
				emitter.row(func(writer *sourceRowWriter) {
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
					emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.callbackRelease, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackRelease), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for index := 0; index < c.CallbackReleaseCount(op); index++ {
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.outcome, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for index := 0; index < c.OutcomeCount(op); index++ {
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.transfer, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for index := 0; index < c.TransferCount(op); index++ {
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.transferOutcome, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransferOutcome), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for index := 0; index < c.TransferCount(op); index++ {
			transfer, found := c.TransferIDAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			for outcome := 0; outcome < c.TransferOutcomeCount(op, index); outcome++ {
				emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.suspension, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSuspension), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for index := 0; index < c.SuspensionCount(op); index++ {
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.resumeOutcome, ok = c.resumeRows(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResumeOutcome), func(emitter *targetRowEmitter, resume ResumeID) {
		for index := 0; index < c.ResumeOutcomeCount(resume); index++ {
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.spawnSibling, ok = c.spawnRows(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawnSibling), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for index := 0; index < c.SpawnCount(op); index++ {
			spawn, found := c.SpawnIDAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			for sibling := 0; sibling < c.SpawnSiblingCount(spawn); sibling++ {
				emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.subedgeArgumentOrigin, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedgeArgumentOrigin), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for index := 0; index < c.SubedgeCount(op); index++ {
			edge, found := c.SubedgeAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			for argument := 0; argument < c.ArgumentOriginCount(edge); argument++ {
				emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.callbackResult, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackResult), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for outcome := 0; outcome < c.OutcomeCount(op); outcome++ {
			for index := 0; index < c.CallbackResultCount(op, outcome); index++ {
				emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.resultAlias, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResultAlias), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for outcome := 0; outcome < c.OutcomeCount(op); outcome++ {
			for index := 0; index < c.ResultAliasCount(op, outcome); index++ {
				emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.produced, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProduced), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for outcome := 0; outcome < c.OutcomeCount(op); outcome++ {
			for index := 0; index < c.ProducedCount(op, outcome); index++ {
				emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.producedCapture, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProducedCapture), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for outcome := 0; outcome < c.OutcomeCount(op); outcome++ {
			for produced := 0; produced < c.ProducedCount(op, outcome); produced++ {
				for capture := 0; capture < c.ProducedCaptureCount(op, outcome, produced); capture++ {
					emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.freshResult, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetFreshResult), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for outcome := 0; outcome < c.OutcomeCount(op); outcome++ {
			for index := 0; index < c.FreshResultCount(op, outcome); index++ {
				emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}

	return true
}
