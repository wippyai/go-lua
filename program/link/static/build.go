package static

import (
	"crypto/sha256"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programstatic "github.com/wippyai/go-lua/program/static"
)

const maxHandle = uint64(^uint32(0))

func fits(count int) bool { return count >= 0 && uint64(count) <= maxHandle }

// Build seals the complete static authority from one finalized Project.
// Literal module resolution uses only its canonical mounts, never Link's
// runtime radix tables.
func Build(input Input) (*Draft, error) {
	if input.Project == nil {
		return nil, errors.New("link/static: invalid build input")
	}
	project := input.Project.Cold()
	mounts := input.Project.Mounts()
	targetID, projectID := project.TargetID(), project.ContentID()
	if !targetID.Available() || !projectID.Available() || !fits(mounts.Count()) {
		return nil, errors.New("link/static: invalid build input")
	}
	c := &Component{
		mounts: mounts,
		byCall: make(map[shardTerm]Resolution), byAlias: make(map[shardTerm]Namespace),
		byQualified: make(map[shardTerm]uint32),
	}
	names := make(map[string]linkproject.Shard, mounts.Count())
	for i := 0; i < mounts.Count(); i++ {
		shard, shardOK := mounts.At(i)
		name, nameOK := mounts.Name(shard)
		p, programOK := mounts.Program(shard)
		if !shardOK || !nameOK || name == "" || !programOK || p == nil || !p.ContentID().Available() {
			return nil, errors.New("link/static: malformed module")
		}
		if _, dup := names[name]; dup {
			return nil, errors.New("link/static: duplicate module name")
		}
		names[name] = shard
		owner := p.ContentID()
		exports, err := staticExports(p)
		if err != nil {
			return nil, err
		}
		content := namespaceContent(targetID, owner, exports)
		if !content.Available() {
			return nil, errors.New("link/static: unavailable namespace content")
		}
		c.namespaces = append(c.namespaces, namespaceRow{shard: shard, content: content, exports: exports})
	}
	for index := 0; index < c.mounts.Count(); index++ {
		shard, ok := c.mounts.At(index)
		if !ok {
			return nil, errors.New("link/static: unavailable mounted Project Shard")
		}
		p, ok := c.program(shard)
		if !ok {
			return nil, errors.New("link/static: unavailable mounted Program")
		}
		module := p.Module()
		for at := 0; at < module.Count(); at++ {
			item, ok := module.ImportAt(at)
			if !ok {
				return nil, errors.New("link/static: malformed Program Import family")
			}
			row, ok := module.Import(item.Term)
			if !ok || row.Call == 0 {
				return nil, errors.New("link/static: malformed Program Import row")
			}
			if row.Request == 0 {
				continue
			}
			name, ok := programString(p, row.Request)
			if !ok {
				return nil, errors.New("link/static: literal Import lacks Program String")
			}
			resolution := resolutionRow{shard: shard, importTerm: item.Term, call: row.Call, literal: row.Request, alias: row.Alias, disposition: ResolutionUnresolved}
			if target, targetOK := names[name]; targetOK {
				resolution.disposition = ResolutionResolved
				index, indexOK := c.mounts.Index(target)
				if !indexOK {
					return nil, errors.New("link/static: resolved namespace Project Shard unavailable")
				}
				resolution.namespace = Namespace{source: c, ordinal: uint32(index + 1)}
			}
			c.resolutions = append(c.resolutions, resolution)
			if !fits(len(c.resolutions)) {
				return nil, errors.New("link/static: resolution overflow")
			}
			h := Resolution{source: c, ordinal: uint32(len(c.resolutions))}
			callKey := shardTerm{shard, row.Call}
			if prior, dup := c.byCall[callKey]; dup && prior != h {
				return nil, errors.New("link/static: conflicting call resolution")
			}
			c.byCall[callKey] = h
			if resolution.disposition == ResolutionResolved && row.Alias != 0 {
				key := shardTerm{shard, row.Alias}
				if prior, dup := c.byAlias[key]; dup && prior != resolution.namespace {
					return nil, errors.New("link/static: conflicting alias")
				}
				c.byAlias[key] = resolution.namespace
			}
		}
	}
	if err := c.buildInputs(); err != nil {
		return nil, err
	}
	if err := c.buildQualified(); err != nil {
		return nil, err
	}
	c.expressionEnds = make([]uint32, c.mounts.Count()+1)
	for i := 0; i < c.mounts.Count(); i++ {
		shard, ok := c.mounts.At(i)
		if !ok {
			return nil, errors.New("link/static: unavailable mounted Project Shard")
		}
		p, ok := c.program(shard)
		if !ok {
			return nil, errors.New("link/static: unavailable mounted Program")
		}
		count := p.Static().StaticTypes().Count()
		if !fits(int(c.expressionEnds[i]) + count) {
			return nil, errors.New("link/static: expression overflow")
		}
		c.expressionEnds[i+1] = c.expressionEnds[i] + uint32(count)
	}
	c.contentID = staticContentID(targetID, projectID, c)
	if !c.contentID.Available() {
		return nil, errors.New("link/static: unavailable content identity")
	}
	var receiptOK bool
	if c.semanticReceipt, receiptOK = buildStaticSemanticSourceReceipt(c); !receiptOK {
		return nil, errors.New("link/static: unavailable semantic-source receipt")
	}
	return &Draft{state: &draftState{component: c, fence: &draftFence{}}}, nil
}

