DROP INDEX IF EXISTS applications_candidate_id_idx;
ALTER TABLE applications DROP COLUMN IF EXISTS candidate_id;
