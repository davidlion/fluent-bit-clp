package outctx2

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
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
	S3Client           *s3.Client
	LogBucket          string
	LastFlushTimestamp int64
}

func CreateS3Client() (*s3.Client, error) {
	// Load the aws credentials. [awsConfig.LoadDefaultConfig] will look for credentials in a
	// specific hierarchy.
	// https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/
	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
	)
	if err != nil {
		log.Printf("[error] Could not load aws credentials: %w", err)
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true // Crucial for MinIO!
		o.BaseEndpoint = aws.String(os.Getenv("AWS_ENDPOINT_URL"))
	})

	return client, nil
}

// AWS error codes.
const (
	invalidCredsCode  = "InvalidClientTokenId"
	bucketMissingCode = "NotFound"
)

func ValidateLogBucket(s3Client *s3.Client, logBucket string) error {
	log.Printf("The upload bucket for logs: " + logBucket)

	// Confirm bucket exists and test aws credentials.
	_, err := s3Client.HeadBucket(context.TODO(), &s3.HeadBucketInput{
		Bucket: aws.String(logBucket),
	})
	if err != nil {
		// AWS does have some error types that can be checked with [error.As] such as
		// [s3.NotFound]. However, it can be difficult to always find the appropriate type. As a
		// result, using aws [smithy-go] to handle error codes.
		// https://aws.github.io/aws-sdk-go-v2/docs/handling-errors/#api-error-responses
		var ae smithy.APIError
		if errors.As(err, &ae) {
			switch code := ae.ErrorCode(); code {
			case invalidCredsCode:
				err = fmt.Errorf("error aws credentials are invalid: %w", err)
			case bucketMissingCode:
				err = fmt.Errorf("error bucket %s could not be found: %w", logBucket, err)
			default:
				err = fmt.Errorf("error aws %s: %w", code, err)
			}
		}
		return err
	}
	return nil
}

func NewStreamingCompressionContext(plugin unsafe.Pointer) (*StreamingCompressionContext, error) {
	// Create S3 client
	s3Client, err := CreateS3Client()
	if err != nil {
		log.Printf("[error] Failed create s3 client")
		return nil, err
	}

	logBucket := output.FLBPluginConfigKey(plugin, "log_bucket")
	err = ValidateLogBucket(s3Client, logBucket)
	if err != nil {
		return nil, err
	}
	log.Printf("Logs are configured to be uploaded to s3://" + logBucket)

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
		S3Client:           s3Client,
		LogBucket:          logBucket,
		LastFlushTimestamp: 0,
	}

	return &ctx, nil
}
