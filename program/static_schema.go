package program

import "math"

// typeAliasRow is a declaration host. gap is the number of
// executable Body roots preceding the non-executable declaration; it is kept
// private so no placement/context vocabulary leaks into Program's API.
type typeAliasRow struct {
	owner, target Term
	name          Key
	gap           uint32
	gapSet        bool
	params        termRange
	paramsSet     bool
	filled        bool
}

func (b *Builder) DeclareTypeAlias(span Span, owner Term, name string) Term {
	if !b.require(b.has(owner, tagBody) && name != "") {
		return 0
	}
	key := b.internExact(exactKey{kind: exactString, text: name})
	if key == 0 {
		return 0
	}
	b.typeAliases = append(b.typeAliases, typeAliasRow{owner: owner, name: key})
	term := b.mint(tagTypeAlias, span, b.familyIndex(len(b.typeAliases)))
	if term == 0 {
		b.typeAliases = b.typeAliases[:len(b.typeAliases)-1]
	}
	return term
}

// SetTypeAliasGap installs the alias's exact executable-body cursor once.
// Declaration identity is intentionally available before this placement so a
// lowerer can hoist names without predicting later statement roots.
func (b *Builder) SetTypeAliasGap(alias Term, gap int) bool {
	if !b.has(alias, tagTypeAlias) || gap < 0 || uint64(gap) > math.MaxUint32 {
		b.poison = true
		return false
	}
	r := &b.typeAliases[alias.index()-1]
	if r.gapSet || r.filled {
		b.poison = true
		return false
	}
	r.gap, r.gapSet = uint32(gap), true
	return true
}
func (b *Builder) FillTypeAlias(alias, target Term) bool {
	if !b.has(alias, tagTypeAlias) || !b.staticTypeNode(target) {
		b.poison = true
		return false
	}
	r := &b.typeAliases[alias.index()-1]
	if r.filled {
		b.poison = true
		return false
	}
	r.target, r.filled = target, true
	return true
}

// SetTypeAliasParams installs the alias's ordered source parameter range once.
// Parameters are predeclared first so constraints may use their exact host.
func (b *Builder) SetTypeAliasParams(alias Term, params []Term) bool {
	if !b.has(alias, tagTypeAlias) {
		b.poison = true
		return false
	}
	r := &b.typeAliases[alias.index()-1]
	if r.paramsSet || r.filled {
		b.poison = true
		return false
	}
	for _, param := range params {
		if !b.has(param, tagTypeParam) || b.typeParams[param.index()-1].owner != alias {
			b.poison = true
			return false
		}
	}
	range_, ok := b.appendPool(&b.typeParamTerms, params)
	if !ok {
		return false
	}
	r.params, r.paramsSet = range_, true
	return true
}
func (b *Builder) staticDeclarationScope(scope Term) (Term, uint32, bool) {
	if !b.has(scope, tagTypeAlias) {
		return 0, 0, false
	}
	r := b.typeAliases[scope.index()-1]
	return r.owner, r.gap, r.gapSet
}
func (b *Builder) validateStaticDeclarations() bool {
	for _, r := range b.typeAliases {
		if !r.filled || !r.gapSet || !b.has(r.owner, tagBody) || int(r.gap) > int(b.bodies[r.owner.index()-1].roots.end-b.bodies[r.owner.index()-1].roots.start) {
			return false
		}
	}
	return true
}
func (p *Program) TypeAlias(term Term) (owner, target Term, name Key, ok bool) {
	if !p.has(term, tagTypeAlias) {
		return 0, 0, 0, false
	}
	r := p.typeAliases[term.index()-1]
	return r.owner, r.target, r.name, true
}
func (p *Program) TypeAliasParamCount(term Term) (int, bool) {
	if !p.has(term, tagTypeAlias) {
		return 0, false
	}
	r := p.typeAliases[term.index()-1]
	return int(r.params.end - r.params.start), true
}
func (p *Program) TypeAliasParamAt(term Term, index int) (Term, bool) {
	if !p.has(term, tagTypeAlias) || index < 0 {
		return 0, false
	}
	r := p.typeAliases[term.index()-1]
	at := r.params.start + uint32(index)
	if at >= r.params.end {
		return 0, false
	}
	return p.typeParamTerms[at], true
}

