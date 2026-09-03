package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

type customSQLCleanup struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	SQL     string `json:"sql"`
	Enabled bool   `json:"enabled"`
}

type cleanupSettings struct {
	EnableMailsAutoCleanup           bool               `json:"enableMailsAutoCleanup"`
	CleanMailsDays                   int                `json:"cleanMailsDays"`
	EnableUnknowMailsAutoCleanup     bool               `json:"enableUnknowMailsAutoCleanup"`
	CleanUnknowMailsDays             int                `json:"cleanUnknowMailsDays"`
	EnableSendBoxAutoCleanup         bool               `json:"enableSendBoxAutoCleanup"`
	CleanSendBoxDays                 int                `json:"cleanSendBoxDays"`
	EnableAddressAutoCleanup         bool               `json:"enableAddressAutoCleanup"`
	CleanAddressDays                 int                `json:"cleanAddressDays"`
	EnableInactiveAddressAutoCleanup bool               `json:"enableInactiveAddressAutoCleanup"`
	CleanInactiveAddressDays         int                `json:"cleanInactiveAddressDays"`
	EnableUnboundAddressAutoCleanup  bool               `json:"enableUnboundAddressAutoCleanup"`
	CleanUnboundAddressDays          int                `json:"cleanUnboundAddressDays"`
	EnableEmptyAddressAutoCleanup    bool               `json:"enableEmptyAddressAutoCleanup"`
	CleanEmptyAddressDays            int                `json:"cleanEmptyAddressDays"`
	CustomSqlCleanupList             []customSQLCleanup `json:"customSqlCleanupList"`
}

func validateCustomSQL(s string) error {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ";"))
	switch {
	case s == "":
		return errors.New("SQL is empty")
	case len(s) > 1000:
		return errors.New("SQL too long (max 1000)")
	case !strings.HasPrefix(strings.ToUpper(s), "DELETE "):
		return errors.New("Only DELETE statements are allowed")
	case strings.Contains(s, ";"):
		return errors.New("Only a single statement is allowed")
	case strings.Contains(s, "--") || strings.Contains(s, "/*"):
		return errors.New("SQL comments are not allowed")
	}
	return nil
}

func (a *App) deleteAddressesWhere(ctx context.Context, cond string, args ...any) error {
	stmts := []string{
		`DELETE FROM raw_mails WHERE address IN (SELECT name FROM address WHERE ` + cond + `)`,
		`DELETE FROM sendbox WHERE address IN (SELECT name FROM address WHERE ` + cond + `)`,
		`DELETE FROM auto_reply_mails WHERE address IN (SELECT name FROM address WHERE ` + cond + `)`,
		`DELETE FROM address_sender WHERE address IN (SELECT name FROM address WHERE ` + cond + `)`,
		`DELETE FROM users_address WHERE address_id IN (SELECT id FROM address WHERE ` + cond + `)`,
		`DELETE FROM address WHERE ` + cond,
	}
	for _, s := range stmts {
		if _, err := a.db.Exec(ctx, s, args...); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) cleanup(ctx context.Context, cleanType string, days int) error {
	if cleanType == "" || days < 0 || days > 1000 {
		return errors.New("Invalid cleanup config")
	}
	batch := 3000
	mod := fmt.Sprintf("-%d day", days)
	log.Printf("cleanup %s before %d days", cleanType, days)
	switch cleanType {
	case "inactiveAddress":
		return a.deleteAddressesWhere(ctx, fmt.Sprintf(`id IN (SELECT id FROM address WHERE updated_at < datetime('now', '%s') ORDER BY updated_at, id LIMIT %d)`, mod, batch))
	case "addressCreated":
		return a.deleteAddressesWhere(ctx, fmt.Sprintf(`id IN (SELECT id FROM address WHERE created_at < datetime('now', '%s') ORDER BY created_at, id LIMIT %d)`, mod, batch))
	case "unboundAddress":
		return a.deleteAddressesWhere(ctx, fmt.Sprintf(`id NOT IN (SELECT address_id FROM users_address) AND created_at < datetime('now', '%s')`, mod))
	case "emptyAddress":
		return a.deleteAddressesWhere(ctx, fmt.Sprintf(`name NOT IN (SELECT DISTINCT address FROM raw_mails WHERE address IS NOT NULL) AND created_at < datetime('now', '%s')`, mod))
	case "mails":
		_, err := a.db.Exec(ctx, `DELETE FROM raw_mails WHERE id IN (SELECT id FROM raw_mails WHERE created_at < datetime('now', ?) ORDER BY created_at, id LIMIT ?)`, mod, batch)
		return err
	case "mails_unknow":
		_, err := a.db.Exec(ctx, `DELETE FROM raw_mails WHERE address NOT IN (SELECT name FROM address) AND created_at < datetime('now', ?)`, mod)
		return err
	case "sendbox":
		_, err := a.db.Exec(ctx, `DELETE FROM sendbox WHERE id IN (SELECT id FROM sendbox WHERE created_at < datetime('now', ?) ORDER BY created_at, id LIMIT ?)`, mod, batch)
		return err
	}
	return errors.New("Invalid clean type")
}

// RunScheduledCleanup mirrors upstream scheduled.ts and is invoked hourly.
func (a *App) RunScheduledCleanup(ctx context.Context) {
	var s cleanupSettings
	if !a.jsonSetting(ctx, "auto_cleanup", &s) {
		return
	}
	run := func(enabled bool, t string, d int) {
		if enabled {
			if err := a.cleanup(ctx, t, d); err != nil {
				log.Printf("cleanup %s: %v", t, err)
			}
		}
	}
	run(s.EnableMailsAutoCleanup, "mails", s.CleanMailsDays)
	run(s.EnableUnknowMailsAutoCleanup, "mails_unknow", s.CleanUnknowMailsDays)
	run(s.EnableSendBoxAutoCleanup, "sendbox", s.CleanSendBoxDays)
	run(s.EnableAddressAutoCleanup, "addressCreated", s.CleanAddressDays)
	run(s.EnableInactiveAddressAutoCleanup, "inactiveAddress", s.CleanInactiveAddressDays)
	run(s.EnableUnboundAddressAutoCleanup, "unboundAddress", s.CleanUnboundAddressDays)
	run(s.EnableEmptyAddressAutoCleanup, "emptyAddress", s.CleanEmptyAddressDays)
	for _, c := range s.CustomSqlCleanupList {
		if !c.Enabled || validateCustomSQL(c.SQL) != nil {
			continue
		}
		if n, err := a.db.Exec(ctx, strings.TrimSuffix(strings.TrimSpace(c.SQL), ";")); err != nil {
			log.Printf("custom cleanup [%s]: %v", c.Name, err)
		} else {
			log.Printf("custom cleanup [%s]: %d rows", c.Name, n)
		}
	}
}

func StartScheduler(ctx context.Context, a *App, every time.Duration) {
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.RunScheduledCleanup(ctx)
			}
		}
	}()
}

var regexpMustCompile = regexp.MustCompile
