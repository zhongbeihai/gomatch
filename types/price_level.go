package types

// a linked list element inside PriceLevel
type OrderNode struct {
	Ord  Order
	Prev *OrderNode
	Next *OrderNode
}

// doubly linked list
// keep the sequence of resting orders in FIFO
// ** not thread-safe **
type PriceLevel struct {
	Head *OrderNode
	Tail *OrderNode
}

func (p *PriceLevel) Enqueue(n *OrderNode) {
	if p.Tail == nil {
		p.Head, p.Tail = n, n
		n.Prev, n.Next = nil, nil
		return
	}

	p.Tail.Next = n
	n.Prev = p.Tail
	n.Next = nil

	p.Tail = n
}

func (p *PriceLevel) Dequeue() *OrderNode {
	if p.Head == nil {
		return nil
	}

	h := p.Head
	if h.Next != nil {
		h.Next.Prev = nil
	} else {
		p.Tail = nil
	}

	p.Head = h.Next
	h.Next, h.Prev = nil, nil
	return h
}
