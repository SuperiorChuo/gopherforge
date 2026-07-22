# System Config & Ops

The system service hosts operational features with one principle: **configurable parameters live in the database and hot-reload; environment variables are only fallbacks**.

- **Settings (hot config)**: `system_settings` stores JSON per `group.key`; the console renders forms metadata-driven. Services read via a TTL-cached reader (~30s to take effect, no restarts). Examples: `security.policy`, `ai.provider`.
- **Dictionaries**: two-level type/item structure backing dropdowns and tags.
- **Notices**: publish/retire announcements shown after login.
- **SMS**: channels (pluggable providers: debug / Aliyun / Tencent) + templates + send logs; the debug provider enables zero-dependency local development.
- **Error codes**: edit user-facing messages online, effective in ~30 seconds — no redeploy to fix a message.
- **Online users**: view and force-logout current sessions.
