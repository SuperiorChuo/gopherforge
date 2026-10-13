#!/usr/bin/env bash
# Linux-only, disposable PostgreSQL test. Required bundle entries are listed below.
# No host ports, no host database mounts, no development database credentials.
set -euo pipefail
[[ "$(uname -s)" == Linux ]] || { echo 'Run on an isolated Linux Docker host, not macOS.' >&2; exit 2; }
BUNDLE="$(cd "${1:?usage: test-public-upgrade.sh BUNDLE}" && pwd)"
for required in migrate migration-rehearsal menu-seed-tests public-menus-fixture.sql migrations v070 public-main; do
  [[ -e "$BUNDLE/$required" ]] || { echo "Missing bundle entry: $required" >&2; exit 2; }
done
echo 'Bundle SHA-256 receipt (source probe until bound to a public candidate commit):'
(cd "$BUNDLE" && find migrate migration-rehearsal menu-seed-tests public-menus-fixture.sql migrations v070 public-main -type f -exec sha256sum {} + | sort)
TMP="$(mktemp -d /tmp/gopherforge-upgrade.XXXXXXXX)"
CID=''
cleanup() {
  local code=$?
  if [[ -n "$CID" ]]; then
    if ((code != 0)); then docker logs "$CID" || true; fi
    docker rm -f "$CID" >/dev/null || code=1
    echo 'CLEANUP: isolated database container removed'
  fi
  rm -rf -- "$TMP"
  exit "$code"
}
trap cleanup EXIT
IMAGE="$(docker image inspect "${POSTGRES_IMAGE:-pgvector/pgvector:0.8.6-pg18}" --format '{{.Id}}')"
echo "PostgreSQL image: $IMAGE"
CID="$(docker run -d --rm --network none --memory 768m --cpus 2 --pids-limit 128 \
  --tmpfs /var/lib/postgresql:rw --label gopherforge.test=public-upgrade \
  -e PGDATA=/var/lib/postgresql/test-data -e POSTGRES_PASSWORD=isolated-test-only "$IMAGE")"
# The image's temporary initialization server accepts Unix sockets before it
# restarts. Probe TCP so readiness means the final database server is available.
for ((i=0; i<60; i++)); do
  if docker exec "$CID" pg_isready -h 127.0.0.1 -U postgres >/dev/null 2>&1; then break; fi
  sleep 1
done
docker exec "$CID" pg_isready -h 127.0.0.1 -U postgres
docker exec "$CID" mkdir /release
docker cp "$BUNDLE/migrate" "$CID:/release/migrate"
docker cp "$BUNDLE/migration-rehearsal" "$CID:/release/migration-rehearsal"
docker cp "$BUNDLE/menu-seed-tests" "$CID:/release/menu-seed-tests"
for path in migrations v070 public-main; do docker cp "$BUNDLE/$path" "$CID:/release/$path"; done
printf '%s\n' 'app:' '  env: test' 'database:' '  driver: postgres' '  host: 127.0.0.1' \
  '  port: 5432' '  user: postgres' '  password: isolated-test-only' '  sslmode: disable' \
  'upload:' '  storage_type: local' > "$TMP/config.yaml"
docker cp "$TMP/config.yaml" "$CID:/release/config.yaml"
run_migration() {
  docker exec -e DB_NAME="$1" "$CID" /release/migrate -config /release/config.yaml -dir "/release/$2" "${3:-up}"
}
sql() { docker exec -i "$CID" psql -X -v ON_ERROR_STOP=1 -U postgres -d "$1"; }
data_digest() {
  # PostgreSQL 18 adds randomized psql restrict keys; omit only these non-data lines.
  docker exec "$CID" pg_dump -U postgres --data-only "$1" | grep -vE '^\\(un)?restrict ' | sha256sum
}
for route in fresh v070 public_main; do
  echo "=== upgrade route: $route ==="
  docker exec "$CID" createdb -U postgres "$route"
  if [[ "$route" != fresh ]]; then
    baseline="${route//_/-}"
    run_migration "$route" "$baseline"
    sql "$route" < "$BUNDLE/public-menus-fixture.sql"
    sql "$route" <<'SQL'
