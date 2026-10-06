package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"gin-quickstart/internal/model"
)

func (r *Repository) cartID(ctx context.Context, tx *sql.Tx, userID string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM carts WHERE user_id = ?`, userID).Scan(&id)
	return id, mapError(err)
}

func (r *Repository) ensureCart(ctx context.Context, tx *sql.Tx, userID string) (string, error) {
	id, timestamp := newID(), now()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO carts (id, user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, id, userID, timestamp, timestamp); err != nil {
		return "", err
	}
	return r.cartID(ctx, tx, userID)
}

func (r *Repository) Cart(ctx context.Context, userID string) (model.Cart, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Cart{}, err
	}
	cartID, err := r.ensureCart(ctx, tx, userID)
	if err != nil {
		_ = tx.Rollback()
		return model.Cart{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Cart{}, err
	}

	cart := model.Cart{ID: cartID, Items: make([]model.CartItem, 0)}
	rows, err := r.db.QueryContext(ctx, `SELECT ci.product_id, p.name, ci.quantity, p.price_cents
		FROM cart_items ci JOIN products p ON p.id = ci.product_id
		WHERE ci.cart_id = ? ORDER BY p.name COLLATE NOCASE`, cartID)
	if err != nil {
		return model.Cart{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item model.CartItem
		if err := rows.Scan(&item.ProductID, &item.Name, &item.Quantity, &item.UnitPriceCents); err != nil {
			return model.Cart{}, err
		}
		lineTotal, err := lineTotal(item.UnitPriceCents, item.Quantity)
		if err != nil {
			return model.Cart{}, err
		}
		item.LineTotalCents = lineTotal
		cart.TotalCents, err = addTotal(cart.TotalCents, lineTotal)
		if err != nil {
			return model.Cart{}, err
		}
		cart.Items = append(cart.Items, item)
	}
	if err := rows.Err(); err != nil {
		return model.Cart{}, err
	}
	return cart, nil
}

func (r *Repository) AddCartItem(ctx context.Context, userID, productID string, quantity int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cartID, err := r.ensureCart(ctx, tx, userID)
	if err != nil {
		return err
	}
	var active int
	err = tx.QueryRowContext(ctx, `SELECT p.active AND c.active FROM products p
		JOIN categories c ON c.id = p.category_id WHERE p.id = ?`, productID).Scan(&active)
	if err != nil {
		return mapError(err)
	}
	if active != 1 {
		return ErrNotFound
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO cart_items (cart_id, product_id, quantity) VALUES (?, ?, ?)
		ON CONFLICT(cart_id, product_id) DO UPDATE SET quantity = cart_items.quantity + excluded.quantity
		WHERE cart_items.quantity <= ? - excluded.quantity`, cartID, productID, quantity, int64(math.MaxInt64))
	if err != nil {
		return mapError(err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return fmt.Errorf("%w: cart quantity overflow", ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE carts SET updated_at = ? WHERE id = ?`, now(), cartID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) SetCartItem(ctx context.Context, userID, productID string, quantity int64) error {
	result, err := r.db.ExecContext(ctx, `UPDATE cart_items SET quantity = ?
		WHERE product_id = ? AND cart_id = (SELECT id FROM carts WHERE user_id = ?)`,
		quantity, productID, userID)
	if err != nil {
		return mapError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	_, err = r.db.ExecContext(ctx, `UPDATE carts SET updated_at = ? WHERE user_id = ?`, now(), userID)
	return err
}

func (r *Repository) RemoveCartItem(ctx context.Context, userID, productID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM cart_items
		WHERE product_id = ? AND cart_id = (SELECT id FROM carts WHERE user_id = ?)`,
		productID, userID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	_, err = r.db.ExecContext(ctx, `UPDATE carts SET updated_at = ? WHERE user_id = ?`, now(), userID)
	return err
}

func lineTotal(price, quantity int64) (int64, error) {
	if price < 0 || quantity <= 0 || (price != 0 && quantity > math.MaxInt64/price) {
		return 0, fmt.Errorf("%w: line total overflow", ErrConflict)
	}
	return price * quantity, nil
}

func addTotal(total, amount int64) (int64, error) {
	if amount < 0 || total > math.MaxInt64-amount {
		return 0, fmt.Errorf("%w: order total overflow", ErrConflict)
	}
	return total + amount, nil
}
