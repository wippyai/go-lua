package transformer

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ObservationConsumer identifies a retained result consumer.  It is part of
// demand identity, never an entry-value identity.
type ObservationConsumer string

const (
	ObservationConsumerSummaryProjection            ObservationConsumer = "summary-projection"
	ObservationConsumerDiagnosticRuleFamily         ObservationConsumer = "diagnostic-rule-families"
	ObservationConsumerDiagnosticDiscriminatedUnion ObservationConsumer = "diagnostic-discriminated-union"
	ObservationConsumerDiagnosticLifecycleResource  ObservationConsumer = "diagnostic-lifecycle-resource"
	ObservationConsumerDiagnosticNilSafetyPresence  ObservationConsumer = "diagnostic-nil-safety-presence"
	ObservationConsumerExportCode                   ObservationConsumer = "export-code"
	ObservationConsumerServiceIDEQueries            ObservationConsumer = "service-ide-queries"
)

// ObservationClass names one independently retained result surface. A
// consumer contract must name every class it reads; classes are deliberately
// transport-neutral so they can be selected at the formal-publication seam.
type ObservationClass string

const (
	ObservationClassEntryExitState    ObservationClass = "entry-exit-state"
	ObservationClassNormalReturn      ObservationClass = "normal-return"
	ObservationClassPointState        ObservationClass = "point-state"
	ObservationClassPointReachability ObservationClass = "point-reachability"
	ObservationClassNodeOutput        ObservationClass = "node-output"
	ObservationClassEdgeReachability  ObservationClass = "edge-reachability"
	ObservationClassCallOutcome       ObservationClass = "call-outcome"
	ObservationClassPathValue         ObservationClass = "path-value"
)

const (
	observationContractSummaryV1    = "summary-v1"
	observationContractFullResultV1 = "full-result-v1"
	observationContractClassesV1    = "observation-classes-v1:"
)

// ObservationContract is an immutable request for one closed observation
// result.  Its representation deliberately has no exported mutable members;
// consumers obtain a contract through one of the constructor functions and
// the union is canonicalized before tier 3 is allowed to freeze.
type ObservationContract struct {
	key       string
	consumers []ObservationConsumer
	classes   []ObservationClass
}

var fullResultV1Classes = []ObservationClass{
	ObservationClassCallOutcome,
	ObservationClassEdgeReachability,
	ObservationClassEntryExitState,
	ObservationClassNodeOutput,
	ObservationClassNormalReturn,
	ObservationClassPathValue,
	ObservationClassPointReachability,
	ObservationClassPointState,
}

var summaryV1Classes = []ObservationClass{
	ObservationClassEntryExitState,
	ObservationClassNormalReturn,
}

// FullResultV1ObservationContract returns the complete-closure request owned
// by one consumer class.  Stage 6 intentionally gives every class this same
// closure; later stages may introduce narrower closures without changing the
// transport or cache identity protocol.
func FullResultV1ObservationContract(consumer ObservationConsumer) ObservationContract {
	return ObservationContract{key: observationContractFullResultV1, consumers: []ObservationConsumer{consumer}, classes: append([]ObservationClass(nil), fullResultV1Classes...)}
}

// ObservationClassesV1Contract requests exactly the supplied retained
// surfaces for one consumer. It is the stage-8 replacement for a consumer's
// full-result-v1 dependency.
func ObservationClassesV1Contract(consumer ObservationConsumer, classes ...ObservationClass) ObservationContract {
	canonical := canonicalObservationClasses(classes)
	return ObservationContract{key: observationClassesKey(canonical), consumers: []ObservationConsumer{consumer}, classes: canonical}
}

// SummaryV1ObservationContract requests the closed summary publication
// surface. It is intentionally owned only by the summary consumer: combining
// it with any other consumer canonically widens to full-result-v1.
func SummaryV1ObservationContract() ObservationContract {
	return ObservationContract{key: observationContractSummaryV1, consumers: []ObservationConsumer{ObservationConsumerSummaryProjection}, classes: append([]ObservationClass(nil), summaryV1Classes...)}
}

