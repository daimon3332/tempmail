package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdvancedEnvironmentValidation(t *testing.T) {
	for _, tc := range []struct {
		key, value string
		valid      bool
	}{
		{"TG_POLLING", "true", true}, {"TG_POLLING", "false", true}, {"TG_POLLING", "yes", false},
		{"TG_MAX_ADDRESS", "12", true}, {"TG_MAX_ADDRESS", "1.5", false}, {"TG_MAX_ADDRESS", "-1", false},
		{"TG_MAX_ADDRESS", "1e3", false}, {"TG_MAX_ADDRESS", "99999999999999999999", false},
		{"S3_URL_EXPIRES", "604801", false}, {"S3_URL_EXPIRES", "", true},
		{"SMTP_CONFIG", `{"*":{"host":"smtp.example.com","port":587,"startTls":true}}`, true},
		{"SMTP_CONFIG", `{"*":{"host":"smtp.example.com","port":70000}}`, false},
		{"SMTP_CONFIG", `{"*":{"host":"smtp.example.com","port":"587"}}`, false},
		{"SMTP_CONFIG", `{"*":{"host":"smtp.example.com","port":587,"secure":"false"}}`, false},
		{"SMTP_CONFIG", `[]`, false}, {"SMTP_CONFIG", `null`, false},
		{"S3_ENDPOINT", "javascript:alert(1)", false}, {"S3_ENDPOINT", "https://storage.example.com", true},
		{"INGEST_TOKEN", "secret\nJWT_SECRET=changed", false},
		{"DOMAINS", "ignored-by-design", true}, {"JWT_SECRET", "changed", false}, {"ORIGIN_TOKEN", "old", false},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			if err := validateEnvironment(map[string]string{tc.key: tc.value}); (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestEnvironmentSyncUsesDedicatedFieldsAndPreservesDeployment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	initial := "# deployment\nJWT_SECRET='do-not-change'\nDB_PATH=/data/tempmail.db\nORIGIN_TOKEN=obsolete\nINGEST_TOKEN='old'\nDOMAINS=['old.example']\n"
	if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}
	rc := runtimeConfig{Domains: []string{"example.com"}, DefaultDomains: []string{"example.com"}, Title: "New title", Environment: map[string]string{"DOMAINS": "attacker.example", "INGEST_TOKEN": `secret$foo"\value`}}
	if err := syncRuntimeEnv(path, rc); err != nil {
		t.Fatal(err)
	}
	got := readEnvValues(path)
	if got["DOMAINS"] != `["example.com"]` || got["TITLE"] != "New title" || got["JWT_SECRET"] != "do-not-change" || got["DB_PATH"] != "/data/tempmail.db" {
		t.Fatalf("canonical or deployment values changed incorrectly")
	}
	if _, ok := got["ORIGIN_TOKEN"]; ok {
		t.Fatal("obsolete token remains")
	}
	if got["INGEST_TOKEN"] != rc.Environment["INGEST_TOKEN"] {
		t.Fatal("secret did not round-trip")
	}
	for _, value := range []string{`a'$var"\tail`, "line1\nline2", `{"*":{"host":"smtp.example.com","port":587,"password":"$abc"}}`} {
		content := []byte("VALUE=" + envValue(value, "VALUE") + "\n")
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
		if readEnvValues(path)["VALUE"] != value {
			t.Fatalf("value failed round-trip")
		}
	}
}

func TestRuntimeConfigRejectsInvalidEnvironmentBeforeSaving(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ctx := context.Background()
	rc := a.effective(ctx)
	rc.Environment = map[string]string{"TG_MAX_ADDRESS": "not-a-number"}
	data, _ := json.Marshal(rc)
	w := httptest.NewRecorder()
	a.adminSaveRuntimeConfig(w, httptest.NewRequest(http.MethodPost, "/admin/runtime_config", strings.NewReader(string(data))))
	if w.Code != 400 {
		t.Fatalf("status=%d", w.Code)
	}
	if _, exists := a.readRuntime(ctx); exists {
		t.Fatal("invalid config was persisted")
	}
}

func TestRuntimeConfigHidesManagedAndDeploymentValues(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	a.cfg.EnvSyncPath = filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(a.cfg.EnvSyncPath, []byte("DOMAINS=example.com\nJWT_SECRET=private\nADMIN_PASSWORDS=private\nTG_POLLING=true\nINGEST_TOKEN=relay\n"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.adminGetRuntimeConfig(w, httptest.NewRequest(http.MethodGet, "/admin/runtime_config", nil))
	var body struct {
		Environment map[string]string  `json:"environment"`
		Fields      []environmentField `json:"environment_fields"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"DOMAINS", "JWT_SECRET", "ADMIN_PASSWORDS", "ORIGIN_TOKEN"} {
		if _, ok := body.Environment[key]; ok {
			t.Fatalf("exposed %s", key)
		}
	}
	if body.Environment["TG_POLLING"] != "true" || len(body.Fields) == 0 {
		t.Fatal("advanced fields missing")
	}
}
