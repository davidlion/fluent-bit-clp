package outctx2

import (
	"fmt"
	"github.com/fluent/fluent-bit-go/output"
	"github.com/klauspost/compress/zstd"
	"github.com/y-scope/clp-ffi-go/ir"
	"log"
	"os"
	"unsafe"
)

type StreamingCompressionContext struct {
	File               *os.File
	ZstdWriter         *zstd.Encoder
	IRWriter           *ir.Writer
	LogBucket          string
	LastFlushTimestamp int64
}

func NewStreamingCompressionContext(plugin unsafe.Pointer) (*StreamingCompressionContext, error) {
	// Fetch plugin parameters
	logBucket := output.FLBPluginConfigKey(plugin, "log_bucket")
	log.Printf("The upload bucket for logs: " + logBucket)

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
		LogBucket:          logBucket,
		LastFlushTimestamp: 0,
	}

	return &ctx, nil
}
