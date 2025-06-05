package outctx2

import (
	"fmt"
	"github.com/klauspost/compress/zstd"
	"github.com/y-scope/clp-ffi-go/ir"
	"os"
	"unsafe"
)

type StreamingCompressionContext struct {
	File               *os.File
	ZstdWriter         *zstd.Encoder
	IRWriter           *ir.Writer
	LastFlushTimestamp int64
}

func NewStreamingCompressionContext(plugin unsafe.Pointer) (*StreamingCompressionContext, error) {
	file, err := os.OpenFile("/tmp/compressed-logs.clp.zstd", os.O_WRONLY|os.O_CREATE, 0660)
	if nil != err {
		return nil, fmt.Errorf("os.Create: %v", err)
	}

	zstdWriter, err := zstd.NewWriter(file)
	if nil != err {
		return nil, fmt.Errorf("zstd.NewWriter: %v", err)
	}
	irWriter, err := ir.NewWriter[ir.FourByteEncoding](zstdWriter)

	ctx := StreamingCompressionContext{
		File:               file,
		ZstdWriter:         zstdWriter,
		IRWriter:           irWriter,
		LastFlushTimestamp: 0,
	}

	return &ctx, nil
}
