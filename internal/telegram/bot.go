package telegram

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"tempmail/internal/auth"
	"tempmail/internal/config"
	"tempmail/internal/db"
	"tempmail/internal/mailparse"
)

const kvPrefix = "temp-mail-telegram"
const settingsKey = "temp-mail-telegram-settings"

// Settings mirrors upstream TelegramSettings (stored in the settings table).
type Settings struct {
	EnableAllowList      bool     `json:"enableAllowList"`
	AllowList            []string `json:"allowList"`
	MiniAppURL           string   `json:"miniAppUrl"`
	EnableGlobalMailPush bool     `json:"enableGlobalMailPush"`
	GlobalMailPushList   []string `json:"globalMailPushList"`
}

// AddressService is implemented by the HTTP layer (address creation/deletion
// live there) to avoid an import cycle.
type AddressService interface {
	NewAddress(ctx context.Context, name, domain string, randomSubdomain bool, sourceMeta string) (address, jwt string, id int64, password *string, err error)
	DeleteAddress(ctx context.Context, addressID int64) error
	RandomName() string
}

type Bot struct {
	cfg   *config.Config
	db    *db.DB
	jwt   *auth.Signer
	addr  AddressService
	http  *http.Client
	token string
}

func New(cfg *config.Config, d *db.DB, signer *auth.Signer, addr AddressService) *Bot {
	return &Bot{cfg: cfg, db: d, jwt: signer, addr: addr, token: cfg.TelegramBotToken,
		http: &http.Client{Timeout: 30 * time.Second}}
}

func (b *Bot) Enabled() bool { return b != nil && b.token != "" }

func (b *Bot) api(method string, payload any) (json.RawMessage, error) {
	body, _ := json.Marshal(payload)
	resp, err := b.http.Post(fmt.Sprintf("https://api.telegram.org/bot%s/%s", b.token, method), "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, errors.New(out.Description)
	}
	return out.Result, nil
}

func (b *Bot) Call(method string, payload any) (json.RawMessage, error) {
	return b.api(method, payload)
}

func (b *Bot) Settings(ctx context.Context) Settings {
	s := Settings{AllowList: []string{}, GlobalMailPushList: []string{}}
	if raw, _ := b.db.GetSetting(ctx, settingsKey); raw != "" {
		_ = json.Unmarshal([]byte(raw), &s)
	}
	if s.AllowList == nil {
		s.AllowList = []string{}
	}
	if s.GlobalMailPushList == nil {
		s.GlobalMailPushList = []string{}
	}
	return s
}

func (b *Bot) SaveSettings(ctx context.Context, s Settings) error {
	raw, _ := json.Marshal(s)
	return b.db.SaveSetting(ctx, settingsKey, string(raw))
}

func (b *Bot) jwtList(ctx context.Context, userID string) []string {
	var list []string
	if raw, _ := b.db.GetSetting(ctx, kvPrefix+":"+userID); raw != "" {
		_ = json.Unmarshal([]byte(raw), &list)
	}
	return list
}

func (b *Bot) saveJWTList(ctx context.Context, userID string, list []string) {
	raw, _ := json.Marshal(list)
	b.db.SaveSetting(ctx, kvPrefix+":"+userID, string(raw))
}

type addressData struct {
	list    []string
	idMap   map[string]int64
	invalid []string
}

func (b *Bot) addressData(ctx context.Context, jwts []string) addressData {
	d := addressData{idMap: map[string]int64{}}
	for _, t := range jwts {
		c, err := b.jwt.Verify(t)
		if err != nil {
			d.list = append(d.list, "[invalid credential]")
			d.invalid = append(d.invalid, t)
			continue
		}
		addr, id := auth.ClaimStr(c, "address"), auth.ClaimInt(c, "address_id")
		if _, found, _ := b.db.ScanString(ctx, `SELECT name FROM address WHERE id = ?`, id); !found {
			d.list = append(d.list, "[invalid address]")
			d.invalid = append(d.invalid, t)
			continue
		}
		d.list = append(d.list, addr)
		d.idMap[addr] = id
	}
	return d
}

// UserAddresses returns the bound addresses with their JWTs (mini app).
func (b *Bot) UserAddresses(ctx context.Context, userID string) []map[string]string {
	out := []map[string]string{}
	for _, t := range b.jwtList(ctx, userID) {
		if c, err := b.jwt.Verify(t); err == nil {
			out = append(out, map[string]string{"address": auth.ClaimStr(c, "address"), "jwt": t})
		}
	}
	return out
}

