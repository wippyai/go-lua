local M = { EVENTS = {
    ACTIVITY_LOGGED = "activity.logged",
    MEMBERSHIP_REMOVED = "membership.removed",
    MEMBERSHIP_SET = "membership.set",
    RECORD_CREATED = "record.created",
    RECORD_DELETED = "record.deleted",
    RECORD_UPDATED = "record.updated",
    RELATION_LINKED = "relation.linked",
    RELATION_UNLINKED = "relation.unlinked",
} }
function M.db_id(): string return "app:db" end
return M
