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
	ExecFill // a deal has been made
	ExecCancel // order has been cancalled or fully filled
)

// inbound limit-order DTO with no GC-heavy fields
// **Price** and **Qnty** are expressed in ticks and minimal quantity to avoid floating-point error
type Order struct {
	Id           uint64 // global unique id
	InstrumentId string // trade pair ID
	Side         Side
	Price        uint64
	Qnty         uint64
	Timestamp    uint64
}

// single fill between taker and maker
type Trade struct {
	TakerId uint64
	MakerId uint64
	Price   uint64 
	Qnty    uint64 // **Trade.Qnty <= Order.Qnty**, allow partial filling
	Seq     uint64 // filled after node successfully replicate to other nodes
}

// --------------------------- Event Polymorphism -----------------------------------
type Event interface {
	ExecType() ExecStatus
	GetSeq() uint64
	setSeq(seq uint64)
}

type baseEvent struct{
	seq uint64
}

func (b *baseEvent) GetSeq() uint64 {return b.seq}
func (b *baseEvent) setSeq(seq uint64) {b.seq = seq}

// Order accept event
type OrderAccpetEvent struct{
	baseEvent
	ThisOrder Order
}

func (o *OrderAccpetEvent) ExecType() ExecStatus {return ExecAccept}

var _ Event = (*OrderAccpetEvent)(nil)

// Trade fill event ()
type TradeFillEvent struct{
	baseEvent
	ThisTrade Trade
}

func (t *TradeFillEvent) ExecType() ExecStatus {return ExecFill}

var _ Event = (*TradeFillEvent)(nil)

// Cancal event represents order is fully removed from book
type CancalEvent struct{
	baseEvent
	OrderId uint64
}

func (c *CancalEvent) ExecType() ExecStatus {return ExecCancel}

var _ Event = (*CancalEvent)(nil)