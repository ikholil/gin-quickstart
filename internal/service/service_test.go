package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gin-quickstart/internal/database"
	"gin-quickstart/internal/model"
	"gin-quickstart/internal/repository"
	"gin-quickstart/internal/service"
)

func newService(t *testing.T) (*service.Service, *repository.Repository) {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	repo := repository.New(db)
	app := service.New(repo, []byte("0123456789abcdef0123456789abcdef"), time.Minute)
	return app, repo
}

func register(t *testing.T, app *service.Service, email string) model.User {
	t.Helper()
	user, err := app.Register(context.Background(), email, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func addProduct(t *testing.T, app *service.Service, price, stock int64) model.Product {
	t.Helper()
	category, err := app.CreateCategory(context.Background(), "Books")
	if err != nil {
		t.Fatal(err)
	}
	product, err := app.CreateProduct(context.Background(), model.Product{
		CategoryID: category.ID, Name: "Go in Practice", PriceCents: price, Stock: stock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return product
}

func address() model.Address {
	return model.Address{RecipientName: "Customer", Line1: "1 Main St", City: "Example", CountryCode: "us"}
}

func TestRegistrationLoginAndAdminBootstrap(t *testing.T) {
	app, _ := newService(t)
	ctx := context.Background()
	if err := app.EnsureAdmin(ctx, "Admin@Example.com", "a sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	if err := app.EnsureAdmin(ctx, "admin@example.com", "a different long password"); err != nil {
		t.Fatalf("bootstrap should be idempotent: %v", err)
	}
	adminLogin, err := app.Login(ctx, "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	if adminLogin.User.Role != "admin" || adminLogin.TokenType != "Bearer" {
		t.Fatalf("unexpected login response: %+v", adminLogin)
	}
	id, role, err := app.ParseToken(adminLogin.AccessToken)
	if err != nil || id != adminLogin.User.ID || role != "admin" {
		t.Fatalf("invalid issued token: id=%q role=%q err=%v", id, role, err)
	}

	user, err := app.Register(ctx, " Customer@Example.com ", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "customer@example.com" || user.Role != "customer" {
		t.Fatalf("unexpected registered user: %+v", user)
	}
	if _, err := app.Login(ctx, user.Email, "wrong password"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v", err)
	}
	if _, err := app.Register(ctx, "customer@example.com", "another correct horse"); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("duplicate email error = %v", err)
	}
	if _, err := app.Register(ctx, "invalid", "correct horse battery"); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("invalid email error = %v", err)
	}
}

func TestCheckoutSnapshotsPricesAndCancellationRestoresStockOnce(t *testing.T) {
	app, repo := newService(t)
	ctx := context.Background()
	customer := register(t, app, "customer@example.com")
	other := register(t, app, "other@example.com")
	product := addProduct(t, app, 1299, 9)

	if err := app.AddCartItem(ctx, customer.ID, product.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := app.AddCartItem(ctx, customer.ID, product.ID, 2); err != nil {
		t.Fatal(err)
	}
	cart, err := app.Cart(ctx, customer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 1 || cart.Items[0].Quantity != 3 || cart.TotalCents != 3897 {
		t.Fatalf("add should increment existing line: %+v", cart)
	}
	if err := app.SetCartItem(ctx, customer.ID, product.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Checkout(ctx, customer.ID, address()); err != nil {
		t.Fatal(err)
	}

	cart, err = app.Cart(ctx, customer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 0 || cart.TotalCents != 0 {
		t.Fatalf("cart was not cleared: %+v", cart)
	}
	orders, err := app.Orders(ctx, customer.ID, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(orders.Items) != 1 {
		t.Fatalf("got %d customer orders, want 1", len(orders.Items))
	}
	order := orders.Items[0]
	if order.State != "pending" || order.TotalCents != 2598 || order.Address.CountryCode != "US" ||
		len(order.Items) != 1 || order.Items[0].ProductName != "Go in Practice" {
		t.Fatalf("unexpected order snapshot: %+v", order)
	}
	if _, err := app.Order(ctx, order.ID, other.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("cross-customer order access error = %v", err)
	}
	storedProduct, err := repo.Product(ctx, product.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if storedProduct.Stock != 7 {
		t.Fatalf("checkout stock = %d, want 7", storedProduct.Stock)
	}

	cancelled, err := app.CancelOrder(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != "cancelled" {
		t.Fatalf("state = %q, want cancelled", cancelled.State)
	}
	storedProduct, err = repo.Product(ctx, product.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if storedProduct.Stock != 9 {
		t.Fatalf("restored stock = %d, want 9", storedProduct.Stock)
	}
	if _, err := app.CancelOrder(ctx, order.ID); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("repeated cancellation error = %v", err)
	}
	storedProduct, err = repo.Product(ctx, product.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if storedProduct.Stock != 9 {
		t.Fatalf("stock after repeated cancellation = %d, want 9", storedProduct.Stock)
	}
}

func TestCheckoutRejectsInsufficientStockWithoutChangingCartOrStock(t *testing.T) {
	app, repo := newService(t)
	ctx := context.Background()
	customer := register(t, app, "customer@example.com")
	product := addProduct(t, app, 500, 1)
	if err := app.AddCartItem(ctx, customer.ID, product.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Checkout(ctx, customer.ID, address()); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("checkout error = %v, want conflict", err)
	}
	stored, err := repo.Product(ctx, product.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Stock != 1 {
		t.Fatalf("stock = %d, want 1", stored.Stock)
	}
	cart, err := app.Cart(ctx, customer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 1 || cart.Items[0].Quantity != 2 {
		t.Fatalf("failed checkout changed cart: %+v", cart)
	}
	orders, err := app.Orders(ctx, customer.ID, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(orders.Items) != 0 {
		t.Fatalf("failed checkout created %d orders", len(orders.Items))
	}
}

func TestConcurrentCheckoutCannotOversell(t *testing.T) {
	app, repo := newService(t)
	ctx := context.Background()
	first := register(t, app, "first@example.com")
	second := register(t, app, "second@example.com")
	product := addProduct(t, app, 100, 1)
	for _, user := range []model.User{first, second} {
		if err := app.AddCartItem(ctx, user.ID, product.ID, 1); err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	var successes atomic.Int32
	var conflicts atomic.Int32
	var wg sync.WaitGroup
	for _, user := range []model.User{first, second} {
		wg.Add(1)
		go func(userID string) {
			defer wg.Done()
			<-start
			_, err := app.Checkout(ctx, userID, address())
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, repository.ErrConflict):
				conflicts.Add(1)
			default:
				t.Errorf("checkout error: %v", err)
			}
		}(user.ID)
	}
	close(start)
	wg.Wait()

	if successes.Load() != 1 || conflicts.Load() != 1 {
		t.Fatalf("successes=%d conflicts=%d, want one each", successes.Load(), conflicts.Load())
	}
	stored, err := repo.Product(ctx, product.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Stock != 0 {
		t.Fatalf("stock = %d, want 0", stored.Stock)
	}
	allOrders, err := app.Orders(ctx, "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(allOrders.Items) != 1 {
		t.Fatalf("got %d orders, want exactly 1", len(allOrders.Items))
	}
}

func TestArchivedResourcesCannotBeAddedToCart(t *testing.T) {
	app, _ := newService(t)
	ctx := context.Background()
	customer := register(t, app, "customer@example.com")
	product := addProduct(t, app, 100, 5)
	inactive := false
	if _, err := app.UpdateProduct(ctx, product.ID, model.ProductPatch{Active: &inactive}); err != nil {
		t.Fatal(err)
	}
	if err := app.AddCartItem(ctx, customer.ID, product.ID, 1); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("archived product add error = %v", err)
	}
}
