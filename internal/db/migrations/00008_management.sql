-- +goose Up
-- Руководящий состав: не входит в «коллектив» при расчёте % опаздывающих (reference/absences/tg_mapping.md).
ALTER TABLE employees ADD COLUMN management boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE employees DROP COLUMN management;
