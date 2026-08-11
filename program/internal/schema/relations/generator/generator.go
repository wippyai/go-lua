// Package generator owns the cold catalog parser, compatibility history, and
// static-source emitter. It is intentionally not linked into Program, Link,
// Target, or the analysis engine.
package generator

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ref struct {
	origin string
	facet  string
}

type entry struct {
	origin      string
	originValue uint32
	facet       string
	facetValue  uint16
	owner       string
	form        string
	revision    uint16
	parents     []ref
}

type historyEntry struct {
	originName string
	facetName  string
	digest     string
}

type historyKey struct {
	origin   uint32
	facet    uint16
	revision uint16
}

type historyTokenKey struct {
	origin uint32
	facet  uint16
}

type historyNameKey struct {
	origin string
	facet  string
}

// retirement is one irreversible catalog deletion. It deliberately repeats
// the final history identity so deleting a live row cannot be disguised as a
// new name, number, revision, or semantic definition.
type retirement struct {
	key   historyKey
	entry historyEntry
}

const (
	relationSchemaPath  = "program/internal/schema/relations/catalog.schema"
	relationHistoryPath = "program/internal/schema/relations/catalog.history"
	relationRetiredPath = "program/internal/schema/relations/catalog.retired"
	semanticSourcePath  = "program/semanticsource/generated.go"
	relationSourcePath  = "program/internal/schema/relations/generated.go"
)

type paths struct {
	schema    string
	history   string
	retired   string
	semantic  string
	relations string
}

// Evidence is an immutable, fixed-order snapshot of the five files that prove
// the checked-in catalog is current. Accessors always return detached bytes.
type Evidence struct {
	schema    []byte
	history   []byte
	retired   []byte
	semantic  []byte
	relations []byte
	digest    [sha256.Size]byte
}

// Check validates the fixed catalog files rooted at root and returns their
// exact raw bytes only when the history and both generated projections are
// current. It does not run a subprocess or write any file.
func Check(root string) (Evidence, error) {
	files := fixedPaths(root)
	artifact, err := build(files, false)
	if err != nil {
		return Evidence{}, err
	}
	semantic, relations, err := checkedFiles(files, artifact)
	if err != nil {
		return Evidence{}, err
	}
	return newEvidence(artifact.schema, artifact.history, artifact.retired, semantic, relations), nil
}

// Run applies the same parser, history validation, and source emission used by
// Check to explicit CLI paths. When check is true it only verifies freshness;
// when writeHistory is true it appends new compatible history records.
func Run(input, history, retired, semanticOutput, relationOutput string, check, writeHistory bool) error {
	if input == "" || history == "" || retired == "" || semanticOutput == "" || relationOutput == "" || check && writeHistory {
		return errors.New("catalog generator: input, history, retired, and both outputs are required; check and write-history conflict")
	}
	files := paths{schema: input, history: history, retired: retired, semantic: semanticOutput, relations: relationOutput}
	artifact, err := build(files, writeHistory)
	if err != nil {
		return err
	}
	if check {
		_, _, err := checkedFiles(files, artifact)
		return err
	}
	if err := write(files.semantic, artifact.semantic); err != nil {
		return err
	}
	if err := write(files.relations, artifact.relations); err != nil {
		return err
	}
	if writeHistory {
		return write(files.history, artifact.history)
	}
	return nil
}

// RawSchema returns an exact detached catalog.schema snapshot.
func (e Evidence) RawSchema() []byte { return append([]byte(nil), e.schema...) }

// RawHistory returns an exact detached catalog.history snapshot.
func (e Evidence) RawHistory() []byte { return append([]byte(nil), e.history...) }

// RawRetired returns an exact detached catalog.retired snapshot.
func (e Evidence) RawRetired() []byte { return append([]byte(nil), e.retired...) }

// SemanticSource returns an exact detached semanticsource generated output.
func (e Evidence) SemanticSource() []byte { return append([]byte(nil), e.semantic...) }

// RelationSource returns an exact detached relations generated output.
func (e Evidence) RelationSource() []byte { return append([]byte(nil), e.relations...) }

// Digest is the fixed-order SHA-256 identity of all evidence bytes.
func (e Evidence) Digest() [sha256.Size]byte { return e.digest }

// SchemaDigest returns the SHA-256 identity of RawSchema.
func (e Evidence) SchemaDigest() [sha256.Size]byte { return sha256.Sum256(e.schema) }

// HistoryDigest returns the SHA-256 identity of RawHistory.
func (e Evidence) HistoryDigest() [sha256.Size]byte { return sha256.Sum256(e.history) }

// RetiredDigest returns the SHA-256 identity of RawRetired.
func (e Evidence) RetiredDigest() [sha256.Size]byte { return sha256.Sum256(e.retired) }

// SemanticSourceDigest returns the SHA-256 identity of SemanticSource.
func (e Evidence) SemanticSourceDigest() [sha256.Size]byte { return sha256.Sum256(e.semantic) }

// RelationSourceDigest returns the SHA-256 identity of RelationSource.
func (e Evidence) RelationSourceDigest() [sha256.Size]byte { return sha256.Sum256(e.relations) }

type artifact struct {
	schema           []byte
	history          []byte
	retired          []byte
	canonicalHistory []byte
	semantic         []byte
	relations        []byte
}

