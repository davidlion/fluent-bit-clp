package main

import (
	"C"
)

// TODO: gci seems to break on these imports
import (
	"context"
	"encoding/json"
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
	"github.com/y-scope/clp-ffi-go/ffi"

	"github.com/y-scope/fluent-bit-clp/internal/decoder"
	"github.com/y-scope/fluent-bit-clp/internal/outctx2"
)

const cPluginName = "out_clp_ir_file"

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	// Gets called only once when the plugin.so is loaded
	return output.FLBPluginRegister(def, cPluginName, "CLP IR file output plugin")
}

//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {
	// Gets called only once for each instance you have configured.
	outCtx, err := outctx2.NewStreamingCompressionContext(plugin)
	if err != nil {
		log.Printf("[error] Failed to initialize plugin: %s", err)
		return output.FLB_ERROR
	}

	// Set the context for this instance so that params can be retrieved during flush.
	output.FLBPluginSetContext(plugin, outCtx)

	return output.FLB_OK
}

// errorAs is a helper for Go <1.20. For Go 1.20+, you can use errors.As.
func errorAs(err error, target any) bool {
	if err == nil {
		return false
	}
	switch t := target.(type) {
	case **smithy.APIError:
		apiErr, ok := err.(smithy.APIError)
		if !ok {
			return false
		}
		*t = &apiErr
		return true
	}
	return false
}

func bucketExists(client *s3.Client, bucket string) (bool, error) {
	_, err := client.HeadBucket(context.TODO(), &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err == nil {
		return true, nil // Bucket exists and is accessible
	}

	var apiErr smithy.APIError
	if ok := errorAs(err, &apiErr); ok {
		code := apiErr.ErrorCode()
		if code == "NotFound" || code == "404" || code == "NoSuchBucket" {
			return false, nil // Bucket does not exist
		}
		// AccessDenied means the bucket exists but you don't own it (or can't access)
		if code == "Forbidden" || code == "403" || code == "AccessDenied" {
			return true, nil // Exists, but not accessible/owned
		}
	}
	return false, err // Unexpected error
}

func bucketCreateIfNotExist(client *s3.Client, bucket string) error {
	exists, err := bucketExists(client, bucket)
	if err != nil {
		log.Printf("[error] Failed to check if bucket exists: %v", err)
		return err
	}

	if exists {
		log.Print("Bucket already exists:", bucket)
		return nil
	}

	createInput := &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}

	_, err = client.CreateBucket(context.TODO(), createInput)
	if err != nil {
		log.Printf("[error] Failed to create bucket: %v", err)
		return err
	}

	log.Print("Bucket created successfully:", bucket)
	return nil
}

func SetBucketPublicRead(client *s3.Client, bucket string) error {
	// This policy allows anyone to GetObject from the bucket:
	policy := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{
				"Effect":    "Allow",
				"Principal": "*",
				"Action":    "s3:GetObject",
				"Resource":  fmt.Sprintf("arn:aws:s3:::%s/*", bucket),
			},
		},
	}
	policyBytes, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("[error] Failed to marshal policy: %w", err)
	}

	_, err = client.PutBucketPolicy(context.TODO(), &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(string(policyBytes)),
	})
	if err != nil {
		return fmt.Errorf("[error] Failed to set bucket policy: %w", err)
	}

	log.Print("Bucket policy set to public-read for bucket:", bucket)
	return nil
}

