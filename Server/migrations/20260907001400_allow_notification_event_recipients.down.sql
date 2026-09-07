DROP INDEX IF EXISTS deployment_notifications_recipient_event_idx;

ALTER TABLE deployment_notifications
ADD CONSTRAINT deployment_notifications_event_id_key UNIQUE (event_id);