func fixedPaths(root string) paths {
	return paths{
		schema:    filepath.Join(root, relationSchemaPath),
		history:   filepath.Join(root, relationHistoryPath),
		retired:   filepath.Join(root, relationRetiredPath),
		semantic:  filepath.Join(root, semanticSourcePath),
		relations: filepath.Join(root, relationSourcePath),
	}
}

func build(files paths, writeHistory bool) (artifact, error) {
	schema, err := os.ReadFile(files.schema)
	if err != nil {
		return artifact{}, err
	}
	entries, err := parseBytes(schema)
	if err != nil {
		return artifact{}, err
	}
	if err := validate(entries); err != nil {
		return artifact{}, err
	}

	history, err := os.ReadFile(files.history)
	if err != nil && !(writeHistory && errors.Is(err, os.ErrNotExist)) {
		return artifact{}, err
	}
	var baseline map[historyKey]historyEntry
	if len(history) != 0 {
		baseline, err = parseHistoryBytes(history)
		if err != nil {
			return artifact{}, err
		}
	} else if writeHistory {
		baseline = make(map[historyKey]historyEntry)
	} else {
		return artifact{}, errors.New("catalog generator: empty history")
	}
	retired, err := os.ReadFile(files.retired)
	if err != nil {
		return artifact{}, err
	}
	retirements, err := parseRetiredBytes(retired)
	if err != nil {
		return artifact{}, err
	}
	if err := validateRetirements(entries, baseline, retirements); err != nil {
		return artifact{}, err
	}
	if writeHistory {
		if err := appendCurrentHistory(entries, baseline, retirements); err != nil {
			return artifact{}, err
		}
		history, err = emitHistory(baseline)
		if err != nil {
			return artifact{}, err
		}
	} else if err := validateCompatibility(entries, baseline, retirements); err != nil {
		return artifact{}, err
	}
	canonicalHistory, err := emitHistory(baseline)
	if err != nil {
		return artifact{}, err
	}
	semantic, err := emitSemanticSource(entries)
	if err != nil {
		return artifact{}, err
	}
	relations, err := emitRelations(entries)
	if err != nil {
		return artifact{}, err
	}
	return artifact{
		schema:           append([]byte(nil), schema...),
		history:          append([]byte(nil), history...),
		retired:          append([]byte(nil), retired...),
		canonicalHistory: canonicalHistory,
		semantic:         semantic,
		relations:        relations,
	}, nil
}

func checkedFiles(files paths, artifact artifact) ([]byte, []byte, error) {
	if err := freshBytes(files.history, artifact.history, artifact.canonicalHistory); err != nil {
		return nil, nil, err
	}
	return checkedOutputs(files, artifact)
}

func checkedOutputs(files paths, artifact artifact) ([]byte, []byte, error) {
	semantic, err := os.ReadFile(files.semantic)
	if err != nil {
		return nil, nil, err
	}
	if err := freshBytes(files.semantic, semantic, artifact.semantic); err != nil {
		return nil, nil, err
	}
	relations, err := os.ReadFile(files.relations)
	if err != nil {
		return nil, nil, err
	}
	if err := freshBytes(files.relations, relations, artifact.relations); err != nil {
		return nil, nil, err
	}
	return semantic, relations, nil
}

func newEvidence(schema, history, retired, semantic, relations []byte) Evidence {
	evidence := Evidence{
		schema:    append([]byte(nil), schema...),
		history:   append([]byte(nil), history...),
		retired:   append([]byte(nil), retired...),
		semantic:  append([]byte(nil), semantic...),
		relations: append([]byte(nil), relations...),
	}
	evidence.digest = evidenceDigest(evidence.schema, evidence.history, evidence.retired, evidence.semantic, evidence.relations)
	return evidence
}

func evidenceDigest(parts ...[]byte) [sha256.Size]byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte("program.schema.relations.generator.evidence.v2\x00"))
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(part)
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest
}

func parse(path string) ([]entry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseBytes(content)
}

func parseBytes(content []byte) ([]entry, error) {
	var entries []entry
	for lineNumber, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 8 {
			return nil, fmt.Errorf("catalog generator: line %d: expected 8 columns", lineNumber+1)
		}
		origin, err := strconv.ParseUint(fields[1], 0, 32)
		if err != nil || origin == 0 {
			return nil, fmt.Errorf("catalog generator: line %d: invalid origin", lineNumber+1)
		}
		facet, err := strconv.ParseUint(fields[3], 0, 16)
		if err != nil {
			return nil, fmt.Errorf("catalog generator: line %d: invalid facet", lineNumber+1)
		}
		revision, err := strconv.ParseUint(fields[6], 0, 16)
		if err != nil || revision == 0 {
			return nil, fmt.Errorf("catalog generator: line %d: invalid revision", lineNumber+1)
		}
		item := entry{origin: fields[0], originValue: uint32(origin), facet: fields[2], facetValue: uint16(facet), owner: fields[4], form: fields[5], revision: uint16(revision)}
		if (item.facet == "-") != (item.facetValue == 0) {
			return nil, fmt.Errorf("catalog generator: line %d: primary facet must be -/0", lineNumber+1)
		}
		if fields[7] != "-" {
			for _, parent := range strings.Split(fields[7], ",") {
				parts := strings.Split(parent, "@")
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					return nil, fmt.Errorf("catalog generator: line %d: invalid parent", lineNumber+1)
				}
				item.parents = append(item.parents, ref{origin: parts[0], facet: parts[1]})
			}
		}
		entries = append(entries, item)
	}
	return entries, nil
}

