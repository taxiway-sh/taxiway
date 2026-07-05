package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/envfile"
	"golang.org/x/crypto/bcrypt"
)

var langfuseClickHouseProjectTables = []string{
	"blob_storage_file_log",
	"dataset_run_items",
	"dataset_run_items_rmt",
	"event_log",
	"observations",
	"project_environments",
	"scores",
	"traces",
}

type langfuseProjectProvision struct {
	OrgID           string
	ProjectID       string
	ProjectName     string
	APIKeyID        string
	PublicKey       string
	HashedSecretKey string
	FastHash        string
	DisplaySecret   string
}

func ensureLabLangfuseProject(state *RootState, stateDir, observabilityDir string, ref config.LabRef) error {
	_ = observabilityDir
	runtime := state.observabilityRuntime()
	proxy := state.proxyRuntime()
	if !waitForLangfuseViaProxy(proxy, 60*time.Second) {
		return fmt.Errorf("Langfuse is not ready; run `taxiway observe up` first")
	}

	observabilityValues, err := loadEnvValues(observabilityEnvPath(state))
	if err != nil {
		return err
	}
	orgID := observabilityValues["LANGFUSE_INIT_ORG_ID"]
	if orgID == "" {
		return fmt.Errorf("LANGFUSE_INIT_ORG_ID is missing; run `taxiway observe up`")
	}
	salt := observabilityValues["LANGFUSE_SALT"]
	if salt == "" {
		return fmt.Errorf("LANGFUSE_SALT is missing; run `taxiway observe up`")
	}

	values, err := readLabGatewayEnv(stateDir, ref)
	if err != nil {
		return err
	}

	projectID := values[labLangfuseProjectIDEnv]
	if projectID == "" {
		if projectID, err = newUUID(); err != nil {
			return fmt.Errorf("generate Langfuse project id: %w", err)
		}
	}
	publicKey := values[labLangfuseProjectPublicKeyEnv]
	secretKey := values[labLangfuseProjectSecretKeyEnv]
	if publicKey == "" || secretKey == "" {
		if publicKey, err = generatedLangfusePublicKey(); err != nil {
			return err
		}
		if secretKey, err = generatedLangfuseSecretKey(); err != nil {
			return err
		}
	}
	apiKeyID, err := newCUIDLike("taxiway")
	if err != nil {
		return fmt.Errorf("generate Langfuse api key id: %w", err)
	}
	hashedSecretKey, err := langfuseBcryptHash(secretKey)
	if err != nil {
		return err
	}

	sql, err := langfuseProjectProvisionSQL(langfuseProjectProvision{
		OrgID:           orgID,
		ProjectID:       projectID,
		ProjectName:     ref.Lab,
		APIKeyID:        apiKeyID,
		PublicKey:       publicKey,
		HashedSecretKey: hashedSecretKey,
		FastHash:        langfuseFastHash(secretKey, salt),
		DisplaySecret:   langfuseDisplaySecretKey(secretKey),
	})
	if err != nil {
		return err
	}
	projectID, err = runLangfuseProjectProvisionSQL(runtime, sql)
	if err != nil {
		return err
	}

	values[labLangfuseProjectIDEnv] = projectID
	values[labLangfuseProjectNameEnv] = ref.Lab
	values[labLangfuseProjectPublicKeyEnv] = publicKey
	values[labLangfuseProjectSecretKeyEnv] = secretKey
	return writeLabGatewayEnv(stateDir, ref, values)
}

