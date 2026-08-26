// Package scalarlower lowers one sealed ingress snapshot into the neutral
// artifact scalar template consumed by the engine. It is deliberately below
// the analysis root and depends only on schema declarations and owner-neutral
// rows; domain composition never enters this boundary.
package scalarlower

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// The v2 identity intentionally bumps the old raw-concatenation formula. The
// canonical identity owner frames both payload parts, so old artifact-role
// semantics must never be accepted as the same role.
const artifactScalarRoleIdentityDomain = "analysis/artifact-scalar-role/v2"
const artifactScalarFactorIdentityDomain = "analysis/artifact-scalar-factor/v1"

type roleBinding struct {
	key    schema.Key
	scalar rows.ArtifactScalarRole
}

type factorBinding struct {
	key    schema.Key
	scalar rows.ArtifactScalarFactor
}

// MountDirectory is immutable Program-template substitution metadata. Rule
// keys and Factor-axis keys stay in distinct planes, so a transfer never
// masquerades as a Rule merely to reach its canonical Factor at mount time.
type MountDirectory struct {
	roles    []roleBinding
	byRule   map[schema.Key]rows.ArtifactScalarRole
	factors  []factorBinding
	byFactor map[schema.Key]rows.ArtifactScalarFactor
}

// Count reports the number of sealed structural roles.
func (directory *MountDirectory) RoleCount() int {
	if directory == nil {
		return 0
	}
	return len(directory.roles)
}

// At returns one role in the deterministic schema-key order retained by the
// directory. It is the mount boundary's read-only enumeration surface.
func (directory *MountDirectory) RoleAt(index int) (schema.Key, rows.ArtifactScalarRole, bool) {
	if directory == nil || index < 0 || index >= len(directory.roles) {
		return "", rows.ArtifactScalarRole{}, false
	}
	binding := directory.roles[index]
	return binding.key, binding.scalar, binding.scalar.Available()
}

// Role resolves one declared structural role by its schema key.
func (directory *MountDirectory) Role(key schema.Key) (rows.ArtifactScalarRole, bool) {
	if directory != nil && key.Available() {
		role, ok := directory.byRule[key]
		return role, ok && role.Available()
	}
	return rows.ArtifactScalarRole{}, false
}

func (directory *MountDirectory) FactorCount() int {
	if directory == nil {
		return 0
	}
	return len(directory.factors)
}

func (directory *MountDirectory) FactorAt(index int) (schema.Key, rows.ArtifactScalarFactor, bool) {
	if directory == nil || index < 0 || index >= len(directory.factors) {
		return "", rows.ArtifactScalarFactor{}, false
	}
	binding := directory.factors[index]
	return binding.key, binding.scalar, binding.scalar.Available()
}

func (directory *MountDirectory) Factor(key schema.Key) (rows.ArtifactScalarFactor, bool) {
	if directory != nil && key.Available() {
		factor, ok := directory.byFactor[key]
		return factor, ok && factor.Available()
	}
	return rows.ArtifactScalarFactor{}, false
}