func validate(entries []entry) error {
	if len(entries) == 0 {
		return errors.New("catalog generator: empty input")
	}
	byRef := make(map[ref]entry, len(entries))
	type originIdentity struct {
		value    uint32
		revision uint16
	}
	type tokenIdentity struct {
		origin   uint32
		facet    uint16
		revision uint16
	}
	origins := map[string]originIdentity{}
	numericOrigins := map[uint32]string{}
	numericTokens := map[tokenIdentity]ref{}
	facets := map[string]uint16{}
	validOwners := map[string]bool{"ProgramSource": true, "ProgramFlow": true, "ProgramStatic": true, "ProgramModule": true, "Target": true, "LinkProject": true, "LinkBoundary": true, "LinkModule": true, "LinkStatic": true, "LinkHost": true}
	validForms := map[string]bool{"Authored": true, "SealDerived": true, "VirtualPredicate": true}
	for _, item := range entries {
		key := ref{origin: item.origin, facet: item.facet}
		if _, duplicate := byRef[key]; duplicate {
			return fmt.Errorf("catalog generator: duplicate relation %s@%s", item.origin, item.facet)
		}
		byRef[key] = item
		origin := originIdentity{value: item.originValue, revision: item.revision}
		if prior, exists := origins[item.origin]; exists && prior != origin {
			return fmt.Errorf("catalog generator: conflicting origin %s", item.origin)
		}
		origins[item.origin] = origin
		if prior, exists := numericOrigins[item.originValue]; exists && prior != item.origin {
			return fmt.Errorf("catalog generator: numeric origin collision %s/%s", prior, item.origin)
		}
		numericOrigins[item.originValue] = item.origin
		token := tokenIdentity{origin: item.originValue, facet: item.facetValue, revision: item.revision}
		if prior, exists := numericTokens[token]; exists {
			return fmt.Errorf("catalog generator: numeric token collision %s@%s/%s@%s", prior.origin, prior.facet, item.origin, item.facet)
		}
		numericTokens[token] = key
		if item.facet != "-" {
			if prior, exists := facets[item.facet]; exists && prior != item.facetValue {
				return fmt.Errorf("catalog generator: conflicting facet %s", item.facet)
			}
			facets[item.facet] = item.facetValue
		}
		if !validOwners[item.owner] || !validForms[item.form] {
			return fmt.Errorf("catalog generator: invalid owner/form for %s@%s", item.origin, item.facet)
		}
	}
	for _, item := range entries {
		if item.facet != "-" {
			if _, exists := byRef[ref{origin: item.origin, facet: "-"}]; !exists {
				return fmt.Errorf("catalog generator: missing primary for %s@%s", item.origin, item.facet)
			}
		}
		seen := map[ref]bool{}
		for _, parent := range item.parents {
			if _, exists := byRef[parent]; !exists || parent == (ref{origin: item.origin, facet: item.facet}) || seen[parent] {
				return fmt.Errorf("catalog generator: invalid parent for %s@%s", item.origin, item.facet)
			}
			seen[parent] = true
		}
	}
	return validateParentGraph(byRef)
}

func validateParentGraph(entries map[ref]entry) error {
	for _, component := range parentSCCs(entries) {
		if len(component) < 2 {
			continue
		}
		owner := entries[component[0]].owner
		for _, key := range component[1:] {
			if entries[key].owner != owner {
				return fmt.Errorf("catalog generator: cyclic parents cross owner components at %s@%s", key.origin, key.facet)
			}
		}
	}
	return validateOwnerGraph(entries)
}

func parentSCCs(entries map[ref]entry) [][]ref {
	keys := make([]ref, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return lessRef(entries, keys[left], keys[right]) })

	index := make(map[ref]int, len(entries))
	lowlink := make(map[ref]int, len(entries))
	onStack := make(map[ref]bool, len(entries))
	stack := make([]ref, 0, len(entries))
	components := make([][]ref, 0, len(entries))
	next := 0
	var visit func(ref)
	visit = func(key ref) {
		index[key] = next
		lowlink[key] = next
		next++
		stack = append(stack, key)
		onStack[key] = true

		parents := append([]ref(nil), entries[key].parents...)
		sort.Slice(parents, func(left, right int) bool { return lessRef(entries, parents[left], parents[right]) })
		for _, parent := range parents {
			if _, seen := index[parent]; !seen {
				visit(parent)
				if lowlink[parent] < lowlink[key] {
					lowlink[key] = lowlink[parent]
				}
			} else if onStack[parent] && index[parent] < lowlink[key] {
				lowlink[key] = index[parent]
			}
		}

		if lowlink[key] != index[key] {
			return
		}
		component := make([]ref, 0, 1)
		for {
			last := len(stack) - 1
			member := stack[last]
			stack = stack[:last]
			onStack[member] = false
			component = append(component, member)
			if member == key {
				break
			}
		}
		sort.Slice(component, func(left, right int) bool { return lessRef(entries, component[left], component[right]) })
		components = append(components, component)
	}
	for _, key := range keys {
		if _, seen := index[key]; !seen {
			visit(key)
		}
	}
	sort.Slice(components, func(left, right int) bool {
		return lessRef(entries, components[left][0], components[right][0])
	})
	return components
}