func langfuseProjectProvisionSQL(p langfuseProjectProvision) (string, error) {
	required := []struct {
		name  string
		value string
	}{
		{"org id", p.OrgID},
		{"project id", p.ProjectID},
		{"project name", p.ProjectName},
		{"api key id", p.APIKeyID},
		{"public key", p.PublicKey},
		{"hashed secret key", p.HashedSecretKey},
		{"fast hash", p.FastHash},
		{"display secret", p.DisplaySecret},
	}
	for _, field := range required {
		if field.value == "" {
			return "", fmt.Errorf("Langfuse %s is missing", field.name)
		}
	}

	metadata, err := json.Marshal(map[string]string{"taxiway_lab": p.ProjectName})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`
with existing_project as (
  select id
  from projects
  where org_id = %[1]s and name = %[2]s and deleted_at is null
  order by created_at asc
  limit 1
),
inserted_project as (
  insert into projects (id, org_id, name, metadata)
  select %[3]s, %[1]s, %[2]s, %[4]s::jsonb
  where not exists (select 1 from existing_project)
  returning id
),
selected_project as (
  select id from inserted_project
  union all
  select id from existing_project
  limit 1
),
upserted_api_key as (
  insert into api_keys (
    id,
    public_key,
    hashed_secret_key,
    fast_hashed_secret_key,
    display_secret_key,
    project_id,
    organization_id,
    scope,
    note,
    is_in_app_agent_key
  )
  select
    %[5]s,
    %[6]s,
    %[7]s,
    %[8]s,
    %[9]s,
    id,
    null,
    'PROJECT'::"ApiKeyScope",
    %[10]s,
    false
  from selected_project
  on conflict (public_key) do update set
    hashed_secret_key = excluded.hashed_secret_key,
    fast_hashed_secret_key = excluded.fast_hashed_secret_key,
    display_secret_key = excluded.display_secret_key,
    project_id = excluded.project_id,
    organization_id = null,
    scope = 'PROJECT'::"ApiKeyScope",
    note = excluded.note,
    is_in_app_agent_key = false
  returning project_id
)
select id from selected_project;
`,
		sqlLiteral(p.OrgID),
		sqlLiteral(p.ProjectName),
		sqlLiteral(p.ProjectID),
		sqlLiteral(string(metadata)),
		sqlLiteral(p.APIKeyID),
		sqlLiteral(p.PublicKey),
		sqlLiteral(p.HashedSecretKey),
		sqlLiteral(p.FastHash),
		sqlLiteral(p.DisplaySecret),
		sqlLiteral("Taxiway lab "+p.ProjectName),
	), nil
}

func runLangfuseProjectProvisionSQL(runtime observabilityRuntime, sql string) (string, error) {
	cmd := exec.Command(
		"docker",
		"exec",
		"-i",
		runtime.PostgresContainer(),
		"psql",
		"-v",
		"ON_ERROR_STOP=1",
		"-U",
		"postgres",
		"-d",
		"postgres",
		"-tA",
	)
	cmd.Stdin = strings.NewReader(sql)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("provision Langfuse project in DB: %w\n%s", err, text)
	}
	projectID := strings.TrimSpace(strings.Split(text, "\n")[0])
	if projectID == "" {
		return "", fmt.Errorf("provision Langfuse project in DB: no project id returned")
	}
	return projectID, nil
}

func removeLabLangfuseProject(state *RootState, stateDir string, ref config.LabRef) error {
	values, err := readLabGatewayEnv(stateDir, ref)
	if err != nil {
		return err
	}
	if values[labLangfuseProjectIDEnv] == "" && values[labLangfuseProjectPublicKeyEnv] == "" && values[labLangfuseProjectSecretKeyEnv] == "" {
		return nil
	}
	runtime := state.observabilityRuntime()
	sql, err := langfuseProjectRemovalSQL(ref.Lab)
	if err != nil {
		return err
	}
	projectIDs, err := runLangfuseProjectRemovalSQL(runtime, sql)
	if err != nil {
		return err
	}
	if len(projectIDs) == 0 {
		return nil
	}
	sql, err = langfuseClickHouseProjectRemovalSQL(projectIDs)
	if err != nil {
		return err
	}
	return runLangfuseClickHouseProjectRemovalSQL(runtime, sql)
}

