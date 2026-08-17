package target

import "github.com/wippyai/go-lua/analysis/program/semanticsource"

func (c *Contract) buildBootSourceRows(views *SourceViews) bool {
	if c == nil || views == nil {
		return false
	}
	var ok bool
	views.boot, ok = sourceRows(c, tokenTarget(semanticsource.OriginTargetBoot, 0), func(emitter *targetRowEmitter) {
		for index := 0; index < c.InitialRootCount(); index++ {
			root, found := c.InitialRootAt(index)
			identity, identityOK := c.InitialRootIdentity(root)
			emitter.row(func(writer *sourceRowWriter) {
				writer.u64(uint64(root))
				writer.text(identity)
				if !found || !identityOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return false
	}
	views.bootEntry, ok = sourceRows(c, tokenTarget(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootEntry), func(emitter *targetRowEmitter) {
		for index := 0; index < c.InitialEntryCount(); index++ {
			root, key, value, mutability, found := c.InitialEntryAt(index)
			valueID, valueOK := c.InitialValueContentID(value)
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.bootMetatableAttachment, ok = sourceRows(c, tokenTarget(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootMetatableAttachment), func(emitter *targetRowEmitter) {
		for index := 0; index < c.InitialMetatableAttachmentCount(); index++ {
			base, metatable, found := c.InitialMetatableAttachmentAt(index)
			writer := func(w *sourceRowWriter) {
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
		return false
	}
	views.bootBinding, ok = sourceRows(c, tokenTarget(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootBinding), func(emitter *targetRowEmitter) {
		for index := 0; index < c.InitialBindingCount(); index++ {
			name, class, value, root, key, found := c.InitialBindingAt(index)
			valueID, valueOK := c.InitialValueContentID(value)
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}
	views.gsub, ok = sourceRows(c, tokenTarget(semanticsource.OriginTargetGsub, 0), func(emitter *targetRowEmitter) {
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
			emitter.row(func(writer *sourceRowWriter) {
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
		return false
	}

	return true
}
