package s3

import (
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/fluent/fluent-bit-go/output"
	"log"
	"unsafe"
)

type S3Context struct {
	Client *s3.Client
	Bucket string
}

func NewS3Context(plugin unsafe.Pointer) (*S3Context, error) {
	client, err := CreateS3Client()
	if err != nil {
		log.Printf("[error] Failed create s3 client")
		return nil, err
	}

	bucket := output.FLBPluginConfigKey(plugin, "log_bucket")

	err = ValidateLogBucket(client, bucket)
	if err != nil {
		return nil, err
	}
	log.Printf("[info] Logs are configured to be uploaded to s3:// %v", bucket)

	ctx := S3Context{
		Client: client,
		Bucket: bucket,
	}

	return &ctx, nil
}
