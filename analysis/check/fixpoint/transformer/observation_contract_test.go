package transformer

import (
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestCanonicalObservationContractUnionsClassesWithoutChangingFullResultKey(t *testing.T) {
	first, err := CanonicalizeObservationContracts(
		FullResultV1ObservationContract(ObservationConsumerExportCode),
		FullResultV1ObservationContract(ObservationConsumerSummaryProjection),
		FullResultV1ObservationContract(ObservationConsumerExportCode),
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalizeObservationContracts(
		FullResultV1ObservationContract(ObservationConsumerSummaryProjection),
		FullResultV1ObservationContract(ObservationConsumerExportCode),
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key() != observationContractFullResultV1 || !first.FullResultV1() ||
		!reflect.DeepEqual(first, second) ||
		!reflect.DeepEqual(first.Consumers(), []ObservationConsumer{ObservationConsumerExportCode, ObservationConsumerSummaryProjection}) {
		t.Fatalf("canonical demand = %#v / %#v", first, second)
	}
}

func TestCanonicalSummaryObservationContractNarrowsOnlyTheSummaryConsumer(t *testing.T) {
	summaryDemand, err := CanonicalizeObservationContracts(SummaryV1ObservationContract())
	if err != nil {
		t.Fatal(err)
	}
	if !summaryDemand.SummaryV1() || summaryDemand.FullResultV1() || summaryDemand.Key() != observationContractSummaryV1 {
		t.Fatalf("summary demand = %#v", summaryDemand)
	}
	mixed, err := CanonicalizeObservationContracts(
		SummaryV1ObservationContract(),
		FullResultV1ObservationContract(ObservationConsumerDiagnosticRuleFamily),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !mixed.FullResultV1() || mixed.SummaryV1() ||
		!reflect.DeepEqual(mixed.Consumers(), []ObservationConsumer{ObservationConsumerDiagnosticRuleFamily, ObservationConsumerSummaryProjection}) {
		t.Fatalf("mixed demand = %#v", mixed)
	}
}

func TestRelationProgramCoverageGuardRejectsUndeclaredConsumerWithoutRetry(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("observation-contract-coverage"))
	body := lexicalidentity.RootBody(namespace)
	demand, err := CanonicalizeObservationContracts(FullResultV1ObservationContract(ObservationConsumerSummaryProjection))
	if err != nil {
		t.Fatal(err)
	}
	telemetry := &FreezeTelemetry{}
	program, err := FreezeRelationProgramWithObservationAndTelemetry(
		[]RelationProgramUnit{formalTemplateFreezeUnit(t, body)}, testAcyclicCallTopology(t, body), demand, telemetry,
	)
	if err != nil {
		t.Fatal(err)
	}
	if telemetry.ObservationContract.Calls != 1 || telemetry.ObservationContract.Elapsed < 0 {
		t.Fatalf("observation-contract telemetry = %#v", telemetry.ObservationContract)
	}
	if err := program.RequireObservation(ObservationConsumerSummaryProjection, "summary projection"); err != nil {
		t.Fatalf("declared consumer rejected: %v", err)
	}
	err = program.RequireObservation(ObservationConsumerExportCode, "manifest exporter")
	var coverage *ObservationCoverageError
	if !errors.As(err, &coverage) || !IsObservationCoverageError(err) || coverage.Consumer != ObservationConsumerExportCode {
		t.Fatalf("undeclared access error = %#v", err)
	}
	if got := program.ObservationContract(); !reflect.DeepEqual(got, demand) {
		t.Fatalf("retained canonical demand = %#v, want %#v", got, demand)
	}
}
