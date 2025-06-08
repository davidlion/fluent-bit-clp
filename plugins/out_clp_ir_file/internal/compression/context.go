package compression

import (
	"fmt"
	"github.com/klauspost/compress/zstd"
	"github.com/y-scope/clp-ffi-go/ir"
	"os"
	"unsafe"
)

type Context struct {
	File       *os.File
	ZstdWriter *zstd.Encoder
	IRWriter   *ir.Writer
}

const defaultFilePerm = 0o600

func NewContext(plugin unsafe.Pointer) (*Context, error) {
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

	ctx := Context{
		File:       file,
		ZstdWriter: zstdWriter,
		IRWriter:   irWriter,
	}

	return &ctx, nil
}
