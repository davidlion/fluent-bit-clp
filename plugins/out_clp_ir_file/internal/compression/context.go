package compression

import (
	"fmt"
	"github.com/klauspost/compress/zstd"
	"github.com/y-scope/clp-ffi-go/ir"
	"os"
)

type Context struct {
	File       *os.File
	ZstdWriter *zstd.Encoder
	IRWriter   *ir.Writer
}

func NewContext() (*Context, error) {
	tempFile, err := os.CreateTemp(os.TempDir(), "clp-irv2-*.clp.zst")
	if err != nil {
		return nil, fmt.Errorf("[error] Failed to create temp file: %w", err)
	}

	zstdWriter, err := zstd.NewWriter(tempFile)
	if nil != err {
		return nil, fmt.Errorf("zstd.NewWriter: %w", err)
	}

	irWriter, err := ir.NewWriter[ir.FourByteEncoding](zstdWriter)
	if nil != err {
		return nil, fmt.Errorf("ir.NewWriter: %w", err)
	}

	ctx := Context{
		File:       tempFile,
		ZstdWriter: zstdWriter,
		IRWriter:   irWriter,
	}

	return &ctx, nil
}
