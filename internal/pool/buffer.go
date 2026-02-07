package pool

import (
	"bytes"
	"io"
	"sync"
)

// BufferPool manages a pool of reusable byte buffers
// This reduces GC pressure and memory allocations significantly
type BufferPool struct {
	pool sync.Pool
	size int
}

// NewBufferPool creates a new buffer pool with specified default size
func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		size: size,
		pool: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, size))
			},
		},
	}
}

// Get retrieves a buffer from the pool
func (bp *BufferPool) Get() *bytes.Buffer {
	buf := bp.pool.Get().(*bytes.Buffer)
	buf.Reset() // Clear any existing data
	return buf
}

// Put returns a buffer to the pool for reuse
func (bp *BufferPool) Put(buf *bytes.Buffer) {
	// Don't return extremely large buffers to pool (memory leak prevention)
	if buf.Cap() > bp.size*10 {
		return // Let GC handle oversized buffers
	}
	bp.pool.Put(buf)
}

// ByteSlicePool manages a pool of reusable byte slices
type ByteSlicePool struct {
	pool sync.Pool
	size int
}

// NewByteSlicePool creates a new byte slice pool
func NewByteSlicePool(size int) *ByteSlicePool {
	return &ByteSlicePool{
		size: size,
		pool: sync.Pool{
			New: func() interface{} {
				slice := make([]byte, size)
				return &slice
			},
		},
	}
}

// Get retrieves a byte slice from the pool
func (bsp *ByteSlicePool) Get() []byte {
	slicePtr := bsp.pool.Get().(*[]byte)
	return (*slicePtr)[:bsp.size]
}

// Put returns a byte slice to the pool
func (bsp *ByteSlicePool) Put(slice []byte) {
	if cap(slice) < bsp.size || cap(slice) > bsp.size*2 {
		return // Don't pool wrong-sized slices
	}
	bsp.pool.Put(&slice)
}

// ReaderPool manages a pool of io.Reader wrappers
type ReaderPool struct {
	pool sync.Pool
}

// NewReaderPool creates a new reader pool
func NewReaderPool() *ReaderPool {
	return &ReaderPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &bytes.Reader{}
			},
		},
	}
}

// Get retrieves a reader from the pool and initializes it with data
func (rp *ReaderPool) Get(data []byte) *bytes.Reader {
	reader := rp.pool.Get().(*bytes.Reader)
	reader.Reset(data)
	return reader
}

// Put returns a reader to the pool
func (rp *ReaderPool) Put(reader *bytes.Reader) {
	rp.pool.Put(reader)
}

// WriterPool manages pooled writers
type WriterPool struct {
	pool sync.Pool
}

// NewWriterPool creates a new writer pool
func NewWriterPool() *WriterPool {
	return &WriterPool{
		pool: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, 4096))
			},
		},
	}
}

// Get retrieves a writer from the pool
func (wp *WriterPool) Get() io.Writer {
	return wp.pool.Get().(*bytes.Buffer)
}

// Put returns a writer to the pool
func (wp *WriterPool) Put(w io.Writer) {
	if buf, ok := w.(*bytes.Buffer); ok {
		buf.Reset()
		wp.pool.Put(buf)
	}
}

// Global pool instances for common use cases
var (
	// Small buffers (4KB) - for JSON, small responses
	SmallBufferPool = NewBufferPool(4 * 1024)

	// Medium buffers (64KB) - for file chunks, larger responses
	MediumBufferPool = NewBufferPool(64 * 1024)

	// Large buffers (1MB) - for video processing
	LargeBufferPool = NewBufferPool(1024 * 1024)

	// Byte slice pools
	SmallSlicePool  = NewByteSlicePool(4 * 1024)
	MediumSlicePool = NewByteSlicePool(64 * 1024)
	LargeSlicePool  = NewByteSlicePool(1024 * 1024)

	// Reader/Writer pools
	GlobalReaderPool = NewReaderPool()
	GlobalWriterPool = NewWriterPool()
)

// Stats returns pool utilization statistics
type PoolStats struct {
	SmallBuffersInUse  int
	MediumBuffersInUse int
	LargeBuffersInUse  int
}

// GetStats returns current pool statistics
// Note: sync.Pool doesn't expose internal stats, this is a placeholder
func GetStats() PoolStats {
	return PoolStats{
		// sync.Pool doesn't track in-use count
		// These would need custom tracking if needed
		SmallBuffersInUse:  0,
		MediumBuffersInUse: 0,
		LargeBuffersInUse:  0,
	}
}
