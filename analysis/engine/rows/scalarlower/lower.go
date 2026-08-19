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
	usedKeys := make(map[schema.Key]struct{})
	for index := 0; index < snapshot.LocalTransferCount(); index++ {
		transfer, transferOK := snapshot.LocalTransferAt(index)
		if !transferOK {
			return nil, nil, false
		}
		for inner := 0; inner < transfer.WritesCount(); inner++ {
			write, writeOK := transfer.WritesAt(inner)
			if !writeOK || !write.Available() {
				return nil, nil, false
			}
			usedKeys[write] = struct{}{}
		}
	}
	for index := 0; index < snapshot.RulePlacementCount(); index++ {
		placement, placementOK := snapshot.RulePlacementAt(index)
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
		Roles: len(usedKeys), Points: snapshot.PointCount(), Edges: snapshot.StructuralEdgeCount(), Transfers: snapshot.LocalTransferCount(), Regions: snapshot.RegionCount(), Events: snapshot.EventCount(), Rules: snapshot.RulePlacementCount(), Bodies: snapshot.BodyTransportCount(),
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
	for index := 0; index < snapshot.PointCount(); index++ {
		row, rowOK := snapshot.PointAt(index)
		if !rowOK || !row.ID().Available() {
			return nil, nil, false
		}
		point, pointOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: row.ID(), Initial: row.Initial()})
		if !pointOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.DecisionCount(); inner++ {
			decision, decisionOK := row.DecisionAt(inner)
			if !decisionOK || !spec.AddPointDecision(point, decision) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < snapshot.StructuralEdgeCount(); index++ {
		row, rowOK := snapshot.StructuralEdgeAt(index)
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
		arm, armOK := structuralArm(row.Arm())
		if !armOK {
			return nil, nil, false
		}
		edge, edgeOK := spec.AddEdge(rows.ArtifactScalarEdge{ID: row.ID(), From: row.From(), To: row.To(), Route: row.RouteID(), Guard: guard, Decision: decision, Component: row.ComponentID(), Mu: mu, Reset: reset, Arm: arm, Guarded: guarded, Truth: truth, HasReset: hasReset})
		if !edgeOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.ResetCount(); inner++ {
			resetPoint, resetOK := row.ResetAt(inner)
			if !resetOK || !spec.AddEdgeReset(edge, resetPoint) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < snapshot.LocalTransferCount(); index++ {
		row, rowOK := snapshot.LocalTransferAt(index)
		if !rowOK {
			return nil, nil, false
		}
		transfer, transferOK := spec.AddTransfer(rows.ArtifactScalarTransfer{ID: row.ID(), From: row.From(), To: row.To(), Full: row.Full()})
		if !transferOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.WritesCount(); inner++ {
			write, writeOK := row.WritesAt(inner)
			role, roleOK := directory.Role(write)
			if !writeOK || !roleOK || !spec.AddTransferFactor(transfer, role) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < snapshot.RegionCount(); index++ {
		row, rowOK := snapshot.RegionAt(index)
		if !rowOK {
			return nil, nil, false
		}
		region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: row.ID(), Head: row.Head(), Parent: row.ParentID(), Cyclic: row.Cyclic()})
		if !regionOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.MemberCount(); inner++ {
			member, memberOK := row.MemberAt(inner)
			if !memberOK || !spec.AddRegionMember(region, member) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < snapshot.EventCount(); index++ {
		row, rowOK := snapshot.EventAt(index)
		if !rowOK {
			return nil, nil, false
		}
		kind, kindOK := eventKind(row.Kind())
		if !kindOK || !spec.AddEvent(rows.ArtifactScalarEvent{Kind: kind, Region: row.RegionID(), Point: row.PointID()}) {
			return nil, nil, false
		}
	}
	for index := 0; index < snapshot.RulePlacementCount(); index++ {
		row, rowOK := snapshot.RulePlacementAt(index)
		if !rowOK {
			return nil, nil, false
		}
		role, roleOK := directory.Role(row.Key())
		stage, stageOK := ruleStage(row.Stage())
		if !roleOK || !stageOK || !spec.AddRule(rows.ArtifactScalarRule{Role: role, Stage: stage, Point: row.PointID(), Input: row.InputPointID(), ID: row.OccurrenceID(), Route: row.PredecessorRouteID()}) {
			return nil, nil, false
		}
	}
	for index := 0; index < snapshot.BodyTransportCount(); index++ {
		row, rowOK := snapshot.BodyTransportAt(index)
		if !rowOK {
			return nil, nil, false
		}
		body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: row.BodyID()})
		if !bodyOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.EntryCount(); inner++ {
			point, pointOK := row.EntryAt(inner)
			if !pointOK || !spec.AddBodyEntry(body, point) {
				return nil, nil, false
			}
		}
		for inner := 0; inner < row.ExitCount(); inner++ {
			point, pointOK := row.ExitAt(inner)
			if !pointOK || !spec.AddBodyExit(body, point) {
				return nil, nil, false
			}
		}
	}
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	return template, directory, templateOK
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

func structuralArm(arm ingress.StructuralArm) (rows.ArtifactStructuralArm, bool) {
	if !arm.Valid() {
		return rows.ArtifactStructuralArmInvalid, false
	}
	return rows.ArtifactStructuralArm(arm), true
}

func eventKind(kind ingress.EventKind) (rows.ArtifactEventKind, bool) {
	if kind < ingress.EventEnter || kind > ingress.EventExit {
		return rows.ArtifactEventInvalid, false
	}
	return rows.ArtifactEventKind(kind), true
}
