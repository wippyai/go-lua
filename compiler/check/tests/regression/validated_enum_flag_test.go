package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestLiteralArrayMembershipFlagNarrowsAfterRejectingInvalidValue(t *testing.T) {
	source := `
		local function select_field(value: string): "created_at" | "updated_at" | "next_run_at"?
			local allowed = { "created_at", "updated_at", "next_run_at" }
			local valid = false
			for _, field in ipairs(allowed) do
				if value == field then
					valid = true
					break
				end
			end
			if not valid then
                print("invalid")
                return nil
            end
			local selected: "created_at" | "updated_at" | "next_run_at" = value
			return selected
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("membership proof was lost: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestLiteralArrayMembershipProofInvalidatesAfterValueWrite(t *testing.T) {
	source := `
		local function select_field(value: string)
			local allowed = { "created_at", "updated_at" }
			local valid = false
			for _, field in ipairs(allowed) do
				if value == field then valid = true; break end
			end
			if not valid then return end
			value = "invalid"
			local selected: "created_at" | "updated_at" = value
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("membership proof survived a later write")
	}
}

func TestLiteralArrayMembershipProofRequiresBuiltinIterator(t *testing.T) {
	source := `
		local function select_field(value: string)
			local ipairs = function() return function() return 1, "invalid" end end
			local allowed = { "created_at", "updated_at" }
			local valid = false
			for _, field in ipairs(allowed) do
				if value == field then valid = true; break end
			end
			if not valid then return end
			local selected: "created_at" | "updated_at" = value
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("shadowed iterator established unsound membership")
	}
}

func TestLiteralArrayMembershipSurvivesDTOConstruction(t *testing.T) {
	allowed := typ.NewUnion(typ.LiteralString("created_at"), typ.LiteralString("updated_at"), typ.LiteralString("next_run_at"))
	ordering := typ.NewRecord().SetDeclared(true).SetOpen(true).OptField("field", typ.NewIntersection(typ.String, allowed)).OptField("direction", typ.NewIntersection(typ.String, typ.NewUnion(typ.LiteralString("ASC"), typ.LiteralString("DESC")))).Build()
	request := typ.NewRecord().SetDeclared(true).SetOpen(true).OptField("ordering", ordering).OptField("filters", typ.NewRecord().SetOpen(true).Build()).OptField("pagination", typ.NewRecord().SetOpen(true).Build()).Build()
	manifest := io.NewManifest("service")
	manifest.SetExport(typ.NewRecursive("Service", func(self typ.Type) typ.Type {
		return typ.NewRecord().SetDeclared(true).MapComponent(typ.String, typ.Any).
			Field("list", typ.Func().Param("self", self).Param("request", request).Build()).Build()
	}))
	reqType := typ.NewRecord().Field("query", typ.Func().Param("self", typ.Self).Param("key", typ.String).Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.LuaError)).Build()).Build()
	resType := typ.NewRecord().
		Field("set_status", typ.Func().Param("self", typ.Self).Param("status", typ.Number).Build()).
		Field("set_content_type", typ.Func().Param("self", typ.Self).Param("content_type", typ.String).Build()).
		Field("write_json", typ.Func().Param("self", typ.Self).Param("value", typ.Any).Build()).Build()
	httpManifest := io.NewManifest("http")
	httpManifest.SetExport(typ.NewRecord().
		Field("request", typ.Func().Returns(typ.NewOptional(reqType)).Build()).
		Field("response", typ.Func().Returns(typ.NewOptional(resType)).Build()).Build())
	source := `
		local service = require("service")
		local http = require("http")
		local function select_field()
			local req = http.request()
			local res = http.response()
			if not req or not res then return end
			local status = req:query("status")
            local class = req:query("class")
            local schedule_type = req:query("schedule_type")
            local task_implementation_id = req:query("task_implementation_id")
            local enabled = req:query("enabled")
            local enabled_bool = nil
            if enabled then
                if enabled == "true" or enabled == "1" then enabled_bool = true
                elseif enabled == "false" or enabled == "0" then enabled_bool = false
                else return end
            end
            local order_by = req:query("order_by") or "created_at"
			local valid_order_fields = { "created_at", "updated_at", "next_run_at" }
			local valid_order_field = false
			for _, field in ipairs(valid_order_fields) do
				if order_by == field then valid_order_field = true; break end
			end
			if not valid_order_field then
				res:set_status(400)
				res:set_content_type("json")
				res:write_json({success=false, error="invalid"})
				return
			end
			local order_direction = req:query("order_direction") or "DESC"
			if order_direction ~= "ASC" and order_direction ~= "DESC" then print("bad direction"); return end
			local request_dto = {ordering = {field = order_by, direction = order_direction}, filters = {}, pagination = {limit=10}}
			if status and status ~= "" then request_dto.filters.status = status end
            if enabled_bool ~= nil then request_dto.filters.enabled = enabled_bool end
			if schedule_type and schedule_type ~= "" then request_dto.filters.schedule_type = schedule_type end
			if task_implementation_id and task_implementation_id ~= "" then request_dto.filters.task_implementation_id = task_implementation_id end
			if class and class ~= "" then request_dto.filters.class = class end
			service:list(request_dto)
		end
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("service", manifest), testutil.WithManifest("http", httpManifest))
	if result.HasError() {
		t.Fatalf("validated field was widened in DTO: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