// TypeKey interns a non-empty static type-name atom. It is intentionally not a
// Term: qualified-reference paths are metadata, never executable syntax.
func (b *Builder) TypeKey(name string) Key {
	if b == nil || !b.require(name != "") {
		return 0
	}
	return b.internExact(exactKey{kind: exactString, text: name})
}

func (b *Builder) DeclareTypeParam(span Span, owner Term, name string) Term {
	if !b.require(b.has(owner, tagTypeAlias) && name != "") {
		return 0
	}
	key := b.TypeKey(name)
	if key == 0 {
		return 0
	}
	b.typeParams = append(b.typeParams, typeParamRow{owner: owner, name: key})
	term := b.mint(tagTypeParam, span, b.familyIndex(len(b.typeParams)))
	if term == 0 {
		b.typeParams = b.typeParams[:len(b.typeParams)-1]
	}
	return term
}

// FillTypeParam records the unique constraint attachment. A zero constraint is
// an explicitly-filled unconstrained parameter, not a construction omission.
func (b *Builder) FillTypeParam(param, constraint Term) bool {
	if !b.has(param, tagTypeParam) || (constraint != 0 && !b.staticTypeNode(constraint)) {
		b.poison = true
		return false
	}
	r := &b.typeParams[param.index()-1]
	if r.constraintFilled {
		b.poison = true
		return false
	}
	r.constraint, r.constraintFilled = constraint, true
	return true
}

func (b *Builder) Primitive(span Span, kind PrimitiveKind) Term {
	if !b.require(kind.valid()) {
		return 0
	}
	b.primitiveTypes = append(b.primitiveTypes, primitiveTypeRow{kind: kind})
	term := b.mint(tagTypePrimitive, span, b.familyIndex(len(b.primitiveTypes)))
	if term == 0 {
		b.primitiveTypes = b.primitiveTypes[:len(b.primitiveTypes)-1]
	}
	return term
}

func (b *Builder) TypeBool(span Span, value bool) Term {
	return b.literal(span, literalTypeRow{kind: LiteralBool, exact: b.internExact(exactKey{kind: exactBool, bool: value})})
}
func (b *Builder) TypeInteger(span Span, value int64) Term {
	return b.literal(span, literalTypeRow{kind: LiteralInteger, exact: b.internExact(exactKey{kind: exactInteger, int: value})})
}
func (b *Builder) TypeFloat(span Span, value float64) Term {
	return b.literal(span, literalTypeRow{kind: LiteralFloat, bits: math.Float64bits(value)})
}
func (b *Builder) TypeString(span Span, value string) Term {
	return b.literal(span, literalTypeRow{kind: LiteralString, exact: b.internExact(exactKey{kind: exactString, text: value})})
}
func (b *Builder) literal(span Span, row literalTypeRow) Term {
	if b.poison {
		return 0
	}
	b.literalTypes = append(b.literalTypes, row)
	term := b.mint(tagTypeLiteral, span, b.familyIndex(len(b.literalTypes)))
	if term == 0 {
		b.literalTypes = b.literalTypes[:len(b.literalTypes)-1]
	}
	return term
}

func (b *Builder) Optional(span Span, inner Term) Term {
	if !b.require(b.staticTypeNode(inner)) {
		return 0
	}
	b.optionalTypes = append(b.optionalTypes, unaryTypeRow{inner: inner})
	term := b.mint(tagTypeOptional, span, b.familyIndex(len(b.optionalTypes)))
	if term == 0 {
		b.optionalTypes = b.optionalTypes[:len(b.optionalTypes)-1]
	}
	return term
}

