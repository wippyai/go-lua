package static

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programstatic "github.com/wippyai/go-lua/program/static"
)

func (c *Component) namespace(v Namespace) (namespaceRow, bool) {
	if c == nil || v.source != c || v.ordinal == 0 || uint64(v.ordinal) > uint64(len(c.namespaces)) {
		return namespaceRow{}, false
	}
	return c.namespaces[v.ordinal-1], true
}
func (c *Component) resolution(v Resolution) (resolutionRow, bool) {
	if c == nil || v.source != c || v.ordinal == 0 || uint64(v.ordinal) > uint64(len(c.resolutions)) {
		return resolutionRow{}, false
	}
	return c.resolutions[v.ordinal-1], true
}
func (c *Component) input(v InputRef) (inputRow, bool) {
	if c == nil || v.source != c || v.ordinal == 0 || uint64(v.ordinal) > uint64(len(c.inputs)) {
		return inputRow{}, false
	}
	return c.inputs[v.ordinal-1], true
}
func (c *Component) expression(shard linkproject.Shard, term keyspace.Term) (Expression, bool) {
	p, ok := c.program(shard)
	if !ok {
		return Expression{}, false
	}
	ref, ok := p.Static().StaticTypes().Ref(term)
	if !ok {
		return Expression{}, false
	}
	e := Expression{source: c, shard: shard, reference: ref}
	return e, c.validExpression(e)
}
func (c *Component) validExpression(e Expression) bool {
	if c == nil || e.source != c || e.reference.Term() == 0 {
		return false
	}
	if _, ok := c.mounts.Index(e.shard); !ok {
		return false
	}
	p, ok := c.program(e.shard)
	if !ok {
		return false
	}
	want, ok := p.Static().StaticTypes().Ref(e.reference.Term())
	if !ok || want.Term() != e.reference.Term() {
		return false
	}
	_, ok = c.Namespaces().ForShard(e.shard)
	return ok
}

func (v Namespaces) Count() int {
	if v.source == nil {
		return 0
	}
	return len(v.source.namespaces)
}
func (v Namespaces) At(i int) (Namespace, bool) {
	if v.source == nil || i < 0 || i >= len(v.source.namespaces) {
		return Namespace{}, false
	}
	return Namespace{source: v.source, ordinal: uint32(i + 1)}, true
}
func (v Namespaces) ForShard(shard linkproject.Shard) (Namespace, bool) {
	if v.source == nil {
		return Namespace{}, false
	}
	index, ok := v.source.mounts.Index(shard)
	if !ok || index < 0 || index >= len(v.source.namespaces) {
		return Namespace{}, false
	}
	n := Namespace{source: v.source, ordinal: uint32(index + 1)}
	row, ok := v.source.namespace(n)
	return n, ok && row.shard == shard
}
func (v Namespaces) Shard(n Namespace) (linkproject.Shard, bool) {
	row, ok := v.source.namespace(n)
	return row.shard, ok
}
func (v Namespaces) ContentID(n Namespace) (keyspace.ContentID, bool) {
	row, ok := v.source.namespace(n)
	return row.content, ok && row.content.Available()
}
func (v Namespaces) Resolver(n Namespace) (Resolver, bool) {
	if _, ok := v.source.namespace(n); !ok {
		return Resolver{}, false
	}
	return Resolver{source: v.source, ordinal: n.ordinal}, true
}
func (v Namespaces) Namespace(r Resolver) (Namespace, bool) {
	n := Namespace{source: r.source, ordinal: r.ordinal}
	_, ok := v.source.namespace(n)
	return n, ok
}
func (v Namespaces) ResolverContentID(r Resolver) (keyspace.ContentID, bool) {
	n, ok := v.Namespace(r)
	if !ok {
		return keyspace.ContentID{}, false
	}
	return v.ContentID(n)
}
func (v Namespaces) ResolverShard(r Resolver) (linkproject.Shard, bool) {
	n, ok := v.Namespace(r)
	if !ok {
		return linkproject.Shard{}, false
	}
	return v.Shard(n)
}
func (v Namespaces) ResolverForShard(shard linkproject.Shard) (Resolver, bool) {
	n, ok := v.ForShard(shard)
	if !ok {
		return Resolver{}, false
	}
	return v.Resolver(n)
}
func (v Namespaces) ForAlias(shard linkproject.Shard, alias keyspace.Term) (Namespace, bool) {
	if v.source == nil || alias == 0 {
		return Namespace{}, false
	}
	if _, ok := v.source.mounts.Index(shard); !ok {
		return Namespace{}, false
	}
	n := v.source.byAlias[shardTerm{shard, alias}]
	_, ok := v.source.namespace(n)
	return n, ok
}
func (v Namespaces) ExportCount(n Namespace) int {
	row, ok := v.source.namespace(n)
	if !ok {
		return 0
	}
	return len(row.exports)
}
func (v Namespaces) ExportExpression(n Namespace, index int) (Expression, bool) {
	row, ok := v.source.namespace(n)
	if !ok || index < 0 || index >= len(row.exports) {
		return Expression{}, false
	}
	return v.source.expression(row.shard, row.exports[index].typeRef)
}
func (v Namespaces) ExportPath(n Namespace, index int, dst []keyspace.Key) ([]keyspace.Key, bool) {
	row, ok := v.source.namespace(n)
	if !ok || index < 0 || index >= len(row.exports) {
		return dst, false
	}
	path := row.exports[index].path
	start := len(dst)
	if cap(dst)-start < len(path) {
		dst = append(dst, make([]keyspace.Key, len(path))...)
	} else {
		dst = dst[:start+len(path)]
	}
	copy(dst[start:], path)
	return dst, true
}

