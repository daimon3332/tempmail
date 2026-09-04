package passkey

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"tempmail/internal/db"
)

// Store persists WebAuthn credentials (user_passkeys) and ceremony sessions.
type Store struct {
	db *db.DB
	wa *webauthn.WebAuthn
}

func New(rpID string, rpOrigins []string, displayName string) (*Store, error) {
	wa, err := webauthn.New(&webauthn.Config{RPID: rpID, RPDisplayName: displayName, RPOrigins: rpOrigins})
	if err != nil {
		return nil, err
	}
	return &Store{wa: wa}, nil
}

func (s *Store) Enabled() bool { return s != nil && s.wa != nil }

type webauthnUser struct {
	id      string
	display string
	creds   []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte          { return []byte(u.id) }
func (u *webauthnUser) WebAuthnName() string        { return u.display }
func (u *webauthnUser) WebAuthnDisplayName() string { return u.display }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.creds
}

func (s *Store) loadUser(ctx context.Context, userID int64, email string) (*webauthnUser, error) {
	creds, err := s.listCredentials(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &webauthnUser{id: strconv.FormatInt(userID, 10), display: email, creds: creds}, nil
}

func (s *Store) listCredentials(ctx context.Context, userID int64) ([]webauthn.Credential, error) {
	rows, err := s.db.Query(ctx, `SELECT passkey FROM user_passkeys WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	out := []webauthn.Credential{}
	for _, r := range rows {
		var c webauthn.Credential
		if json.Unmarshal([]byte(r.Str("passkey")), &c) == nil {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *Store) saveCredential(ctx context.Context, userID int64, name, passkeyID string, cred webauthn.Credential) error {
	raw, _ := json.Marshal(cred)
	_, err := s.db.Exec(ctx,
		`INSERT INTO user_passkeys (user_id, passkey_name, passkey_id, passkey, counter)
		 VALUES (?, ?, ?, ?, ?)`,
		userID, name, passkeyID, string(raw), cred.Authenticator.SignCount)
	return err
}

type rawSession struct {
	Raw        json.RawMessage `json:"raw"`
	UserID     int64           `json:"user_id"`
	Expiration time.Time       `json:"exp"`
	IsLogin    bool            `json:"is_login"`
}

func (s *Store) putSession(ctx context.Context, key string, ses *webauthn.SessionData, userID int64, isLogin bool) error {
	raw, _ := json.Marshal(ses)
	rec, _ := json.Marshal(rawSession{Raw: raw, UserID: userID, Expiration: time.Now().Add(5 * time.Minute), IsLogin: isLogin})
	return s.db.SaveSetting(ctx, "passkey-session:"+key, string(rec))
}

func (s *Store) takeSession(ctx context.Context, key string) (*webauthn.SessionData, error) {
	val, _ := s.db.GetSetting(ctx, "passkey-session:"+key)
	if val == "" {
		return nil, errors.New("unexpected challenge")
	}
	var rec rawSession
	if json.Unmarshal([]byte(val), &rec) != nil || time.Now().After(rec.Expiration) {
		return nil, errors.New("challenge expired")
	}
	var ses webauthn.SessionData
	if json.Unmarshal(rec.Raw, &ses) != nil {
		return nil, errors.New("invalid challenge")
	}
	s.db.DeleteSetting(ctx, "passkey-session:"+key)
	return &ses, nil
}

// ---- registration ----

func (s *Store) RegisterBegin(ctx context.Context, userID int64, email string) (map[string]any, error) {
	u, err := s.loadUser(ctx, userID, email)
	if err != nil {
		return nil, err
	}
	options, ses, err := s.wa.BeginRegistration(u)
	if err != nil {
		return nil, err
	}
	if err := s.putSession(ctx, ses.Challenge, ses, userID, false); err != nil {
		return nil, err
	}
	return optionsToMap(options, ses.Challenge), nil
}

func (s *Store) RegisterFinish(ctx context.Context, userID int64, email, name string, raw []byte) (*webauthn.Credential, error) {
	parsed, err := protocol.ParseCredentialCreationResponseBytes(raw)
	if err != nil {
		return nil, err
	}
	// The client echoes the challenge in a top-level "challenge" field as the
	// session key.
	var top struct {
		Challenge string `json:"challenge"`
	}
	json.Unmarshal(raw, &top)
	if top.Challenge == "" {
		return nil, errors.New("challenge is required")
	}
	ses, err := s.takeSession(ctx, top.Challenge)
	if err != nil {
		return nil, err
	}
	u, err := s.loadUser(ctx, userID, email)
	if err != nil {
		return nil, err
	}
	cred, err := s.wa.CreateCredential(u, *ses, parsed)
	if err != nil {
		return nil, err
	}
	passkeyID := b64URL(cred.ID)
	if err := s.saveCredential(ctx, userID, name, passkeyID, *cred); err != nil {
		return nil, err
	}
	return cred, nil
}

// ---- login ----

func (s *Store) LoginBegin(ctx context.Context) (map[string]any, error) {
	options, ses, err := s.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, err
	}
	if err := s.putSession(ctx, ses.Challenge, ses, 0, true); err != nil {
		return nil, err
	}
	return optionsToMap(options, ses.Challenge), nil
}

func (s *Store) LoginFinish(ctx context.Context, raw []byte) (int64, string, error) {
	parsed, err := protocol.ParseCredentialRequestResponseBytes(raw)
	if err != nil {
		return 0, "", err
	}
	var top struct {
		Challenge string `json:"challenge"`
	}
	json.Unmarshal(raw, &top)
	ses, err := s.takeSession(ctx, top.Challenge)
	if err != nil {
		return 0, "", err
	}
	var resolvedUser *webauthnUser
	cred, err := s.wa.ValidateDiscoverableLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		uid, e := strconv.ParseInt(string(userHandle), 10, 64)
		if e != nil {
			return nil, errors.New("invalid user handle")
		}
		email, _, _ := s.db.ScanString(ctx, `SELECT user_email FROM users WHERE id = ?`, uid)
		if email == "" {
			return nil, errors.New("user not found")
		}
		u, e := s.loadUser(ctx, uid, email)
		if e != nil {
			return nil, e
		}
		resolvedUser = u
		return u, nil
	}, *ses, parsed)
	if err != nil {
		return 0, "", err
	}
	if resolvedUser == nil {
		return 0, "", errors.New("user not found")
	}
	uid, _ := strconv.ParseInt(resolvedUser.id, 10, 64)
	s.db.Exec(ctx, `UPDATE user_passkeys SET counter = ?, updated_at = datetime('now') WHERE user_id = ? AND passkey_id = ?`,
		cred.Authenticator.SignCount, uid, b64URL(cred.ID))
	return uid, resolvedUser.display, nil
}

// ---- management ----

func (s *Store) List(ctx context.Context, userID int64) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx,
		`SELECT passkey_id, passkey_name, created_at, updated_at FROM user_passkeys WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, map[string]any{"passkey_id": r["passkey_id"], "passkey_name": r["passkey_name"],
			"created_at": r["created_at"], "updated_at": r["updated_at"]})
	}
	return out, nil
}

func (s *Store) Rename(ctx context.Context, userID int64, id, name string) error {
	if name == "" || len(name) > 255 {
		return errors.New("invalid passkey name")
	}
	_, err := s.db.Exec(ctx, `UPDATE user_passkeys SET passkey_name = ?, updated_at = datetime('now')
		WHERE user_id = ? AND passkey_id = ?`, name, userID, id)
	return err
}

func (s *Store) Delete(ctx context.Context, userID int64, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM user_passkeys WHERE user_id = ? AND passkey_id = ?`, userID, id)
	return err
}

func optionsToMap(options any, challenge string) map[string]any {
	out := map[string]any{"challenge": challenge}
	b, _ := json.Marshal(options)
	json.Unmarshal(b, &out)
	return out
}

func b64URL(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
