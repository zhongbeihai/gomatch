package engine

import (
	"errors"
	"gomatch/types"
	"time"
)

// Engine encapsulates routing and execution loops.
type Engine struct {
	router   *Router
	outCh    chan []types.Event
	maxBatch int
}

// Opts configures the Engine.
type Opts struct {
    MinPx    int64
    MaxPx    int64
    Tick     int64
    OutBuffer int   // channel buffer size
    MaxBatch  int   
}

func NewEngine(opts Opts) *Engine {
    eng := &Engine{
        router: NewRouter(opts.MinPx, opts.MaxPx, opts.Tick),
        outCh:  make(chan []types.Event, opts.OutBuffer),
        maxBatch: opts.MaxBatch,
    }
    return eng
}

func (e *Engine) Submit(ord *types.Order) error {
	if ord == nil {
		return errors.New("order is nil")
	}

	b := e.router.getBook(ord.InstrumentId)
	// Exec synchronously per order; could batch if needed.
	event, err := b.ExecLimitPriceOrder(ord)
	if err != nil {
		return err
	}
	// non-blocking send
    select {
    case e.outCh <- event:
    default:
    }
    return nil
}

func (e *Engine) PollEvent(timeout time.Duration) [][]types.Event{
	var batches [][]types.Event
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for i := 0; i < e.maxBatch; i++ {
		select{
		case evts := <- e.outCh:
			batches = append(batches, evts)
		case <- timer.C:
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