// CanonicalizeObservationContracts unions enabled consumer requests.  The
// demand key intentionally excludes entry values: entry substitution happens
// only after the tier-3 dependency product is frozen.
func CanonicalizeObservationContracts(contracts ...ObservationContract) (ObservationContract, error) {
	consumers := make(map[ObservationConsumer]struct{}, len(contracts))
	classes := make(map[ObservationClass]struct{}, len(fullResultV1Classes))
	for _, contract := range contracts {
		if len(contract.consumers) == 0 || len(contract.classes) == 0 || !validObservationContractKey(contract.key) {
			return ObservationContract{}, fmt.Errorf("transformer: invalid observation contract")
		}
		if observationClassesKey(canonicalObservationClasses(contract.classes)) != contract.key &&
			!(contract.key == observationContractFullResultV1 && sameObservationClasses(canonicalObservationClasses(contract.classes), fullResultV1Classes)) &&
			!(contract.key == observationContractSummaryV1 && sameObservationClasses(canonicalObservationClasses(contract.classes), summaryV1Classes)) {
			return ObservationContract{}, fmt.Errorf("transformer: invalid observation contract class closure")
		}
		for _, consumer := range contract.consumers {
			if !validObservationConsumer(consumer) {
				return ObservationContract{}, fmt.Errorf("transformer: invalid observation consumer %q", consumer)
			}
			if contract.key == observationContractSummaryV1 && consumer != ObservationConsumerSummaryProjection {
				return ObservationContract{}, fmt.Errorf("transformer: summary observation contract has foreign consumer %q", consumer)
			}
			consumers[consumer] = struct{}{}
		}
		for _, class := range contract.classes {
			if !validObservationClass(class) {
				return ObservationContract{}, fmt.Errorf("transformer: invalid observation class %q", class)
			}
			classes[class] = struct{}{}
		}
	}
	if len(consumers) == 0 {
		return ObservationContract{}, fmt.Errorf("transformer: observation demand has no enabled consumers")
	}
	canonicalConsumers := make([]ObservationConsumer, 0, len(consumers))
	for consumer := range consumers {
		canonicalConsumers = append(canonicalConsumers, consumer)
	}
	sort.Slice(canonicalConsumers, func(i, j int) bool { return canonicalConsumers[i] < canonicalConsumers[j] })
	canonicalClasses := make([]ObservationClass, 0, len(classes))
	for class := range classes {
		canonicalClasses = append(canonicalClasses, class)
	}
	canonicalClasses = canonicalObservationClasses(canonicalClasses)
	key := observationClassesKey(canonicalClasses)
	if sameObservationClasses(canonicalClasses, fullResultV1Classes) {
		key = observationContractFullResultV1
	} else if sameObservationClasses(canonicalClasses, summaryV1Classes) && len(canonicalConsumers) == 1 && canonicalConsumers[0] == ObservationConsumerSummaryProjection {
		key = observationContractSummaryV1
	}
	return ObservationContract{key: key, consumers: canonicalConsumers, classes: canonicalClasses}, nil
}

func validObservationContractKey(key string) bool {
	return key == observationContractSummaryV1 || key == observationContractFullResultV1 || strings.HasPrefix(key, observationContractClassesV1)
}

func validObservationConsumer(consumer ObservationConsumer) bool {
	switch consumer {
	case ObservationConsumerSummaryProjection, ObservationConsumerDiagnosticRuleFamily,
		ObservationConsumerDiagnosticDiscriminatedUnion, ObservationConsumerDiagnosticLifecycleResource,
		ObservationConsumerDiagnosticNilSafetyPresence,
		ObservationConsumerExportCode, ObservationConsumerServiceIDEQueries:
		return true
	default:
		return false
	}
}

