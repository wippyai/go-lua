package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// pointValueEnvBuilder materializes the finite abstract environment seen by
// constraint narrowing at one CFG point. It owns only solver scratch reuse and
// canonical path resolution; lattice operations stay in constraint/domain code.
type pointValueEnvBuilder struct {
	s           *Solution
	point       cfg.Point
	targetPath  constraint.Path
	baseType    typ.Type
	constraints []constraint.Constraint

	visibleVersions    map[cfg.SymbolID]cfg.Version
	queryVisibleLookup bool
	versionIDs         map[cfg.SymbolID]int
	missingVersions    map[cfg.SymbolID]struct{}
	hasMissingVersions bool

	values     map[constraint.PathKey]typ.Type
	unresolved map[constraint.PathKey]struct{}
	mode       pointValueEnvMode
}

type pointValueEnvMode uint8

const (
	pointValueEnvSolved pointValueEnvMode = iota
	pointValueEnvConditionProof
)

func newPointValueEnvBuilder(
	s *Solution,
	p cfg.Point,
	targetPath constraint.Path,
	baseType typ.Type,
	constraints []constraint.Constraint,
	mode pointValueEnvMode,
) *pointValueEnvBuilder {
	b := &pointValueEnvBuilder{
		s:           s,
		point:       p,
		targetPath:  targetPath,
		baseType:    baseType,
		constraints: constraints,
		mode:        mode,
	}
	b.prepareVisibleVersions()
	b.prepareScratch()
	return b
}

func (b *pointValueEnvBuilder) build() map[constraint.PathKey]typ.Type {
	b.seedTarget()
	b.collectConstraintPaths()
	return b.values
}

func (b *pointValueEnvBuilder) prepareVisibleVersions() {
	visibleCount := len(b.s.declaredSyms)
	if b.s.inputs != nil && b.s.inputs.Graph != nil {
		b.visibleVersions = b.s.inputs.Graph.AllVisibleVersions(b.point)
		visibleCount = len(b.visibleVersions)
	}
	b.queryVisibleLookup = b.visibleVersions != nil
	b.prepareValueMap(visibleCount)
	if !b.queryVisibleLookup {
		b.prepareVersionLookup(visibleCount)
	}
}

func (b *pointValueEnvBuilder) prepareScratch() {
	unresolved := b.s.scratchUnresolvedPaths
	if unresolved == nil {
		unresolved = make(map[constraint.PathKey]struct{}, estimateUnresolvedPathCapacity(len(b.constraints)))
		b.s.scratchUnresolvedPaths = unresolved
	}
	clear(unresolved)
	b.unresolved = unresolved
}

func (b *pointValueEnvBuilder) prepareValueMap(visibleCount int) {
	values := b.s.scratchValueMap
	if values == nil {
		values = make(map[constraint.PathKey]typ.Type, estimatePointValueMapCapacity(visibleCount, len(b.constraints)))
		b.s.scratchValueMap = values
	}
	clear(values)
	b.values = values
}

func (b *pointValueEnvBuilder) prepareVersionLookup(visibleCount int) {
	versionIDs := b.s.scratchVersionIDs
	if versionIDs == nil {
		versionIDs = make(map[cfg.SymbolID]int, estimateVersionCacheCapacity(visibleCount))
		b.s.scratchVersionIDs = versionIDs
	}
	clear(versionIDs)
	b.versionIDs = versionIDs

	missingVersions := b.s.scratchMissingVersions
	if missingVersions == nil {
		missingVersions = make(map[cfg.SymbolID]struct{}, 8)
		b.s.scratchMissingVersions = missingVersions
	}
	clear(missingVersions)
	b.missingVersions = missingVersions
}

func (b *pointValueEnvBuilder) seedTarget() {
	targetKey := b.keyAtPoint(b.targetPath)
	if targetKey != "" {
		b.values[targetKey] = b.baseType
	}
}

func (b *pointValueEnvBuilder) collectConstraintPaths() {
	for _, c := range b.constraints {
		constraint.VisitPaths(c, func(path constraint.Path) bool {
			b.recordConstraintPath(path)
			return false
		})
	}
}

