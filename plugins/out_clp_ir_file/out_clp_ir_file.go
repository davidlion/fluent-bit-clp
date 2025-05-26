package main

import (
	"C"
	"encoding/json"
	"log"
	"os"
	"time"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"

	"github.com/klauspost/compress/zstd"

	"github.com/y-scope/clp-ffi-go/ffi"
	"github.com/y-scope/clp-ffi-go/ir"

	"github.com/y-scope/fluent-bit-clp/internal/decoder"
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
	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	// Gets called with a batch of records to be written to an instance.

	// Open file for writing and make IR Writer (move to FLBPluginInit).
	file, err := os.OpenFile("/tmp/path.ir.zstd", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0660)
	if nil != err {
		log.Printf("[error] os.Create: %v", err)
		return output.FLB_ERROR
	}
	// var zstdWriter io.WriteCloser
	zstdWriter, err := zstd.NewWriter(file)
	if nil != err {
		log.Printf("[error] zstd.NewWriter failed: %v", err)
		return output.FLB_ERROR
	}
	irWriter, err := ir.NewWriter[ir.FourByteEncoding](zstdWriter)

	// Decode logs from fluent bit and write to IR.
	dec := decoder.New(data, int(length))
	for {
		flbTimestamp, jsonRecord, err := decoder.GetRecord(dec)
		if err != nil {
			log.Printf("[info] decoder.GetRecord error: %v", err)
			break
		}

		var timestamp time.Time
		switch t := flbTimestamp.(type) {
		case decoder.FlbTime:
			timestamp = t.Time
		case uint64:
			timestamp = time.Unix(int64(t), 0)
		default:
			log.Printf("time provided invalid, defaulting to now. Invalid type is %T", t)
			timestamp = time.Now()
		}

		var userKvPairs map[string]any
		err = json.Unmarshal(jsonRecord, &userKvPairs)
		if err != nil {
			log.Printf("[error] failed to unmarshal json record %v: %w", jsonRecord, err)
			return output.FLB_ERROR
		}

		var event *ffi.LogEvent = ffi.NewLogEvent()
		event.AutoKvPairs["timestamp"] = timestamp.UnixMilli()
		event.UserKvPairs = userKvPairs
		_, err = irWriter.WriteLogEvent(*event)
		if nil != err {
			log.Printf("[error] ir.Writer.WriteLogEvent failed: %v", err)
			return output.FLB_ERROR
		}
	}

	// Cleanup the writers (move to FLBPluginExitCtx).
	err = irWriter.Close()
	if nil != err {
		log.Printf("[error] ir.Writer.Close failed: %v", err)
		return output.FLB_ERROR
	}
	zstdWriter.Close()

    return output.FLB_OK
}

//export FLBPluginExitCtx
func FLBPluginExitCtx(ctx unsafe.Pointer) int {
	return output.FLB_OK
}

func main() {
}
