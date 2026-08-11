package static

import "github.com/wippyai/go-lua/program/semanticsource"

func staticSourceToken(facet semanticsource.Facet) (semanticsource.Token, bool) {
	definition, ok := semanticsource.Definition(semanticsource.OriginLinkStatic, facet)
	if !ok {
		return semanticsource.Token{}, false
	}
	return definition.Token(), true
}

func (c *Component) buildStaticViews() (SemanticSourceViews, bool) {
	if c == nil || !c.contentID.Available() {
		return SemanticSourceViews{}, false
	}
	owner := c.contentID
	views := SemanticSourceViews{owner: owner}

	token, ok := staticSourceToken(0)
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.static, ok = staticTypedRows(c, token, func(emitter *staticRowEmitter) {
		namespaces := c.Namespaces()
		for index := 0; index < namespaces.Count(); index++ {
			namespace, found := namespaces.At(index)
			id, idOK := namespaces.ContentID(namespace)
			shard, shardOK := namespaces.Shard(namespace)
			mount, mountOK := c.mounts.Index(shard)
			emitter.row(func(writer *staticReceiptWriter) {
				writer.id(id)
				writer.u64(uint64(mount + 1))
				if !found || !idOK || !shardOK || !mountOK || mount < 0 {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}

	token, ok = staticSourceToken(semanticsource.FacetLinkStaticResolution)
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.resolution, ok = staticTypedRows(c, token, func(emitter *staticRowEmitter) {
		resolutions := c.Resolutions()
		namespaces := c.Namespaces()
		for index := 0; index < resolutions.Count(); index++ {
			resolution, found := resolutions.At(index)
			shard, importTerm, call, literal, sourceOK := resolutions.Source(resolution)
			alias, aliasOK := resolutions.Alias(resolution)
			disposition, dispositionOK := resolutions.Disposition(resolution)
			mount, mountOK := c.mounts.Index(shard)
			emitter.row(func(writer *staticReceiptWriter) {
				writer.u64(uint64(mount + 1))
				writer.term(importTerm)
				writer.term(call)
				writer.term(literal)
				writer.term(alias)
				writer.u64(uint64(disposition))
				rowOK := found && sourceOK && aliasOK && dispositionOK && mountOK && mount >= 0
				if disposition == ResolutionResolved {
					namespace, namespaceOK := resolutions.Namespace(resolution)
					namespaceID, idOK := namespaces.ContentID(namespace)
					writer.id(namespaceID)
					rowOK = rowOK && namespaceOK && idOK
				} else if disposition != ResolutionUnresolved {
					rowOK = false
				}
				if !rowOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}

	token, ok = staticSourceToken(semanticsource.FacetLinkStaticExpression)
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.expression, ok = staticTypedRows(c, token, func(emitter *staticRowEmitter) {
		expressions := c.Expressions()
		for index := 0; index < expressions.Count(); index++ {
			expression, found := expressions.At(index)
			ref, refOK := expressions.Ref(expression)
			emitter.row(func(writer *staticReceiptWriter) {
				writer.expression(ref)
				if !found || !refOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}

	token, ok = staticSourceToken(semanticsource.FacetLinkStaticExport)
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.export, ok = staticTypedRows(c, token, func(emitter *staticRowEmitter) {
		namespaces := c.Namespaces()
		expressions := c.Expressions()
		for namespaceIndex := 0; namespaceIndex < namespaces.Count(); namespaceIndex++ {
			namespace, found := namespaces.At(namespaceIndex)
			namespaceID, idOK := namespaces.ContentID(namespace)
			if !found || !idOK {
				emitter.failed = true
				return
			}
			for exportIndex := 0; exportIndex < namespaces.ExportCount(namespace); exportIndex++ {
				path, pathOK := namespaces.ExportPath(namespace, exportIndex, nil)
				expression, expressionOK := namespaces.ExportExpression(namespace, exportIndex)
				ref, refOK := expressions.Ref(expression)
				emitter.row(func(writer *staticReceiptWriter) {
					writer.id(namespaceID)
					writer.u64(uint64(namespaceIndex + 1))
					writer.u64(uint64(exportIndex))
					writer.u64(uint64(len(path)))
					for _, key := range path {
						writer.u64(uint64(key))
						if key == 0 {
							writer.valid = false
						}
					}
					writer.expression(ref)
					if !pathOK || !expressionOK || !refOK || len(path) == 0 {
						writer.valid = false
					}
				})
			}
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}

	token, ok = staticSourceToken(semanticsource.FacetLinkStaticInput)
	if !ok {
		return SemanticSourceViews{}, false
	}
	views.input, ok = staticTypedRows(c, token, func(emitter *staticRowEmitter) {
		inputs := c.Inputs()
		expressions := c.Expressions()
		for index := 0; index < inputs.Count(); index++ {
			input, found := inputs.At(index)
			inputID, idOK := inputs.ID(input)
			kind, source, expression, operand, frontierBody, frontierCursor, sourceOK := inputs.Source(input)
			ref, refOK := expressions.Ref(expression)
			emitter.row(func(writer *staticReceiptWriter) {
				writer.id(inputID)
				writer.u64(uint64(index + 1))
				writer.u64(uint64(kind))
				writer.term(source)
				writer.expression(ref)
				writer.term(operand)
				writer.term(frontierBody)
				writer.u64(uint64(frontierCursor))
				if !found || !idOK || !sourceOK || !refOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return SemanticSourceViews{}, false
	}

	return views, views.valid()
}

func buildStaticSemanticSourceReceipt(c *Component) (SemanticSourceReceipt, bool) {
	views, ok := c.buildStaticViews()
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	receipt := SemanticSourceReceipt{owner: c.contentID, views: views}
	return receipt, receipt.Valid()
}
