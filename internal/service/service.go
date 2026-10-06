package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"gin-quickstart/internal/model"
	"gin-quickstart/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidInput       = errors.New("invalid input")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	countryCodePattern    = regexp.MustCompile(`^[A-Z]{2}$`)
)

type Service struct {
	repository *repository.Repository
	jwtSecret  []byte
	tokenTTL   time.Duration
}

func New(repository *repository.Repository, jwtSecret []byte, tokenTTL time.Duration) *Service {
	return &Service{repository: repository, jwtSecret: jwtSecret, tokenTTL: tokenTTL}
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) > 254 {
		return "", fmt.Errorf("%w: email is too long", ErrInvalidInput)
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || !strings.Contains(email, "@") {
		return "", fmt.Errorf("%w: invalid email", ErrInvalidInput)
	}
	return email, nil
}

func publicUser(user repository.UserCredentials) (model.User, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, user.CreatedAt)
	if err != nil {
		return model.User{}, err
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, user.UpdatedAt)
	if err != nil {
		return model.User{}, err
	}
	return model.User{ID: user.ID, Email: user.Email, Role: user.Role, CreatedAt: createdAt, UpdatedAt: updatedAt}, nil
}

func (s *Service) Register(ctx context.Context, email, password string) (model.User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return model.User{}, err
	}
	if len(password) < 12 || len(password) > 72 {
		return model.User{}, fmt.Errorf("%w: password must contain 12 to 72 bytes", ErrInvalidInput)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, err
	}
	id, err := s.repository.CreateUser(ctx, email, string(hash), "customer")
	if err != nil {
		return model.User{}, err
	}
	user, err := s.repository.UserByID(ctx, id)
	if err != nil {
		return model.User{}, err
	}
	return publicUser(user)
}

type LoginResult struct {
	AccessToken string     `json:"access_token"`
	TokenType   string     `json:"token_type"`
	ExpiresAt   time.Time  `json:"expires_at"`
	User        model.User `json:"user"`
}

type tokenClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	user, err := s.repository.UserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	public, err := publicUser(user)
	if err != nil {
		return LoginResult{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(s.tokenTTL)
	claims := tokenClaims{
		Role: user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: signed, TokenType: "Bearer", ExpiresAt: expiresAt, User: public}, nil
}

func (s *Service) ParseToken(raw string) (string, string, error) {
	claims := &tokenClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrUnauthorized
		}
		return s.jwtSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuedAt())
	if err != nil || !token.Valid || claims.Subject == "" ||
		(claims.Role != "customer" && claims.Role != "admin") ||
		claims.ExpiresAt == nil || claims.IssuedAt == nil {
		return "", "", ErrUnauthorized
	}
	return claims.Subject, claims.Role, nil
}

func (s *Service) EnsureAdmin(ctx context.Context, email, password string) error {
	if email == "" && password == "" {
		return nil
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	if len(password) < 12 || len(password) > 72 {
		return fmt.Errorf("%w: admin password must contain 12 to 72 bytes", ErrInvalidInput)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repository.EnsureAdmin(ctx, normalized, string(hash))
}

func (s *Service) Ping(ctx context.Context) error {
	return s.repository.Ping(ctx)
}

func (s *Service) Categories(ctx context.Context, page, pageSize int) (model.Page[model.Category], error) {
	return s.repository.Categories(ctx, page, pageSize)
}

func (s *Service) AdminCategories(ctx context.Context, page, pageSize int) (model.Page[model.Category], error) {
	return s.repository.AdminCategories(ctx, page, pageSize)
}

func (s *Service) Category(ctx context.Context, id string) (model.Category, error) {
	return s.repository.Category(ctx, id, true)
}

func (s *Service) AdminCategory(ctx context.Context, id string) (model.Category, error) {
	return s.repository.Category(ctx, id, false)
}

func (s *Service) CreateCategory(ctx context.Context, name string) (model.Category, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return model.Category{}, fmt.Errorf("%w: category name is required and must be at most 120 characters", ErrInvalidInput)
	}
	return s.repository.CreateCategory(ctx, name)
}

func (s *Service) UpdateCategory(ctx context.Context, id string, name *string, active *bool) (model.Category, error) {
	if name == nil && active == nil {
		return model.Category{}, fmt.Errorf("%w: no category fields to update", ErrInvalidInput)
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" || len(trimmed) > 120 {
			return model.Category{}, fmt.Errorf("%w: category name is required and must be at most 120 characters", ErrInvalidInput)
		}
		name = &trimmed
	}
	return s.repository.UpdateCategory(ctx, id, name, active)
}

func (s *Service) Products(ctx context.Context, filter model.ProductFilter) (model.Page[model.Product], error) {
	return s.repository.Products(ctx, filter)
}

func (s *Service) AdminProducts(ctx context.Context, filter model.ProductFilter) (model.Page[model.Product], error) {
	return s.repository.AdminProducts(ctx, filter)
}

func (s *Service) Product(ctx context.Context, id string) (model.Product, error) {
	return s.repository.Product(ctx, id, true)
}

func (s *Service) AdminProduct(ctx context.Context, id string) (model.Product, error) {
	return s.repository.Product(ctx, id, false)
}

func validateProduct(product model.Product) error {
	if strings.TrimSpace(product.CategoryID) == "" || strings.TrimSpace(product.Name) == "" ||
		len(product.Name) > 200 || len(product.Description) > 10000 ||
		product.PriceCents < 0 || product.Stock < 0 {
		return fmt.Errorf("%w: invalid product fields", ErrInvalidInput)
	}
	return nil
}