func (b *Bot) NewAddress(ctx context.Context, userID, address string, randomSub bool) (string, string, *string, error) {
	address = strings.TrimSpace(address)
	name, domain := address, ""
	if i := strings.Index(address, "@"); i >= 0 {
		name, domain = address[:i], address[i+1:]
	}
	list := b.jwtList(ctx, userID)
	if len(list) >= b.cfg.TGMaxAddress {
		return "", "", nil, fmt.Errorf("Max address count reached (%d)", b.cfg.TGMaxAddress)
	}
	if name == "" || b.cfg.DisableCustomAddressName {
		name = b.addr.RandomName()
	}
	addr, token, _, pw, err := b.addr.NewAddress(ctx, name, domain, randomSub, "tg:"+userID)
	if err != nil {
		return "", "", nil, err
	}
	b.saveJWTList(ctx, userID, append(list, token))
	b.db.SaveSetting(ctx, kvPrefix+":"+addr, userID)
	return addr, token, pw, nil
}

func (b *Bot) Bind(ctx context.Context, userID, token string) (string, error) {
	c, err := b.jwt.Verify(token)
	if err != nil || auth.ClaimStr(c, "address") == "" {
		return "", errors.New("Invalid credential")
	}
	addr := auth.ClaimStr(c, "address")
	list := b.jwtList(ctx, userID)
	if _, ok := b.addressData(ctx, list).idMap[addr]; ok {
		return addr, nil
	}
	if len(list) >= b.cfg.TGMaxAddress {
		return "", errors.New("Max address count reached, run /cleaninvalidaddress first")
	}
	b.saveJWTList(ctx, userID, append(list, token))
	b.db.SaveSetting(ctx, kvPrefix+":"+addr, userID)
	return addr, nil
}

func (b *Bot) Unbind(ctx context.Context, userID, addr string) {
	var keep []string
	for _, t := range b.jwtList(ctx, userID) {
		if c, err := b.jwt.Verify(t); err == nil && auth.ClaimStr(c, "address") == addr {
			continue
		}
		keep = append(keep, t)
	}
	b.saveJWTList(ctx, userID, keep)
	b.db.DeleteSetting(ctx, kvPrefix+":"+addr)
}

// UnbindByAddress is called when an address is deleted elsewhere.
func (b *Bot) UnbindByAddress(ctx context.Context, addr string) {
	if b == nil {
		return
	}
	if uid, _ := b.db.GetSetting(ctx, kvPrefix+":"+addr); uid != "" {
		b.Unbind(ctx, uid, addr)
	}
}

func (b *Bot) delete(ctx context.Context, userID, addr string) error {
	d := b.addressData(ctx, b.jwtList(ctx, userID))
	id, ok := d.idMap[addr]
	if !ok {
		return errors.New("Address is not yours")
	}
	b.Unbind(ctx, userID, addr)
	return b.addr.DeleteAddress(ctx, id)
}

// ---- update handling ----

type update struct {
	Message       *message `json:"message"`
	CallbackQuery *struct {
		ID      string   `json:"id"`
		From    *tgUser  `json:"from"`
		Message *message `json:"message"`
		Data    string   `json:"data"`
	} `json:"callback_query"`
}

type tgUser struct {
	ID int64 `json:"id"`
}

type message struct {
	MessageID int64   `json:"message_id"`
	From      *tgUser `json:"from"`
	Chat      struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	Text string `json:"text"`
}

var commands = []map[string]string{
	{"command": "start", "description": "开始使用 | Get started"},
	{"command": "new", "description": "新建邮箱 /new <name>@<domain> | Create address"},
	{"command": "address", "description": "查看邮箱地址列表 | View address list"},
	{"command": "bind", "description": "绑定邮箱 /bind <凭证> | Bind address"},
	{"command": "unbind", "description": "解绑邮箱 /unbind <地址> | Unbind address"},
	{"command": "delete", "description": "删除邮箱 /delete <地址> | Delete address"},
	{"command": "mails", "description": "查看邮件 /mails <地址> | View mails"},
	{"command": "cleaninvalidaddress", "description": "清理无效地址 | Clean invalid addresses"},
}

func (b *Bot) SetCommands() error {
	_, err := b.api("setMyCommands", map[string]any{"commands": commands})
	return err
}

func (b *Bot) SetWebhook(u string) error {
	_, err := b.api("setWebhook", map[string]any{"url": u})
	return err
}

