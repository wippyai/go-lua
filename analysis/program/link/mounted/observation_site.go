package mounted

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// BranchProducer is one execution producer of a branch observation and the
// geometry that relates it to the observation's evidence. Point is the
// execution key a result is queried at; Anchor is the program-issued base
// evidence point the result is recorded against. The two coincide when the
// producer executes at the base point, and are related by a chain of sealed
// full-environment transfers otherwise.
type BranchProducer struct {
	Key        schema.Key
	Occurrence identity.ContentID
	Point      identity.ContentID
	Anchor     identity.ContentID
	ValueID    identity.ContentID
}

func (producer BranchProducer) Available() bool {
	return producer.Key.Available() && producer.Occurrence.Available() &&
		producer.Point.Available() && producer.Anchor.Available() && producer.ValueID.Available()
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
	case left.Key < right.Key:
		return -1
	case left.Key > right.Key:
		return 1
	}
	return 0
}

// ObservationSite is one place a mounted result is observed: the mount, the
// artifact-local observation identity, the kind of observation, and the source
// span it reports at. A produced-value site - a branch condition or a type
// conformance subject - additionally carries the geometry of the occurrences
// that produce its value; a static site carries none, because its evidence is
// static and needs no execution key.
type ObservationSite struct {
	Mount     identity.ContentID
	Local     identity.ContentID
	Kind      structure.DiagnosticObservationKind
	Location  programsource.Span
	ValueID   identity.ContentID
	producers []BranchProducer
}

