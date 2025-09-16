package orders

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReservationRepo struct{ DB *pgxpool.Pool }

// Cek apakah seluruh item utk order sudah RESERVED (idempotency short-circuit).
func (r *ReservationRepo) SudahReserved(ctx context.Context, orderID string, itemCount int) (bool, error) {
	var n int
	err := r.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM reservations
		WHERE order_id = $1 AND status = 'RESERVED'`, orderID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n == itemCount, nil
}

// ReserveAll: lock stok per product (FOR UPDATE) -> kurangi -> catat reservation (idempotent).
// Jika ada kekurangan pada salah satu item, tidak ada perubahan yg di-commit (rollback).
func (r *ReservationRepo) ReserveAll(ctx context.Context, orderID string, items []ItemQty) (ok bool, details []StockRejectedDetail, err error) {
	tx, err := r.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, nil, err
	}
	defer tx.Rollback(ctx)

	var rejects []StockRejectedDetail

	for _, it := range items {
		var stock int
		if err := tx.QueryRow(ctx, `SELECT stock FROM products WHERE id=$1 FOR UPDATE`, it.ProductID).Scan(&stock); err != nil {
			return false, nil, err
		}
		if stock < it.Qty {
			rejects = append(rejects, StockRejectedDetail{
				ProductID: it.ProductID, Required: it.Qty, Available: stock,
			})
			continue
		}

		ct, err := tx.Exec(ctx, `UPDATE products SET stock = stock - $2 WHERE id=$1`, it.ProductID, it.Qty)
		if err != nil {
			return false, nil, err
		}
		if ct.RowsAffected() != 1 {
			rejects = append(rejects, StockRejectedDetail{
				ProductID: it.ProductID, Required: it.Qty, Available: stock,
			})
			continue
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO reservations(order_id, product_id, qty, status)
			VALUES ($1,$2,$3,'RESERVED')
			ON CONFLICT (order_id, product_id) DO NOTHING
		`, orderID, it.ProductID, it.Qty); err != nil {
			return false, nil, err
		}
	}

	if len(rejects) > 0 {
		return false, rejects, nil // rollback via defer
	}
	if err := tx.Commit(ctx); err != nil {
		return false, nil, err
	}
	return true, nil, nil
}

func (r *ReservationRepo) ReleaseAll(ctx context.Context, orderID string) error {
	tx, err := r.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `SELECT product_id, qty FROM reservations WHERE order_id=$1 AND status='RESERVED'`, orderID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rec struct {
		pid string
		qty int
	}
	var recs []rec
	for rows.Next() {
		var x rec
		if err := rows.Scan(&x.pid, &x.qty); err != nil {
			return err
		}
		recs = append(recs, x)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, x := range recs {
		if _, err := tx.Exec(ctx, `UPDATE products SET stock = stock + $2 WHERE id=$1`, x.pid, x.qty); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE reservations SET status='RELEASED' WHERE order_id=$1 AND status='RESERVED'`, orderID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
