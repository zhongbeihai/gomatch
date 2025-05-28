package book

import (
	"errors"
	"gomatch/types"
	"sync"
)

type Book struct {
	instrumentId string
	minPx        int64
	tick         int64
	levels       []*types.PriceLevel // by index: (price - minPx) / tick
	bestBid      int64               // index, -1 means no bid so far
	bestAsk      int64               // index, -1 means no ask so far

	onPool sync.Pool
}

func NewBook(instrumentId string, minPx, maxPx, tick int64) *Book {
	if maxPx <= minPx || tick <= 0 || (maxPx-minPx)%tick != 0 {
		panic("invalid price range")
	}

	size := (maxPx - minPx) / tick
	return &Book{
		instrumentId: instrumentId,
		minPx:        minPx,
		tick:         tick,
		// represent the order at each price level
		levels:  make([]*types.PriceLevel, size),
		bestBid: -1,
		bestAsk: -1,

		onPool: sync.Pool{New: func() any {
			return new(types.OrderNode)
		}},
	}
}

func (b *Book) ExecLimitPriceOrder(ord *types.Order) ([]types.Event, error) {
	// ---------------- validation ----------------
	if ord.Price < b.minPx || (ord.Price-b.minPx)%b.tick != 0 {
		return nil, errors.New("invalid price")
	}
	if ord.Qnty <= 0 {
		return nil, errors.New("qty <= 0")
	}

	events := make([]types.Event, 0, 8)
	events = append(events, &types.OrderAccpetEvent{ThisOrder: *ord})

	remain := ord.Qnty
	switch ord.Side {
	case types.Buy:
		for b.bestAsk >= 0 && ord.Price >= b.priceOfIdx(b.bestAsk) && remain > 0 {
			lvl := b.levels[b.bestAsk]
			// The place at bestAsk currently has no resting orders.
			if lvl == nil || lvl.Head == nil {
				b.advanceBestAsk()
				continue
			}

			maker := lvl.Dequeue()
			qntyProvidedByMaker := maker.Ord.Qnty
			minn := min(qntyProvidedByMaker, remain)
			remain -= minn
			maker.Ord.Qnty -= minn
			events = append(events, &types.TradeFillEvent{
				ThisTrade: types.Trade{
					TakerId: ord.Id,
					MakerId: maker.Ord.Id,
					Price:   b.priceOfIdx(b.bestAsk),
					Qnty:    minn,
				},
			})

			if maker.Ord.Qnty == 0 {
				events = append(events, &types.CancalEvent{OrderId: maker.Ord.Id})
				b.onPool.Put(maker)
			} else {
				lvl.Enqueue(maker)
			}
		}
	case types.Sell:
		for b.bestBid >= 0 && ord.Price <= b.priceOfIdx(b.bestBid) && remain > 0 {
			lvl := b.levels[b.bestBid]
			// The place at bestBid currently has no resting orders.
			if lvl == nil || lvl.Head == nil {
				b.advanceBestBid()
				continue
			}

			maker := lvl.Dequeue()
			qty := min(remain, maker.Ord.Qnty)
			remain -= qty
			maker.Ord.Qnty -= qty
			events = append(events, &types.TradeFillEvent{ThisTrade: types.Trade{
				TakerId: ord.Id,
				MakerId: maker.Ord.Id,
				Price:   b.priceOfIdx(b.bestBid),
				Qnty:    qty,
			}})
			if maker.Ord.Qnty == 0 {
				events = append(events, &types.CancalEvent{OrderId: maker.Ord.Id})
				b.onPool.Put(maker)
			} else {
				lvl.Enqueue(maker)
			}
		}
	default:
		return nil, errors.New("unknown side")
	}

	if remain > 0 { // remain quantity become passive order
		passiveIdx := b.idxOfPrice(ord.Price)
		node := b.onPool.Get().(*types.OrderNode)
		node.Ord = *ord

		if b.levels[passiveIdx] == nil {
			b.levels[passiveIdx] = &types.PriceLevel{}
		}
		b.levels[passiveIdx].Enqueue(node)
		if ord.Side == types.Buy {
			if b.bestBid < 0 || passiveIdx > b.bestBid {
				b.bestBid = passiveIdx
			}
		} else {
			if b.bestAsk < 0 || passiveIdx < b.bestAsk {
				b.bestAsk = passiveIdx
			}
		}
	}

	b.ensureBestPointers()
	return events, nil
}

// Apply replays a committed event onto the orderbook (Follower).
func (b *Book) Apply(evt types.Event) {
	switch e := evt.(type) {
	case *types.OrderAccpetEvent:
		// passive order insertion handled implicitly when Trade/Cancel events processed; nothing here.
		// Accept itself doesn't mutate book state for follower.
	case *types.TradeFillEvent:
		b.applyTrade(&e.ThisTrade)
	case *types.CancalEvent:
		b.applyCancel(uint64(e.OrderId))
	}
}

// --------------------------------------- internal helpers ----------------------------------------------

// index -> price
func (b *Book) priceOfIdx(idx int64) int64 {
	return b.minPx + idx*b.tick
}

func (b *Book) idxOfPrice(px int64) int64 {
	return (px - b.minPx) / b.tick
}

func (b *Book) advanceBestAsk() {
	for i := b.bestAsk; i >= 0; i-- {
		if lvl := b.levels[i]; lvl != nil && lvl.Head != nil {
			b.bestAsk = i
			return
		}
	}
	b.bestAsk = -1
}

func (b *Book) advanceBestBid() {
	for i := b.bestBid; i >= 0; i-- {
		if lvl := b.levels[i]; lvl != nil && lvl.Head != nil {
			b.bestBid = i
			return
		}
	}
	b.bestBid = -1
}

func (b *Book) ensureBestPointers() {
	if b.bestBid >= 0 {
		if lvl := b.levels[b.bestBid]; lvl == nil || lvl.Head == nil {
			b.advanceBestBid()
		}
	}
	if b.bestAsk >= 0 {
		if lvl := b.levels[b.bestAsk]; lvl == nil || lvl.Head == nil {
			b.advanceBestAsk()
		}
	}
}

func (b *Book) applyTrade(t *types.Trade) {
	// For follower replay: adjust order quantities; simplified implementation skips for brevity.
}

func (b *Book) applyCancel(id uint64) {
	// For follower replay: remove order id; simplified implementation skips.
}
