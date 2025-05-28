# gomatch
Implemented high-performance order matching engine written in Go, designed to handle real-time trading operations for financial markets

# High-level design
```
┌──────────────────────────────────┐
│  合约分片节点 Node （单进程）     │
│                                  │
│   ┌──────────────┐               │
│   │ Net FrontEnd │←──客户端订单   │
│   └──────┬───────┘               │
│          ▼                       │
│   ┌──────────────┐               │
│   │ Ingress 队列 │  SPSC RingBuf │
│   └──────┬───────┘               │
│          ▼                       │
│   ┌──────────────┐  Raft 提案    │
│   │ Raft Engine  │◀─集群心跳╱复制│
│   └──────┬───────┘               │
│          ▼                       │
│   ┌──────────────┐               │
│   │ StateMachine │  Leader:撮合  │
│   │  (OrderBook) │  Follower:回放│
│   └──────┬───────┘               │
│   │  WAL Writer  │ NVMe 顺序写   │
│   └──────┬───────┘               │
│          ▼                       │
│   ┌──────────────┐               │
│   │ Snapshot Mgr │ S3/本地快照   │
│   └──────┬───────┘               │
│          ▼                       │
│   ┌──────────────┐               │
│   │ Market Pub   │ WebSocket/UDP │
│   └──────────────┘               │
└──────────────────────────────────┘
```