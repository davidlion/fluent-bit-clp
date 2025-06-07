package main

import (
	"C"
)

// TODO: gci seems to break on these imports
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/smithy-go"
	"log"
	"os"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

// AWS error codes.
const (
	invalidCredsCode  = "InvalidClientTokenId"
	bucketMissingCode = "NotFound"
)

func upload(bucket, localPath, remotePath string) error {
	// Load the aws credentials. [awsConfig.LoadDefaultConfig] will look for credentials in a
	// specific hierarchy.
	// https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/
	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
	)
	if err != nil {
		log.Printf("[error] Could not load aws credentials: %w", err)
		return err
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true // Crucial for MinIO!
		o.BaseEndpoint = aws.String(os.Getenv("AWS_ENDPOINT_URL"))
	})

	// Confirm bucket exists and test aws credentials.
	_, err = s3Client.HeadBucket(context.TODO(), &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
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
				err = fmt.Errorf("error bucket %s could not be found: %w", bucket, err)
			default:
				err = fmt.Errorf("error aws %s: %w", code, err)
			}
		}
		return err
	}

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

	if s3Client != nil {
		log.Print("Successfully uploaded file to", bucket, remotePath)
	} else {
		log.Print("[error] Failed to upload file to", bucket, remotePath)
	}

	return nil
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int /* tag */, _ *C.char) int {
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
	if err := upload(streamingCompressionContext.LogBucket,
		"/tmp/compressed-logs.clp.zstd", "compressed-logs.clp.zst"); err != nil {
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
