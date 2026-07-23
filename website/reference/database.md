# 数据库表结构

本文档由 `tools/dbdoc` 连接数据库自动生成，请勿手改；schema 变更后重新生成。

## 目录

- **身份与权限**（11）：`departments` `menu_permissions` `menus` `permissions` `role_data_scope_departments` `role_permissions` `roles` `sys_posts` `sys_user_posts` `user_roles` `users`
- **认证与安全**（5）：`console_routes` `console_sessions` `oauth_bindings` `password_history` `totp_recovery_codes`
- **多租户**（2）：`tenant_packages` `tenants`
- **系统运营**（10）：`dict_items` `dict_types` `error_codes` `notices` `scheduled_job_logs` `scheduled_jobs` `sms_channels` `sms_logs` `sms_templates` `system_settings`
- **审计日志**（3）：`audit_logs` `login_logs` `operation_logs`
- **文件服务**（1）：`files`
- **审批流（BPM）**（5）：`bpm_cc_record` `bpm_process_definition` `bpm_process_instance` `bpm_process_log` `bpm_task`
- **迁移框架**（2）：`goose_db_version` `schema_migrations`

## 身份与权限

### departments

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| name | character varying(100) | 否 |  |  |
| code | character varying(50) | 是 | NULL::character varying |  |
| parent_id | bigint | 否 | 0 |  |
| leader | character varying(50) | 是 | ''::character varying |  |
| phone | character varying(20) | 是 | ''::character varying |  |
| email | character varying(100) | 是 | ''::character varying |  |
| sort | bigint | 否 | 0 |  |
| status | smallint | 否 | 1 |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| tenant_id | bigint | 否 | 1 |  |
| leader_user_id | bigint | 否 | 0 |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (parent_id)`
- `btree (tenant_id, code) [UNIQUE]`
- `btree (tenant_id)`

### menu_permissions

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| menu_id | bigint | 否 |  |  |
| permission_id | bigint | 否 |  |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (permission_id)`
- `btree (menu_id, permission_id) [UNIQUE]`

### menus

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| name | character varying(50) | 否 |  |  |
| title | character varying(50) | 否 |  |  |
| icon | character varying(100) | 是 | ''::character varying |  |
| path | character varying(255) | 是 | ''::character varying |  |
| component | character varying(255) | 是 | ''::character varying |  |
| parent_id | bigint | 否 | 0 |  |
| sort | bigint | 否 | 0 |  |
| status | smallint | 否 | 1 |  |
| hidden | smallint | 否 | 0 |  |
| permission | character varying(100) | 是 | ''::character varying |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (parent_id)`

### permissions

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| name | character varying(50) | 否 |  |  |
| code | character varying(100) | 否 |  |  |
| type | smallint | 否 |  |  |
| path | character varying(255) | 是 | ''::character varying |  |
| method | character varying(10) | 是 | ''::character varying |  |
| parent_id | bigint | 否 | 0 |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| description | character varying(255) | 否 | ''::character varying |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (code) [UNIQUE]`
- `btree (parent_id)`

### role_data_scope_departments

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| role_id | bigint | 否 |  |  |
| department_id | bigint | 否 |  |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (department_id)`
- `btree (role_id, department_id) [UNIQUE]`

### role_permissions

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| role_id | bigint | 否 |  |  |
| permission_id | bigint | 否 |  |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (permission_id)`
- `btree (role_id, permission_id) [UNIQUE]`

### roles

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| name | character varying(50) | 否 |  |  |
| code | character varying(50) | 否 |  |  |
| description | character varying(255) | 是 | ''::character varying |  |
| data_scope | character varying(32) | 否 | 'self'::character varying |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| tenant_id | bigint | 否 | 1 |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (data_scope)`
- `btree (tenant_id, code) [UNIQUE]`
- `btree (tenant_id)`

### sys_posts

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| tenant_id | bigint | 否 | 1 |  |
| code | character varying(64) | 否 |  |  |
| name | character varying(64) | 否 |  |  |
| sort | bigint | 否 | 0 |  |
| status | smallint | 否 | 1 |  |
| remark | character varying(500) | 是 | ''::character varying |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (tenant_id)`
- `btree (tenant_id, code) [UNIQUE]`

### sys_user_posts

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| user_id | bigint | 否 |  |  |
| post_id | bigint | 否 |  |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (post_id)`
- `btree (user_id, post_id) [UNIQUE]`

### user_roles

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| user_id | bigint | 否 |  |  |
| role_id | bigint | 否 |  |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (role_id)`
- `btree (user_id, role_id) [UNIQUE]`

