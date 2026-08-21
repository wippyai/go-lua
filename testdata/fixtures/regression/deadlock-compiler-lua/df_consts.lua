local M = {}

M.STATUS = {
    PENDING = "pending",
    TEMPLATE = "template"
}

M.DATA_TYPE = {
    NODE_INPUT = "node_input",
    NODE_OUTPUT = "node_output",
    WORKFLOW_INPUT = "workflow_input",
    WORKFLOW_OUTPUT = "workflow_output"
}

M.CONTENT_TYPE = {
    JSON = "application/json",
    TEXT = "text/plain",
    REFERENCE = "dataflow/reference"
}

M.COMMAND_TYPES = {
    CREATE_DATA = "create_data",
    CREATE_NODE = "create_node"
}

return M