func langfuseProjectRemovalSQL(labName string) (string, error) {
	if labName != "" {
		return fmt.Sprintf(`
with target_project as (
  select id
  from projects
  where metadata->>'taxiway_lab' = %[1]s
)
delete from projects
where id in (select id from target_project)
returning id;
`, sqlLiteral(labName)), nil
	}
	return "", fmt.Errorf("Langfuse project lab name is missing")
}

func runLangfuseProjectRemovalSQL(runtime observabilityRuntime, sql string) ([]string, error) {
	cmd := exec.Command(
		"docker",
		"exec",
		"-i",
		runtime.PostgresContainer(),
		"psql",
		"-v",
		"ON_ERROR_STOP=1",
		"-U",
		"postgres",
		"-d",
		"postgres",
		"-tA",
	)
	cmd.Stdin = strings.NewReader(sql)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return nil, fmt.Errorf("remove Langfuse project in DB: %w\n%s", err, text)
	}
	if text == "" {
		return nil, nil
	}
	var ids []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "DELETE ") {
			ids = append(ids, line)
		}
	}
	return ids, nil
}

func langfuseClickHouseProjectRemovalSQL(projectIDs []string) (string, error) {
	if len(projectIDs) == 0 {
		return "", fmt.Errorf("Langfuse project ids are missing")
	}
	literals := make([]string, 0, len(projectIDs))
	for _, id := range projectIDs {
		if id == "" {
			return "", fmt.Errorf("Langfuse project id is missing")
		}
		literals = append(literals, sqlLiteral(id))
	}
	projectIDList := strings.Join(literals, ", ")

	var b strings.Builder
	for _, table := range langfuseClickHouseProjectTables {
		fmt.Fprintf(&b, "ALTER TABLE %s DELETE WHERE project_id IN (%s);\n", table, projectIDList)
	}
	return b.String(), nil
}

func runLangfuseClickHouseProjectRemovalSQL(runtime observabilityRuntime, sql string) error {
	cmd := exec.Command(
		"docker",
		"exec",
		"-i",
		runtime.ClickHouseContainer(),
		"clickhouse-client",
		"--multiquery",
	)
	cmd.Stdin = strings.NewReader(sql)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return fmt.Errorf("remove Langfuse project in ClickHouse: %w\n%s", err, text)
	}
	return nil
}

func langfuseBcryptHash(secret string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), 11)
	if err != nil {
		return "", fmt.Errorf("hash Langfuse secret key: %w", err)
	}
	return string(hash), nil
}

func langfuseFastHash(secret, salt string) string {
	saltHash := sha256.Sum256([]byte(salt))
	hash := sha256.New()
	hash.Write([]byte(secret))
	hash.Write([]byte(hex.EncodeToString(saltHash[:])))
	return hex.EncodeToString(hash.Sum(nil))
}

func langfuseDisplaySecretKey(secret string) string {
	if len(secret) <= 10 {
		return secret
	}
	return secret[:6] + "..." + secret[len(secret)-4:]
}

func sqlLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func generatedLangfusePublicKey() (string, error) {
	u, err := newUUID()
	return "pk-lf-" + u, err
}

func generatedLangfuseSecretKey() (string, error) {
	u, err := newUUID()
	return "sk-lf-" + u, err
}

func newCUIDLike(prefix string) (string, error) {
	u, err := newUUID()
	if err != nil {
		return "", err
	}
	return prefix + strings.ReplaceAll(u, "-", ""), nil
}

func loadEnvValues(path string) (map[string]string, error) {
	values, err := envfile.Load(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	return values, nil
}

func writeEnvValues(path string, values map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create env directory: %w", err)
	}
	var out strings.Builder
	for _, key := range envfile.SortedKeys(values) {
		if values[key] == "" {
			continue
		}
		fmt.Fprintf(&out, "%s=%s\n", key, values[key])
	}
	return os.WriteFile(path, []byte(out.String()), 0o600)
}
