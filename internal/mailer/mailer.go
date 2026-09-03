package mailer

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"sort"
	"strings"
	"time"

	"tempmail/internal/config"
)

type Mailer struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Mailer { return &Mailer{cfg: cfg} }

type Message struct {
	FromName  string
	From      string
	ToName    string
	To        string
	Subject   string
	Content   string
	IsHTML    bool
	InReplyTo string
}

func addr(name, email string) string {
	if name == "" {
		return "<" + email + ">"
	}
	return mime.QEncoding.Encode("utf-8", name) + " <" + email + ">"
}

// Build renders an RFC 5322 message with base64 body (safe for any charset).
func Build(m Message) []byte {
	var b strings.Builder
	ctype := "text/plain"
	if m.IsHTML {
		ctype = "text/html"
	}
	fmt.Fprintf(&b, "From: %s\r\n", addr(m.FromName, m.From))
	fmt.Fprintf(&b, "To: %s\r\n", addr(m.ToName, m.To))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%d.%s>\r\n", time.Now().UnixNano(), m.From)
	if m.InReplyTo != "" {
		fmt.Fprintf(&b, "In-Reply-To: %s\r\n", m.InReplyTo)
	}
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: %s; charset=utf-8\r\n", ctype)
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(m.Content))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return []byte(b.String())
}

// Send delivers raw bytes from → to using the relay configured for from's
// domain, or directly to the recipient's MX hosts when no relay exists.
func (m *Mailer) Send(from, to string, raw []byte) error {
	domain := from[strings.LastIndex(from, "@")+1:]
	if relay := m.cfg.RelayFor(domain); relay != nil {
		return sendRelay(relay, from, to, raw)
	}
	return sendDirect(m.cfg.SMTPHostname, from, to, raw)
}

func sendRelay(r *config.SMTPRelay, from, to string, raw []byte) error {
	port := r.Port
	if port == 0 {
		port = 587
	}
	host := fmt.Sprintf("%s:%d", r.Host, port)
	var c *smtp.Client
	var err error
	if r.Secure {
		conn, derr := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", host, &tls.Config{ServerName: r.Host})
		if derr != nil {
			return derr
		}
		c, err = smtp.NewClient(conn, r.Host)
	} else {
		c, err = smtp.Dial(host)
	}
	if err != nil {
		return err
	}
	defer c.Close()
	if !r.Secure {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: r.Host}); err != nil {
				return err
			}
		}
	}
	if r.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", r.Username, r.Password, r.Host)); err != nil {
			return err
		}
	}
	envelopeFrom := from
	if r.From != "" {
		envelopeFrom = r.From
	}
	return deliver(c, envelopeFrom, to, raw)
}

func sendDirect(helo, from, to string, raw []byte) error {
	domain := to[strings.LastIndex(to, "@")+1:]
	mxs, err := net.LookupMX(domain)
	if err != nil || len(mxs) == 0 {
		mxs = []*net.MX{{Host: domain}}
	}
	sort.Slice(mxs, func(i, j int) bool { return mxs[i].Pref < mxs[j].Pref })
	var lastErr error
	for _, mx := range mxs {
		host := strings.TrimSuffix(mx.Host, ".")
		conn, err := net.DialTimeout("tcp", host+":25", 15*time.Second)
		if err != nil {
			lastErr = err
			continue
		}
		c, err := smtp.NewClient(conn, host)
		if err != nil {
			lastErr = err
			continue
		}
		if err := c.Hello(helo); err != nil {
			c.Close()
			lastErr = err
			continue
		}
		if ok, _ := c.Extension("STARTTLS"); ok {
			_ = c.StartTLS(&tls.Config{ServerName: host})
		}
		err = deliver(c, from, to, raw)
		c.Close()
		if err == nil {
			return nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no MX host reachable")
	}
	return lastErr
}

func deliver(c *smtp.Client, from, to string, raw []byte) error {
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