func (v Expressions) Count() int {
	if v.source == nil || len(v.source.expressionEnds) != v.source.mounts.Count()+1 {
		return 0
	}
	return int(v.source.expressionEnds[len(v.source.expressionEnds)-1])
}
func (v Expressions) At(index int) (Expression, bool) {
	if v.source == nil || index < 0 || index >= v.Count() {
		return Expression{}, false
	}
	i := sort.Search(v.source.mounts.Count(), func(i int) bool { return int(v.source.expressionEnds[i+1]) > index })
	if i >= v.source.mounts.Count() {
		return Expression{}, false
	}
	shard, ok := v.source.mounts.At(i)
	if !ok {
		return Expression{}, false
	}
	p, ok := v.source.program(shard)
	if !ok {
		return Expression{}, false
	}
	ref, ok := p.Static().StaticTypes().At(index - int(v.source.expressionEnds[i]))
	if !ok {
		return Expression{}, false
	}
	return v.source.expression(shard, ref.Term())
}
func (v Expressions) Reference(e Expression) (programstatic.StaticTypeRef, bool) {
	if !v.source.validExpression(e) {
		return programstatic.StaticTypeRef{}, false
	}
	return e.reference, true
}
func (v Expressions) Shard(e Expression) (linkproject.Shard, bool) {
	if !v.source.validExpression(e) {
		return linkproject.Shard{}, false
	}
	return e.shard, true
}
func (v Expressions) Resolver(e Expression) (Resolver, bool) {
	if !v.source.validExpression(e) {
		return Resolver{}, false
	}
	return v.source.Namespaces().ResolverForShard(e.shard)
}
func (v Expressions) For(r Resolver, ref programstatic.StaticTypeRef) (Expression, bool) {
	if v.source == nil || ref.Term() == 0 {
		return Expression{}, false
	}
	shard, ok := v.source.Namespaces().ResolverShard(r)
	if !ok {
		return Expression{}, false
	}
	p, ok := v.source.program(shard)
	if !ok {
		return Expression{}, false
	}
	want, ok := p.Static().StaticTypes().Ref(ref.Term())
	if !ok || want.Term() != ref.Term() {
		return Expression{}, false
	}
	return v.source.expression(shard, ref.Term())
}
func (v Expressions) Ref(e Expression) (ExpressionRef, bool) {
	if !v.source.validExpression(e) || !v.source.contentID.Available() {
		return ExpressionRef{}, false
	}
	index, ok := v.source.mounts.Index(e.shard)
	if !ok || index < 0 || uint64(index+1) > uint64(^uint32(0)) {
		return ExpressionRef{}, false
	}
	return ExpressionRef{staticID: v.source.contentID, shardOrdinal: uint32(index + 1), reference: e.reference.Term()}, true
}
func (v Expressions) Find(ref ExpressionRef) (Expression, bool) {
	if v.source == nil || !v.source.contentID.Available() || ref.staticID != v.source.contentID || ref.shardOrdinal == 0 || ref.reference == 0 {
		return Expression{}, false
	}
	shard, ok := v.source.mounts.At(int(ref.shardOrdinal) - 1)
	if !ok {
		return Expression{}, false
	}
	p, ok := v.source.program(shard)
	if !ok {
		return Expression{}, false
	}
	want, ok := p.Static().StaticTypes().Ref(ref.reference)
	if !ok || want.Term() != ref.reference {
		return Expression{}, false
	}
	return v.source.expression(shard, ref.reference)
}
func (v Expressions) Qualified(e Expression) (Expression, bool) {
	if v.source == nil || !v.source.validExpression(e) {
		return Expression{}, false
	}
	term := e.reference.Term()
	ordinal := v.source.byQualified[shardTerm{e.shard, term}]
	if ordinal == 0 || uint64(ordinal) > uint64(len(v.source.qualified)) {
		return Expression{}, false
	}
	row := v.source.qualified[ordinal-1]
	p, ok := v.source.program(e.shard)
	if !ok || row.owner != p.ContentID() || row.reference != term || !row.providerOwner.Available() || row.target == 0 {
		return Expression{}, false
	}
	shard, ok := v.source.Namespaces().ResolverShard(row.resolver)
	if !ok {
		return Expression{}, false
	}
	return v.source.expression(shard, row.target)
}