func (b *Builder) Union(span Span, terms []Term) Term {
	return b.termsType(span, tagTypeUnion, terms, &b.unionTypes)
}

func (b *Builder) Intersection(span Span, terms []Term) Term {
	return b.termsType(span, tagTypeIntersection, terms, &b.intersectionTypes)
}

func (b *Builder) termsType(span Span, tag uint8, terms []Term, rows *[]termsTypeRow) Term {
	if !b.require(len(terms) >= 2) {
		return 0
	}
	for _, term := range terms {
		if !b.require(b.staticTypeNode(term)) {
			return 0
		}
	}
	range_, ok := b.appendPool(&b.staticTypeTerms, terms)
	if !ok {
		return 0
	}
	*rows = append(*rows, termsTypeRow{terms: range_})
	term := b.mint(tag, span, b.familyIndex(len(*rows)))
	if term == 0 {
		*rows = (*rows)[:len(*rows)-1]
		b.staticTypeTerms = b.staticTypeTerms[:range_.start]
	}
	return term
}

// TypeRef records a resolved declaration reference. Its source spelling may
// be bare or qualified; source qualification is independent of resolution.
func (b *Builder) TypeRef(span Span, pkg, name string, target Term) Term {
	if !b.require(name != "" && b.staticTypeTarget(target)) {
		return 0
	}
	pkgKey, nameKey := Key(0), b.TypeKey(name)
	if pkg != "" {
		pkgKey = b.TypeKey(pkg)
	}
	if nameKey == 0 || (pkg != "" && pkgKey == 0) {
		return 0
	}
	return b.appendTypeRef(span, typeRefRow{state: TypeRefDeclaration, target: target, pkg: pkgKey, name: nameKey})
}

// UnresolvedTypeRef preserves an unresolved bare or qualified source name.
func (b *Builder) UnresolvedTypeRef(span Span, pkg, name string) Term {
	if !b.require(name != "") {
		return 0
	}
	pkgKey, nameKey := Key(0), b.TypeKey(name)
	if pkg != "" {
		pkgKey = b.TypeKey(pkg)
	}
	if nameKey == 0 || (pkg != "" && pkgKey == 0) {
		return 0
	}
	return b.appendTypeRef(span, typeRefRow{state: TypeRefUnresolved, pkg: pkgKey, name: nameKey})
}

// QualifiedTypeRef records module-path resolution for a qualified source
// reference. The canonical path is ordered and never shares a declaration
// target.
func (b *Builder) QualifiedTypeRef(span Span, pkg, name string, path []Key) Term {
	return b.qualifiedTypeRef(span, pkg, name, path)
}

func (b *Builder) qualifiedTypeRef(span Span, pkg, name string, path []Key) Term {
	if !b.require(pkg != "" && name != "" && len(path) != 0) {
		return 0
	}
	for _, key := range path {
		if !b.require(b.staticTypeKey(key)) {
			return 0
		}
	}
	start, end, ok := boundedRange(len(b.typeRefPathKeys), len(path))
	if !ok {
		b.poison = true
		return 0
	}
	pkgKey, nameKey := b.TypeKey(pkg), b.TypeKey(name)
	if pkgKey == 0 || nameKey == 0 {
		return 0
	}
	b.typeRefPathKeys = append(b.typeRefPathKeys, path...)
	term := b.appendTypeRef(span, typeRefRow{state: TypeRefCanonicalPath, pkg: pkgKey, name: nameKey, path: keyRange{start: start, end: end}})
	if term == 0 {
		b.typeRefPathKeys = b.typeRefPathKeys[:start]
	}
	return term
}

func (b *Builder) appendTypeRef(span Span, row typeRefRow) Term {
	b.typeRefs = append(b.typeRefs, row)
	term := b.mint(tagTypeRef, span, b.familyIndex(len(b.typeRefs)))
	if term == 0 {
		b.typeRefs = b.typeRefs[:len(b.typeRefs)-1]
	}
	return term
}

