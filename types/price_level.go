package types

// a linked list element inside PriceLevel
type orderNode struct {
	ord  Order
	prev *orderNode
	next *orderNode
}

// doubly linked list
// keep the sequence of resting orders in FIFO
// ** not thread-safe **
type PriceLevel struct {
	head *orderNode
	tail *orderNode
}

func (p *PriceLevel) enquque(n *orderNode) {
	if p.tail == nil {
		p.head, p.tail = n, n
		n.prev, n.next = nil, nil
		return
	}

	p.tail.next = n
	n.prev = p.tail
	n.next = nil

	p.tail = n
}

func (p *PriceLevel) dequeue() *orderNode {
	if p.head == nil {
		return nil
	}

	h := p.head
	if h.next != nil {
		h.next.prev = nil
	}else{
		p.tail = nil
	}

	p.head = h.next
	h.next, h.prev = nil, nil
	return h
}