func (s *Service) CreateProduct(ctx context.Context, product model.Product) (model.Product, error) {
	product.CategoryID = strings.TrimSpace(product.CategoryID)
	product.Name = strings.TrimSpace(product.Name)
	product.Description = strings.TrimSpace(product.Description)
	if err := validateProduct(product); err != nil {
		return model.Product{}, err
	}
	category, err := s.repository.Category(ctx, product.CategoryID, true)
	if err != nil {
		return model.Product{}, err
	}
	if !category.Active {
		return model.Product{}, repository.ErrNotFound
	}
	return s.repository.CreateProduct(ctx, product)
}

func (s *Service) UpdateProduct(ctx context.Context, id string, patch model.ProductPatch) (model.Product, error) {
	if patch.CategoryID == nil && patch.Name == nil && patch.Description == nil &&
		patch.PriceCents == nil && patch.Stock == nil && patch.Active == nil {
		return model.Product{}, fmt.Errorf("%w: no product fields to update", ErrInvalidInput)
	}
	if patch.CategoryID != nil {
		value := strings.TrimSpace(*patch.CategoryID)
		if value == "" {
			return model.Product{}, fmt.Errorf("%w: category_id is required", ErrInvalidInput)
		}
		if _, err := s.repository.Category(ctx, value, true); err != nil {
			return model.Product{}, err
		}
		patch.CategoryID = &value
	}
	if patch.Name != nil {
		value := strings.TrimSpace(*patch.Name)
		if value == "" || len(value) > 200 {
			return model.Product{}, fmt.Errorf("%w: product name is required and must be at most 200 characters", ErrInvalidInput)
		}
		patch.Name = &value
	}
	if patch.Description != nil {
		value := strings.TrimSpace(*patch.Description)
		if len(value) > 10000 {
			return model.Product{}, fmt.Errorf("%w: product description is too long", ErrInvalidInput)
		}
		patch.Description = &value
	}
	if patch.PriceCents != nil && *patch.PriceCents < 0 {
		return model.Product{}, fmt.Errorf("%w: price_cents cannot be negative", ErrInvalidInput)
	}
	if patch.Stock != nil && *patch.Stock < 0 {
		return model.Product{}, fmt.Errorf("%w: stock cannot be negative", ErrInvalidInput)
	}
	return s.repository.UpdateProduct(ctx, id, patch)
}

func (s *Service) Cart(ctx context.Context, userID string) (model.Cart, error) {
	return s.repository.Cart(ctx, userID)
}

func (s *Service) AddCartItem(ctx context.Context, userID, productID string, quantity int64) error {
	if quantity <= 0 || productID == "" {
		return fmt.Errorf("%w: product_id and positive quantity are required", ErrInvalidInput)
	}
	return s.repository.AddCartItem(ctx, userID, productID, quantity)
}

func (s *Service) SetCartItem(ctx context.Context, userID, productID string, quantity int64) error {
	if quantity <= 0 || productID == "" {
		return fmt.Errorf("%w: positive quantity is required", ErrInvalidInput)
	}
	return s.repository.SetCartItem(ctx, userID, productID, quantity)
}

func (s *Service) RemoveCartItem(ctx context.Context, userID, productID string) error {
	if productID == "" {
		return fmt.Errorf("%w: product ID is required", ErrInvalidInput)
	}
	return s.repository.RemoveCartItem(ctx, userID, productID)
}

func (s *Service) Checkout(ctx context.Context, userID string, address model.Address) (model.Order, error) {
	address.RecipientName = strings.TrimSpace(address.RecipientName)
	address.Line1 = strings.TrimSpace(address.Line1)
	address.Line2 = strings.TrimSpace(address.Line2)
	address.City = strings.TrimSpace(address.City)
	address.Region = strings.TrimSpace(address.Region)
	address.PostalCode = strings.TrimSpace(address.PostalCode)
	address.CountryCode = strings.ToUpper(strings.TrimSpace(address.CountryCode))
	address.Phone = strings.TrimSpace(address.Phone)
	if address.RecipientName == "" || address.Line1 == "" || address.City == "" ||
		!countryCodePattern.MatchString(address.CountryCode) ||
		len(address.RecipientName) > 200 || len(address.Line1) > 300 ||
		len(address.Line2) > 300 || len(address.City) > 200 ||
		len(address.Region) > 200 || len(address.PostalCode) > 40 || len(address.Phone) > 40 {
		return model.Order{}, fmt.Errorf("%w: invalid shipping address", ErrInvalidInput)
	}
	return s.repository.Checkout(ctx, userID, address)
}

func (s *Service) Orders(ctx context.Context, ownerID string, page, pageSize int) (model.Page[model.Order], error) {
	return s.repository.Orders(ctx, ownerID, page, pageSize)
}

func (s *Service) Order(ctx context.Context, id, ownerID string) (model.Order, error) {
	if id == "" {
		return model.Order{}, fmt.Errorf("%w: order ID is required", ErrInvalidInput)
	}
	return s.repository.Order(ctx, id, ownerID)
}

func (s *Service) CancelOrder(ctx context.Context, id string) (model.Order, error) {
	if id == "" {
		return model.Order{}, fmt.Errorf("%w: order ID is required", ErrInvalidInput)
	}
	return s.repository.CancelOrder(ctx, id)
}
