package main

import (
	"context"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"gin-quickstart/internal/database"
	"gin-quickstart/internal/repository"
)

func TestRequireDevelopment(t *testing.T) {
	if err := requireDevelopment("development"); err != nil {
		t.Fatalf("development environment rejected: %v", err)
	}
	for _, appEnv := range []string{"", "test", "production"} {
		if err := requireDevelopment(appEnv); err == nil {
			t.Errorf("APP_ENV %q should not be allowed to seed", appEnv)
		}
	}
}

func TestSeedIsIdempotentAndCreatesDemoCustomers(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "seed.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for range 2 {
		if err := seed(ctx, db); err != nil {
			t.Fatal(err)
		}
	}

	for _, check := range []struct {
		table string
		want  int
	}{
		{table: "categories", want: len(seedCategories)},
		{table: "products", want: len(seedProducts)},
		{table: "users", want: len(seedCustomers)},
	} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+check.table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != check.want {
			t.Errorf("%s count = %d, want %d", check.table, count, check.want)
		}
	}

	repo := repository.New(db)
	for _, demoCustomer := range seedCustomers {
		user, err := repo.UserByEmail(ctx, demoCustomer.email)
		if err != nil {
			t.Fatal(err)
		}
		if user.Role != "customer" {
			t.Errorf("demo account %q role = %q, want customer", demoCustomer.email, user.Role)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(demoCustomer.password)); err != nil {
			t.Errorf("demo password for %q does not match seeded hash", demoCustomer.email)
		}
	}
}
