//go:build !unix && !windows

package alloc

import "github.com/tetratelabs/wazero/experimental"

func Allocator() experimental.MemoryAllocator {
	return nil
}
