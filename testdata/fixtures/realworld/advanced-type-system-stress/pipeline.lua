type Plugin = {id: string, enabled: boolean, config: {[string]: any}}
type Pipeline = {name: string, plugins: {Plugin}, env: {[string]: string}}

local M = {}
M.Plugin = Plugin
M.Pipeline = Pipeline

function M.new(name: string): Pipeline
    return {
        name = name,
        plugins = {},
        env = {},
    }
end

function M.add_plugin(pipeline: Pipeline, plugin: Plugin): Pipeline
    table.insert(pipeline.plugins, plugin)
    return pipeline
end

function M.enable_defaults(pipeline: Pipeline, defaults: {[string]: string}?): Pipeline
    for key, value in pairs(defaults or {}) do
        pipeline.env[key] = value
    end
    return M.add_plugin(pipeline, {
        id = "logger",
        enabled = true,
        config = {level = pipeline.env.LOG_LEVEL or "info"},
    })
end

return M