### users

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| username | character varying(50) | 否 |  |  |
| password | character varying(255) | 否 |  |  |
| nickname | character varying(50) | 是 | ''::character varying |  |
| email | character varying(100) | 是 | NULL::character varying |  |
| phone | character varying(20) | 是 | NULL::character varying |  |
| avatar | character varying(255) | 是 | ''::character varying |  |
| department_id | bigint | 否 | 0 |  |
| must_change_password | boolean | 否 | false |  |
| status | smallint | 否 | 1 |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| password_changed_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| totp_secret | character varying(255) | 否 | ''::character varying |  |
| totp_enabled | boolean | 否 | false |  |
| tenant_id | bigint | 否 | 1 |  |
| is_platform_admin | boolean | 否 | false |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (department_id)`
- `btree (tenant_id, email) WHERE ((email IS NOT NULL) AND ((email)::text <> ''::text)) [UNIQUE]`
- `btree (tenant_id)`
- `btree (tenant_id, phone) WHERE ((phone IS NOT NULL) AND ((phone)::text <> ''::text)) [UNIQUE]`
- `btree (tenant_id, username) [UNIQUE]`

## 认证与安全

### console_routes

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| route_key | character varying(64) | 否 |  |  |
| path | character varying(255) | 否 |  |  |
| name | character varying(128) | 否 |  |  |
| component_key | character varying(128) | 否 |  |  |
| redirect | character varying(255) | 是 | ''::character varying |  |
| parent_key | character varying(64) | 是 | ''::character varying |  |
| sort_order | bigint | 是 | 1000 |  |
| hidden | boolean | 是 | false |  |
| public | boolean | 是 | false |  |
| enabled | boolean | 是 | true |  |
| permissions_json | jsonb | 是 |  |  |
| roles_json | jsonb | 是 |  |  |
| meta_json | jsonb | 是 |  |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (route_key)`

**索引**

- `btree (enabled)`
- `btree (name) [UNIQUE]`
- `btree (parent_key)`
- `btree (path) [UNIQUE]`
- `btree (sort_order)`

### console_sessions

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| session_id | character varying(64) | 否 |  |  |
| username | character varying(128) | 否 |  |  |
| issued_at | timestamp(3) with time zone | 否 |  |  |
| expires_at | timestamp(3) with time zone | 否 |  |  |
| revoked_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| last_seen_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| client_ip_hash | character varying(64) | 是 | ''::character varying |  |
| user_agent_hash | character varying(64) | 是 | ''::character varying |  |
| user_agent_preview | character varying(255) | 是 | ''::character varying |  |
| created_at | timestamp(3) with time zone | 否 |  |  |

**键与约束**

- `PRIMARY KEY (session_id)`

**索引**

- `btree (expires_at)`
- `btree (issued_at)`
- `btree (revoked_at)`
- `btree (username)`

### oauth_bindings

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| user_id | bigint | 否 |  |  |
| provider | character varying(50) | 否 |  |  |
| provider_user_id | character varying(100) | 否 |  |  |
| access_token | character varying(255) | 是 | ''::character varying |  |
| refresh_token | character varying(255) | 是 | ''::character varying |  |
| expires_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (user_id)`
- `btree (provider, provider_user_id) [UNIQUE]`
- `btree (user_id, provider) [UNIQUE]`

### password_history

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| user_id | bigint | 否 |  |  |
| password_hash | character varying(255) | 否 |  |  |
| changed_at | timestamp(3) with time zone | 否 |  |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (user_id, changed_at)`
- `btree (user_id)`

### totp_recovery_codes

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| user_id | bigint | 否 |  |  |
| code_hash | character varying(255) | 否 |  |  |
| used_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`
- `PRIMARY KEY (id)`

**索引**

- `btree (user_id)`
- `btree (user_id, used_at)`

## 多租户

### tenant_packages

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| name | character varying(128) | 否 |  |  |
| permission_codes | jsonb | 否 | '[]'::jsonb |  |
| status | smallint | 否 | 1 |  |
| remark | character varying(255) | 否 | ''::character varying |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (name) [UNIQUE]`

