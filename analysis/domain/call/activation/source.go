// Package activation owns Call's body-target activation selector.
//
// The package is deliberately a narrow source-domain seam.  Call supplies the
// exact dispatch read and the Body capability; the caller supplies the already
// issued SourceAssembly points and FactorEdge transports.  This package owns
// neither a Batch nor a topology and never creates an Application×Body rule
// product.
package activation

import (
	"sort"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// applicationSemanticVersion is the stable representation version for the
// existing Project Application identity used by engine activation.  The
// digest is still issued by Project.ApplicationID; this version is only the
// representation fence and is not an additional Application vocabulary.
const applicationSemanticVersion uint64 = 1

// Spec supplies the already-declared Call owner and the semantic names for
// this one structural activation family.  Link is explicit so a source cannot
// accidentally bind an equal-looking Call owner from another sealed Link.
type Spec struct {
	Link      *link.Link
	Calls     *callowner.Owner
	Semantic  engine.SemanticKey
	Family    engine.SemanticKey
	Admission engine.SemanticKey
	// Routes is the complete immutable Body selector catalog. It is supplied
	// before Composition.Seal so Source.run never depends on mutable stage
	// state. FactorEdges remain solve-local Entry data.
	Routes []Route
}

// Entry is one canonical static body route. Entries must be supplied in the
// order of Calls.Algebra().Bodies(); each body has one selected plan endpoint.
// FactorEdges are copied into the shared engine plan during Stage and are
// otherwise owned by the caller's SourceAssembly. They are structural-only:
// no Rule prototype, callback, or summary carrier is admitted here.
type Entry struct {
	Body        calldomain.Body
	Target      engine.SemanticKey
	Endpoint    engine.SemanticKey
	FactorEdges []engine.ActivationFactorEdge
}

// Route is the immutable runtime projection of one canonical Call Body.
// FactorEdges are deliberately absent: they contain SourceAssembly-owned
// endpoints and are admitted by Session.Stage for each solve. The route
// identity itself is Link-scoped and can therefore be shared by every solve
// of one compiled analyzer plan.
type Route struct {
	Body     calldomain.Body
	Target   engine.SemanticKey
	Endpoint engine.SemanticKey
}

// route is the runtime selector catalog. It contains only the exact Call Body
// witness and the already-authored semantic pair; it is a canonical slice,
// not a map or a second target index.
type route struct {
	body     calldomain.Body
	target   engine.SemanticKey
	endpoint engine.SemanticKey
}

type roleABI struct {
	key  engine.SemanticKey
	mode uint8
}

const (
	roleImport uint8 = 1 << iota
	roleExport
)

// Source is one immutable Call body activation declaration. Per-assembly
// activation state lives in Session; Source never retains one-shot plans or
// prepared operands and is safe to share across concurrent solves.
type Source struct {
	composition *engine.Composition
	link        *link.Link
	calls       *callowner.Owner
	bodies      calldomain.Bodies
	family      engine.ActivationFamily
	rule        *engine.ActivationRule
	callRead    engine.Read[engine.OrderedCells[calldomain.Value]]
	routes      []route
}

// Session is one solve-local activation transaction. It owns the exact
// SourceAssembly-issued PreparedActivationPlan and finalized ActivationPlan;
// neither can cross into another solve or be retained by Source.
type Session struct {
	source   *Source
	assembly *engine.SourceAssembly
	roles    []engine.SemanticKey
	prepared *engine.PreparedActivationPlan
	plan     *engine.ActivationPlan
}

// Declare records one exact Call read and one output-free activation trigger.
// It does not enumerate bodies or create a source topology; those are staged
// only after the caller has issued the canonical body endpoints.
func Declare(composition *engine.Composition, spec Spec) (*Source, bool) {
	if composition == nil || spec.Link == nil || spec.Calls == nil || spec.Calls.Algebra() == nil ||
		spec.Calls.Link() != spec.Link || !spec.Link.ContentID().Available() ||
		!spec.Semantic.Available() || !spec.Family.Available() || !spec.Admission.Available() ||
		spec.Semantic == spec.Family || spec.Semantic == spec.Admission || spec.Family == spec.Admission {
		return nil, false
	}

	bodies := spec.Calls.Algebra().Bodies()
	routes, routesOK := cloneRoutes(spec.Routes, bodies)
	if !routesOK {
		return nil, false
	}
	source := &Source{
		composition: composition,
		link:        spec.Link,
		calls:       spec.Calls,
		bodies:      bodies,
		routes:      routes,
	}
	family, familyOK := engine.DeclareActivationFamily(composition, spec.Family)
	if !familyOK {
		return nil, false
	}
	rule, ruleOK := engine.DeclareActivationRule(composition, engine.ActivationRuleSpec{
		Semantic:  spec.Semantic,
		Family:    family,
		Inputs:    1,
		Admission: engine.AdmitActivationByTrustedTheorem(spec.Admission),
		Declare: func(rule *engine.ActivationRule) bool {
			input, inputOK := rule.InputAt(0)
			if !inputOK {
				return false
			}
			read, readOK := engine.ReadFrom(rule, input, spec.Calls.ExactRead())
			if readOK {
				source.callRead = read
			}
			return readOK
		},
		Run: source.run,
	})
	if !ruleOK || rule == nil {
		return nil, false
	}
	source.family, source.rule = family, rule
	return source, true
}

func cloneRoutes(input []Route, bodies calldomain.Bodies) ([]route, bool) {
	if len(input) != bodies.Count() {
		return nil, false
	}
	routes := make([]route, len(input))
	for index, candidate := range input {
		body, bodyOK := bodies.At(index)
		if !bodyOK || !candidate.Body.Same(body) || !candidate.Target.Available() ||
			!candidate.Endpoint.Available() || candidate.Target == candidate.Endpoint {
			return nil, false
		}
		routes[index] = route{body: body, target: candidate.Target, endpoint: candidate.Endpoint}
	}
	return routes, true
}

// Stage admits one structural plan row per canonical Call Body into the
// caller-owned open SourceAssembly. The caller must have issued every edge
// endpoint in that same assembly. No private Batch, copied Program topology,
// or caller×callee rule instance is created.
func (source *Source) Stage(assembly *engine.SourceAssembly, entries []Entry) (*Session, bool) {
	if source == nil || source.composition == nil || !source.composition.Sealed() ||
		assembly == nil || source.bodies.Count() == 0 || len(entries) != source.bodies.Count() ||
		len(source.routes) != source.bodies.Count() {
		return nil, false
	}
	plans := make([]engine.ActivationPlanEntry, len(entries))
	var canonicalABI []roleABI
	for index, entry := range entries {
		body, bodyOK := source.bodies.At(index)
		expected := source.routes[index]
		if !bodyOK || !entry.Body.Same(body) || !entry.Body.Same(expected.body) ||
			entry.Target != expected.target || entry.Endpoint != expected.endpoint ||
			!entry.Target.Available() || !entry.Endpoint.Available() || len(entry.FactorEdges) == 0 {
			return nil, false
		}
		edges := append([]engine.ActivationFactorEdge(nil), entry.FactorEdges...)
		entryABI := make([]roleABI, 0)
		for _, edge := range edges {
			if edge.SourceRole.Available() {
				addRoleABI(&entryABI, edge.SourceRole, roleImport)
			}
			if edge.TargetRole.Available() {
				addRoleABI(&entryABI, edge.TargetRole, roleExport)
			}
		}
		sort.Slice(entryABI, func(left, right int) bool { return semanticLess(entryABI[left].key, entryABI[right].key) })
		if index == 0 {
			canonicalABI = entryABI
		} else if !sameRoleABI(canonicalABI, entryABI) {
			return nil, false
		}
		plans[index] = engine.ActivationPlanEntry{
			Target:      entry.Target,
			Endpoint:    entry.Endpoint,
			FactorEdges: edges,
		}
	}
	prepared, preparedOK := engine.StageActivationPlan(assembly, source.composition, source.family, plans)
	if !preparedOK || prepared == nil {
		return nil, false
	}
	roles := make([]engine.SemanticKey, len(canonicalABI))
	for index, role := range canonicalABI {
		roles[index] = role.key
	}
	return &Session{source: source, assembly: assembly, roles: roles, prepared: prepared}, true
}

// Finalize closes this source's static activation catalog through the exact
// SourceAssembly that staged it. Finalization is intentionally separate from
// Stage so the caller can issue all body points and seal one shared source
// transaction first.
func (session *Session) Finalize() (*engine.ActivationPlan, bool) {
	if session == nil || session.source == nil || session.plan != nil || session.prepared == nil ||
		len(session.source.routes) == 0 || session.assembly == nil {
		return nil, false
	}
	plan, planOK := engine.FinalizeActivationPlan(session.assembly, session.prepared)
	if !planOK || plan == nil {
		return nil, false
	}
	session.plan = plan
	return plan, true
}

// Prepare admits the late-bound activation occurrence into the caller's one
// SourceAssembly. The exact trigger rule remains private to this Source; the
// composition root receives only the opaque SourceInstance and cannot submit
// a different activation schema or create a second structural authority.
//
// The returned capability is consumed later by Assembly.ActivationMember,
// after Finalize has produced the shared plan and the trigger instance exists.
func (source *Source) Prepare(assembly *engine.SourceAssembly, occurrence engine.SourceOccurrence, entity engine.SemanticKey) (engine.SourceInstance, bool) {
	if source == nil || source.rule == nil || source.composition == nil || assembly == nil || !entity.Available() {
		return engine.SourceInstance{}, false
	}
	return assembly.PrepareActivation(occurrence, entity, source.rule)
}

// Trigger binds one existing Project base Application to the finalized shared
// plan. Its only data read is Call's exact Application read. Dynamic structural
// roles from the staged FactorEdges receive zero-read ports at this one base;
// there is still no per-body instance. The caller supplies the Assembly-issued
// ActivationBase when the trigger is attached to its canonical body point.
func (session *Session) Trigger(application linkproject.Application, base engine.ActivationBase) (*engine.StructuralInstance, bool) {
	if session == nil || session.source == nil || session.plan == nil {
		return nil, false
	}
	source := session.source
	if source.rule == nil || source.calls == nil || source.link == nil {
		return nil, false
	}
	project := source.link.Project()
	if project == nil {
		return nil, false
	}
	applicationID, applicationIDOK := project.ApplicationID(application)
	applicationKey, applicationKeyOK := engine.NewSemanticKey([32]byte(applicationID), applicationSemanticVersion)
	callKey, callKeyOK := source.calls.Algebra().KeyForApplication(application)
	callRef, callRefOK := source.calls.Locate(callKey)
	if !applicationIDOK || !applicationID.Available() || !applicationKeyOK || !callKeyOK || !callRefOK {
		return nil, false
	}
	ports := make([]*engine.ActivationPort, len(session.roles))
	for index, role := range session.roles {
		port, portOK := engine.NewActivationPort(role, base)
		if !portOK {
			return nil, false
		}
		ports[index] = port
	}
	return engine.NewActivationTrigger(source.rule, applicationKey, session.plan, ports, func(binding *engine.StructuralBinding) bool {
		return engine.StructuralRead(binding, source.callRead, callRef)
	})
}

// run is the only Call-to-plan projection. Top is conservatively connected to
// every canonical body. Non-top values connect only explicitly known Body
// targets; operation/seed targets and the opaque remainder are deliberately
// ignored by this seam because their domains own those relations.
func (source *Source) run(value engine.Activation) bool {
	application, applicationOK := engine.ActivationApplication(value)
	cells, readOK := engine.ActivationReadValue(value, source.callRead)
	if !applicationOK || !readOK || cells.Count() != 1 {
		return false
	}
	fact, present, available := cells.At(0)
	if !available {
		return false
	}
	if !present {
		return true
	}
	return source.visitRoutes(fact, func(item route) bool {
		return engine.Activate(value, application, item.target, item.endpoint)
	})
}

// visitRoutes applies one callback to the body routes selected by a Call
// value. Keeping this as a callback traversal avoids a per-activation route
// slice while making the selector law independently testable.
func (source *Source) visitRoutes(value calldomain.Value, visit func(route) bool) bool {
	if source == nil || visit == nil {
		return false
	}
	if value.IsTop() {
		for _, item := range source.routes {
			if !visit(item) {
				return false
			}
		}
		return true
	}
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, targetOK := value.KnownTargetAt(index)
		if !targetOK {
			return false
		}
		body, bodyOK := target.Body()
		if !bodyOK {
			continue
		}
		item, routeOK := source.routeFor(body)
		if !routeOK || !visit(item) {
			return false
		}
	}
	return true
}

func (source *Source) routeFor(body calldomain.Body) (route, bool) {
	if source == nil || !body.Valid() {
		return route{}, false
	}
	index, indexOK := source.bodies.Index(body)
	if !indexOK || index < 0 || index >= len(source.routes) {
		return route{}, false
	}
	item := source.routes[index]
	return item, item.body.Same(body)
}

func addRoleABI(roles *[]roleABI, candidate engine.SemanticKey, mode uint8) {
	if roles == nil || !candidate.Available() {
		return
	}
	for index := range *roles {
		if (*roles)[index].key == candidate {
			(*roles)[index].mode |= mode
			return
		}
	}
	*roles = append(*roles, roleABI{key: candidate, mode: mode})
}

func sameRoleABI(left, right []roleABI) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func semanticLess(left, right engine.SemanticKey) bool {
	leftDigest, rightDigest := left.Digest(), right.Digest()
	for index := range leftDigest {
		if leftDigest[index] != rightDigest[index] {
			return leftDigest[index] < rightDigest[index]
		}
	}
	return left.Version() < right.Version()
}
