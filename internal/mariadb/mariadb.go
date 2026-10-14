// Package mariadb is the MariaDB backend of ThothMemory Mini. It persists the
// shared memory domain with the same keys, gates and history guarantees as
// the PostgreSQL backend, using a deliberately small connection footprint.
package mariadb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/sqlstore"
	"github.com/go-sql-driver/mysql"
)

//go:embed schema/*.sql
var schema embed.FS

// Store is the shared SQL implementation with the MariaDB dialect.
type Store struct{ *sqlstore.Store }

var dialect = sqlstore.Dialect{
	ForUpdate:           " FOR UPDATE",
	NameCollation:       "name COLLATE utf8mb4_uca1400_ai_ci",
	ProjectByRepository: "repository_key=UNHEX(SHA2(?,256))",
	Time:                func(t time.Time) any { return t.UTC() },
	Wrap:                wrap,
	Duplicate: func(e error) bool {
		var me *mysql.MySQLError
		return errors.As(e, &me) && me.Number == 1062
	},
	WriteTx:    &sql.TxOptions{Isolation: sql.LevelReadCommitted},
	SnapshotTx: &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true},
}

// Config describes one database login. Socket takes precedence over Addr.
type Config struct {
	User, Password, Database, Socket, Addr string
}

func Open(ctx context.Context, c Config) (*Store, error) {
	m := mysql.NewConfig()
	m.User, m.Passwd, m.DBName = c.User, c.Password, c.Database
	if c.Socket != "" {
		m.Net, m.Addr = "unix", c.Socket
	} else {
		m.Net, m.Addr = "tcp", c.Addr
	}
	m.ParseTime, m.Loc, m.ClientFoundRows = true, time.UTC, true
	m.Collation = "utf8mb4_bin"
	// Client-side interpolation with utf8mb4 is one round trip per query
	// instead of prepare/execute/close; values are escaped by the driver.
	m.InterpolateParams = true
	m.Timeout, m.ReadTimeout, m.WriteTimeout = 5*time.Second, 30*time.Second, 30*time.Second
	m.Params = map[string]string{
		"time_zone":                "'+00:00'",
		"transaction_isolation":    "'READ-COMMITTED'",
		"sql_mode":                 "'STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'",
		"max_statement_time":       "10",
		"innodb_lock_wait_timeout": "10",
	}
	connector, e := mysql.NewConnector(m)
	if e != nil {
		return nil, e
	}
	db := sql.OpenDB(connector)
	// One server thread per connection: keep the pool small and let idle
	// connections close so an idle phone holds no extra MariaDB threads.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(2 * time.Minute)
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, wrap(e)
	}
	return &Store{&sqlstore.Store{DB: db, D: dialect}}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) Info() memory.Info {
	return memory.Info{Product: "ThothMemory Mini", Edition: "mini", Version: memory.MiniVersion, Database: "mariadb", Capabilities: map[string]bool{"english_stemming": false, "full_text_index": false, "substring_search": true, "dashboard_explorer": true, "export_import": true}}
}

func (s *Store) Call(ctx context.Context, p memory.Principal, tool string, a memory.Input) (any, error) {
	return memory.Call(ctx, s, p, tool, a)
}

// wrap hides driver messages (schema names, SQL fragments) behind a typed
// error while keeping the SQLSTATE for classification and retries.
func wrap(e error) error {
	var me *mysql.MySQLError
	if errors.As(e, &me) {
		// 1213 deadlock and 1205 lock wait timeout roll back; the caller may retry.
		return &memory.DatabaseError{State: string(me.SQLState[:]), Code: int(me.Number), Retryable: me.Number == 1213 || me.Number == 1205, Err: e}
	}
	return e
}

const migrationLock = "thothmemory_mini_migrate"

// Migrate applies embedded schema files. MariaDB DDL commits implicitly, so a
// migration is recorded as 'applying' before it runs and 'applied' after; an
// interrupted migration is refused instead of being silently re-run.
func (s *Store) Migrate(ctx context.Context) error {
	c, e := s.DB.Conn(ctx)
	if e != nil {
		return wrap(e)
	}
	defer c.Close()
	var got sql.NullInt64
	if e = c.QueryRowContext(ctx, "SELECT GET_LOCK(?,30)", migrationLock).Scan(&got); e != nil {
		return wrap(e)
	}
	if !got.Valid || got.Int64 != 1 {
		return errors.New("another migration holds the schema lock")
	}
	defer c.ExecContext(context.WithoutCancel(ctx), "SELECT RELEASE_LOCK(?)", migrationLock)
	if _, e = c.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version VARCHAR(100) NOT NULL PRIMARY KEY, checksum CHAR(64) NOT NULL, status VARCHAR(10) NOT NULL CHECK(status IN ('applying','applied')), applied_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6))) ENGINE=InnoDB DEFAULT CHARSET=ascii COLLATE=ascii_bin"); e != nil {
		return wrap(e)
	}
	files, e := schema.ReadDir("schema")
	if e != nil {
		return e
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
	for _, f := range files {
		b, _ := schema.ReadFile("schema/" + f.Name())
		h := sha256.Sum256(b)
		sum := hex.EncodeToString(h[:])
		var old, status string
		e = c.QueryRowContext(ctx, "SELECT checksum,status FROM schema_migrations WHERE version=?", f.Name()).Scan(&old, &status)
		switch {
		case e == nil && status != "applied":
			return fmt.Errorf("migration %s was interrupted; restore the pre-upgrade backup into a new volume", f.Name())
		case e == nil && old != sum:
			return errors.New("migration checksum mismatch")
		case e == nil:
			continue
		case !errors.Is(e, sql.ErrNoRows):
			return wrap(e)
		}
		if _, e = c.ExecContext(ctx, "INSERT INTO schema_migrations(version,checksum,status) VALUES(?,?,'applying')", f.Name(), sum); e != nil {
			return wrap(e)
		}
		for _, stmt := range statements(string(b)) {
			if _, e = c.ExecContext(ctx, stmt); e != nil {
				return fmt.Errorf("migration %s: %w", f.Name(), wrap(e))
			}
		}
		if _, e = c.ExecContext(ctx, "UPDATE schema_migrations SET status='applied',applied_at=UTC_TIMESTAMP(6) WHERE version=?", f.Name()); e != nil {
			return wrap(e)
		}
	}
	return nil
}