func validateOwnerGraph(entries map[ref]entry) error {
	edges := make(map[string]map[string]struct{}, len(entries))
	for _, item := range entries {
		for _, parent := range item.parents {
			parentOwner := entries[parent].owner
			if parentOwner == item.owner {
				continue
			}
			if edges[item.owner] == nil {
				edges[item.owner] = make(map[string]struct{})
			}
			edges[item.owner][parentOwner] = struct{}{}
		}
	}
	owners := make([]string, 0, len(edges))
	for owner := range edges {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	state := make(map[string]uint8, len(owners))
	var visit func(string) error
	visit = func(owner string) error {
		switch state[owner] {
		case 1:
			return fmt.Errorf("catalog generator: cyclic owner dependencies at %s", owner)
		case 2:
			return nil
		}
		state[owner] = 1
		parents := make([]string, 0, len(edges[owner]))
		for parent := range edges[owner] {
			parents = append(parents, parent)
		}
		sort.Strings(parents)
		for _, parent := range parents {
			if err := visit(parent); err != nil {
				return err
			}
		}
		state[owner] = 2
		return nil
	}
	for _, owner := range owners {
		if err := visit(owner); err != nil {
			return err
		}
	}
	return nil
}

func lessRef(entries map[ref]entry, left, right ref) bool {
	leftEntry := entries[left]
	rightEntry := entries[right]
	if leftEntry.originValue != rightEntry.originValue {
		return leftEntry.originValue < rightEntry.originValue
	}
	if leftEntry.facetValue != rightEntry.facetValue {
		return leftEntry.facetValue < rightEntry.facetValue
	}
	if leftEntry.revision != rightEntry.revision {
		return leftEntry.revision < rightEntry.revision
	}
	if left.origin != right.origin {
		return left.origin < right.origin
	}
	return left.facet < right.facet
}

func parseHistory(path string) (map[historyKey]historyEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseHistoryBytes(content)
}

func parseRetired(path string) (map[historyTokenKey]retirement, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseRetiredBytes(content)
}

func parseHistoryBytes(content []byte) (map[historyKey]historyEntry, error) {
	result := make(map[historyKey]historyEntry)
	for lineNumber, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 6 {
			return nil, fmt.Errorf("catalog generator: history line %d: expected 6 columns", lineNumber+1)
		}
		if fields[0] == "" || fields[2] == "" {
			return nil, fmt.Errorf("catalog generator: history line %d: empty name", lineNumber+1)
		}
		origin, err := strconv.ParseUint(fields[1], 0, 32)
		if err != nil || origin == 0 {
			return nil, fmt.Errorf("catalog generator: history line %d: invalid origin", lineNumber+1)
		}
		facet, err := strconv.ParseUint(fields[3], 0, 16)
		if err != nil {
			return nil, fmt.Errorf("catalog generator: history line %d: invalid facet", lineNumber+1)
		}
		if (fields[2] == "-") != (facet == 0) {
			return nil, fmt.Errorf("catalog generator: history line %d: invalid primary facet", lineNumber+1)
		}
		revision, err := strconv.ParseUint(fields[4], 0, 16)
		if err != nil || revision == 0 {
			return nil, fmt.Errorf("catalog generator: history line %d: invalid revision", lineNumber+1)
		}
		if _, err := hex.DecodeString(fields[5]); err != nil || len(fields[5]) != sha256.Size*2 {
			return nil, fmt.Errorf("catalog generator: history line %d: invalid digest", lineNumber+1)
		}
		key := historyKey{origin: uint32(origin), facet: uint16(facet), revision: uint16(revision)}
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("catalog generator: duplicate history relation origin=0x%08X facet=%d revision %d", key.origin, key.facet, key.revision)
		}
		result[key] = historyEntry{originName: fields[0], facetName: fields[2], digest: fields[5]}
	}
	if len(result) == 0 {
		return nil, errors.New("catalog generator: empty history")
	}
	if err := validateHistoricalNames(result); err != nil {
		return nil, err
	}
	return result, nil
}

