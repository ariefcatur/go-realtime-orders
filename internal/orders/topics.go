package orders

const (
	TopicOrderCreated      = "order.created"
	TopicStockReserved     = "order.stock.reserved"
	TopicStockRejected     = "order.stock.rejected"
	TopicPaymentAuthorized = "order.payment.authorized"
	TopicPaymentFailed     = "order.payment.failed"
	TopicOrderFinalized    = "order.finalized"
)

// Partition key = order_id, supaya semua event 1 order maintain urutan.
func PartitionKey(orderID string) []byte { return []byte(orderID) }
