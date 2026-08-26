package arrangement_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func TestExecutionSealsDependencyRootAndRecurrenceSchedule(t *testing.T) {
	value := newFixture(t)
	addresses := value.addresses(t)
	book, ok := address.Bind(value.certificate, addresses)
	if !ok {
		t.Fatal("address bind")
	}
	planValue, ok := arrangement.Derive(value.certificate, book, &arrangementInventory{fence: book.Fence(), slot: 301}, expand.EmptyCatalog(), []binding.PartitionDirectory{})
	if !ok || !planValue.Available() {
		t.Fatal("arrangement derive")
	}
	execution := planValue.Execution()
	if !execution.Available() {
		t.Fatal("execution unavailable")
	}
	schedule := execution.DependencySchedule()
	if !schedule.Available() || schedule.DependencyCount() != 1 {
		t.Fatal("dependency schedule unavailable")
	}
	record, ok := schedule.Dependency(value.dependency)
	if !ok || !record.Available() || record.ID() != value.dependency || record.Root() != value.expression || !record.Node().Available() || len(record.Reads()) != 0 || len(record.ColumnReads()) != 1 || record.ColumnReads()[0] != value.column || len(record.Writes()) != 0 || record.Component() != 0 {
		t.Fatal("dependency record did not retain the checked projection")
	}
	root, ok := execution.Entry(value.expression)
	if !ok || !record.Node().Available() || record.Node().Digest() != root.Digest() {
		t.Fatal("dependency record escaped its sealed root node")
	}
	schedules := execution.Schedules()
	if len(schedules) != 1 || schedules[0].Dependency() != record.Dependency() || schedules[0].Node().Digest() != root.Digest() {
		t.Fatal("schedule did not retain the dependency-owned physical root")
	}
	components := schedule.Components()
	if len(components) != 1 || !components[0].Available() || components[0].Order() != 0 || components[0].Recurrence() != arrangement.RecurrenceAcyclic || len(components[0].Members()) != 1 || components[0].Members()[0] != value.dependency || len(components[0].Edges()) != 0 || len(components[0].Heads()) != 0 {
		t.Fatal("sealed SCC schedule shape is wrong")
	}

	columns := record.ColumnReads()
	columns[0] = model.ColumnID{}
	if again, ok := schedule.Dependency(value.dependency); !ok || again.ColumnReads()[0] != value.column {
		t.Fatal("dependency column read set escaped through a defensive copy")
	}
	if len(schedule.WakeRelation(value.relation)) != 0 || len(schedule.WakeColumn(value.column)) != 1 {
		t.Fatal("Input vector did not install its exact column wake")
	}
	members := components[0].Members()
	members[0] = model.DependencyID{}
	if again, ok := schedule.Component(0); !ok || again.Members()[0] != value.dependency {
		t.Fatal("SCC member set escaped through a defensive copy")
	}
	if _, ok := schedule.Dependency(model.DependencyID{}); ok {
		t.Fatal("zero dependency lookup accepted")
	}
	if zero := (arrangement.Execution{}).DependencySchedule(); zero.Available() {
		t.Fatal("zero execution redeemed a dependency schedule")
	}
}
