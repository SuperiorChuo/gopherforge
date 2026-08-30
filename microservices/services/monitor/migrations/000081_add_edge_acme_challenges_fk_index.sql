-- +goose Up

-- P1 证据批（2026-08-30）：全库 pg_constraint 扫描仅此一处外键缺索引。
-- edge_acme_challenges.certificate_id 指向 edge_tls_certificates(id) ON DELETE CASCADE，
-- 父表删除沿外键逐行定位子行，缺索引时是顺序扫描； challenge 表按证书生命周期
-- 持续增删，补上索引让级联删除回到索引查找。

CREATE INDEX IF NOT EXISTS idx_edge_acme_challenges_certificate_id
    ON edge_acme_challenges (certificate_id);

-- +goose Down

DROP INDEX IF EXISTS idx_edge_acme_challenges_certificate_id;
