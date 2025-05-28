package types

// represent buy / sell direction
type Side uint8

const (
	Buy Side = iota
	Sell
)

type ExecStatus uint8

const (
	ExecAccept ExecStatus = iota // order has been accepted by the node
	ExecFill                     // a deal has been made
	ExecCancel                   // order has been cancalled or fully filled
)

// inbound limit-order DTO with no GC-heavy fields
// **Price** and **Qnty** are expressed in ticks and minimal quantity to avoid floating-point error
type Order struct {
	Id           int64  // global unique id
	InstrumentId string // instrument ID
	Side         Side
	Price        int64
	Qnty         int64
	Timestamp    int64
}

// single fill between taker and maker
type Trade struct {
	TakerId int64
	MakerId int64
	Price   int64
	Qnty    int64 // **Trade.Qnty <= Order.Qnty**, allow partial filling
	Seq     int64 // filled after node successfully replicate to other nodes
}

// --------------------------- Event Polymorphism -----------------------------------
type Event interface {
	ExecType() ExecStatus
	GetSeq() int64
	setSeq(seq int64)
}

type baseEvent struct {
	seq int64
}

func (b *baseEvent) GetSeq() int64    { return b.seq }
func (b *baseEvent) setSeq(seq int64) { b.seq = seq }

// Order accept event
type OrderAccpetEvent struct {
	baseEvent
	ThisOrder Order
}

func (o *OrderAccpetEvent) ExecType() ExecStatus { return ExecAccept }

var _ Event = (*OrderAccpetEvent)(nil)

// Trade fill event ()
type TradeFillEvent struct {
	baseEvent
	ThisTrade Trade
}

func (t *TradeFillEvent) ExecType() ExecStatus { return ExecFill }

var _ Event = (*TradeFillEvent)(nil)

// Cancal event represents order is fully removed from book
type CancalEvent struct {
	baseEvent
	OrderId int64
}

func (c *CancalEvent) ExecType() ExecStatus { return ExecCancel }

var _ Event = (*CancalEvent)(nil)
