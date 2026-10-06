package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	sqlite "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("resource not found")
	ErrConflict = errors.New("resource conflict")
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func newID() string {
	return uuid.NewString()
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == 19 {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse database timestamp: %w", err)
	}
	return parsed, nil
}

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash, role string) (string, error) {
	id, timestamp := newID(), now()
	_, err := r.db.ExecContext(ctx, `INSERT INTO users
		(id, email, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, email, passwordHash, role, timestamp, timestamp)
	if err != nil {
		return "", mapError(err)
	}
	return id, nil
}

type UserCredentials struct {
	ID           string
	Email        string
	Role         string
	PasswordHash string
	CreatedAt    string
	UpdatedAt    string
}

func (r *Repository) UserByEmail(ctx context.Context, email string) (UserCredentials, error) {
	var user UserCredentials
	err := r.db.QueryRowContext(ctx, `SELECT id, email, role, password_hash, created_at, updated_at
		FROM users WHERE email = ?`, email).Scan(
		&user.ID, &user.Email, &user.Role, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return UserCredentials{}, mapError(err)
	}
	return user, nil
}

func (r *Repository) UserByID(ctx context.Context, id string) (UserCredentials, error) {
	var user UserCredentials
	err := r.db.QueryRowContext(ctx, `SELECT id, email, role, password_hash, created_at, updated_at
		FROM users WHERE id = ?`, id).Scan(
		&user.ID, &user.Email, &user.Role, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return UserCredentials{}, mapError(err)
	}
	return user, nil
}

func (r *Repository) EnsureAdmin(ctx context.Context, email, passwordHash string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var role string
	err = tx.QueryRowContext(ctx, `SELECT role FROM users WHERE email = ?`, email).Scan(&role)
	switch {
	case err == nil:
		if role != "admin" {
			return fmt.Errorf("%w: bootstrap email belongs to a non-admin account", ErrConflict)
		}
		return tx.Commit()
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	timestamp := now()
	_, err = tx.ExecContext(ctx, `INSERT INTO users
		(id, email, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, 'admin', ?, ?)`,
		newID(), email, passwordHash, timestamp, timestamp)
	if err != nil {
		return mapError(err)
	}
	return tx.Commit()
}