func validObservationClass(class ObservationClass) bool {
	switch class {
	case ObservationClassEntryExitState, ObservationClassNormalReturn, ObservationClassPointState,
		ObservationClassPointReachability, ObservationClassNodeOutput, ObservationClassEdgeReachability,
		ObservationClassCallOutcome, ObservationClassPathValue:
		return true
	default:
		return false
	}
}

func canonicalObservationClasses(classes []ObservationClass) []ObservationClass {
	seen := make(map[ObservationClass]struct{}, len(classes))
	for _, class := range classes {
		seen[class] = struct{}{}
	}
	out := make([]ObservationClass, 0, len(seen))
	for class := range seen {
		out = append(out, class)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func observationClassesKey(classes []ObservationClass) string {
	parts := make([]string, len(classes))
	for index, class := range classes {
		parts[index] = string(class)
	}
	return observationContractClassesV1 + strings.Join(parts, "+")
}

func sameObservationClasses(left, right []ObservationClass) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (c ObservationContract) valid() bool {
	canonical, err := CanonicalizeObservationContracts(c)
	return err == nil && canonical.key == c.key && sameObservationConsumers(canonical.consumers, c.consumers) && sameObservationClasses(canonical.classes, c.classes)
}

func sameObservationConsumers(left, right []ObservationConsumer) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// Key is the stable demand/cache identity.  It is deliberately structural and
// does not encode a concrete State or any other entry value.
func (c ObservationContract) Key() string { return c.key }

// Consumers returns a detached canonical consumer inventory.
func (c ObservationContract) Consumers() []ObservationConsumer {
	return append([]ObservationConsumer(nil), c.consumers...)
}

// Classes returns the detached, canonical retained-surface inventory.
func (c ObservationContract) Classes() []ObservationClass {
	return append([]ObservationClass(nil), c.classes...)
}

// FullResultV1 reports whether this contract requests the complete stage-6
// closure.
func (c ObservationContract) FullResultV1() bool {
	return c.valid() && c.key == observationContractFullResultV1
}

// SummaryV1 reports whether this contract requests only the closed summary
// surface. A caller requiring any point-state reader must declare a wider
// contract rather than relying on a retry.
func (c ObservationContract) SummaryV1() bool {
	return c.valid() && c.key == observationContractSummaryV1
}

// ObservationCoverageError is a shaped, non-retryable failure.  A provider or
// evaluator must never widen demand by retrying a full freeze after this error.
type ObservationCoverageError struct {
	Consumer  ObservationConsumer
	Provider  string
	DemandKey string
}

func (e *ObservationCoverageError) Error() string {
	return fmt.Sprintf("transformer: observation coverage violation: provider %q read outside declared closure for %q (demand %q)", e.Provider, e.Consumer, e.DemandKey)
}

// IsObservationCoverageError permits callers to preserve the fail-closed
// behavior without matching presentation text.
func IsObservationCoverageError(err error) bool {
	var coverage *ObservationCoverageError
	return errors.As(err, &coverage)
}

type observationCoverageGuard struct{ demand ObservationContract }

func newObservationCoverageGuard(demand ObservationContract) (observationCoverageGuard, error) {
	if !demand.valid() {
		return observationCoverageGuard{}, fmt.Errorf("transformer: observation coverage has no declared closure")
	}
	return observationCoverageGuard{demand: demand}, nil
}

func (g observationCoverageGuard) require(consumer ObservationConsumer, provider string) error {
	for _, declared := range g.demand.consumers {
		if declared == consumer {
			return nil
		}
	}
	return &ObservationCoverageError{Consumer: consumer, Provider: strings.TrimSpace(provider), DemandKey: g.demand.key}
}

func (g observationCoverageGuard) requireClass(consumer ObservationConsumer, class ObservationClass, provider string) error {
	if err := g.require(consumer, provider); err != nil {
		return err
	}
	for _, declared := range g.demand.classes {
		if declared == class {
			return nil
		}
	}
	return &ObservationCoverageError{Consumer: consumer, Provider: strings.TrimSpace(provider), DemandKey: g.demand.key}
}
