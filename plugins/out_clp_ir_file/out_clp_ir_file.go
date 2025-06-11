package main

import (
	"C"
)

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"time"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
	"github.com/y-scope/clp-ffi-go/ffi"

	"github.com/y-scope/fluent-bit-clp/internal/decoder"
	"github.com/y-scope/fluent-bit-clp/plugins/out_clp_ir_file/internal"
)

const (
	PluginName  = "out_clp_ir_file"
	filePathKey = "file_path"
)

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	// Gets called only once when the plugin.so is loaded
	return output.FLBPluginRegister(def, PluginName, "CLP IR file output plugin")
}

//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {
	// Gets called only once for each instance you have configured.
	outCtx, err := internal.NewPluginContext(plugin)
	if err != nil {
		log.Printf("[error] Failed to initialize plugin: %s.", err)
		return output.FLB_ERROR
	}

	// Set the context for this instance so that params can be retrieved during flush.
	output.FLBPluginSetContext(plugin, outCtx)

	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	// Retrieve plugin context.
	p := output.FLBPluginGetContext(ctx)
	pluginCtx, ok := p.(*internal.PluginContext)
	if !ok {
		log.Println("[error] Could not read context during flush.")
		return output.FLB_ERROR
	}

	tagStr := C.GoString(tag)

	dec := decoder.New(data, int(length))
	for {
		flbTimestamp, jsonRecord, err := decoder.GetRecord(dec)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("[error] decoder.GetRecord error: %v", err)
			}
			break
		}

		var timestamp time.Time
		switch t := flbTimestamp.(type) {
		case decoder.FlbTime:
			timestamp = t.Time
		case uint64:
			timestamp = time.UnixMilli(int64(t))
		default:
			log.Printf("[warn] Invalid time type (%T), defaulting to now.", t)
			timestamp = time.Now()
		}

		var userKvPairs map[string]any
		if err := json.Unmarshal(jsonRecord, &userKvPairs); err != nil {
			log.Printf("[error] Failed to unmarshal JSON record %q: %v", string(jsonRecord), err)
			continue // Should simply log skip to the next event
		}

		ingestionCtx, err := internal.GetOrCreateIngestionContext(pluginCtx, tagStr)
		if err != nil || ingestionCtx == nil {
			log.Printf("[error] Failed to get or create ingestion context for tag %s: %v",
				tagStr, err)
			continue // Should simply log skip to the next event
		}

		// CLP IrV2 makes a differentiation between auto-generated KV and user KV. For now,
		// we mark the timestamp and file_path as auto-generated KV, leaving the rest as user KV.
		event := ffi.NewLogEvent()
		event.AutoKvPairs["timestamp"] = timestamp.UnixMilli()
		// Extract file_path if it exists, otherwise set as empty string.
		filePath, exists := userKvPairs[filePathKey]
		if exists {
			delete(userKvPairs, filePathKey)
		} else {
			filePath = ""
		}
		event.AutoKvPairs[filePathKey] = filePath

		event.UserKvPairs = userKvPairs
		if _, err := ingestionCtx.Compression.IRWriter.WriteLogEvent(*event); err != nil {
			log.Printf("[error] Failed to write log event: %v", err)
			continue // Should simply log skip to the next event
		}

		// Handle log level with fallback and warning.
		var level int
		if lvl, found := userKvPairs["LogLevel"]; found {
			switch lvlStr := lvl.(type) {
			case string:
				switch lvlStr {
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
				default:
					log.Printf("[warn] Unknown log level %q, defaulting to 1 (info).", lvlStr)
					level = 1
				}
			default:
				log.Printf("[warn] LogLevel is not a string, defaulting to 1 (info).")
				level = 1
			}
		} else {
			log.Printf("[warn] LogLevel not found, defaulting to 1 (info).")
			level = 1
		}

		ingestionCtx.Flush.Update(level, timestamp)
	}

	return output.FLB_OK
}

//export FLBPluginExitCtx
func FLBPluginExitCtx(ctx unsafe.Pointer) int {
	p := output.FLBPluginGetContext(ctx)
	pluginCtx, ok := p.(*internal.PluginContext)
	if !ok {
		log.Println("[error] Could not read context during exit.")
		return output.FLB_ERROR
	}

	// Iterate and gracefully shutdown all ingestion contexts
	for path, ingestionCtx := range pluginCtx.Ingestion {
		flushCtx := ingestionCtx.Flush
		flushCtx.Mutex.Lock()

		// Stop timers to prevent further flushes
		if flushCtx.HardTimer != nil {
			flushCtx.HardTimer.Stop()
			flushCtx.HardTimer = nil
		}
		if flushCtx.SoftTimer != nil {
			flushCtx.SoftTimer.Stop()
			flushCtx.SoftTimer = nil
		}

		// Trigger final flush (calls userCallback)
		log.Printf("[info] Graceful shutdown: flushing logs for %q", path)
		flushCtx.Mutex.Unlock() // Unlock before callback to avoid deadlock
		flushCtx.Callback()
	}

	log.Println("[info] Plugin shutdown complete.")
	return output.FLB_OK
}

func main() {
}
