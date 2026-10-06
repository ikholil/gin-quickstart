package repository

import (
	"context"
	"strings"

	"gin-quickstart/internal/model"
)

func scanCategory(row interface{ Scan(...any) error }) (model.Category, error) {
	var category model.Category
	var active int
	var createdAt, updatedAt string
	if err := row.Scan(&category.ID, &category.Name, &active, &createdAt, &updatedAt); err != nil {
		return model.Category{}, mapError(err)
	}
	category.Active = active == 1
	var err error
	if category.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Category{}, err
	}
	if category.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Category{}, err
	}
	return category, nil
}

func categoryColumns() string {
	return `id, name, active, created_at, updated_at`
}

func (r *Repository) Category(ctx context.Context, id string, activeOnly bool) (model.Category, error) {
	query := `SELECT ` + categoryColumns() + ` FROM categories WHERE id = ?`
	if activeOnly {
		query += ` AND active = 1`
	}
	return scanCategory(r.db.QueryRowContext(ctx, query, id))
}

func (r *Repository) Categories(ctx context.Context, page, pageSize int) (model.Page[model.Category], error) {
	return r.categoryList(ctx, page, pageSize, true)
}

func (r *Repository) AdminCategories(ctx context.Context, page, pageSize int) (model.Page[model.Category], error) {
	return r.categoryList(ctx, page, pageSize, false)
}

func (r *Repository) categoryList(ctx context.Context, page, pageSize int, activeOnly bool) (model.Page[model.Category], error) {
	var result model.Page[model.Category]
	result.Page, result.PageSize = page, pageSize
	where := ``
	if activeOnly {
		where = ` WHERE active = 1`
	}
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM categories`+where).Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+categoryColumns()+`
		FROM categories`+where+` ORDER BY name COLLATE NOCASE LIMIT ? OFFSET ?`,
		pageSize, (page-1)*pageSize)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.Items = make([]model.Category, 0)
	for rows.Next() {
		category, err := scanCategory(rows)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, category)
	}
	return result, rows.Err()
}

func (r *Repository) CreateCategory(ctx context.Context, name string) (model.Category, error) {
	id, timestamp := newID(), now()
	_, err := r.db.ExecContext(ctx, `INSERT INTO categories (id, name, active, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?)`, id, name, timestamp, timestamp)
	if err != nil {
		return model.Category{}, mapError(err)
	}
	return r.Category(ctx, id, false)
}

func (r *Repository) UpdateCategory(ctx context.Context, id string, name *string, active *bool) (model.Category, error) {
	var nameValue, activeValue any
	if name != nil {
		nameValue = *name
	}
	if active != nil {
		activeValue = boolInt(*active)
	}
	result, err := r.db.ExecContext(ctx, `UPDATE categories SET name = COALESCE(?, name),
		active = COALESCE(?, active), updated_at = ? WHERE id = ?`, nameValue, activeValue, now(), id)
	if err != nil {
		return model.Category{}, mapError(err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return model.Category{}, err
	}
	if changed == 0 {
		return model.Category{}, ErrNotFound
	}
	return r.Category(ctx, id, false)
}

func scanProduct(row interface{ Scan(...any) error }) (model.Product, error) {
	var product model.Product
	var active int
	var createdAt, updatedAt string
	if err := row.Scan(&product.ID, &product.CategoryID, &product.Name, &product.Description,
		&product.PriceCents, &product.Stock, &active, &createdAt, &updatedAt); err != nil {
		return model.Product{}, mapError(err)
	}
	product.Active = active == 1
	var err error
	if product.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Product{}, err
	}
	if product.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Product{}, err
	}
	return product, nil
}

func productColumns() string {
	return `id, category_id, name, description, price_cents, stock, active, created_at, updated_at`
}

func (r *Repository) Product(ctx context.Context, id string, activeOnly bool) (model.Product, error) {
	query := `SELECT ` + productColumns() + ` FROM products WHERE id = ?`
	if activeOnly {
		query += ` AND active = 1 AND EXISTS (SELECT 1 FROM categories c WHERE c.id = products.category_id AND c.active = 1)`
	}
	return scanProduct(r.db.QueryRowContext(ctx, query, id))
}

func (r *Repository) Products(ctx context.Context, filter model.ProductFilter) (model.Page[model.Product], error) {
	return r.productList(ctx, filter, true)
}

func (r *Repository) AdminProducts(ctx context.Context, filter model.ProductFilter) (model.Page[model.Product], error) {
	return r.productList(ctx, filter, false)
}

func (r *Repository) productList(ctx context.Context, filter model.ProductFilter, activeOnly bool) (model.Page[model.Product], error) {
	var result model.Page[model.Product]
	result.Page, result.PageSize = filter.Page, filter.PageSize
	where := ` WHERE 1 = 1`
	if activeOnly {
		where += ` AND p.active = 1 AND c.active = 1`
	}
	args := make([]any, 0, 4)
	if filter.Query != "" {
		where += ` AND (p.name LIKE ? ESCAPE '\' OR p.description LIKE ? ESCAPE '\')`
		term := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(filter.Query) + "%"
		args = append(args, term, term)
	}
	if filter.CategoryID != "" {
		where += ` AND p.category_id = ?`
		args = append(args, filter.CategoryID)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM products p JOIN categories c ON c.id = p.category_id`+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	queryArgs := append(append([]any{}, args...), filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT p.`+strings.ReplaceAll(productColumns(), `, `, `, p.`)+`
		FROM products p JOIN categories c ON c.id = p.category_id`+where+`
		ORDER BY p.name COLLATE NOCASE LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.Items = make([]model.Product, 0)
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, product)
	}
	return result, rows.Err()
}

func (r *Repository) CreateProduct(ctx context.Context, product model.Product) (model.Product, error) {
	id, timestamp := newID(), now()
	_, err := r.db.ExecContext(ctx, `INSERT INTO products
		(id, category_id, name, description, price_cents, stock, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, product.CategoryID, product.Name, product.Description, product.PriceCents, product.Stock, timestamp, timestamp)
	if err != nil {
		return model.Product{}, mapError(err)
	}
	return r.Product(ctx, id, false)
}

func (r *Repository) UpdateProduct(ctx context.Context, id string, patch model.ProductPatch) (model.Product, error) {
	var activeValue any
	if patch.Active != nil {
		activeValue = boolInt(*patch.Active)
	}
	result, err := r.db.ExecContext(ctx, `UPDATE products SET category_id = COALESCE(?, category_id),
		name = COALESCE(?, name), description = COALESCE(?, description),
		price_cents = COALESCE(?, price_cents), stock = COALESCE(?, stock),
		active = COALESCE(?, active), updated_at = ? WHERE id = ?`,
		patch.CategoryID, patch.Name, patch.Description, patch.PriceCents, patch.Stock, activeValue, now(), id)
	if err != nil {
		return model.Product{}, mapError(err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return model.Product{}, err
	}
	if changed == 0 {
		return model.Product{}, ErrNotFound
	}
	return r.Product(ctx, id, false)
}