// parseRetiredBytes parses the append-only retirement ledger. Unlike history,
// an empty ledger is valid: no relation has been retired yet.
func parseRetiredBytes(content []byte) (map[historyTokenKey]retirement, error) {
	result := make(map[historyTokenKey]retirement)
	names := make(map[historyNameKey]historyTokenKey)
	for lineNumber, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 6 {
			return nil, fmt.Errorf("catalog generator: retired line %d: expected 6 columns", lineNumber+1)
		}
		if fields[0] == "" || fields[2] == "" {
			return nil, fmt.Errorf("catalog generator: retired line %d: empty name", lineNumber+1)
		}
		origin, err := strconv.ParseUint(fields[1], 0, 32)
		if err != nil || origin == 0 {
			return nil, fmt.Errorf("catalog generator: retired line %d: invalid origin", lineNumber+1)
		}
		facet, err := strconv.ParseUint(fields[3], 0, 16)
		if err != nil {
			return nil, fmt.Errorf("catalog generator: retired line %d: invalid facet", lineNumber+1)
		}
		if (fields[2] == "-") != (facet == 0) {
			return nil, fmt.Errorf("catalog generator: retired line %d: invalid primary facet", lineNumber+1)
		}
		revision, err := strconv.ParseUint(fields[4], 0, 16)
		if err != nil || revision == 0 {
			return nil, fmt.Errorf("catalog generator: retired line %d: invalid final revision", lineNumber+1)
		}
		if _, err := hex.DecodeString(fields[5]); err != nil || len(fields[5]) != sha256.Size*2 {
			return nil, fmt.Errorf("catalog generator: retired line %d: invalid digest", lineNumber+1)
		}
		key := historyKey{origin: uint32(origin), facet: uint16(facet), revision: uint16(revision)}
		token := historyTokenKey{origin: key.origin, facet: key.facet}
		if _, duplicate := result[token]; duplicate {
			return nil, fmt.Errorf("catalog generator: duplicate retired relation origin=0x%08X facet=%d", token.origin, token.facet)
		}
		name := historyNameKey{origin: fields[0], facet: fields[2]}
		if prior, duplicate := names[name]; duplicate {
			return nil, fmt.Errorf("catalog generator: duplicate retired relation name %s@%s at origin=0x%08X facet=%d", name.origin, name.facet, prior.origin, prior.facet)
		}
		result[token] = retirement{key: key, entry: historyEntry{originName: fields[0], facetName: fields[2], digest: fields[5]}}
		names[name] = token
	}
	return result, nil
}

func appendCurrentHistory(entries []entry, baseline map[historyKey]historyEntry, retirements map[historyTokenKey]retirement) error {
	if err := validateHistoricalPresence(entries, baseline, retirements, true); err != nil {
		return err
	}
	definitions := entryIndex(entries)
	for _, item := range entries {
		key := historyKeyFor(item)
		if err := validateHistoricalName(baseline, key, item); err != nil {
			return err
		}
		digest, err := semanticDigest(item, definitions)
		if err != nil {
			return err
		}
		if prior, exists := baseline[key]; exists {
			if !sameHistoryIdentity(prior, item) || prior.digest != digest {
				return fmt.Errorf("catalog generator: cannot rewrite history for %s@%s revision %d", item.origin, item.facet, key.revision)
			}
			continue
		}
		baseline[key] = historyEntry{originName: item.origin, facetName: item.facet, digest: digest}
	}
	return validateHistoricalPresence(entries, baseline, retirements, false)
}

func emitHistory(baseline map[historyKey]historyEntry) ([]byte, error) {
	ordered := make([]historyKey, 0, len(baseline))
	for key := range baseline {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].origin != ordered[right].origin {
			return ordered[left].origin < ordered[right].origin
		}
		if ordered[left].facet != ordered[right].facet {
			return ordered[left].facet < ordered[right].facet
		}
		return ordered[left].revision < ordered[right].revision
	})
	var out bytes.Buffer
	out.WriteString("# Generated B0 relation semantic revision history; DO NOT EDIT.\n")
	out.WriteString("# Columns: origin-name origin-number facet-name facet-number revision semantic-digest\n")
	for _, key := range ordered {
		item := baseline[key]
		fmt.Fprintf(&out, "%s 0x%08X %s %d %d %s\n", item.originName, key.origin, item.facetName, key.facet, key.revision, item.digest)
	}
	return out.Bytes(), nil
}

func validateCompatibility(entries []entry, baseline map[historyKey]historyEntry, retirements map[historyTokenKey]retirement) error {
	if err := validateRetirements(entries, baseline, retirements); err != nil {
		return err
	}
	if err := validateHistoricalPresence(entries, baseline, retirements, false); err != nil {
		return err
	}
	definitions := entryIndex(entries)
	for _, item := range entries {
		key := historyKeyFor(item)
		if err := validateHistoricalName(baseline, key, item); err != nil {
			return err
		}
		prior, known := baseline[key]
		if !known {
			return fmt.Errorf("catalog generator: missing history for %s@%s revision %d", item.origin, item.facet, key.revision)
		}
		digest, err := semanticDigest(item, definitions)
		if err != nil {
			return err
		}
		if !sameHistoryIdentity(prior, item) || digest != prior.digest {
			return fmt.Errorf("catalog generator: semantic mutation without revision bump for %s@%s", item.origin, item.facet)
		}
	}
	return nil
}

func validateCurrentHistory(entries []entry, baseline map[historyKey]historyEntry, retirements map[historyTokenKey]retirement) error {
	if err := validateRetirements(entries, baseline, retirements); err != nil {
		return err
	}
	if err := validateHistoricalPresence(entries, baseline, retirements, false); err != nil {
		return err
	}
	definitions := entryIndex(entries)
	for _, item := range entries {
		key := historyKeyFor(item)
		if err := validateHistoricalName(baseline, key, item); err != nil {
			return err
		}
		prior, present := baseline[key]
		digest, err := semanticDigest(item, definitions)
		if err != nil {
			return err
		}
		if !present || !sameHistoryIdentity(prior, item) || prior.digest != digest {
			return fmt.Errorf("catalog generator: stale history for %s@%s", item.origin, item.facet)
		}
	}
	return nil
}

