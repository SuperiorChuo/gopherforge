SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

DROP TABLE IF EXISTS `wm_system_setting`;
DROP TABLE IF EXISTS `wm_console_session`;
DROP TABLE IF EXISTS `wm_console_route`;
DROP TABLE IF EXISTS `scheduled_job_logs`;
DROP TABLE IF EXISTS `scheduled_jobs`;
DROP TABLE IF EXISTS `oauth_bindings`;
DROP TABLE IF EXISTS `dict_items`;
DROP TABLE IF EXISTS `dict_types`;
DROP TABLE IF EXISTS `notices`;
DROP TABLE IF EXISTS `login_logs`;
DROP TABLE IF EXISTS `files`;
DROP TABLE IF EXISTS `wm_audit_log`;
DROP TABLE IF EXISTS `operation_logs`;
DROP TABLE IF EXISTS `menu_permissions`;
DROP TABLE IF EXISTS `role_data_scope_departments`;
DROP TABLE IF EXISTS `role_permissions`;
DROP TABLE IF EXISTS `user_roles`;
DROP TABLE IF EXISTS `menus`;
DROP TABLE IF EXISTS `permissions`;
DROP TABLE IF EXISTS `roles`;
DROP TABLE IF EXISTS `users`;
DROP TABLE IF EXISTS `departments`;
DROP TABLE IF EXISTS `schema_migrations`;

SET FOREIGN_KEY_CHECKS = 1;