INSERT INTO tenants (id,code,name) VALUES (99001,'release-tenant','Release sentinel');
INSERT INTO users (id,tenant_id,username,password,nickname) VALUES (99001,99001,'release-user','test-only-sentinel','preserve-me');
INSERT INTO roles (id,tenant_id,name,code) VALUES (99001,99001,'Release role','release-role');
INSERT INTO permissions (id,name,code,type) VALUES (99001,'Custom permission','release:custom',2);
INSERT INTO role_permissions (role_id,permission_id) VALUES (99001,99001);
INSERT INTO menus (id,name,title,path) VALUES (99001,'CustomMenu','Custom menu','/release-custom');
INSERT INTO ops_task_runs (id,run_id,task_key,service,status,started_at,message) VALUES (99001,'release-history','release.task','monitor','succeeded',NOW(),'preserve-task-history');
-- Minimal relational fixtures exercise ownership/FK preservation, not service AutoMigrate acceptance.
CREATE TABLE bpm_process_definition(id BIGSERIAL PRIMARY KEY, tenant_id BIGINT NOT NULL, name TEXT NOT NULL);
CREATE TABLE bpm_process_instance(id BIGSERIAL PRIMARY KEY, definition_id BIGINT REFERENCES bpm_process_definition(id));
INSERT INTO bpm_process_definition(id,tenant_id,name) VALUES (99001,99001,'preserve-definition');
INSERT INTO bpm_process_instance(id,definition_id) VALUES (99001,99001);
-- Consume then delete higher IDs: MAX(id) alone must never rewind any allocator.
SELECT setval(pg_get_serial_sequence('menus','id'), 200000, true);
INSERT INTO menus(name,title,path) VALUES ('release-deleted-menu','deleted','/release-deleted');
DELETE FROM menus WHERE name='release-deleted-menu';
SELECT setval(pg_get_serial_sequence('bpm_process_definition','id'), 210000, true);
INSERT INTO bpm_process_definition(tenant_id,name) VALUES (99001,'release-deleted-definition');
DELETE FROM bpm_process_definition WHERE name='release-deleted-definition';
SELECT setval(pg_get_serial_sequence('public.monitor_metric_samples','id'), 220000, true);
INSERT INTO public.monitor_metric_samples(category,metric,value) VALUES ('test','release-deleted-metric',1);
DELETE FROM public.monitor_metric_samples WHERE metric='release-deleted-metric';
SELECT setval(pg_get_serial_sequence('ops_task_runs','id'), 230000, true);
INSERT INTO ops_task_runs(run_id,task_key,service,status,started_at) VALUES ('release-deleted-task','release.task','monitor','succeeded',NOW());
DELETE FROM ops_task_runs WHERE run_id='release-deleted-task';
-- Missing default with an existing unowned repaired sequence must preserve that high-water too.
CREATE TABLE bpm_task(id BIGINT PRIMARY KEY);
INSERT INTO bpm_task VALUES (99001);
CREATE SCHEMA IF NOT EXISTS bpm_svc;
CREATE SEQUENCE bpm_svc.bpm_task_id_seq_repaired;
SELECT setval('bpm_svc.bpm_task_id_seq_repaired', 260000, true);
CREATE SCHEMA release_probe;
CREATE TABLE release_probe.identities AS SELECT 'public.ops_task_runs'::regclass::oid AS task_oid, 'public.bpm_process_definition'::regclass::oid AS definition_oid;
SQL
  fi
  run_migration "$route" migrations
  run_migration "$route" migrations
  sql "$route" <<'SQL'
DO $$ BEGIN
  IF (SELECT MAX(version_id) FROM goose_db_version WHERE is_applied) <> 113 THEN
    RAISE EXCEPTION 'wrong migration head';
  END IF;
  IF to_regclass('ai_svc.agent_definitions') IS NULL OR to_regclass('monitor_svc.ops_task_runs') IS NULL THEN
    RAISE EXCEPTION 'missing full-product tables';
  END IF;
END $$;
SQL
  if [[ "$route" != fresh ]]; then
    sql "$route" <<'SQL'
