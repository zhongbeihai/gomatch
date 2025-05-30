# Logics Explaination
## engine.Engine
1.  **Responsibilities of `bookWorker(instr string, inCh <-chan *types.Order)`**:
    * **Gets the corresponding order book**: Through `b := e.router.getBook(instr)`, it obtains the dedicated order book instance for the specific financial instrument (`instr`). This order book `b` is responsible for all order matching logic for that instrument.
    * **Continuously listens to the channel**: It runs in a `for ord := range inCh` loop, which continuously receives orders `ord` from its dedicated input channel `inCh`.
    * **Submits the order to the order book**: When an order `ord` is received from `inCh`, it calls `evts, _ := b.ExecLimitPriceOrder(ord)` to submit the order to the order book `b` for processing. After processing, the order book returns the resulting events (like trades, order confirmations, etc.).
    * **Forwards events**: The resulting events `evts` are then attempted to be sent to the engine's main output channel `e.outCh`. If `e.outCh` is full, the events are dropped (due to the `default` case in the `select` statement).

2.  **Responsibilities of `getChan(instr string) chan *types.Order`**:
    * **Gets or creates the channel**: It first checks if an input channel for the corresponding `instr` (trading pair/financial instrument) already exists in the `e.inChans` map.
        * If it exists, it directly returns this existing channel.
        * If it doesn't exist, it creates a new buffered channel `ch` (with buffer size `e.inBufSize`).
    * **Stores the new channel**: The newly created channel `ch` is stored in the `e.inChans` map with `instr` as the key.
    * **Starts a dedicated goroutine to run `bookWorker`**: Crucially, after creating a new channel, it immediately launches a new goroutine to run `go e.bookWorker(instr, ch)`. This `bookWorker` goroutine will be specifically responsible for processing orders from this newly created `ch` channel.
    * **Thread safety**: All these operations are protected by a mutex (`e.mu.Lock()` and `defer e.mu.Unlock()`), ensuring concurrent-safe access to the `e.inChans` map and the atomicity of the "check-and-create" logic.

3.  **Responsibilities of `Submit(ctx context.Context, ord *types.Order) error`**:
    * **Gets the target channel**: It first calls `ch := e.getChan(ord.InstrumentId)` to get or create the input channel `ch` corresponding to the `InstrumentId` of the order `ord`.
    * **Puts the order into the channel**: It then attempts to send the order `ord` to this channel `ch` (`case ch <- ord:`).
        * If the channel `ch` has space, the order is sent successfully, and `Submit` returns `nil`.
        * If the channel `ch` is full, causing the send operation to block, and the `ctx` (context) is canceled at this time (e.g., due to a timeout or an explicit cancellation by the caller), `Submit` will return an error indicating that the submission was canceled or timed out (`case <- ctx.Done():`).
    * **Separation of order processing**: As you mentioned, `Submit` is only responsible for placing the order into the correct "mailbox" (i.e., channel `ch`). The actual order processing work is done by the `bookWorker` running in **other goroutines**, which are precisely the ones listening to these channels.