package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	"tempmail/internal/config"
	"tempmail/internal/db"
	"tempmail/internal/mailer"
	"tempmail/internal/mailparse"

	gosmtp "github.com/emersion/go-smtp"
)

// Hooks lets the HTTP layer react to stored mail (webhooks) without an
// import cycle.
type Hooks interface {
	OnMailStored(ctx context.Context, address string, mailID int64, raw string, parsed *mailparse.Mail)
}

type Server struct {
	cfg    *config.Config
	db     *db.DB
	mailer *mailer.Mailer
	hooks  Hooks
	srv    *gosmtp.Server
}

func New(cfg *config.Config, d *db.DB, m *mailer.Mailer, hooks Hooks) *Server {
	s := &Server{cfg: cfg, db: d, mailer: m, hooks: hooks}
	srv := gosmtp.NewServer(s)
	srv.Addr = cfg.SMTPAddr
	srv.Domain = cfg.SMTPHostname
	srv.ReadTimeout = 60 * time.Second
	srv.WriteTimeout = 60 * time.Second
	srv.MaxMessageBytes = cfg.MaxMessageBytes
	srv.MaxRecipients = 50
	srv.AllowInsecureAuth = true
	s.srv = srv
	return s
}

func (s *Server) ListenAndServe() error { return s.srv.ListenAndServe() }
func (s *Server) Close() error          { return s.srv.Close() }

func (s *Server) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	return &session{s: s, remote: c.Conn().RemoteAddr().String()}, nil
}

type session struct {
	s      *Server
	remote string
	from   string
	rcpts  []string
}

func (x *session) AuthMechanisms() []string { return nil }
func (x *session) Auth(string) (gosmtp.AuthSession, error) {
	return nil, errors.New("authentication not supported")
}

func (x *session) Mail(from string, _ *gosmtp.MailOptions) error {
	x.from = strings.ToLower(strings.TrimSpace(from))
	return nil
}

func (x *session) acceptsDomain(domain string) bool {
	for _, d := range x.s.cfg.Domains {
		if domain == d || strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	return false
}

func normalizeAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	i := strings.LastIndex(addr, "@")
	if i < 0 {
		return addr
	}
	return addr[:i] + "@" + strings.ToLower(addr[i+1:])
}

func (x *session) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	to = normalizeAddress(to)
	i := strings.LastIndex(to, "@")
	if i < 0 || !x.acceptsDomain(to[i+1:]) {
		return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 1, 1}, Message: "Relay not permitted"}
	}
	if x.s.blockUnknown() {
		if _, found, _ := x.s.db.ScanInt(context.Background(), `SELECT id FROM address WHERE name = ?`, to); !found {
			return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 1, 1}, Message: "Unknown address"}
		}
	}
	x.rcpts = append(x.rcpts, to)
	return nil
}

func (x *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(io.LimitReader(r, x.s.cfg.MaxMessageBytes+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) > x.s.cfg.MaxMessageBytes {
		return &gosmtp.SMTPError{Code: 552, EnhancedCode: gosmtp.EnhancedCode{5, 3, 4}, Message: "Message too large"}
	}
	ctx := context.Background()
	if x.s.isBlocked(ctx, x.from) {
		return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 7, 1}, Message: "Reject from address"}
	}
	parsed, _ := mailparse.Parse(raw)
	if x.s.isJunk(parsed) {
		return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 7, 1}, Message: "Junk mail"}
	}
	messageID := ""
	if parsed != nil {
		messageID = parsed.MessageID
	}
	for _, to := range x.rcpts {
		if _, err := x.s.store(ctx, x.from, to, messageID, raw, parsed); err != nil {
			log.Printf("smtp: store %s -> %s: %v", x.from, to, err)
			return &gosmtp.SMTPError{Code: 451, EnhancedCode: gosmtp.EnhancedCode{4, 3, 0}, Message: "Failed to save message"}
		}
	}
	return nil
}

func (x *session) Reset()        { x.from, x.rcpts = "", nil }
func (x *session) Logout() error { return nil }

func (s *Server) blockUnknown() bool {
	if s.cfg.BlockUnknownAddress {
		return true
	}
	var rule struct {
		BlockReceiveUnknowAddressEmail bool `json:"blockReceiveUnknowAddressEmail"`
	}
	if raw, _ := s.db.GetSetting(context.Background(), "email_rule_settings"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &rule)
	}
	return rule.BlockReceiveUnknowAddressEmail
}

func (s *Server) isBlocked(ctx context.Context, from string) bool {
	var list []string
	if raw, _ := s.db.GetSetting(ctx, "temp-mail-email-black-list"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &list)
	}
	for _, w := range list {
		if w != "" && strings.Contains(from, w) {
			return true
		}
	}
	return false
}