### tenants

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| code | character varying(64) | 否 |  |  |
| name | character varying(128) | 否 |  |  |
| status | smallint | 否 | 1 |  |
| plan | character varying(64) | 否 | 'free'::character varying |  |
| max_users | bigint | 否 | 0 |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| package_id | bigint | 是 |  |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (code) [UNIQUE]`
- `btree (package_id)`

## 系统运营

### dict_items

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| dict_type_id | bigint | 否 |  |  |
| label | character varying(100) | 否 |  |  |
| value | character varying(100) | 否 |  |  |
| sort | bigint | 是 | 0 |  |
| status | smallint | 是 | 1 |  |
| remark | character varying(255) | 是 | ''::character varying |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (dict_type_id)`

### dict_types

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| name | character varying(100) | 否 |  |  |
| code | character varying(100) | 否 |  |  |
| description | character varying(255) | 是 | ''::character varying |  |
| status | smallint | 是 | 1 |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (code) [UNIQUE]`

### error_codes

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| code | character varying(128) | 否 |  |  |
| message | character varying(512) | 否 |  |  |
| memo | character varying(255) | 是 | ''::character varying |  |
| scope | character varying(64) | 否 | 'global'::character varying |  |
| status | smallint | 是 | 1 |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (code) [UNIQUE]`

### notices

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| title | character varying(200) | 否 |  |  |
| content | text | 否 |  |  |
| type | smallint | 是 | 1 |  |
| status | smallint | 是 | 1 |  |
| creator_id | bigint | 是 |  |  |
| creator | character varying(50) | 是 | ''::character varying |  |
| start_time | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| end_time | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| tenant_id | bigint | 否 | 1 |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (creator_id)`
- `btree (tenant_id)`

### scheduled_job_logs

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| job_id | bigint | 否 |  |  |
| job_name | character varying(100) | 否 |  |  |
| status | smallint | 是 | 1 |  |
| message | text | 是 |  |  |
| duration | bigint | 是 | 0 |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (created_at)`
- `btree (job_id, created_at DESC)`

### scheduled_jobs

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| name | character varying(100) | 否 |  |  |
| group_name | character varying(50) | 是 | 'default'::character varying |  |
| cron_expression | character varying(50) | 否 |  |  |
| invoke_target | character varying(255) | 否 |  |  |
| description | character varying(500) | 是 | ''::character varying |  |
| status | smallint | 是 | 1 |  |
| concurrent | smallint | 是 | 0 |  |
| last_run_time | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| next_run_time | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (name) [UNIQUE]`

### sms_channels

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| tenant_id | bigint | 否 | 1 |  |
| name | character varying(100) | 否 |  |  |
| provider | character varying(32) | 否 |  |  |
| config | json | 是 |  |  |
| status | smallint | 否 | 1 |  |
| remark | character varying(255) | 否 | ''::character varying |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (tenant_id)`

### sms_logs

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| tenant_id | bigint | 否 | 1 |  |
| mobile | character varying(32) | 否 | ''::character varying |  |
| template_code | character varying(100) | 否 | ''::character varying |  |
| content | text | 否 | ''::text |  |
| params | json | 是 |  |  |
| channel_id | bigint | 否 | 0 |  |
| channel_name | character varying(100) | 否 | ''::character varying |  |
| provider | character varying(32) | 否 | ''::character varying |  |
| status | character varying(16) | 否 | 'sending'::character varying |  |
| provider_msg_id | character varying(128) | 否 | ''::character varying |  |
| error | character varying(512) | 否 | ''::character varying |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (mobile)`
- `btree (status)`
- `btree (template_code)`
- `btree (tenant_id)`

### sms_templates

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| tenant_id | bigint | 否 | 1 |  |
| code | character varying(100) | 否 |  |  |
| name | character varying(100) | 否 |  |  |
| channel_id | bigint | 否 | 0 |  |
| content | text | 否 | ''::text |  |
| type | smallint | 否 | 1 |  |
| provider_template_id | character varying(100) | 否 | ''::character varying |  |
| status | smallint | 否 | 1 |  |
| remark | character varying(255) | 否 | ''::character varying |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (channel_id)`
- `btree (tenant_id, code) [UNIQUE]`

### system_settings

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| setting_key | character varying(128) | 否 |  |  |
| value_json | jsonb | 是 |  |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |

**键与约束**

- `PRIMARY KEY (setting_key)`

**索引**

- `btree (updated_at)`

## 审计日志