func (b *pointValueEnvBuilder) recordConstraintPath(path constraint.Path) {
	if path.IsEmpty() || path.Symbol == 0 {
		return
	}
	path = normalizeConstraintPathForQuery(path)
	canonicalKey := b.keyAtPoint(path)
	if canonicalKey == "" {
		return
	}
	if _, exists := b.values[canonicalKey]; exists {
		return
	}
	if _, knownUnresolved := b.unresolved[canonicalKey]; knownUnresolved {
		return
	}
	if b.mode == pointValueEnvConditionProof {
		b.recordConditionProofPath(canonicalKey, path)
		return
	}
	if t := b.s.projectedValueAtPoint(b.point, string(canonicalKey)); t != nil {
		b.values[canonicalKey] = t
		return
	}
	if b.deriveFromTargetBase(canonicalKey, path) {
		return
	}
	if b.deriveFromRootType(canonicalKey, path) {
		return
	}
	b.unresolved[canonicalKey] = struct{}{}
}

func (b *pointValueEnvBuilder) recordConditionProofPath(key constraint.PathKey, path constraint.Path) {
	if b.deriveFromTargetBase(key, path) {
		return
	}
	if b.deriveFromRootType(key, path) {
		return
	}
	b.unresolved[key] = struct{}{}
}

func (b *pointValueEnvBuilder) keyAtPoint(path constraint.Path) constraint.PathKey {
	if path.IsEmpty() {
		return ""
	}
	if path.IsPlaceholder() {
		return b.s.pkResolver.KeyAt(b.point, path)
	}
	if path.Symbol == 0 {
		return ""
	}
	if path.Version != 0 {
		return b.s.pkResolver.KeyAtVersion(path.Symbol, path.Version, path.Segments)
	}
	if b.queryVisibleLookup {
		ver, ok := b.visibleVersions[path.Symbol]
		if !ok || ver.IsZero() {
			return ""
		}
		return b.s.pkResolver.KeyAtVersion(path.Symbol, ver.ID, path.Segments)
	}
	if b.hasMissingVersions {
		if _, missing := b.missingVersions[path.Symbol]; missing {
			return ""
		}
	}
	if verID, ok := b.versionIDs[path.Symbol]; ok {
		return b.s.pkResolver.KeyAtVersion(path.Symbol, verID, path.Segments)
	}
	ver := b.s.pkResolver.VersionAtSym(b.point, path.Symbol)
	if ver.IsZero() {
		b.missingVersions[path.Symbol] = struct{}{}
		b.hasMissingVersions = true
		return ""
	}
	b.versionIDs[path.Symbol] = ver.ID
	return b.s.pkResolver.KeyAtVersion(path.Symbol, ver.ID, path.Segments)
}

func (b *pointValueEnvBuilder) deriveFromTargetBase(key constraint.PathKey, path constraint.Path) bool {
	if !isDescendantOf(path, b.targetPath) || b.baseType == nil {
		return false
	}
	relativeSegs := path.Segments[len(b.targetPath.Segments):]
	if len(relativeSegs) == 0 {
		return false
	}
	derived, ok := deriveTypeFrom(b.s.resolver, b.baseType, relativeSegs)
	if !ok {
		return false
	}
	b.values[key] = derived
	return true
}

func (b *pointValueEnvBuilder) deriveFromRootType(key constraint.PathKey, path constraint.Path) bool {
	rootType, ok := b.resolveRootType(path.Symbol)
	if !ok {
		return false
	}
	if len(path.Segments) == 0 {
		b.values[key] = rootType
		return true
	}
	derived, ok := deriveTypeFrom(b.s.resolver, rootType, path.Segments)
	if !ok {
		return false
	}
	b.values[key] = derived
	return true
}

func (b *pointValueEnvBuilder) resolveRootType(sym cfg.SymbolID) (typ.Type, bool) {
	if sym == 0 {
		return nil, false
	}
	rootKey := b.keyAtPoint(constraint.Path{Symbol: sym})
	if rootKey == "" {
		return nil, false
	}
	if t, ok := b.values[rootKey]; ok && t != nil {
		return t, true
	}
	if b.mode == pointValueEnvConditionProof {
		if declType := b.declaredType(sym); declType != nil {
			b.values[rootKey] = declType
			return declType, true
		}
		return nil, false
	}
	if t := b.s.projectedValueAtPoint(b.point, string(rootKey)); t != nil {
		if declType := b.declaredType(sym); typ.IsUnknown(t) && declType != nil && !typ.IsUnknown(declType) {
			t = declType
		}
		b.values[rootKey] = t
		return t, true
	}
	if declType := b.declaredType(sym); declType != nil {
		b.values[rootKey] = declType
		return declType, true
	}
	return nil, false
}

func (b *pointValueEnvBuilder) declaredType(sym cfg.SymbolID) typ.Type {
	if b.s.inputs == nil || b.s.inputs.DeclaredTypes == nil {
		return nil
	}
	return b.s.inputs.DeclaredTypes[sym]
}
