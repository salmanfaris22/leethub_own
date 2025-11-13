package database

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
)

// ConnectPostgres establishes a connection to the PostgreSQL database.
func ConnectPostgres() *pgx.Conn {
	connStr := "postgres://postgres:password@localhost:5432/exdex"
	conn, err := pgx.Connect(context.Background(), connStr)
	if err != nil {
		log.Fatalf("❌ Failed to connect to PostgreSQL: %v", err)
	}
	fmt.Println("✅ Connected to PostgreSQL")
	return conn
}

// AutoMigrate creates the exdex_token table if it does not exist.
func AutoMigrate(conn *pgx.Conn) {
	_, err := conn.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS exdex_token (
			id SERIAL PRIMARY KEY,
			base_currency TEXT NOT NULL,
			quote_currency TEXT NOT NULL,
			price NUMERIC NOT NULL,
			price_change NUMERIC DEFAULT 0,
			price_change_percent NUMERIC DEFAULT 0,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(base_currency, quote_currency)
		)
	`)
	if err != nil {
		log.Fatalf("❌ Failed to create table: %v", err)
	}
	fmt.Println("✅ Table ready: exdex_token")
}
