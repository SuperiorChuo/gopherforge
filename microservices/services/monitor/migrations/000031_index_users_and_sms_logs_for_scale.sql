-- +goose Up
-- 补 000015 的两处遗漏，原则与其一致（见该文件抬头）。
--   1. users 列表按 (tenant_id, created_at DESC) 分页，但 users 从未建过 created_at
--      相关索引（000001 只建了 username/email/phone/department_id，000012 补的是
--      tenant_* 唯一索引），每翻一页都要把该租户的用户全取出来排序；
--   2. sms_logs 建于 000019，晚于 000015 却没跟上其口径：缺列表用的复合索引，
--      且带着两个 000015 明确要清理的形态——mobile 只被 LIKE '%kw%' 使用（btree
--      命中不了），status 是 sending/success/failure 三值低基数。
-- 当前两表都很小，直接 CREATE INDEX 即可；若未来在大表上补，改用 CONCURRENTLY。

CREATE INDEX IF NOT EXISTS idx_users_tenant_created ON users (tenant_id, created_at DESC);

-- sms_logs 列表：WHERE tenant_id = ? ORDER BY id DESC。
CREATE INDEX IF NOT EXISTS idx_sms_logs_tenant_id_desc ON sms_logs (tenant_id, id DESC);
-- 保留 created_at 单列索引给按时间的 retention 清理（与 000015 第 3 条同口径）。
CREATE INDEX IF NOT EXISTS idx_sms_logs_created_at ON sms_logs (created_at);

DROP INDEX IF EXISTS idx_sms_logs_mobile;
DROP INDEX IF EXISTS idx_sms_logs_status;
-- 被上面的复合索引前缀覆盖。
DROP INDEX IF EXISTS idx_sms_logs_tenant_id;

-- +goose Down
CREATE INDEX IF NOT EXISTS idx_sms_logs_tenant_id ON sms_logs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_sms_logs_status ON sms_logs (status);
CREATE INDEX IF NOT EXISTS idx_sms_logs_mobile ON sms_logs (mobile);

DROP INDEX IF EXISTS idx_sms_logs_created_at;
DROP INDEX IF EXISTS idx_sms_logs_tenant_id_desc;
DROP INDEX IF EXISTS idx_users_tenant_created;
