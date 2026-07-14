package program

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/relationcall"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// callRouteKind gives exactly one runtime authority to every call in a sealed
// call surface. The concrete lexical kind exists only while lexical producers
// are migrated to Relations; it is not a second chance for a relation miss.
type callRouteKind uint8

const (
	callRouteRelationLexical callRouteKind = iota + 1
	callRouteConcreteLexical
	callRouteBoundary
)

// callRouteRoutingDigest identifies only the durable routing decision: owner,
// call surface, route kinds, producer identities, and boundary shapes. It
// deliberately excludes run-local cell ordinals and generation pointers.
//
// It is NOT a semantic Relation or result-cache fence: specialized Summary
// payloads and the transitive frozen Relation dependency closure are outside
// this digest. A cache must separately validate its complete relation artifact
// or tracked Summary reads. Keeping the type private prevents accidental use by
// the public cache API while that semantic artifact digest does not yet exist.
type callRouteRoutingDigest [sha256.Size]byte

func (d callRouteRoutingDigest) available() bool { return d != callRouteRoutingDigest{} }

// callRouteDispatchSeal is a run-local, partition-specific authority. Unlike
// routingDigest, pointer identity also fences the semantic Relation snapshot.
type callRouteDispatchSeal struct{ identity byte }

// callRoutePartition is an immutable, generation-validated routing product.
// kinds is dense by CFG point, while present distinguishes non-call points.
// relationCatalog contains exactly the relation-owned lexical subset.
type callRoutePartition struct {
	owner           relationConsumerIdentity
	pointCount      int
	kinds           []callRouteKind
	present         []bool
	relations       transformer.RelationSnapshot
	relationCatalog relationcall.Catalog
	routingDigest   callRouteRoutingDigest
	dispatchSeal    *callRouteDispatchSeal
}