func upload(localPath, remotePath string) error {
	bucket := "logs"

	// Load AWS config from default environment
	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithEndpointResolver(
			aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL: os.Getenv("AWS_ENDPOINT_URL"),
				}, nil
			}),
		),
	)
	if err != nil {
		log.Printf("[error] Failed to load config: %v", err)
		return err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true // Crucial for MinIO!
	})

	// TODO: fix the problem where the bucket fails to create if it doesn't exist and set bucket
	// policy to public read.
	// bucketCreateIfNotExist(client, bucket)
	// err = SetBucketPublicRead(client, bucket)
	// if err != nil {
	// 	log.Printf("[error] Failed to set bucket public: %v", err)
	// 	return err
	// }

	// Open the file
	file, err := os.Open(localPath)
	if err != nil {
		log.Printf("[error] Failed to open file %q, %v", localPath, err)
		return err
	}
	defer file.Close()

	log.Println("Opened file:", localPath)

	// Upload file to path
	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(remotePath),
		Body:   file,

		// Optional:
		// ContentType: aws.String("application/zstd"),
		// ACL:         types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		log.Printf("[error] Failed to upload file, %v", err)
		return err
	}

	if client != nil {
		log.Print("Successfully uploaded file to", bucket, remotePath)
	} else {
		log.Print("[error] Failed to upload file to", bucket, remotePath)
	}

	return nil
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, /* tag */_ *C.char) int {
	// Gets called with a batch of records to be written to an instance.
	p := output.FLBPluginGetContext(ctx)

	streamingCompressionContext, ok := p.(*outctx2.StreamingCompressionContext)
	if !ok {
		log.Println("Could not read context during flush")
		return output.FLB_ERROR
	}

	// Decode logs from fluent bit and write to IR.
	irWriter := streamingCompressionContext.IRWriter
	dec := decoder.New(data, int(length))
	for {
		flbTimestamp, jsonRecord, err := decoder.GetRecord(dec)
		if err != nil {
			log.Printf("[info] decoder.GetRecord error: %v", err)
			break
		}

		var timestamp int64
		switch t := flbTimestamp.(type) {
		case decoder.FlbTime:
			timestamp = t.UnixMilli()
		case uint64:
			timestamp = int64(t)
		default:
			log.Printf("time provided invalid, defaulting to now. Invalid type is %T", t)
			timestamp = time.Now().UnixMilli()
		}

		var userKvPairs map[string]any
		err = json.Unmarshal(jsonRecord, &userKvPairs)
		if err != nil {
			log.Printf("[error] Failed to unmarshal json record %v: %v", jsonRecord, err)
			return output.FLB_ERROR
		}

		event := ffi.NewLogEvent()
		event.AutoKvPairs["timestamp"] = timestamp
		event.UserKvPairs = userKvPairs
		_, err = irWriter.WriteLogEvent(*event)
		if nil != err {
			log.Printf("[error] ir.Writer.WriteLogEvent failed: %v", err)
			return output.FLB_ERROR
		}
	}

	// // Flush if necessary
	// lastFlushTimestamp := streamingCompressionContext.LastFlushTimestamp
	// currentTime := time.Now().UnixMilli()
	// if currentTime-lastFlushTimestamp > 60 {
	// 	err := streamingCompressionContext.ZstdWriter.Flush()
	// 	if err != nil {
	// 		return 0
	// 	}
	// 	streamingCompressionContext.LastFlushTimestamp = currentTime

	// 	// Upload to s3
	// 	upload("/tmp/path.ir.zstd", "compressed-logs.clp.zst")
	// }

	if err := streamingCompressionContext.ZstdWriter.Flush(); err != nil {
		return output.FLB_ERROR
	}

	// Upload to s3
	if err := upload("/tmp/compressed-logs.clp.zstd", "compressed-logs.clp.zst"); err != nil {
		return output.FLB_ERROR
	}

	return output.FLB_OK
}

//export FLBPluginExitCtx
func FLBPluginExitCtx(ctx unsafe.Pointer) int {
	p := output.FLBPluginGetContext(ctx)

	outCtx2, ok := p.(*outctx2.StreamingCompressionContext)
	if !ok {
		log.Printf("[error] could not read context during flush")
		return output.FLB_ERROR
	}

	irWriter := outCtx2.IRWriter

	// Cleanup the writers (move to FLBPluginExitCtx).
	err := irWriter.Close()
	if nil != err {
		log.Printf("[error] ir.Writer.Close failed: %v", err)
		return output.FLB_ERROR
	}

	zstdWriter := outCtx2.ZstdWriter
	zstdWriter.Close()

	return output.FLB_OK
}

func main() {
}
