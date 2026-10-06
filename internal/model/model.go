package model

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Category struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Product struct {
	ID          string    `json:"id"`
	CategoryID  string    `json:"category_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PriceCents  int64     `json:"price_cents"`
	Stock       int64     `json:"stock"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProductPatch struct {
	CategoryID  *string `json:"category_id"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	PriceCents  *int64  `json:"price_cents"`
	Stock       *int64  `json:"stock"`
	Active      *bool   `json:"active"`
}

type CartItem struct {
	ProductID      string `json:"product_id"`
	Name           string `json:"name"`
	Quantity       int64  `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	LineTotalCents int64  `json:"line_total_cents"`
}

type Cart struct {
	ID         string     `json:"id"`
	Items      []CartItem `json:"items"`
	TotalCents int64      `json:"total_cents"`
}

type Address struct {
	RecipientName string `json:"recipient_name"`
	Line1         string `json:"line1"`
	Line2         string `json:"line2,omitempty"`
	City          string `json:"city"`
	Region        string `json:"region,omitempty"`
	PostalCode    string `json:"postal_code,omitempty"`
	CountryCode   string `json:"country_code"`
	Phone         string `json:"phone,omitempty"`
}

type OrderItem struct {
	ProductID      string `json:"product_id"`
	ProductName    string `json:"product_name"`
	Quantity       int64  `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	LineTotalCents int64  `json:"line_total_cents"`
}

type Order struct {
	ID         string      `json:"id"`
	CustomerID string      `json:"customer_id,omitempty"`
	State      string      `json:"state"`
	Items      []OrderItem `json:"items"`
	Address    Address     `json:"shipping_address"`
	TotalCents int64       `json:"total_cents"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

type Page[T any] struct {
	Items    []T   `json:"data"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

type ProductFilter struct {
	Query      string
	CategoryID string
	Page       int
	PageSize   int
}
