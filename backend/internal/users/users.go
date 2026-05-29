package users

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID          string
	ZitadelID   string
	Email       string
	DisplayName string
	AvatarURL   string
	Role        string
	Status      string
	APIKey      string
	CreatedAt   time.Time
}

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// GenerateAPIKey returns a cryptographically secure API key with 'sgpu_' prefix.
func GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sgpu_" + hex.EncodeToString(b), nil
}

// FindOrCreate finds an existing user by Zitadel ID or creates a new one.
// The first user ever created gets role='admin' and status='active'.
// All subsequent users get role='user' and status='pending'.
func (s *Store) FindOrCreate(ctx context.Context, zitadelID, email, displayName, avatarURL string) (*User, bool, error) {
	// Try to find existing user
	u, err := s.GetByZitadelID(ctx, zitadelID)
	if err == nil {
		return u, false, nil
	}

	// Generate API key
	apiKey, err := GenerateAPIKey()
	if err != nil {
		return nil, false, fmt.Errorf("generate api key: %w", err)
	}

	// Check if this is the very first user
	var count int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return nil, false, err
	}

	role := "user"
	userStatus := "pending"
	if count == 0 {
		role = "admin"
		userStatus = "active"
	}

	u = &User{}
	err = s.db.QueryRow(ctx, `
		INSERT INTO users (zitadel_id, email, display_name, avatar_url, role, status, api_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, zitadel_id, email, display_name, avatar_url, role, status, api_key, created_at`,
		zitadelID, email, displayName, avatarURL, role, userStatus, apiKey,
	).Scan(&u.ID, &u.ZitadelID, &u.Email, &u.DisplayName, &u.AvatarURL,
		&u.Role, &u.Status, &u.APIKey, &u.CreatedAt)
	if err != nil {
		return nil, false, fmt.Errorf("insert user: %w", err)
	}
	return u, true, nil
}

func (s *Store) GetByZitadelID(ctx context.Context, zitadelID string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(ctx, `
		SELECT id, zitadel_id, email, COALESCE(display_name,''), COALESCE(avatar_url,''),
		       role, status, api_key, created_at
		FROM users WHERE zitadel_id=$1`, zitadelID).
		Scan(&u.ID, &u.ZitadelID, &u.Email, &u.DisplayName, &u.AvatarURL,
			&u.Role, &u.Status, &u.APIKey, &u.CreatedAt)
	return u, err
}

func (s *Store) GetByID(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(ctx, `
		SELECT id, zitadel_id, email, COALESCE(display_name,''), COALESCE(avatar_url,''),
		       role, status, api_key, created_at
		FROM users WHERE id=$1`, id).
		Scan(&u.ID, &u.ZitadelID, &u.Email, &u.DisplayName, &u.AvatarURL,
			&u.Role, &u.Status, &u.APIKey, &u.CreatedAt)
	return u, err
}

func (s *Store) GetByAPIKey(ctx context.Context, apiKey string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(ctx, `
		SELECT id, zitadel_id, email, COALESCE(display_name,''), COALESCE(avatar_url,''),
		       role, status, api_key, created_at
		FROM users WHERE api_key=$1`, apiKey).
		Scan(&u.ID, &u.ZitadelID, &u.Email, &u.DisplayName, &u.AvatarURL,
			&u.Role, &u.Status, &u.APIKey, &u.CreatedAt)
	return u, err
}

func (s *Store) SetStatus(ctx context.Context, userID, newStatus string) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET status=$1 WHERE id=$2`, newStatus, userID)
	return err
}

func (s *Store) SetRole(ctx context.Context, userID, role string) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET role=$1 WHERE id=$2`, role, userID)
	return err
}

func (s *Store) ListPending(ctx context.Context) ([]*User, error) {
	return s.listWhere(ctx, `status='pending'`)
}

func (s *Store) ListAll(ctx context.Context) ([]*User, error) {
	return s.listWhere(ctx, `TRUE`)
}

func (s *Store) listWhere(ctx context.Context, cond string) ([]*User, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, zitadel_id, email, COALESCE(display_name,''), COALESCE(avatar_url,''),
		       role, status, api_key, created_at
		FROM users WHERE `+cond+` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.ZitadelID, &u.Email, &u.DisplayName, &u.AvatarURL,
			&u.Role, &u.Status, &u.APIKey, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// RegenerateAPIKey issues a new API key for the user.
func (s *Store) RegenerateAPIKey(ctx context.Context, userID string) (string, error) {
	apiKey, err := GenerateAPIKey()
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(ctx, `UPDATE users SET api_key=$1 WHERE id=$2`, apiKey, userID)
	return apiKey, err
}
