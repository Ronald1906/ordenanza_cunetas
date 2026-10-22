package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Begin(context.Context) (pgx.Tx, error)
}

func ConnectDB(url string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("configuración PostgreSQL: %w", err)
	}
	config.ConnConfig.RuntimeParams["timezone"] = "America/Guayaquil"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
func EnsureAdmin(db *pgxpool.Pool) error {
	username := strings.TrimSpace(os.Getenv("ADMIN_USER"))
	password := os.Getenv("ADMIN_PASSWORD")
	if username == "" || password == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var exists bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM usuario WHERE usuario=$1)`, username).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO usuario(usuario,password_hash,nombres) VALUES($1,$2,'ADMINISTRADOR') ON CONFLICT(usuario) DO NOTHING`, username, string(hash))
	return err
}
