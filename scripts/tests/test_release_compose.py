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


if __name__ == "__main__":
    unittest.main()
