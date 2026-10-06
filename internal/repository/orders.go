package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"gin-quickstart/internal/model"
)

func (r *Repository) Checkout(ctx context.Context, userID string, address model.Address) (model.Order, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Order{}, err
	}
	defer tx.Rollback()

	cartID, err := r.cartID(ctx, tx, userID)
	if errors.Is(err, ErrNotFound) {
		return model.Order{}, fmt.Errorf("%w: cart is empty", ErrConflict)
	}
	if err != nil {
		return model.Order{}, err
	}

	rows, err := tx.QueryContext(ctx, `SELECT p.id, p.name, ci.quantity, p.price_cents, p.stock, p.active, c.active
		FROM cart_items ci JOIN products p ON p.id = ci.product_id
		JOIN categories c ON c.id = p.category_id WHERE ci.cart_id = ? ORDER BY p.id`, cartID)
	if err != nil {
		return model.Order{}, err
	}
	type checkoutLine struct {
		item       model.OrderItem
		stock      int64
		productOn  int
		categoryOn int
	}
	lines := make([]checkoutLine, 0)
	for rows.Next() {
		var line checkoutLine
		if err := rows.Scan(&line.item.ProductID, &line.item.ProductName, &line.item.Quantity,
			&line.item.UnitPriceCents, &line.stock, &line.productOn, &line.categoryOn); err != nil {
			rows.Close()
			return model.Order{}, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return model.Order{}, err
	}
	if err := rows.Close(); err != nil {
		return model.Order{}, err
	}
	if len(lines) == 0 {
		return model.Order{}, fmt.Errorf("%w: cart is empty", ErrConflict)
	}

	total := int64(0)
	for i := range lines {
		line := &lines[i]
		if line.productOn != 1 || line.categoryOn != 1 {
			return model.Order{}, fmt.Errorf("%w: a cart product is inactive", ErrConflict)
		}
		if line.stock < line.item.Quantity {
			return model.Order{}, fmt.Errorf("%w: insufficient stock for %s", ErrConflict, line.item.ProductID)
		}
		lineTotal, err := lineTotal(line.item.UnitPriceCents, line.item.Quantity)
		if err != nil {
			return model.Order{}, err
		}
		line.item.LineTotalCents = lineTotal
		total, err = addTotal(total, lineTotal)
		if err != nil {
			return model.Order{}, err
		}
	}

	orderID, timestamp := newID(), now()
	_, err = tx.ExecContext(ctx, `INSERT INTO orders
		(id, user_id, state, total_cents, recipient_name, address_line1, address_line2, city, region,
		 postal_code, country_code, phone, created_at, updated_at)
		VALUES (?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		orderID, userID, total, address.RecipientName, address.Line1, address.Line2, address.City,
		address.Region, address.PostalCode, address.CountryCode, address.Phone, timestamp, timestamp)
	if err != nil {
		return model.Order{}, mapError(err)
	}
	for _, line := range lines {
		result, err := tx.ExecContext(ctx, `UPDATE products SET stock = stock - ?, updated_at = ?
			WHERE id = ? AND stock >= ? AND active = 1`, line.item.Quantity, timestamp,
			line.item.ProductID, line.item.Quantity)
		if err != nil {
			return model.Order{}, err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return model.Order{}, err
		}
		if changed != 1 {
			return model.Order{}, fmt.Errorf("%w: insufficient stock for %s", ErrConflict, line.item.ProductID)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO order_items
			(id, order_id, product_id, product_name, unit_price_cents, quantity, line_total_cents)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, newID(), orderID, line.item.ProductID, line.item.ProductName,
			line.item.UnitPriceCents, line.item.Quantity, line.item.LineTotalCents)
		if err != nil {
			return model.Order{}, mapError(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cart_items WHERE cart_id = ?`, cartID); err != nil {
		return model.Order{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Order{}, err
	}
	return r.Order(ctx, orderID, "")
}

func scanOrderHeader(row interface{ Scan(...any) error }) (model.Order, string, error) {
	var order model.Order
	var addressLine1, addressLine2, city, region, postalCode, countryCode, phone string
	var createdAt, updatedAt string
	if err := row.Scan(&order.ID, &order.CustomerID, &order.State, &order.TotalCents,
		&order.Address.RecipientName, &addressLine1, &addressLine2, &city, &region,
		&postalCode, &countryCode, &phone, &createdAt, &updatedAt); err != nil {
		return model.Order{}, "", mapError(err)
	}
	order.Address.Line1, order.Address.Line2 = addressLine1, addressLine2
	order.Address.City, order.Address.Region = city, region
	order.Address.PostalCode, order.Address.CountryCode, order.Address.Phone = postalCode, countryCode, phone
	var err error
	if order.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Order{}, "", err
	}
	if order.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Order{}, "", err
	}
	return order, order.ID, nil
}

