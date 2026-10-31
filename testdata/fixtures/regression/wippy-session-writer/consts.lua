local env = require("env")
local time = require("time")

type SessionConfig = {
    database_resource: string?,
    token_checkpoint_threshold: number?,
    max_message_limit: number?,
    checkpoint_function_id: string?,
    title_function_id: string?,
    default_host: string?,
    session_security_scope: string?,
    gc_interval: string?,
    delegation_func_id: string?,
    on_session_end_func_id: string?,
    encryption_key: string?,
    enable_agent_cache: boolean?,
    delegation_description_suffix: string?,
}

local consts = {
    -- Environment variable IDs
    ENV_IDS = {
        DATABASE_RESOURCE = "wippy.session.env:database_resource",
        TOKEN_CHECKPOINT_THRESHOLD = "wippy.session.env:token_checkpoint_threshold",
        MAX_MESSAGE_LIMIT = "wippy.session.env:max_message_limit",
        CHECKPOINT_FUNCTION_ID = "wippy.session.env:checkpoint_function_id",
        TITLE_FUNCTION_ID = "wippy.session.env:title_function_id",
        DEFAULT_HOST = "wippy.session.env:default_host",
        SESSION_SECURITY_SCOPE = "wippy.session.env:session_security_scope",
        GC_INTERVAL = "wippy.session.env:gc_interval",
        DELEGATION_FUNC_ID = "wippy.session.env:delegation_func_id",
        ON_SESSION_END_FUNC_ID = "wippy.session.env:on_session_end_func_id",
        ENCRYPTION_KEY = "ENCRYPTION_KEY"
    },

    -- Context Keys
    CONTEXT_KEYS = {
        CURRENT_CHECKPOINT_ID = "current_checkpoint_id",
        FULL_CONTEXT = "full_context"
    },

    -- Process Constants
    PROCESS = {
        SESSION_ID = "wippy.session.process:session",
    },

    -- Plugin Topics
    PLUGIN_TOPICS = {
        OPEN = "open",
        CLOSE = "close",
        MESSAGE = "message",
        COMMAND = "command",
        SHUTDOWN = "shutdown",
        RESUME = "resume"
    },

    -- Session Status Constants
    STATUS = {
        IDLE = "idle",
        RUNNING = "running",
        FAILED = "failed",
        ERROR = "error",
        COMPLETED = "completed"
    },

    -- Operation Types
    OP_TYPE = {
        -- Message flow
        HANDLE_MESSAGE = "handle_message",
        AGENT_STEP = "agent_step",
        PROCESS_TOOLS = "process_tools",
        AGENT_CONTINUE = "agent_continue",

        -- Control directives (split)
        CONTROL_ARTIFACTS = "control_artifacts",
        CONTROL_CONTEXT = "control_context",
        CONTROL_MEMORY = "control_memory",
        CONTROL_CONFIG = "control_config",

        -- Background operations
        GENERATE_TITLE = "generate_title",
        CREATE_CHECKPOINT = "create_checkpoint",

        -- Background trigger operation
        CHECK_BACKGROUND_TRIGGERS = "check_background_triggers",

        -- Session management
        AGENT_CHANGE = "agent_change",
        MODEL_CHANGE = "model_change",
        EXECUTE_FUNCTION = "execute_function",

        -- Control flow
        INTERCEPT_EXECUTION = "intercept_execution",

        HANDLE_CONTEXT = "handle_context",
    },

    -- Session Commands
    COMMANDS = {
        STOP = "stop",
        AGENT = "agent",
        MODEL = "model",
        ARTIFACT = "artifact",
        CONTEXT = "context"
    },

    -- Message Types
    MSG_TYPE = {
        USER = "user",
        ASSISTANT = "assistant",
        SYSTEM = "system",
        DEVELOPER = "developer",
        FUNCTION = "function",
        PRIVATE_FUNCTION = "private_function",
        ARTIFACT = "artifact",
        DELEGATION = "delegation",
    },

    -- Function Call Status
    FUNC_STATUS = {
        PENDING = "pending",
        SUCCESS = "success",
        ERROR = "error"
    },

    -- Session Topics for Actor Communication
    TOPICS = {
        STOP = "stop",
        MESSAGE = "message",
        COMMAND = "command",
        CONTINUE = "continue",
        CONTEXT = "context",
        ERROR = "error",
        SESSION_OPENED = "session.opened",
        SESSION_CLOSED = "session.closed",
        FINISH_AND_EXIT = "finish_and_exit"
    },

    -- Upstream Update Types
    UPSTREAM_TYPES = {
        UPDATE = "update",
        ERROR = "error",
        RECEIVED = "received",
        RESPONSE_STARTED = "response_started",
        INVALIDATE = "invalidate",
        COMMAND_RESPONSE = "command_response",
        CONTENT = "content",
        FUNCTION_CALL = "function_call",
        FUNCTION_SUCCESS = "function_success",
        FUNCTION_ERROR = "function_error"
    },

    -- Topic Prefixes
    TOPIC_PREFIXES = {
        SESSION = "session:",
        MESSAGE = ":message:"
    },

    -- Handler Types
    HANDLER_TYPES = {
        MESSAGE = "message",
        COMMAND = "command"
    },

    -- Session Operation Types
    SESSION_OPS = {
        CREATE = "created",
        RECONNECT = "reconnected"
    },

    -- Context Types
    CONTEXT_TYPES = {
        SESSION = "session",
        CONVERSATION_SUMMARY = "conversation_summary"
    },

    -- Session Kinds
    SESSION_KINDS = {
        DEFAULT = "default"
    },

    -- Artifact Types
    ARTIFACT_TYPES = {
        INLINE = "inline",
        VIEW_REF = "view_ref"
    },

    -- Artifact Display Types
    ARTIFACT_DISPLAY = {
        INLINE = "inline",
        STANDALONE = "standalone"
    },

    -- Artifact Status
    ARTIFACT_STATUS = {
        IDLE = "idle",
        ACTIVE = "active",
        ERROR = "error"
    },

    ARTIFACT_INSTRUCTIONS = {
        REFERENCE_TEMPLATE =
        'To display the "%s" artifact, insert this exact tag where you want it to appear: <artifact id="%s"/>. Do not wrap this tag in code blocks, quotes, or backticks.',
        WITH_PREVIEW_TEMPLATE =
        'To display the "%s" artifact, insert this exact tag where you want it to appear: <artifact id="%s"/>. Do not wrap this tag in code blocks, quotes, or backticks. %s',
        VIEW_REF_TEMPLATE = 'To display the interactive "%s" component, insert: <artifact id="%s"/>. This component: %s'
    },

    -- Content Types
    CONTENT_TYPES = {
        HTML = "text/html",
        MARKDOWN = "text/markdown",
        TEXT = "text/plain",
        JSON = "application/json"
    },

    -- System Actions
    SYSTEM_ACTIONS = {
        ARTIFACT_CREATED = "artifact_created",
        ARTIFACT_UPDATED = "artifact_updated",
        AGENT_CHANGED = "agent_changed",
        MODEL_CHANGED = "model_changed",
        SESSION_INIT = "session_init",
        TITLE_GENERATED = "title_generated",
        CHECKPOINT_CREATED = "checkpoint_created"
    },

    -- Error Codes for Plugin
    ERROR_CODES = {
        INVALID_JSON = "invalid_json",
        SESSION_LIMIT = "session_limit_reached",
        SESSION_ID_GEN = "session_id_gen_error",
        SESSION_SPAWN = "session_spawn_error",
        INVALID_SESSION_ID = "invalid_session_id",
        SESSION_NOT_FOUND = "session_not_found",
        INVALID_MESSAGE_TYPE = "invalid_message_type",
        TOKEN_INVALID = "token_invalid",
        AGENT_ERROR = "agent_error",
        STORAGE_ERROR = "storage_error"
    },

    -- Limits
    LIMITS = {
        MAX_SESSIONS_PER_USER = 300
    },

    -- Timeouts
    TIMEOUTS = {
        CANCEL = "5s",
        SESSION_INACTIVITY = "1800s",
        SHUTDOWN_GRACE = "10s"
    },

    CONTEXT_ACTIONS = {
        WRITE = "write",
        DELETE = "delete"
    },

    CONTEXT_COMMANDS = {
        COMMAND_SUCCESS = "command_success"
    },

    -- Error Messages
    ERR = {
        MISSING_ARGS = "Missing required arguments",
        MISSING_TOKEN = "Missing start token",
        INIT_FAILED = "Session initialization failed",
        FAILED_STATUS = "Session is in failed status",
        FAILED_STATE = "Session is in failed state",
        FAILED_COMMANDS = "Session failed, commands not available",
        EMPTY_MESSAGE = "Message cannot be empty",
        BUSY = "Session is busy processing another request",
        NO_AGENT = "No agent configured",
        AGENT_NAME_REQUIRED = "Agent name is required",
        AGENT_LOAD_FAILED = "Failed to load agent",
        MODEL_NAME_REQUIRED = "Model name is required",
        MESSAGE_ID_FAILED = "Failed to generate message ID",
        STORE_MESSAGE_FAILED = "Failed to store message",
        RESPONSE_ID_FAILED = "Failed to generate response ID",
        FUNCTION_NAME_REQUIRED = "Function name is required",
        FUNCTION_RESULT_REQUIRED = "Function result is required",
        UNSUPPORTED_COMMAND = "Unsupported command",
        CONTEXT_KEY_REQUIRED = "Context key is required",
        INVALID_CONTEXT_ACTION = "Invalid context action",
        CONTEXT_ACTION_REQUIRED = "Context action is required",
        CONTEXT_GET_FAILED = "Failed to get context",
        CONTEXT_UPDATE_FAILED = "Failed to update context",
        CONTEXT_ID_REQUIRED = "Context ID is required"
    },

    -- Internal Constants (not configurable)
    INTERNAL = {
        MIN_CONVERSATION_EXCHANGES = 3,
        TITLE_TRIGGER_MESSAGE_COUNT = 2,
        ENABLE_AGENT_CACHE = false,
        DELEGATION_DESCRIPTION_SUFFIX =
        " Tool call will be executed in a blocking mode and you will receive the result. Each delegation is isolated and does not have access to your current context. Ensure you pass all necessary context and information in your delegation message to the target agent."
    },

    -- Defaults for environment variables
    DEFAULTS = {
        MAX_MESSAGE_LIMIT = 2500,
        CHECKPOINT_FUNCTION_ID = "wippy.session.funcs:checkpoint",
        TITLE_FUNCTION_ID = "wippy.session.funcs:title",
        GC_INTERVAL = "300s",
    }
}

