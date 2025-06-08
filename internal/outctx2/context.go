package outctx2

import (
	"fmt"
	"github.com/y-scope/fluent-bit-clp/internal/compression"
	"github.com/y-scope/fluent-bit-clp/internal/s3"
	"log"
	"time"
	"unsafe"

	"github.com/y-scope/fluent-bit-clp/internal/flush"
)

type PluginCtx struct {
	Compression compression.Context
	S3          s3.Context
	Flush       flush.Manager
}

func NewPluginContext(plugin unsafe.Pointer) (*PluginCtx, error) {
	s3Ctx, err := s3.NewContext(plugin)
	if err != nil {
		log.Printf("[error] Failed to create s3 context")
	}

	compressionCtx, err := compression.NewContext(plugin)
	if err != nil {
		log.Printf("[error] Failed to create compression context")
	}

	// All the times are taken from:
	// https://github.com/y-scope/clp-loglib-py/blob/main/src/clp_logging/handlers.py#L185
	// TODO: update to use enum
	flushManager, err := flush.NewManager(
		[]time.Duration{
			3 * time.Second, // DEBUG
			3 * time.Second, // INFO
			3 * time.Second, // WARN
			3 * time.Second, // ERROR
			3 * time.Second, // FATAL
		},
		[]time.Duration{
			3 * time.Second, // DEBUG
			3 * time.Second, // INFO
			3 * time.Second, // WARN
			3 * time.Second, // ERROR
			3 * time.Second, // FATAL
		},
		0,
		func() {
			if err := compressionCtx.ZstdWriter.Flush(); err != nil {
				log.Printf("flush flush failed because zstdWriter.Flush failed: %v", err)
			}

			if err := s3.UploadToS3(
				s3Ctx.Client, s3Ctx.Bucket,
				"/tmp/compressed-logs.clp.zstd",
				"compressed-logs.clp.zst",
			); err != nil {
				log.Printf("Failed to upload to S3")
			}
		},
	)
	if nil != err {
		return nil, fmt.Errorf("flush.NewManager: %w", err)
	}

	ctx := PluginCtx{
		Compression: *compressionCtx,
		S3:          *s3Ctx,
		Flush:       flushManager,
	}
	return &ctx, nil
}
