package s3

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"log"
	"os"
)

func CreateS3Client() (*s3.Client, error) {
	// Load the aws credentials. [awsConfig.LoadDefaultConfig] will look for credentials in a
	// specific hierarchy.
	// https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = "us-west-1"
	}
	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithRegion(awsRegion), // Crucial for MinIO!
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
	log.Printf("[info] Uploaded %v to s3://%v%v", localPath, bucket, remotePath)

	return nil
}
