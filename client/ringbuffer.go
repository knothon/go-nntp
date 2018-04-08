package nntpclient

func newRingBuffer(cap int) *ringBuffer {
	return &ringBuffer{
		Capacity: cap,
		Buffer:   make([]byte, cap, cap),
		position: 0,
		used:     0}
}

type ringBuffer struct {
	Capacity int
	Buffer   []byte
	position int
	used     int
}

func (b *ringBuffer) Write(buf []byte) {
	bufCount := len(buf)

	count := b.Capacity
	if count > bufCount {
		count = bufCount
	}
	start := bufCount - count

	for i := 0; i < count; i++ {
		bufferPos := (b.position + i) % b.Capacity
		b.Buffer[bufferPos] = buf[start+i]
		if b.used < b.Capacity {
			b.used++
		}
	}
}

func (b *ringBuffer) Equals(buf []byte) bool {
	if len(buf) != b.used {
		return false
	}

	for i := 0; i < b.used; i++ {
		bufferPos := (b.position + i) % b.Capacity
		if b.Buffer[bufferPos] != buf[i] {
			return false
		}
	}

	return true
}
