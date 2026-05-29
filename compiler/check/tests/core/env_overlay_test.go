package core

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// migrationManifest creates a manifest for a migration DSL module.
// The define function accepts a callback and injects "up" and "down"
// as callback-scoped globals via EnvOverlay.
func migrationManifest() *io.Manifest {
	upFn := typ.Func().
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Build()

	downFn := typ.Func().
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Build()

	callbackSpec := (&contract.CallbackSpec{
		InputSource: effect.ParamRef{Index: 1},
		Cardinality: contract.CardExactlyOnce,
	}).WithEnvOverlay(map[string]typ.Type{
		"up":   upFn,
		"down": downFn,
	})

	defineSpec := contract.NewSpec().
		WithCallback(1, callbackSpec)

	defineFn := typ.Func().
		Param("name", typ.String).
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Spec(defineSpec).
		Build()

	moduleType := typ.NewRecord().
		Field("define", defineFn).
		Build()

	m := io.NewManifest("migration")
	m.SetExport(moduleType)
	return m
}

func migrationManifestWithTransactionDB() *io.Manifest {
	txType := typ.NewInterface("migration.Transaction", []typ.Method{
		{
			Name: "query",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("sql", typ.String).
				OptParam("params", typ.Any).
				Returns(typ.Any, typ.NewOptional(typ.LuaError)).
				Build(),
		},
		{
			Name: "execute",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("sql", typ.String).
				OptParam("params", typ.Any).
				Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})
	stepFn := typ.Func().Param("db", txType).Returns(typ.Nil).Build()
	upFn := typ.Func().Param("fn", stepFn).Returns(typ.Nil).Build()
	overlay := map[string]typ.Type{}
	databaseCallbackSpec := (&contract.CallbackSpec{
		InputSource: effect.ParamRef{Index: 1},
		Cardinality: contract.CardExactlyOnce,
	}).WithEnvOverlay(overlay)
	databaseFn := typ.Func().
		Param("db_type", typ.String).
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Spec(contract.NewSpec().WithCallback(1, databaseCallbackSpec)).
		Build()
	migrationFn := typ.Func().
		Param("description", typ.String).
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Build()

	callbackSpec := (&contract.CallbackSpec{
		InputSource: effect.ParamRef{Index: 0},
		Cardinality: contract.CardExactlyOnce,
	}).WithEnvOverlay(overlay)
	overlay["migration"] = migrationFn
	overlay["database"] = databaseFn
	overlay["up"] = upFn
	overlay["down"] = upFn
	overlay["after"] = upFn

	defineFn := typ.Func().
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Spec(contract.NewSpec().WithCallback(0, callbackSpec)).
		Build()

	moduleType := typ.NewRecord().
		Field("define", defineFn).
		Build()

	m := io.NewManifest("migration_lib")
	m.SetExport(moduleType)
	m.DefineType("Transaction", txType)
	return m
}

func sqlManifestWithServiceDB() *io.Manifest {
	txType := typ.NewInterface("sql.Transaction", []typ.Method{
		{
			Name: "query",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("sql", typ.String).
				Variadic(typ.Any).
				Returns(typ.NewArray(typ.NewMap(typ.String, typ.Any)), typ.NewOptional(typ.LuaError)).
				Build(),
		},
		{
			Name: "execute",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("sql", typ.String).
				Variadic(typ.Any).
				Returns(typ.NewRecord().
					Field("rows_affected", typ.Integer).
					Field("last_insert_id", typ.Integer).
					Build(), typ.NewOptional(typ.LuaError)).
				Build(),
		},
		{
			Name: "commit",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
				Build(),
		},
		{
			Name: "rollback",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "type",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.String, typ.NewOptional(typ.LuaError)).
				Build(),
		},
		{
			Name: "begin",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(txType, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})

	spec := contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
	sqlTypes := typ.NewRecord().
		Field("POSTGRES", typ.LiteralString("postgres")).
		Field("SQLITE", typ.LiteralString("sqlite")).
		Build()
	moduleType := typ.NewRecord().
		Field("get", typ.Func().
			Param("dsn", typ.String).
			Returns(dbType, typ.NewOptional(typ.LuaError)).
			Spec(spec).
			Build()).
		Field("type", sqlTypes).
		Build()

	m := io.NewManifest("sql")
	m.SetExport(moduleType)
	m.DefineType("DB", dbType)
	m.DefineType("Transaction", txType)
	return m
}

func sqlManifestWithServiceDBAndBuilder() *io.Manifest {
	m := sqlManifestWithServiceDB()
	txType := m.Types["Transaction"]
	dbType := m.Types["DB"]
	runnerType := typ.NewUnion(dbType, txType)
	execResult := typ.NewRecord().
		Field("rows_affected", typ.Integer).
		Field("last_insert_id", typ.Integer).
		Build()
	queryExecutorType := typ.NewInterface("sql.QueryExecutor", []typ.Method{
		{
			Name: "exec",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(execResult, typ.NewOptional(typ.LuaError)).
				Build(),
		},
		{
			Name: "query",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.NewArray(typ.NewMap(typ.String, typ.Any)), typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})
	insertBuilderType := typ.NewInterface("sql.InsertBuilder", []typ.Method{
		{
			Name: "set_map",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("values", typ.NewMap(typ.String, typ.Any)).
				Returns(typ.Self).
				Build(),
		},
		{
			Name: "run_with",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("runner", runnerType).
				Returns(queryExecutorType).
				Build(),
		},
	})
	selectBuilderType := typ.NewInterface("sql.SelectBuilder", []typ.Method{
		{
			Name: "from",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("table", typ.String).
				Returns(typ.Self).
				Build(),
		},
		{
			Name: "where",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("clause", typ.String).
				Variadic(typ.Any).
				Returns(typ.Self).
				Build(),
		},
		{
			Name: "run_with",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("runner", runnerType).
				Returns(queryExecutorType).
				Build(),
		},
	})
	builderType := typ.NewRecord().
		Field("insert", typ.Func().Param("table", typ.String).Returns(insertBuilderType).Build()).
		Field("select", typ.Func().Variadic(typ.String).Returns(selectBuilderType).Build()).
		Build()
	m.SetExport(typ.NewRecord().
		Field("get", typ.Func().Param("dsn", typ.String).Returns(dbType, typ.NewOptional(typ.LuaError)).Build()).
		Field("type", typ.NewRecord().
			Field("POSTGRES", typ.LiteralString("postgres")).
			Field("SQLITE", typ.LiteralString("sqlite")).
			Build()).
		Field("builder", builderType).
		Build())
	return m
}

// testFrameworkManifest creates a manifest for a test DSL module.
// The "it" function accepts a callback with "expect" injected via EnvOverlay.
func testFrameworkManifest() *io.Manifest {
	expectFn := typ.Func().
		Param("value", typ.Any).
		Returns(typ.Boolean).
		Build()

	callbackSpec := (&contract.CallbackSpec{
		InputSource: effect.ParamRef{Index: 1},
		Cardinality: contract.CardExactlyOnce,
	}).WithEnvOverlay(map[string]typ.Type{
		"expect": expectFn,
	})

	itSpec := contract.NewSpec().
		WithCallback(1, callbackSpec)

	itFn := typ.Func().
		Param("name", typ.String).
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Spec(itSpec).
		Build()

	m := io.NewManifest("testing")
	m.AddGlobal("it", itFn)
	return m
}

func TestEnvOverlay_MigrationDSL(t *testing.T) {
	source := `
		migration.define("Create users table", function()
			up(function()
				local x = 1
			end)
			down(function()
				local y = 2
			end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration", migrationManifest()))
	if result.HasError() {
		t.Errorf("expected no errors inside migration callback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_MigrationDSLTypedTransactionRejectsServiceTypeMethod(t *testing.T) {
	source := `
		migration_lib.define(function()
			database("postgres", function()
				up(function(db)
					local db_type, err = db:type()
				end)
			end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration_lib", migrationManifestWithTransactionDB()))
	if !result.HasError() {
		t.Fatal("expected migration transaction db to reject service-only type() method")
	}
	messages := strings.Join(testutil.ErrorMessages(result.Diagnostics), "\n")
	if !strings.Contains(messages, "type") {
		t.Fatalf("expected diagnostic to mention missing type method, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_MigrationDSLTypedTransactionAllowsQueryAndExecute(t *testing.T) {
	source := `
		migration_lib.define(function()
			database("postgres", function()
				up(function(db)
					local rows, qerr = db:query("SELECT 1")
					if qerr then return end
					local ok, xerr = db:execute("CREATE TABLE users(id TEXT)")
					if xerr then return end
				end)
			end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration_lib", migrationManifestWithTransactionDB()))
	if result.HasError() {
		t.Fatalf("expected migration transaction query/execute methods to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_MigrationCallbackParameterFeedsOuterHelper(t *testing.T) {
	source := `
		local function create_admin_user(db)
			local result, err = db:execute("INSERT INTO users (id) VALUES (?)", {"admin"})
			if err then error(err) end
			return result
		end

		migration_lib.define(function()
			migration("seed admin", function()
				database("postgres", function()
					up(function(db)
						create_admin_user(db)
					end)
				end)
			end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration_lib", migrationManifestWithTransactionDB()))
	if result.HasError() {
		t.Fatalf("expected migration callback parameter to feed outer helper parameter evidence, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_MigrationCallbackParameterFeedsHelperArgumentDemand(t *testing.T) {
	source := `
		local function run_with(db: migration_lib.Transaction)
			return db
		end

		local function create_admin_user(db)
			local executor = run_with(db)
			return executor
		end

		migration_lib.define(function()
			migration("seed admin", function()
				database("postgres", function()
					up(function(db)
						create_admin_user(db)
					end)
				end)
			end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration_lib", migrationManifestWithTransactionDB()))
	if result.HasError() {
		t.Fatalf("expected helper parameter to satisfy nested typed argument demand, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_MigrationCallbackParameterFeedsHelperMethodArgumentDemand(t *testing.T) {
	source := `
		type Query = {
			run_with: (any, migration_lib.Transaction) -> any,
		}

		local query: Query = {
			run_with = function(self, db)
				return db
			end
		}

		local function create_admin_user(db)
			local executor = query:run_with(db)
			return executor
		end

		migration_lib.define(function()
			migration("seed admin", function()
				database("postgres", function()
					up(function(db)
						create_admin_user(db)
					end)
				end)
			end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration_lib", migrationManifestWithTransactionDB()))
	if result.HasError() {
		t.Fatalf("expected helper parameter to satisfy nested typed method argument demand, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_MigrationCallbackParameterSatisfiesSQLBuilderRunnerUnion(t *testing.T) {
	source := `
		local sql = require("sql")

		local function admin_exists(db)
			local query = sql.builder.select("COUNT(*) as count")
				:from("app_user_groups")
				:where("group_id = ?", "app.security:admin")
			local executor = query:run_with(db)
			local results, err = executor:query()
			if err then error(err) end
			local first = results[1]
			if not first then return false end
			return first.count > 0
		end

		local function create_admin_user(db)
			if admin_exists(db) then
				return nil
			end
			local user_query = sql.builder.insert("app_users")
				:set_map({
					user_id = "admin",
					email = "admin@example.test",
					status = "active",
				})
			local user_executor = user_query:run_with(db)
			local result, err = user_executor:exec()
			if err then error(err) end
			return result
		end

		migration_lib.define(function()
			migration("seed admin", function()
				database("postgres", function()
					up(function(db)
						create_admin_user(db)
					end)
				end)
				database("sqlite", function()
					up(function(db)
						create_admin_user(db)
					end)
				end)
			end)
		end)
	`
	result := testutil.Check(
		source,
		testutil.WithStdlib(),
		testutil.WithManifest("migration_lib", migrationManifestWithTransactionDB()),
		testutil.WithManifest("sql", sqlManifestWithServiceDBAndBuilder()),
	)
	if result.HasError() {
		t.Fatalf("expected migration callback parameter to satisfy SQL builder runner union, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_HelperParameterSatisfiesOwnMethodArgumentDemand(t *testing.T) {
	source := `
		type Query = {
			run_with: (any, migration_lib.Transaction) -> any,
		}

		local query: Query = {
			run_with = function(self, db)
				return db
			end
		}

		local function create_admin_user(db)
			local executor = query:run_with(db)
			return executor
		end
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration_lib", migrationManifestWithTransactionDB()))
	if result.HasError() {
		t.Fatalf("expected helper parameter to satisfy its own typed method argument demand, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_RuntimeSQLServiceDBAllowsTypeMethod(t *testing.T) {
	source := `
		local sql = require("sql")
		local db, err = sql.get("app:db")
		if err then return end

		local db_type, type_err = db:type()
		if type_err then return end
		if db_type == sql.type.POSTGRES then
			return
		end
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifestWithServiceDB()))
	if result.HasError() {
		t.Fatalf("expected runtime sql service db:type() to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_CallbackParameterUsesImportedModuleType(t *testing.T) {
	source := `
		local sql = require("sql")

		local function up(fn: (sql.Transaction) -> any)
		end

		up(function(db)
			local rows, qerr = db:query("SELECT 1")
			if qerr then return end
			local result, xerr = db:execute("CREATE TABLE users(id TEXT)")
			if xerr then return end
			local changed: integer = result.rows_affected
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifestWithServiceDB()))
	if result.HasError() {
		t.Fatalf("expected callback db to infer sql.Transaction from imported module type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_CallbackParameterImportedTransactionRejectsServiceDBMethod(t *testing.T) {
	source := `
		local sql = require("sql")

		local function up(fn: (sql.Transaction) -> any)
		end

		up(function(db)
			local db_type, err = db:type()
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifestWithServiceDB()))
	if !result.HasError() {
		t.Fatal("expected callback db inferred as sql.Transaction to reject sql.DB-only type() method")
	}
	messages := strings.Join(testutil.ErrorMessages(result.Diagnostics), "\n")
	if !strings.Contains(messages, "type") {
		t.Fatalf("expected diagnostic to mention missing type method, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_CallbackParameterUsesImportedModuleTypeAlias(t *testing.T) {
	source := `
		local sql = require("sql")

		type MigrationStep = (sql.Transaction) -> any

		local function up(fn: MigrationStep)
		end

		up(function(db)
			local result, xerr = db:execute("CREATE TABLE users(id TEXT)")
			if xerr then return end
			local changed: integer = result.rows_affected
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifestWithServiceDB()))
	if result.HasError() {
		t.Fatalf("expected callback type alias to preserve sql.Transaction parameter, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_FactoryReturnCallbackUsesImportedModuleType(t *testing.T) {
	source := `
		local sql = require("sql")

		type MigrationStep = (sql.Transaction) -> any

		local function create_up_fn(): (MigrationStep) -> ()
			return function(fn: MigrationStep)
			end
		end

		local up = create_up_fn()
		up(function(db)
			local result, xerr = db:execute("CREATE TABLE users(id TEXT)")
			if xerr then return end
			local changed: integer = result.rows_affected
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifestWithServiceDB()))
	if result.HasError() {
		t.Fatalf("expected factory-returned callback API to preserve sql.Transaction parameter, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_FactoryReturnCallbackUsesPlainCallbackType(t *testing.T) {
	source := `
		type Step = (number) -> any

		local function create_step_fn(): (Step) -> ()
			return function(fn: Step)
			end
		end

		local step = create_step_fn()
		step(function(value)
			local n: number = value
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected factory-returned callback API to preserve plain callback parameter, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_FactoryReturnCallbackWithExplicitLocalType(t *testing.T) {
	source := `
		local sql = require("sql")

		type MigrationStep = (sql.Transaction) -> any
		type UpFn = (MigrationStep) -> ()

		local function create_up_fn(): UpFn
			return function(fn: MigrationStep)
			end
		end

		local up: UpFn = create_up_fn()
		up(function(db)
			local result, xerr = db:execute("CREATE TABLE users(id TEXT)")
			if xerr then return end
			local changed: integer = result.rows_affected
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifestWithServiceDB()))
	if result.HasError() {
		t.Fatalf("expected explicitly typed factory local to preserve sql.Transaction parameter, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_FactoryReturnCallbackImmediateCallUsesImportedModuleType(t *testing.T) {
	source := `
		local sql = require("sql")

		type MigrationStep = (sql.Transaction) -> any

		local function create_up_fn(): (MigrationStep) -> ()
			return function(fn: MigrationStep)
			end
		end

		create_up_fn()(function(db)
			local result, xerr = db:execute("CREATE TABLE users(id TEXT)")
			if xerr then return end
			local changed: integer = result.rows_affected
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifestWithServiceDB()))
	if result.HasError() {
		t.Fatalf("expected immediate factory-returned callback API to preserve sql.Transaction parameter, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_InferredGlobalCallbackUsesImportedModuleType(t *testing.T) {
	source := `
		local sql = require("sql")

		type MigrationStep = (sql.Transaction) -> any

		local function create_up_fn(): (MigrationStep) -> ()
			return function(fn: MigrationStep)
			end
		end

		local function define(fn: () -> any)
			_G.up = create_up_fn()
			fn()
			_G.up = nil
		end

		define(function()
			up(function(db)
				local rows, qerr = db:query("SELECT 1")
				if qerr then return end
				local result, xerr = db:execute("CREATE TABLE users(id TEXT)")
				if xerr then return end
				local changed: integer = result.rows_affected
			end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifestWithServiceDB()))
	if result.HasError() {
		t.Fatalf("expected inferred global callback to preserve sql.Transaction parameter, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_InferredGlobalCallbackImportedTransactionRejectsServiceDBMethod(t *testing.T) {
	source := `
		local sql = require("sql")

		type MigrationStep = (sql.Transaction) -> any

		local function create_up_fn(): (MigrationStep) -> ()
			return function(fn: MigrationStep)
			end
		end

		local function define(fn: () -> any)
			_G.up = create_up_fn()
			fn()
			_G.up = nil
		end

		define(function()
			up(function(db)
				local db_type, err = db:type()
			end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifestWithServiceDB()))
	if !result.HasError() {
		t.Fatal("expected inferred global callback db to reject sql.DB-only type() method")
	}
	messages := strings.Join(testutil.ErrorMessages(result.Diagnostics), "\n")
	if !strings.Contains(messages, "type") {
		t.Fatalf("expected diagnostic to mention missing type method, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_TestDSL(t *testing.T) {
	manifest := testFrameworkManifest()

	source := `
		it("should work", function()
			expect(42)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("testing", manifest))
	if result.HasError() {
		t.Errorf("expected no errors inside test callback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_InferredBasic(t *testing.T) {
	source := `
		local function define(name: string, fn: fun())
			_G.up = function(cb: fun()) cb() end
			fn()
			_G.up = nil
		end

		define("test", function()
			up(function() end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with inferred overlay, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_InferredMultipleGlobals(t *testing.T) {
	source := `
		local function define(name: string, fn: fun())
			_G.up = function(cb: fun()) cb() end
			_G.down = function(cb: fun()) cb() end
			fn()
			_G.up = nil
			_G.down = nil
		end

		define("test", function()
			up(function() end)
			down(function() end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with multiple inferred overlays, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_InferredMissingCleanup(t *testing.T) {
	// No _G.up = nil cleanup, so overlay should NOT be inferred.
	// Calling up() should produce an error since it is not typed.
	source := `
		local function define(name: string, fn: fun())
			_G.up = function(cb: fun()) cb() end
			fn()
		end

		define("test", function()
			local x: fun(cb: fun()) = up
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Error("expected error when cleanup is missing (no overlay inferred)")
	}
}

func TestEnvOverlay_InferredScopeIsolation(t *testing.T) {
	// Inferred globals should not be visible outside the callback.
	source := `
		local function define(name: string, fn: fun())
			_G.up = function(cb: fun()) cb() end
			fn()
			_G.up = nil
		end

		define("test", function()
			up(function() end)
		end)

		local x: fun(cb: fun()) = up
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Error("inferred overlay globals should not be typed outside callback")
	}
}

func TestEnvOverlay_InferredNonParamCall(t *testing.T) {
	// Call to a non-parameter function should not trigger overlay.
	source := `
		local function helper() end

		local function define(name: string, fn: fun())
			_G.up = function(cb: fun()) cb() end
			helper()
			_G.up = nil
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("non-param call should not cause errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_ScopeIsolation(t *testing.T) {
	// "up" is available inside the callback via EnvOverlay
	t.Run("inside callback", func(t *testing.T) {
		source := `
			migration.define("Create table", function()
				up(function() end)
			end)
		`
		result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration", migrationManifest()))
		if result.HasError() {
			t.Errorf("up should be visible inside callback, got: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	// "up" is NOT available outside the callback, so assigning it to a
	// typed variable should produce an error (it resolves as unknown).
	t.Run("outside callback", func(t *testing.T) {
		source := `
			migration.define("Create table", function()
				up(function() end)
			end)

			local x: fun(fn: fun(): nil): nil = up
		`
		result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration", migrationManifest()))
		if !result.HasError() {
			t.Error("up should NOT be typed outside callback, expected type error")
		}
	})
}