DO $$ DECLARE next_id BIGINT; BEGIN
  IF NOT EXISTS (SELECT 1 FROM users WHERE id=99001 AND tenant_id=99001 AND nickname='preserve-me' AND password='test-only-sentinel') THEN
    RAISE EXCEPTION 'user/tenant data changed';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM role_permissions WHERE role_id=99001 AND permission_id=99001)
     OR NOT EXISTS (SELECT 1 FROM permissions WHERE id=99001 AND code='release:custom')
     OR NOT EXISTS (SELECT 1 FROM menus WHERE id=99001 AND path='/release-custom') THEN
    RAISE EXCEPTION 'custom permission/menu changed';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM monitor_svc.ops_task_runs WHERE id=99001 AND run_id='release-history' AND message='preserve-task-history') THEN
    RAISE EXCEPTION 'task history lost';
  END IF;
  IF 'monitor_svc.ops_task_runs'::regclass::oid <> (SELECT task_oid FROM release_probe.identities)
     OR 'bpm_svc.bpm_process_definition'::regclass::oid <> (SELECT definition_oid FROM release_probe.identities) THEN
    RAISE EXCEPTION 'table identity changed';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM bpm_svc.bpm_process_definition WHERE tenant_id=99001 AND name='preserve-definition') THEN
    RAISE EXCEPTION 'business data changed';
  END IF;
  BEGIN
    INSERT INTO bpm_svc.bpm_process_instance(definition_id) VALUES (-1);
    RAISE EXCEPTION 'foreign key lost';
  EXCEPTION WHEN foreign_key_violation THEN NULL;
  END;
  INSERT INTO bpm_svc.bpm_process_definition(tenant_id,name) VALUES (99001,'new-definition') RETURNING id INTO next_id;
  IF next_id <= 210001 THEN RAISE EXCEPTION 'owned business sequence rewound'; END IF;
  INSERT INTO menus(name,title,path) VALUES ('release-after-menu','after','/release-after') RETURNING id INTO next_id;
  IF next_id <= 200001 THEN RAISE EXCEPTION 'menu sequence rewound'; END IF;
  INSERT INTO monitor_svc.monitor_metric_samples(category,metric,value) VALUES ('test','release-after',1) RETURNING id INTO next_id;
  IF next_id <= 220001 THEN RAISE EXCEPTION 'shared unowned sequence rewound'; END IF;
  INSERT INTO bpm_svc.bpm_task DEFAULT VALUES RETURNING id INTO next_id;
  IF next_id <= 260000 THEN RAISE EXCEPTION 'existing repaired sequence rewound'; END IF;
  INSERT INTO monitor_svc.ops_task_runs(run_id,task_key,service,status,started_at) VALUES ('release-after','release.task','monitor','succeeded',NOW()) RETURNING id INTO next_id;
  IF next_id <= 230001 THEN RAISE EXCEPTION 'task sequence rewound'; END IF;
END $$;
SQL
  fi
  before_down="$(data_digest "$route")"
  if run_migration "$route" migrations down > "$TMP/down.log" 2>&1; then
    echo 'Unsafe downgrade unexpectedly succeeded' >&2; exit 1
  fi
  grep -q 'Full product upgrade is forward-only' "$TMP/down.log"
  [[ "$before_down" == "$(data_digest "$route")" ]] || { echo 'Rejected downgrade changed data or sequences' >&2; exit 1; }
  echo "PASS: $route migrations/idempotence/data/constraints/sequences/rollback-guard"
  docker exec -e GAK_RELEASE_TEST_CONFIRM=isolated-release-database \
    -e "GAK_RELEASE_TEST_DSN=host=127.0.0.1 port=5432 user=postgres password=isolated-test-only dbname=$route sslmode=disable" \
    "$CID" /release/menu-seed-tests -test.run '^TestReleaseMenuBootstrapPostgres$' -test.v -test.timeout 90s
  echo "PASS: $route real PostgreSQL default menu bootstrap"
done

# Exercise the exact rehearsal binary used by CI, including its unchanged legacy mode.
for mode in legacy forward; do
  args=(); dir=v070
  if [[ "$mode" == forward ]]; then args=(-forward-only); dir=migrations; fi
  docker exec "$CID" /release/migration-rehearsal -config /release/config.yaml \
    -dir "/release/$dir" -database "release_rehearsal_$mode" "${args[@]}"
  echo "PASS: CI rehearsal $mode"
done

# Existing custom menus must fail before AI insertion, without silently losing/reparenting them.
for menu_id in 900 901 902; do
  db="menu_conflict_$menu_id"
  docker exec "$CID" createdb -U postgres "$db"
  run_migration "$db" public-main
  printf "INSERT INTO menus(id,name,title,path) VALUES (%s,'release-collision','custom','/release-custom');\n" "$menu_id" | sql "$db"
  before="$(data_digest "$db")"
  if run_migration "$db" migrations > "$TMP/conflict.log" 2>&1; then
    echo "Menu ID $menu_id conflict unexpectedly accepted" >&2; exit 1
  fi
  grep -q 'AI menu identity conflict' "$TMP/conflict.log"
  [[ "$before" == "$(data_digest "$db")" ]] || { echo 'Rejected conflict changed database data' >&2; exit 1; }
  echo "PASS: menu conflict $menu_id refused without changing data"
done
