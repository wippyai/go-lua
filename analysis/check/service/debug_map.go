package service

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/embedding"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// DebugMapSchemaVersion pins the exported artifact debug-map DTO and canonical
// byte encoding. Bump it with its hash guard and SCHEMA_VERSIONS.md entry when
// either changes.
const DebugMapSchemaVersion = 2

// EngineBuildTag is the single engine-version component stamped into static
// artifact IDs. A release changes this constant exactly once when the emitted
// artifact/debug semantics change; development builds use the checked-in tag.
const EngineBuildTag = "go-lua-engine-debug-map-v2"

// DebugPointID and DebugPhase are the lowering-owned, body-local execution
// identity vocabulary re-exported at the completed-result boundary.
type DebugPointID = wir.DebugPointID
type DebugPhase = wir.DebugPhase

const (
	DebugPhaseBefore  = wir.DebugPhaseBefore
	DebugPhaseAfter   = wir.DebugPhaseAfter
	DebugPhaseCall    = wir.DebugPhaseCall
	DebugPhaseReturn  = wir.DebugPhaseReturn
	DebugPhaseSuspend = wir.DebugPhaseSuspend
)

// DbgLocal, DebugAnchor, and DebugMapEntry are the solved-body projections
// carried by a per-body debug map.
type DbgLocal = body.DbgLocal
type DebugAnchor = body.DebugAnchor
type DebugMapEntry = body.DebugMapEntry

// BodyDebugMap is the deterministic artifact debug map for one solved body.
// Entries are an ordered map encoding of DebugPointID -> source/debug payload;
// callers must preserve this order when serializing an admitted artifact.
type BodyDebugMap struct {
	BodyID        BodyID
	BodyDigest    embedding.BodyInputDigest
	SchemaVersion int
	Digest        Digest
	Entries       []DebugMapEntry
}

// CanonicalBytes returns the schema-pinned deterministic encoding of Entries.
// BodyID and BodyDigest scope the map externally and are intentionally not
// folded into its content digest: StaticArtifactID composes both separately.
func (m BodyDebugMap) CanonicalBytes() []byte {
	return canonicalDebugMapBytes(m.SchemaVersion, m.Entries)
}

// StaticArtifactID names the exact static body artifact admitted to the arena
// runtime. It is not a deployment/runtime-instance identifier.
type StaticArtifactID struct {
	UnitDigest     Digest
	BodyDigest     embedding.BodyInputDigest
	Profile        string
	EngineBuildTag string
	DebugMapDigest Digest
}

// Valid reports whether every required static-artifact identity component is
// present. A zero digest cannot identify a compiled artifact.
func (id StaticArtifactID) Valid() bool {
	return !id.UnitDigest.IsZero() && id.BodyDigest != 0 && id.EngineBuildTag != "" && !id.DebugMapDigest.IsZero()
}

// String is the canonical, length-delimited static artifact ID form.
func (id StaticArtifactID) String() string {
	return "static-artifact-v1|" +
		canonicalArtifactField("unit", id.UnitDigest.String()) + "|" +
		canonicalArtifactField("body", fmt.Sprintf("%016x", id.BodyDigest)) + "|" +
		canonicalArtifactField("profile", id.Profile) + "|" +
		canonicalArtifactField("engine", id.EngineBuildTag) + "|" +
		canonicalArtifactField("debug-map", id.DebugMapDigest.String())
}

func canonicalArtifactField(name, value string) string {
	return name + "=" + strconv.Itoa(len(value)) + ":" + value
}

// StaticArtifact associates one completed-result body with the exact static
// artifact identity its codegen debug map must carry.
type StaticArtifact struct {
	BodyID BodyID
	ID     StaticArtifactID
}

func canonicalDebugMapBytes(schemaVersion int, entries []DebugMapEntry) []byte {
	var out bytes.Buffer
	writeDebugString(&out, "go-lua-debug-map")
	writeDebugUint64(&out, uint64(schemaVersion))
	writeDebugUint64(&out, uint64(len(entries)))
	for _, entry := range entries {
		writeDebugUint64(&out, uint64(entry.ID.Ordinal))
		writeDebugString(&out, entry.ID.Phase.String())
		writeDebugSpan(&out, entry.SourceSpan.StartLine, entry.SourceSpan.StartCol, entry.SourceSpan.EndLine, entry.SourceSpan.EndCol)
		writeDebugSpan(&out, entry.Anchor.StartLine, entry.Anchor.StartCol, entry.Anchor.EndLine, entry.Anchor.EndCol)
		writeDebugBool(&out, entry.MaySuspend)
		writeDebugUint64(&out, uint64(len(entry.Visible)))
		for _, local := range entry.Visible {
			writeDebugUint64(&out, uint64(local.LocalID))
			writeDebugString(&out, local.Name)
			writeDebugUint64(&out, uint64(local.Kind))
		}
	}
	return out.Bytes()
}

func writeDebugSpan(out *bytes.Buffer, startLine, startCol, endLine, endCol int) {
	writeDebugUint64(out, uint64(startLine))
	writeDebugUint64(out, uint64(startCol))
	writeDebugUint64(out, uint64(endLine))
	writeDebugUint64(out, uint64(endCol))
}

func writeDebugString(out *bytes.Buffer, value string) {
	writeDebugUint64(out, uint64(len(value)))
	_, _ = out.WriteString(value)
}

func writeDebugUint64(out *bytes.Buffer, value uint64) {
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], value)
	_, _ = out.Write(data[:])
}

func writeDebugBool(out *bytes.Buffer, value bool) {
	if value {
		writeDebugUint64(out, 1)
		return
	}
	writeDebugUint64(out, 0)
}

func cloneBodyDebugMaps(in []BodyDebugMap) []BodyDebugMap {
	if len(in) == 0 {
		return nil
	}
	out := append([]BodyDebugMap(nil), in...)
	for i := range out {
		out[i].Entries = append([]DebugMapEntry(nil), in[i].Entries...)
		for j := range out[i].Entries {
			out[i].Entries[j].Visible = append([]DbgLocal(nil), in[i].Entries[j].Visible...)
		}
	}
	return out
}

func cloneStaticArtifacts(in []StaticArtifact) []StaticArtifact {
	return append([]StaticArtifact(nil), in...)
}
