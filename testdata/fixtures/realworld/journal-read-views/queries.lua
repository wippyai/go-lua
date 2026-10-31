local M = {}

function M.spans(journal_id: string, opts: any): (any?, string?) return nil, nil end
function M.activity(journal_id: string, opts: any): (any?, string?) return nil, nil end
function M.authority(journal_id: string, opts: any?): (any?, string?) return nil, nil end
function M.state(journal_id: string, opts: any?): (any?, string?) return nil, nil end
function M.brief(journal_id: string, opts: any): (any?, string?) return nil, nil end
function M.health(journal_id: string, opts: any?): (any?, string?) return nil, nil end
function M.tasks(journal_id: string, opts: any): (any?, string?) return nil, nil end
function M.search_text(journal_id: string, query_text: string, opts: any): (any?, string?) return nil, nil end
function M.references(journal_id: string, opts: any): (any?, string?) return nil, nil end
function M.branch(journal_id: string, task_id: any, opts: any): (any?, string?) return nil, nil end
function M.frontier(journal_id: string, opts: any): (any?, string?) return nil, nil end
function M.changes(journal_id: string, opts: any): (any?, string?) return nil, nil end
function M.tree(journal_id: string, root_task_id: any, depth: any, limit: any): (any?, string?) return nil, nil end

return M
