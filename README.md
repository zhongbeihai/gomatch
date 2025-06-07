# gomatch
Implemented high-performance order matching engine written in Go, designed to handle real-time trading operations for financial markets

**For detailed design and module explanations, see [doc/guide/guide.md](doc/guide/guide.md)**
## Features

- High-performance, low-latency order matching engine
- Price-time priority matching logic (FIFO within price level)
- Supports multiple instruments with sharded order books
- Event-driven architecture: order acceptance, trade, and cancel events
- Designed for distributed deployment with leader/follower (Raft) architecture
- Asynchronous event publishing for market data feeds
- Clear modular design for easy extension and integration

## Functionality

- Accepts and matches limit orders (buy/sell) for multiple instruments
- Maintains price-level FIFO order books per instrument
- Generates and replays events for distributed consistency
- TCP-based network frontend for order intake (can be extended to REST/WS/FIX)
- Raft-based consensus for high availability and data consistency
- Market data publishing interface for downstream consumers

## Tech Stack

- **Go (Golang)**: Core language, leveraging goroutines and channels for concurrency
- **sync.Pool**: Object pooling to reduce GC overhead
- **Raft**: Distributed consensus (interface-abstracted, can use etcd-raft or HashiCorp Raft)
- **TCP Networking**: Order intake protocol (extensible)
- **Modular Structure**:
  - [`types`](types/model.go): Core data structures and event interfaces
  - [`book`](book/book.go): Single-instrument order book and matching logic
  - [`engine`](engine/engine.go): Multi-instrument routing and matching engine
  - [`node`](node/node.go): Node lifecycle, Raft integration, network frontend

## Directory Structure

- [`types/`](types/model.go): Core types and event definitions
- [`book/`](book/book.go): Order book and matching implementation
- [`engine/`](engine/engine.go): Engine and router logic
- [`node/`](node/node.go): Node management and distributed integration

## Usage

1. Clone the repository and set up your Go environment.
2. Implement the RaftNode and MarketPublisher interfaces as needed.
3. Start a matching node using [`node.Node`](node/node.go).



