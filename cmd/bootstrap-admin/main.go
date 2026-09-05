package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"ai-token/internal/config"
	"ai-token/internal/db"
	"ai-token/internal/ids"
	"ai-token/internal/mfa"
	"ai-token/internal/passwords"
)

var platformPermissions = []string{
	"tenant:read",
	"tenant:update",
	"member:invite",
	"member:remove",
	"project:read",
	"project:update",
	"token:create",
	"token:revoke",
	"usage:read",
	"billing:read",
	"billing:refund",
	"payment:read",
	"payment:update",
	"channel:read",
	"channel:update",
	"channel:read_secret",
	"group:read",
	"group:update",
	"token:read",
	"token:update",
	"price:publish",
	"price:read",
	"billing:update",
	"finance:read",
	"operations:read",
	"operations:update",
	"security:read",
	"security:update",
	"role:read",
	"role:update",
	"user:create",
	"user:read",
	"user:update",
	"user:freeze",
	"audit:read",
	"audit:export",
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	password := os.Getenv("ADMIN_PASSWORD")
	if email == "" || password == "" {
		log.Fatal("ADMIN_EMAIL and ADMIN_PASSWORD are required")
	}
	if len(cfg.MFAEncryptionKey) == 0 {
		log.Fatal("MFA_ENCRYPTION_KEY is required")
	}

	passwordHash, err := passwords.Hash(password)
	if err != nil {
		log.Fatal(err)
	}
	mfaSecret := strings.TrimSpace(os.Getenv("ADMIN_MFA_SECRET"))
	generatedMFASecret := mfaSecret == ""
	if generatedMFASecret {
		mfaSecret, err = mfa.NewSecret()
		if err != nil {
			log.Fatal(err)
		}
	}
	if _, err := mfa.Code(mfaSecret, time.Now(), 6); err != nil {
		log.Fatal("ADMIN_MFA_SECRET is not a valid base32 TOTP secret")
	}
	secretBox, err := mfa.NewSecretBox(cfg.MFAEncryptionKey)
	if err != nil {
		log.Fatal(err)
	}
	encryptedMFA, err := secretBox.Seal([]byte(mfaSecret))
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	if err := db.Migrate(ctx, conn, cfg.MigrationsDir); err != nil {
		log.Fatal(err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	userID, err := ids.New()
	if err != nil {
		log.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (
			id, email, password_hash, status, password_changed_at
		) VALUES ($1, $2, $3, 'active', now())
	`, userID, email, passwordHash); err != nil {
		log.Fatal(err)
	}

	roleID, err := ids.New()
	if err != nil {
		log.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO platform_roles (id, code, name, status)
		VALUES ($1, 'platform_owner', 'Platform Owner', 'active')
		ON CONFLICT (code) DO UPDATE SET status = 'active'
		RETURNING id::text
	`, roleID).Scan(&roleID); err != nil {
		log.Fatal(err)
	}

	for _, permission := range platformPermissions {
		parts := strings.SplitN(permission, ":", 2)
		permissionID, err := ids.New()
		if err != nil {
			log.Fatal(err)
		}
		var storedPermissionID string
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO platform_permissions (id, resource, action, name)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (resource, action) DO UPDATE SET name = EXCLUDED.name
			RETURNING id::text
		`, permissionID, parts[0], parts[1], permission).Scan(&storedPermissionID); err != nil {
			log.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO platform_role_permissions (role_id, permission_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, roleID, storedPermissionID); err != nil {
			log.Fatal(err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO platform_user_roles (user_id, role_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, userID, roleID); err != nil {
		log.Fatal(err)
	}

	mfaID, err := ids.New()
	if err != nil {
		log.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO mfa_credentials (
			id, user_id, type, label, status, encrypted_secret, verified_at
		) VALUES ($1, $2, 'totp', 'bootstrap-authenticator', 'active', $3, now())
	`, mfaID, userID, []byte(encryptedMFA)); err != nil {
		log.Fatal(err)
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	log.Printf("created platform admin %s", email)
	if generatedMFASecret {
		log.Printf("generated TOTP secret (store it securely; it will not be shown again): %s", mfaSecret)
	}
}
