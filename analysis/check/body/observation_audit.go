package body

import (
	"fmt"
	"os"
	"sync"
)

// ObservationAuditContract is the transport-neutral declared closure for one
// post-solve consumer. It intentionally uses strings so body remains below
// the transformer contract package in the import graph.
type ObservationAuditContract struct {
	Consumer string
	Classes  []string
}

// ObservationAudit is an opt-in, read-only recorder for contract-completion
// work. It never changes a publication or rejects a read: callers use it to
// compare the complete publication with the closure each consumer declared.
type ObservationAudit struct {
	declared map[string]map[string]struct{}

	scopeMu sync.Mutex
	readsMu sync.Mutex
	reads   map[observationAuditRead]struct{}
}

type observationAuditRead struct {
	consumer string
	class    string
	body     string
	point    int
}

// NewObservationAudit returns nil unless GOLUA_CONTRACT_AUDIT=1. A nil audit
// keeps the ordinary result read-model allocation- and behavior-equivalent.
func NewObservationAudit(contracts ...ObservationAuditContract) *ObservationAudit {
	if os.Getenv("GOLUA_CONTRACT_AUDIT") != "1" {
		return nil
	}
	audit := &ObservationAudit{
		declared: make(map[string]map[string]struct{}, len(contracts)),
		reads:    make(map[observationAuditRead]struct{}),
	}
	for _, contract := range contracts {
		if contract.Consumer == "" {
			continue
		}
		classes := audit.declared[contract.Consumer]
		if classes == nil {
			classes = make(map[string]struct{}, len(contract.Classes))
			audit.declared[contract.Consumer] = classes
		}
		for _, class := range contract.Classes {
			classes[class] = struct{}{}
		}
	}
	return audit
}

// AttachObservationAuditTree gives every result reachable from root the same
// per-solve recorder. The recorder is discarded together with the result tree,
// which keeps fixture audit runs bounded to one fixture at a time.
func AttachObservationAuditTree(root *Result, audit *ObservationAudit) {
	if root == nil || audit == nil {
		return
	}
	seen := make(map[*Result]struct{})
	var attach func(*Result)
	attach = func(result *Result) {
		if result == nil {
			return
		}
		if _, duplicate := seen[result]; duplicate {
			return
		}
		seen[result] = struct{}{}
		result.observationAudit = audit
		for _, child := range result.functions {
			attach(child)
		}
	}
	attach(root)
}

// WithObservationAuditConsumer attributes all Result reads in run, including
// reads through nested lexical-body Results, to consumer. Consumer scopes are
// serialized only in audit mode; normal execution has no synchronization.
func WithObservationAuditConsumer(root *Result, consumer string, run func()) {
	if root == nil || root.observationAudit == nil || consumer == "" {
		run()
		return
	}
	audit := root.observationAudit
	audit.scopeMu.Lock()
	defer audit.scopeMu.Unlock()
	seen := make(map[*Result]struct{})
	previous := make(map[*Result]string)
	var set func(*Result)
	set = func(result *Result) {
		if result == nil || result.observationAudit != audit {
			return
		}
		if _, duplicate := seen[result]; duplicate {
			return
		}
		seen[result] = struct{}{}
		previous[result] = result.observationConsumer
		result.observationConsumer = consumer
		for _, child := range result.functions {
			set(child)
		}
	}
	set(root)
	defer func() {
		for result, prior := range previous {
			result.observationConsumer = prior
		}
	}()
	run()
}

func (r *Result) auditObservation(class string, point int) {
	if r == nil || r.observationAudit == nil || r.observationConsumer == "" {
		return
	}
	r.observationAudit.record(r.observationConsumer, class, r.StableLexicalBodyID().String(), point)
}

func (a *ObservationAudit) record(consumer, class, body string, point int) {
	if a == nil || consumer == "" || class == "" {
		return
	}
	if classes := a.declared[consumer]; classes != nil {
		if _, declared := classes[class]; declared {
			return
		}
	}
	read := observationAuditRead{consumer: consumer, class: class, body: body, point: point}
	a.readsMu.Lock()
	defer a.readsMu.Unlock()
	if _, seen := a.reads[read]; seen {
		return
	}
	a.reads[read] = struct{}{}
	fmt.Fprintf(os.Stderr, "GOLUA_CONTRACT_AUDIT consumer=%s class=%s body=%s point=%d\n", consumer, class, body, point)
}