// Lower is the sole sealed-snapshot-to-engine structural boundary. It runs
// once while publishing the content-addressed cache entry. Stage semantics
// have already been issued into each Program rule occurrence; lowering copies
// that occurrence without consulting or reconstructing its declaration. The plan
// supplies only the closed Factor denominator used by artifact transfers.
func Lower(snapshot *ingress.Snapshot, vocabulary structure.Table, machine schemaissuance.Plan) (*rows.ArtifactScalarTemplate, *MountDirectory, bool) {
	if snapshot == nil || !snapshot.Available() {
		return nil, nil, false
	}
	program := snapshot.Program()
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	bodyCount, bodiesPublished := program.BodyCount()
	transferCount, transfersPublished := program.LocalTransferCount()
	pointCount, pointsPublished := programschema.PointFamily().Count(&program.Frozen, catalog)
	_, pointDecisionsPublished := programschema.PointDecisionFamily().Count(&program.Frozen, catalog)
	edgeCount, edgesPublished := programschema.EnvironmentEdgeFamily().Count(&program.Frozen, catalog)
	_, resetsPublished := programschema.EnvironmentResetFamily().Count(&program.Frozen, catalog)
	regionCount, regionsPublished := programschema.RegionFamily().Count(&program.Frozen, catalog)
	_, regionMembersPublished := programschema.RegionMemberFamily().Count(&program.Frozen, catalog)
	eventCount, eventsPublished := programschema.WTOEventFamily().Count(&program.Frozen, catalog)
	if !program.Available() || !catalogOK || !bodiesPublished || !transfersPublished ||
		!pointsPublished || !pointDecisionsPublished || !edgesPublished || !resetsPublished ||
		!regionsPublished || !regionMembersPublished || !eventsPublished {
		return nil, nil, false
	}
	declaredFactors := make(map[schema.Key]struct{})
	for _, key := range machine.Axes() {
		if !key.Available() {
			return nil, nil, false
		}
		declaredFactors[key] = struct{}{}
	}
	for index := 0; index < transferCount; index++ {
		transfer, transferOK := program.LocalTransferAt(index)
		if !transferOK {
			return nil, nil, false
		}
		for inner := 0; inner < transfer.WritesCount(); inner++ {
			write, writeOK := program.LocalTransferWriteFor(index, inner)
			key, keyOK := write.Key()
			if !writeOK || !keyOK {
				return nil, nil, false
			}
			if _, declared := declaredFactors[key]; !declared {
				return nil, nil, false
			}
		}
	}
	ruleCount, rulesPublished := program.RuleOccurrenceCount()
	if !rulesPublished {
		return nil, nil, false
	}
	usedRules := make(map[schema.Key]struct{}, ruleCount)
	for index := 0; index < ruleCount; index++ {
		placement, placementOK := program.RuleOccurrenceAt(index)
		if !placementOK || !placement.Key().Available() {
			return nil, nil, false
		}
		usedRules[placement.Key()] = struct{}{}
	}
	spec, specOK := rows.NewArtifactScalarSpec(snapshot.ArtifactID(), snapshot.ProgramID(), snapshot.SchemaID(), rows.ArtifactScalarCapacity{
		Roles: len(usedRules), Factors: len(declaredFactors), Points: pointCount, Edges: edgeCount, Transfers: transferCount, Regions: regionCount, Events: eventCount, Rules: ruleCount, Bodies: bodyCount,
	})
	if !specOK || !spec.InstallStageTable(machine.Table()) {
		return nil, nil, false
	}
	directory := &MountDirectory{
		roles: make([]roleBinding, 0, len(usedRules)), byRule: make(map[schema.Key]rows.ArtifactScalarRole, len(usedRules)),
		factors: make([]factorBinding, 0, len(declaredFactors)), byFactor: make(map[schema.Key]rows.ArtifactScalarFactor, len(declaredFactors)),
	}
	ordered := make([]schema.Key, 0, len(usedRules))
	for key := range usedRules {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	artifactID := snapshot.ArtifactID()
	for _, key := range ordered {
		scalar, scalarOK := identity.DeriveContentID(artifactScalarRoleIdentityDomain, artifactID[:], []byte(key))
		if !scalarOK {
			return nil, nil, false
		}
		role, roleOK := spec.DeclareRole(scalar)
		if !roleOK {
			return nil, nil, false
		}
		directory.roles = append(directory.roles, roleBinding{key: key, scalar: role})
		directory.byRule[key] = role
	}
	ordered = ordered[:0]
	for key := range declaredFactors {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	for _, key := range ordered {
		scalar, scalarOK := identity.DeriveContentID(artifactScalarFactorIdentityDomain, artifactID[:], []byte(key))
		if !scalarOK {
			return nil, nil, false
		}
		factor, factorOK := spec.DeclareFactor(scalar)
		if !factorOK {
			return nil, nil, false
		}
		directory.factors = append(directory.factors, factorBinding{key: key, scalar: factor})
		directory.byFactor[key] = factor
	}
	for index := 0; index < pointCount; index++ {
		row, rowOK := programschema.PointFamily().At(&program.Frozen, catalog, index)
		if !rowOK || !row.ID().Available() {
			return nil, nil, false
		}
		point, pointOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: row.ID(), Initial: row.Initial()})
		if !pointOK {
			return nil, nil, false
		}
		decisionOffset, _, decisionsOK := row.DecisionSpan()
		if !decisionsOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.DecisionCount(); inner++ {
			decision, decisionOK := programschema.PointDecisionFamily().At(&program.Frozen, catalog, int(decisionOffset)+inner)
			if !decisionOK || !spec.AddPointDecision(point, decision.ID()) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < edgeCount; index++ {
		row, rowOK := programschema.EnvironmentEdgeFamily().At(&program.Frozen, catalog, index)
		if !rowOK {
			return nil, nil, false
		}
		guard, guarded := row.GuardID()
		decision, decisionOK := row.DecisionID()
		truth, truthOK := row.Truth()
		mu, hasMu := row.MuPathID()
		reset, hasReset := row.ResetDigest()
		if guarded != decisionOK || guarded != truthOK || hasMu != hasReset {
			return nil, nil, false
		}
		arm := rows.ArtifactStructuralArm(row.Arm())
		if !arm.Valid() {
			return nil, nil, false
		}
		edge, edgeOK := spec.AddEdge(rows.ArtifactScalarEdge{ID: row.ID(), From: row.Departure(), To: row.To(), Route: row.RouteID(), Guard: guard, Decision: decision, Component: row.ComponentID(), Mu: mu, Reset: reset, Arm: arm, Guarded: guarded, Truth: truth, HasReset: hasReset})
		if !edgeOK {
			return nil, nil, false
		}
		resetOffset, _, resetSpanOK := row.ResetSpan()
		if !resetSpanOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.ResetCount(); inner++ {
			resetPoint, resetOK := programschema.EnvironmentResetFamily().At(&program.Frozen, catalog, int(resetOffset)+inner)
			if !resetOK || !spec.AddEdgeReset(edge, resetPoint.ID()) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < transferCount; index++ {
		row, rowOK := program.LocalTransferAt(index)
		if !rowOK {
			return nil, nil, false
		}
		transfer, transferOK := spec.AddTransfer(rows.ArtifactScalarTransfer{ID: row.ID(), From: row.From(), To: row.To(), Full: row.Full()})
		if !transferOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.WritesCount(); inner++ {
			write, writeOK := program.LocalTransferWriteFor(index, inner)
			key, keyOK := write.Key()
			factor, factorOK := directory.Factor(key)
			if !writeOK || !keyOK || !factorOK || !spec.AddTransferFactor(transfer, factor) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < regionCount; index++ {
		row, rowOK := programschema.RegionFamily().At(&program.Frozen, catalog, index)
		if !rowOK {
			return nil, nil, false
		}
		memberOffset, _, membersOK := row.MemberSpan()
		head, headOK := programschema.RegionMemberFamily().At(&program.Frozen, catalog, int(memberOffset))
		if !membersOK || !headOK {
			return nil, nil, false
		}
		region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: row.ID(), Head: head.ID(), Parent: row.ParentID(), Cyclic: row.Cyclic()})
		if !regionOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.MemberCount(); inner++ {
			member, memberOK := programschema.RegionMemberFamily().At(&program.Frozen, catalog, int(memberOffset)+inner)
			if !memberOK || !spec.AddRegionMember(region, member.ID()) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < eventCount; index++ {
		row, rowOK := programschema.WTOEventFamily().At(&program.Frozen, catalog, index)
		if !rowOK {
			return nil, nil, false
		}
		kind := rows.ArtifactEventKind(row.Kind())
		if kind < rows.ArtifactEventEnter || kind > rows.ArtifactEventExit || !spec.AddEvent(rows.ArtifactScalarEvent{Kind: kind, Region: row.RegionID(), Point: row.PointID()}) {
			return nil, nil, false
		}
	}
	for index := 0; index < ruleCount; index++ {
		row, rowOK := program.RuleOccurrenceAt(index)
		if !rowOK {
			return nil, nil, false
		}
		role, roleOK := directory.Role(row.Key())
		route, hasRoute := row.PredecessorRoute()
		native, nativeOK := row.Native()
		// The candidate row was resolved by issuance. Only its ordinal travels:
		// the row space it indexes is sealed in the rule's compiled plan, so
		// restating it here would give one address two spellings.
		candidate, sourcePresent := row.Source()
		inputCount := row.InputPointCount()
		if inputCount < 0 || inputCount > 6 {
			return nil, nil, false
		}
		var inputs [6]identity.ContentID
		for inputIndex := 0; inputIndex < inputCount; inputIndex++ {
			input, inputOK := row.InputPointAt(inputIndex)
			if !inputOK {
				return nil, nil, false
			}
			inputs[inputIndex] = input
		}
		if !roleOK || !row.Available() || !nativeOK || hasRoute != route.Available() ||
			!spec.AddRule(rows.ArtifactScalarRule{Role: role, Stage: row.Stage(), Point: row.PointID(), Inputs: inputs, InputCount: uint8(inputCount), ID: occurrenceID(program, row), Route: route.ID, RoutePoint: route.Point, Native: native, Source: candidate.Ordinal, SourcePresent: sourcePresent}) {
			return nil, nil, false
		}
	}
	for index := 0; index < bodyCount; index++ {
		row, rowOK := program.BodyAt(index)
		if !rowOK || !row.Available() {
			return nil, nil, false
		}
		body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: row.ID()})
		if !bodyOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.EntryCount(); inner++ {
			entry, entryOK := program.BodyEntryFor(index, inner)
			point, pointOK := entry.PointID(), entryOK
			if !pointOK || !spec.AddBodyEntry(body, point) {
				return nil, nil, false
			}
		}
		seen := make(map[identity.ContentID]struct{})
		outcomeOffset, outcomeCount, outcomesOK := row.OutcomeSpan()
		if !outcomesOK {
			return nil, nil, false
		}
		for outcomeIndex := uint32(0); outcomeIndex < outcomeCount; outcomeIndex++ {
			outcome, outcomeOK := program.BodyOutcomeFor(index, int(outcomeIndex))
			if !outcomeOK || !acceptedOutcome(vocabulary, outcome.Kind()) {
				if !outcomeOK {
					return nil, nil, false
				}
				continue
			}
			_, pointCount, pointsOK := outcome.PointSpan()
			if !pointsOK {
				return nil, nil, false
			}
			for pointIndex := uint32(0); pointIndex < pointCount; pointIndex++ {
				child, childOK := program.OutcomePointFor(int(outcomeOffset+outcomeIndex), int(pointIndex))
				point := child.PointID()
				if !childOK || child.OutcomeID() != outcome.ID() || !point.Available() {
					return nil, nil, false
				}
				if _, duplicate := seen[point]; duplicate {
					continue
				}
				seen[point] = struct{}{}
				if !spec.AddBodyExit(body, point) {
					return nil, nil, false
				}
			}
		}
	}
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	return template, directory, templateOK
}

func occurrenceID(program programschema.Program, placement programschema.RuleOccurrence) identity.ContentID {
	ordinal, ok := placement.Occurrence()
	if !ok {
		return identity.ContentID{}
	}
	row, ok := program.OccurrenceAt(int(ordinal))
	if !ok {
		return identity.ContentID{}
	}
	return row.ID()
}

func acceptedOutcome(vocabulary structure.Table, kind programschema.OutcomeKind) bool {
	member, ok := vocabulary.At(structure.CategoryOutcome, uint16(kind))
	return ok && member.Accepted()
}