// statements splits a schema file on semicolons that end a line, skipping
// comment lines. Schema files never place such a semicolon inside a statement.
func statements(src string) []string {
	var out []string
	var cur strings.Builder
	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		cur.WriteString(line)
		cur.WriteString("\n")
		if strings.HasSuffix(t, ";") {
			out = append(out, strings.TrimSuffix(strings.TrimSpace(cur.String()), ";"))
			cur.Reset()
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, cur.String())
	}
	return out
}

func (s *Store) SchemaVersion(ctx context.Context) (string, error) {
	var v sql.NullString
	e := s.DB.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations WHERE status='applied'").Scan(&v)
	return v.String, wrap(e)
}

var safePassword = regexp.MustCompile(`^[A-Za-z0-9._~+/=-]{24,200}$`)

// GrantRuntime creates the least-privilege runtime login if it is missing:
// read everything, append history, update only mutable state. It cannot
// insert attestations or imports, which remain owner operations.
func (s *Store) GrantRuntime(ctx context.Context, user, password string) error {
	if !safePassword.MatchString(password) || !regexp.MustCompile(`^[a-z_]{1,32}$`).MatchString(user) {
		return errors.New("runtime credential must be 24..200 base64/url-safe characters")
	}
	var db string
	if e := s.DB.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&db); e != nil {
		return wrap(e)
	}
	q := "`" + strings.ReplaceAll(db, "`", "``") + "`"
	var exists bool
	if e := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM mysql.user WHERE user=? AND host='%')", user).Scan(&exists); e != nil {
		return wrap(e)
	}
	stmts := []string{}
	if !exists {
		stmts = append(stmts, "CREATE USER '"+user+"'@'%' IDENTIFIED BY '"+password+"'")
	}
	stmts = append(stmts, "GRANT SELECT ON "+q+".* TO '"+user+"'@'%'",
		"GRANT INSERT ON "+q+".principals TO '"+user+"'@'%'")
	for _, t := range []string{"projects", "agents", "sessions", "tasks", "task_dependencies", "evidence", "events", "records", "operations"} {
		stmts = append(stmts, "GRANT INSERT ON "+q+"."+t+" TO '"+user+"'@'%'")
	}
	for _, t := range []string{"projects", "sessions", "tasks"} {
		stmts = append(stmts, "GRANT UPDATE ON "+q+"."+t+" TO '"+user+"'@'%'")
	}
	for _, st := range stmts {
		if _, e := s.DB.ExecContext(ctx, st); e != nil {
			return wrap(e)
		}
	}
	return nil
}

// Bootstrap secures a freshly installed server: it sets the owner password,
// drops every account except the owner and MariaDB's internal mariadb.sys,
// and creates the database. It is idempotent.
func (s *Store) Bootstrap(ctx context.Context, db, password string) error {
	if !safePassword.MatchString(password) || !regexp.MustCompile(`^[a-z_]{1,32}$`).MatchString(db) {
		return errors.New("invalid bootstrap parameters")
	}
	var users []struct{ name, host string }
	r, e := s.DB.QueryContext(ctx, "SELECT user,host FROM mysql.global_priv")
	if e != nil {
		return wrap(e)
	}
	for r.Next() {
		var u struct{ name, host string }
		if e = r.Scan(&u.name, &u.host); e != nil {
			r.Close()
			return wrap(e)
		}
		users = append(users, u)
	}
	r.Close()
	if e != nil {
		return e
	}
	for _, u := range users {
		name, host := u.name, u.host
		if (name == "root" && host == "localhost") || name == "mariadb.sys" || name == "PUBLIC" {
			continue
		}
		if _, e = s.DB.ExecContext(ctx, "DROP USER ?@?", name, host); e != nil {
			return wrap(e)
		}
	}
	for _, st := range []string{"ALTER USER 'root'@'localhost' IDENTIFIED BY '" + password + "'", "CREATE DATABASE IF NOT EXISTS `" + db + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"} {
		if _, e = s.DB.ExecContext(ctx, st); e != nil {
			return wrap(e)
		}
	}
	return nil
}

// Status reports engine, schema, table storage and row counts.
func (s *Store) Status(ctx context.Context) (memory.Row, error) {
	var version string
	var bytes float64
	if e := s.DB.QueryRowContext(ctx, "SELECT VERSION(),(SELECT COALESCE(SUM(data_length+index_length),0) FROM information_schema.tables WHERE table_schema=DATABASE())").Scan(&version, &bytes); e != nil {
		return nil, wrap(e)
	}
	schemaVersion, e := s.SchemaVersion(ctx)
	if e != nil {
		return nil, e
	}
	counts, e := s.Counts(ctx)
	if e != nil {
		return nil, e
	}
	return memory.Row{"engine": "MariaDB " + version, "schema": schemaVersion, "database_bytes": bytes, "counts": counts}, nil
}
