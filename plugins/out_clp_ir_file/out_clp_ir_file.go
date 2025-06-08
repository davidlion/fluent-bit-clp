package main

import (
	"C"
)

import (
	"encoding/json"
	"log"
	"time"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
	"github.com/y-scope/clp-ffi-go/ffi"

	"github.com/y-scope/fluent-bit-clp/internal/decoder"
)

const PluginName = "out_clp_ir_file"

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	// Gets called only once when the plugin.so is loaded
	return output.FLBPluginRegister(def, PluginName, "CLP IR file output plugin")
}

//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {
	// Gets called only once for each instance you have configured.
	outCtx, err := NewContext(plugin)
	if err != nil {
		log.Printf("[error] Failed to initialize plugin: %s", err)
		return output.FLB_ERROR
	}

	// Set the context for this instance so that params can be retrieved during flush.
	output.FLBPluginSetContext(plugin, outCtx)

	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int /* tag */, _ *C.char) int {
	// Gets called with a batch of records to be written to an instance.
	p := output.FLBPluginGetContext(ctx)

	pluginCtx, ok := p.(*Context)
	if !ok {
		log.Println("Could not read context during flush")
		return output.FLB_ERROR
	}

	// Decode logs from fluent bit and write to IR.
	irWriter := pluginCtx.Compression.IRWriter
	dec := decoder.New(data, int(length))
	// TODO: tracking variables if you want to update timers once at the end based on the highest
	// log level seen and last timestamp
	var lastTimestamp time.Time
	var maxLogLevel int
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
			timestamp = time.UnixMilli(int64(t))
		default:
			log.Printf("time provided invalid, defaulting to now. Invalid type is %T", t)
			timestamp = time.Now()
		}

		var userKvPairs map[string]any
		err = json.Unmarshal(jsonRecord, &userKvPairs)
		if err != nil {
			log.Printf("[error] Failed to unmarshal json record %v: %v", jsonRecord, err)
			return output.FLB_ERROR
		}

		event := ffi.NewLogEvent()
		event.AutoKvPairs["timestamp"] = timestamp.UnixMilli()
		event.UserKvPairs = userKvPairs
		_, err = irWriter.WriteLogEvent(*event)
		if nil != err {
			log.Printf("[error] ir.Writer.WriteLogEvent failed: %v", err)
			return output.FLB_ERROR
		}

		// TODO: update to use enum
		// TODO: parse/handle the log level found
		var level int
		switch userKvPairs["LogLevel"] {
		case "debug":
			level = 0
		case "info":
			level = 1
		case "warn":
			level = 2
		case "error":
			level = 3
		case "fatal":
			level = 4
		}

		if maxLogLevel < level {
			maxLogLevel = level
		}
		if timestamp.After(lastTimestamp) {
			lastTimestamp = timestamp
		}
	}

	pluginCtx.Flush.Update(maxLogLevel, lastTimestamp)

	return output.FLB_OK
}

//export FLBPluginExitCtx
func FLBPluginExitCtx(ctx unsafe.Pointer) int {
	p := output.FLBPluginGetContext(ctx)

	pluginCtx, ok := p.(*Context)
	if !ok {
		log.Printf("[error] could not read context during flush")
		return output.FLB_ERROR
	}

	irWriter := pluginCtx.Compression.IRWriter

	// Cleanup the writers (move to FLBPluginExitCtx).
	err := irWriter.Close()
	if nil != err {
		log.Printf("[error] ir.Writer.Close failed: %v", err)
		return output.FLB_ERROR
	}

	zstdWriter := pluginCtx.Compression.ZstdWriter
	zstdWriter.Close()

	return output.FLB_OK
}

func main() {
}