func historyKeyFor(item entry) historyKey {
	return historyKey{origin: item.originValue, facet: item.facetValue, revision: item.revision}
}

func entryIndex(entries []entry) map[ref]entry {
	definitions := make(map[ref]entry, len(entries))
	for _, item := range entries {
		definitions[ref{origin: item.origin, facet: item.facet}] = item
	}
	return definitions
}

func sameHistoryIdentity(history historyEntry, item entry) bool {
	return history.originName == item.origin && history.facetName == item.facet
}

func validateHistoricalNames(baseline map[historyKey]historyEntry) error {
	byToken := make(map[historyTokenKey]historyEntry, len(baseline))
	byName := make(map[historyNameKey]historyTokenKey, len(baseline))
	for key, item := range baseline {
		token := historyTokenKey{origin: key.origin, facet: key.facet}
		name := historyNameKey{origin: item.originName, facet: item.facetName}
		if prior, exists := byToken[token]; exists && (prior.originName != item.originName || prior.facetName != item.facetName) {
			return fmt.Errorf("catalog generator: renamed numeric history relation origin=0x%08X facet=%d", key.origin, key.facet)
		}
		if prior, exists := byName[name]; exists && prior != token {
			return fmt.Errorf("catalog generator: reused historical relation name %s@%s", item.originName, item.facetName)
		}
		byToken[token] = item
		byName[name] = token
	}
	return nil
}

func validateHistoricalName(baseline map[historyKey]historyEntry, key historyKey, item entry) error {
	for priorKey, prior := range baseline {
		if priorKey.origin == key.origin && priorKey.facet == key.facet && !sameHistoryIdentity(prior, item) {
			return fmt.Errorf("catalog generator: renamed numeric history relation origin=0x%08X facet=%d", key.origin, key.facet)
		}
		if prior.originName == item.origin && prior.facetName == item.facet && (priorKey.origin != key.origin || priorKey.facet != key.facet) {
			return fmt.Errorf("catalog generator: reused historical relation name %s@%s", item.origin, item.facet)
		}
	}
	return nil
}

func validateHistoricalPresence(entries []entry, baseline map[historyKey]historyEntry, retirements map[historyTokenKey]retirement, allowAdvance bool) error {
	current := make(map[historyTokenKey]entry, len(entries))
	for _, item := range entries {
		current[historyTokenKey{origin: item.originValue, facet: item.facetValue}] = item
	}
	maximum := make(map[historyTokenKey]uint16, len(baseline))
	for key := range baseline {
		token := historyTokenKey{origin: key.origin, facet: key.facet}
		if key.revision > maximum[token] {
			maximum[token] = key.revision
		}
	}
	for token, revision := range maximum {
		item, exists := current[token]
		if !exists {
			if _, retired := retirements[token]; retired {
				continue
			}
			return fmt.Errorf("catalog generator: historical relation disappeared origin=0x%08X facet=%d revision %d", token.origin, token.facet, revision)
		}
		if _, retired := retirements[token]; retired {
			return fmt.Errorf("catalog generator: retired relation revived origin=0x%08X facet=%d", token.origin, token.facet)
		}
		if item.revision < revision {
			return fmt.Errorf("catalog generator: historical relation revision rollback origin=0x%08X facet=%d current %d historical %d", token.origin, token.facet, item.revision, revision)
		}
		if !allowAdvance && item.revision > revision {
			return fmt.Errorf("catalog generator: historical relation revision lacks history origin=0x%08X facet=%d current %d historical %d", token.origin, token.facet, item.revision, revision)
		}
	}
	return nil
}

// validateRetirements makes deletion a one-way catalog state transition. A
// row is retireable only at the final, exact history identity; it then reserves
// both the numeric token and the generated name forever.
func validateRetirements(entries []entry, baseline map[historyKey]historyEntry, retirements map[historyTokenKey]retirement) error {
	currentByToken := make(map[historyTokenKey]entry, len(entries))
	currentByName := make(map[historyNameKey]historyTokenKey, len(entries))
	for _, item := range entries {
		token := historyTokenKey{origin: item.originValue, facet: item.facetValue}
		currentByToken[token] = item
		currentByName[historyNameKey{origin: item.origin, facet: item.facet}] = token
	}
	maximum := make(map[historyTokenKey]uint16, len(baseline))
	for key := range baseline {
		token := historyTokenKey{origin: key.origin, facet: key.facet}
		if key.revision > maximum[token] {
			maximum[token] = key.revision
		}
	}
	for token, retired := range retirements {
		prior, exists := baseline[retired.key]
		if !exists || prior != retired.entry {
			return fmt.Errorf("catalog generator: retired relation lacks exact final history origin=0x%08X facet=%d revision %d", token.origin, token.facet, retired.key.revision)
		}
		if maximum[token] != retired.key.revision {
			return fmt.Errorf("catalog generator: retired relation does not name final history revision origin=0x%08X facet=%d retired %d final %d", token.origin, token.facet, retired.key.revision, maximum[token])
		}
		if _, active := currentByToken[token]; active {
			return fmt.Errorf("catalog generator: retired relation remains active origin=0x%08X facet=%d", token.origin, token.facet)
		}
		name := historyNameKey{origin: retired.entry.originName, facet: retired.entry.facetName}
		if active, exists := currentByName[name]; exists {
			return fmt.Errorf("catalog generator: retired relation name revived %s@%s at origin=0x%08X facet=%d", name.origin, name.facet, active.origin, active.facet)
		}
	}
	return nil
}