// newCallRoutePartition classifies the complete sealed call surface and binds
// relation routes to the same frozen snapshot generation. Identity, width,
// target body, relation shape, and contextual publication drift reject the
// complete partition; partially classified tables are never returned.
func newCallRoutePartition(snapshot relationRunSnapshot, owner relationConsumerIdentity) (callRoutePartition, error) {
	if snapshot.generation == nil || owner.Generation == nil || owner.Generation != snapshot.generation {
		return callRoutePartition{}, fmt.Errorf("program: call route generation drift")
	}
	if owner.Summary.Ref.IsZero() || owner.Prepared == nil || owner.BodyDigest == 0 || owner.Prepared.IdentityDigest() != owner.BodyDigest {
		return callRoutePartition{}, fmt.Errorf("program: call route owner identity drift")
	}
	plan := owner.Prepared.OperationPlan()
	if plan == nil {
		return callRoutePartition{}, fmt.Errorf("program: call route owner has no operation plan")
	}
	surface, exact := plan.CallSurface()
	if !exact || !surface.Complete() || !surface.Digest().Available() ||
		surface.Owner() != owner.Prepared.StableLexicalBodyID() {
		return callRoutePartition{}, fmt.Errorf("program: call route surface identity drift")
	}
	direct, ok := snapshot.DirectCalls(owner)
	if !ok {
		return callRoutePartition{}, fmt.Errorf("program: call route consumer identity drift")
	}
	if plan.PointCount() != surface.PointCount() || direct.PointCount() != plan.PointCount() {
		return callRoutePartition{}, fmt.Errorf("program: call route width drift: plan=%d surface=%d direct=%d", plan.PointCount(), surface.PointCount(), direct.PointCount())
	}

	out := callRoutePartition{
		owner:        owner,
		pointCount:   plan.PointCount(),
		kinds:        make([]callRouteKind, plan.PointCount()),
		present:      make([]bool, plan.PointCount()),
		relations:    snapshot.relations,
		dispatchSeal: &callRouteDispatchSeal{},
	}
	relationRoutes := make([]relationcall.Route, 0, len(direct.Cells()))
	routedRelations := 0
	for _, site := range surface.Sites() {
		if uint64(site.Point) >= uint64(out.pointCount) || out.present[site.Point] {
			return callRoutePartition{}, fmt.Errorf("program: call route surface point %d is ambiguous", site.Point)
		}
		if _, represented := plan.Facts().CallSiteView(site.Point); !represented {
			return callRoutePartition{}, fmt.Errorf("program: call route point %d has no call fact", site.Point)
		}
		out.present[site.Point] = true
		directTarget, converted := direct.Lookup(site.Point)
		switch site.Target.Kind() {
		case operationplan.CallSurfaceTargetLexical:
			if !converted {
				out.kinds[site.Point] = callRouteConcreteLexical
				continue
			}
			targetBody, lexical := site.Target.LexicalBody()
			identity, found := snapshot.Identity(directTarget.Cell)
			if !lexical || !found || identity.Generation != owner.Generation || identity.Prepared == nil ||
				identity.Summary.Ref.IsZero() || identity.BodyDigest == 0 || identity.Prepared.IdentityDigest() != identity.BodyDigest ||
				identity.Prepared.StableLexicalBodyID() != targetBody {
				return callRoutePartition{}, fmt.Errorf("program: call route target identity drift at point %d", site.Point)
			}
			relation, found := snapshot.Lookup(identity)
			if !found || relation.Shape() != directTarget.Shape || relation.ContextualReason() != "" || relation.Widened() {
				return callRoutePartition{}, fmt.Errorf("program: call route target shape drift at point %d", site.Point)
			}
			dependencyKey := identity.Summary
			var specialized summary.Summary
			hasSpecialized := false
			if contextual, contextualRoute := snapshot.DependencyKey(owner, site.Point); contextualRoute {
				dependencyKey = contextual
				specialized, hasSpecialized = snapshot.contextSummaryByKey(contextual)
				if !hasSpecialized {
					return callRoutePartition{}, fmt.Errorf("program: call route contextual publication drift at point %d", site.Point)
				}
			} else if !paramsOnlyShape(relation.Shape()) {
				return callRoutePartition{}, fmt.Errorf("program: call route target at point %d requires a contextual publication", site.Point)
			}
			out.kinds[site.Point] = callRouteRelationLexical
			relationRoutes = append(relationRoutes, relationcall.Route{Point: site.Point, Target: relationcall.Target{
				Cell: identity.Cell, SummaryKey: dependencyKey, LexicalSummaryKey: identity.Summary,
				Specialized: specialized, HasSpecialized: hasSpecialized,
			}})
			routedRelations++
		case operationplan.CallSurfaceTargetExternal, operationplan.CallSurfaceTargetRejected:
			// Rejected is an explicit boundary classification from the independently
			// sealed binder/lowering call census, not an inference made here. The
			// current target payload intentionally retains no rejected lexical
			// candidate, so this layer cannot safely reconstruct one from a callee
			// symbol. Any relation route at the point is nevertheless contradictory
			// authority and rejects the complete partition below.
			if converted {
				return callRoutePartition{}, fmt.Errorf("program: non-lexical call point %d owns a relation target", site.Point)
			}
			out.kinds[site.Point] = callRouteBoundary
		default:
			return callRoutePartition{}, fmt.Errorf("program: call route point %d has invalid target kind", site.Point)
		}
	}
	if routedRelations != directRouteCount(direct) {
		return callRoutePartition{}, fmt.Errorf("program: relation route is outside the sealed call surface")
	}
	relationCatalog, err := relationcall.NewCatalog(out.pointCount, relationRoutes)
	if err != nil {
		return callRoutePartition{}, err
	}
	out.relationCatalog = relationCatalog
	out.routingDigest, err = digestCallRouteRouting(out, surface, snapshot)
	if err != nil || !out.routingDigest.available() {
		if err == nil {
			err = fmt.Errorf("program: call route routing digest unavailable")
		}
		return callRoutePartition{}, err
	}
	return out, nil
}

func directRouteCount(catalog transformer.DirectCallCatalog) int {
	count := 0
	for raw := 0; raw < catalog.PointCount(); raw++ {
		if _, ok := catalog.Lookup(cfg.Point(raw)); ok {
			count++
		}
	}
	return count
}

func (p callRoutePartition) route(point cfg.Point) (callRouteKind, bool) {
	if uint64(point) >= uint64(p.pointCount) || uint64(point) >= uint64(len(p.present)) ||
		uint64(point) >= uint64(len(p.kinds)) || !p.present[point] {
		return 0, false
	}
	return p.kinds[point], true
}

