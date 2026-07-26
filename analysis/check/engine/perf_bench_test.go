package engine_test

// This file pins the allocation pathology quantified by the edge-matrix
// profile (the Stage-3 partition read copy at 68.7% of alloc_space,
// joinClosure, guard-cube canonicalization, strings.genSplit key parsing,
// engine.declaredTypeForTerm scans) at the whole-program entry point:
// engine.Check and lint.CheckProject.
//
// Run (compare with benchstat; allocs/op is the primary regression gate —
// sec/op swings tens of percent under machine load with zero code change,
// while B/op and allocs/op reproduce byte-identically). Two trees are only
// comparable in sec/op when their runs are interleaved, because the machine
// drifts by more than 20% over the span of one full sweep:
//
//	go test ./analysis/check/engine/... ./analysis/check/fixpoint/equation/... \
//	  -run '^$' -bench . -benchmem -count=10 | tee new.txt
//	benchstat old.txt new.txt
//
// Baseline (minimum of 10 reps, the least load-contaminated sample):
//
//	CheckSmall                    6.658ms   5.477Mi   51.54k allocs
//	CheckBranchy                  61.84ms   55.03Mi   611.4k allocs
//	CheckLoopy                    25.30ms   22.88Mi   140.5k allocs
//	CheckTableHeavy               841.3µs   807.8Ki   6.418k allocs
//	FullFixture_EdgeMatrix        3.013s    3.824Gi   46.61M allocs
//	FullFixture_PluginSupervisor  899.8ms   1.436Gi   5.000M allocs
//
// EdgeMatrix still allocates 3.82GiB / 46.6M allocs to check one 48KB source
// file. The persistent partition view took the per-read fact copy out of that
// figure, which is what the halved B/op measures; the remaining allocation
// count is dominated by engine-side fact construction, not by the partition
// reads isolated in fixpoint/equation/partition_bench_test.go.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/lint"
)

// benchRepositoryRoot mirrors corpusRepositoryRoot (corpus_root_test.go) but
// accepts *testing.B: benchmarks never receive a *testing.T.
func benchRepositoryRoot(b *testing.B) string {
	b.Helper()
	dir, err := os.Getwd()
	if err != nil {
		b.Fatalf("get working directory: %v", err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil && !info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			b.Fatal("could not find repository go.mod")
		}
		dir = parent
	}
}

func readBenchFile(b *testing.B, path string) string {
	b.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("read fixture %s: %v", path, err)
	}
	return string(data)
}

const benchSmallSource = `type Point = {x: number, y: number}

local function make_point(x: number, y: number): Point
    return {x = x, y = y}
end

local function add(a: Point, b: Point): Point
    return {x = a.x + b.x, y = a.y + b.y}
end

local function scale(p: Point, factor: number): Point
    return {x = p.x * factor, y = p.y * factor}
end

local function distance_sq(a: Point, b: Point): number
    local dx = a.x - b.x
    local dy = a.y - b.y
    return dx * dx + dy * dy
end

local origin: Point = make_point(0, 0)
local unit: Point = make_point(1, 1)
local sum: Point = add(origin, unit)
local doubled: Point = scale(sum, 2)
local dist: number = distance_sq(origin, doubled)

local function label(p: Point): string
    return "(" .. tostring(p.x) .. ", " .. tostring(p.y) .. ")"
end

local rendered: string = label(doubled)
`