const orderHeaderColumns = `id, user_id, state, total_cents, recipient_name, address_line1, address_line2,
	city, region, postal_code, country_code, phone, created_at, updated_at`

func (r *Repository) Order(ctx context.Context, id, ownerID string) (model.Order, error) {
	query := `SELECT ` + orderHeaderColumns + ` FROM orders WHERE id = ?`
	args := []any{id}
	if ownerID != "" {
		query += ` AND user_id = ?`
		args = append(args, ownerID)
	}
	order, _, err := scanOrderHeader(r.db.QueryRowContext(ctx, query, args...))
	if err != nil {
		return model.Order{}, err
	}
	order.Items, err = r.orderItems(ctx, r.db, id)
	return order, err
}

func (r *Repository) orderItems(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, orderID string) ([]model.OrderItem, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT product_id, product_name, quantity, unit_price_cents, line_total_cents
		FROM order_items WHERE order_id = ? ORDER BY id`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.OrderItem, 0)
	for rows.Next() {
		var item model.OrderItem
		if err := rows.Scan(&item.ProductID, &item.ProductName, &item.Quantity,
			&item.UnitPriceCents, &item.LineTotalCents); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Orders(ctx context.Context, ownerID string, page, pageSize int) (model.Page[model.Order], error) {
	var result model.Page[model.Order]
	result.Page, result.PageSize = page, pageSize
	where := ` FROM orders`
	args := []any{}
	if ownerID != "" {
		where += ` WHERE user_id = ?`
		args = append(args, ownerID)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT count(*)`+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	queryArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT `+orderHeaderColumns+where+
		` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	orders := make([]model.Order, 0)
	for rows.Next() {
		order, _, err := scanOrderHeader(rows)
		if err != nil {
			rows.Close()
			return result, err
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	result.Items = make([]model.Order, 0, len(orders))
	for _, order := range orders {
		order.Items, err = r.orderItems(ctx, r.db, order.ID)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, order)
	}
	return result, nil
}

func (r *Repository) CancelOrder(ctx context.Context, id string) (model.Order, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Order{}, err
	}
	defer tx.Rollback()
	var state string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM orders WHERE id = ?`, id).Scan(&state); err != nil {
		return model.Order{}, mapError(err)
	}
	if state != "pending" {
		return model.Order{}, fmt.Errorf("%w: only pending orders can be cancelled", ErrConflict)
	}
	rows, err := tx.QueryContext(ctx, `SELECT product_id, quantity FROM order_items WHERE order_id = ?`, id)
	if err != nil {
		return model.Order{}, err
	}
	type restockLine struct {
		productID string
		quantity  int64
	}
	lines := make([]restockLine, 0)
	for rows.Next() {
		var line restockLine
		if err := rows.Scan(&line.productID, &line.quantity); err != nil {
			rows.Close()
			return model.Order{}, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return model.Order{}, err
	}
	if err := rows.Close(); err != nil {
		return model.Order{}, err
	}
	if len(lines) == 0 {
		return model.Order{}, fmt.Errorf("%w: order has no lines", ErrConflict)
	}
	timestamp := now()
	result, err := tx.ExecContext(ctx, `UPDATE orders SET state = 'cancelled', updated_at = ?
		WHERE id = ? AND state = 'pending'`, timestamp, id)
	if err != nil {
		return model.Order{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return model.Order{}, err
	}
	if changed != 1 {
		return model.Order{}, fmt.Errorf("%w: only pending orders can be cancelled", ErrConflict)
	}
	for _, line := range lines {
		result, err := tx.ExecContext(ctx, `UPDATE products SET stock = stock + ?, updated_at = ?
			WHERE id = ? AND stock <= ? - ?`,
			line.quantity, timestamp, line.productID, int64(math.MaxInt64), line.quantity)
		if err != nil {
			return model.Order{}, err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return model.Order{}, err
		}
		if changed != 1 {
			return model.Order{}, fmt.Errorf("%w: stock restoration overflow", ErrConflict)
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Order{}, err
	}
	return r.Order(ctx, id, "")
}
