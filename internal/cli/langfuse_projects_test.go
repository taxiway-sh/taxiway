package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLangfuseFastHashMatchesLangfuseAlgorithm(t *testing.T) {
	hash := langfuseFastHash("sk-lf-test-secret", "langfuse-salt")

	assert.Equal(t, "ccd10c011cb521e143e47281d1c86daceb4b2dbc0afa19cb0005fb4eb5b0fd3f", hash)
}

func TestLangfuseDisplaySecretKeyMasksSecret(t *testing.T) {
	assert.Equal(t, "sk-lf-...cdef", langfuseDisplaySecretKey("sk-lf-1234567890abcdef"))
}

func TestLangfuseProjectSQLCreatesProjectAndProjectKey(t *testing.T) {
	sql, err := langfuseProjectProvisionSQL(langfuseProjectProvision{
		OrgID:           "org-1",
		ProjectID:       "project-1",
		ProjectName:     "test-codex",
		APIKeyID:        "api-key-1",
		PublicKey:       "pk-lf-lab",
		HashedSecretKey: "$2a$11$hash",
		FastHash:        "fast-hash",
		DisplaySecret:   "sk-lf-...abcd",
	})

	require.NoError(t, err)
	assert.Contains(t, sql, `insert into projects (id, org_id, name, metadata)`)
	assert.Contains(t, sql, `'project-1', 'org-1', 'test-codex', '{"taxiway_lab":"test-codex"}'::jsonb`)
	assert.Contains(t, sql, `insert into api_keys`)
	assert.Contains(t, sql, `'api-key-1',`)
	assert.Contains(t, sql, `'pk-lf-lab',`)
	assert.Contains(t, sql, `'$2a$11$hash',`)
	assert.Contains(t, sql, `'fast-hash',`)
	assert.Contains(t, sql, `'sk-lf-...abcd',`)
	assert.Contains(t, sql, `on conflict (public_key) do update`)
}

func TestLangfuseProjectSQLRejectsMissingRequiredValues(t *testing.T) {
	_, err := langfuseProjectProvisionSQL(langfuseProjectProvision{
		OrgID:       "org-1",
		ProjectName: "test-codex",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "project id")
}

func TestLangfuseProjectRemovalSQLHardDeletesProject(t *testing.T) {
	sql, err := langfuseProjectRemovalSQL("test-codex")

	require.NoError(t, err)
	assert.Contains(t, sql, `with target_project as (`)
	assert.Contains(t, sql, `metadata->>'taxiway_lab' = 'test-codex'`)
	assert.NotContains(t, sql, `deleted_at is null`)
	assert.NotContains(t, sql, `name = 'test-codex'`)
	assert.Contains(t, sql, `delete from projects`)
	assert.Contains(t, sql, `returning id`)
	assert.NotContains(t, sql, `set deleted_at = now()`)
}

func TestLangfuseClickHouseRemovalSQLDeletesKnownProjectTables(t *testing.T) {
	sql, err := langfuseClickHouseProjectRemovalSQL([]string{"project-1", "project-2"})

	require.NoError(t, err)
	assert.Contains(t, sql, `ALTER TABLE traces DELETE WHERE project_id IN ('project-1', 'project-2')`)
	assert.Contains(t, sql, `ALTER TABLE observations DELETE WHERE project_id IN ('project-1', 'project-2')`)
	assert.Contains(t, sql, `ALTER TABLE project_environments DELETE WHERE project_id IN ('project-1', 'project-2')`)
	assert.NotContains(t, sql, `analytics_traces`)
}

func TestLangfuseProjectRemovalSQLRejectsMissingLabName(t *testing.T) {
	_, err := langfuseProjectRemovalSQL("")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "lab name")
}