const benchBranchySource = `type Admin = {kind: "admin", id: string, level: number}
type Guest = {kind: "guest", id: string, expires: number}
type Banned = {kind: "banned", id: string, reason: string}
type Principal = Admin | Guest | Banned

local function classify(p: Principal): string
    if p.kind == "admin" then
        if p.level >= 10 then
            return "superadmin:" .. p.id
        elseif p.level >= 5 then
            return "admin:" .. p.id
        else
            return "junior-admin:" .. p.id
        end
    elseif p.kind == "guest" then
        if p.expires > 0 then
            return "guest:" .. p.id
        else
            return "expired-guest:" .. p.id
        end
    else
        return "banned:" .. p.id .. ":" .. p.reason
    end
end

local function level_or_default(p: Principal, default: number): number
    if p.kind == "admin" then
        return p.level
    end
    return default
end

local function describe(p: Principal, verbose: boolean): string
    local base = classify(p)
    if verbose then
        if p.kind == "admin" then
            return base .. " level=" .. tostring(p.level)
        elseif p.kind == "guest" then
            return base .. " expires=" .. tostring(p.expires)
        else
            return base .. " reason=" .. p.reason
        end
    end
    return base
end

local admin: Admin = {kind = "admin", id = "a1", level = 7}
local guest: Guest = {kind = "guest", id = "g1", expires = 3600}
local banned: Banned = {kind = "banned", id = "b1", reason = "abuse"}

local principals: {Principal} = {admin, guest, banned}

local function summarize(list: {Principal}): string
    local out = ""
    for _, p in ipairs(list) do
        if p.kind == "admin" and p.level >= 5 then
            out = out .. describe(p, true) .. ";"
        elseif p.kind == "guest" and p.expires > 0 then
            out = out .. describe(p, false) .. ";"
        else
            out = out .. classify(p) .. ";"
        end
    end
    return out
end

local report: string = summarize(principals)
`

const benchLoopySource = `local function sum_range(n: integer): integer
    local total: integer = 0
    for i = 1, n do
        total = total + i
    end
    return total
end

local function sum_array(values: {number}): number
    local total: number = 0
    for _, v in ipairs(values) do
        total = total + v
    end
    return total
end

local function count_positive(values: {number}): integer
    local count: integer = 0
    local i = 1
    while i <= #values do
        if values[i] > 0 then
            count = count + 1
        end
        i = i + 1
    end
    return count
end

local function nested_sum(matrix: {{number}}): number
    local total: number = 0
    for _, row in ipairs(matrix) do
        for _, cell in ipairs(row) do
            total = total + cell
        end
    end
    return total
end

local function build_squares(n: integer): {integer}
    local out: {integer} = {}
    for i = 1, n do
        out[i] = i * i
    end
    return out
end

local function first_over(values: {number}, threshold: number): number?
    for _, v in ipairs(values) do
        if v > threshold then
            return v
        end
    end
    return nil
end

local samples: {number} = {1, -2, 3, -4, 5, 6, -7, 8}
local matrix: {{number}} = {{1, 2}, {3, 4}, {5, 6}}

local range_total: integer = sum_range(50)
local array_total: number = sum_array(samples)
local positives: integer = count_positive(samples)
local matrix_total: number = nested_sum(matrix)
local squares: {integer} = build_squares(10)
local found: number? = first_over(samples, 4)
`

