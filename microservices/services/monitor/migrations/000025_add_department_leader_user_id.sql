-- +goose Up
-- 部门主管外键（BPM M2 dept_leader 审批人规则的数据来源，设计文档 §8-Q1 方案 A）：
-- departments.Leader 原为纯字符串展示名，无法解析为审批人；新增 leader_user_id
-- 指向 users.id（逻辑外键，0=未设，不做级联约束——主管离职/删除由审批侧
-- emptyFallback 三策略兜底）。
-- 列属 identity 服务的 departments 表，但迁移统一放 monitor/migrations（migrate job 唯一执行目录）。
ALTER TABLE departments ADD COLUMN IF NOT EXISTS leader_user_id BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE departments DROP COLUMN IF EXISTS leader_user_id;
