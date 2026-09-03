package importer

import (
	"context"
	"path/filepath"
	"testing"

	"tempmail/internal/db"
)

const dump = `PRAGMA defer_foreign_keys=TRUE;
CREATE TABLE raw_mails ( id INTEGER PRIMARY KEY, message_id TEXT, source TEXT, address TEXT, raw TEXT, metadata TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP );
INSERT INTO "raw_mails" ("id","message_id","source","address","raw","metadata","created_at") VALUES(7,'<a@b>','s@x.com','u1@d.org',replace(replace('Subject: it''s; ok\r\n\r\nbody (1, 2)','\r',char(13)),'\n',char(10)),NULL,'2026-01-01 00:00:00');
CREATE TABLE address ( id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT UNIQUE, password TEXT, source_meta TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP );
INSERT INTO "address" ("id","name","password","source_meta","created_at","updated_at") VALUES(3,'u1@d.org',NULL,'admin','2026-01-01 00:00:00','2026-01-01 00:00:00');
CREATE TABLE users ( id INTEGER PRIMARY KEY, user_email TEXT UNIQUE NOT NULL, password TEXT NOT NULL, user_info TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP );
INSERT INTO "users" ("id","user_email","password","user_info","created_at","updated_at") VALUES(9,'me@x.com','h',NULL,'2026-01-01 00:00:00','2026-01-01 00:00:00');
CREATE TABLE users_address ( id INTEGER PRIMARY KEY, user_id INTEGER, address_id INTEGER UNIQUE, created_at DATETIME DEFAULT CURRENT_TIMESTAMP );
INSERT INTO "users_address" ("id","user_id","address_id","created_at") VALUES(1,9,3,'2026-01-01 00:00:00');
DELETE FROM sqlite_sequence;
CREATE INDEX idx_raw_mails_address ON raw_mails(address);
`

func TestPrimaryThenMerge(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	if _, err := Run(ctx, d, dump, Primary); err != nil {
		t.Fatal(err)
	}
	raw, _, _ := d.ScanString(ctx, `SELECT raw FROM raw_mails WHERE id = 7`)
	if raw != "Subject: it's; ok\r\n\r\nbody (1, 2)" {
		t.Fatalf("raw mismatch: %q", raw)
	}
	if id, _, _ := d.ScanInt(ctx, `SELECT id FROM address WHERE name='u1@d.org'`); id != 3 {
		t.Fatalf("id not preserved: %d", id)
	}
	// merge the same dump: address dup ignored, user dup ignored, mail duplicated with new id
	st, err := Run(ctx, d, dump, Merge)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := d.Count(ctx, `SELECT COUNT(*) FROM address`); n != 1 {
		t.Fatalf("address count %d", n)
	}
	if n, _ := d.Count(ctx, `SELECT COUNT(*) FROM raw_mails`); n != 2 {
		t.Fatalf("mail count %d", n)
	}
	if n, _ := d.Count(ctx, `SELECT COUNT(*) FROM users_address WHERE user_id=9 AND address_id=3`); n != 1 {
		t.Fatalf("binding lost after merge, skipped=%d", st.Skipped)
	}
}
