-- +goose Up
CREATE TABLE user_parcel_tasks (
  user_id uuid NOT NULL,
  parcel_id uuid NOT NULL,
  id uuid NOT NULL,
  message text NOT NULL CHECK (length(btrim(message)) > 0 AND octet_length(message) <= 640),
  due_at timestamptz NOT NULL,
  timezone text NOT NULL CHECK (length(timezone) BETWEEN 1 AND 100),
  status text NOT NULL CHECK (status IN ('open','completed','canceled')),
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  schedule_version bigint NOT NULL DEFAULT 1 CHECK (schedule_version > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, id),
  UNIQUE (user_id, parcel_id, id),
  FOREIGN KEY (user_id, parcel_id) REFERENCES user_parcels(user_id, parcel_id) ON DELETE CASCADE
);
CREATE INDEX user_parcel_tasks_queue_idx ON user_parcel_tasks(user_id, status, due_at, id);
CREATE INDEX user_parcel_tasks_parcel_idx ON user_parcel_tasks(user_id, parcel_id, due_at, id);
CREATE TABLE user_task_reminders (
  user_id uuid NOT NULL,
  parcel_id uuid NOT NULL,
  task_id uuid NOT NULL,
  version bigint NOT NULL CHECK (version > 0),
  due_at timestamptz NOT NULL,
  state text NOT NULL CHECK (state IN ('queued','stubbed','canceled','missed')),
  changed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, task_id, version),
  FOREIGN KEY (user_id, parcel_id, task_id) REFERENCES user_parcel_tasks(user_id, parcel_id, id) ON DELETE CASCADE
);
CREATE INDEX user_task_reminders_due_idx ON user_task_reminders(due_at, task_id) WHERE state='queued';
-- +goose Down
DROP TABLE user_task_reminders;
DROP TABLE user_parcel_tasks;