// QualifiedCount and QualifiedAt expose the sealed qualified-reference
// relation in canonical shard/reference order without exposing its row form.
func (v Expressions) QualifiedCount() int {
	if v.source == nil {
		return 0
	}
	return len(v.source.qualified)
}

func (v Expressions) QualifiedAt(index int) (Expression, Expression, bool) {
	if v.source == nil || index < 0 || index >= len(v.source.qualified) {
		return Expression{}, Expression{}, false
	}
	row := v.source.qualified[index]
	consumer, ok := v.source.expression(row.consumerShard, row.reference)
	if !ok {
		return Expression{}, Expression{}, false
	}
	providerShard, ok := v.source.Namespaces().ResolverShard(row.resolver)
	if !ok {
		return Expression{}, Expression{}, false
	}
	provider, ok := v.source.expression(providerShard, row.target)
	return consumer, provider, ok
}

func (v Resolutions) Count() int {
	if v.source == nil {
		return 0
	}
	return len(v.source.resolutions)
}
func (v Resolutions) At(i int) (Resolution, bool) {
	if v.source == nil || i < 0 || i >= len(v.source.resolutions) {
		return Resolution{}, false
	}
	return Resolution{source: v.source, ordinal: uint32(i + 1)}, true
}
func (v Resolutions) ForImport(shard linkproject.Shard, item keyspace.Term) (Resolution, bool) {
	if v.source == nil || item == 0 {
		return Resolution{}, false
	}
	p, ok := v.source.program(shard)
	if !ok {
		return Resolution{}, false
	}
	row, ok := p.Module().Import(item)
	if !ok || row.Call == 0 {
		return Resolution{}, false
	}
	r := v.source.byCall[shardTerm{shard, row.Call}]
	_, ok = v.source.resolution(r)
	return r, ok
}
func (v Resolutions) ForCall(shard linkproject.Shard, call keyspace.Term) (Resolution, bool) {
	if v.source == nil || call == 0 {
		return Resolution{}, false
	}
	if _, ok := v.source.mounts.Index(shard); !ok {
		return Resolution{}, false
	}
	r := v.source.byCall[shardTerm{shard, call}]
	_, ok := v.source.resolution(r)
	return r, ok
}
func (v Resolutions) ForCallInShard(resolver Resolver, call keyspace.Term) (Resolution, bool) {
	shard, ok := v.source.Namespaces().ResolverShard(resolver)
	if !ok || call == 0 {
		return Resolution{}, false
	}
	r := v.source.byCall[shardTerm{shard, call}]
	_, ok = v.source.resolution(r)
	return r, ok
}
func (v Resolutions) Source(r Resolution) (linkproject.Shard, keyspace.Term, keyspace.Term, keyspace.Term, bool) {
	row, ok := v.source.resolution(r)
	if !ok {
		return linkproject.Shard{}, 0, 0, 0, false
	}
	return row.shard, row.importTerm, row.call, row.literal, true
}
func (v Resolutions) Alias(r Resolution) (keyspace.Term, bool) {
	row, ok := v.source.resolution(r)
	return row.alias, ok
}
func (v Resolutions) Disposition(r Resolution) (ResolutionDisposition, bool) {
	row, ok := v.source.resolution(r)
	if !ok || (row.disposition != ResolutionResolved && row.disposition != ResolutionUnresolved) {
		return ResolutionInvalid, false
	}
	return row.disposition, true
}
func (v Resolutions) Namespace(r Resolution) (Namespace, bool) {
	row, ok := v.source.resolution(r)
	if !ok || row.disposition != ResolutionResolved {
		return Namespace{}, false
	}
	_, ok = v.source.namespace(row.namespace)
	return row.namespace, ok
}