func (b *Bot) Status() (map[string]any, error) {
	info, err := b.api("getWebhookInfo", map[string]any{})
	if err != nil {
		return nil, err
	}
	cmds, _ := b.api("getMyCommands", map[string]any{})
	return map[string]any{"info": info, "commands": cmds}, nil
}

func (b *Bot) reply(chatID int64, text string, markup any) {
	p := map[string]any{"chat_id": chatID, "text": text}
	if markup != nil {
		p["reply_markup"] = markup
	}
	if _, err := b.api("sendMessage", p); err != nil {
		log.Printf("telegram sendMessage: %v", err)
	}
}

// HandleUpdate processes one webhook/polling update.
func (b *Bot) HandleUpdate(ctx context.Context, raw []byte) {
	var u update
	if json.Unmarshal(raw, &u) != nil {
		return
	}
	if u.CallbackQuery != nil {
		b.handleCallback(ctx, &u)
		return
	}
	m := u.Message
	if m == nil || m.Chat.Type != "private" || m.From == nil {
		return
	}
	uid := strconv.FormatInt(m.From.ID, 10)
	s := b.Settings(ctx)
	if s.EnableAllowList && !containsStr(s.AllowList, uid) {
		b.reply(m.Chat.ID, "You do not have permission to use this bot", nil)
		return
	}
	cmd, arg := m.Text, ""
	if i := strings.IndexAny(m.Text, " \n"); i >= 0 {
		cmd, arg = m.Text[:i], strings.TrimSpace(m.Text[i+1:])
	}
	if j := strings.Index(cmd, "@"); j > 0 {
		cmd = cmd[:j]
	}
	switch cmd {
	case "/start":
		var sb strings.Builder
		sb.WriteString("Welcome to Temp Mail\n\n")
		if b.cfg.Prefix != "" {
			sb.WriteString("Current prefix: " + b.cfg.Prefix + "\n")
		}
		d, _ := json.Marshal(b.cfg.Domains)
		sb.WriteString("Domains: " + string(d) + "\nCommands:\n")
		for _, c := range commands {
			sb.WriteString("/" + c["command"] + ": " + c["description"] + "\n")
		}
		b.reply(m.Chat.ID, sb.String(), nil)
	case "/new":
		addr, token, pw, err := b.NewAddress(ctx, uid, arg, false)
		if err != nil {
			b.reply(m.Chat.ID, "Failed to create address: "+err.Error(), nil)
			return
		}
		text := "Address created\nAddress: " + addr + "\n"
		if pw != nil {
			text += "Password: " + *pw + "\n"
		}
		b.reply(m.Chat.ID, text+"Credential: "+token, nil)
	case "/bind":
		if arg == "" {
			b.reply(m.Chat.ID, "Please input the address credential", nil)
			return
		}
		addr, err := b.Bind(ctx, uid, arg)
		if err != nil {
			b.reply(m.Chat.ID, "Bind failed: "+err.Error(), nil)
			return
		}
		b.reply(m.Chat.ID, "Bind success\nAddress: "+addr, nil)
	case "/unbind":
		if arg == "" {
			b.reply(m.Chat.ID, "Please input the address", nil)
			return
		}
		b.Unbind(ctx, uid, arg)
		b.reply(m.Chat.ID, "Unbind success\nAddress: "+arg, nil)
	case "/delete":
		if arg == "" {
			b.reply(m.Chat.ID, "Please input the address", nil)
			return
		}
		if err := b.delete(ctx, uid, arg); err != nil {
			b.reply(m.Chat.ID, "Delete failed: "+err.Error(), nil)
			return
		}
		b.reply(m.Chat.ID, "Deleted "+arg, nil)
	case "/address":
		d := b.addressData(ctx, b.jwtList(ctx, uid))
		b.reply(m.Chat.ID, "Address list\n\n"+joinAddr(d.list), nil)
	case "/cleaninvalidaddress":
		list := b.jwtList(ctx, uid)
		d := b.addressData(ctx, list)
		var keep []string
		for _, t := range list {
			if !containsStr(d.invalid, t) {
				keep = append(keep, t)
			}
		}
		b.saveJWTList(ctx, uid, keep)
		b.reply(m.Chat.ID, "Cleaned\n\nCurrent address list\n\n"+joinAddr(b.addressData(ctx, keep).list), nil)
	case "/mails":
		b.queryMail(ctx, m.Chat.ID, uid, arg, 0, 0)
	}
}

