package internal

import (
	"log"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/fluent/fluent-bit-go/output"
	"github.com/klauspost/compress/zstd"
	"github.com/y-scope/clp-ffi-go/ir"
)

// FlushContext manages timing and callback logic for log flushing.
type FlushContext struct {
	defaultLogLevel int
	hardDeltas      []time.Duration
	HardTimer       *time.Timer
	hardTimeout     time.Time
	softDelta       time.Duration
	softDeltas      []time.Duration
	SoftTimer       *time.Timer
	userCallback    func()
	Mutex           sync.Mutex
}

// CompressionContext encapsulates file and compression writers.
type CompressionContext struct {
	File       *os.File
	ZstdWriter *zstd.Encoder
	IRWriter   *ir.Writer
}

// IngestionContext contains compression and flush contexts for a particular log path.
type IngestionContext struct {
	Compression *CompressionContext
	Flush       *FlushContext
}

// S3Context holds AWS S3 configuration and client.
type S3Context struct {
	Client *s3.Client
	Bucket string
}

// PluginContext is the top-level context for the plugin.
type PluginContext struct {
	S3        *S3Context
	Ingestion map[string]*IngestionContext
}

// NewPluginContext initializes a new PluginContext
func NewPluginContext(plugin unsafe.Pointer) (*PluginContext, error) {
	client, err := S3CreateClient()
	if err != nil {
		log.Printf("[error] Failed to create S3 client: %v", err)
		return nil, err
	}

	bucket := output.FLBPluginConfigKey(plugin, "log_bucket")
	if err := S3ValidateLogBucket(client, bucket); err != nil {
		log.Printf("[error] Failed to validate log bucket %q: %v", bucket, err)
		return nil, err
	}
	log.Printf("[info] Logs are configured to be uploaded to s3://%s", bucket)

	pluginCtx := &PluginContext{
		S3: &S3Context{
			Client: client,
			Bucket: bucket,
		},
		Ingestion: make(map[string]*IngestionContext),
	}

	return pluginCtx, nil
}