func (b *Builder) Generic(span Span, base Term, args []Term) Term {
	if !b.require(b.has(base, tagTypeRef) && len(args) != 0) {
		return 0
	}
	for _, arg := range args {
		if !b.require(b.staticTypeNode(arg)) {
			return 0
		}
	}
	range_, ok := b.appendPool(&b.staticTypeTerms, args)
	if !ok {
		return 0
	}
	b.genericTypes = append(b.genericTypes, genericTypeRow{base: base, args: range_})
	term := b.mint(tagTypeGeneric, span, b.familyIndex(len(b.genericTypes)))
	if term == 0 {
		b.genericTypes = b.genericTypes[:len(b.genericTypes)-1]
		b.staticTypeTerms = b.staticTypeTerms[:range_.start]
	}
	return term
}

func (b *Builder) staticTypeNode(term Term) bool {
	if !b.valid(term) {
		return false
	}
	switch term.tag() {
	case tagTypePrimitive, tagTypeLiteral, tagTypeOptional, tagTypeUnion, tagTypeIntersection, tagTypeRef, tagTypeGeneric, tagTypeOf:
		return true
	default:
		return false
	}
}

func (b *Builder) staticTypeTarget(term Term) bool {
	return b.has(term, tagTypeAlias) || b.has(term, tagTypeParam)
}

func (b *Builder) staticTypeKey(key Key) bool {
	return key != 0 && uint64(key) <= uint64(len(b.exactKeys)) && b.exactKeys[key-1].kind == exactString && b.exactKeys[key-1].text != ""
}