// Finalize consumes a construction-only Draft without mutating its sealed
// semantic authority.
func (d *Draft) Finalize() (*Component, error) {
	if d == nil || d.state == nil {
		return nil, errors.New("link/static: invalid finalization")
	}
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	if d.state.consumed || d.state.component == nil || !d.state.component.contentID.Available() {
		return nil, errors.New("link/static: invalid finalization")
	}
	d.state.consumed = true
	if d.state.fence != nil {
		d.state.fence.consumed = true
	}
	component := d.state.component
	d.state.component = nil
	return component, nil
}

func (c *Component) buildInputs() error {
	for index := 0; index < c.mounts.Count(); index++ {
		shard, ok := c.mounts.At(index)
		if !ok {
			return errors.New("link/static: unavailable mounted Project Shard")
		}
		p, ok := c.program(shard)
		if !ok {
			return errors.New("link/static: unavailable mounted Program")
		}
		resolver, ok := c.Namespaces().ResolverForShard(shard)
		if !ok {
			return errors.New("link/static: input resolver unavailable")
		}
		appendInput := func(kind InputKind, source, target, operand keyspace.Term) error {
			body, cursor, ok := p.Source().Index().Frontier(source)
			if !ok || body == 0 || cursor < 0 || uint64(cursor) > maxHandle {
				return errors.New("link/static: source frontier unavailable")
			}
			c.inputs = append(c.inputs, inputRow{owner: p.ContentID(), kind: kind, source: source, target: target, operand: operand, resolver: resolver, frontierBody: body, frontierCursor: uint32(cursor)})
			if !fits(len(c.inputs)) {
				return errors.New("link/static: input overflow")
			}
			return nil
		}
		staticView := p.Static()
		typeOfs := staticView.Operators().TypeOfs()
		for i := 0; i < typeOfs.Count(); i++ {
			source, ok := typeOfs.At(i)
			if !ok {
				return errors.New("link/static: malformed TypeOf family")
			}
			_, operand, ok := typeOfs.Get(source)
			if !ok || source == 0 || operand == 0 {
				return errors.New("link/static: malformed TypeOf")
			}
			if err := appendInput(InputTypeOf, source, source, operand); err != nil {
				return err
			}
		}
		annotations := staticView.Operands().Annotations()
		values := p.Flow().Authored().Values()
		for i := 0; i < annotations.Count(); i++ {
			source, ok := annotations.At(i)
			if !ok || source == 0 {
				return errors.New("link/static: malformed Annotation family")
			}
			annotation, ok := annotations.Get(source)
			if _, targetOK := staticView.StaticTypes().Ref(annotation.Target); !ok || !targetOK || annotation.Target == 0 || annotation.Values == 0 {
				return errors.New("link/static: malformed Annotation")
			}
			count, ok := values.Len(annotation.Values)
			if !ok {
				return errors.New("link/static: malformed Annotation Values")
			}
			for arg := 0; arg < count; arg++ {
				operand, ok := values.Member(annotation.Values, arg)
				if !ok || operand == 0 {
					return errors.New("link/static: malformed Annotation argument")
				}
				if err := appendInput(InputAnnotation, source, annotation.Target, operand); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *Component) buildQualified() error {
	for index := 0; index < c.mounts.Count(); index++ {
		shard, ok := c.mounts.At(index)
		if !ok {
			return errors.New("link/static: unavailable mounted Project Shard")
		}
		p, ok := c.program(shard)
		if !ok {
			return errors.New("link/static: unavailable mounted Program")
		}
		owner := p.ContentID()
		staticView := p.Static()
		references := staticView.References()
		for i := 0; i < references.Count(); i++ {
			reference, ok := references.At(i)
			if !ok {
				return errors.New("link/static: malformed TypeRef family")
			}
			resolution, _, root, ok := references.Get(reference)
			if !ok {
				return errors.New("link/static: malformed TypeRef")
			}
			if root == 0 || (resolution != programstatic.TypeRefUnresolved && resolution != programstatic.TypeRefCanonicalPath) {
				continue
			}
			namespace, found := c.byAlias[shardTerm{shard, root}]
			if !found {
				continue
			}
			row, found, err := c.resolveQualified(p, shard, owner, reference, namespace)
			if err != nil {
				return err
			}
			if !found {
				continue
			}
			key := shardTerm{shard, reference}
			if _, dup := c.byQualified[key]; dup {
				return errors.New("link/static: duplicate qualified type")
			}
			c.qualified = append(c.qualified, row)
			if !fits(len(c.qualified)) {
				return errors.New("link/static: qualified type overflow")
			}
			c.byQualified[key] = uint32(len(c.qualified))
		}
	}
	return nil
}

func (c *Component) resolveQualified(p *program.Program, consumerShard linkproject.Shard, owner keyspace.ContentID, reference keyspace.Term, namespace Namespace) (qualifiedRow, bool, error) {
	path, ok := typeReferencePath(p, reference)
	if !ok {
		return qualifiedRow{}, false, errors.New("link/static: malformed qualified type path")
	}
	providerShard, ok := c.Namespaces().Shard(namespace)
	if !ok {
		return qualifiedRow{}, false, errors.New("link/static: qualified namespace unavailable")
	}
	provider, ok := c.program(providerShard)
	if !ok {
		return qualifiedRow{}, false, errors.New("link/static: qualified provider unavailable")
	}
	resolver, ok := c.Namespaces().Resolver(namespace)
	if !ok {
		return qualifiedRow{}, false, errors.New("link/static: qualified resolver unavailable")
	}
	ns, ok := c.namespace(namespace)
	if !ok {
		return qualifiedRow{}, false, errors.New("link/static: malformed namespace")
	}
	for _, export := range ns.exports {
		if len(export.path) != len(path) {
			continue
		}
		match := true
		for i, key := range export.path {
			value, valid := provider.Source().Keys().Exact(key)
			if !valid || value != path[i] {
				match = false
				break
			}
		}
		if match {
			return qualifiedRow{consumerShard: consumerShard, owner: owner, reference: reference, providerOwner: provider.ContentID(), target: export.typeRef, resolver: resolver}, true, nil
		}
	}
	return qualifiedRow{}, false, nil
}

func typeReferencePath(p *program.Program, reference keyspace.Term) ([]keyspace.LiteralValue, bool) {
	if p == nil {
		return nil, false
	}
	references := p.Static().References()
	length, ok := references.CanonicalCount(reference)
	at := references.CanonicalAt
	offset := 0
	if !ok || length == 0 {
		length, ok = references.SourceCount(reference)
		if !ok || length < 2 {
			return nil, false
		}
		at, offset, length = references.SourceAt, 1, length-1
	}
	path := make([]keyspace.LiteralValue, length)
	keys := p.Source().Keys()
	for i := range path {
		key, valid := at(reference, i+offset)
		if !valid {
			return nil, false
		}
		path[i], valid = keys.Exact(key)
		if !valid {
			return nil, false
		}
	}
	return path, true
}

func programString(p *program.Program, term keyspace.Term) (string, bool) {
	if p == nil || keyspace.TermFamily(term) != keyspace.FamilyString {
		return "", false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return "", false
	}
	strings := p.Source().Literals().Strings()
	got, _, value, ok := strings.At(int(ordinal) - 1)
	return value, ok && got == term
}

func staticExports(p *program.Program) ([]exportRow, error) {
	if p == nil {
		return nil, errors.New("link/static: missing Program export surface")
	}
	roots := make(map[keyspace.Term]struct{})
	entry := p.Module().Entry()
	returns := p.Flow().Authored().Control().Returns()
	values := p.Flow().Authored().Values()
	for i := 0; i < entry.ReturnCount(); i++ {
		returned, ok := entry.ReturnAt(i)
		if !ok {
			return nil, errors.New("link/static: malformed EntryReturn")
		}
		_, returnedValues, ok := returns.Get(returned)
		if !ok {
			return nil, errors.New("link/static: malformed Return")
		}
		count, ok := values.Len(returnedValues)
		if !ok {
			return nil, errors.New("link/static: malformed Return Values")
		}
		for ordinal := 0; ordinal < count; ordinal++ {
			if root, found := entry.RootCell(returned, ordinal); found {
				roots[root] = struct{}{}
			}
		}
	}
	publications := p.Static().Publications()
	if len(roots) == 0 || publications.Count() == 0 {
		return nil, nil
	}
	exports := make([]exportRow, 0)
	directBindings := p.Flow().DirectBindings()
	for i := 0; i < publications.Count(); i++ {
		publication, ok := publications.At(i)
		if !ok {
			return nil, errors.New("link/static: malformed TypePublication family")
		}
		_, _, ref, ok := publications.Get(publication)
		if !ok || ref == 0 {
			return nil, errors.New("link/static: malformed TypePublication")
		}
		root, _, depth, ok := directBindings.Publication(publication)
		pathCursor, cursorOK := directBindings.PublicationPath(publication)
		if !ok || !cursorOK || root == 0 || depth <= 0 {
			return nil, errors.New("link/static: malformed TypePublication path")
		}
		path := make([]keyspace.Key, 0, depth)
		for segment := 0; segment < depth; segment++ {
			key, next, segmentOK := pathCursor.Segment()
			if !segmentOK {
				return nil, errors.New("link/static: malformed TypePublication path")
			}
			path = append(path, key)
			pathCursor = next
		}
		for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
			path[left], path[right] = path[right], path[left]
		}
		if _, yes := roots[root]; yes {
			exports = append(exports, exportRow{root: root, path: path, typeRef: ref})
		}
	}
	sort.Slice(exports, func(i, j int) bool { return compareExport(exports[i], exports[j]) < 0 })
	for i := 1; i < len(exports); i++ {
		if comparePath(exports[i-1].path, exports[i].path) == 0 {
			return nil, errors.New("link/static: duplicate export path")
		}
	}
	return exports, nil
}
func compareExport(a, b exportRow) int {
	if n := comparePath(a.path, b.path); n != 0 {
		return n
	}
	if a.root < b.root {
		return -1
	}
	if a.root > b.root {
		return 1
	}
	if a.typeRef < b.typeRef {
		return -1
	}
	if a.typeRef > b.typeRef {
		return 1
	}
	return 0
}
func comparePath(a, b []keyspace.Key) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}
func namespaceContent(targetID, programID keyspace.ContentID, exports []exportRow) (id keyspace.ContentID) {
	if !targetID.Available() || !programID.Available() {
		return id
	}
	h := sha256.New()
	var w canonical.Writer
	if w.Reset(h, "program/link/static-namespace", 1) != nil || w.Record(1) != nil || w.Bytes(targetID[:]) != nil || w.Bytes(programID[:]) != nil || w.Count(uint64(len(exports))) != nil {
		return id
	}
	for _, x := range exports {
		if x.root == 0 || x.typeRef == 0 || len(x.path) == 0 || w.Uint(uint64(x.root)) != nil || w.Count(uint64(len(x.path))) != nil {
			return id
		}
		for _, k := range x.path {
			if k == 0 || w.Uint(uint64(k)) != nil {
				return id
			}
		}
		if w.Uint(uint64(x.typeRef)) != nil {
			return id
		}
	}
	if w.Finish() != nil {
		return id
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}
