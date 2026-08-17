package mounted

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/program/target"
)

// BranchProducer is one execution producer of a branch observation and the
// geometry that relates it to the observation's evidence. Point is the
// execution key a result is queried at; Anchor is the program-issued base
// evidence point the result is recorded against. The two coincide when the
// producer executes at the base point, and are related by a chain of sealed
// full-environment transfers otherwise.
type BranchProducer struct {
	Role       programartifact.RuleRole
	Occurrence identity.ContentID
	Point      identity.ContentID
	Anchor     identity.ContentID
}

func (producer BranchProducer) Available() bool {
	return mountedRuleRole(producer.Role) && producer.Occurrence.Available() &&
		producer.Point.Available() && producer.Anchor.Available()
}

func compareBranchProducer(left, right BranchProducer) int {
	if order := bytes.Compare(left.Point[:], right.Point[:]); order != 0 {
		return order
	}
	if order := bytes.Compare(left.Anchor[:], right.Anchor[:]); order != 0 {
		return order
	}
	if order := bytes.Compare(left.Occurrence[:], right.Occurrence[:]); order != 0 {
		return order
	}
	switch {
	case left.Role < right.Role:
		return -1
	case left.Role > right.Role:
		return 1
	}
	return 0
}

// ObservationSite is one place a mounted result is observed: the mount, the
// artifact-local observation identity, the kind of observation, and the source
// span it reports at. A branch site additionally carries its producer geometry;
// the other kinds carry none, because their evidence is static and needs no
// execution key.
type ObservationSite struct {
	Mount     identity.ContentID
	Local     identity.ContentID
	Kind      structure.DiagnosticObservationKind
	Location  programsource.Span
	producers []BranchProducer
}

func (site ObservationSite) Available() bool {
	if !site.Mount.Available() || !site.Local.Available() || !validSiteSpan(site.Location) {
		return false
	}
	switch site.Kind {
	case structure.DiagnosticObservationBranchCondition:
		return site.branchGeometryAvailable()
	case structure.DiagnosticObservationTypeReferenceUnresolved, structure.DiagnosticObservationValueReferenceUnresolved:
		return len(site.producers) == 0
	default:
		return false
	}
}

// ProducerCount and ProducerAt expose the anchor-to-execution mapping of a
// branch site in canonical order.
func (site ObservationSite) ProducerCount() int { return len(site.producers) }

func (site ObservationSite) ProducerAt(index int) (BranchProducer, bool) {
	if index < 0 || index >= len(site.producers) {
		return BranchProducer{}, false
	}
	return site.producers[index], true
}

// branchGeometryAvailable states the bijection a branch site is admitted
// under: at least one producer, and no execution key or anchor used twice. A
// second producer sharing one base witness would make the anchor ambiguous, so
// the site fails closed rather than selecting one.
func (site ObservationSite) branchGeometryAvailable() bool {
	if len(site.producers) == 0 {
		return false
	}
	executions := make(map[identity.ContentID]struct{}, len(site.producers))
	anchors := make(map[identity.ContentID]struct{}, len(site.producers))
	for index, producer := range site.producers {
		if !producer.Available() || index != 0 && compareBranchProducer(site.producers[index-1], producer) >= 0 {
			return false
		}
		if _, duplicate := executions[producer.Point]; duplicate {
			return false
		}
		executions[producer.Point] = struct{}{}
		if _, duplicate := anchors[producer.Anchor]; duplicate {
			return false
		}
		anchors[producer.Anchor] = struct{}{}
	}
	return true
}

func compareObservationSite(left, right ObservationSite) int {
	if order := bytes.Compare(left.Mount[:], right.Mount[:]); order != 0 {
		return order
	}
	return bytes.Compare(left.Local[:], right.Local[:])
}

// ObservationSites is the frozen observation-site census of a sealed Link. It
// is ordered by the bytes of the site key alone, so the census a consumer reads
// is a function of the sealed content and not of the order the artifacts were
// mounted, indexed, or published in.
type ObservationSites struct {
	rows   []ObservationSite
	sealed bool
}

func (census ObservationSites) Available() bool { return census.sealed }

func (census ObservationSites) Count() int {
	if !census.Available() {
		return 0
	}
	return len(census.rows)
}

func (census ObservationSites) At(index int) (ObservationSite, bool) {
	if !census.Available() || index < 0 || index >= len(census.rows) {
		return ObservationSite{}, false
	}
	return census.rows[index], true
}

// SealObservationSites derives the census from the placed artifacts and the
// Boundary that owns the two Link relations a branch site needs: the mounted
// span-to-Value and semantic-occurrence-to-Value substitutions. Everything
// else -- the observation rows, the producing rule occurrences, the transfer
// chain an anchor is recovered through -- is read from the artifact.
func SealObservationSites(boundary *linkboundary.Component, mounts []Mount) (ObservationSites, bool) {
	if boundary == nil || !mountsAvailable(mounts) {
		return ObservationSites{}, false
	}
	contract, contractOK := boundary.Target()
	if !contractOK || contract == nil {
		return ObservationSites{}, false
	}
	values := boundary.Values()
	rows := make([]ObservationSite, 0)
	for _, mount := range mounts {
		mountRows, mountOK := mountObservationSites(values, contract, mount)
		if !mountOK {
			return ObservationSites{}, false
		}
		rows = append(rows, mountRows...)
	}
	sort.Slice(rows, func(left, right int) bool {
		return compareObservationSite(rows[left], rows[right]) < 0
	})
	for index, row := range rows {
		if !row.Available() || index != 0 && compareObservationSite(rows[index-1], row) >= 0 {
			return ObservationSites{}, false
		}
	}
	return ObservationSites{rows: rows, sealed: true}, true
}

