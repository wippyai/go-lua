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
	ObservationConsumerSummaryProjection    ObservationConsumer = "summary-projection"
	ObservationConsumerDiagnosticRuleFamily ObservationConsumer = "diagnostic-rule-families"
	ObservationConsumerExportCode           ObservationConsumer = "export-code"
	ObservationConsumerServiceIDEQueries    ObservationConsumer = "service-ide-queries"
)

const (
	observationContractSummaryV1    = "summary-v1"
	observationContractFullResultV1 = "full-result-v1"
)

// ObservationContract is an immutable request for one closed observation
// result.  Its representation deliberately has no exported mutable members;
// consumers obtain a contract through one of the constructor functions and
// the union is canonicalized before tier 3 is allowed to freeze.
type ObservationContract struct {
	key       string
	consumers []ObservationConsumer
}

// FullResultV1ObservationContract returns the complete-closure request owned
// by one consumer class.  Stage 6 intentionally gives every class this same
// closure; later stages may introduce narrower closures without changing the
// transport or cache identity protocol.
func FullResultV1ObservationContract(consumer ObservationConsumer) ObservationContract {
	return ObservationContract{key: observationContractFullResultV1, consumers: []ObservationConsumer{consumer}}
}

// SummaryV1ObservationContract requests the closed summary publication
// surface. It is intentionally owned only by the summary consumer: combining
// it with any other consumer canonically widens to full-result-v1.
func SummaryV1ObservationContract() ObservationContract {
	return ObservationContract{key: observationContractSummaryV1, consumers: []ObservationConsumer{ObservationConsumerSummaryProjection}}
}

// CanonicalizeObservationContracts unions enabled consumer requests.  The
// demand key intentionally excludes entry values: entry substitution happens
// only after the tier-3 dependency product is frozen.
func CanonicalizeObservationContracts(contracts ...ObservationContract) (ObservationContract, error) {
	consumers := make(map[ObservationConsumer]struct{}, len(contracts))
	summaryOnly := len(contracts) != 0
	for _, contract := range contracts {
		if (contract.key != observationContractSummaryV1 && contract.key != observationContractFullResultV1) || len(contract.consumers) == 0 {
			return ObservationContract{}, fmt.Errorf("transformer: invalid observation contract")
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
		if contract.key != observationContractSummaryV1 {
			summaryOnly = false
		}
	}
	if len(consumers) == 0 {
		return ObservationContract{}, fmt.Errorf("transformer: observation demand has no enabled consumers")
	}
	canonical := make([]ObservationConsumer, 0, len(consumers))
	for consumer := range consumers {
		canonical = append(canonical, consumer)
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i] < canonical[j] })
	if summaryOnly && len(canonical) == 1 && canonical[0] == ObservationConsumerSummaryProjection {
		return ObservationContract{key: observationContractSummaryV1, consumers: canonical}, nil
	}
	return ObservationContract{key: observationContractFullResultV1, consumers: canonical}, nil
}

func validObservationConsumer(consumer ObservationConsumer) bool {
	switch consumer {
	case ObservationConsumerSummaryProjection, ObservationConsumerDiagnosticRuleFamily,
		ObservationConsumerExportCode, ObservationConsumerServiceIDEQueries:
		return true
	default:
		return false
	}
}

func (c ObservationContract) valid() bool {
	canonical, err := CanonicalizeObservationContracts(c)
	return err == nil && canonical.key == c.key && sameObservationConsumers(canonical.consumers, c.consumers)
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
