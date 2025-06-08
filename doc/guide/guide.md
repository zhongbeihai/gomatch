# High Level Design
```text
                       +---------------------+
                       |     Client          |
                       |  (WS / REST / FIX)  |
                       +----------+----------+
                                  |
                                  v
                       +----------+----------+
                       |    API Gateway      |
                       | rate-limit, auth,   |
                       |  TLS offload        |
                       +----------+----------+
                                  |
                                  v
                       +----------+----------+
                       | Pre-Trade Risk Svc  |
                       |   (gRPC / NATS)     |
                       +----------+----------+
                                  |
                                  v
                       +----------+----------+
                       |    Order Router     |
                       | (instrument sharding)|
                       +----------+----------+
                                 / \
                                /   \
                               v     v
               +---------------+       +---------------+
               | Matching Eng A|       | Matching Eng Z|
               | (Raft leader +|       | (Raft leader +|
               |  followers)   |       |  followers)   |
               +-------+-------+       +-------+-------+
                       \                       /
                        \                     /
                         v                   v
                       +-------------------------+
                       |     Event Log           |
                       | (Kafka/Redpanda topic   |
                       |  per instrument)        |
                       +-----------+-------------+
                                   |
                    +--------------+--------------+
                    |                             |
                    v                             v
          +---------+---------+         +---------+---------+
          | Market-Data Fan-  |         |      Ledger       |
          |      Out          |         | (balances,settle, |
          |                    |         |     risk)         |
          +-------------------+         +-------------------+
```
# Module Dependency
- **types**  
  - Defines the core data structures and interfaces:  
    - `Order`, `Trade`, `Side`, `ExecType`  
    - The `Event` interface and its implementations (`OrderAccept`, `TradeEvent`, `CancelEvent`)

- **book**  
  - Imports **types**  
  - Implements a single-instrument order book:  
    - `Book.Exec(*types.Order) ([]types.Event, error)` – run price/time-priority matching  
    - `Book.Apply(types.Event)` – replay events for a follower  
    - Internal data: price‐indexed levels + FIFO linked lists

- **engine**  
  - Imports **book** and **types**  
  - Manages many `Book` instances and exposes a simple API:  
    - **Router**: `instr → *book.Book` map  
    - **Engine**:  
      - `Submit(*types.Order)` → routes to the right `Book` and calls `Exec`  
      - `PollEvents(timeout)` → collects pending event batches for Raft  
      - `Apply(types.Event)` → routes and replays events in follower mode

---

#Leader (Matching) Flow

![](./doc/img/1748464684737.jpg)
1. Submit
- `Engine.Submit(ord)`
- Routes to `Router.getBook(instr)` → `Book.Exec(ord)`
- Pushes the resulting `[]Event` into an internal channel
2. PollEvents
- `Engine.PollEvents(timeout)`
- Drains up to N batches of events for the Node to propose via Raft
3. Raft Commit & ACK

- Node calls `raft.Propose(batch)`
- On quorum commit, Node fills each event’s `.Seq`
- Sends ACK to the client and publishes trades to the market feed

---
# Reponsibilities of Leader and Folllower
## Leader Node Responsibilities
1. **Order Intake & Routing**  
   - Accepts client orders (via API Gateway / network front-end).  
   - Calls `Engine.Submit(order)` to route into the correct `Book`.

2. **Local Matching**  
   - Invokes `Book.Exec(order)` to perform price–time priority matching.  
   - Generates a batch of `Event`s (`OrderAccept`, `TradeEvent`, `CancelEvent`).

3. **Consensus & Durability**  
   - Calls `raft.Propose(eventBatch)` to replicate the batch to a Raft quorum.  
   - Waits for majority commit, then assigns each event’s `Seq` number.

4. **Client ACK & Market Broadcast**  
   - Sends ACK (including sequence numbers) back to the client.  
   - Publishes trades & depth deltas to the downstream market-data feed.

5. **Health & Leadership**  
   - Emits Raft heartbeats (`AppendEntries` with no new entries).  
   - Monitors follower liveness and steps down if superseded by higher term.

## Follower Node Responsibilities
1. **Raft Log Replication**  
   - Receives `AppendEntries` RPCs from the Leader containing committed event batches.  
   - Persists entries to local WAL and applies build-in Raft state machine.

2. **State Machine Replay**  
   - In Raft’s commit callback, calls `Engine.Apply(evt)` for each `Event`.  
   - `Engine.Apply` routes to the correct `Book` and invokes `Book.Apply(evt)` to update in-memory order-book.

3. **Read-Only Serving**  
   - Serves depth / best-bid/ask queries against its local `Book` without blocking the Leader.  
   - Supplies market-data subscribers with read snapshots or incremental deltas.

