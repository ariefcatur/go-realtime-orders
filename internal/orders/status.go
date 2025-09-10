package orders

type Status string

const (
	StatusCreated       Status = "CREATED"
	StatusStockReserved Status = "STOCK_RESERVED"
	StatusPaid          Status = "PAID"
	StatusCompleted     Status = "COMPLETED"
	StatusFailed        Status = "FAILED"
)

var validNext = map[Status]map[Status]bool{
	StatusCreated:       {StatusStockReserved: true, StatusFailed: true},
	StatusStockReserved: {StatusPaid: true, StatusFailed: true},
	StatusPaid:          {StatusCompleted: true},
	StatusCompleted:     {},
	StatusFailed:        {},
}

func CanTransition(from, to Status) bool {
	return validNext[from][to]
}