// callRouteRejection is reported instead of consulting another provider. The
// solve transaction will later latch the first rejection and suppress all
// publication from that attempt.
type callRouteRejection struct {
	Owner  summary.SummaryKey
	Point  cfg.Point
	Kind   callRouteKind
	Reason string
}

// callRouteRelationProvider is a sealed resolver bound to one exact partition.
// Callers supply only semantic adaptation callbacks; the partition overwrites
// relation storage, point catalog, and dynamic target lookup with its own frozen
// authorities before constructing the resolver.
type callRouteRelationProvider struct {
	dispatchSeal *callRouteDispatchSeal
	resolver     relationcall.Resolver
}

func (p callRoutePartition) bindRelationProvider(config relationcall.Config) (callRouteRelationProvider, error) {
	if err := p.validateDispatchShape(); err != nil {
		return callRouteRelationProvider{}, err
	}
	if !p.routingDigest.available() {
		return callRouteRelationProvider{}, fmt.Errorf("program: call route relation provider has no routing identity")
	}
	config.Relations = p.relations
	catalog := p.relationCatalog
	config.Catalog = &catalog
	config.TargetFor = nil
	return callRouteRelationProvider{
		dispatchSeal: p.dispatchSeal,
		resolver:     relationcall.NewResolver(config),
	}, nil
}

func (p callRouteRelationProvider) matches(partition callRoutePartition) bool {
	return p.resolver != nil && p.dispatchSeal != nil && p.dispatchSeal == partition.dispatchSeal
}

type callRouteProviders struct {
	relation callRouteRelationProvider
	concrete callpayload.CallOutcomeProvider
	boundary callpayload.CallOutcomeProvider
	reject   func(callRouteRejection)
}

// outcomeProvider dispatches to exactly one provider selected during
// preparation. A relation miss is a transaction rejection, never permission
// to invoke the concrete lexical or boundary providers.
func (p callRoutePartition) outcomeProvider(providers callRouteProviders) (callpayload.CallOutcomeProvider, error) {
	if err := p.validateDispatchShape(); err != nil {
		return nil, err
	}
	if providers.reject == nil {
		return nil, fmt.Errorf("program: call route rejection latch unavailable")
	}
	for point, present := range p.present {
		if present && p.kinds[point] == callRouteRelationLexical && !providers.relation.matches(p) {
			return nil, fmt.Errorf("program: call route relation provider identity drift")
		}
	}
	relationResolver := providers.relation.resolver
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		point, hasPoint := site.Point()
		kind, classified := p.route(point)
		if !hasPoint || !classified {
			p.reject(providers, callRouteRejection{Owner: p.owner.Summary, Point: point, Reason: "unclassified call point"})
			return callpayload.CallOutcome{}
		}
		switch kind {
		case callRouteRelationLexical:
			if relationResolver == nil {
				p.reject(providers, callRouteRejection{Owner: p.owner.Summary, Point: point, Kind: kind, Reason: "relation provider unavailable"})
				return callpayload.CallOutcome{}
			}
			out, handled := relationResolver(ctx, site, in, read)
			if !handled {
				p.reject(providers, callRouteRejection{Owner: p.owner.Summary, Point: point, Kind: kind, Reason: "relation route missed"})
				return callpayload.CallOutcome{}
			}
			return out
		case callRouteConcreteLexical:
			if providers.concrete != nil {
				return providers.concrete(ctx, site, in, read)
			}
		case callRouteBoundary:
			if providers.boundary != nil {
				return providers.boundary(ctx, site, in, read)
			}
		default:
			p.reject(providers, callRouteRejection{Owner: p.owner.Summary, Point: point, Kind: kind, Reason: "invalid route kind"})
			return callpayload.CallOutcome{}
		}
		p.reject(providers, callRouteRejection{Owner: p.owner.Summary, Point: point, Kind: kind, Reason: "selected provider unavailable"})
		return callpayload.CallOutcome{}
	}, nil
}

func (p callRoutePartition) reject(providers callRouteProviders, rejection callRouteRejection) {
	providers.reject(rejection)
}

