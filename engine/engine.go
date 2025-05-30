package engine

import (
	"context"
	"fmt"
	"gomatch/types"
	"sync"
	"time"
)

// Engine encapsulates routing and execution loops.
type Engine struct {
	router    *Router
	outCh     chan []types.Event
	maxBatch  int
	mu        sync.Mutex
	inChans   map[string]chan *types.Order
	inBufSize int
}

// Opts configures the Engine.
type Opts struct {
	MinPx      int64
	MaxPx      int64
	Tick       int64
	OutBuffer  int // channel buffer size
	MaxBatch   int
	IntBufSize int
}

func NewEngine(opts Opts) *Engine {
	eng := &Engine{
		router:    NewRouter(opts.MinPx, opts.MaxPx, opts.Tick),
		outCh:     make(chan []types.Event, opts.OutBuffer),
		maxBatch:  opts.MaxBatch,
		inChans:   make(map[string]chan *types.Order),
		inBufSize: opts.IntBufSize,
	}
	return eng
}

func (e *Engine) Submit(ctx context.Context, ord *types.Order)  error{
    ch := e.getChan(ord.InstrumentId)
    select{
    case ch <- ord:
        return nil
    case <- ctx.Done():
        return fmt.Errorf("Order submission was cancelled or time out (%s): %w", ord.InstrumentId, ctx.Err())
    }
}

// getChan returns or creates shard channel and worker.
func (e *Engine) getChan(instr string) chan *types.Order {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ch, ok := e.inChans[instr]; ok {
		return ch
	}
	ch := make(chan *types.Order, e.inBufSize)
	e.inChans[instr] = ch
	go e.bookWorker(instr, ch)
	return ch
}

// bookWorker runs in its own goroutine per shard.
func (e *Engine) bookWorker(instr string, inCh <-chan *types.Order) {
	b := e.router.getBook(instr)
	for ord := range inCh {
		evts, _ := b.ExecLimitPriceOrder(ord)
		select {
		case e.outCh <- evts:
		default:
		}
	}
}

func (e *Engine) PollEvent(timeout time.Duration) [][]types.Event {
	var batches [][]types.Event
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for i := 0; i < e.maxBatch; i++ {
		select {
		case evts := <-e.outCh:
			batches = append(batches, evts)
		case <-timer.C:
			return batches
		}
	}

	return batches
}

// Apply replays an event for follower mode.
func (e *Engine) Apply(evt types.Event) {
	// cast and route apply
	switch ev := evt.(type) {
	case *types.OrderAccpetEvent:
		b := e.router.getBook(ev.ThisOrder.InstrumentId)
		b.Apply(ev)
	case *types.TradeFillEvent:
		// similar, book state updated by trade and cancel events following
		b := e.router.getBook("") // instr should be included in event if needed
		b.Apply(ev)
	case *types.CancalEvent:
		b := e.router.getBook("")
		b.Apply(ev)
	}
}