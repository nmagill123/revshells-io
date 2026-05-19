package broker

import "sync"

type RingBuffer struct {
	mu   sync.Mutex
	buf  [][]byte
	size int
	cap  int
	head int
	seq  int
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buf: make([][]byte, capacity),
		cap: capacity,
	}
}

func (r *RingBuffer) Write(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	r.buf[r.head] = cp
	r.head = (r.head + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
	r.seq++
}

func (r *RingBuffer) Seq() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seq
}

func (r *RingBuffer) Snapshot() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]byte, 0, r.size)
	start := 0
	if r.size == r.cap {
		start = r.head
	}
	for i := 0; i < r.size; i++ {
		idx := (start + i) % r.cap
		out = append(out, r.buf[idx])
	}
	return out
}
