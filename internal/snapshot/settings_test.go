package snapshot_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

func uses(s snapshot.Tree) map[string]snapshot.SettingUse {
	found := map[string]snapshot.SettingUse{}
	for _, use := range s.Uses {
		found[use.Name] = use
	}
	return found
}

func TestBuildFindsWhichFilesHoldASettingName(t *testing.T) {
	files := map[string]string{
		".env.example":    "PORT=1\nSESSION_GRACE_MS=2\nLOG_LEVEL=info\nA_OIDC_ISSUER=x\nBFF_TEST_TOKEN=y\nONLY_SCRIPT=z\nAPP_BATCH_SIZE=1\nAPP_TIMEOUT=2\nAPP_MODE=3\nJOB_RETRIES=4\nAPP_=5\n",
		"src/a.ts":        "process.env.PORT; const EXPORT_PORT = 1; TRANSPORT; cfg.sessionGraceMs; required(env, \"OIDC_ISSUER\"); TOKEN\n",
		"tests/a.test.ts": "log_level; BFF_TEST_TOKEN", // a name at the very end, with no line end
		"scripts/x.ts":    "ONLY_SCRIPT\n",
		"src/config.py":   "class S(BaseSettings):\n    model_config = SettingsConfigDict(env_prefix=\"APP_\")\n    batch_size: int = 1\n    MODE: str = \"x\"\n",
		"src/other.py":    "timeout = 3\n",
		"src/jobs.py":     "env_prefix = 'job_'\nretries = 1\n",
	}
	s := snapshot.Build(snapshottest.Files(files, files), conventions.Default())
	want := map[string]snapshot.SettingUse{
		"PORT":             {Name: "PORT", Product: []string{"src/a.ts"}},
		"SESSION_GRACE_MS": {Name: "SESSION_GRACE_MS", Product: []string{"src/a.ts"}},
		"LOG_LEVEL":        {Name: "LOG_LEVEL", Test: []string{"tests/a.test.ts"}},
		"A_OIDC_ISSUER":    {Name: "A_OIDC_ISSUER", Rest: "OIDC_ISSUER"},
		"BFF_TEST_TOKEN":   {Name: "BFF_TEST_TOKEN", Test: []string{"tests/a.test.ts"}},
		"ONLY_SCRIPT":      {Name: "ONLY_SCRIPT"},
		"APP_BATCH_SIZE":   {Name: "APP_BATCH_SIZE", Prefixed: []string{"src/config.py"}},
		"APP_TIMEOUT":      {Name: "APP_TIMEOUT", Rest: "TIMEOUT"},
		"APP_MODE":         {Name: "APP_MODE", Rest: "MODE"},
		"JOB_RETRIES":      {Name: "JOB_RETRIES", Prefixed: []string{"src/jobs.py"}},
		"APP_":             {Name: "APP_", Product: []string{"src/config.py"}},
	}
	for _, side := range []snapshot.Tree{s.Now, s.Before} {
		if got := uses(side); !reflect.DeepEqual(got, want) {
			t.Errorf("Build.Uses =\n%+v\nwant\n%+v", got, want)
		}
	}
	files["src/a.ts"] = strings.Replace(files["src/a.ts"], "process.env.PORT; ", "", 1)
	if got := uses(snapshot.Build(snapshottest.Files(nil, files), conventions.Default()).Now)["PORT"]; len(got.Product) != 0 {
		t.Errorf("PORT inside EXPORT_PORT and TRANSPORT counted as a read: %+v", got)
	}
}

func TestBuildKeepsCommentedOutNamesApart(t *testing.T) {
	s := snapshot.Build(snapshottest.Files(nil, map[string]string{".env.example": "# LOG_LEVEL=debug\nLOG_LEVEL=info\nPORT=1\n"}), conventions.Default())
	if got := s.Now.Settings; !reflect.DeepEqual(got.Names, []string{"LOG_LEVEL", "LOG_LEVEL", "PORT"}) || !reflect.DeepEqual(got.Active, []string{"LOG_LEVEL", "PORT"}) {
		t.Errorf("Build.Settings = %+v, want names with the commented one and active without it", got)
	}
}

func TestBuildCountsDynamicReadsPerLanguage(t *testing.T) {
	files := map[string]string{
		"src/a.ts":             "process.env[key]\nprocess.env[key2]\nprocess.env[\"X\"]\n",
		"cmd/main.go":          "x := os.Getenv(name)\n",
		"tests/a.test.ts":      "process.env[key]\n",
		"bin/run.sh":           "process.env[key]\n",
		"src/b.py":             "os.environ[key]\n",
		"src/main/java/A.java": "System.getenv(name);\nSystem.getenv(\"X\");\n",
		"src/main/kotlin/B.kt": "System.getenv(key)\n",
		"src/config.rs":        "env::var(name);\nenv::var(\"X\");\n",
	}
	got := snapshot.Build(snapshottest.Files(nil, files), conventions.Default()).Now.Dynamic
	if want := []snapshot.DynamicRead{{Path: "cmd/main.go", Count: 1}, {Path: "src/a.ts", Count: 2}, {Path: "src/b.py", Count: 1}, {Path: "src/config.rs", Count: 1}, {Path: "src/main/java/A.java", Count: 1}, {Path: "src/main/kotlin/B.kt", Count: 1}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Build.Dynamic = %+v, want %+v", got, want)
	}
}

func TestBuildReadsComposeEnvironmentKeys(t *testing.T) {
	compose := strings.ReplaceAll(`services:
  api:
    environment:
      - PORT=8080
      - "REDIS_URL=redis://cache:6379"
      - DEBUG
      - <<: *defaults
    env_file: .env
  db:
    environment:
      POSTGRES_DB: app
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      POSTGRES_USER: "$DB_USER"
      LOG_LEVEL:
  cache:
    environment: {A: 1}
`, "\n", "\r\n")
	files := map[string]string{"docker-compose.yml": compose, "deploy/compose.prod.yaml": "x:\n  environment:\n    - OTHER=1\n",
		"node_modules/x/compose.yml": compose, "compose.txt": compose}
	s := snapshot.Build(snapshottest.Files(nil, files), conventions.Default())
	want := []snapshot.ComposeFile{
		{Path: "deploy/compose.prod.yaml"},
		{Path: "docker-compose.yml", Keys: []string{"DEBUG", "LOG_LEVEL", "POSTGRES_PASSWORD", "POSTGRES_USER"}, Unread: 3},
	}
	if !reflect.DeepEqual(s.Now.Compose, want) {
		t.Errorf("Build.Compose = %+v, want %+v", s.Now.Compose, want)
	}
	if printed := fmt.Sprintf("%+v", s); strings.Contains(printed, "8080") || strings.Contains(printed, "redis://") || strings.Contains(printed, "DB_PASSWORD") || strings.Contains(printed, "DB_USER") {
		t.Errorf("Build holds a compose value: %s", printed)
	}
}

func TestFormsAreAsWrittenLowerAndCamel(t *testing.T) {
	for name, want := range map[string][]string{"SESSION_GRACE_MS": {"SESSION_GRACE_MS", "session_grace_ms", "sessionGraceMs"}, "PORT": {"PORT", "port"}} {
		if got := snapshot.Forms(name); !reflect.DeepEqual(got, want) {
			t.Errorf("Forms(%s) = %q, want %q", name, got, want)
		}
	}
}
