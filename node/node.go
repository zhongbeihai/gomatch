// -------------------------------------------------
// File: node/node.go
// Package: node
// -------------------------------------------------
// A minimal compilable framework for a "Matching Node", composed of:
//   • Net FrontEnd (TCP / placeholder for protocol parsing)
//   • Engine       (our previously implemented matching core)
//   • Raft         (interface-abstracted for easy replacement with etcd-raft / hashicorp-raft)
//   • Market Publisher (placeholder for asynchronous broadcasting)
//
// Notes:
//   1. This code passes `go vet` and `go test` (empty tests); actual network & Raft details need to be integrated with real libraries.
//   2. Uses a callback to deliver committed event batches from Raft back to `engine.Apply()`.
// -------------------------------------------------

package node

import (
	"context"
	"fmt"
	"gomatch/engine"
	"gomatch/types"
	"log"
	"net"
	"sync"
	"time"
)

// -------------------------------- Raft Interface --------------------------------
type RaftNode interface {
	// `Proprose` write `Event` batch into Raft log **(Called by leader)**
	Propose(ctx context.Context, batch []types.Event) error
	AddCommitListener(f func([]types.Event))
	IsLeader() bool
	LeaderAddr() string
	Shutdown(ctx context.Context) error
}

// --------------------------------Markert Publisher Interface --------------------------------
type MarketPublisher interface {
	Publish(evts []types.Event)
	Shutdown()
}

// -------------------------------- Node Options --------------------------------
type Options struct {
	EngineOpts engine.Opts
	ListenAddr string
	PollTick   time.Duration // the frequency of `PollEvent`
}

type Node struct {
	opts      Options
	engine    *engine.Engine
	raft      RaftNode
	publisher MarketPublisher
	listener  net.Listener
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// Consrtuction func of Node, require initialized RaftNode and MarketPublisher
func NewNode(opts Options, raft RaftNode, publi MarketPublisher) *Node {
	nctx, cancel := context.WithCancel(context.Background())
	nd := &Node{
		opts:      opts,
		engine:    engine.NewEngine(opts.EngineOpts),
		raft:      raft,
		publisher: publi,
		ctx:       nctx,
		cancel:    cancel,
	}
	nd.raft.AddCommitListener(nd.onRaftCommit)
	return nd
}

func (n *Node) Start() error{
	listener, err := net.Listen("tcp", n.opts.ListenAddr)
	if  err != nil {
		return err
	}
	n.listener = listener
	n.wg.Add(1)
	go n.acceptLoop()

	n.wg.Add(1)
	go n.proposeLoop()
	
	return nil
}

func (n *Node) acceptLoop() {
	defer n.wg.Done()
	for {
		conn, err := n.listener.Accept()
		if err != nil{
			select {
			case <- n.ctx.Done():
				return
			default:
				log.Println("accept error:", err)
				continue
			}
		}
		n.wg.Add(1)
		go n.handleConn(conn)
	}
}

// `handleConn“ parse ""id instr side price qty"."
func (n *Node) handleConn(c net.Conn) {
	defer n.wg.Done()
	defer c.Close()
	for {
		var (
			id          int64
			instr       string
			sideStr     string
			price, qnty int64
		)
		if _, err := fmt.Fscan(c, &id, &instr, &sideStr, &price, &qnty); err != nil{
			return
		}
		
		side := types.Buy
		if sideStr == "SELL" {
			side = types.Sell
		}
		ord := &types.Order{Id: id, InstrumentId: instr, Side: side, Price: price, Qnty: qnty}

		if !n.raft.IsLeader() {
			redir := fmt.Sprintf("REDIRECT %s\n", n.raft.LeaderAddr())
			c.Write([]byte(redir))
			continue
		}
		n.engine.Submit(n.ctx, ord)
		c.Write([]byte("ACK\n"))
	}
}

func (n *Node) proposeLoop() {
	defer n.wg.Done()

	ticker := time.NewTicker(n.opts.PollTick)
	defer ticker.Stop()

	for {
		select {
		case <- n.ctx.Done():
			return
		case <- ticker.C:
			if !n.raft.IsLeader() {continue}
			batches := n.engine.PollEvent(n.opts.PollTick)
			for _, batch := range batches{
				if err := n.raft.Propose(n.ctx, batch); err != nil{
					log.Println("raft propose err:", err)
				}
			}
		}
	}
}

func (n *Node) onRaftCommit(batch []types.Event) {
	for _, evt := range batch{
		n.engine.Apply(evt)
	}

	n.publisher.Publish(batch)
}

func (n *Node) Stop(ctx context.Context) error{
	n.cancel()
	n.listener.Close()
	n.raft.Shutdown(ctx)
	n.publisher.Shutdown()

	done := make(chan struct{})
	go func ()  {
		n.wg.Wait()
		close(done)
	}()

	select{
	case <- done:
		return nil
	case <- ctx.Done():
		return ctx.Err()
	}
}
