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
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// The v2 identity intentionally bumps the old raw-concatenation formula. The
// canonical identity owner frames both payload parts, so old artifact-role
// semantics must never be accepted as the same role.
const artifactScalarRoleIdentityDomain = "analysis/artifact-scalar-role/v2"

type roleBinding struct {
	key    schema.Key
	scalar rows.ArtifactScalarRole
}

// RoleDirectory is immutable Program-template metadata. It contains no
// Link-local capability and is shared with the template cache.
type RoleDirectory struct {
	bindings []roleBinding
	byKey    map[schema.Key]rows.ArtifactScalarRole
}

// Count reports the number of sealed structural roles.
func (directory *RoleDirectory) Count() int {
	if directory == nil {
		return 0
	}
	return len(directory.bindings)
}

// At returns one role in the deterministic schema-key order retained by the
// directory. It is the mount boundary's read-only enumeration surface.
func (directory *RoleDirectory) At(index int) (schema.Key, rows.ArtifactScalarRole, bool) {
	if directory == nil || index < 0 || index >= len(directory.bindings) {
		return "", rows.ArtifactScalarRole{}, false
	}
	binding := directory.bindings[index]
	return binding.key, binding.scalar, binding.scalar.Available()
}

// Role resolves one declared structural role by its schema key.
func (directory *RoleDirectory) Role(key schema.Key) (rows.ArtifactScalarRole, bool) {
	if directory != nil && key.Available() {
		role, ok := directory.byKey[key]
		return role, ok && role.Available()
	}
	return rows.ArtifactScalarRole{}, false
}

// Lower is the sole sealed-snapshot-to-engine structural boundary. It runs
// once while publishing the content-addressed cache entry. Stage admission is
// projected from the sealed schema declaration table, not from a composed
// domain registry.
func Lower(snapshot *ingress.Snapshot, vocabulary structure.Table) (*rows.ArtifactScalarTemplate, *RoleDirectory, bool) {
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
	usedKeys := make(map[schema.Key]struct{})
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
			usedKeys[key] = struct{}{}
		}
	}
	ruleCount, rulesPublished := program.RuleOccurrenceCount()
	if !rulesPublished {
		return nil, nil, false
	}
	for index := 0; index < ruleCount; index++ {
		placement, placementOK := program.RuleOccurrenceAt(index)
		if !placementOK || !placement.Key().Available() {
			return nil, nil, false
		}
		usedKeys[placement.Key()] = struct{}{}
	}
	laws, lawsOK := stageLaws(vocabulary)
	if !lawsOK {
		return nil, nil, false
	}
	spec, specOK := rows.NewArtifactScalarSpec(snapshot.ArtifactID(), snapshot.ProgramID(), snapshot.SchemaID(), rows.ArtifactScalarCapacity{
		Roles: len(usedKeys), Points: pointCount, Edges: edgeCount, Transfers: transferCount, Regions: regionCount, Events: eventCount, Rules: ruleCount, Bodies: bodyCount,
	})
	if !specOK || !spec.InstallStageLaws(laws) {
		return nil, nil, false
	}
	directory := &RoleDirectory{bindings: make([]roleBinding, 0, len(usedKeys)), byKey: make(map[schema.Key]rows.ArtifactScalarRole, len(usedKeys))}
	ordered := make([]schema.Key, 0, len(usedKeys))
	for key := range usedKeys {
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
		directory.bindings = append(directory.bindings, roleBinding{key: key, scalar: role})
		directory.byKey[key] = role
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
		edge, edgeOK := spec.AddEdge(rows.ArtifactScalarEdge{ID: row.ID(), From: row.From(), To: row.To(), Route: row.RouteID(), Guard: guard, Decision: decision, Component: row.ComponentID(), Mu: mu, Reset: reset, Arm: arm, Guarded: guarded, Truth: truth, HasReset: hasReset})
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
			role, roleOK := directory.Role(key)
			if !writeOK || !keyOK || !roleOK || !spec.AddTransferFactor(transfer, role) {
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
		stage, stageOK := ruleStage(uint8(row.Stage()))
		input, _ := row.InputPoint()
		route, _ := row.PredecessorRouteID()
		if !roleOK || !stageOK || !row.Available() || !spec.AddRule(rows.ArtifactScalarRule{Role: role, Stage: stage, Point: row.PointID(), Input: input, ID: occurrenceID(program, row), Route: route}) {
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

func stageLaws(vocabulary structure.Table) ([]rows.ArtifactStageLaw, bool) {
	count := vocabulary.Count(structure.CategoryIssuanceStage)
	if count == 0 {
		return nil, false
	}
	byKey := make(map[schema.Key]rows.ArtifactRuleStage, count)
	for ordinal := uint16(1); int(ordinal) <= count; ordinal++ {
		entry, entryOK := vocabulary.At(structure.CategoryIssuanceStage, ordinal)
		if !entryOK || !entry.Key().Available() {
			return nil, false
		}
		stage := rows.ArtifactRuleStage(entry.Ordinal())
		if !stage.Valid() {
			return nil, false
		}
		byKey[entry.Key()] = stage
	}
	laws := make([]rows.ArtifactStageLaw, 0, count)
	for ordinal := uint16(1); int(ordinal) <= count; ordinal++ {
		entry, entryOK := vocabulary.At(structure.CategoryIssuanceStage, ordinal)
		if !entryOK {
			return nil, false
		}
		if !entry.Native() && !entry.Predecessor().Available() {
			continue
		}
		law := rows.ArtifactStageLaw{Stage: rows.ArtifactRuleStage(entry.Ordinal()), Native: entry.Native()}
		if entry.Predecessor().Available() {
			predecessor, predecessorOK := byKey[entry.Predecessor()]
			if !predecessorOK {
				return nil, false
			}
			law.Predecessor = predecessor
		}
		if !law.Valid() {
			return nil, false
		}
		laws = append(laws, law)
	}
	return laws, len(laws) > 0
}

func ruleStage(stage uint8) (rows.ArtifactRuleStage, bool) {
	converted := rows.ArtifactRuleStage(stage)
	if !converted.Valid() {
		return rows.ArtifactRuleStageInvalid, false
	}
	return converted, true
}

func acceptedOutcome(vocabulary structure.Table, kind programschema.OutcomeKind) bool {
	member, ok := vocabulary.At(structure.CategoryOutcome, uint16(kind))
	return ok && member.Accepted()
}