func semanticDigest(item entry, definitions map[ref]entry) (string, error) {
	hash := sha256.New()
	writeSemanticPart := func(value string) {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	writeSemanticPart(item.origin)
	writeSemanticPart(strconv.FormatUint(uint64(item.originValue), 10))
	writeSemanticPart(item.facet)
	writeSemanticPart(strconv.FormatUint(uint64(item.facetValue), 10))
	writeSemanticPart(item.owner)
	writeSemanticPart(item.form)
	type parentIdentity struct {
		origin      string
		originValue uint32
		facet       string
		facetValue  uint16
		revision    uint16
	}
	parents := make([]parentIdentity, 0, len(item.parents))
	for _, parent := range item.parents {
		definition, exists := definitions[parent]
		if !exists {
			return "", fmt.Errorf("catalog generator: unknown parent for semantic digest %s@%s", item.origin, item.facet)
		}
		parents = append(parents, parentIdentity{
			origin:      definition.origin,
			originValue: definition.originValue,
			facet:       definition.facet,
			facetValue:  definition.facetValue,
			revision:    definition.revision,
		})
	}
	sort.Slice(parents, func(left, right int) bool {
		if parents[left].originValue != parents[right].originValue {
			return parents[left].originValue < parents[right].originValue
		}
		if parents[left].facetValue != parents[right].facetValue {
			return parents[left].facetValue < parents[right].facetValue
		}
		if parents[left].revision != parents[right].revision {
			return parents[left].revision < parents[right].revision
		}
		if parents[left].origin != parents[right].origin {
			return parents[left].origin < parents[right].origin
		}
		return parents[left].facet < parents[right].facet
	})
	for _, parent := range parents {
		writeSemanticPart(parent.origin)
		writeSemanticPart(strconv.FormatUint(uint64(parent.originValue), 10))
		writeSemanticPart(parent.facet)
		writeSemanticPart(strconv.FormatUint(uint64(parent.facetValue), 10))
		writeSemanticPart(strconv.FormatUint(uint64(parent.revision), 10))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func emitSemanticSource(entries []entry) ([]byte, error) {
	type namedOrigin struct {
		name  string
		value uint32
	}
	type namedFacet struct {
		name  string
		value uint16
	}
	originValues := map[string]uint32{}
	facetValues := map[string]uint16{}
	for _, item := range entries {
		originValues[item.origin] = item.originValue
		if item.facet != "-" {
			facetValues[item.facet] = item.facetValue
		}
	}
	origins := make([]namedOrigin, 0, len(originValues))
	for name, value := range originValues {
		origins = append(origins, namedOrigin{name, value})
	}
	sort.Slice(origins, func(i, j int) bool { return origins[i].value < origins[j].value })
	facets := make([]namedFacet, 0, len(facetValues))
	for name, value := range facetValues {
		facets = append(facets, namedFacet{name, value})
	}
	sort.Slice(facets, func(i, j int) bool { return facets[i].name < facets[j].name })
	var out bytes.Buffer
	out.WriteString("// Code generated from program/internal/schema/relations/catalog.schema; DO NOT EDIT.\n\npackage semanticsource\n\nimport \"sync\"\n\nconst (\n")
	for _, origin := range origins {
		fmt.Fprintf(&out, "\tOrigin%s Origin = 0x%08X\n", origin.name, origin.value)
	}
	out.WriteString(")\n\nconst (\n")
	for _, facet := range facets {
		fmt.Fprintf(&out, "\tFacet%s Facet = %d\n", facet.name, facet.value)
	}
	out.WriteString(")\n\nvar catalogDefinitions = [...]RelationDef{\n")
	for _, item := range entries {
		fmt.Fprintf(&out, "\t{token: Token{origin: Origin%s, facet: %s, revision: Revision(%d), digest: 0x%016X}, seal: relationDefinitionSeal},\n", item.origin, sourceFacetExpr(item.facet), item.revision, sourceTokenDigest(item.originValue, item.facetValue, item.revision))
	}
	out.WriteString("}\n\nvar (\n\tcatalogSchemaOnce sync.Once\n\tcatalogSchema     Schema\n)\n\nfunc CatalogSchema() Schema {\n\tcatalogSchemaOnce.Do(func() {\n\t\tcatalogSchema = issuedSchema(catalogDefinitions[:]...)\n\t})\n\treturn catalogSchema\n}\n\n// catalogDefinition is generated from the sole catalog source. It returns a\n// value copy from private immutable storage, so repeated hot lookups neither\n// materialize Schema.Definitions nor recompute token identities.\nfunc catalogDefinition(origin Origin, facet Facet) (RelationDef, bool) {\n\tswitch origin {\n")
	byOrigin := make(map[string][]int, len(entries))
	order := make([]string, 0, len(entries))
	for index, item := range entries {
		if _, exists := byOrigin[item.origin]; !exists {
			order = append(order, item.origin)
		}
		byOrigin[item.origin] = append(byOrigin[item.origin], index)
	}
	for _, origin := range order {
		fmt.Fprintf(&out, "\tcase Origin%s:\n\t\tswitch facet {\n", origin)
		for _, index := range byOrigin[origin] {
			item := entries[index]
			fmt.Fprintf(&out, "\t\tcase %s:\n\t\t\treturn catalogDefinitions[%d], true\n", sourceFacetExpr(item.facet), index)
		}
		out.WriteString("\t\t}\n")
	}
	out.WriteString("\t}\n\treturn RelationDef{}, false\n}\n")
	return format.Source(out.Bytes())
}

func sourceTokenDigest(origin uint32, facet uint16, revision uint16) uint64 {
	var data [8]byte
	binary.BigEndian.PutUint32(data[0:4], origin)
	binary.BigEndian.PutUint16(data[4:6], facet)
	binary.BigEndian.PutUint16(data[6:8], revision)
	identity := sha256.Sum256(data[:])
	return binary.BigEndian.Uint64(identity[:8])
}

func emitRelations(entries []entry) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("// Code generated from catalog.schema; DO NOT EDIT.\n\npackage relations\n\nimport (\n\t\"sync\"\n\n\t\"github.com/wippyai/go-lua/program/semanticsource\"\n)\n\nfunc catalogDefinition(origin semanticsource.Origin, facet semanticsource.Facet) semanticsource.RelationDef {\n\tdefinition, _ := semanticsource.Definition(origin, facet)\n\treturn definition\n}\n\n// CatalogToken resolves one generated schema name for cold claims/codegen only.\nfunc CatalogToken(name string) (semanticsource.Token, bool) {\n\tswitch name {\n")
	for _, item := range entries {
		fmt.Fprintf(&out, "\tcase %q:\n\t\tdefinition := catalogDefinition(semanticsource.Origin%s, %s)\n\t\treturn definition.Token(), definition != (semanticsource.RelationDef{})\n", relationName(item), item.origin, facetExpr(item.facet))
	}
	out.WriteString("\tdefault:\n\t\treturn semanticsource.Token{}, false\n\t}\n}\n\n// CatalogName is the exact inverse of CatalogToken for issued catalog tokens.\nfunc CatalogName(token semanticsource.Token) (string, bool) {\n")
	for _, item := range entries {
		fmt.Fprintf(&out, "\tif token == catalogDefinition(semanticsource.Origin%s, %s).Token() {\n\t\treturn %q, true\n\t}\n", item.origin, facetExpr(item.facet), relationName(item))
	}
	out.WriteString("\treturn \"\", false\n}\n\nfunc CanonicalSchema() (*Schema, error) {\n\tsource := semanticsource.CatalogSchema()\n\tdefinition := catalogDefinition\n\tparents := func(definitions ...semanticsource.RelationDef) []semanticsource.Token {\n\t\ttokens := make([]semanticsource.Token, len(definitions))\n\t\tfor index, definition := range definitions {\n\t\t\ttokens[index] = definition.Token()\n\t\t}\n\t\treturn tokens\n\t}\n\treturn Seal(source, []Row{\n")
	for _, item := range entries {
		fmt.Fprintf(&out, "\t\t{Definition: definition(semanticsource.Origin%s, %s), Owner: Owner%s, Form: Form%s", item.origin, facetExpr(item.facet), item.owner, item.form)
		if len(item.parents) != 0 {
			out.WriteString(", Parents: parents(")
			for index, parent := range item.parents {
				if index != 0 {
					out.WriteString(", ")
				}
				fmt.Fprintf(&out, "definition(semanticsource.Origin%s, %s)", parent.origin, facetExpr(parent.facet))
			}
			out.WriteString(")")
		}
		out.WriteString("},\n")
	}
	out.WriteString("\t})\n}\n")
	generated := bytes.Replace(out.Bytes(), []byte("func CanonicalSchema() (*Schema, error) {"), []byte("var (\n\tcanonicalSchemaOnce sync.Once\n\tcanonicalSchema     *Schema\n\tcanonicalSchemaErr  error\n)\n\n// CanonicalSchema returns the one immutable generated relation schema. The\n// schema exposes only detached snapshots, so callers cannot mutate the\n// cached authority. Initialization errors are cached and fail closed.\nfunc CanonicalSchema() (*Schema, error) {\n\tcanonicalSchemaOnce.Do(func() {\n\t\tcanonicalSchema, canonicalSchemaErr = buildCanonicalSchema()\n\t})\n\tif canonicalSchemaErr != nil || canonicalSchema == nil {\n\t\treturn nil, canonicalSchemaErr\n\t}\n\treturn canonicalSchema, nil\n}\n\nfunc buildCanonicalSchema() (*Schema, error) {"), 1)
	return format.Source(generated)
}

func facetExpr(name string) string {
	if name == "-" {
		return "0"
	}
	return "semanticsource.Facet" + name
}

func sourceFacetExpr(name string) string {
	if name == "-" {
		return "0"
	}
	return "Facet" + name
}

func relationName(item entry) string { return item.origin + "@" + item.facet }

func write(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func fresh(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return freshBytes(path, got, want)
}

func freshBytes(path string, got, want []byte) error {
	if !bytes.Equal(got, want) {
		return fmt.Errorf("catalog generator: stale output %s", path)
	}
	return nil
}
