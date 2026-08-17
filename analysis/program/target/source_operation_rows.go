package target

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

func (c *Contract) buildOperationSourceRows(views *SourceViews) bool {
	if c == nil || views == nil {
		return false
	}
	var ok bool
	views.contract, ok = sourceRows(c, tokenTarget(semanticsource.OriginTargetContract, 0), func(emitter *targetRowEmitter) {
		emitter.row(func(writer *sourceRowWriter) { writer.id(c.ContentID()) })
	})
	if !ok {
		return false
	}
	views.operation, ok = c.operationRows(tokenTarget(semanticsource.OriginTargetOperation, 0), func(writer *sourceRowWriter, op Operation) bool {
		return c.operationIdentity(op, writer)
	})
	if !ok {
		return false
	}
	views.abi, ok = c.operationRows(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI), func(writer *sourceRowWriter, op Operation) bool {
		return c.operationIdentity(op, writer)
	})
	if !ok {
		return false
	}
	views.subedge, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for index := 0; index < c.SubedgeCount(op); index++ {
			edge, found := c.SubedgeAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.callback, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback), func(emitter *targetRowEmitter, op Operation, _ identity.ContentID) {
		for index := 0; index < c.CallbackCount(op); index++ {
			callback, found := c.CallbackAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.binding, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for index := 0; index < c.BindingCount(op); index++ {
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.resume, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume), func(emitter *targetRowEmitter, op Operation, _ identity.ContentID) {
		for index := 0; index < c.ResumeCount(op); index++ {
			resume, found := c.ResumeIDAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.spawn, ok = c.operationRowsNested(tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn), func(emitter *targetRowEmitter, op Operation, opID identity.ContentID) {
		for index := 0; index < c.SpawnCount(op); index++ {
			spawn, found := c.SpawnIDAt(op, index)
			if !found {
				emitter.failed = true
				return
			}
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.opaque, ok = sourceRows(c, tokenTarget(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque), func(emitter *targetRowEmitter) {
		op, found := c.Opaque()
		if !found {
			emitter.failed = true
			return
		}
		emitter.row(func(writer *sourceRowWriter) {
			if !c.operationIdentity(op, writer) {
				writer.valid = false
			}
		})
	})
	if !ok {
		return false
	}

	if !c.buildOperationEffectRows(views) {
		return false
	}
	return true
}
