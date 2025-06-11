package internal

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/klauspost/compress/zstd"
	"github.com/y-scope/clp-ffi-go/ir"
	"log"
	"os"
	"time"
)

// GetOrCreateIngestionContext returns an existing IngestionContext for the given path,
// or creates and registers a new one.
func GetOrCreateIngestionContext(pluginCtx *PluginContext, path string) (*IngestionContext, error) {
	// Return existing ingestion pluginCtx if available
	if ingestionContext, exists := pluginCtx.Ingestion[path]; exists {
		return ingestionContext, nil
	}

	tempFile, err := os.CreateTemp(os.TempDir(), "clp-irv2-*.clp.zst")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	zstdWriter, err := zstd.NewWriter(tempFile)
	if nil != err {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return nil, fmt.Errorf("failed to create zstd writer: %w", err)
	}

	irWriter, err := ir.NewWriter[ir.FourByteEncoding](zstdWriter)
	if nil != err {
		zstdWriter.Close()
		tempFile.Close()
		os.Remove(tempFile.Name())
		return nil, fmt.Errorf("failed to create IR writer: %w", err)
	}

	// All the times are taken from:
	// https://github.com/y-scope/clp-loglib-py/blob/main/src/clp_logging/handlers.py#L185
	// TODO: update to use enum
	hardDeltas := []time.Duration{
		3 * time.Second, // DEBUG
		3 * time.Second, // INFO
		3 * time.Second, // WARN
		3 * time.Second, // ERROR
		3 * time.Second, // FATAL
	}
	softDeltas := []time.Duration{
		3 * time.Second, // DEBUG
		3 * time.Second, // INFO
		3 * time.Second, // WARN
		3 * time.Second, // ERROR
		3 * time.Second, // FATAL
	}

	// Timers must be stopped and drained if not used, but here we assume they're managed in FlushContext logic.
	flushCtx := &FlushContext{
		hardDeltas:      hardDeltas,
		hardTimer:       time.NewTimer(0),
		softDeltas:      softDeltas,
		softTimer:       time.NewTimer(0),
		defaultLogLevel: 0,
		userCallback: func() {
			if err := zstdWriter.Flush(); err != nil {
				log.Printf("[error] zstdWriter.Flush failed: %v", err)
			}

			if err := S3Upload(
				pluginCtx.S3.Client, pluginCtx.S3.Bucket,
				tempFile.Name(),
				path+".clp.zst",
			); err != nil {
				log.Printf("[error] Failed to upload to S3: %v", err)
			}
		},
	}

	ingestionContext := &IngestionContext{
		Compression: &CompressionContext{
			File:       tempFile,
			ZstdWriter: zstdWriter,
			IRWriter:   irWriter,
		},
		Flush: flushCtx,
	}

	pluginCtx.Ingestion[path] = ingestionContext
	return ingestionContext, nil
}

// S3CreateClient creates an AWS S3 client with credentials and endpoint configuration.
func S3CreateClient() (*s3.Client, error) {
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = "us-west-1"
	}

	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithRegion(awsRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("could not load aws credentials: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true // Required for MinIO!
		if endpoint := os.Getenv("AWS_ENDPOINT_URL"); endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	return client, nil
}

// AWS error codes used for special error handling.
const (
	invalidCredsCode  = "InvalidClientTokenId"
	bucketMissingCode = "NotFound"
)

// S3ValidateLogBucket checks if the given bucket exists and that credentials work.
func S3ValidateLogBucket(s3Client *s3.Client, logBucket string) error {
	_, err := s3Client.HeadBucket(
		context.TODO(),
		&s3.HeadBucketInput{Bucket: aws.String(logBucket)},
	)
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			switch code := ae.ErrorCode(); code {
			case invalidCredsCode:
				return fmt.Errorf("aws credentials are invalid: %w", err)
			case bucketMissingCode:
				return fmt.Errorf("bucket %q could not be found: %w", logBucket, err)
			default:
				return fmt.Errorf("aws error [%s]: %w", code, err)
			}
		}
		return err
	}
	return nil
}

// S3Upload uploads the specified local file to the given S3 bucket and path.
func S3Upload(s3Client *s3.Client, bucket, localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file %q: %w", localPath, err)
	}
	defer file.Close()

	_, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(remotePath),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("failed to upload %s to s3://%s/%s: %w", localPath, bucket, remotePath, err)
	}
	log.Printf("[info] Uploaded %s to s3://%s/%s", localPath, bucket, remotePath)
	return nil
}
