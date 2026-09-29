-- Shared type surfaces for the generic automation engine. Dynamic provider
-- payloads stay `any`, but stable envelopes returned by automations_lib are
-- exported here so API wrappers and provider installers can share one contract.

local registry = require("registry")

local M = {}

M.DB_CONFIG = "kickside.automation:db_config"
M._registry = registry

type Map = { [string]: any }

type ModuleRef = any
type ContractInstance = { [string]: any }
type ErrorValue = any

type RollbackStep = {
    target: string,
    args: Map?,
}

type RollbackChain = { RollbackStep }

type RollbackRunner = (string, Map) -> ErrorValue?

type InstallRecorder = {
    rollback: (string, Map?) -> (),
}

type CreatedResource = Map

-- A declarative schedule the install body asks the engine to create after the
-- component is registered. The engine calls automation_schedule.create_action_schedule
-- for each, threads the rollback recorder, and writes the resulting schedule_id
-- back into state[state_key].
type ScheduleSpec = {
    name: string?,
    method: string,
    schedule_type: string,
    schedule_expression: string,
    state_key: string?,
    description: string?,
    args: Map?,
    timeout_seconds: integer?,
    max_retries: integer?,
    enabled: boolean?,
    resolve_live: boolean?,
    app_scope: string?,
}

type InstallBodyResult = {
    state: Map,
    metadata: Map?,
    public_state: Map?,
    created: { CreatedResource }?,
    schedules: { ScheduleSpec }?,
}

type InstallBody = (InstallRecorder) -> (InstallBodyResult?, any?)

type InstallOptions = {
    runner: RollbackRunner?,
}

type InstallResult = {
    state: Map,
    metadata: Map,
    public_state: Map?,
    created: { CreatedResource }?,
    schedules: { ScheduleSpec }?,
    rollback: { RollbackStep },
}

-- One schedule the engine created for an installed automation: the cron task_id
-- plus the body-supplied identity, surfaced so callers can act on the new
-- schedule without re-reading private state.
type CreatedSchedule = Map

type ReplayOptions = {
    runner: RollbackRunner?,
    options: Map?,
}

type LifecycleMethods = { [string]: boolean }

type BindingMethods = { [string]: string }

type BindingContract = {
    contract: string?,
    methods: BindingMethods?,
}

type RegistryEntry = {
    id: string,
    kind: string?,
    meta: Map?,
    data: {
        contracts: { BindingContract }?,
    }?,
}

type AutomationAction = {
    name: string,
    contract: string,
    target: string,
}

type ViewComponent = {
    id: string,
    name: string,
    title: string,
    tag_name: string,
    base_path: string,
    entry_point: string,
    url: string,
}

type OwnedResourceSpec = {
    kind: string,
    state_key: string,
}

type ClassValue = string | { string }

type AutomationType = {
    id: string,
    name: string,
    title: string,
    description: string,
    icon: string,
    category: string,
    class: ClassValue?,
    primary: boolean,
    reconfigurable: boolean?,
    inputs: Map?,
    component: Map?,
    source: Map?,
    public_state_schema: Map?,
    delete_options: Map?,
    actions: { AutomationAction },
}

type ListTypesOptions = {
    class: string?,
    category: string?,
}

type ComponentRow = {
    component_id: string,
    impl_id: string,
    parent_id: string?,
    created_at: any?,
    updated_at: any?,
    meta: Map?,
    private_context: Map?,
    access_level: integer?,
}

type RegistryModule = {
    find: (Map) -> ({ RegistryEntry }?, ErrorValue?),
    get: (string) -> (RegistryEntry?, ErrorValue?),
}

type FuncExecutor = {
    call: (FuncExecutor, string, Map) -> (any, ErrorValue?),
}

type FuncsModule = {
    new: () -> (FuncExecutor?, ErrorValue?),
}

type AccessMasks = {
    NONE: integer,
    READ: integer,
    WRITE: integer,
    DELETE: integer,
    ADMIN: integer,
    FULL: integer,
}

function M.db_id(): string
    local entry = select(1, (M._registry or registry).get(M.DB_CONFIG)) :: any
    return entry.meta.db_id :: string
end

type ComponentRegistrationResult = {
    component_id: string?,
}

type ComponentAccessContext = {
    impl_id: string?,
}

type ComponentDeleteResult = {
    success: boolean?,
    error: ErrorValue?,
}

type ComponentUpdateResult = {
    success: boolean?,
    error: ErrorValue?,
}

type ComponentService = {
    register: (ComponentService, Map) -> (ComponentRegistrationResult?, ErrorValue?),
    get_access_context: (ComponentService, Map) -> (ComponentAccessContext?, ErrorValue?),
    update: (ComponentService, Map) -> (ComponentUpdateResult?, ErrorValue?),
    delete: (ComponentService, Map) -> (ComponentDeleteResult?, ErrorValue?),
}

