#!/usr/bin/env bash
# Run prebuilt Linux test binaries in a disposable, network-isolated PostgreSQL.
# This is a database/query probe, not full service startup or API/UI acceptance.
set -euo pipefail
[[ $(uname -s) == Linux ]] || { echo 'Linux disposable test host required' >&2; exit 2; }
[[ ${GAK_RELEASE_TEST_CONFIRM:-} == isolated-release-database ]] || { echo 'Explicit isolated harness confirmation required' >&2; exit 2; }
BUNDLE=$(cd "${1:?usage: test-fullread-postgres.sh BUNDLE}" && pwd -P)
BINS=(fullread ai ai-document ai-auth ai-system cc cc-config crm im mp notify visibility system ticket media)
for name in "${BINS[@]}"; do
  [[ -f "$BUNDLE/$name-tests" && ! -L "$BUNDLE/$name-tests" && -x "$BUNDLE/$name-tests" ]]
  sha256sum "$BUNDLE/$name-tests"
done
CID=''
cleanup() {
  local rc=$?
  trap - EXIT
  if [[ -n "$CID" ]]; then
    [[ "$CID" =~ ^[a-f0-9]{64}$ ]] || exit 3
    [[ $(docker inspect --format '{{index .Config.Labels "gopherforge.test"}}' "$CID") == fullread ]] || exit 3
    if (( rc != 0 )); then docker logs "$CID" >&2 || true; fi
    docker rm -f "$CID" >/dev/null || exit 3
    echo "CLEANUP: exact isolated fullread container removed: $CID"
  fi
  exit "$rc"
}
trap cleanup EXIT
CID=$(docker run -d --network none --memory 768m --cpus 2 --pids-limit 128 \
  --label gopherforge.test=fullread --tmpfs /var/lib/postgresql \
  -e PGDATA=/var/lib/postgresql/test-data -e POSTGRES_PASSWORD=isolated-test-only \
  "${POSTGRES_IMAGE:-pgvector/pgvector:0.8.6-pg18}")
echo "RECEIPT: isolated fullread container=$CID"
docker inspect --format 'IMAGE: {{.Image}}' "$CID"
ready=0
for (( i=0; i<60; i++ )); do
  if docker exec "$CID" pg_isready -h 127.0.0.1 -U postgres >/dev/null 2>&1; then ready=1; break; fi
  sleep 1
done
[[ $ready == 1 ]] || { echo 'PostgreSQL readiness failed' >&2; exit 1; }
docker exec "$CID" createdb -U postgres release_fullread
for name in "${BINS[@]}"; do
  docker cp "$BUNDLE/$name-tests" "$CID:/tmp/$name-tests"
  echo "RUN: $name complete-set database regressions"
  docker exec \
    -e GAK_RELEASE_TEST_CONFIRM=isolated-release-database \
    -e 'GAK_FULLREAD_TEST_DSN=host=127.0.0.1 port=5432 user=postgres password=isolated-test-only dbname=release_fullread sslmode=disable' \
    "$CID" "/tmp/$name-tests" -test.run '^(TestCompleteSet|TestReleaseFullread|TestSpeechSettings)' -test.v -test.timeout 90s
done
echo 'PASS: bounded complete-set reads on real isolated PostgreSQL (fifteen packages)'
