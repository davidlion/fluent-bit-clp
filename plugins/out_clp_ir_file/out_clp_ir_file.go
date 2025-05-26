package main

import (
	"C"
	"fmt"
	"os"
	"unsafe"

	"github.com/y-scope/fluent-bit-clp/internal/decoder"
	"github.com/fluent/fluent-bit-go/output"
	"github.com/y-scope/clp-ffi-go/ffi"
	"github.com/y-scope/clp-ffi-go/ir"
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
		return "", fmt.Errorf("os.Create: %v", err)
	}
	// var zstdWriter io.WriteCloser
	zstdWriter, err := zstd.NewWriter(file)
	if nil != err {
		return "", fmt.Errorf("zstd.NewWriter failed: %v", err)
	}
	var irWriter *ir.Writer
	irWriter, err := NewWriter[FourByteEncoding](zstdWriter)

	// Decode logs from fluent bit and write to IR.
	dec := decoder.New(data, size)
	for {
		ts, jsonRecord, err := decoder.GetRecord(dec)
		if err != nil {
			return logEvents, err
		}
		var userKvPairs map[string]any
		err := json.Unmarshal(jsonRecord, &userKvPairs)
		if err != nil {
			return "", fmt.Errorf("failed to unmarshal json record %v: %w", jsonRecord, err)
		}

		var event *ffi.LogEvent = ffi.NewLogEvent()
		event.AutoKvPairs["timestamp"] = decodeTs(ts).UnixMilli()
		event.UserKvPairs = userKvPairs
		_, err := irWriter.WriteLogEvent(*event)
		if nil != err {
			return "", fmt.Errorf("ir.Writer.WriteLogEvent failed: %v", err)
		}
	}


	// Cleanup the writers (move to FLBPluginExitCtx).
	err := irWriter.Close()
	if nil != err {
		t.Fatalf("ir.Writer.Close failed: %v", err)
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