// validateStaticCore proves the static type forest without materializing an
// attachment Term or a duplicate host-to-root table. Alias targets and type
// parameter constraints are the only non-Term roots; every concrete type node
// has exactly one parent in that forest.
func (b *Builder) validateStaticCore() bool {
	var parents [tagCount][]Term
	paramSeen := make([]bool, len(b.typeParams))
	parents[tagTypePrimitive] = make([]Term, len(b.primitiveTypes))
	parents[tagTypeLiteral] = make([]Term, len(b.literalTypes))
	parents[tagTypeOptional] = make([]Term, len(b.optionalTypes))
	parents[tagTypeUnion] = make([]Term, len(b.unionTypes))
	parents[tagTypeIntersection] = make([]Term, len(b.intersectionTypes))
	parents[tagTypeRef] = make([]Term, len(b.typeRefs))
	parents[tagTypeGeneric] = make([]Term, len(b.genericTypes))
	parents[tagTypeOf] = make([]Term, len(b.typeOfs))
	attach := func(parent, child Term) bool {
		if !b.staticTypeNode(child) || !b.valid(parent) {
			return false
		}
		slot := &parents[child.tag()][child.index()-1]
		if *slot != 0 {
			return false
		}
		*slot = parent
		return true
	}
	validTerms := func(r termRange, minimum int) bool {
		return r.start <= r.end && uint64(r.end) <= uint64(len(b.staticTypeTerms)) && int(r.end-r.start) >= minimum
	}
	validParams := func(r termRange) bool {
		return r.start <= r.end && uint64(r.end) <= uint64(len(b.typeParamTerms))
	}
	validPath := func(r keyRange) bool {
		if r.start >= r.end || uint64(r.end) > uint64(len(b.typeRefPathKeys)) {
			return false
		}
		for _, key := range b.typeRefPathKeys[r.start:r.end] {
			if !b.staticTypeKey(key) {
				return false
			}
		}
		return true
	}
	for i, row := range b.typeAliases {
		alias := makeTerm(tagTypeAlias, uint32(i+1))
		if !row.filled || !row.gapSet || !row.paramsSet || !b.has(row.owner, tagBody) || !b.staticTypeKey(row.name) || !validParams(row.params) || !b.staticTypeNode(row.target) || !attach(alias, row.target) {
			return false
		}
		for _, param := range b.typeParamTerms[row.params.start:row.params.end] {
			if !b.has(param, tagTypeParam) || b.typeParams[param.index()-1].owner != alias || paramSeen[param.index()-1] {
				return false
			}
			paramSeen[param.index()-1] = true
		}
	}
	for i, row := range b.typeParams {
		param := makeTerm(tagTypeParam, uint32(i+1))
		if !paramSeen[i] || !b.has(row.owner, tagTypeAlias) || !b.staticTypeKey(row.name) || !row.constraintFilled {
			return false
		}
		if row.constraint != 0 && (!b.staticTypeNode(row.constraint) || !attach(param, row.constraint)) {
			return false
		}
	}
	for _, row := range b.primitiveTypes {
		if !row.kind.valid() {
			return false
		}
	}
	for _, row := range b.literalTypes {
		switch row.kind {
		case LiteralBool:
			if row.exact == 0 || uint64(row.exact) > uint64(len(b.exactKeys)) || b.exactKeys[row.exact-1].kind != exactBool || row.bits != 0 {
				return false
			}
		case LiteralInteger:
			if row.exact == 0 || uint64(row.exact) > uint64(len(b.exactKeys)) || b.exactKeys[row.exact-1].kind != exactInteger || row.bits != 0 {
				return false
			}
		case LiteralFloat:
			if row.exact != 0 {
				return false
			}
		case LiteralString:
			if row.exact == 0 || uint64(row.exact) > uint64(len(b.exactKeys)) || b.exactKeys[row.exact-1].kind != exactString || row.bits != 0 {
				return false
			}
		default:
			return false
		}
	}
	for i, row := range b.optionalTypes {
		if !attach(makeTerm(tagTypeOptional, uint32(i+1)), row.inner) {
			return false
		}
	}
	for i, row := range b.unionTypes {
		if !validTerms(row.terms, 2) {
			return false
		}
		for _, child := range b.staticTypeTerms[row.terms.start:row.terms.end] {
			if !attach(makeTerm(tagTypeUnion, uint32(i+1)), child) {
				return false
			}
		}
	}
	for i, row := range b.intersectionTypes {
		if !validTerms(row.terms, 2) {
			return false
		}
		for _, child := range b.staticTypeTerms[row.terms.start:row.terms.end] {
			if !attach(makeTerm(tagTypeIntersection, uint32(i+1)), child) {
				return false
			}
		}
	}
	for _, row := range b.typeRefs {
		switch row.state {
		case TypeRefUnresolved:
			if !b.staticTypeKey(row.name) || row.target != 0 || row.path.start != 0 || row.path.end != 0 || (row.pkg != 0 && !b.staticTypeKey(row.pkg)) {
				return false
			}
		case TypeRefDeclaration:
			if !b.staticTypeKey(row.name) || !b.staticTypeTarget(row.target) || row.path.start != 0 || row.path.end != 0 || (row.pkg != 0 && !b.staticTypeKey(row.pkg)) {
				return false
			}
		case TypeRefCanonicalPath:
			if row.target != 0 || !b.staticTypeKey(row.pkg) || !b.staticTypeKey(row.name) || !validPath(row.path) {
				return false
			}
		default:
			return false
		}
	}
	for i, row := range b.genericTypes {
		if !b.has(row.base, tagTypeRef) || !validTerms(row.args, 1) || !attach(makeTerm(tagTypeGeneric, uint32(i+1)), row.base) {
			return false
		}
		for _, child := range b.staticTypeTerms[row.args.start:row.args.end] {
			if !attach(makeTerm(tagTypeGeneric, uint32(i+1)), child) {
				return false
			}
		}
	}
	for tag, slots := range parents {
		if uint8(tag) == tagTypeOf {
			continue
		}
		for _, parent := range slots {
			if parent == 0 {
				return false
			}
		}
	}
	for i, row := range b.typeOfs {
		parent := parents[tagTypeOf][i]
		if parent == 0 {
			// The existing Cell-host TypeOf relation is a direct attachment.
			// Declaration hosts are never an implicit escape hatch: their
			// TypeOf must occur in the alias target or parameter constraint.
			if !b.has(row.scope, tagCell) || b.staticScopeBody(row.scope) == 0 {
				return false
			}
			continue
		}
		for {
			switch parent.tag() {
			case tagTypeAlias, tagTypeParam:
				if parent != row.scope {
					return false
				}
				goto typeOfAttached
			}
			if !b.staticTypeNode(parent) {
				return false
			}
			parent = parents[parent.tag()][parent.index()-1]
			if parent == 0 {
				return false
			}
		}
	typeOfAttached:
	}
	return true
}

