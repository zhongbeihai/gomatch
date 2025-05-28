package engine

import (
	"gomatch/book"
	"hash/fnv"
	"log"
	"sync"
)

type Router struct {
	mu    sync.RWMutex
	books map[string]*book.Book
	minPx int64
	maxPx int64
	tick  int64
}

func NewRouter(minPx, maxPx, tick int64) *Router {
	if minPx < maxPx || tick < 0 {
		log.Print("invalid params")
		return nil
	}

	return &Router{
		books: make(map[string]*book.Book),
		minPx: minPx,
		maxPx: maxPx,
		tick:  tick,
	}
}

func (r *Router) getBook(instrumentId string) *book.Book {
	r.mu.RLock()
	b, ok := r.books[instrumentId]
	r.mu.RUnlock()
	if !ok {
		r.mu.Lock()
		defer r.mu.Unlock()

		if b, ok = r.books[instrumentId]; !ok {
			b = book.NewBook(instrumentId, r.minPx, r.maxPx, r.tick)
			r.books[instrumentId] = b
		}
	}

	return b
}

func hashKey(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32()
}
