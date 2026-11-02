-- +goose Up
-- Telegram: сообщения приходят из двух источников — бот (source='bot') и экспорт Telegram Desktop (source='export').
-- В обычной (не супер-) группе нумерация сообщений своя у каждого аккаунта, поэтому message_id бота и экспорта
-- не совпадают и уникальны только внутри источника. Повторы между источниками ищутся по (from_id, sent_at, text).
ALTER TABLE tg_messages ADD COLUMN source text NOT NULL DEFAULT 'bot' CHECK (source IN ('bot','export'));
ALTER TABLE tg_messages DROP CONSTRAINT tg_messages_pkey;
ALTER TABLE tg_messages ADD PRIMARY KEY (chat_id, source, message_id);
CREATE INDEX ON tg_messages (from_id, sent_at);

ALTER TABLE attendance_events ADD COLUMN tg_source text;
CREATE INDEX ON attendance_events (chat_id, tg_source, message_id) WHERE source = 'telegram';

-- ФИО, которым человек подписывается в сообщениях (из seed/tg_aliases.csv) — второй признак после Telegram ID.
ALTER TABLE tg_aliases ADD COLUMN fio text;

-- +goose Down
ALTER TABLE tg_aliases DROP COLUMN fio;
DROP INDEX IF EXISTS attendance_events_chat_id_tg_source_message_id_idx;
ALTER TABLE attendance_events DROP COLUMN tg_source;
DROP INDEX IF EXISTS tg_messages_from_id_sent_at_idx;
ALTER TABLE tg_messages DROP CONSTRAINT tg_messages_pkey;
ALTER TABLE tg_messages ADD PRIMARY KEY (chat_id, message_id);
ALTER TABLE tg_messages DROP COLUMN source;