func (p *Program) TypeParam(term Term) (owner Term, name Key, constraint Term, ok bool) {
	if !p.has(term, tagTypeParam) {
		return 0, 0, 0, false
	}
	r := p.typeParams[term.index()-1]
	return r.owner, r.name, r.constraint, true
}

func (p *Program) Primitive(term Term) (kind PrimitiveKind, ok bool) {
	if !p.has(term, tagTypePrimitive) {
		return 0, false
	}
	return p.primitiveTypes[term.index()-1].kind, true
}

func (p *Program) Literal(term Term) (value LiteralValue, ok bool) {
	if !p.has(term, tagTypeLiteral) {
		return LiteralValue{}, false
	}
	r := p.literalTypes[term.index()-1]
	switch r.kind {
	case LiteralBool:
		return LiteralValue{Kind: r.kind, Bool: p.exactKeys[r.exact-1].bool}, true
	case LiteralInteger:
		return LiteralValue{Kind: r.kind, Integer: p.exactKeys[r.exact-1].int}, true
	case LiteralFloat:
		return LiteralValue{Kind: r.kind, FloatBits: r.bits}, true
	case LiteralString:
		return LiteralValue{Kind: r.kind, String: p.exactKeys[r.exact-1].text}, true
	default:
		return LiteralValue{}, false
	}
}

func (p *Program) Optional(term Term) (inner Term, ok bool) {
	if !p.has(term, tagTypeOptional) {
		return 0, false
	}
	return p.optionalTypes[term.index()-1].inner, true
}

func (p *Program) UnionLen(term Term) (int, bool) {
	return p.termsTypeCount(term, tagTypeUnion, p.unionTypes)
}
func (p *Program) IntersectionLen(term Term) (int, bool) {
	return p.termsTypeCount(term, tagTypeIntersection, p.intersectionTypes)
}
func (p *Program) termsTypeCount(term Term, tag uint8, rows []termsTypeRow) (int, bool) {
	if !p.has(term, tag) {
		return 0, false
	}
	r := rows[term.index()-1].terms
	return int(r.end - r.start), true
}
func (p *Program) UnionMember(term Term, index int) (Term, bool) {
	return p.termsTypeAt(term, tagTypeUnion, p.unionTypes, index)
}
func (p *Program) IntersectionMember(term Term, index int) (Term, bool) {
	return p.termsTypeAt(term, tagTypeIntersection, p.intersectionTypes, index)
}
func (p *Program) termsTypeAt(term Term, tag uint8, rows []termsTypeRow, index int) (Term, bool) {
	if !p.has(term, tag) || index < 0 {
		return 0, false
	}
	r := rows[term.index()-1].terms
	at := r.start + uint32(index)
	if at >= r.end {
		return 0, false
	}
	return p.staticTypeTerms[at], true
}