func (v Inputs) Count() int {
	if v.source == nil {
		return 0
	}
	return len(v.source.inputs)
}
func (v Inputs) At(i int) (InputRef, bool) {
	if v.source == nil || i < 0 || i >= len(v.source.inputs) {
		return InputRef{}, false
	}
	return InputRef{source: v.source, ordinal: uint32(i + 1)}, true
}
func (v Inputs) Source(input InputRef) (InputKind, keyspace.Term, Expression, keyspace.Term, keyspace.Term, int, bool) {
	row, ok := v.source.input(input)
	if !ok || row.kind == InputInvalid || row.source == 0 || row.target == 0 {
		return InputInvalid, 0, Expression{}, 0, 0, 0, false
	}
	shard, ok := v.source.Namespaces().ResolverShard(row.resolver)
	if !ok {
		return InputInvalid, 0, Expression{}, 0, 0, 0, false
	}
	e, ok := v.source.expression(shard, row.target)
	if !ok {
		return InputInvalid, 0, Expression{}, 0, 0, 0, false
	}
	return row.kind, row.source, e, row.operand, row.frontierBody, int(row.frontierCursor), true
}
func (v Inputs) ID(input InputRef) (keyspace.ContentID, bool) {
	row, ok := v.source.input(input)
	if !ok || !v.source.contentID.Available() || !row.owner.Available() ||
		(row.kind != InputTypeOf && row.kind != InputAnnotation) ||
		row.source == 0 || row.target == 0 || row.operand == 0 || row.frontierBody == 0 {
		return keyspace.ContentID{}, false
	}
	resolver, ok := v.source.Namespaces().ResolverContentID(row.resolver)
	if !ok || !resolver.Available() {
		return keyspace.ContentID{}, false
	}
	var words [7 * 8]byte
	binary.BigEndian.PutUint64(words[0:8], uint64(row.source))
	binary.BigEndian.PutUint64(words[8:16], uint64(row.operand))
	binary.BigEndian.PutUint64(words[16:24], uint64(row.frontierBody))
	binary.BigEndian.PutUint64(words[24:32], uint64(row.frontierCursor))
	binary.BigEndian.PutUint64(words[32:40], uint64(input.ordinal))
	binary.BigEndian.PutUint64(words[40:48], uint64(row.kind))
	binary.BigEndian.PutUint64(words[48:56], uint64(row.target))
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.program.link/static-input/v2"))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(v.source.contentID[:])
	_, _ = h.Write(row.owner[:])
	_, _ = h.Write(resolver[:])
	_, _ = h.Write(words[:])
	var id keyspace.ContentID
	copy(id[:], h.Sum(nil))
	return id, id.Available()
}

// SchemaContentCount/At expose static namespace content for Link dependency
// assembly without opening a duplicate namespace plane.
func (v Cold) SchemaContentCount() int {
	if !v.live() {
		return 0
	}
	return len(v.schema)
}
func (v Cold) SchemaContentAt(i int) (keyspace.ContentID, bool) {
	if !v.live() || i < 0 || i >= len(v.schema) {
		return keyspace.ContentID{}, false
	}
	id := v.schema[i]
	return id, id.Available()
}

