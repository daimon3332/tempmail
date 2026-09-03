package mailparse

import "testing"

const multipart = "From: =?utf-8?b?5rWL6K+V?= <a@b.com>\r\nTo: x@y.org\r\nSubject: =?utf-8?q?c=C3=B3digo_123?=\r\nMessage-ID: <m1@b.com>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"B1\"\r\n\r\n--B1\r\nContent-Type: multipart/alternative; boundary=\"B2\"\r\n\r\n--B2\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n5L2g5aW9\r\n--B2\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<b>hi</b>\r\n--B2--\r\n--B1\r\nContent-Type: application/pdf; name=\"f.pdf\"\r\nContent-Disposition: attachment; filename=\"f.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\nAAEC\r\n--B1--\r\n"

func TestParse(t *testing.T) {
	m, err := Parse([]byte(multipart))
	if err != nil {
		t.Fatal(err)
	}
	if m.Sender != "测试 <a@b.com>" || m.FromAddress != "a@b.com" {
		t.Fatalf("sender %q from %q", m.Sender, m.FromAddress)
	}
	if m.Subject != "código 123" {
		t.Fatalf("subject %q", m.Subject)
	}
	if m.Text != "你好" || m.HTML != "<b>hi</b>" {
		t.Fatalf("text %q html %q", m.Text, m.HTML)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Filename != "f.pdf" || m.Attachments[0].Size != 3 {
		t.Fatalf("attachments %+v", m.Attachments)
	}
	if m.MessageID != "<m1@b.com>" {
		t.Fatalf("message id %q", m.MessageID)
	}
}

func TestParsePlain(t *testing.T) {
	m, err := Parse([]byte("From: a@b.com\r\nSubject: s\r\n\r\nplain body"))
	if err != nil || m.Text != "plain body" || m.HTML != "" {
		t.Fatalf("%v %+v", err, m)
	}
}
