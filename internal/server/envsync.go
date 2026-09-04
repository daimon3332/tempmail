package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readEnvValues(path string) map[string]string {
	values := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return values
	}
	scanner := bufio.NewScanner(strings.NewReader(string(b)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key != "" {
			if key == "ADMIN_PASSWORDS" {
				values[key] = ""
			} else {
				values[key] = strings.Trim(strings.TrimSpace(parts[1]), "\"")
			}
		}
	}
	return values
}

// syncRuntimeEnv updates only keys managed by the settings UI and preserves
// comments and unrelated deployment variables in the existing env file.
func syncRuntimeEnv(path string, rc runtimeConfig) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("environment sync path is empty")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	values := map[string]string{}
	for key, value := range rc.Environment {
		if validEnvKey(key) {
			values[key] = value
		}
	}
	setList := func(key string, v []string) { raw, _ := json.Marshal(v); values[key] = string(raw) }
	setList("DOMAINS", rc.Domains)
	setList("DEFAULT_DOMAINS", rc.DefaultDomains)
	setList("RANDOM_SUBDOMAIN_DOMAINS", rc.RandomSubdomainDomains)
	setList("FORWARD_ADDRESS_LIST", rc.ForwardAddressList)
	setList("JUNK_MAIL_CHECK_LIST", rc.JunkMailCheckList)
	setList("PASSWORDS", nonEmptyList(rc.SitePassword))
	setList("ADMIN_PASSWORDS", nonEmptyList(rc.AdminPassword))
	values["PREFIX"] = rc.Prefix
	values["MIN_ADDRESS_LEN"] = fmt.Sprint(rc.MinAddressLen)
	values["MAX_ADDRESS_LEN"] = fmt.Sprint(rc.MaxAddressLen)
	values["ADDRESS_REGEX"] = rc.AddressRegex
	values["ADDRESS_CHECK_REGEX"] = rc.AddressCheckRegex
	values["RANDOM_SUBDOMAIN_LENGTH"] = fmt.Sprint(rc.RandomSubdomainLength)
	values["ENABLE_USER_CREATE_EMAIL"] = fmt.Sprint(rc.EnableUserCreateEmail)
	values["DISABLE_ANONYMOUS_USER_CREATE_EMAIL"] = fmt.Sprint(rc.DisableAnonymousUserCreateEmail)
	values["DISABLE_CUSTOM_ADDRESS_NAME"] = fmt.Sprint(rc.DisableCustomAddressName)
	values["ENABLE_USER_DELETE_EMAIL"] = fmt.Sprint(rc.EnableUserDeleteEmail)
	values["ENABLE_MAIL_READ_STATUS"] = fmt.Sprint(rc.EnableMailReadStatus)
	values["ENABLE_ADDRESS_PASSWORD"] = fmt.Sprint(rc.EnableAddressPassword)
	values["ENABLE_WEBHOOK"] = fmt.Sprint(rc.EnableWebhook)
	values["ENABLE_AUTO_REPLY"] = fmt.Sprint(rc.EnableAutoReply)
	values["ENABLE_CHECK_JUNK_MAIL"] = fmt.Sprint(rc.EnableCheckJunkMail)
	values["BLOCK_UNKNOWN_ADDRESS"] = fmt.Sprint(rc.BlockUnknownAddress)
	values["AI_EXTRACT_ENDPOINT"] = rc.AIEndpoint
	values["AI_EXTRACT_API_KEY"] = rc.AIAPIKey
	values["AI_EXTRACT_MODEL"] = rc.AIModel
	values["RATE_LIMIT_PER_MINUTE"] = fmt.Sprint(rc.RateLimitPerMinute)
	values["API_KEY"] = rc.APIKey
	return writeEnvValues(path, b, values)
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func nonEmptyList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return []string{}
	}
	return []string{v}
}

func writeEnvValues(path string, original []byte, values map[string]string) error {
	scanner := bufio.NewScanner(strings.NewReader(string(original)))
	seen := map[string]bool{}
	lines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || !strings.Contains(line, "=") {
			lines = append(lines, line)
			continue
		}
		key := strings.TrimSpace(strings.SplitN(line, "=", 2)[0])
		if value, ok := values[key]; ok {
			lines = append(lines, key+"="+envValue(value, key))
			seen[key] = true
		} else {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for key, value := range values {
		if !seen[key] {
			lines = append(lines, key+"="+envValue(value, key))
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".env.sync-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	content := []byte(strings.Join(lines, "\n") + "\n")
	if _, err = tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpName, path); err == nil {
		return nil
	}
	// Docker bind-mounted files cannot be replaced with rename(2). Fall back
	// to an in-place write for that case while retaining atomic replacement for
	// regular files.
	return os.WriteFile(path, content, 0600)
}

func envValue(value, key string) string {
	if strings.HasPrefix(value, "[") || value == "true" || value == "false" || key == "RATE_LIMIT_PER_MINUTE" || key == "MIN_ADDRESS_LEN" || key == "MAX_ADDRESS_LEN" || key == "RANDOM_SUBDOMAIN_LENGTH" {
		return value
	}
	quoted, _ := json.Marshal(value)
	return string(quoted)
}
