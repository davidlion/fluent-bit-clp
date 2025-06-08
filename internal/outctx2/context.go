package outctx2

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/fluent/fluent-bit-go/output"
	"github.com/klauspost/compress/zstd"
	"github.com/y-scope/clp-ffi-go/ir"

	"github.com/y-scope/fluent-bit-clp/internal/timeout"
)

type StreamingCompressionContext struct {
	File           *os.File
	ZstdWriter     *zstd.Encoder
	IRWriter       *ir.Writer
	S3Client       *s3.Client
	LogBucket      string
	TimeoutManager timeout.Manager
}

func CreateS3Client() (*s3.Client, error) {
	// Load the aws credentials. [awsConfig.LoadDefaultConfig] will look for credentials in a
	// specific hierarchy.
	// https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/
	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
	)
	if err != nil {
		return nil, fmt.Errorf("could not load aws credentials: %w", err)
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
	log.Printf("The upload bucket for logs: %v", logBucket)

	// Confirm bucket exists and test aws credentials.
	_, err := s3Client.HeadBucket(
		context.TODO(),
		&s3.HeadBucketInput{Bucket: aws.String(logBucket), ExpectedBucketOwner: nil},
	)
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

func UploadToS3(s3Client *s3.Client, bucket, localPath, remotePath string) error {
	// Open the file
	file, err := os.Open(localPath)
	if err != nil {
		log.Printf("[error] Failed to open file %q, %v", localPath, err)
		return err
	}
	defer file.Close()

	log.Println("Opened file:", localPath)

	// Upload file to path
	_, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(remotePath),
		Body:   file,
	})
	if err != nil {
		log.Printf("[error] Failed to upload file, %v", err)
		return err
	}
	log.Printf("Uploaded %v to s3://%v%v", localPath, bucket, remotePath)

	return nil
}

const defaultFilePerm = 0o600

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
	log.Printf("Logs are configured to be uploaded to s3:// %v", logBucket)

	file, err := os.OpenFile(
		"/tmp/compressed-logs.clp.zstd",
		os.O_WRONLY|os.O_CREATE,
		defaultFilePerm,
	)
	if nil != err {
		return nil, fmt.Errorf("os.Create: %w", err)
	}

	zstdWriter, err := zstd.NewWriter(file)
	if nil != err {
		return nil, fmt.Errorf("zstd.NewWriter: %w", err)
	}

	irWriter, err := ir.NewWriter[ir.FourByteEncoding](zstdWriter)
	if nil != err {
		return nil, fmt.Errorf("ir.NewWriter: %w", err)
	}

	// All the times are taken from:
	// https://github.com/y-scope/clp-loglib-py/blob/main/src/clp_logging/handlers.py#L185
	// TODO: update to use enum
	timeoutManager, err := timeout.NewManager(
		[]time.Duration{
			30 * time.Minute, // DEBUG
			30 * time.Minute, // INFO
			10 * time.Minute, // WARN
			5 * time.Minute,  // ERROR
			5 * time.Minute,  // FATAL
		},
		[]time.Duration{
			3 * time.Minute,  // DEBUG
			3 * time.Minute,  // INFO
			15 * time.Second, // WARN
			10 * time.Second, // ERROR
			5 * time.Second,  // FATAL
		},
		0,
		func() {
			if err := zstdWriter.Flush(); err != nil {
				log.Printf("timeout flush failed because zstdWriter.Flush failed: %v", err)
			}

			if err := UploadToS3(
				s3Client,
				logBucket,
				"/tmp/compressed-logs.clp.zstd",
				"compressed-logs.clp.zst",
			); err != nil {
				log.Printf("Failed to upload to S3")
			}
		},
	)
	if nil != err {
		return nil, fmt.Errorf("timeout.NewManager: %w", err)
	}

	ctx := StreamingCompressionContext{
		File:           file,
		ZstdWriter:     zstdWriter,
		IRWriter:       irWriter,
		S3Client:       s3Client,
		LogBucket:      logBucket,
		TimeoutManager: timeoutManager,
	}
	return &ctx, nil
}
