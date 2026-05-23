-- +goose Up
RENAME TABLE
  `wm_audit_log` TO `audit_logs`,
  `wm_console_route` TO `console_routes`,
  `wm_console_session` TO `console_sessions`,
  `wm_system_setting` TO `system_settings`;

ALTER TABLE `audit_logs`
  RENAME INDEX `idx_wm_audit_log_created_at` TO `idx_audit_logs_created_at`,
  RENAME INDEX `idx_wm_audit_log_actor_type` TO `idx_audit_logs_actor_type`,
  RENAME INDEX `idx_wm_audit_log_actor_id` TO `idx_audit_logs_actor_id`,
  RENAME INDEX `idx_wm_audit_log_action` TO `idx_audit_logs_action`,
  RENAME INDEX `idx_wm_audit_log_target_type` TO `idx_audit_logs_target_type`,
  RENAME INDEX `idx_wm_audit_log_target_id` TO `idx_audit_logs_target_id`;

ALTER TABLE `console_routes`
  RENAME INDEX `idx_wm_console_route_path` TO `idx_console_routes_path`,
  RENAME INDEX `idx_wm_console_route_name` TO `idx_console_routes_name`,
  RENAME INDEX `idx_wm_console_route_sort_order` TO `idx_console_routes_sort_order`,
  RENAME INDEX `idx_wm_console_route_parent_key` TO `idx_console_routes_parent_key`,
  RENAME INDEX `idx_wm_console_route_enabled` TO `idx_console_routes_enabled`;

ALTER TABLE `console_sessions`
  RENAME INDEX `idx_wm_console_session_username` TO `idx_console_sessions_username`,
  RENAME INDEX `idx_wm_console_session_issued_at` TO `idx_console_sessions_issued_at`,
  RENAME INDEX `idx_wm_console_session_expires_at` TO `idx_console_sessions_expires_at`,
  RENAME INDEX `idx_wm_console_session_revoked_at` TO `idx_console_sessions_revoked_at`;

ALTER TABLE `system_settings`
  RENAME INDEX `idx_wm_system_setting_updated_at` TO `idx_system_settings_updated_at`;

-- +goose Down
ALTER TABLE `system_settings`
  RENAME INDEX `idx_system_settings_updated_at` TO `idx_wm_system_setting_updated_at`;

ALTER TABLE `console_sessions`
  RENAME INDEX `idx_console_sessions_username` TO `idx_wm_console_session_username`,
  RENAME INDEX `idx_console_sessions_issued_at` TO `idx_wm_console_session_issued_at`,
  RENAME INDEX `idx_console_sessions_expires_at` TO `idx_wm_console_session_expires_at`,
  RENAME INDEX `idx_console_sessions_revoked_at` TO `idx_wm_console_session_revoked_at`;

ALTER TABLE `console_routes`
  RENAME INDEX `idx_console_routes_path` TO `idx_wm_console_route_path`,
  RENAME INDEX `idx_console_routes_name` TO `idx_wm_console_route_name`,
  RENAME INDEX `idx_console_routes_sort_order` TO `idx_wm_console_route_sort_order`,
  RENAME INDEX `idx_console_routes_parent_key` TO `idx_wm_console_route_parent_key`,
  RENAME INDEX `idx_console_routes_enabled` TO `idx_wm_console_route_enabled`;

ALTER TABLE `audit_logs`
  RENAME INDEX `idx_audit_logs_created_at` TO `idx_wm_audit_log_created_at`,
  RENAME INDEX `idx_audit_logs_actor_type` TO `idx_wm_audit_log_actor_type`,
  RENAME INDEX `idx_audit_logs_actor_id` TO `idx_wm_audit_log_actor_id`,
  RENAME INDEX `idx_audit_logs_action` TO `idx_wm_audit_log_action`,
  RENAME INDEX `idx_audit_logs_target_type` TO `idx_wm_audit_log_target_type`,
  RENAME INDEX `idx_audit_logs_target_id` TO `idx_wm_audit_log_target_id`;

RENAME TABLE
  `system_settings` TO `wm_system_setting`,
  `console_sessions` TO `wm_console_session`,
  `console_routes` TO `wm_console_route`,
  `audit_logs` TO `wm_audit_log`;
