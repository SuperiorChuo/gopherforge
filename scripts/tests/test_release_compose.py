"""Contracts for the documented complete-product Compose fallback."""
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCHEMAS = {
    "ai": "ai_svc", "mp": "mp_svc", "im": "im_svc",
    "monitor": "monitor_svc", "audit": "audit_svc", "cc": "cc_svc",
    "crm": "crm_svc", "visibility": "vis_svc", "ticket": "ticket_svc",
    "pay": "pay_svc", "bpm": "bpm_svc",
}
IDENTITY_CLIENTS = ("ai", "notify", "crm", "visibility", "bpm")
IM_VISITOR_PREFIXES = (
    "/api/v1/im/visitor", "/api/v1/im/conversations",
    "/api/v1/im/widget", "/api/v1/im/attachments",
)


def environment(source, service):
    # These manifests use service-local, six-space scalar environment maps.
    # Reject missing/duplicate blocks or keys rather than accepting a comment
    # or a value attached to a different service. The release matrix additionally
    # parses the complete manifests as YAML.
    blocks = re.findall(
        rf"^  {re.escape(service)}-service:\n(.*?)(?=^  \S|\Z)",
        source, re.M | re.S,
    )
    if len(blocks) != 1:
        raise AssertionError(f"expected one service block: {service}")
    maps = re.findall(r"^    environment:\n((?:^      .*\n|^\s*\n)*)", blocks[0], re.M)
    if len(maps) != 1:
        raise AssertionError(f"expected one environment map: {service}")
    pairs = re.findall(r"^      ([A-Z][A-Z0-9_]*):\s*([^\n]*)$", maps[0], re.M)
    if len(dict(pairs)) != len(pairs):
        raise AssertionError(f"duplicate environment key: {service}")
    return dict(pairs)


