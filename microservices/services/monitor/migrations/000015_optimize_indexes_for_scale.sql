-- +goose Up
-- 千万级数据准备：把日志/消息类高增长表的单列索引整合为匹配实际查询的复合索引。
-- 原则：
--   1. 列表查询统一走 (tenant_id, created_at DESC) 前缀（tenant.ApplyFilter + ORDER BY created_at DESC）；
--   2. 删除只被 LIKE '%kw%' 使用的 btree 索引（无法命中）和低基数单列索引（actor_type 等），
--      减少千万级写入时的索引维护开销；
--   3. 保留 created_at 单列索引给按时间的 retention 清理（DELETE WHERE created_at < ?，无租户过滤）。
-- 注：当前表都很小，直接 CREATE INDEX 即可；若未来在大表上补索引，改用 CONCURRENTLY。

-- operation_logs：列表 tenant+created_at DESC；按操作人 user_id+时间；request_id 精确查。
CREATE INDEX IF NOT EXISTS idx_operation_logs_tenant_created ON operation_logs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_operation_logs_user_created ON operation_logs (user_id, created_at DESC);
DROP INDEX IF EXISTS idx_operation_logs_tenant_id;
DROP INDEX IF EXISTS idx_operation_logs_user_id;
DROP INDEX IF EXISTS idx_operation_logs_actor_type;
DROP INDEX IF EXISTS idx_operation_logs_actor_id;

-- login_logs：列表 + 登录限流热路径（status=0 AND created_at>=? AND username|ip=?）。
CREATE INDEX IF NOT EXISTS idx_login_logs_tenant_created ON login_logs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_login_logs_user_created ON login_logs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_login_logs_fail_username ON login_logs (username, created_at) WHERE status = 0;
CREATE INDEX IF NOT EXISTS idx_login_logs_fail_ip ON login_logs (ip, created_at) WHERE status = 0;
DROP INDEX IF EXISTS idx_login_logs_tenant_id;
DROP INDEX IF EXISTS idx_login_logs_user_id;

-- audit_logs：列表 tenant+created_at DESC；实体变更历史 (target_type, target_id, 时间)。
-- action / target_id 保留单列（精确过滤 + facets/统计）；actor_* 只有 LIKE 或低基数，删除。
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_created ON audit_logs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_target ON audit_logs (target_type, target_id, created_at DESC);
DROP INDEX IF EXISTS idx_audit_logs_tenant_id;
DROP INDEX IF EXISTS idx_audit_logs_actor_type;
DROP INDEX IF EXISTS idx_audit_logs_actor_id;
DROP INDEX IF EXISTS idx_audit_logs_target_type;

-- files：列表 tenant+created_at DESC；(tenant_id,user_id)、hash、user_id 维持不变。
CREATE INDEX IF NOT EXISTS idx_files_tenant_created ON files (tenant_id, created_at DESC);
DROP INDEX IF EXISTS idx_files_tenant_id;

-- scheduled_job_logs：按任务查执行历史。
CREATE INDEX IF NOT EXISTS idx_scheduled_job_logs_job_created ON scheduled_job_logs (job_id, created_at DESC);
DROP INDEX IF EXISTS idx_scheduled_job_logs_job_id;

-- +goose Down
CREATE INDEX IF NOT EXISTS idx_operation_logs_tenant_id ON operation_logs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_operation_logs_user_id ON operation_logs (user_id);
CREATE INDEX IF NOT EXISTS idx_operation_logs_actor_type ON operation_logs (actor_type);
CREATE INDEX IF NOT EXISTS idx_operation_logs_actor_id ON operation_logs (actor_id);
DROP INDEX IF EXISTS idx_operation_logs_tenant_created;
DROP INDEX IF EXISTS idx_operation_logs_user_created;

CREATE INDEX IF NOT EXISTS idx_login_logs_tenant_id ON login_logs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_login_logs_user_id ON login_logs (user_id);
DROP INDEX IF EXISTS idx_login_logs_tenant_created;
DROP INDEX IF EXISTS idx_login_logs_user_created;
DROP INDEX IF EXISTS idx_login_logs_fail_username;
DROP INDEX IF EXISTS idx_login_logs_fail_ip;

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_id ON audit_logs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_type ON audit_logs (actor_type);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_id ON audit_logs (actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_target_type ON audit_logs (target_type);
DROP INDEX IF EXISTS idx_audit_logs_tenant_created;
DROP INDEX IF EXISTS idx_audit_logs_target;

CREATE INDEX IF NOT EXISTS idx_files_tenant_id ON files (tenant_id);
DROP INDEX IF EXISTS idx_files_tenant_created;

CREATE INDEX IF NOT EXISTS idx_scheduled_job_logs_job_id ON scheduled_job_logs (job_id);
DROP INDEX IF EXISTS idx_scheduled_job_logs_job_created;
