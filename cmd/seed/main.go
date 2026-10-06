package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gin-quickstart/internal/config"
	"gin-quickstart/internal/database"
	"gin-quickstart/internal/repository"
	"gin-quickstart/internal/service"
)

type category struct {
	id   string
	name string
}

type product struct {
	id          string
	categoryID  string
	name        string
	description string
	priceCents  int64
	stock       int64
}

type customer struct {
	id       string
	email    string
	password string
}

var seedCategories = []category{
	{id: "seed-category-books", name: "Books"},
	{id: "seed-category-electronics", name: "Electronics"},
	{id: "seed-category-accessories", name: "Accessories"},
}

var seedProducts = []product{
	{id: "seed-product-go-in-practice", categoryID: "seed-category-books", name: "Go in Practice", description: "A practical guide to building applications with Go.", priceCents: 3499, stock: 25},
	{id: "seed-product-clean-code", categoryID: "seed-category-books", name: "Clean Code", description: "A software craftsmanship reference.", priceCents: 4299, stock: 12},
	{id: "seed-product-the-pragmatic-programmer", categoryID: "seed-category-books", name: "The Pragmatic Programmer", description: "Tools and habits for effective software development.", priceCents: 3999, stock: 18},
	{id: "seed-product-usb-keyboard", categoryID: "seed-category-electronics", name: "USB Keyboard", description: "Compact wired keyboard with a standard layout.", priceCents: 2499, stock: 30},
	{id: "seed-product-wireless-mouse", categoryID: "seed-category-electronics", name: "Wireless Mouse", description: "Wireless mouse with adjustable sensitivity.", priceCents: 1999, stock: 24},
	{id: "seed-product-usb-c-hub", categoryID: "seed-category-electronics", name: "USB-C Hub", description: "Multi-port adapter for USB-C laptops.", priceCents: 4599, stock: 9},
	{id: "seed-product-webcam", categoryID: "seed-category-electronics", name: "HD Webcam", description: "USB webcam for video calls and streaming.", priceCents: 5299, stock: 7},
	{id: "seed-product-laptop-stand", categoryID: "seed-category-accessories", name: "Laptop Stand", description: "Adjustable aluminum desktop laptop stand.", priceCents: 3199, stock: 15},
	{id: "seed-product-desk-mat", categoryID: "seed-category-accessories", name: "Desk Mat", description: "Large non-slip desk mat.", priceCents: 1799, stock: 21},
	{id: "seed-product-usb-c-cable", categoryID: "seed-category-accessories", name: "USB-C Cable", description: "One-meter braided USB-C charging cable.", priceCents: 899, stock: 50},
	{id: "seed-product-headphones", categoryID: "seed-category-electronics", name: "Wired Headphones", description: "Over-ear wired headphones with a 3.5 mm plug.", priceCents: 2899, stock: 11},
	{id: "seed-product-notebook", categoryID: "seed-category-accessories", name: "Pocket Notebook", description: "Hardcover pocket notebook with ruled pages.", priceCents: 599, stock: 40},
}

var seedCustomers = []customer{
	{id: "seed-customer-alex", email: "alex@example.test", password: "demo-alex-password"},
	{id: "seed-customer-sam", email: "sam@example.test", password: "demo-sam-password"},
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if err := requireDevelopment(cfg.AppEnv); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	repo := repository.New(db)
	if err := service.New(repo, cfg.JWTSecret, cfg.JWTTTL).
		EnsureAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return fmt.Errorf("provision configured admin: %w", err)
	}
	if err := seed(ctx, db); err != nil {
		return err
	}
	log.Printf("development seed ready: %d categories, %d products, %d demo customers",
		len(seedCategories), len(seedProducts), len(seedCustomers))
	return nil
}

func requireDevelopment(appEnv string) error {
	if appEnv != "development" {
		return fmt.Errorf("refusing to seed: APP_ENV must be development (got %q)", appEnv)
	}
	return nil
}

func seed(ctx context.Context, db *sql.DB) error {
	hashes := make([]string, len(seedCustomers))
	for i, demoCustomer := range seedCustomers {
		hash, err := bcrypt.GenerateFromPassword([]byte(demoCustomer.password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash demo customer password: %w", err)
		}
		hashes[i] = string(hash)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)

	for _, item := range seedCategories {
		_, err := tx.ExecContext(ctx, `INSERT INTO categories
			(id, name, active, created_at, updated_at) VALUES (?, ?, 1, ?, ?)
			ON CONFLICT(id) DO UPDATE SET name = excluded.name, active = 1, updated_at = excluded.updated_at`,
			item.id, item.name, timestamp, timestamp)
		if err != nil {
			return fmt.Errorf("seed category %q: %w", item.name, err)
		}
	}
	for _, item := range seedProducts {
		_, err := tx.ExecContext(ctx, `INSERT INTO products
			(id, category_id, name, description, price_cents, stock, active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
			ON CONFLICT(id) DO UPDATE SET category_id = excluded.category_id, name = excluded.name,
				description = excluded.description, price_cents = excluded.price_cents,
				stock = excluded.stock, active = 1, updated_at = excluded.updated_at`,
			item.id, item.categoryID, item.name, item.description, item.priceCents, item.stock, timestamp, timestamp)
		if err != nil {
			return fmt.Errorf("seed product %q: %w", item.name, err)
		}
	}
	for i, item := range seedCustomers {
		result, err := tx.ExecContext(ctx, `INSERT INTO users
			(id, email, password_hash, role, created_at, updated_at)
			VALUES (?, ?, ?, 'customer', ?, ?)
			ON CONFLICT(id) DO UPDATE SET password_hash = excluded.password_hash, updated_at = excluded.updated_at
			WHERE users.role = 'customer' AND users.email = excluded.email`,
			item.id, item.email, hashes[i], timestamp, timestamp)
		if err != nil {
			return fmt.Errorf("seed customer %q: %w", item.email, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return fmt.Errorf("seed customer %q conflicts with an existing account", item.email)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit development seed: %w", err)
	}
	return nil
}