func (p *Program) TypeRef(term Term) (state TypeRefState, target Term, pkg, name Key, ok bool) {
	if !p.has(term, tagTypeRef) {
		return 0, 0, 0, 0, false
	}
	r := p.typeRefs[term.index()-1]
	return r.state, r.target, r.pkg, r.name, true
}
func (p *Program) TypeRefPathLen(term Term) (int, bool) {
	if !p.has(term, tagTypeRef) {
		return 0, false
	}
	r := p.typeRefs[term.index()-1]
	return int(r.path.end - r.path.start), true
}
func (p *Program) TypeRefPathAt(term Term, index int) (Key, bool) {
	if !p.has(term, tagTypeRef) || index < 0 {
		return 0, false
	}
	r := p.typeRefs[term.index()-1]
	at := r.path.start + uint32(index)
	if at >= r.path.end {
		return 0, false
	}
	return p.typeRefPathKeys[at], true
}

func (p *Program) Generic(term Term) (base Term, ok bool) {
	if !p.has(term, tagTypeGeneric) {
		return 0, false
	}
	r := p.genericTypes[term.index()-1]
	return r.base, true
}
func (p *Program) GenericArgLen(term Term) (int, bool) {
	if !p.has(term, tagTypeGeneric) {
		return 0, false
	}
	r := p.genericTypes[term.index()-1]
	return int(r.args.end - r.args.start), true
}
func (p *Program) GenericArgAt(term Term, index int) (Term, bool) {
	if !p.has(term, tagTypeGeneric) || index < 0 {
		return 0, false
	}
	r := p.genericTypes[term.index()-1]
	at := r.args.start + uint32(index)
	if at >= r.args.end {
		return 0, false
	}
	return p.staticTypeTerms[at], true
}

func (p *Program) TypeAliasCount() int {
	if p == nil {
		return 0
	}
	return len(p.typeAliases)
}
func (p *Program) TypeAliasAt(index int) (Term, bool) {
	return familyTerm(tagTypeAlias, p.TypeAliasCount(), index)
}
func (p *Program) TypeParamCount() int {
	if p == nil {
		return 0
	}
	return len(p.typeParams)
}
func (p *Program) TypeParamAt(index int) (Term, bool) {
	return familyTerm(tagTypeParam, p.TypeParamCount(), index)
}
func (p *Program) PrimitiveCount() int {
	if p == nil {
		return 0
	}
	return len(p.primitiveTypes)
}
func (p *Program) PrimitiveAt(index int) (Term, bool) {
	return familyTerm(tagTypePrimitive, p.PrimitiveCount(), index)
}
func (p *Program) LiteralCount() int {
	if p == nil {
		return 0
	}
	return len(p.literalTypes)
}
func (p *Program) LiteralAt(index int) (Term, bool) {
	return familyTerm(tagTypeLiteral, p.LiteralCount(), index)
}
func (p *Program) OptionalCount() int {
	if p == nil {
		return 0
	}
	return len(p.optionalTypes)
}
func (p *Program) OptionalAt(index int) (Term, bool) {
	return familyTerm(tagTypeOptional, p.OptionalCount(), index)
}
func (p *Program) UnionCount() int {
	if p == nil {
		return 0
	}
	return len(p.unionTypes)
}
func (p *Program) UnionAt(index int) (Term, bool) {
	return familyTerm(tagTypeUnion, p.UnionCount(), index)
}
func (p *Program) IntersectionCount() int {
	if p == nil {
		return 0
	}
	return len(p.intersectionTypes)
}
func (p *Program) IntersectionAt(index int) (Term, bool) {
	return familyTerm(tagTypeIntersection, p.IntersectionCount(), index)
}
func (p *Program) TypeRefCount() int {
	if p == nil {
		return 0
	}
	return len(p.typeRefs)
}
func (p *Program) TypeRefAt(index int) (Term, bool) {
	return familyTerm(tagTypeRef, p.TypeRefCount(), index)
}
func (p *Program) GenericCount() int {
	if p == nil {
		return 0
	}
	return len(p.genericTypes)
}
func (p *Program) GenericAt(index int) (Term, bool) {
	return familyTerm(tagTypeGeneric, p.GenericCount(), index)
}
