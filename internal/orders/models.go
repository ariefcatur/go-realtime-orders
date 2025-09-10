package orders

import "time"

type Product struct {
	ID         string
	SKU        string
	Name       string
	Stock      int
	PriceCents int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Order struct {
	ID         string
	ExternalID string
	UserID     string
	Status     Status // lihat status.go
	TotalCents int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type OrderItem struct {
	ID         string
	OrderID    string
	ProductID  string
	Qty        int
	PriceCents int
}

type Reservation struct {
	ID        string
	OrderID   string
	ProductID string
	Qty       int
	Status    string // RESERVED | RELEASED
	CreatedAt time.Time
}
