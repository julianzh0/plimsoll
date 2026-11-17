package snapshot_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func TestBuildKeepsCommentedOutNamesApart(t *testing.T) {
	s := snapshot.Build(snapshottest.Files(nil, map[string]string{".env.example": "# LOG_LEVEL=debug\nLOG_LEVEL=info\nPORT=1\n"}), conventions.Default(), snapshottest.Readings(t))
	if got := s.Now.Settings; !reflect.DeepEqual(got.Names, []string{"LOG_LEVEL", "LOG_LEVEL", "PORT"}) || !reflect.DeepEqual(got.Active, []string{"LOG_LEVEL", "PORT"}) {
		t.Errorf("Build.Settings = %+v, want names with the commented one and active without it", got)
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
	s := snapshot.Build(snapshottest.Files(nil, files), conventions.Default(), snapshottest.Readings(t))
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