4. **Fast Failover**  
   - Participates in Raft elections when no leader heartbeat arrives.  
   - Upon winning, immediately begins accepting `Submit()` calls and executing matching logic.

5. **Background Tasks**  
   - Takes snapshots of the book state (`Book.Snapshot()`) for fast restart.  
   - Runs maintenance such as snapshotting, WAL compaction, and offline analytics without impacting the Leader.

---

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

## Node
A fully functional matching node (Node) must both host a local matching engine (Engine) and handle high-availability replication across nodes (Raft), while also providing external order and query interfaces. Below is an explanation of the Node design in English, covering its modules, data flow, and operational flow.

### 1. Core Modules

1. **Net FrontEnd**  
   - Receives client order/cancel requests (WebSocket / REST / FIX).  
   - Parses them into `types.Order` and forwards to the Engine or redirects to the current Leader.  

2. **Engine**  
   - Our previously implemented `engine.Engine`, which includes:  
     - **Router**: Maps `instr → Book`  
     - **Submit**: Sends the order to the corresponding `bookWorker` goroutine  
     - **PollEvents**: Collects event batches generated by each `Book`  

3. **Raft Node**  
   - Uses a mature Raft library (e.g., etcd-raft / HashiCorp Raft).  
   - **Leader**: Calls `raftNode.Propose(batch)` to write and replicate event batches fetched from the Engine into the log.  
   - **Follower**: In the `OnCommit` callback, reversely calls `Engine.Apply(evt)` to replay events.  

4. **Market Publisher**  
   - Broadcasts committed `TradeEvent`, `DepthDelta`, etc. to downstream consumers via WebSocket/UDP multicast.  

5. **Persistent Storage**  
   - Local WAL (Raft logs) + snapshot files for cold start and recovery.  
   - (Optional) Writes the same event stream to Kafka for use by other systems.  

6. **Health & Control Plane**  
   - Periodically reports metrics (matching latency, Raft status, channel lengths, etc.) to Prometheus.  
   - Exposes the current Leader address via etcd/Consul or Kubernetes API for use by gateways and monitoring systems.  


### 2. Procedure
```
[Client TCP Conn]
       |
       V
 [handleConn()] --(parse order)-->
       |
[Is Leader?] --> NO --> "REDIRECT to Leader"
       |
      YES
       |
 [engine.Submit(order)]
       |
       V
[proposeLoop()] --(poll & propose events)-->
       |
       V
  [Raft.Propose(batch)]  <-- cluster-wide consensus
       |
[onRaftCommit(batch)]
       |
[engine.Apply()] + [publisher.Publish()]
```

###
```
Client                Node(Leader)           Engine                Book                Raft
  |                       |                    |                    |                   |
  |---Order Request------->|                    |                    |                   |
  |                       |                    |                    |                   |
  |                handleConn                  |                    |                   |
  |                       |---Submit---------->|                    |                   |
  |                       |                    |---getChan--------->|                   |
  |                       |                    |   (start worker)   |                   |
  |                       |                    |---bookWorker------>|                   |
  |                       |                    |   (order)          |                   |
  |                       |                    |                    |---ExecLimitPriceOrder
  |                       |                    |                    |   (match, gen events)
  |                       |                    |<--events-----------|                   |
  |                       |                    |---outCh----------->|                   |
  |                       |                    |                    |                   |
  |                       |<--ACK--------------|                    |                   |
  |                       |                    |                    |                   |
  |                proposeLoop                 |                    |                   |
  |                       |---PollEvent------->|                    |                   |
  |                       |<--batches----------|                    |                   |
  |                       |---raft.Propose---->|                    |                   |
  |                       |                    |                    |                   |
  |                       |<--onRaftCommit-----|                    |                   |
  |                       |---engine.Apply---->|                    |                   |
  |                       |                    |---router.getBook-->|                   |
  |                       |                    |                    |---Book.Apply------|
  |                       |                    |                    |   (applyTrade/applyCancel)
  |                       |                    |                    |                   |
  |                       |---publisher.Publish|                    |                   |
  |                       |                    |                    |                   |
```

### 3. node.ProposeLoop
```
proposeLoop
   │
   ├─> Triggered every PollTick
   │
   ├─> engine.PollEvent(timeout)
   │      │
   │      └─> Collect event batches from matching engine
   │
   ├─> raft.Propose(batch)
   │      │
   │      └─> Raft log replication (asynchronously)
   │
   ├─> onRaftCommit(batch)   <-- (callback registered via AddCommitListener)
   │      │
   │      ├─> engine.Apply(evt) for each event in batch
   │      │
   │      └─> publisher.Publish(batch)
   │
   └─> Wait for next tick or ctx.Done()
```