package icloud

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type rawJar struct {
	mu sync.Mutex
	m  map[string]string
}

func newRawJar() *rawJar { return &rawJar{m: map[string]string{}} }

func (j *rawJar) header() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	keys := make([]string, 0, len(j.m))
	for k := range j.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+j.m[k])
	}
	return strings.Join(parts, "; ")
}

// absorb applies Set-Cookie lines verbatim, dropping cookies Apple expires.
func (j *rawJar) absorb(lines []string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, line := range lines {
		pair, attrs, _ := strings.Cut(line, ";")
		name, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok || name == "" {
			continue
		}
		attrs = strings.ToLower(attrs)
		if value == "" || value == `""` || strings.Contains(attrs, "max-age=0") || strings.Contains(attrs, "1970") {
			delete(j.m, name)
			continue
		}
		j.m[name] = value
	}
}

func (j *rawJar) snapshot() map[string]string {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make(map[string]string, len(j.m))
	for k, v := range j.m {
		out[k] = v
	}
	return out
}

// ParseCookies accepts a JSON object, a browser-exported JSON array of
// {name, value}, or a raw "a=1; b=2" Cookie header.
func ParseCookies(input string) (map[string]string, error) {
	input = strings.TrimSpace(input)
	out := map[string]string{}
	switch {
	case strings.HasPrefix(input, "{"):
		if err := json.Unmarshal([]byte(input), &out); err != nil {
			return nil, errors.New("invalid cookie JSON")
		}
	case strings.HasPrefix(input, "["):
		var list []struct{ Name, Value string }
		if err := json.Unmarshal([]byte(input), &list); err != nil {
			return nil, errors.New("invalid cookie JSON")
		}
		for _, c := range list {
			out[c.Name] = c.Value
		}
	default:
		input = strings.TrimPrefix(input, "Cookie:")
		for _, part := range strings.Split(input, ";") {
			if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
				out[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	delete(out, "")
	if out["X-APPLE-WEBAUTH-TOKEN"] == "" && out["X-APPLE-DS-WEB-SESSION-TOKEN"] == "" {
		return nil, errors.New("cookies must include X-APPLE-WEBAUTH-TOKEN")
	}
	return out, nil
}

// HMEHeader is the X-Icloud-Hme header Apple adds to forwarded mail:
// p = the Hide My Email address, f = forward target, s = original sender.
type HMEHeader struct {
	Alias     string
	ForwardTo string
	Sender    string
}

func ParseHMEHeader(v string) HMEHeader {
	var h HMEHeader
	for _, part := range strings.Split(strings.Join(strings.Fields(v), " "), ";") {
		k, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		addr := bareAddress(val)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "p":
			h.Alias = addr
		case "f":
			h.ForwardTo = addr
		case "s":
			h.Sender = addr
		}
	}
	return h
}

func bareAddress(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.LastIndex(v, "<"); i >= 0 {
		v = v[i+1:]
		if j := strings.Index(v, ">"); j >= 0 {
			v = v[:j]
		}
	}
	v = strings.ToLower(strings.TrimSpace(v))
	if !strings.Contains(v, "@") {
		return ""
	}
	return v
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
