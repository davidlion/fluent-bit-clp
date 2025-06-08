package s3

import (
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/fluent/fluent-bit-go/output"
	"log"
	"unsafe"
)

type Context struct {
	Client *s3.Client
	Bucket string
}

func NewContext(plugin unsafe.Pointer) (*Context, error) {
	client, err := CreateS3Client()
	if err != nil {
		log.Printf("[error] Failed create s3 client")
		return nil, err
	}

	bucket := output.FLBPluginConfigKey(plugin, "log_bucket")
	err = ValidateLogBucket(client, bucket)

	err = ValidateLogBucket(client, bucket)
	if err != nil {
		return nil, err
	}
	log.Printf("Logs are configured to be uploaded to s3:// %v", bucket)

	ctx := Context{
		Client: client,
		Bucket: bucket,
	}

	return &ctx, nil
}