type ComponentModule = {
    query: (Map) -> ({ ComponentRow }?, ErrorValue?),
    list_system: (Map) -> { ComponentRow }?,
    get_context: (string, integer) -> (AutomationPrivateContext?, ErrorValue?),
    get_private_context: (string) -> (AutomationPrivateContext?, ErrorValue?),
    validate_access: (string, integer) -> (any?, ErrorValue?),
    get_service: () -> (ComponentService?, ErrorValue?),
    open: (string, integer, string) -> (ContractInstance?, ErrorValue?),
    notify_access: ((string, string?, Map?, Map?) -> (Map?, ErrorValue?))?,
    set_meta: ((string, Map, Map?) -> (boolean, ErrorValue?))?,
    ACCESS: AccessMasks,
}

type LoggerInstance = {
    warn: (LoggerInstance, string, Map?) -> (),
}

type InstalledAutomation = {
    id: string,
    type: string,
    title: string,
    icon: string,
    description: string,
    class: ClassValue?,
    created_at: any,
    updated_at: any,
    access_level: integer,
    parent_id: string?,
    public_state: Map?,
    flow_ref: Map?,
}

type ListInstalledOptions = {
    actor_id: string?,
    class: string?,
    parent_id: string?,
    limit: number?,
    offset: number?,
}

type InstallTypeResult = {
    id: string,
    type: string,
    metadata: Map,
    created: { CreatedResource }?,
    schedule_ids: { string }?,
    schedules: { CreatedSchedule }?,
}

-- Persisted execution-identity carrier. The execution primitive's runtime
-- FrozenIdentity is serialized through execution_identity.to_row before it is
-- stored in component private_context.
type ExecutionIdentity = {
    actor_id: string,
    actor_context: any,
}

type AutomationPrivateContext = Map & {
    _rollback: { RollbackStep }?,
    _execution_identity: ExecutionIdentity?,
}

type RuntimeAutomation = {
    id: string,
    type: string,
    metadata: Map,
    execution_identity: ExecutionIdentity?,
    state: Map?,
}

type UninstallResult = {
    id: string,
    replayed: integer,
    replay_failed: integer,
}

type CallActionResult = {
    id: string,
    method: string,
    result: any,
}

type PatchStateOptions = {
    delete_keys: { string }?,
    public_meta: Map?,
}

M.AUTOMATION_TYPE = "kickside.automation"
M.LIFECYCLE_METHODS = {
    install = true,
    delete = true,
} :: LifecycleMethods

M.Map = Map
M.ModuleRef = ModuleRef
M.ContractInstance = ContractInstance
M.ErrorValue = ErrorValue
M.RollbackStep = RollbackStep
M.RollbackChain = RollbackChain
M.RollbackRunner = RollbackRunner
M.InstallRecorder = InstallRecorder
M.CreatedResource = CreatedResource
M.ScheduleSpec = ScheduleSpec
M.CreatedSchedule = CreatedSchedule
M.InstallBodyResult = InstallBodyResult
M.InstallBody = InstallBody
M.InstallOptions = InstallOptions
M.InstallResult = InstallResult
M.ReplayOptions = ReplayOptions
M.LifecycleMethods = LifecycleMethods
M.BindingMethods = BindingMethods
M.BindingContract = BindingContract
M.RegistryEntry = RegistryEntry
M.AutomationAction = AutomationAction
M.ViewComponent = ViewComponent
M.OwnedResourceSpec = OwnedResourceSpec
M.ClassValue = ClassValue
M.AutomationType = AutomationType
M.ListTypesOptions = ListTypesOptions
M.ComponentRow = ComponentRow
M.RegistryModule = RegistryModule
M.FuncExecutor = FuncExecutor
M.FuncsModule = FuncsModule
M.AccessMasks = AccessMasks
M.ComponentRegistrationResult = ComponentRegistrationResult
M.ComponentAccessContext = ComponentAccessContext
M.ComponentDeleteResult = ComponentDeleteResult
M.ComponentUpdateResult = ComponentUpdateResult
M.ComponentService = ComponentService
M.ComponentModule = ComponentModule
M.LoggerInstance = LoggerInstance
M.InstalledAutomation = InstalledAutomation
M.ListInstalledOptions = ListInstalledOptions
M.InstallTypeResult = InstallTypeResult
M.AutomationPrivateContext = AutomationPrivateContext
M.ExecutionIdentity = ExecutionIdentity
M.RuntimeAutomation = RuntimeAutomation
M.UninstallResult = UninstallResult
M.CallActionResult = CallActionResult
M.PatchStateOptions = PatchStateOptions

return M

