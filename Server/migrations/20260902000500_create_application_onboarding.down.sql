DROP TABLE IF EXISTS application_onboardings;

ALTER TABLE argocd_application_snapshots
DROP COLUMN IF EXISTS argocd_project;
