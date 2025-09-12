package orders

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ItemInput struct {
	ProductID string `json:"product_id"`
	Qty       int    `json:"qty"`
}

type ItemInputSKU struct {
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}

type Repo struct{ DB *pgxpool.Pool }

var ErrAlreadyExists = errors.New("order already exists")

// CreateOrderTx: idempotent via external_id.
// - jika external_id sudah ada -> return existing order_id + total (existed=true).
func (r *Repo) CreateOrderTx(ctx context.Context, externalID, userID string, items []ItemInput) (orderID string, total int, existed bool, err error) {
	// cek existing by external_id
	row := r.DB.QueryRow(ctx, `SELECT id, total_cents FROM orders WHERE external_id=$1`, externalID)
	if err = row.Scan(&orderID, &total); err == nil {
		return orderID, total, true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, err
	}

	tx, err := r.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// hitung total berdasarkan price dari table products (hindari trust dari client)
	type priced struct {
		id    string
		price int
	}
	productIDs := make([]any, 0, len(items))
	params := ""
	for i, it := range items {
		if i > 0 {
			params += ","
		}
		params += fmt.Sprintf("$%d", i+1)
		productIDs = append(productIDs, it.ProductID)
	}
	rows, err := tx.Query(ctx, `SELECT id, price_cents FROM products WHERE id IN (`+params+`)`, productIDs...)
	if err != nil {
		return "", 0, false, err
	}
	prices := map[string]int{}
	for rows.Next() {
		var p priced
		if err := rows.Scan(&p.id, &p.price); err != nil {
			return "", 0, false, err
		}
		prices[p.id] = p.price
	}
	if err := rows.Err(); err != nil {
		return "", 0, false, err
	}

	for _, it := range items {
		price, ok := prices[it.ProductID]
		if !ok {
			return "", 0, false, fmt.Errorf("product not found: %s", it.ProductID)
		}
		if it.Qty <= 0 {
			return "", 0, false, fmt.Errorf("invalid qty for product %s", it.ProductID)
		}
		total += price * it.Qty
	}

	orderID = uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO orders(id, external_id, user_id, status, total_cents)
		VALUES ($1, $2, $3, 'CREATED', $4)
	`, orderID, externalID, userID, total)
	if err != nil {
		return "", 0, false, err
	}

	// insert items
	for _, it := range items {
		price := prices[it.ProductID]
		_, err = tx.Exec(ctx, `
			INSERT INTO order_items(order_id, product_id, qty, price_cents)
			VALUES ($1, $2, $3, $4)`,
			orderID, it.ProductID, it.Qty, price,
		)
		if err != nil {
			return "", 0, false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", 0, false, err
	}
	return orderID, total, false, nil
}

func (r *Repo) GetOrderStatus(ctx context.Context, orderID string) (Status, error) {
	var s string
	err := r.DB.QueryRow(ctx, `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&s)
	if err != nil {
		return "", err
	}
	return Status(s), nil
}

func (r *Repo) ListProducts(ctx context.Context) ([]Product, error) {
	rows, err := r.DB.Query(ctx, `SELECT id, sku, name, stock, price_cents, created_at, updated_at
                                FROM products ORDER BY sku`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.SKU, &p.Name, &p.Stock, &p.PriceCents, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) CreateOrderBySKU(ctx context.Context, externalID, userID string, items []ItemInputSKU) (orderID string, total int, existed bool, err error) {
	// cek existing
	row := r.DB.QueryRow(ctx, `SELECT id, total_cents FROM orders WHERE external_id=$1`, externalID)
	if err = row.Scan(&orderID, &total); err == nil {
		return orderID, total, true, nil
	}

	tx, err := r.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// ambil id & price dari sku
	skus := make([]any, 0, len(items))
	params := ""
	for i, it := range items {
		if i > 0 {
			params += ","
		}
		params += fmt.Sprintf("$%d", i+1)
		skus = append(skus, it.SKU)
	}
	rows, err := tx.Query(ctx, `SELECT id, sku, price_cents FROM products WHERE sku IN (`+params+`)`, skus...)
	if err != nil {
		return "", 0, false, err
	}
	type pp struct {
		id, sku string
		price   int
	}
	bySKU := map[string]pp{}
	for rows.Next() {
		var p pp
		if err := rows.Scan(&p.id, &p.sku, &p.price); err != nil {
			return "", 0, false, err
		}
		bySKU[p.sku] = p
	}
	if err := rows.Err(); err != nil {
		return "", 0, false, err
	}

	for _, it := range items {
		p, ok := bySKU[it.SKU]
		if !ok {
			return "", 0, false, fmt.Errorf("product not found: sku=%s", it.SKU)
		}
		if it.Qty <= 0 {
			return "", 0, false, fmt.Errorf("invalid qty for sku=%s", it.SKU)
		}
		total += p.price * it.Qty
	}

	orderID = uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO orders(id, external_id, user_id, status, total_cents)
	                          VALUES ($1,$2,$3,'CREATED',$4)`, orderID, externalID, userID, total); err != nil {
		return "", 0, false, err
	}
	for _, it := range items {
		p := bySKU[it.SKU]
		if _, err = tx.Exec(ctx, `INSERT INTO order_items(order_id, product_id, qty, price_cents)
		                          VALUES ($1,$2,$3,$4)`, orderID, p.id, it.Qty, p.price); err != nil {
			return "", 0, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", 0, false, err
	}
	return orderID, total, false, nil
}