func mountObservationSites(values linkboundary.Values, contract *target.Contract, mount Mount) ([]ObservationSite, bool) {
	var producersByValue map[identity.ContentID][]BranchProducer
	var transfers map[identity.ContentID]programartifact.LocalTransfer
	rows := make([]ObservationSite, 0)
	for index := 0; index < mount.Artifact.DiagnosticObservationCount(); index++ {
		observation, observationOK := mount.Artifact.DiagnosticObservationAt(index)
		if !observationOK || !observation.Available() {
			return nil, false
		}
		location, locationOK := observation.Location()
		if !locationOK {
			return nil, false
		}
		site := ObservationSite{Mount: mount.ModuleKey, Local: observation.ID(), Kind: observation.Kind(), Location: location}
		switch observation.Kind() {
		case structure.DiagnosticObservationBranchCondition:
			if producersByValue == nil {
				var producersOK bool
				producersByValue, producersOK = mountedValueProducers(values, mount)
				if !producersOK {
					return nil, false
				}
			}
			producers, evidence, resolvedOK := branchProducers(values, producersByValue, mount, observation)
			if !resolvedOK {
				return nil, false
			}
			// A branch is a complete semantic fact even where this target
			// declares no producing rule role for its value. Such a branch is
			// unobservable rather than malformed, so it contributes no site,
			// and the transfer index it would have needed stays unbuilt.
			if len(producers) == 0 {
				continue
			}
			if transfers == nil {
				var transfersOK bool
				transfers, transfersOK = fullEnvironmentTransfers(mount.Artifact)
				if !transfersOK {
					return nil, false
				}
			}
			if !anchorBranchProducers(producers, evidence, transfers) {
				return nil, false
			}
			site.producers = producers
		case structure.DiagnosticObservationTypeReferenceUnresolved:
			unresolved, unresolvedOK := observation.UnresolvedTypeReference()
			path, pathOK := unresolved.Path()
			if !unresolvedOK || !pathOK || len(path) == 0 || !unresolved.StaticReferenceID().Available() {
				return nil, false
			}
		case structure.DiagnosticObservationValueReferenceUnresolved:
			unresolved, unresolvedOK := observation.UnresolvedValueReference()
			name, nameOK := unresolved.Name()
			if !unresolvedOK || !nameOK || name == "" || !unresolved.ReadID().Available() || !unresolved.CellID().Available() {
				return nil, false
			}
			// The program proves a binder-implicit global candidate; the Link
			// owns the absence judgment. A configured initial binding answers
			// the candidate, so there is nothing left to observe.
			if _, _, _, _, configured := contract.InitialBinding(name); configured {
				continue
			}
		default:
			return nil, false
		}
		if !site.Available() {
			return nil, false
		}
		rows = append(rows, site)
	}
	return rows, true
}

// branchProducers resolves one branch observation's execution producers and the
// base evidence points they must anchor to. An empty producer set is a valid
// answer: the branch's value has no producing rule role at this target.
func branchProducers(
	values linkboundary.Values,
	producersByValue map[identity.ContentID][]BranchProducer,
	mount Mount,
	observation programartifact.DiagnosticObservationRow,
) ([]BranchProducer, []identity.ContentID, bool) {
	branch, branchOK := observation.BranchCondition()
	if !branchOK {
		return nil, nil, false
	}
	value, valueOK := values.ForMountedSpan(mount.ModuleKey, branch.ValueSpanID())
	valueID, valueIDOK := values.ID(value)
	evidence, evidenceOK := branch.EvidencePoints()
	if !valueOK || !valueIDOK || !evidenceOK || len(evidence) == 0 {
		return nil, nil, false
	}
	return append([]BranchProducer(nil), producersByValue[valueID]...), evidence, true
}

// anchorBranchProducers binds each producer to the base evidence point it
// reports against and freezes the result in canonical order. The evidence
// points and the anchors form a bijection: every base witness of the branch is
// claimed exactly once, so a producer that shares another's witness is broken
// geometry rather than a row to choose between.
func anchorBranchProducers(producers []BranchProducer, evidence []identity.ContentID, transfers map[identity.ContentID]programartifact.LocalTransfer) bool {
	anchors := make(map[identity.ContentID]struct{}, len(producers))
	for index := range producers {
		anchor, anchorOK := evidenceAnchor(evidence, producers[index].Point, transfers)
		if !anchorOK {
			return false
		}
		producers[index].Anchor = anchor
		anchors[anchor] = struct{}{}
	}
	sort.Slice(producers, func(left, right int) bool {
		return compareBranchProducer(producers[left], producers[right]) < 0
	})
	return len(anchors) == len(evidence)
}