-- Get database resource only (lightweight)
function consts.get_db_resource()
    local db_resource, _ = env.get(consts.ENV_IDS.DATABASE_RESOURCE)
    return db_resource
end

-- Load configuration from environment variables
function consts.get_config()
    local database_resource, _ = env.get(consts.ENV_IDS.DATABASE_RESOURCE)
    local token_checkpoint_threshold, _ = env.get(consts.ENV_IDS.TOKEN_CHECKPOINT_THRESHOLD)
    local max_message_limit, _ = env.get(consts.ENV_IDS.MAX_MESSAGE_LIMIT)
    local checkpoint_function_id, _ = env.get(consts.ENV_IDS.CHECKPOINT_FUNCTION_ID)
    local title_function_id, _ = env.get(consts.ENV_IDS.TITLE_FUNCTION_ID)
    local default_host, _ = env.get(consts.ENV_IDS.DEFAULT_HOST)
    local session_security_scope, _ = env.get(consts.ENV_IDS.SESSION_SECURITY_SCOPE)
    local gc_interval, _ = env.get(consts.ENV_IDS.GC_INTERVAL)
    local delegation_func_id, _ = env.get(consts.ENV_IDS.DELEGATION_FUNC_ID)
    local on_session_end_func_id, _ = env.get(consts.ENV_IDS.ON_SESSION_END_FUNC_ID)
    local encryption_key, _ = env.get(consts.ENV_IDS.ENCRYPTION_KEY)

    return {
        -- Base configuration
        database_resource = database_resource,
        token_checkpoint_threshold = tonumber(token_checkpoint_threshold),
        max_message_limit = tonumber(max_message_limit),
        checkpoint_function_id = checkpoint_function_id,
        title_function_id = title_function_id,
        default_host = default_host,
        session_security_scope = session_security_scope,
        gc_interval = gc_interval,
        delegation_func_id = delegation_func_id,
        on_session_end_func_id = on_session_end_func_id,
        encryption_key = encryption_key,

        -- Internal constants
        enable_agent_cache = consts.INTERNAL.ENABLE_AGENT_CACHE,
        delegation_description_suffix = consts.INTERNAL.DELEGATION_DESCRIPTION_SUFFIX
    }
end

return consts

