package transformer

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// RelationProgramManifest is the detached tier-1 directory for one relation
// forest. It is deliberately schema/topology only: it owns no execution
// authority, entry value, workspace, or pointer into the frozen program.
//
// Stage 2 records every current identity gap explicitly and never authorizes
// reuse. Later stages may replace an unavailable authority with a versioned
// structural digest, but cannot turn this manifest into an execution cache.
type RelationProgramManifest struct {
	units       []RelationProgramUnitManifest
	fingerprint [sha256.Size]byte
	missing     []RelationProgramManifestAuthority
	sealed      bool
}

// RelationProgramManifestAuthority names an input which must gain a canonical
// structural version before a tier-1 manifest may become a reuse key.
type RelationProgramManifestAuthority string

const (
	ManifestOperationPlan        RelationProgramManifestAuthority = "operation-plan-full"
	ManifestKeySpace             RelationProgramManifestAuthority = "keyspace"
	ManifestEntrySeedSchema      RelationProgramManifestAuthority = "entry-seed-schema"
	ManifestInitialStateSchema   RelationProgramManifestAuthority = "initial-state-schema"
	ManifestExecutionAuthorities RelationProgramManifestAuthority = "path-n4-n5-and-provider-authorities"
	ManifestDirectDeclarations   RelationProgramManifestAuthority = "direct-lexical-declarations"
)

// RelationProgramUnitManifest is one immutable lexical directory record. All
// fields are scalar values or copied value slices; it intentionally contains
// neither an authority pointer nor a solve-local value.
type RelationProgramUnitManifest struct {
	body             lexicalidentity.StableLexicalBodyID
	graphID          uint64
	pointCount       uint32
	shape            Shape
	component        uint32
	registrySchema   [sha256.Size]byte
	callSurface      operationplan.CallSurfaceDigest
	domainLanes      []string
	widenThresholds  []int64
	nodeReadDigest   [sha256.Size]byte
	definitionDigest [sha256.Size]byte
}

func (m RelationProgramManifest) Valid() bool { return m.sealed && len(m.units) != 0 }

// Reusable is deliberately false throughout Stage 2. A complete value-only
// directory is not permission to guess versions for opaque authorities.
func (m RelationProgramManifest) Reusable() bool { return false }

func (m RelationProgramManifest) Fingerprint() [sha256.Size]byte { return m.fingerprint }

func (m RelationProgramManifest) Units() []RelationProgramUnitManifest {
	out := append([]RelationProgramUnitManifest(nil), m.units...)
	for index := range out {
		out[index].domainLanes = append([]string(nil), out[index].domainLanes...)
		out[index].widenThresholds = append([]int64(nil), out[index].widenThresholds...)
	}
	return out
}

func (m RelationProgramManifest) MissingAuthorities() []RelationProgramManifestAuthority {
	return append([]RelationProgramManifestAuthority(nil), m.missing...)
}

func (m RelationProgramUnitManifest) Body() lexicalidentity.StableLexicalBodyID { return m.body }
func (m RelationProgramUnitManifest) GraphID() uint64                           { return m.graphID }
func (m RelationProgramUnitManifest) PointCount() uint32                        { return m.pointCount }
func (m RelationProgramUnitManifest) Shape() Shape                              { return m.shape }
func (m RelationProgramUnitManifest) Component() uint32                         { return m.component }
func (m RelationProgramUnitManifest) RegistrySchema() [sha256.Size]byte         { return m.registrySchema }
func (m RelationProgramUnitManifest) CallSurface() operationplan.CallSurfaceDigest {
	return m.callSurface
}
func (m RelationProgramUnitManifest) DomainLanes() []string {
	return append([]string(nil), m.domainLanes...)
}
func (m RelationProgramUnitManifest) WidenThresholds() []int64 {
	return append([]int64(nil), m.widenThresholds...)
}
func (m RelationProgramUnitManifest) NodeReadDigest() [sha256.Size]byte   { return m.nodeReadDigest }
func (m RelationProgramUnitManifest) DefinitionDigest() [sha256.Size]byte { return m.definitionDigest }

func (m RelationProgramManifest) clone() RelationProgramManifest {
	m.units = m.Units()
	m.missing = append([]RelationProgramManifestAuthority(nil), m.missing...)
	return m
}

