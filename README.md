# gomatch
Implemented high-performance order matching engine written in Go, designed to handle real-time trading operations for financial markets

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

