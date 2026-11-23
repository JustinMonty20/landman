-- +goose Up
CREATE INDEX vacancy_live_structures_generation_pin_idx ON vacancy_live_structures(generation_id,pin);
CREATE INDEX vacancy_live_assessments_generation_parcel_av_idx ON vacancy_live_assessments(generation_id,parcel_number,parcel_year,assess_category);
-- +goose Down
DROP INDEX vacancy_live_assessments_generation_parcel_av_idx;
DROP INDEX vacancy_live_structures_generation_pin_idx;
