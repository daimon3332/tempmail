package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUserSettingsRenewsTokenNearExpiry(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO users(id, user_email, password) VALUES(7, 'u@example.com', 'x')`,
		`INSERT INTO user_roles(user_id, role_text) VALUES(7, 'admin')`,
	} {
		if _, err := a.db.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	settings := func(exp time.Duration) map[string]any {
		t.Helper()
		tok, _ := a.jwt.Sign(map[string]any{"user_email": "u@example.com", "user_id": 7, "exp": time.Now().Add(exp).Unix()})
		req := httptest.NewRequest(http.MethodGet, "/user_api/settings", nil)
		req.Header.Set("x-user-token", tok)
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	fresh := settings(20 * 24 * time.Hour)
	if fresh["new_user_token"] != nil || fresh["access_token"] == nil {
		t.Fatalf("fresh token: want access token only, got %v / %v", fresh["new_user_token"], fresh["access_token"])
	}
	renewed, _ := settings(3 * 24 * time.Hour)["new_user_token"].(string)
	claims, err := a.jwt.Verify(renewed)
	if err != nil {
		t.Fatalf("renewed token invalid: %v", err)
	}
	if exp, _ := claims["exp"].(float64); time.Until(time.Unix(int64(exp), 0)) < 29*24*time.Hour {
		t.Fatalf("renewed token expires too early: %v", claims["exp"])
	}
	req := httptest.NewRequest(http.MethodGet, "/user_api/settings", nil)
	expired, _ := a.jwt.Sign(map[string]any{"user_email": "u@example.com", "user_id": 7, "exp": time.Now().Add(-time.Minute).Unix()})
	req.Header.Set("x-user-token", expired)
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired token status %d, want 401", rec.Code)
	}
}