func (site ObservationSite) Available() bool {
	if !site.Mount.Available() || !site.Local.Available() || !validSiteSpan(site.Location) {
		return false
	}
	switch site.Kind {
	case structure.DiagnosticObservationBranchCondition:
		return site.ValueID.Available() && site.branchGeometryAvailable()
	case structure.DiagnosticObservationTypeConformance:
		return site.ValueID.Available() && site.producerGeometryAvailable()
	case structure.DiagnosticObservationTypeReferenceUnresolved, structure.DiagnosticObservationValueReferenceUnresolved:
		return !site.ValueID.Available() && len(site.producers) == 0
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

// producerGeometryAvailable is the geometry every produced-value site is
// admitted under: at least one producer, in canonical order, and no execution
// key claimed twice. One measured value may be established on several paths,
// so this law does not fix how many producers a base witness carries.
func (site ObservationSite) producerGeometryAvailable() bool {
	if len(site.producers) == 0 {
		return false
	}
	executions := make(map[identity.ContentID]struct{}, len(site.producers))
	for index, producer := range site.producers {
		if !producer.Available() || index != 0 && compareBranchProducer(site.producers[index-1], producer) >= 0 {
			return false
		}
		if _, duplicate := executions[producer.Point]; duplicate {
			return false
		}
		executions[producer.Point] = struct{}{}
	}
	return true
}

// branchGeometryAvailable adds the branch law: one producer per base witness.
// A second producer sharing one base witness would make a branch's anchor
// ambiguous, so the site fails closed rather than selecting one.
func (site ObservationSite) branchGeometryAvailable() bool {
	if !site.producerGeometryAvailable() {
		return false
	}
	anchors := make(map[identity.ContentID]struct{}, len(site.producers))
	for _, producer := range site.producers {
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

// SealObservationSites derives the census from the sealed ingress snapshots
// and the Boundary that owns the two Link relations a branch site needs: the
// mounted span-to-Value and semantic-occurrence-to-Value substitutions.
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
	var anchors map[identity.ContentID]identity.ContentID
	rows := make([]ObservationSite, 0)
	for index := 0; index < mount.Snapshot.DiagnosticObservationCount(); index++ {
		observation, observationOK := mount.Snapshot.DiagnosticObservationAt(index)
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
			producers, evidence, valueID, resolvedOK := branchProducers(values, producersByValue, mount, observation)
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
			if anchors == nil {
				var anchorsOK bool
				anchors, anchorsOK = localStageAnchors(mount.Snapshot)
				if !anchorsOK {
					return nil, false
				}
			}
			if !anchorBranchProducers(producers, evidence, anchors) {
				return nil, false
			}
			site.ValueID = valueID
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
		case structure.DiagnosticObservationTypeConformance:
			conformance, conformanceOK := observation.TypeConformance()
			if !conformanceOK || !conformance.OwnerID().Available() || !conformance.MeasuredValueID().Available() ||
				!conformance.DeclaredStaticTypeID().Available() || !conformance.SpanID().Available() {
				return nil, false
			}
			valueID, valueOK := conformanceValueID(values, mount, conformance.MeasuredValueID(), conformance.SpanID())
			evidence, evidenceOK := conformance.EvidencePoints()
			if !valueOK || !evidenceOK || len(evidence) == 0 {
				return nil, false
			}
			if producersByValue == nil {
				var producersOK bool
				producersByValue, producersOK = mountedValueProducers(values, mount)
				if !producersOK {
					return nil, false
				}
			}
			producers := append([]BranchProducer(nil), producersByValue[valueID]...)
			// A conformance subject whose value has no producing rule role at
			// this target is unobservable rather than malformed, exactly as a
			// branch condition is: it contributes no site.
			if len(producers) == 0 {
				continue
			}
			if anchors == nil {
				var anchorsOK bool
				anchors, anchorsOK = localStageAnchors(mount.Snapshot)
				if !anchorsOK {
					return nil, false
				}
			}
			if !anchorBranchProducers(producers, evidence, anchors) {
				return nil, false
			}
			site.producers = producers
			site.ValueID = valueID
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
	observation ingress.DiagnosticObservation,
) ([]BranchProducer, []identity.ContentID, identity.ContentID, bool) {
	branch, branchOK := observation.BranchCondition()
	if !branchOK {
		return nil, nil, identity.ContentID{}, false
	}
	value, valueOK := values.ForMountedSpan(mount.ModuleKey, branch.ValueSpanID())
	valueID, valueIDOK := values.ID(value)
	evidence, evidenceOK := branch.EvidencePoints()
	if !valueOK || !valueIDOK || !evidenceOK || len(evidence) == 0 {
		return nil, nil, identity.ContentID{}, false
	}
	producers := append([]BranchProducer(nil), producersByValue[valueID]...)
	coordinate := valueID
	for _, producer := range producers {
		if producer.ValueID.Available() {
			coordinate = producer.ValueID
			break
		}
	}
	return producers, evidence, coordinate, true
}

// conformanceValueID rebinds the measured value through this mount. The row
// carries the value's semantic occurrence, so the substitution is the same one
// a branch condition takes; the span relation answers the values a target
// binds by operator span rather than by occurrence.
func conformanceValueID(values linkboundary.Values, mount Mount, measuredID, spanID identity.ContentID) (identity.ContentID, bool) {
	if mount.Snapshot == nil || !measuredID.Available() || !spanID.Available() {
		return identity.ContentID{}, false
	}
	value, valueOK := values.ForMountedSemantic(mount.ModuleKey, measuredID)
	if !valueOK {
		value, valueOK = values.ForMountedSpan(mount.ModuleKey, spanID)
	}
	if !valueOK {
		return identity.ContentID{}, false
	}
	return values.ID(value)
}

// anchorBranchProducers binds each producer to the base evidence point it
// reports against and freezes the result in canonical order. Every base
// witness must be reached: a row whose producers leave one evidence point
// unanchored is broken geometry rather than a row to read partially.
func anchorBranchProducers(producers []BranchProducer, evidence []identity.ContentID, stages map[identity.ContentID]identity.ContentID) bool {
	anchors := make(map[identity.ContentID]struct{}, len(producers))
	for index := range producers {
		anchor, anchorOK := evidenceAnchor(evidence, producers[index].Point, stages)
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
	if mount.Snapshot == nil {
		return nil, false
	}
	program := mount.Snapshot.Program()
	ruleCount, rulePublished := program.RuleOccurrenceCount()
	if !rulePublished {
		return nil, false
	}
	producers := make(map[identity.ContentID][]BranchProducer)
	for ruleIndex := 0; ruleIndex < ruleCount; ruleIndex++ {
		rule, ruleOK := program.RuleOccurrenceAt(ruleIndex)
		ordinal, ordinalOK := rule.Occurrence()
		occurrence, occurrenceOK := program.OccurrenceAt(int(ordinal))
		if !ruleOK || !ordinalOK || !occurrenceOK || !rule.Key().Available() || !occurrence.ID().Available() {
			return nil, false
		}
		outputID := occurrence.ID()
		value, valueOK := values.ForMountedSemantic(mount.ModuleKey, outputID)
		if programschema.SpanResultOccurrence(occurrence.Kind()) {
			value, valueOK = values.ForMountedSpan(mount.ModuleKey, outputID)
		}
		point := rule.PointID()
		if !valueOK {
			continue
		}
		valueID, valueIDOK := values.ID(value)
		if !valueIDOK || !point.Available() || !rule.Key().Available() {
			return nil, false
		}
		candidate := BranchProducer{Key: rule.Key(), Occurrence: occurrence.ID(), Point: point, ValueID: valueID}
		duplicate := false
		for _, prior := range producers[valueID] {
			if prior.Key == candidate.Key && prior.Occurrence == candidate.Occurrence && prior.Point == candidate.Point {
				duplicate = true
				break
			}
		}
		if !duplicate {
			producers[valueID] = append(producers[valueID], candidate)
		}
	}
	return producers, true
}

// localStageAnchors folds the Program-issued local-transfer rows into their
// structural stage sources. A full transfer is the unique source when present.
// Without one, factor transfers establish a source only when they unanimously
// name it; conflicting partial sources remain unanchored and therefore fail
// closed if an observation tries to cross that stage.
func localStageAnchors(snapshot *ingress.Snapshot) (map[identity.ContentID]identity.ContentID, bool) {
	if snapshot == nil {
		return nil, false
	}
	program := snapshot.Program()
	transferCount, transfersPublished := program.LocalTransferCount()
	if !program.Available() || !transfersPublished {
		return nil, false
	}
	full := make(map[identity.ContentID]identity.ContentID)
	partial := make(map[identity.ContentID]identity.ContentID)
	conflicted := make(map[identity.ContentID]struct{})
	for index := 0; index < transferCount; index++ {
		edge, edgeOK := program.LocalTransferAt(index)
		if !edgeOK || !edge.ID().Available() {
			return nil, false
		}
		to, from := edge.To(), edge.From()
		if edge.Full() {
			if _, duplicate := full[to]; duplicate {
				return nil, false
			}
			full[to] = from
			continue
		}
		if prior, present := partial[to]; present && prior != from {
			conflicted[to] = struct{}{}
			continue
		}
		partial[to] = from
	}
	anchors := make(map[identity.ContentID]identity.ContentID, len(full)+len(partial))
	for to, from := range partial {
		if _, conflict := conflicted[to]; !conflict {
			anchors[to] = from
		}
	}
	for to, from := range full {
		anchors[to] = from
	}
	return anchors, true
}

// evidenceAnchor walks one producer's execution point back to the base
// evidence point it reports against. The producer either executes at a base
// point directly, or reaches it through an acyclic chain of full-environment
// transfers.
func evidenceAnchor(evidence []identity.ContentID, execution identity.ContentID, stages map[identity.ContentID]identity.ContentID) (identity.ContentID, bool) {
	if !execution.Available() || len(evidence) == 0 {
		return identity.ContentID{}, false
	}
	for _, point := range evidence {
		if execution == point {
			return point, true
		}
	}
	seen := make(map[identity.ContentID]struct{}, len(stages))
	current := execution
	for steps := 0; steps <= len(stages); steps++ {
		if _, duplicate := seen[current]; duplicate {
			return identity.ContentID{}, false
		}
		seen[current] = struct{}{}
		from, found := stages[current]
		if !found || !from.Available() || from == current {
			return identity.ContentID{}, false
		}
		current = from
		for _, point := range evidence {
			if current == point {
				return point, true
			}
		}
	}
	return identity.ContentID{}, false
}

func validSiteSpan(span programsource.Span) bool {
	if span.File == "" || span.StartLine == 0 || span.StartCol == 0 {
		return false
	}
	_, ok := programsource.CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	return ok
}