### audit_logs

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| actor_type | character varying(64) | 是 | 'operator'::character varying |  |
| actor_id | character varying(128) | 是 | 'web-console'::character varying |  |
| action | character varying(128) | 否 |  |  |
| target_type | character varying(64) | 否 |  |  |
| target_id | character varying(128) | 否 |  |  |
| before_json | jsonb | 是 |  |  |
| after_json | jsonb | 是 |  |  |
| summary | text | 是 |  |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| tenant_id | bigint | 否 | 1 |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (action)`
- `btree (created_at)`
- `btree (target_type, target_id, created_at DESC)`
- `btree (target_id)`
- `btree (tenant_id, created_at DESC)`

### login_logs

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| user_id | bigint | 是 |  |  |
| username | character varying(50) | 是 | ''::character varying |  |
| login_type | smallint | 是 | 1 |  |
| status | smallint | 是 | 1 |  |
| ip | character varying(45) | 是 | ''::character varying |  |
| location | character varying(100) | 是 | ''::character varying |  |
| device | character varying(100) | 是 | ''::character varying |  |
| os | character varying(50) | 是 | ''::character varying |  |
| browser | character varying(100) | 是 | ''::character varying |  |
| user_agent | character varying(500) | 是 | ''::character varying |  |
| message | character varying(255) | 是 | ''::character varying |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| tenant_id | bigint | 否 | 1 |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (created_at)`
- `btree (ip, created_at) WHERE (status = 0)`
- `btree (username, created_at) WHERE (status = 0)`
- `btree (tenant_id, created_at DESC)`
- `btree (user_id, created_at DESC)`

### operation_logs

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| user_id | bigint | 是 |  |  |
| username | character varying(50) | 是 | ''::character varying |  |
| actor_type | character varying(64) | 是 | 'operator'::character varying |  |
| actor_id | character varying(128) | 是 | 'web-console'::character varying |  |
| request_id | character varying(64) | 是 | ''::character varying |  |
| module | character varying(50) | 是 | ''::character varying |  |
| action | character varying(50) | 是 | ''::character varying |  |
| method | character varying(10) | 是 | ''::character varying |  |
| path | character varying(255) | 是 | ''::character varying |  |
| query | character varying(1024) | 是 | ''::character varying |  |
| request_body | text | 是 |  |  |
| response_body | text | 是 |  |  |
| status | bigint | 是 | 0 |  |
| ip | character varying(45) | 是 | ''::character varying |  |
| user_agent | character varying(500) | 是 | ''::character varying |  |
| latency | bigint | 是 | 0 |  |
| error_msg | character varying(1024) | 是 | ''::character varying |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| tenant_id | bigint | 否 | 1 |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (created_at)`
- `btree (request_id)`
- `btree (tenant_id, created_at DESC)`
- `btree (user_id, created_at DESC)`

## 文件服务

### files

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | IDENTITY |  |
| user_id | bigint | 是 |  |  |
| file_name | character varying(255) | 否 |  |  |
| file_path | character varying(500) | 否 |  |  |
| file_size | bigint | 否 | 0 |  |
| file_type | character varying(50) | 是 | ''::character varying |  |
| mime_type | character varying(100) | 是 | ''::character varying |  |
| extension | character varying(20) | 是 | ''::character varying |  |
| storage_type | character varying(20) | 是 | 'local'::character varying |  |
| url | character varying(500) | 是 | ''::character varying |  |
| hash | character varying(64) | 是 | ''::character varying |  |
| created_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| updated_at | timestamp(3) with time zone | 是 | NULL::timestamp with time zone |  |
| image_width | integer | 否 | 0 |  |
| image_height | integer | 否 | 0 |  |
| thumbnail_path | character varying(500) | 是 | ''::character varying |  |
| thumbnail_url | character varying(500) | 是 | ''::character varying |  |
| thumbnail_width | integer | 否 | 0 |  |
| thumbnail_height | integer | 否 | 0 |  |
| tenant_id | bigint | 否 | 1 |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (hash)`
- `btree (tenant_id, created_at DESC)`
- `btree (tenant_id, user_id)`
- `btree (user_id)`

## 审批流（BPM）

