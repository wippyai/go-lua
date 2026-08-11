// Package equation owns concrete equation topology. Cold composition declares
// domain schemas; this package turns exact anchored occurrences into the one
// executable graph. Builder references are local wiring only: canonical keys
// are derived here and are never accepted from a caller.
package equation

import (
	"context"
	"crypto/sha256"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// identityVersion changes whenever retained equation identity changes.  This
// version replaces the former conflated point and rule-instance identity with the one
// topology-owned sealed Batch of Site/Occurrence/Operand capabilities, and
// makes every input retain its complete boundary transport law.
const identityVersion = 16

// RuleRef and PointRef name rows in one transient Builder input only. They
// are intentionally not semantic identities and never survive compilation.
// References are one-based so zero is always invalid.
type RuleRef uint64
type PointRef uint64

func RuleAt(index int) RuleRef {
	if index < 0 {
		return 0
	}
	return RuleRef(uint64(index) + 1)
}

func PointAt(index int) PointRef {
	if index < 0 {
		return 0
	}
	return PointRef(uint64(index) + 1)
}

// Decision is one canonical symbolic decision identity. Scopes and edge
// reindexes contain only these identities; no carrier atom, value, or
// executable predicate crosses the equation boundary.
type Decision struct{ key composition.Key }

func NewDecision(key composition.Key) (Decision, bool) {
	decision := Decision{key: key}
	return decision, decision.Available()
}

func (decision Decision) Available() bool      { return decision.key.Available() }
func (decision Decision) Key() composition.Key { return decision.key }

// Scope is a finite canonical decision universe. It belongs to each issued
// Point and to each Group output, never to a schedule region or Mu head.
type Scope struct {
	row *scopeRow
}

type scopeRow struct {
	key       composition.Key
	decisions []Decision
}

func EmptyScope() Scope {
	scope, _ := NewScope()
	return scope
}

func NewScope(decisions ...Decision) (Scope, bool) {
	normalized := append([]Decision(nil), decisions...)
	sort.Slice(normalized, func(i, j int) bool { return lessKey(normalized[i].key, normalized[j].key) })
	for index, decision := range normalized {
		if !decision.Available() || index > 0 && decision.key == normalized[index-1].key {
			return Scope{}, false
		}
	}
	key, ok := identityKey("analysis/engine/equation/scope", func(writer *canonical.Writer) bool {
		if writer.Count(uint64(len(normalized))) != nil {
			return false
		}
		for _, decision := range normalized {
			if !writeKey(writer, decision.key) {
				return false
			}
		}
		return true
	})
	if !ok {
		return Scope{}, false
	}
	return Scope{row: &scopeRow{key: key, decisions: normalized}}, true
}

func (scope Scope) Available() bool { return scope.row != nil && scope.row.key.Available() }
func (scope Scope) Key() composition.Key {
	if !scope.Available() {
		return composition.Key{}
	}
	return scope.row.key
}
func (scope Scope) Count() int {
	if !scope.Available() {
		return 0
	}
	return len(scope.row.decisions)
}
func (scope Scope) At(index int) (Decision, bool) {
	if !scope.Available() || index < 0 || index >= len(scope.row.decisions) {
		return Decision{}, false
	}
	return scope.row.decisions[index], true
}
func (scope Scope) contains(decision Decision) bool {
	if !scope.Available() || !decision.Available() {
		return false
	}
	index := sort.Search(len(scope.row.decisions), func(index int) bool {
		return !lessKey(scope.row.decisions[index].key, decision.key)
	})
	return index < len(scope.row.decisions) && scope.row.decisions[index].key == decision.key
}

func sameScope(left, right Scope) bool {
	return left.Available() && right.Available() && left.row.key == right.row.key
}

type DecisionDisposition uint8

const (
	DecisionIdentity DecisionDisposition = iota + 1
	DecisionForget
	DecisionRename
	DecisionSubstitute
)

// DecisionMap is one total source-decision disposition in a Reindex.
// Identity/Rename carry Target; Substitute carries an Expr over target Scope.
type DecisionMap struct {
	Source      Decision
	Disposition DecisionDisposition
	Target      Decision
	Expr        Expr
}

func Identity(source Decision) DecisionMap {
	return DecisionMap{Source: source, Disposition: DecisionIdentity, Target: source}
}
func Forget(source Decision) DecisionMap {
	return DecisionMap{Source: source, Disposition: DecisionForget}
}
func Rename(source, target Decision) DecisionMap {
	return DecisionMap{Source: source, Disposition: DecisionRename, Target: target}
}
func Substitute(source Decision, expr Expr) DecisionMap {
	return DecisionMap{Source: source, Disposition: DecisionSubstitute, Expr: expr}
}

// Reindex is one total simultaneous edge transport relation. It is declared
// on a directed Group input and is deliberately not attached to an SCC,
// schedule region, source Point, or Mu head.
type Reindex struct {
	source Scope
	target Scope
	maps   []DecisionMap
	key    composition.Key
}

func IdentityReindex(scope Scope) Reindex {
	if !scope.Available() {
		return Reindex{}
	}
	maps := make([]DecisionMap, len(scope.row.decisions))
	for index, decision := range scope.row.decisions {
		maps[index] = Identity(decision)
	}
	reindex, _ := NewReindex(scope, scope, maps)
	return reindex
}

func NewReindex(source, target Scope, maps []DecisionMap) (Reindex, bool) {
	if !source.Available() || !target.Available() || len(maps) != len(source.row.decisions) {
		return Reindex{}, false
	}
	normalized := append([]DecisionMap(nil), maps...)
	sort.Slice(normalized, func(i, j int) bool { return lessKey(normalized[i].Source.key, normalized[j].Source.key) })
	plainTargets := make(map[composition.Key]struct{}, len(normalized))
	for index, mapping := range normalized {
		if !mapping.Source.Available() || !source.contains(mapping.Source) || index > 0 && mapping.Source.key == normalized[index-1].Source.key {
			return Reindex{}, false
		}
		switch mapping.Disposition {
		case DecisionIdentity:
			if mapping.Target != mapping.Source || !target.contains(mapping.Target) || mapping.Expr.Available() {
				return Reindex{}, false
			}
			if _, duplicate := plainTargets[mapping.Target.key]; duplicate {
				return Reindex{}, false
			}
			plainTargets[mapping.Target.key] = struct{}{}
		case DecisionForget:
			if mapping.Target.Available() || mapping.Expr.Available() {
				return Reindex{}, false
			}
		case DecisionRename:
			if !mapping.Target.Available() || mapping.Target == mapping.Source || !target.contains(mapping.Target) || mapping.Expr.Available() {
				return Reindex{}, false
			}
			if _, duplicate := plainTargets[mapping.Target.key]; duplicate {
				return Reindex{}, false
			}
			plainTargets[mapping.Target.key] = struct{}{}
		case DecisionSubstitute:
			if mapping.Target.Available() || !mapping.Expr.Available() {
				return Reindex{}, false
			}
			for _, decision := range mapping.Expr.Decisions() {
				if !target.contains(decision) {
					return Reindex{}, false
				}
			}
		default:
			return Reindex{}, false
		}
	}
	for index, decision := range source.row.decisions {
		if normalized[index].Source != decision {
			return Reindex{}, false
		}
	}
	key, ok := identityKey("analysis/engine/equation/reindex", func(writer *canonical.Writer) bool {
		return writeScope(writer, source) && writeScope(writer, target) && writeDecisionMaps(writer, normalized)
	})
	if !ok {
		return Reindex{}, false
	}
	return Reindex{source: source, target: target, maps: normalized, key: key}, true
}

func (reindex Reindex) Available() bool      { return reindex.key.Available() }
func (reindex Reindex) Key() composition.Key { return reindex.key }
func (reindex Reindex) Source() Scope        { return reindex.source }
func (reindex Reindex) Target() Scope        { return reindex.target }
func (reindex Reindex) Count() int           { return len(reindex.maps) }
func (reindex Reindex) At(index int) (DecisionMap, bool) {
	if !reindex.Available() || index < 0 || index >= len(reindex.maps) {
		return DecisionMap{}, false
	}
	return reindex.maps[index], true
}

// Identity reports the complete, issued-scope-preserving no-op relation.  It
// is deliberately stricter than semantic equivalence: input state reuse is
// lawful only when every source decision retains itself in the same scope.
func (reindex Reindex) Identity() bool {
	if !reindex.Available() || !sameScope(reindex.source, reindex.target) || len(reindex.maps) != len(reindex.source.row.decisions) {
		return false
	}
	for index, decision := range reindex.source.row.decisions {
		mapping := reindex.maps[index]
		if mapping.Disposition != DecisionIdentity || mapping.Source != decision || mapping.Target != decision || mapping.Expr.Available() {
			return false
		}
	}
	return true
}

// Input is one directed boundary edge.  It atomically retains its source and
// target Sites, exact provenance, precondition over the source scope, total
// simultaneous reindex, and postcondition over the target scope.  Builder
// references never stand in for those semantic coordinates.
type Input struct {
	source     Site
	target     Site
	provenance composition.Key
	pre        Expr
	omega      Reindex
	post       Expr
	point      Point
	key        composition.Key
	identity   bool
}

// BoundaryInput is the sole input constructor.  It retains no mutable or raw
// source key and cannot be used before its Site capabilities seal.
func BoundaryInput(source, target Site, provenance composition.Key, pre Expr, omega Reindex, post Expr) Input {
	input := Input{source: source, target: target, provenance: provenance, pre: pre, omega: omega, post: post}
	if !input.validBoundary() {
		return Input{}
	}
	key, ok := deriveInputKey(input)
	if !ok {
		return Input{}
	}
	input.key = key
	input.identity = input.identityTransport()
	return input
}

func (input Input) Available() bool             { return input.key.Available() && input.validBoundary() }
func (input Input) Point() Point                { return input.point }
func (input Input) Source() Site                { return input.source }
func (input Input) Target() Site                { return input.target }
func (input Input) Provenance() composition.Key { return input.provenance }
func (input Input) Pre() Expr                   { return input.pre }
func (input Input) Reindex() Reindex            { return input.omega }
func (input Input) Post() Expr                  { return input.post }
func (input Input) Key() composition.Key        { return input.key }

// IdentityTransport is the only fast path allowed to retain an immutable
// input state.  Equal-looking scopes are insufficient: the issued scope,
// complete identity reindex, and both canonical formulas must all match.
func (input Input) IdentityTransport() bool {
	return input.identity
}

func (input Input) identityTransport() bool {
	return input.key.Available() && input.source.Scope().Key() == input.target.Scope().Key() && sameScope(input.source.Scope(), input.target.Scope()) &&
		input.omega.Identity() && input.pre.IsTrue() && input.post.IsTrue()
}

func (input Input) validBoundary() bool {
	if !input.source.Available() || !input.target.Available() || input.source.batch != input.target.batch || !input.provenance.Available() || !input.pre.Available() || !input.post.Available() || !input.omega.Available() {
		return false
	}
	if !sameScope(input.source.Scope(), input.omega.Source()) || !sameScope(input.target.Scope(), input.omega.Target()) {
		return false
	}
	for _, decision := range input.pre.Decisions() {
		if !input.source.Scope().contains(decision) {
			return false
		}
	}
	for _, decision := range input.post.Decisions() {
		if !input.target.Scope().contains(decision) {
			return false
		}
	}
	return true
}

// Point is an equation-issued identity. Its key is private so only this
// package can create a usable point.
type Point struct {
	graph *Graph
	key   composition.Key
	site  Site
}

func (point Point) Available() bool { return point.key.Available() }
func (point Point) Key() composition.Key {
	return point.key
}
func (point Point) Scope() Scope { return point.site.Scope() }
func (point Point) Site() Site   { return point.site }

// Init returns the immutable initialization formula/disposition owned by this
// Point's exact Site. Runtime must not infer ingress from an empty producer
// range or fabricate a truth condition.
func (point Point) Init() (Expr, InitDisposition, bool) { return point.site.Init() }
func (point Point) HasInit() bool {
	_, disposition, ok := point.site.Init()
	return ok && disposition == InitPresent
}

// Surface is the canonical identity of an engine-issued typed capability. It
// has no carrier slot or callback identity.
type SurfaceForm uint8

const (
	SurfaceReadExact SurfaceForm = iota + 1
	SurfaceReadSummary
	SurfaceReadSelect
	SurfaceWriteExact
	SurfaceWriteSelect
	SurfaceWriteRoute
)

// TargetMode is the equation-local update authority of one write surface. It
// deliberately does not expose a carrier target: equation describes which
// sealed capability is required, while the later binder materializes it.
//
// Only an exact write has update authority.  Reads and staged write selectors
// must retain TargetModeNone.  A strong exact Local is the one-based dense
// Factor key (raw key 0 is Local 1); a weak exact Local is an opaque,
// non-zero topology identity and is never decoded as a raw Factor key.
type TargetMode uint8

const (
	TargetModeNone TargetMode = iota
	TargetModeStrong
	TargetModeWeak
)

type Surface struct {
	Factor     composition.Key
	Form       SurfaceForm
	Local      uint64
	Semantic   composition.Key
	Normalizer composition.Key
	Mode       TargetMode
}

func (surface Surface) Available() bool {
	if !surface.Factor.Available() || surface.Local == 0 {
		return false
	}
	switch surface.Form {
	case SurfaceReadExact, SurfaceReadSummary, SurfaceReadSelect, SurfaceWriteSelect, SurfaceWriteRoute:
		return surface.Mode == TargetModeNone
	case SurfaceWriteExact:
		return surface.Mode == TargetModeStrong || surface.Mode == TargetModeWeak
	default:
		return false
	}
}

// StructuralSurface identifies the exact structural support coordinate and
// its declared Composition/Solver law. It deliberately has no Factor field.
type StructuralSurface struct {
	Local    uint64
	Semantic composition.Key
}

func (surface StructuralSurface) Available() bool {
	return surface.Local != 0 && surface.Semantic.Available()
}

type ResolvedRead struct {
	Index   uint64
	Surface Surface
}

// ResolvedCarry has no local surface: the cold schema fixes its output
// Factor and the index fixes its exact input source.
type ResolvedCarry struct{ Index uint64 }

// CandidateRelation seals one prior staged-write relation for a resolved
// selector target. Matches is indexed by the current candidate ordinal; each
// inner vector is the sorted unique set of matching prior candidate ordinals.
// Read dependencies intentionally have no relation row.
type CandidateRelation struct {
	Prior   uint64
	Matches [][]uint64
}

type ResolvedWrite struct {
	Index   uint64
	Surface Surface
	// Route is the one-based ReadSelect ordinal consumed by a route write.
	// It is zero for exact and static-selector writes.
	Route            uint64
	Candidates       []uint64
	TargetCandidates []Surface
	Relations        []CandidateRelation
}
type ResolvedSupport struct {
	Index   uint64
	Surface StructuralSurface
}
type ResolvedPrune struct {
	Index   uint64
	Surface StructuralSurface
}

// RuleInstance is a caller declaration of facts needed to derive one exact
// Occurrence. Operand is deliberately independent from occurrence identity,
// but Batch admission proves it belongs to this exact occurrence before any
// rule can enter topology.
type RuleInstance struct {
	Schema        composition.Key
	OperandFamily composition.Key
	Occurrence    Occurrence
	Operand       Operand
	Reads         []ResolvedRead
	Carries       []ResolvedCarry
	Writes        []ResolvedWrite
	Supports      []ResolvedSupport
	Prunes        []ResolvedPrune

	// activation is issued only while an accepted dynamic Member materializes
	// this otherwise ordinary row.  It is deliberately not builder input and
	// does not participate in its canonical row identity: the derived dynamic
	// occurrence/operand already carry that namespace.  The runtime uses this
	// private witness to expose exact coordinates to a typed Rule without
	// creating an application×endpoint operand table.
	activation Member
}

// PointSpec declares one stable state coordinate from one exact sealed Site.
// Scope and initialization are consumed from that Site; a caller cannot split
// them across a Point declaration or manufacture recursive point identities.
type PointSpec struct {
	Site Site
}

// Group is one atomic RHS hyperedge. Every member reads its exact same ordered
// input snapshot and contributes only to the declared output Point. Dynamic
// relation ownership is represented only by the Member-bound template which
// materialized these ordinary rows; Group carries no parallel candidate tag.
type Group struct {
	Members []RuleRef
	Output  PointRef
	Inputs  []Input
	// EnvironmentInput is one extra, exact boundary for the Group's program
	// environment. It is deliberately outside Inputs: Inputs remain exactly
	// the Rule schema's conjunctive dependency ports. Its zero value is the
	// unavailable Input and therefore means no environment seed.
	EnvironmentInput Input
	// premise is topology-issued accepted evidence. It is intentionally not
	// part of the public builder grammar; ordinary groups leave it zero and
	// compile as True.
	premise Expr
}

// EnvironmentEdge is one structural control edge into a Point. It carries an
// existing exact Input and has no Rule, Member, Group, or candidate identity.
// Target is retained explicitly so an edge cannot be retargeted by resolving
// only an equal-looking Site later in lowering.
type EnvironmentEdge struct {
	Target PointRef
	Input  Input
}

// FactorEdge is one structural Point-to-Point transport that projects the
// carried Contribution to exactly one Factor plane at its target.  It has no
// Rule, Member, Group, or callback authority.  Factor is the sealed cold
// Composition key of the CarryForm that issued the row.
type FactorEdge struct {
	Target PointRef
	Input  Input
	Factor composition.Key
}

// QueryInstance is one concrete observation declaration. Point is an input
// specification rather than a fabricated point identity; Key is derived from
// Family, the resolved observed Point, and its positional resolved surfaces.
// Structural support queries carry an empty surface vector.
type QueryInstance struct {
	Family   composition.Key
	Point    PointRef
	Surfaces []Surface
}

// SummaryMapping assigns one resolved summary-read surface its closed raw-key
// set. Keys use the Factor's raw dense coordinate [0, KeyEnd); this differs
// intentionally from exact Surface.Local, which is one-based so zero remains
// unavailable. The Factor binder checks KeyEnd because it alone owns the
// typed dense key universe.
//
// Different summary surfaces may describe the same key set. They remain
// distinct semantic surfaces, but weak-target coverage resolves those aliases
// to one canonical unit representative during topology compilation.
type SummaryMapping struct {
	Surface Surface
	Keys    []uint64
}

// WeakTargetMapping assigns one weak exact-write surface the closed set of
// read units it covers. Coverage is a set: callers must provide it in strict
// Surface order without duplicates. The compiled Graph exposes its canonical
// unit representatives through a flat immutable catalog.
type WeakTargetMapping struct {
	Surface    Surface
	Candidates []Surface
}

// TopologySpec is one disposable Builder input. Reordering rows and
// consistently renumbering these local references leaves all compiled
// identities unchanged. It has no authority after SealTopology succeeds.
type TopologySpec struct {
	Batch              *Batch
	Rules              []RuleInstance
	Points             []PointSpec
	Groups             []Group
	Queries            []QueryInstance
	EnvironmentEdges   []EnvironmentEdge
	FactorEdges        []FactorEdge
	ActivationBindings []ActivationBinding
	Summaries          []SummaryMapping
	WeakTargets        []WeakTargetMapping
}

// Query is an equation-issued retained observation identity.
type Query struct {
	graph    *Graph
	key      composition.Key
	point    Point
	family   composition.Key
	surfaces []Surface
}

func (query Query) Key() composition.Key    { return query.key }
func (query Query) Point() Point            { return query.point }
func (query Query) Family() composition.Key { return query.family }
func (query Query) Surfaces() []Surface     { return append([]Surface(nil), query.surfaces...) }

type canonicalInstance struct {
	key composition.Key
	row RuleInstance
}

type groupCore struct{ key composition.Key }

func deriveInstanceKey(row RuleInstance, catalog topologyCatalog) (composition.Key, bool) {
	return identityKey("analysis/engine/equation/rule-instance", func(writer *canonical.Writer) bool {
		return writeKey(writer, row.Schema) && writeKey(writer, row.OperandFamily) && writeOccurrence(writer, row.Occurrence) && writeOperand(writer, row.Operand) &&
			writeReads(writer, row.Reads) && writeCarries(writer, row.Carries) &&
			writeWrites(writer, row.Writes) && writeRuleCatalog(writer, row, catalog) &&
			writeSupports(writer, row.Supports) && writePrunes(writer, row.Prunes)
	})
}

func deriveGroupCore(members []canonicalInstance) (groupCore, bool) {
	key, ok := identityKey("analysis/engine/equation/group-members", func(writer *canonical.Writer) bool {
		if writer.Count(uint64(len(members))) != nil {
			return false
		}
		for _, member := range members {
			if !writeKey(writer, member.key) {
				return false
			}
		}
		return true
	})
	return groupCore{key: key}, ok
}

func deriveInputKey(input Input) (composition.Key, bool) {
	return identityKey("analysis/engine/equation/input-boundary", func(writer *canonical.Writer) bool {
		return input.source.Available() && input.target.Available() && input.provenance.Available() && input.pre.Available() && input.omega.Available() && input.post.Available() &&
			writeSite(writer, input.source) && writeSite(writer, input.target) && writeKey(writer, input.provenance) && writeExpr(writer, input.pre) && writeReindex(writer, input.omega) && writeExpr(writer, input.post)
	})
}

// deriveGroupKey retains the stable output coordinate and every ordered
// resolved predecessor relation directly.  Point identities have already been
// issued, so this is one finite non-recursive encoding even for feedback.
func deriveGroupKey(core groupCore, output Point, inputs []Input) (composition.Key, bool) {
	return derivePremisedGroupKey(core, output, inputs, TrueExpr())
}

// derivePremisedGroupKey is the one identity authority for both ordinary and
// accepted dynamic Groups. Builder-authored groups receive True; only
// Topology expansion may supply a retained accepted premise.
func derivePremisedGroupKey(core groupCore, output Point, inputs []Input, premise Expr) (composition.Key, bool) {
	return identityKey("analysis/engine/equation/group", func(writer *canonical.Writer) bool {
		if !premise.Available() || !writeKey(writer, core.key) || !writePoint(writer, output) || !writeExpr(writer, premise) || writer.Count(uint64(len(inputs))) != nil {
			return false
		}
		for _, input := range inputs {
			if !input.Available() || !writeKey(writer, input.key) || !writePoint(writer, input.point) {
				return false
			}
		}
		return true
	})
}

func derivePremisedGroupKeyWithEnvironment(core groupCore, output Point, inputs []Input, premise Expr, environmentInput Input) (composition.Key, bool) {
	return identityKey("analysis/engine/equation/group", func(writer *canonical.Writer) bool {
		if !premise.Available() || !environmentInput.Available() || !writeKey(writer, core.key) || !writePoint(writer, output) || !writeExpr(writer, premise) || !writeKey(writer, environmentInput.key) || writer.Count(uint64(len(inputs))) != nil {
			return false
		}
		for _, input := range inputs {
			if !input.Available() || !writeKey(writer, input.key) || !writePoint(writer, input.point) {
				return false
			}
		}
		return true
	})
}

// derivePoint is the sole issued-point identity authority.  Its Site already
// pins source identity, issued scope, initialization formula, and disposition.
func derivePoint(site Site) (Point, bool) {
	key, ok := identityKey("analysis/engine/equation/point", func(writer *canonical.Writer) bool {
		return writeSite(writer, site)
	})
	return Point{key: key, site: site}, ok
}

func deriveQueryKey(row QueryInstance, point Point, catalog topologyCatalog) (composition.Key, bool) {
	return identityKey("analysis/engine/equation/query", func(writer *canonical.Writer) bool {
		if !writeKey(writer, row.Family) || !writeKey(writer, point.key) || !writeScope(writer, point.Scope()) || writer.Count(uint64(len(row.Surfaces))) != nil {
			return false
		}
		for _, surface := range row.Surfaces {
			if !writeSurface(writer, surface) || !writeSurfaceCatalog(writer, surface, catalog) {
				return false
			}
		}
		return true
	})
}

func identityKey(domain string, encode func(*canonical.Writer) bool) (composition.Key, bool) {
	hash := sha256.New()
	var writer canonical.Writer
	if writer.Reset(context.Background(), hash, domain, identityVersion) != nil || !encode(&writer) || writer.Finish() != nil {
		return composition.Key{}, false
	}
	digest := hash.Sum(nil)
	if len(digest) != len(composition.ID{}) {
		return composition.Key{}, false
	}
	var id composition.ID
	copy(id[:], digest)
	key := composition.Key{ID: id, Version: identityVersion}
	return key, key.Available()
}

func writeKey(writer *canonical.Writer, key composition.Key) bool {
	return writer.Bytes(key.ID[:]) == nil && writer.Uint(key.Version) == nil
}

func writeSite(writer *canonical.Writer, site Site) bool {
	return site.Available() && writeKey(writer, site.Key())
}

func writeOccurrence(writer *canonical.Writer, occurrence Occurrence) bool {
	return occurrence.Available() && writeKey(writer, occurrence.Key()) && writeSite(writer, occurrence.Site()) &&
		writeKey(writer, occurrence.Entity()) && writer.Uint(uint64(occurrence.Kind())) == nil
}

func writeOperand(writer *canonical.Writer, operand Operand) bool {
	return operand.Available() && writeKey(writer, operand.Key()) && writeOccurrence(writer, operand.Occurrence()) && writeKey(writer, operand.Entity())
}

func writeScope(writer *canonical.Writer, scope Scope) bool {
	if !scope.Available() {
		return false
	}
	if writer.Count(uint64(len(scope.row.decisions))) != nil {
		return false
	}
	for _, decision := range scope.row.decisions {
		if !decision.Available() || !writeKey(writer, decision.key) {
			return false
		}
	}
	return true
}

func writePoint(writer *canonical.Writer, point Point) bool {
	return point.Available() && writeKey(writer, point.key) && writeSite(writer, point.site)
}

func writeExpr(writer *canonical.Writer, expr Expr) bool {
	if !expr.Available() || writer.Uint(uint64(expr.root)) != nil || writer.Count(uint64(len(expr.nodes))) != nil {
		return false
	}
	for _, node := range expr.nodes {
		if !node.decision.Available() || writer.Uint(uint64(node.low)) != nil || writer.Uint(uint64(node.high)) != nil || !writeKey(writer, node.decision.key) {
			return false
		}
	}
	return true
}

func writeDecisionMaps(writer *canonical.Writer, maps []DecisionMap) bool {
	if writer.Count(uint64(len(maps))) != nil {
		return false
	}
	for _, mapping := range maps {
		if !mapping.Source.Available() || writer.Uint(uint64(mapping.Disposition)) != nil || !writeKey(writer, mapping.Source.key) {
			return false
		}
		switch mapping.Disposition {
		case DecisionIdentity, DecisionRename:
			if !mapping.Target.Available() || !writeKey(writer, mapping.Target.key) {
				return false
			}
		case DecisionForget:
		case DecisionSubstitute:
			if !writeExpr(writer, mapping.Expr) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func writeReindex(writer *canonical.Writer, reindex Reindex) bool {
	return reindex.Available() && writeScope(writer, reindex.source) && writeScope(writer, reindex.target) && writeDecisionMaps(writer, reindex.maps)
}

func writeSurface(writer *canonical.Writer, surface Surface) bool {
	return writer.Uint(uint64(surface.Form)) == nil && writeKey(writer, surface.Factor) && writer.Uint(surface.Local) == nil &&
		writeKey(writer, surface.Semantic) && writeKey(writer, surface.Normalizer) && writer.Uint(uint64(surface.Mode)) == nil
}

func writeStructuralSurface(writer *canonical.Writer, surface StructuralSurface) bool {
	return writer.Uint(surface.Local) == nil && writeKey(writer, surface.Semantic)
}

func writeReads(writer *canonical.Writer, rows []ResolvedRead) bool {
	if writer.Count(uint64(len(rows))) != nil {
		return false
	}
	for _, row := range rows {
		if writer.Uint(row.Index) != nil || !writeSurface(writer, row.Surface) {
			return false
		}
	}
	return true
}

func writeCarries(writer *canonical.Writer, rows []ResolvedCarry) bool {
	if writer.Count(uint64(len(rows))) != nil {
		return false
	}
	for _, row := range rows {
		if writer.Uint(row.Index) != nil {
			return false
		}
	}
	return true
}

func writeWrites(writer *canonical.Writer, rows []ResolvedWrite) bool {
	if writer.Count(uint64(len(rows))) != nil {
		return false
	}
	for _, row := range rows {
		if writer.Uint(row.Index) != nil || !writeSurface(writer, row.Surface) || writer.Uint(row.Route) != nil || !writeOrdinals(writer, row.Candidates) || !writeSurfaces(writer, row.TargetCandidates) || !writeCandidateRelations(writer, row.Relations) {
			return false
		}
	}
	return true
}

func writeSurfaces(writer *canonical.Writer, values []Surface) bool {
	if writer.Count(uint64(len(values))) != nil {
		return false
	}
	for _, value := range values {
		if !writeSurface(writer, value) {
			return false
		}
	}
	return true
}

func writeOrdinals(writer *canonical.Writer, values []uint64) bool {
	if writer.Count(uint64(len(values))) != nil {
		return false
	}
	for _, value := range values {
		if writer.Uint(value) != nil {
			return false
		}
	}
	return true
}

func writeCandidateRelations(writer *canonical.Writer, values []CandidateRelation) bool {
	if writer.Count(uint64(len(values))) != nil {
		return false
	}
	for _, relation := range values {
		if writer.Uint(relation.Prior) != nil || writer.Count(uint64(len(relation.Matches))) != nil {
			return false
		}
		for _, matches := range relation.Matches {
			if !writeOrdinals(writer, matches) {
				return false
			}
		}
	}
	return true
}

func writeSupports(writer *canonical.Writer, rows []ResolvedSupport) bool {
	if writer.Count(uint64(len(rows))) != nil {
		return false
	}
	for _, row := range rows {
		if writer.Uint(row.Index) != nil || !writeStructuralSurface(writer, row.Surface) {
			return false
		}
	}
	return true
}

func writePrunes(writer *canonical.Writer, rows []ResolvedPrune) bool {
	if writer.Count(uint64(len(rows))) != nil {
		return false
	}
	for _, row := range rows {
		if writer.Uint(row.Index) != nil || !writeStructuralSurface(writer, row.Surface) {
			return false
		}
	}
	return true
}
