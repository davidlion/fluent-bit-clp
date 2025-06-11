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

type IngestionContext struct {
	Compression *compression.CompressionContext
	Flush       *flush.FlushContext
}

type Context struct {
	S3        s3.S3Context
	Ingestion map[string]*IngestionContext
}

func GetIngestionContext(context *Context, path string) (*IngestionContext, error) {
	if ingestionContext, exists := context.Ingestion[path]; exists {
		return ingestionContext, nil
	}

	compressionCtx, err := compression.NewCompressionContext()
	if err != nil {
		log.Printf("[error] Failed to initialize compression context")
		return nil, err
	}

	// All the times are taken from:
	// https://github.com/y-scope/clp-loglib-py/blob/main/src/clp_logging/handlers.py#L185
	// TODO: update to use enum
	flushCtx, err := flush.NewFlushContext(
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
				context.S3.Client, context.S3.Bucket,
				compressionCtx.File.Name(),
				path+".clp.zst",
			); err != nil {
				log.Printf("[error] Failed to upload to S3")
			}
		},
	)

	if err != nil {
		log.Printf("[error] Fafiled to initialize flush context.")
		return nil, err
	}

	ingestionContext := &IngestionContext{
		Compression: compressionCtx,
		Flush:       flushCtx,
	}

	context.Ingestion[path] = ingestionContext

	return ingestionContext, nil
}

func NewContext(plugin unsafe.Pointer) (*Context, error) {
	s3Ctx, err := s3.NewS3Context(plugin)
	if err != nil {
		log.Printf("[error] Failed to create s3 context")
		return nil, fmt.Errorf("flush.NewCompressionContext: %w", err)
	}

	ctx := Context{
		S3:        *s3Ctx,
		Ingestion: make(map[string]*IngestionContext),
	}
	return &ctx, nil
}