func joinAddr(list []string) string {
	if len(list) == 0 {
		return "(empty)"
	}
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = "Address: " + a
	}
	return strings.Join(out, "\n")
}

func (b *Bot) handleCallback(ctx context.Context, u *update) {
	cq := u.CallbackQuery
	defer b.api("answerCallbackQuery", map[string]any{"callback_query_id": cq.ID})
	if cq.From == nil || cq.Message == nil || !strings.HasPrefix(cq.Data, "mail_") {
		return
	}
	parts := strings.Split(cq.Data, "_")
	if len(parts) != 3 {
		return
	}
	idx, _ := strconv.Atoi(parts[2])
	b.queryMail(ctx, cq.Message.Chat.ID, strconv.FormatInt(cq.From.ID, 10), parts[1], idx, cq.Message.MessageID)
}

func (b *Bot) queryMail(ctx context.Context, chatID int64, uid, addr string, idx int, editID int64) {
	d := b.addressData(ctx, b.jwtList(ctx, uid))
	if addr == "" && len(d.list) > 0 {
		addr = d.list[0]
	}
	if _, ok := d.idMap[addr]; !ok {
		b.reply(chatID, "Address not bound: "+addr, nil)
		return
	}
	row, _ := b.db.QueryOne(ctx, `SELECT * FROM raw_mails WHERE address = ? ORDER BY id DESC LIMIT 1 OFFSET ?`, addr, idx)
	text := "No more mails"
	var mailID int64
	if row != nil {
		mailID = row.Int("id")
		text = b.formatMail(row.Str("raw"), addr, row.Str("created_at"), row.Str("metadata"))
	}
	buttons := []map[string]any{{"text": "Prev", "callback_data": fmt.Sprintf("mail_%s_%d", addr, idx-1)}}
	if idx <= 0 {
		buttons = buttons[:0]
	}
	if u := b.miniAppURL(ctx, mailID); u != "" {
		buttons = append(buttons, map[string]any{"text": "View", "web_app": map[string]string{"url": u}})
	}
	if row != nil {
		buttons = append(buttons, map[string]any{"text": "Next", "callback_data": fmt.Sprintf("mail_%s_%d", addr, idx+1)})
	}
	markup := map[string]any{"inline_keyboard": [][]map[string]any{buttons}}
	if editID != 0 {
		if _, err := b.api("editMessageText", map[string]any{"chat_id": chatID, "message_id": editID, "text": text, "reply_markup": markup}); err != nil {
			log.Printf("telegram edit: %v", err)
		}
		return
	}
	b.reply(chatID, text, markup)
}

func (b *Bot) miniAppURL(ctx context.Context, mailID int64) string {
	s := b.Settings(ctx)
	if s.MiniAppURL == "" || mailID == 0 {
		return ""
	}
	u, err := url.Parse(s.MiniAppURL)
	if err != nil {
		return ""
	}
	u.Path = "/telegram_mail"
	q := u.Query()
	q.Set("mail_id", strconv.FormatInt(mailID, 10))
	u.RawQuery = q.Encode()
	return u.String()
}

func (b *Bot) formatMail(raw, addr, createdAt, metadata string) string {
	m, err := mailparse.Parse([]byte(raw))
	if err != nil || m == nil {
		return "Failed to parse mail"
	}
	text := m.Text
	if len(text) > 1000 {
		text = text[:1000] + "\n\n...\n(message too long, view in app)"
	}
	if text == "" {
		text = "(no text content, view in app)"
	}
	var sb strings.Builder
	if ai := formatAIExtract(metadata); ai != "" {
		sb.WriteString(ai)
	}
	sb.WriteString("From: " + m.Sender + "\nTo: " + addr + "\n")
	if createdAt != "" {
		sb.WriteString("Date: " + createdAt + "\n")
	}
	sb.WriteString("Subject: " + m.Subject + "\nContent:\n" + text)
	return sb.String()
}

func formatAIExtract(metadata string) string {
	if metadata == "" {
		return ""
	}
	var meta struct {
		AI struct {
			Type       string `json:"type"`
			Result     string `json:"result"`
			ResultText string `json:"result_text"`
		} `json:"ai_extract"`
	}
	if json.Unmarshal([]byte(metadata), &meta) != nil || meta.AI.Type == "" || meta.AI.Type == "none" || meta.AI.Result == "" {
		return ""
	}
	labels := map[string]string{"auth_code": "Code", "auth_link": "Auth link", "service_link": "Service link",
		"subscription_link": "Subscription link", "other_link": "Link"}
	label, ok := labels[meta.AI.Type]
	if !ok {
		return ""
	}
	extra := ""
	if meta.AI.Type != "auth_code" && meta.AI.ResultText != "" && meta.AI.ResultText != meta.AI.Result {
		extra = " (" + meta.AI.ResultText + ")"
	}
	return "AI extract\n" + label + ": " + meta.AI.Result + extra + "\n\n"
}

