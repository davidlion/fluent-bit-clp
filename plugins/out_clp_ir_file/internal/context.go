package internal

import (
	"fmt"
	"github.com/y-scope/fluent-bit-clp/plugins/out_clp_ir_file/internal/compression"
	"github.com/y-scope/fluent-bit-clp/plugins/out_clp_ir_file/internal/flush"
	"github.com/y-scope/fluent-bit-clp/plugins/out_clp_ir_file/internal/s3"
	"log"
	"time"
	"unsafe"
)

type Context struct {
	Compression compression.Context
	S3          s3.Context
	Flush       flush.Manager
}

func NewContext(plugin unsafe.Pointer) (*Context, error) {
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
				log.Printf("[error] Flush failed because zstdWriter.Flush failed: %v", err)
			}

			if err := s3.UploadToS3(
				s3Ctx.Client, s3Ctx.Bucket,
				"/tmp/compressed-logs.clp.zstd",
				"/compressed-logs.clp.zst",
			); err != nil {
				log.Printf("[error] Failed to upload to S3")
			}
		},
	)
	if nil != err {
		return nil, fmt.Errorf("flush.NewManager: %w", err)
	}

	ctx := Context{
		Compression: *compressionCtx,
		S3:          *s3Ctx,
		Flush:       flushManager,
	}
	return &ctx, nil
}