### bpm_cc_record

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | nextval('bpm_cc_record_id_seq'::regclass) |  |
| tenant_id | bigint | 否 | 1 |  |
| instance_id | bigint | 否 |  |  |
| node_id | character varying(64) | 否 |  |  |
| user_id | bigint | 否 |  |  |
| read_at | timestamp with time zone | 是 |  |  |
| created_at | timestamp with time zone | 是 |  |  |
| node_name | character varying(128) | 否 | ''::character varying |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (instance_id)`
- `btree (tenant_id, user_id)`

### bpm_process_definition

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | nextval('bpm_process_definition_id_seq'::regclass) |  |
| tenant_id | bigint | 否 | 1 |  |
| key | character varying(64) | 否 |  |  |
| version | bigint | 否 | 1 |  |
| name | character varying(128) | 否 |  |  |
| status | character varying(16) | 否 | 'draft'::character varying |  |
| node_tree | jsonb | 否 |  |  |
| form_schema | jsonb | 是 |  |  |
| biz_type | character varying(32) | 是 |  |  |
| remark | character varying(256) | 是 |  |  |
| created_by | bigint | 否 | 0 |  |
| created_at | timestamp with time zone | 是 |  |  |
| updated_at | timestamp with time zone | 是 |  |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (biz_type)`
- `btree (status)`
- `btree (tenant_id)`
- `btree (tenant_id, key, version) [UNIQUE]`

### bpm_process_instance

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | nextval('bpm_process_instance_id_seq'::regclass) |  |
| tenant_id | bigint | 否 | 1 |  |
| definition_id | bigint | 否 |  |  |
| definition_key | character varying(64) | 否 |  |  |
| title | character varying(256) | 否 |  |  |
| biz_type | character varying(32) | 否 |  |  |
| biz_id | character varying(64) | 否 |  |  |
| status | character varying(16) | 否 | 'running'::character varying |  |
| current_node_id | character varying(64) | 是 |  |  |
| form_snapshot | jsonb | 否 |  |  |
| variables | jsonb | 是 |  |  |
| initiator_id | bigint | 否 |  |  |
| initiator_dept | bigint | 否 | 0 |  |
| finished_at | timestamp with time zone | 是 |  |  |
| created_at | timestamp with time zone | 是 |  |  |
| updated_at | timestamp with time zone | 是 |  |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (definition_key)`
- `btree (tenant_id, initiator_id, status)`
- `btree (tenant_id, status)`
- `btree (tenant_id, biz_type, biz_id) WHERE ((status)::text = 'running'::text) [UNIQUE]`

### bpm_process_log

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | nextval('bpm_process_log_id_seq'::regclass) |  |
| tenant_id | bigint | 否 | 1 |  |
| instance_id | bigint | 否 |  |  |
| node_id | character varying(64) | 是 |  |  |
| task_id | bigint | 否 | 0 |  |
| action | character varying(32) | 否 |  |  |
| operator_id | bigint | 否 | 0 |  |
| detail | jsonb | 是 |  |  |
| created_at | timestamp with time zone | 是 |  |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (tenant_id)`
- `btree (instance_id, created_at)`

### bpm_task

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | bigint | 否 | nextval('bpm_task_id_seq'::regclass) |  |
| tenant_id | bigint | 否 | 1 |  |
| instance_id | bigint | 否 |  |  |
| node_id | character varying(64) | 否 |  |  |
| node_name | character varying(128) | 否 |  |  |
| round | bigint | 否 | 1 |  |
| assignee_id | bigint | 否 |  |  |
| origin_assignee | bigint | 否 | 0 |  |
| multi_mode | character varying(8) | 否 | 'OR'::character varying |  |
| seq_order | bigint | 否 | 0 |  |
| status | character varying(16) | 否 | 'pending'::character varying |  |
| comment | character varying(512) | 是 |  |  |
| timeout_at | timestamp with time zone | 是 |  |  |
| reminded_at | timestamp with time zone | 是 |  |  |
| acted_at | timestamp with time zone | 是 |  |  |
| created_at | timestamp with time zone | 是 |  |  |
| updated_at | timestamp with time zone | 是 |  |  |

**键与约束**

- `PRIMARY KEY (id)`

**索引**

- `btree (origin_assignee)`
- `btree (instance_id, node_id, round)`
- `btree (tenant_id, assignee_id, status)`

## 迁移框架

### goose_db_version

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| id | integer | 否 | IDENTITY |  |
| version_id | bigint | 否 |  |  |
| is_applied | boolean | 否 |  |  |
| tstamp | timestamp without time zone | 否 | now() |  |

**键与约束**

- `PRIMARY KEY (id)`

### schema_migrations

| 列 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| version | character varying(255) | 否 |  |  |
| checksum | character varying(64) | 是 | ''::character varying |  |
| applied_at | timestamp with time zone | 否 | CURRENT_TIMESTAMP |  |

**键与约束**

- `PRIMARY KEY (version)`