CREATE TABLE `departments` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL,
  `code` varchar(50) DEFAULT NULL,
  `parent_id` bigint unsigned NOT NULL DEFAULT 0,
  `leader` varchar(50) DEFAULT '',
  `phone` varchar(20) DEFAULT '',
  `email` varchar(100) DEFAULT '',
  `sort` bigint NOT NULL DEFAULT 0,
  `status` tinyint NOT NULL DEFAULT 1,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_departments_code` (`code`),
  KEY `idx_departments_parent_id` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `users` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `username` varchar(50) NOT NULL,
  `password` varchar(255) NOT NULL,
  `nickname` varchar(50) DEFAULT '',
  `email` varchar(100) DEFAULT NULL,
  `phone` varchar(20) DEFAULT NULL,
  `avatar` varchar(255) DEFAULT '',
  `department_id` bigint unsigned NOT NULL DEFAULT 0,
  `must_change_password` tinyint(1) NOT NULL DEFAULT 0,
  `status` tinyint NOT NULL DEFAULT 1,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_users_username` (`username`),
  UNIQUE KEY `idx_users_email` (`email`),
  UNIQUE KEY `idx_users_phone` (`phone`),
  KEY `idx_users_department_id` (`department_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `roles` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(50) NOT NULL,
  `code` varchar(50) NOT NULL,
  `description` varchar(255) DEFAULT '',
  `data_scope` varchar(32) NOT NULL DEFAULT 'self',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_roles_code` (`code`),
  KEY `idx_roles_data_scope` (`data_scope`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `permissions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(50) NOT NULL,
  `code` varchar(100) NOT NULL,
  `type` tinyint NOT NULL,
  `path` varchar(255) DEFAULT '',
  `method` varchar(10) DEFAULT '',
  `parent_id` bigint unsigned NOT NULL DEFAULT 0,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_permissions_code` (`code`),
  KEY `idx_permissions_parent_id` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `menus` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(50) NOT NULL,
  `title` varchar(50) NOT NULL,
  `icon` varchar(100) DEFAULT '',
  `path` varchar(255) DEFAULT '',
  `component` varchar(255) DEFAULT '',
  `parent_id` bigint unsigned NOT NULL DEFAULT 0,
  `sort` bigint NOT NULL DEFAULT 0,
  `status` tinyint NOT NULL DEFAULT 1,
  `hidden` tinyint NOT NULL DEFAULT 0,
  `permission` varchar(100) DEFAULT '',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_menus_parent_id` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `user_roles` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL,
  `role_id` bigint unsigned NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_roles_user_role` (`user_id`,`role_id`),
  KEY `idx_user_roles_role_id` (`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `role_permissions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `role_id` bigint unsigned NOT NULL,
  `permission_id` bigint unsigned NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_role_permissions_role_permission` (`role_id`,`permission_id`),
  KEY `idx_role_permissions_permission_id` (`permission_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `role_data_scope_departments` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `role_id` bigint unsigned NOT NULL,
  `department_id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_role_data_scope_department` (`role_id`,`department_id`),
  KEY `idx_role_data_scope_departments_department_id` (`department_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `menu_permissions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `menu_id` bigint unsigned NOT NULL,
  `permission_id` bigint unsigned NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_menu_permissions_menu_permission` (`menu_id`,`permission_id`),
  KEY `idx_menu_permissions_permission_id` (`permission_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `operation_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned DEFAULT NULL,
  `username` varchar(50) DEFAULT '',
  `actor_type` varchar(64) DEFAULT 'operator',
  `actor_id` varchar(128) DEFAULT 'web-console',
  `request_id` varchar(64) DEFAULT '',
  `module` varchar(50) DEFAULT '',
  `action` varchar(50) DEFAULT '',
  `method` varchar(10) DEFAULT '',
  `path` varchar(255) DEFAULT '',
  `query` varchar(1024) DEFAULT '',
  `request_body` text,
  `response_body` text,
  `status` bigint DEFAULT 0,
  `ip` varchar(45) DEFAULT '',
  `user_agent` varchar(500) DEFAULT '',
  `latency` bigint DEFAULT 0,
  `error_msg` varchar(1024) DEFAULT '',
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_operation_logs_created_at` (`created_at`),
  KEY `idx_operation_logs_user_id` (`user_id`),
  KEY `idx_operation_logs_actor_type` (`actor_type`),
  KEY `idx_operation_logs_actor_id` (`actor_id`),
  KEY `idx_operation_logs_request_id` (`request_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `wm_audit_log` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `actor_type` varchar(64) DEFAULT 'operator',
  `actor_id` varchar(128) DEFAULT 'web-console',
  `action` varchar(128) NOT NULL,
  `target_type` varchar(64) NOT NULL,
  `target_id` varchar(128) NOT NULL,
  `before_json` json DEFAULT NULL,
  `after_json` json DEFAULT NULL,
  `summary` text,
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_wm_audit_log_created_at` (`created_at`),
  KEY `idx_wm_audit_log_actor_type` (`actor_type`),
  KEY `idx_wm_audit_log_actor_id` (`actor_id`),
  KEY `idx_wm_audit_log_action` (`action`),
  KEY `idx_wm_audit_log_target_type` (`target_type`),
  KEY `idx_wm_audit_log_target_id` (`target_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `files` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned DEFAULT NULL,
  `file_name` varchar(255) NOT NULL,
  `file_path` varchar(500) NOT NULL,
  `file_size` bigint NOT NULL DEFAULT 0,
  `file_type` varchar(50) DEFAULT '',
  `mime_type` varchar(100) DEFAULT '',
  `extension` varchar(20) DEFAULT '',
  `storage_type` varchar(20) DEFAULT 'local',
  `url` varchar(500) DEFAULT '',
  `hash` varchar(64) DEFAULT '',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_files_user_id` (`user_id`),
  KEY `idx_files_hash` (`hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `login_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned DEFAULT NULL,
  `username` varchar(50) DEFAULT '',
  `login_type` tinyint DEFAULT 1,
  `status` tinyint DEFAULT 1,
  `ip` varchar(45) DEFAULT '',
  `location` varchar(100) DEFAULT '',
  `device` varchar(100) DEFAULT '',
  `os` varchar(50) DEFAULT '',
  `browser` varchar(100) DEFAULT '',
  `user_agent` varchar(500) DEFAULT '',
  `message` varchar(255) DEFAULT '',
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_login_logs_created_at` (`created_at`),
  KEY `idx_login_logs_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `dict_types` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL,
  `code` varchar(100) NOT NULL,
  `description` varchar(255) DEFAULT '',
  `status` tinyint DEFAULT 1,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_dict_types_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `dict_items` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `dict_type_id` bigint unsigned NOT NULL,
  `label` varchar(100) NOT NULL,
  `value` varchar(100) NOT NULL,
  `sort` bigint DEFAULT 0,
  `status` tinyint DEFAULT 1,
  `remark` varchar(255) DEFAULT '',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_dict_items_dict_type_id` (`dict_type_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `oauth_bindings` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL,
  `provider` varchar(50) NOT NULL,
  `provider_user_id` varchar(100) NOT NULL,
  `access_token` varchar(255) DEFAULT '',
  `refresh_token` varchar(255) DEFAULT '',
  `expires_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_oauth_bindings_user_id` (`user_id`),
  UNIQUE KEY `uk_oauth_bindings_provider_user` (`provider`,`provider_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `scheduled_jobs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL,
  `group_name` varchar(50) DEFAULT 'default',
  `cron_expression` varchar(50) NOT NULL,
  `invoke_target` varchar(255) NOT NULL,
  `description` varchar(500) DEFAULT '',
  `status` tinyint DEFAULT 1,
  `concurrent` tinyint DEFAULT 0,
  `last_run_time` datetime(3) DEFAULT NULL,
  `next_run_time` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_scheduled_jobs_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `scheduled_job_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `job_id` bigint unsigned NOT NULL,
  `job_name` varchar(100) NOT NULL,
  `status` tinyint DEFAULT 1,
  `message` text,
  `duration` bigint DEFAULT 0,
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_scheduled_job_logs_job_id` (`job_id`),
  KEY `idx_scheduled_job_logs_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `notices` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `title` varchar(200) NOT NULL,
  `content` text NOT NULL,
  `type` tinyint DEFAULT 1,
  `status` tinyint DEFAULT 1,
  `creator_id` bigint unsigned DEFAULT NULL,
  `creator` varchar(50) DEFAULT '',
  `start_time` datetime(3) DEFAULT NULL,
  `end_time` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_notices_creator_id` (`creator_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `wm_console_route` (
  `route_key` varchar(64) NOT NULL,
  `path` varchar(255) NOT NULL,
  `name` varchar(128) NOT NULL,
  `component_key` varchar(128) NOT NULL,
  `redirect` varchar(255) DEFAULT '',
  `parent_key` varchar(64) DEFAULT '',
  `sort_order` bigint DEFAULT 1000,
  `hidden` tinyint(1) DEFAULT 0,
  `public` tinyint(1) DEFAULT 0,
  `enabled` tinyint(1) DEFAULT 1,
  `permissions_json` json DEFAULT NULL,
  `roles_json` json DEFAULT NULL,
  `meta_json` json DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`route_key`),
  UNIQUE KEY `idx_wm_console_route_path` (`path`),
  UNIQUE KEY `idx_wm_console_route_name` (`name`),
  KEY `idx_wm_console_route_sort_order` (`sort_order`),
  KEY `idx_wm_console_route_parent_key` (`parent_key`),
  KEY `idx_wm_console_route_enabled` (`enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `wm_console_session` (
  `session_id` varchar(64) NOT NULL,
  `username` varchar(128) NOT NULL,
  `issued_at` datetime(3) NOT NULL,
  `expires_at` datetime(3) NOT NULL,
  `revoked_at` datetime(3) DEFAULT NULL,
  `last_seen_at` datetime(3) DEFAULT NULL,
  `client_ip_hash` varchar(64) DEFAULT '',
  `user_agent_hash` varchar(64) DEFAULT '',
  `user_agent_preview` varchar(255) DEFAULT '',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`session_id`),
  KEY `idx_wm_console_session_username` (`username`),
  KEY `idx_wm_console_session_issued_at` (`issued_at`),
  KEY `idx_wm_console_session_expires_at` (`expires_at`),
  KEY `idx_wm_console_session_revoked_at` (`revoked_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `wm_system_setting` (
  `setting_key` varchar(128) NOT NULL,
  `value_json` json DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`setting_key`),
  KEY `idx_wm_system_setting_updated_at` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `schema_migrations` (
  `version` varchar(255) NOT NULL,
  `checksum` varchar(64) DEFAULT '',
  `applied_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `departments` (`id`,`name`,`code`,`parent_id`,`leader`,`sort`,`status`,`created_at`,`updated_at`) VALUES
(1,'Headquarters','HQ',0,'admin',1,1,NOW(3),NOW(3));

INSERT INTO `users` (`id`,`username`,`password`,`nickname`,`email`,`phone`,`avatar`,`department_id`,`must_change_password`,`status`,`created_at`,`updated_at`) VALUES
(1,'admin','$2a$10$x.kr72XCSvMcgkM0etydcOLlVPxYcjR8bK4sRYdf0VQmMxz8LskBe','管理员','admin@example.com',NULL,'',1,0,1,NOW(3),NOW(3));

INSERT INTO `roles` (`id`,`name`,`code`,`description`,`data_scope`,`created_at`,`updated_at`) VALUES
(1,'超级管理员','super_admin','全部系统权限','all',NOW(3),NOW(3)),
(2,'管理员','admin','系统管理员','department_and_children',NOW(3),NOW(3)),
(3,'普通用户','user','标准用户权限','self',NOW(3),NOW(3));

INSERT INTO `permissions` (`id`,`name`,`code`,`type`,`path`,`method`,`parent_id`,`created_at`,`updated_at`) VALUES
(1,'仪表盘','dashboard.view',1,'/dashboard','GET',0,NOW(3),NOW(3)),
(2,'系统管理','system',1,'/system','',0,NOW(3),NOW(3)),
(3,'用户列表','system:user:list',2,'/api/v1/users','GET',2,NOW(3),NOW(3)),
(4,'用户详情','system:user:detail',2,'/api/v1/users/:id','GET',2,NOW(3),NOW(3)),
(5,'创建用户','system:user:create',2,'/api/v1/users','POST',2,NOW(3),NOW(3)),
(6,'更新用户','system:user:update',2,'/api/v1/users/:id','PUT',2,NOW(3),NOW(3)),
(7,'删除用户','system:user:delete',2,'/api/v1/users/:id','DELETE',2,NOW(3),NOW(3)),
(8,'角色列表','system:role:list',2,'/api/v1/roles','GET',2,NOW(3),NOW(3)),
(9,'创建角色','system:role:create',2,'/api/v1/roles','POST',2,NOW(3),NOW(3)),
(10,'更新角色','system:role:update',2,'/api/v1/roles/:id','PUT',2,NOW(3),NOW(3)),
(11,'删除角色','system:role:delete',2,'/api/v1/roles/:id','DELETE',2,NOW(3),NOW(3)),
(12,'权限列表','system:permission:list',2,'/api/v1/permissions','GET',2,NOW(3),NOW(3)),
(13,'创建权限','system:permission:create',2,'/api/v1/permissions','POST',2,NOW(3),NOW(3)),
(14,'更新权限','system:permission:update',2,'/api/v1/permissions/:id','PUT',2,NOW(3),NOW(3)),
(15,'删除权限','system:permission:delete',2,'/api/v1/permissions/:id','DELETE',2,NOW(3),NOW(3)),
(16,'菜单列表','system:menu:list',2,'/api/v1/menus','GET',2,NOW(3),NOW(3)),
(17,'创建菜单','system:menu:create',2,'/api/v1/menus','POST',2,NOW(3),NOW(3)),
(18,'更新菜单','system:menu:update',2,'/api/v1/menus/:id','PUT',2,NOW(3),NOW(3)),
(19,'删除菜单','system:menu:delete',2,'/api/v1/menus/:id','DELETE',2,NOW(3),NOW(3)),
(20,'部门列表','system:department:list',2,'/api/v1/departments','GET',2,NOW(3),NOW(3)),
(21,'创建部门','system:department:create',2,'/api/v1/departments','POST',2,NOW(3),NOW(3)),
(22,'更新部门','system:department:update',2,'/api/v1/departments/:id','PUT',2,NOW(3),NOW(3)),
(23,'删除部门','system:department:delete',2,'/api/v1/departments/:id','DELETE',2,NOW(3),NOW(3)),
(24,'操作日志','system:log:operation',2,'/api/v1/operation-logs','GET',2,NOW(3),NOW(3)),
(25,'审计日志','system:log:audit',2,'/api/v1/logs/audit','GET',2,NOW(3),NOW(3)),
(26,'登录日志','system:log:login',2,'/api/v1/login-logs','GET',2,NOW(3),NOW(3)),
(27,'在线用户','system:online-user:list',2,'/api/v1/online-users','GET',2,NOW(3),NOW(3)),
(28,'强制下线','system:online-user:kick',2,'/api/v1/online-users/:token_id','DELETE',2,NOW(3),NOW(3)),
(29,'通知公告','system:notice:list',2,'/api/v1/notices','GET',2,NOW(3),NOW(3)),
(30,'创建公告','system:notice:create',2,'/api/v1/notices','POST',2,NOW(3),NOW(3)),
(31,'更新公告','system:notice:update',2,'/api/v1/notices/:id','PUT',2,NOW(3),NOW(3)),
(32,'删除公告','system:notice:delete',2,'/api/v1/notices/:id','DELETE',2,NOW(3),NOW(3)),
(33,'文件列表','system:file:list',2,'/api/v1/files','GET',2,NOW(3),NOW(3)),
(34,'上传文件','system:file:upload',2,'/api/v1/files/upload','POST',2,NOW(3),NOW(3)),
(35,'删除文件','system:file:delete',2,'/api/v1/files/:id','DELETE',2,NOW(3),NOW(3)),
(36,'字典列表','system:dict:list',2,'/api/v1/dict-types','GET',2,NOW(3),NOW(3)),
(37,'创建字典','system:dict:create',2,'/api/v1/dict-types','POST',2,NOW(3),NOW(3)),
(38,'更新字典','system:dict:update',2,'/api/v1/dict-types/:id','PUT',2,NOW(3),NOW(3)),
(39,'删除字典','system:dict:delete',2,'/api/v1/dict-types/:id','DELETE',2,NOW(3),NOW(3)),
(40,'服务器监控','system:monitor:server',2,'/api/v1/monitor/server','GET',2,NOW(3),NOW(3)),
(41,'数据库监控','system:monitor:mysql',2,'/api/v1/monitor/mysql','GET',2,NOW(3),NOW(3)),
(42,'缓存监控','system:monitor:redis',2,'/api/v1/monitor/redis','GET',2,NOW(3),NOW(3)),
(43,'定时任务','system:job:list',2,'/api/v1/monitor/jobs','GET',2,NOW(3),NOW(3)),
(44,'创建任务','system:job:create',2,'/api/v1/monitor/jobs','POST',2,NOW(3),NOW(3)),
(45,'更新任务','system:job:update',2,'/api/v1/monitor/jobs/:id','PUT',2,NOW(3),NOW(3)),
(46,'删除任务','system:job:delete',2,'/api/v1/monitor/jobs/:id','DELETE',2,NOW(3),NOW(3)),
(47,'执行任务','system:job:run',2,'/api/v1/monitor/jobs/:id/run','POST',2,NOW(3),NOW(3)),
(48,'控制台设置读取','settings.read',2,'/api/v1/auth/routes','GET',2,NOW(3),NOW(3)),
(49,'控制台设置写入','settings.write',2,'/api/v1/console-routes','POST',2,NOW(3),NOW(3)),
(50,'权限治理读取','rbac.read',2,'/api/v1/users','GET',2,NOW(3),NOW(3)),
(51,'权限治理写入','rbac.write',2,'/api/v1/users','POST',2,NOW(3),NOW(3)),
(52,'日志读取','logs.read',2,'/api/v1/operation-logs','GET',2,NOW(3),NOW(3));

INSERT INTO `menus` (`id`,`name`,`title`,`icon`,`path`,`component`,`parent_id`,`sort`,`status`,`hidden`,`permission`,`created_at`,`updated_at`) VALUES
(1,'dashboard','仪表盘','dashboard','/dashboard','Layout',0,0,1,0,'',NOW(3),NOW(3)),
(2,'dashboard-index','系统概览','dashboard','/dashboard/index','dashboard/index',1,1,1,0,'dashboard.view',NOW(3),NOW(3)),
(10,'system','系统管理','setting','/system','Layout',0,1,1,0,'',NOW(3),NOW(3)),
(11,'user','用户管理','user','/system/user','system/user/index',10,1,1,0,'system:user:list',NOW(3),NOW(3)),
(12,'role','角色管理','user-safety','/system/role','system/role/index',10,2,1,0,'system:role:list',NOW(3),NOW(3)),
(13,'permission','权限管理','secured','/system/permission','system/permission/index',10,3,1,0,'system:permission:list',NOW(3),NOW(3)),
(14,'menu','菜单管理','menu','/system/menu','system/menu/index',10,4,1,0,'system:menu:list',NOW(3),NOW(3)),
(15,'department','部门管理','root-list','/system/department','system/department/index',10,5,1,0,'system:department:list',NOW(3),NOW(3)),
(16,'file','文件管理','file','/system/file','system/file/index',10,6,1,0,'system:file:list',NOW(3),NOW(3)),
(17,'dict','字典管理','data-base','/system/dict','system/dict/index',10,7,1,0,'system:dict:list',NOW(3),NOW(3)),
(18,'notice','通知公告','notification','/system/notice','system/notice/index',10,8,1,0,'system:notice:list',NOW(3),NOW(3)),
(19,'online-user','在线用户','user-list','/system/online-user','system/online-user/index',10,9,1,0,'system:online-user:list',NOW(3),NOW(3)),
(20,'operation-log','操作日志','time','/system/operation-log','system/operation-log/index',10,10,1,0,'system:log:operation',NOW(3),NOW(3)),
(21,'login-log','登录日志','time','/system/login-log','system/login-log/index',10,11,1,0,'system:log:login',NOW(3),NOW(3)),
(30,'monitor','系统监控','chart-analytics','/monitor','Layout',0,2,1,0,'',NOW(3),NOW(3)),
(31,'monitor-job','定时任务','time','/monitor/job','monitor/job/index',30,1,1,0,'system:job:list',NOW(3),NOW(3)),
(32,'monitor-server','服务器监控','server','/monitor/server','monitor/server/index',30,2,1,0,'system:monitor:server',NOW(3),NOW(3)),
(33,'monitor-mysql','数据库监控','data-base','/monitor/mysql','monitor/mysql/index',30,3,1,0,'system:monitor:mysql',NOW(3),NOW(3)),
(34,'monitor-redis','缓存监控','data','/monitor/redis','monitor/redis/index',30,4,1,0,'system:monitor:redis',NOW(3),NOW(3)),
(40,'profile','个人中心','user-circle','/profile','Layout',0,99,1,1,'',NOW(3),NOW(3)),
(41,'profile-index','个人中心','user','/profile/index','profile/index',40,1,1,0,'',NOW(3),NOW(3));

INSERT INTO `user_roles` (`user_id`,`role_id`) VALUES (1,1);
INSERT INTO `role_permissions` (`role_id`,`permission_id`) SELECT 1, `id` FROM `permissions`;
INSERT INTO `role_permissions` (`role_id`,`permission_id`) SELECT 2, `id` FROM `permissions` WHERE `code` IN ('dashboard.view','system:user:list','system:role:list','system:permission:list','system:menu:list','system:department:list','system:log:operation','system:log:login','system:file:list','system:dict:list','system:notice:list','system:online-user:list','system:monitor:server','system:monitor:mysql','system:monitor:redis','system:job:list','settings.read','rbac.read','logs.read');

INSERT INTO `menu_permissions` (`menu_id`,`permission_id`)
SELECT m.`id`, p.`id`
FROM `menus` m
JOIN `permissions` p ON p.`code` = m.`permission`
WHERE m.`permission` <> '';

INSERT INTO `dict_types` (`id`,`name`,`code`,`description`,`status`,`created_at`,`updated_at`) VALUES
(1,'用户状态','user_status','常用用户状态值',1,NOW(3),NOW(3)),
(2,'公告类型','notice_type','通知公告类型值',1,NOW(3),NOW(3));

INSERT INTO `dict_items` (`dict_type_id`,`label`,`value`,`sort`,`status`,`remark`,`created_at`,`updated_at`) VALUES
(1,'启用','1',1,1,'',NOW(3),NOW(3)),
(1,'禁用','0',2,1,'',NOW(3),NOW(3)),
(2,'通知','1',1,1,'',NOW(3),NOW(3)),
(2,'公告','2',2,1,'',NOW(3),NOW(3));

INSERT INTO `notices` (`id`,`title`,`content`,`type`,`status`,`creator_id`,`creator`,`created_at`,`updated_at`) VALUES
(1,'欢迎使用','后台管理系统数据库已准备就绪。',1,1,1,'admin',NOW(3),NOW(3));

INSERT INTO `wm_console_route` (`route_key`,`path`,`name`,`component_key`,`sort_order`,`enabled`,`permissions_json`,`roles_json`,`meta_json`,`created_at`,`updated_at`) VALUES
('dashboard','/dashboard','Dashboard','DashboardPage',100,1,'["dashboard.view"]','[]','{"title":"仪表盘","navTitle":"仪表盘","groupId":"monitor","icon":"SettingIcon","permissions":["dashboard.view"],"seedVersion":6}',NOW(3),NOW(3)),
('rbac-users','/rbac/users','RbacUsers','RbacGovernancePage',210,1,'["rbac.read","logs.read"]','[]','{"title":"用户管理","navTitle":"用户管理","groupId":"rbac","icon":"SettingIcon","permissions":["rbac.read","logs.read"],"seedVersion":6}',NOW(3),NOW(3)),
('rbac-roles','/rbac/roles','RbacRoles','RbacGovernancePage',220,1,'["rbac.read","logs.read"]','[]','{"title":"角色管理","navTitle":"角色管理","groupId":"rbac","icon":"SettingIcon","permissions":["rbac.read","logs.read"],"seedVersion":6}',NOW(3),NOW(3)),
('rbac-policies','/rbac/policies','RbacPolicies','RbacGovernancePage',230,1,'["rbac.read","logs.read"]','[]','{"title":"权限管理","navTitle":"权限管理","groupId":"rbac","icon":"SettingIcon","permissions":["rbac.read","logs.read"],"seedVersion":6}',NOW(3),NOW(3)),
('rbac-departments','/rbac/departments','RbacDepartments','RbacGovernancePage',240,1,'["rbac.read","logs.read"]','[]','{"title":"部门管理","navTitle":"部门管理","groupId":"rbac","icon":"SettingIcon","permissions":["rbac.read","logs.read"],"seedVersion":6}',NOW(3),NOW(3)),
('audit','/audit','Audit','AuditPage',270,1,'["logs.read"]','[]','{"title":"审计日志","navTitle":"审计日志","groupId":"security","icon":"SettingIcon","permissions":["logs.read"],"seedVersion":6}',NOW(3),NOW(3)),
('security-logins','/security/logins','SecurityLogins','SecurityLoginsPage',280,1,'["logs.read"]','[]','{"title":"登录日志","navTitle":"登录日志","groupId":"security","icon":"SettingIcon","permissions":["logs.read"],"seedVersion":6}',NOW(3),NOW(3)),
('settings-routes','/settings/routes','ConsoleRoutes','ConsoleRoutesPage',315,1,'["settings.write"]','[]','{"title":"路由设置","navTitle":"路由设置","groupId":"settings","icon":"SettingIcon","permissions":["settings.write"],"seedVersion":6}',NOW(3),NOW(3));

SET FOREIGN_KEY_CHECKS = 1;