// isJunk mirrors upstream junk_mail_policy: reject when Authentication-Results
// reports a failure for any configured check (default spf/dkim/dmarc).
func (s *Server) isJunk(m *mailparse.Mail) bool {
	if !s.cfg.EnableCheckJunkMail || m == nil {
		return false
	}
	checks := s.cfg.JunkMailCheckList
	if len(checks) == 0 {
		checks = []string{"spf", "dkim", "dmarc"}
	}
	for _, h := range m.Headers {
		if !strings.EqualFold(h.Key, "Authentication-Results") {
			continue
		}
		v := strings.ToLower(h.Value)
		for _, c := range checks {
			if strings.Contains(v, c+"=fail") || strings.Contains(v, c+"=softfail") {
				return true
			}
		}
	}
	return false
}

// Ingest stores a message delivered by an external relay (e.g. the
// Cloudflare Email Routing worker) through the same pipeline as SMTP.
func (s *Server) Ingest(ctx context.Context, from, to string, raw []byte) (int64, error) {
	to = normalizeAddress(to)
	from = strings.ToLower(strings.TrimSpace(from))
	if s.isBlocked(ctx, from) {
		return 0, errors.New("sender blocked")
	}
	parsed, _ := mailparse.Parse(raw)
	if s.isJunk(parsed) {
		return 0, errors.New("junk mail")
	}
	messageID := ""
	if parsed != nil {
		messageID = parsed.MessageID
	}
	return s.store(ctx, from, to, messageID, raw, parsed)
}

func (s *Server) store(ctx context.Context, from, to, messageID string, raw []byte, parsed *mailparse.Mail) (int64, error) {
	var mid any
	if messageID != "" {
		mid = messageID
	}
	var unread any
	if s.cfg.EnableMailReadStatus {
		unread = 1
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO raw_mails (source, address, raw, message_id, is_unread) VALUES (?, ?, ?, ?, ?)`,
		from, to, string(raw), mid, unread)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	log.Printf("mail: stored %d from=%s to=%s", id, from, to)
	go func() {
		s.forward(from, to, raw)
		s.autoReply(context.Background(), from, to, messageID)
		if s.hooks != nil {
			s.hooks.OnMailStored(context.Background(), to, id, string(raw), parsed)
		}
	}()
	return id, nil
}

type forwardRule struct {
	Domains         []string `json:"domains"`
	Forward         string   `json:"forward"`
	SourcePatterns  []string `json:"sourcePatterns"`
	SourceMatchMode string   `json:"sourceMatchMode"`
}

func matchSource(from string, patterns []string, mode string) bool {
	if len(patterns) == 0 {
		return true
	}
	all := mode == "all"
	for _, p := range patterns {
		if len(p) > 200 {
			continue
		}
		re, err := regexp.Compile("(?i)" + p)
		matched := err == nil && re.MatchString(from)
		if all && !matched {
			return false
		}
		if !all && matched {
			return true
		}
	}
	return all
}

func (s *Server) forward(from, to string, raw []byte) {
	targets := append([]string{}, s.cfg.ForwardAddressList...)
	var rule struct {
		EmailForwardingList []forwardRule `json:"emailForwardingList"`
	}
	if v, _ := s.db.GetSetting(context.Background(), "email_rule_settings"); v != "" {
		_ = json.Unmarshal([]byte(v), &rule)
	}
	toDomain := to[strings.LastIndex(to, "@")+1:]
	for _, r := range rule.EmailForwardingList {
		if r.Forward == "" || !matchSource(from, r.SourcePatterns, r.SourceMatchMode) {
			continue
		}
		if len(r.Domains) == 0 {
			targets = append(targets, r.Forward)
			continue
		}
		for _, d := range r.Domains {
			d = strings.ToLower(strings.TrimSpace(d))
			if d == "" || toDomain == d || strings.HasSuffix(toDomain, "."+d) {
				targets = append(targets, r.Forward)
				break
			}
		}
	}
	for _, t := range targets {
		if err := s.mailer.Send(to, t, raw); err != nil {
			log.Printf("smtp: forward to %s failed: %v", t, err)
		}
	}
}

func (s *Server) autoReply(ctx context.Context, from, to, messageID string) {
	if !s.cfg.EnableAutoReply || messageID == "" || from == "" {
		return
	}
	row, err := s.db.QueryOne(ctx, `SELECT * FROM auto_reply_mails where address = ? and enabled = 1`, to)
	if err != nil || row == nil {
		return
	}
	prefix := row.Str("source_prefix")
	if prefix != "" {
		if len(prefix) > 2 && strings.HasPrefix(prefix, "/") && strings.HasSuffix(prefix, "/") {
			re, err := regexp.Compile(prefix[1 : len(prefix)-1])
			if err != nil || !re.MatchString(from) {
				return
			}
		} else if !strings.HasPrefix(from, prefix) {
			return
		}
	}
	subject, body := row.Str("subject"), row.Str("message")
	if subject == "" {
		subject = "Auto-reply"
	}
	if body == "" {
		body = "This is an auto-reply message, please recontact later."
	}
	name := row.Str("name")
	if name == "" {
		name = to
	}
	raw := mailer.Build(mailer.Message{FromName: name, From: to, To: from, Subject: subject, Content: body, InReplyTo: messageID})
	if err := s.mailer.Send(to, from, raw); err != nil {
		log.Printf("smtp: auto reply to %s failed: %v", from, err)
	}
}