class ComposeRuntimeContracts(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.compose = (ROOT / "microservices/docker-compose.yml").read_text()
        cls.stack = (ROOT / "microservices/docker-stack.yml").read_text()

    def test_schema_paths_resolve_migrated_tables_in_both_deployment_modes(self):
        for name, schema in SCHEMAS.items():
            for mode, source in (("compose", self.compose), ("stack", self.stack)):
                with self.subTest(service=name, mode=mode):
                    self.assertEqual(environment(source, name).get("DB_SEARCH_PATH"), f"{schema},public")

    def test_identity_owner_and_clients_share_explicit_internal_credential(self):
        for mode, source in (("compose", self.compose), ("stack", self.stack)):
            for name in ("identity", *IDENTITY_CLIENTS):
                with self.subTest(service=name, mode=mode):
                    env = environment(source, name)
                    # Empty remains fail-closed; never add a public default secret.
                    self.assertEqual(env.get("INTERNAL_TOKEN"), "${INTERNAL_TOKEN:-}")
                    if name != "identity":
                        self.assertEqual(env.get("IDENTITY_API_BASE"), "http://go-admin-kit-identity:8083")

    def test_audit_delivery_has_event_bus_and_authenticated_notify(self):
        # The migrated outbox requires the Audit worker's NATS connection;
        # Notify's client is disabled unless both endpoint and token are set.
        expected = {
            ("audit", "NOTIFY_API_BASE"): "${NOTIFY_API_BASE:-http://go-admin-kit-notify:8095}",
            ("audit", "NOTIFY_INTERNAL_TOKEN"): "${NOTIFY_INTERNAL_TOKEN:-}",
            ("notify", "NOTIFY_INTERNAL_TOKEN"): "${NOTIFY_INTERNAL_TOKEN:-}",
            ("audit", "NATS_URL"): "nats://go-admin-kit-nats:4222",
        }
        for mode, source in (("compose", self.compose), ("stack", self.stack)):
            for (service, key), value in expected.items():
                with self.subTest(mode=mode, service=service, key=key):
                    self.assertEqual(environment(source, service).get(key), value)

    def test_im_backend_router_leaves_management_routes_to_frontend(self):
        permissions = (ROOT / "microservices/web/src/router/route-permissions.ts").read_text()
        admin_paths = re.findall(r"^\s*'(/im/[^']+)':", permissions, re.M)
        self.assertIn("/im/sites", admin_paths)
        backend_paths = (
            "/api/v1/im/health/live", "/api/v1/im/admin/sites", "/api/v1/im/visitor/session",
            "/im/ws", "/im/visitor", "/im/visitor/static", "/im/visitor/static/main.js",
            "/im/widget", "/im/widget/widget.js", "/im/uploads", "/im/uploads/file.jpg",
        )
        for mode, source in (("compose", self.compose), ("stack", self.stack)):
            blocks = re.findall(r"^  im-service:\n(.*?)(?=^  \S|\Z)", source, re.M | re.S)
            self.assertEqual(len(blocks), 1)
            rules = re.findall(r"^ +traefik\.http\.routers\.im\.rule: ([^\n]+)$", blocks[0], re.M)
            self.assertEqual(len(rules), 1)
            terms = []
            # Validate the actual, intentionally simple OR-only Traefik rule;
            # unsupported matcher syntax must fail, not silently be ignored.
            for term in rules[0].strip("\"'").split(" || "):
                match = re.fullmatch(r"(Path|PathPrefix)\(`([^`]+)`\)", term)
                self.assertIsNotNone(match, term)
                terms.append(match.groups())

            def matches(path):
                return any(path == value if kind == "Path" else path.startswith(value)
                           for kind, value in terms)

            for path in backend_paths:
                with self.subTest(mode=mode, backend=path):
                    self.assertTrue(matches(path))
            for path in (*admin_paths, "/im/widget-settings", "/im/visitor-admin", "/im/ws-debug", "/im/uploads-manager"):
                with self.subTest(mode=mode, frontend=path):
                    self.assertFalse(matches(path))
    def test_im_visitor_router_skips_access_token_but_strips_client_identity(self):
        # Guest tokens are IM-issued Bearer credentials, not auth JWTs, so the
        # gateway must route these REST paths without access-token verification
        # (Compose used to reject every legitimate guest). The shared bypass
        # must never skip auth-sanitize: identity headers stay authoritative
        # only if the gateway cleared client-supplied X-Auth-* values first.
        for mode, source in (("compose", self.compose), ("stack", self.stack)):
            blocks = re.findall(r"^  im-service:\n(.*?)(?=^  \S|\Z)", source, re.M | re.S)
            self.assertEqual(len(blocks), 1)
            rules = re.findall(r"^ +traefik\.http\.routers\.im-open\.rule: ([^\n]+)$", blocks[0], re.M)
            self.assertEqual(len(rules), 1)
            terms = []
            for term in rules[0].strip("\"'").split(" || "):
                match = re.fullmatch(r"(Path|PathPrefix)\(`([^`]+)`\)", term)
                self.assertIsNotNone(match, term)
                terms.append(match.groups())
            priorities = re.findall(
                r"^ +traefik\.http\.routers\.im-open\.priority: ['\"]?(\d+)['\"]?$", blocks[0], re.M)
            self.assertEqual(len(priorities), 1)
            self.assertGreater(int(priorities[0]), 100)  # must win over the auth-chain router
            middlewares = re.findall(
                r"^ +traefik\.http\.routers\.im-open\.middlewares: ([^\n]+)$", blocks[0], re.M)
            self.assertEqual(middlewares, ["auth-sanitize@docker" if mode == "stack" else "auth-sanitize"])
            main_middlewares = re.findall(
                r"^ +traefik\.http\.routers\.im\.middlewares: ([^\n]+)$", blocks[0], re.M)
            self.assertEqual(main_middlewares, ["auth-chain@docker" if mode == "stack" else "auth-chain"])

            def matches(path):
                return any(path == value if kind == "Path" else path.startswith(value)
                           for kind, value in terms)

            for prefix in IM_VISITOR_PREFIXES:
                with self.subTest(mode=mode, visitor=prefix):
                    self.assertTrue(matches(prefix))
                    self.assertTrue(matches(prefix + "/item"))
            for path in ("/api/v1/im/agent/me", "/api/v1/im/admin/sites",
                         "/im/visitor", "/api/v1/imx/conversations", "/api/v1/other"):
                with self.subTest(mode=mode, protected=path):
                    self.assertFalse(matches(path))

    def test_im_service_verifies_agent_tokens_with_their_owner(self):
        # Mixed visitor/agent endpoints cannot trust gateway headers alone;
        # im-service must reach auth-service's /internal/verify for revocation
        # and current IM permissions.
        for mode, source in (("compose", self.compose), ("stack", self.stack)):
            with self.subTest(mode=mode):
                self.assertEqual(
                    environment(source, "im").get("AUTH_API_BASE"),
                    "${AUTH_API_BASE:-http://go-admin-kit-auth:8082}",
                )


if __name__ == "__main__":
    unittest.main()