const benchTableHeavySource = `type Address = {street: string, city: string, zip: string}
type Contact = {email: string, phone: string?}
type Employee = {
    id: string,
    name: string,
    address: Address,
    contact: Contact,
    manager_id: string?,
}
type Department = {
    name: string,
    employees: {[string]: Employee},
    lead_id: string,
}
type Company = {
    departments: {[string]: Department},
    employee_index: {[string]: Employee},
}

local function make_address(street: string, city: string, zip: string): Address
    return {street = street, city = city, zip = zip}
end

local function make_employee(id: string, name: string, addr: Address, email: string): Employee
    return {id = id, name = name, address = addr, contact = {email = email, phone = nil}, manager_id = nil}
end

local hq: Address = make_address("1 Main St", "Springfield", "00001")
local branch: Address = make_address("2 Side St", "Shelbyville", "00002")

local alice: Employee = make_employee("e1", "Alice", hq, "alice@example.com")
local bob: Employee = make_employee("e2", "Bob", hq, "bob@example.com")
local carol: Employee = make_employee("e3", "Carol", branch, "carol@example.com")

bob.manager_id = alice.id
carol.manager_id = alice.id

local eng: Department = {
    name = "Engineering",
    employees = {[alice.id] = alice, [bob.id] = bob},
    lead_id = alice.id,
}
local sales: Department = {
    name = "Sales",
    employees = {[carol.id] = carol},
    lead_id = carol.id,
}

local company: Company = {
    departments = {[eng.name] = eng, [sales.name] = sales},
    employee_index = {[alice.id] = alice, [bob.id] = bob, [carol.id] = carol},
}

local function lookup_employee(c: Company, id: string): Employee?
    return c.employee_index[id]
end

local function department_size(d: Department): integer
    local count: integer = 0
    for _ in pairs(d.employees) do
        count = count + 1
    end
    return count
end

local function manager_name(c: Company, emp: Employee): string?
    if emp.manager_id == nil then
        return nil
    end
    local manager = lookup_employee(c, emp.manager_id)
    if manager == nil then
        return nil
    end
    return manager.name
end

local eng_size: integer = department_size(eng)
local sales_size: integer = department_size(sales)
local bob_manager: string? = manager_name(company, bob)
local carol_manager: string? = manager_name(company, carol)
local missing: Employee? = lookup_employee(company, "nope")
`

func BenchmarkCheckSmall(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Check(benchSmallSource); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCheckBranchy(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Check(benchBranchySource); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCheckLoopy(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Check(benchLoopySource); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCheckTableHeavy(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Check(benchTableHeavySource); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFullFixture_EdgeMatrix runs the exact fixture the memprof was
// captured against (testdata/fixtures/semantic/type-engine-edge-matrix). It
// has a single file and no require()s, so plain engine.Check is the same
// path the fixture harness uses to reach it.
func BenchmarkFullFixture_EdgeMatrix(b *testing.B) {
	dir := filepath.Join(benchRepositoryRoot(b), "testdata", "fixtures", "semantic", "type-engine-edge-matrix")
	source := readBenchFile(b, filepath.Join(dir, "main.lua"))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Check(source); err != nil {
			b.Fatal(err)
		}
	}
}

// buildPluginSupervisorInput mirrors fixtureDiagnostics (fixture_harness_test.go):
// entries in manifest file order, target the last (main) module, and host
// manifests for the declared package imports via the same fixtureHostManifest
// used by the fixture oracle. The plugin-supervisor fixture requires("time")
// across several files, so its import resolution is not a trivial
// engine.Check call -- CheckProject is the harness path that resolves it.
func buildPluginSupervisorInput(b *testing.B) lint.ProjectInput {
	b.Helper()
	dir := filepath.Join(benchRepositoryRoot(b), "testdata", "fixtures", "realworld", "plugin-supervisor-runtime")
	manifestData, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		b.Fatalf("read manifest: %v", err)
	}
	var parsed struct {
		Files    []string `json:"files"`
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(manifestData, &parsed); err != nil {
		b.Fatalf("parse manifest: %v", err)
	}
	entries := make([]lint.Entry, 0, len(parsed.Files))
	for _, file := range parsed.Files {
		entries = append(entries, lint.Entry{
			Path:       file,
			ModulePath: strings.TrimSuffix(file, ".lua"),
			Source:     readBenchFile(b, filepath.Join(dir, file)),
		})
	}
	input := lint.ProjectInput{
		Entries: entries,
		Targets: []string{strings.TrimSuffix(parsed.Files[len(parsed.Files)-1], ".lua")},
	}
	for _, pkg := range parsed.Packages {
		input.Manifests = append(input.Manifests, fixtureHostManifest(pkg))
	}
	return input
}

func BenchmarkFullFixture_PluginSupervisor(b *testing.B) {
	input := buildPluginSupervisorInput(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := lint.CheckProject(context.Background(), input); err != nil {
			b.Fatal(err)
		}
	}
}
