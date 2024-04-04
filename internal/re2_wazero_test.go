//go:build !tinygo.wasm && !re2_cgo

package internal

import (
	"context"
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

func TestEncodeMemory(t *testing.T) {
	// Make sure that encodeMemory produces a valid code.

	maxPagesCases := []uint32{
		1,     // 64 KB.
		2,     // 128 KB.
		3,     // 192 KB.
		4,     // 256 KB.
		10,    // 640 KB.
		100,   // 6.40 MB.
		1000,  // 64 MB.
		10000, // 640 MB.
		20000, // 1.28 GB.
		40000, // 2.56 GB.
		65536, // 4 GB (max).
	}

	ctx := context.Background()
	const enabledFeatures = api.CoreFeaturesV2 | experimental.CoreFeaturesThreads

	for _, maxPages := range maxPagesCases {
		t.Run(strconv.Itoa(int(maxPages)), func(t *testing.T) {
			mem := encodeMemory(maxPages)

			rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCoreFeatures(enabledFeatures))

			if _, err := rt.CompileModule(ctx, mem); err != nil {
				t.Errorf("InstantiateWithConfig(%s) failed: %v", hex.EncodeToString(mem), err)
			}
		})
	}
}