func freezeRelationProgramManifest(units []RelationProgramUnit, topology operationplan.CallTopology) (RelationProgramManifest, error) {
	if len(units) == 0 || !topology.Complete() {
		return RelationProgramManifest{}, fmt.Errorf("transformer: tier-1 manifest has incomplete forest input")
	}
	ordered := append([]RelationProgramUnit(nil), units...)
	sort.Slice(ordered, func(i, j int) bool { return bytes.Compare(ordered[i].Body[:], ordered[j].Body[:]) < 0 })
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.relation-program.tier-1.v1"))
	out := RelationProgramManifest{
		units: make([]RelationProgramUnitManifest, len(ordered)),
		missing: []RelationProgramManifestAuthority{
			ManifestOperationPlan, ManifestKeySpace, ManifestEntrySeedSchema,
			ManifestInitialStateSchema, ManifestExecutionAuthorities, ManifestDirectDeclarations,
		},
	}
	for index, unit := range ordered {
		if unit.Body == (lexicalidentity.StableLexicalBodyID{}) || unit.Graph == nil || unit.Plan == nil || !unit.Domain.Valid() {
			return RelationProgramManifest{}, fmt.Errorf("transformer: tier-1 manifest has incomplete lexical unit")
		}
		registrySchema, err := relationProgramManifestRegistrySchema(unit.Registry)
		if err != nil {
			return RelationProgramManifest{}, err
		}
		surface, exact := unit.Plan.CallSurface()
		if !exact || !surface.Complete() {
			return RelationProgramManifest{}, fmt.Errorf("transformer: tier-1 manifest has incomplete call surface for %s", unit.Body)
		}
		lanes := unit.Domain.Lanes().IDs()
		laneNames := make([]string, len(lanes))
		for laneIndex, lane := range lanes {
			laneNames[laneIndex] = string(lane)
		}
		options := unit.Domain.Options()
		record := RelationProgramUnitManifest{
			body: unit.Body, graphID: unit.Graph.ID(), pointCount: uint32(unit.Graph.Size()), shape: unit.Shape,
			component: topology.Component(unit.Body), registrySchema: registrySchema, callSurface: surface.Digest(),
			domainLanes: laneNames, widenThresholds: append([]int64(nil), options.WidenThresholds...),
			nodeReadDigest: relationProgramManifestNodeReads(unit.NodeReads), definitionDigest: relationProgramManifestDefinitions(unit.Definitions),
		}
		out.units[index] = record
		relationProgramManifestWriteUnit(h, record)
	}
	// The complete topology is structural; global contracts are intentionally
	// omitted because they are product values, not workspace-independent schema.
	for _, body := range topology.Bodies() {
		_, _ = h.Write(body[:])
		var scratch [4]byte
		var wide [8]byte
		binary.LittleEndian.PutUint32(scratch[:], topology.Component(body))
		_, _ = h.Write(scratch[:])
		for _, site := range topology.Sites(body) {
			binary.LittleEndian.PutUint32(scratch[:], uint32(site.Point()))
			_, _ = h.Write(scratch[:])
			for _, candidate := range site.Candidates() {
				_, _ = h.Write([]byte(candidate.Identity.Kind))
				_, _ = h.Write([]byte(candidate.Identity.Site))
				binary.LittleEndian.PutUint64(wide[:], candidate.Identity.Index)
				_, _ = h.Write(wide[:])
				_, _ = h.Write(candidate.Target[:])
			}
		}
		captures, globals, _, ok := topology.Boundary(body)
		if !ok {
			return RelationProgramManifest{}, fmt.Errorf("transformer: tier-1 manifest has no topology boundary for %s", body)
		}
		for _, symbol := range append(captures, globals...) {
			binary.LittleEndian.PutUint32(scratch[:], uint32(symbol))
			_, _ = h.Write(scratch[:])
		}
	}
	copy(out.fingerprint[:], h.Sum(nil))
	out.sealed = true
	return out, nil
}

func relationProgramManifestRegistrySchema(registry *axis.Registry) ([sha256.Size]byte, error) {
	if registry == nil {
		return [sha256.Size]byte{}, fmt.Errorf("transformer: tier-1 manifest has no axis registry")
	}
	plan, err := registry.CanonicalPlan()
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	identity, ok := plan.AuthorityIdentity()
	if !ok {
		return [sha256.Size]byte{}, fmt.Errorf("transformer: tier-1 manifest has no canonical axis registry identity")
	}
	return identity, nil
}

func relationProgramManifestNodeReads(reads [][]cfg.Point) [sha256.Size]byte {
	h := sha256.New()
	var scratch [4]byte
	for point, dependencies := range reads {
		binary.LittleEndian.PutUint32(scratch[:], uint32(point))
		_, _ = h.Write(scratch[:])
		for _, dependency := range dependencies {
			binary.LittleEndian.PutUint32(scratch[:], uint32(dependency))
			_, _ = h.Write(scratch[:])
		}
	}
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

func relationProgramManifestDefinitions(definitions []RelationProgramDefinition) [sha256.Size]byte {
	h := sha256.New()
	var scratch [4]byte
	for _, definition := range definitions {
		_, _ = h.Write(definition.Target[:])
		binary.LittleEndian.PutUint32(scratch[:], uint32(definition.Point))
		_, _ = h.Write(scratch[:])
		if definition.ExternallyReachable {
			_, _ = h.Write([]byte{1})
		} else {
			_, _ = h.Write([]byte{0})
		}
	}
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

func relationProgramManifestWriteUnit(h interface{ Write([]byte) (int, error) }, unit RelationProgramUnitManifest) {
	_, _ = h.Write(unit.body[:])
	var scratch [8]byte
	binary.LittleEndian.PutUint64(scratch[:], unit.graphID)
	_, _ = h.Write(scratch[:])
	binary.LittleEndian.PutUint32(scratch[:4], unit.pointCount)
	_, _ = h.Write(scratch[:4])
	binary.LittleEndian.PutUint32(scratch[:4], unit.component)
	_, _ = h.Write(scratch[:4])
	_, _ = h.Write(unit.registrySchema[:])
	_, _ = h.Write(unit.callSurface[:])
	for _, lane := range unit.domainLanes {
		_, _ = h.Write([]byte(lane))
	}
	for _, threshold := range unit.widenThresholds {
		binary.LittleEndian.PutUint64(scratch[:], uint64(threshold))
		_, _ = h.Write(scratch[:])
	}
	_, _ = h.Write(unit.nodeReadDigest[:])
	_, _ = h.Write(unit.definitionDigest[:])
}
