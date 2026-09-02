// Package postgres stores bank data in PostgreSQL.
//
// It exists for deployments that maintain the dataset centrally and run more
// than one instance of the service. The embedded and file backends remain the
// default, because they need nothing external to answer a request.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/netzfabrikcom/iban-pizza/bankdata"
)

// Store is a PostgreSQL backed repository.
type Store struct {
	pool *pgxpool.Pool
}

var (
	_ bankdata.Repository = (*Store)(nil)
	_ bankdata.Writer     = (*Store)(nil)
)

// schema creates the tables. It is applied on connect so that a fresh database
// needs no separate migration step.
const schema = `
CREATE TABLE IF NOT EXISTS bank_source (
    name         TEXT PRIMARY KEY,
    country      TEXT NOT NULL,
    url          TEXT NOT NULL DEFAULT '',
    retrieved_at TIMESTAMPTZ NOT NULL,
    record_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS bank (
    country     TEXT NOT NULL,
    bank_code   TEXT NOT NULL,
    name        TEXT NOT NULL,
    short_name  TEXT NOT NULL DEFAULT '',
    zip         TEXT NOT NULL DEFAULT '',
    city        TEXT NOT NULL DEFAULT '',
    bic         TEXT NOT NULL DEFAULT '',
    check_algo  TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL,
    PRIMARY KEY (country, bank_code)
);

CREATE INDEX IF NOT EXISTS bank_bic_idx ON bank (bic) WHERE bic <> '';
CREATE INDEX IF NOT EXISTS bank_source_idx ON bank (source);
`

// Open connects and applies the schema.
func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

const selectColumns = `country, bank_code, name, short_name, zip, city, bic, check_algo, source`

func scanBank(row pgx.Row) (bankdata.Bank, error) {
	var b bankdata.Bank
	err := row.Scan(&b.Country, &b.BankCode, &b.Name, &b.ShortName,
		&b.Zip, &b.City, &b.BIC, &b.CheckAlgo, &b.Source)
	return b, err
}

func (s *Store) Find(ctx context.Context, key bankdata.Key) (bankdata.Bank, error) {
	key = key.Normalize()
	row := s.pool.QueryRow(ctx,
		`SELECT `+selectColumns+` FROM bank WHERE country = $1 AND bank_code = $2`,
		key.Country, key.BankCode)

	b, err := scanBank(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bankdata.Bank{}, bankdata.ErrNotFound
	}
	return b, err
}

func (s *Store) FindByBIC(ctx context.Context, bic string) ([]bankdata.Bank, error) {
	bic = strings.ToUpper(strings.TrimSpace(bic))
	rows, err := s.pool.Query(ctx,
		`SELECT `+selectColumns+` FROM bank WHERE bic = $1 ORDER BY country, bank_code`, bic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []bankdata.Bank
	for rows.Next() {
		b, err := scanBank(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, bankdata.ErrNotFound
	}
	return out, nil
}

func (s *Store) Search(ctx context.Context, q bankdata.Query) ([]bankdata.Bank, error) {
	// Conditions are built with placeholders rather than interpolation, so a
	// search term cannot become part of the statement.
	var (
		where []string
		args  []any
	)
	add := func(cond string, arg any) {
		args = append(args, arg)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}

	if v := strings.ToUpper(strings.TrimSpace(q.Country)); v != "" {
		add("country = $%d", v)
	}
	if v := strings.ToUpper(strings.TrimSpace(q.BIC)); v != "" {
		add("bic LIKE $%d", v+"%")
	}
	if v := strings.TrimSpace(q.Name); v != "" {
		add("name ILIKE $%d", "%"+v+"%")
	}

	sql := `SELECT ` + selectColumns + ` FROM bank`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, clampLimit(q.Limit))
	sql += fmt.Sprintf(" ORDER BY country, bank_code LIMIT $%d", len(args))

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []bankdata.Bank{}
	for rows.Next() {
		b, err := scanBank(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func clampLimit(n int) int {
	switch {
	case n <= 0:
		return bankdata.DefaultLimit
	case n > bankdata.MaxLimit:
		return bankdata.MaxLimit
	default:
		return n
	}
}

func (s *Store) Stats(ctx context.Context) (bankdata.Stats, error) {
	var st bankdata.Stats
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM bank`).Scan(&st.TotalRecords); err != nil {
		return st, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT country, name, url, retrieved_at, record_count FROM bank_source ORDER BY country`)
	if err != nil {
		return st, err
	}
	defer rows.Close()

	for rows.Next() {
		var src bankdata.SourceInfo
		if err := rows.Scan(&src.Country, &src.Name, &src.URL, &src.RetrievedAt, &src.RecordCount); err != nil {
			return st, err
		}
		st.Sources = append(st.Sources, src)
	}
	return st, rows.Err()
}

// Replace swaps every record of one source inside a transaction, so readers
// never observe a partially loaded country.
func (s *Store) Replace(ctx context.Context, info bankdata.SourceInfo, banks []bankdata.Bank) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM bank WHERE source = $1`, info.Name); err != nil {
		return fmt.Errorf("clear source: %w", err)
	}

	rowsIn := make([][]any, 0, len(banks))
	for _, b := range banks {
		rowsIn = append(rowsIn, []any{
			strings.ToUpper(b.Country), b.BankCode, b.Name, b.ShortName,
			b.Zip, b.City, b.BIC, b.CheckAlgo, info.Name,
		})
	}
	if _, err := tx.CopyFrom(ctx,
		pgx.Identifier{"bank"},
		[]string{"country", "bank_code", "name", "short_name", "zip", "city", "bic", "check_algo", "source"},
		pgx.CopyFromRows(rowsIn),
	); err != nil {
		return fmt.Errorf("insert banks: %w", err)
	}

	if _, err := tx.Exec(ctx, `
        INSERT INTO bank_source (name, country, url, retrieved_at, record_count)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (name) DO UPDATE SET
            country = EXCLUDED.country,
            url = EXCLUDED.url,
            retrieved_at = EXCLUDED.retrieved_at,
            record_count = EXCLUDED.record_count`,
		info.Name, strings.ToUpper(info.Country), info.URL, info.RetrievedAt, len(banks),
	); err != nil {
		return fmt.Errorf("record source: %w", err)
	}

	return tx.Commit(ctx)
}
