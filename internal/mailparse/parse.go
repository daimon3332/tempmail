package mailparse

import (
	"bytes"
	"io"
	"mime"
	"net/mail"
	"strings"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
)

type Attachment struct {
	Filename    string `json:"filename"`
	MimeType    string `json:"mimeType"`
	Disposition string `json:"disposition"`
	Size        int    `json:"size"`
	Content     []byte `json:"-"`
}

type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Mail struct {
	Sender      string       `json:"sender"`
	FromAddress string       `json:"-"`
	Subject     string       `json:"subject"`
	Text        string       `json:"text"`
	HTML        string       `json:"html"`
	MessageID   string       `json:"-"`
	Headers     []Header     `json:"headers"`
	Attachments []Attachment `json:"attachments"`
}

// Parse extracts the fields the upstream project derives with postal-mime.
func Parse(raw []byte) (*Mail, error) {
	m := &Mail{Headers: []Header{}, Attachments: []Attachment{}}
	ent, err := message.Read(bytes.NewReader(raw))
	if err != nil && !message.IsUnknownCharset(err) && !message.IsUnknownEncoding(err) {
		return nil, err
	}
	dec := new(mime.WordDecoder)
	decode := func(s string) string {
		if d, err := dec.DecodeHeader(s); err == nil {
			return d
		}
		return s
	}
	fields := ent.Header.Fields()
	for fields.Next() {
		v, err := fields.Text()
		if err != nil {
			v = decode(fields.Value())
		}
		m.Headers = append(m.Headers, Header{Key: fields.Key(), Value: v})
	}
	m.Subject = decode(ent.Header.Get("Subject"))
	m.MessageID = strings.TrimSpace(ent.Header.Get("Message-ID"))
	from := decode(ent.Header.Get("From"))
	if a, err := mail.ParseAddress(from); err == nil {
		m.FromAddress = a.Address
		if a.Name != "" {
			m.Sender = a.Name + " <" + a.Address + ">"
		} else {
			m.Sender = " <" + a.Address + ">"
		}
	} else {
		m.Sender = from
		m.FromAddress = from
	}
	walk(ent, m)
	return m, nil
}

func walk(ent *message.Entity, m *Mail) {
	if mr := ent.MultipartReader(); mr != nil {
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil && !message.IsUnknownCharset(err) && !message.IsUnknownEncoding(err) {
				break
			}
			if p == nil {
				break
			}
			walk(p, m)
		}
		return
	}
	ctype, params, _ := ent.Header.ContentType()
	disp, dparams, _ := ent.Header.ContentDisposition()
	body, _ := io.ReadAll(ent.Body)
	filename := dparams["filename"]
	if filename == "" {
		filename = params["name"]
	}
	isText := strings.HasPrefix(ctype, "text/")
	if disp == "attachment" || (filename != "" && !isText) || (!isText && ctype != "") {
		if filename == "" {
			filename = "attachment"
		}
		if disp == "" {
			disp = "attachment"
		}
		m.Attachments = append(m.Attachments, Attachment{
			Filename: filename, MimeType: ctype, Disposition: disp, Size: len(body), Content: body,
		})
		return
	}
	switch ctype {
	case "text/html":
		if m.HTML == "" {
			m.HTML = string(body)
		}
	default:
		if m.Text == "" {
			m.Text = string(body)
		}
	}
}
