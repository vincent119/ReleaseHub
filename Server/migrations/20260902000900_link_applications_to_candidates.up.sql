ALTER TABLE applications
ADD COLUMN candidate_id UUID NULL REFERENCES argocd_application_candidates(id);

CREATE UNIQUE INDEX applications_candidate_id_idx
ON applications (candidate_id)
WHERE candidate_id IS NOT NULL;
