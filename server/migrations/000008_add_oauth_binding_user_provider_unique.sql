-- +goose Up
-- Delete older duplicate rows before adding the user/provider unique key.
DELETE ob FROM `oauth_bindings` ob
JOIN `oauth_bindings` newer
  ON newer.`user_id` = ob.`user_id`
 AND newer.`provider` = ob.`provider`
 AND newer.`id` > ob.`id`;

ALTER TABLE `oauth_bindings`
  ADD UNIQUE KEY `uk_oauth_bindings_user_provider` (`user_id`,`provider`);

-- +goose Down
DROP INDEX `uk_oauth_bindings_user_provider` ON `oauth_bindings`;
