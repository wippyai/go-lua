package grammarproof

import (
	"encoding/hex"
	"fmt"
)

// Production is one actual yacc alternative and one accepted source that made
// the temporary generated parser reduce it. Key is nonterminal#ordinal,
// exactly matching the parser-grammar extraction.
type Production struct {
	Key     string
	Witness string
}

// Evidence is generated, checked-in, cold proof material. It is intentionally
// not a Program term and never participates in parsing or lowering.
type Evidence struct {
	Digest        string
	TraceDigest   string
	IngressDigest string
	Productions   []Production
	Ingress       []Ingress
}

// Validate checks the generated parser-reduction and public-ingress ledgers
// against the independently extracted live grammar and complete source corpus.
func (e Evidence) Validate(live []liveProduction, sources []source, digest, traceDigest string) error {
	if e.Digest != digest {
		return fmt.Errorf("grammar reduction evidence is stale: run go generate ./program/internal/grammarproof")
	}
	if e.TraceDigest != traceDigest {
		return fmt.Errorf("grammar reduction trace inputs are stale: run go generate ./program/internal/grammarproof")
	}
	if e.IngressDigest != ingressDigest(e.Ingress) {
		return fmt.Errorf("grammar ingress evidence has an invalid digest")
	}
	if len(e.Ingress) != len(sources) {
		return fmt.Errorf("grammar ingress evidence has %d sources, corpus has %d", len(e.Ingress), len(sources))
	}
	corpus := make(map[string]bool, len(sources))
	for _, source := range sources {
		if source.id == "" || corpus[source.id] {
			return fmt.Errorf("grammar ingress validation has an invalid corpus source")
		}
		corpus[source.id] = true
	}
	ingress := make(map[string]Ingress, len(e.Ingress))
	for _, row := range e.Ingress {
		if row.Source == "" || row.ProgramID == "" {
			return fmt.Errorf("grammar ingress evidence contains an incomplete row")
		}
		if !corpus[row.Source] {
			return fmt.Errorf("grammar ingress evidence contains non-corpus source %s", row.Source)
		}
		if _, err := hex.DecodeString(row.ProgramID); err != nil || len(row.ProgramID) != 64 {
			return fmt.Errorf("grammar ingress evidence has invalid Program identity for %s", row.Source)
		}
		if _, exists := ingress[row.Source]; exists {
			return fmt.Errorf("grammar ingress evidence duplicates source %s", row.Source)
		}
		ingress[row.Source] = row
	}
	for source := range corpus {
		if _, exists := ingress[source]; !exists {
			return fmt.Errorf("grammar ingress evidence lacks corpus source %s", source)
		}
	}
	if len(e.Productions) != len(live) {
		return fmt.Errorf("grammar reduction evidence has %d productions, live grammar has %d", len(e.Productions), len(live))
	}
	byKey := make(map[string]Production, len(e.Productions))
	for _, production := range e.Productions {
		if production.Key == "" {
			return fmt.Errorf("grammar reduction evidence contains an incomplete production")
		}
		if production.Witness == "" {
			return fmt.Errorf("grammar production %s has no accepted reduction witness", production.Key)
		}
		if _, exists := byKey[production.Key]; exists {
			return fmt.Errorf("grammar reduction evidence duplicates %s", production.Key)
		}
		byKey[production.Key] = production
	}
	for _, production := range live {
		witness, exists := byKey[production.key]
		if !exists {
			return fmt.Errorf("live grammar production %s lacks reduction evidence", production.key)
		}
		if _, exists := ingress[witness.Witness]; !exists {
			return fmt.Errorf("grammar production %s cites witness %s outside public ingress", production.key, witness.Witness)
		}
	}
	return nil
}

// ValidateGenerated rejects an upstream generated artifact that differs from
// this freshly collected snapshot. Matrix generation calls this before it
// consumes traces, which prevents a new matrix from silently certifying stale
// parser or Program ingress evidence.
func (s Snapshot) ValidateGenerated() error {
	if !sameEvidence(Generated, s.Evidence) {
		return fmt.Errorf("grammar proof evidence is stale: run go generate ./program/internal/grammarproof")
	}
	return nil
}

func sameEvidence(left, right Evidence) bool {
	if left.Digest != right.Digest || left.TraceDigest != right.TraceDigest || left.IngressDigest != right.IngressDigest ||
		len(left.Productions) != len(right.Productions) || len(left.Ingress) != len(right.Ingress) {
		return false
	}
	for index := range left.Productions {
		if left.Productions[index] != right.Productions[index] {
			return false
		}
	}
	for index := range left.Ingress {
		if left.Ingress[index] != right.Ingress[index] {
			return false
		}
	}
	return true
}