// mountedValueProducers builds the mounted Value-to-producer inverse once per
// artifact. Boundary owns both substitutions it reads: a rule whose result is
// an operator span is rebound through the span relation, and every other value
// rule through the semantic-occurrence relation.
func mountedValueProducers(values linkboundary.Values, mount Mount) (map[identity.ContentID][]BranchProducer, bool) {
	producers := make(map[identity.ContentID][]BranchProducer)
	for roleIndex := 0; roleIndex < programartifact.MountedRuleRoleCount(); roleIndex++ {
		role, roleOK := programartifact.MountedRuleRoleAt(roleIndex)
		if !roleOK {
			return nil, false
		}
		for ruleIndex := 0; ruleIndex < mount.Artifact.RuleOccurrenceCount(role); ruleIndex++ {
			rule, ruleOK := mount.Artifact.RuleOccurrenceAt(role, ruleIndex)
			if !ruleOK || !rule.Available() {
				return nil, false
			}
			if rule.OutputKind() != programartifact.RuleOutputValue {
				continue
			}
			outputID, outputOK := rule.OutputSemanticID()
			if !outputOK {
				continue
			}
			value, valueOK := values.ForMountedSemantic(mount.ModuleKey, outputID)
			if spanResultRole(role) {
				value, valueOK = values.ForMountedSpan(mount.ModuleKey, outputID)
			}
			point, pointOK := rule.PointAt(0)
			if !valueOK {
				continue
			}
			valueID, valueIDOK := values.ID(value)
			if !valueIDOK || !pointOK || !point.Available() || rule.PointCount() != 1 {
				return nil, false
			}
			candidate := BranchProducer{Role: role, Occurrence: rule.ID(), Point: point}
			duplicate := false
			for _, prior := range producers[valueID] {
				if prior.Role == candidate.Role && prior.Occurrence == candidate.Occurrence && prior.Point == candidate.Point {
					duplicate = true
					break
				}
			}
			if !duplicate {
				producers[valueID] = append(producers[valueID], candidate)
			}
		}
	}
	return producers, true
}

// spanResultRole names the rule roles whose result identity is the operator's
// own program-owned span rather than a semantic occurrence.
func spanResultRole(role programartifact.RuleRole) bool {
	return role == programartifact.RuleRoleValueBinaryArithmetic ||
		role == programartifact.RuleRoleValueBinaryEquality ||
		role == programartifact.RuleRoleValueBinaryOrder
}

// fullEnvironmentTransfers indexes the sealed full-environment transfers of one
// artifact by destination. Two full rows reaching one execution point make the
// anchor ambiguous even when their sources agree, so admission rejects the pair
// instead of choosing. Factor transports carry only named factors and are
// outside this bridge by design.
func fullEnvironmentTransfers(artifact *programartifact.Artifact) (map[identity.ContentID]programartifact.LocalTransfer, bool) {
	transfers := make(map[identity.ContentID]programartifact.LocalTransfer, artifact.LocalTransferCount())
	for index := 0; index < artifact.LocalTransferCount(); index++ {
		edge, edgeOK := artifact.LocalTransferAt(index)
		if !edgeOK || !edge.Available() {
			return nil, false
		}
		if !edge.FullEnvironment() {
			continue
		}
		if _, duplicate := transfers[edge.To()]; duplicate {
			return nil, false
		}
		transfers[edge.To()] = edge
	}
	return transfers, true
}

// evidenceAnchor walks one producer's execution point back to the base
// evidence point it reports against. The producer either executes at a base
// point directly, or reaches it through an acyclic chain of full-environment
// transfers.
func evidenceAnchor(evidence []identity.ContentID, execution identity.ContentID, transfers map[identity.ContentID]programartifact.LocalTransfer) (identity.ContentID, bool) {
	if !execution.Available() || len(evidence) == 0 {
		return identity.ContentID{}, false
	}
	for _, point := range evidence {
		if execution == point {
			return point, true
		}
	}
	seen := make(map[identity.ContentID]struct{}, len(transfers))
	current := execution
	for steps := 0; steps <= len(transfers); steps++ {
		if _, duplicate := seen[current]; duplicate {
			return identity.ContentID{}, false
		}
		seen[current] = struct{}{}
		edge, found := transfers[current]
		if !found || !edge.Available() || !edge.FullEnvironment() || edge.To() != current {
			return identity.ContentID{}, false
		}
		current = edge.From()
		for _, point := range evidence {
			if current == point {
				return point, true
			}
		}
	}
	return identity.ContentID{}, false
}

func mountedRuleRole(role programartifact.RuleRole) bool {
	for index := 0; index < programartifact.MountedRuleRoleCount(); index++ {
		candidate, ok := programartifact.MountedRuleRoleAt(index)
		if !ok {
			return false
		}
		if candidate == role {
			return true
		}
	}
	return false
}

func validSiteSpan(span programsource.Span) bool {
	if span.File == "" || span.StartLine == 0 || span.StartCol == 0 {
		return false
	}
	_, ok := programsource.CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	return ok
}