// ContentID is the versioned identity of the complete Static constituent.
func (v Cold) ContentID() keyspace.ContentID {
	if !v.live() {
		return keyspace.ContentID{}
	}
	return v.contentID
}

func (v Cold) live() bool {
	return v.contentID.Available() && (v.fence == nil || !v.fence.consumed)
}

func writeStaticRows(w *canonical.Writer, c *Component) bool {
	if c == nil || w == nil || w.Count(uint64(len(c.namespaces))) != nil {
		return false
	}
	for _, n := range c.namespaces {
		index, ok := c.mounts.Index(n.shard)
		if !ok || !n.content.Available() || w.Uint(uint64(index+1)) != nil || w.Bytes(n.content[:]) != nil || w.Count(uint64(len(n.exports))) != nil {
			return false
		}
		for _, x := range n.exports {
			if x.root == 0 || x.typeRef == 0 || len(x.path) == 0 || w.Uint(uint64(x.root)) != nil || w.Count(uint64(len(x.path))) != nil {
				return false
			}
			for _, k := range x.path {
				if k == 0 || w.Uint(uint64(k)) != nil {
					return false
				}
			}
			if w.Uint(uint64(x.typeRef)) != nil {
				return false
			}
		}
	}
	if w.Count(uint64(len(c.resolutions))) != nil {
		return false
	}
	for _, r := range c.resolutions {
		index, ok := c.mounts.Index(r.shard)
		if !ok || r.importTerm == 0 || r.call == 0 || r.literal == 0 || w.Uint(uint64(index+1)) != nil || w.Uint(uint64(r.importTerm)) != nil || w.Uint(uint64(r.call)) != nil || w.Uint(uint64(r.literal)) != nil || w.Uint(uint64(r.alias)) != nil || w.Uint(uint64(r.disposition)) != nil || w.Uint(uint64(r.namespace.ordinal)) != nil {
			return false
		}
		if r.disposition == ResolutionResolved && (!c.validNamespace(r.namespace)) {
			return false
		}
		if r.disposition == ResolutionUnresolved && r.namespace != (Namespace{}) {
			return false
		}
		if r.disposition != ResolutionResolved && r.disposition != ResolutionUnresolved {
			return false
		}
	}
	if w.Count(uint64(len(c.inputs))) != nil {
		return false
	}
	for _, in := range c.inputs {
		if !in.owner.Available() || (in.kind != InputTypeOf && in.kind != InputAnnotation) || in.source == 0 || in.target == 0 || in.operand == 0 || in.frontierBody == 0 || w.Bytes(in.owner[:]) != nil || w.Uint(uint64(in.kind)) != nil || w.Uint(uint64(in.source)) != nil || w.Uint(uint64(in.target)) != nil || w.Uint(uint64(in.operand)) != nil || w.Uint(uint64(in.resolver.ordinal)) != nil || w.Uint(uint64(in.frontierBody)) != nil || w.Uint(uint64(in.frontierCursor)) != nil || !c.validResolver(in.resolver) {
			return false
		}
	}
	if w.Count(uint64(len(c.qualified))) != nil {
		return false
	}
	for _, q := range c.qualified {
		consumerIndex, consumerOK := c.mounts.Index(q.consumerShard)
		if !consumerOK || !q.owner.Available() || q.reference == 0 || !q.providerOwner.Available() || q.target == 0 || !c.validResolver(q.resolver) || w.Uint(uint64(consumerIndex+1)) != nil || w.Bytes(q.owner[:]) != nil || w.Uint(uint64(q.reference)) != nil || w.Bytes(q.providerOwner[:]) != nil || w.Uint(uint64(q.target)) != nil || w.Uint(uint64(q.resolver.ordinal)) != nil {
			return false
		}
	}
	return true
}
func (c *Component) validNamespace(n Namespace) bool { _, ok := c.namespace(n); return ok }
func (c *Component) validResolver(r Resolver) bool   { _, ok := c.Namespaces().Namespace(r); return ok }