func (p callRoutePartition) validateDispatchShape() error {
	if p.dispatchSeal == nil || p.pointCount < 0 || len(p.kinds) != p.pointCount || len(p.present) != p.pointCount ||
		p.relationCatalog.PointCount() != p.pointCount {
		return fmt.Errorf("program: malformed call route partition")
	}
	for point, present := range p.present {
		kind := p.kinds[point]
		if !present {
			if kind != 0 {
				return fmt.Errorf("program: non-call point %d has route kind %d", point, kind)
			}
			continue
		}
		switch kind {
		case callRouteRelationLexical:
			if _, ok := p.relationCatalog.Lookup(cfg.Point(point)); !ok {
				return fmt.Errorf("program: relation call point %d has no sealed target", point)
			}
		case callRouteConcreteLexical, callRouteBoundary:
			if _, ok := p.relationCatalog.Lookup(cfg.Point(point)); ok {
				return fmt.Errorf("program: non-relation call point %d has a sealed relation target", point)
			}
		default:
			return fmt.Errorf("program: call point %d has invalid route kind %d", point, kind)
		}
	}
	return nil
}

func digestCallRouteRouting(partition callRoutePartition, surface operationplan.CallSurface, snapshot relationRunSnapshot) (callRouteRoutingDigest, error) {
	if partition.owner.Summary.Ref.Kind == ref.KindCFG {
		return callRouteRoutingDigest{}, fmt.Errorf("program: process-local CFG owner cannot identify a route artifact")
	}
	hash := sha256.New()
	writeRouteBytes(hash, []byte("wippy.program.call-route-partition.v1"))
	writeRouteSummaryKey(hash, partition.owner.Summary)
	writeRouteUint64(hash, partition.owner.BodyDigest)
	surfaceDigest := surface.Digest()
	writeRouteBytes(hash, surfaceDigest[:])
	writeRouteUint64(hash, uint64(partition.pointCount))
	for _, site := range surface.Sites() {
		kind, _ := partition.route(site.Point)
		writeRouteUint64(hash, uint64(site.Point))
		_, _ = hash.Write([]byte{byte(kind)})
		if kind != callRouteRelationLexical {
			continue
		}
		target, ok := partition.relationCatalog.Lookup(site.Point)
		identity, found := snapshot.Identity(target.Cell)
		if !ok || !found || identity.Prepared == nil || target.SummaryKey.Ref.Kind == ref.KindCFG || target.LexicalSummaryKey.Ref.Kind == ref.KindCFG {
			return callRouteRoutingDigest{}, fmt.Errorf("program: call route routing digest target identity drift at point %d", site.Point)
		}
		stableBody := identity.Prepared.StableLexicalBodyID()
		writeRouteBytes(hash, stableBody[:])
		writeRouteUint64(hash, identity.BodyDigest)
		writeRouteSummaryKey(hash, target.SummaryKey)
		writeRouteSummaryKey(hash, target.LexicalSummaryKey)
		if target.HasSpecialized {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
		relation, _ := snapshot.Lookup(identity)
		writeRouteShape(hash, relation.Shape())
	}
	var out callRouteRoutingDigest
	copy(out[:], hash.Sum(nil))
	return out, nil
}

type callRouteWriter interface{ Write([]byte) (int, error) }

func writeRouteSummaryKey(writer callRouteWriter, key summary.SummaryKey) {
	_, _ = writer.Write([]byte{byte(key.Ref.Kind)})
	writeRouteUint64(writer, key.Ref.ID)
	writeRouteUint64(writer, uint64(key.Entry.Values))
	writeRouteUint64(writer, uint64(key.Entry.Facts))
	writeRouteUint64(writer, uint64(key.Entry.References))
}

func writeRouteShape(writer callRouteWriter, shape transformer.Shape) {
	writeRouteUint64(writer, uint64(shape.Params))
	writeRouteUint64(writer, uint64(shape.Captures))
	writeRouteUint64(writer, uint64(shape.Globals))
	writeRouteUint64(writer, uint64(shape.Results))
	writeRouteUint64(writer, uint64(shape.HeapTemplates))
}

func writeRouteBytes(writer callRouteWriter, value []byte) {
	writeRouteUint64(writer, uint64(len(value)))
	_, _ = writer.Write(value)
}

func writeRouteUint64(writer callRouteWriter, value uint64) {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	_, _ = writer.Write(raw[:])
}