// PushMail notifies the bound Telegram user (and the global push list).
func (b *Bot) PushMail(ctx context.Context, addr string, mailID int64, raw string, parsed *mailparse.Mail, metadata string) {
	if !b.Enabled() {
		return
	}
	uid, _ := b.db.GetSetting(ctx, kvPrefix+":"+addr)
	s := b.Settings(ctx)
	targets := []string{}
	if s.EnableGlobalMailPush {
		targets = append(targets, s.GlobalMailPushList...)
	}
	if uid != "" && !containsStr(targets, uid) {
		targets = append(targets, uid)
	}
	if len(targets) == 0 {
		return
	}
	text := b.formatMail(raw, addr, time.Now().UTC().Format(time.RFC1123), metadata)
	var markup any
	if u := b.miniAppURL(ctx, mailID); u != "" {
		markup = map[string]any{"inline_keyboard": [][]map[string]any{{{"text": "View", "web_app": map[string]string{"url": u}}}}}
	}
	for _, t := range targets {
		chatID, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			continue
		}
		b.reply(chatID, text, markup)
		if b.cfg.EnableTGPushAttachment && parsed != nil {
			for _, at := range parsed.Attachments {
				if len(at.Content) == 0 || len(at.Content) > 50<<20 {
					continue
				}
				b.sendDocument(chatID, at.Filename, at.Content, "From: "+parsed.Sender+"\nSubject: "+parsed.Subject)
			}
		}
	}
}

func (b *Bot) sendDocument(chatID int64, name string, content []byte, caption string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	w.WriteField("caption", caption)
	fw, _ := w.CreateFormFile("document", name)
	fw.Write(content)
	w.Close()
	resp, err := b.http.Post(fmt.Sprintf("https://api.telegram.org/bot%s/sendDocument", b.token), w.FormDataContentType(), &buf)
	if err != nil {
		log.Printf("telegram sendDocument: %v", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

// CheckInitData validates Telegram Mini App initData and returns the user id.
func (b *Bot) CheckInitData(initData string) (string, error) {
	vals, err := url.ParseQuery(initData)
	if err != nil {
		return "", errors.New("Invalid initData")
	}
	hash := vals.Get("hash")
	user := vals.Get("user")
	if hash == "" || user == "" {
		return "", errors.New("Invalid initData")
	}
	if authDate, _ := strconv.ParseInt(vals.Get("auth_date"), 10, 64); authDate+300 < time.Now().Unix() {
		return "", errors.New("Auth date expired")
	}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+vals.Get(k))
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(b.token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(parts, "\n")))
	if hex.EncodeToString(mac.Sum(nil)) != hash {
		return "", errors.New("Invalid initData")
	}
	var u struct {
		ID json.Number `json:"id"`
	}
	if json.Unmarshal([]byte(user), &u) != nil || u.ID == "" {
		return "", errors.New("Invalid initData")
	}
	return u.ID.String(), nil
}

// IsSuperUser reports whether the user is on the global push list.
func (b *Bot) IsSuperUser(ctx context.Context, uid string) bool {
	s := b.Settings(ctx)
	return s.EnableGlobalMailPush && containsStr(s.GlobalMailPushList, uid)
}

func (b *Bot) OwnsAddress(ctx context.Context, uid, addr string) bool {
	_, ok := b.addressData(ctx, b.jwtList(ctx, uid)).idMap[addr]
	return ok
}

// RunPolling uses getUpdates instead of a webhook (for setups without a
// public HTTPS endpoint).
func (b *Bot) RunPolling(ctx context.Context) {
	b.api("deleteWebhook", map[string]any{})
	var offset int64
	for ctx.Err() == nil {
		res, err := b.api("getUpdates", map[string]any{"offset": offset, "timeout": 25})
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		var updates []json.RawMessage
		json.Unmarshal(res, &updates)
		for _, u := range updates {
			var id struct {
				UpdateID int64 `json:"update_id"`
			}
			json.Unmarshal(u, &id)
			offset = id.UpdateID + 1
			b.HandleUpdate(ctx, u)
		}
	}
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
