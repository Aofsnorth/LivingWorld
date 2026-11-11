package overworld

/*
#cgo !windows LDFLAGS: -L${SRCDIR}/../../../native/target/release -Wl,-rpath,${SRCDIR}/../../../native/target/release -lsteel_bridge
#cgo darwin LDFLAGS: -framework Security -framework CoreFoundation
#cgo linux LDFLAGS: -lm -ldl -lpthread
#include "../../../native/steel.h"
#include <stdlib.h>

#ifdef _WIN32
#define WIN32_LEAN_AND_MEAN
#include <windows.h>

typedef SteelContext *(*fn_steel_context_new)(int64_t, uint32_t, uint32_t, uint8_t, SteelBuffer *);
typedef SteelBuffer (*fn_steel_generate_chunk)(const SteelContext *, int32_t, int32_t);
typedef void (*fn_steel_context_free)(SteelContext *);
typedef void (*fn_steel_buffer_free)(SteelBuffer);

static fn_steel_context_new p_steel_context_new = NULL;
static fn_steel_generate_chunk p_steel_generate_chunk = NULL;
static fn_steel_context_free p_steel_context_free = NULL;
static fn_steel_buffer_free p_steel_buffer_free = NULL;

static int lw_steel_load_dll(const char *dll_path) {
	if (p_steel_context_new != NULL) {
		return 0;
	}
	HMODULE mod = NULL;
	if (dll_path != NULL && dll_path[0] != '\0') {
		mod = LoadLibraryA(dll_path);
	}
	if (mod == NULL) {
		mod = LoadLibraryA("steel_bridge.dll");
	}
	if (mod == NULL) {
		return -1;
	}
	p_steel_context_new = (fn_steel_context_new)GetProcAddress(mod, "steel_context_new");
	p_steel_generate_chunk = (fn_steel_generate_chunk)GetProcAddress(mod, "steel_generate_chunk");
	p_steel_context_free = (fn_steel_context_free)GetProcAddress(mod, "steel_context_free");
	p_steel_buffer_free = (fn_steel_buffer_free)GetProcAddress(mod, "steel_buffer_free");
	if (!p_steel_context_new || !p_steel_generate_chunk || !p_steel_context_free || !p_steel_buffer_free) {
		return -2;
	}
	return 0;
}

static SteelContext *lw_steel_context_new(int64_t seed, uint32_t workers, uint32_t cache_chunks, uint8_t deterministic, SteelBuffer *error) {
	return p_steel_context_new(seed, workers, cache_chunks, deterministic, error);
}

static SteelBuffer lw_steel_generate_chunk(const SteelContext *context, int32_t cx, int32_t cz) {
	return p_steel_generate_chunk(context, cx, cz);
}

static void lw_steel_context_free(SteelContext *context) {
	p_steel_context_free(context);
}

static void lw_steel_buffer_free(SteelBuffer buffer) {
	p_steel_buffer_free(buffer);
}
#else
static int lw_steel_load_dll(const char *dll_path) {
	(void)dll_path;
	return 0;
}

static SteelContext *lw_steel_context_new(int64_t seed, uint32_t workers, uint32_t cache_chunks, uint8_t deterministic, SteelBuffer *error) {
	return steel_context_new(seed, workers, cache_chunks, deterministic, error);
}

static SteelBuffer lw_steel_generate_chunk(const SteelContext *context, int32_t cx, int32_t cz) {
	return steel_generate_chunk(context, cx, cz);
}

static void lw_steel_context_free(SteelContext *context) {
	steel_context_free(context);
}

static void lw_steel_buffer_free(SteelBuffer buffer) {
	steel_buffer_free(buffer);
}
#endif
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"
)

type nativeContext = C.SteelContext

type nativeBuffer struct {
	raw C.SteelBuffer
}

var (
	loadDLLOnce sync.Once
	loadDLLErr  error
)

func ensureNativeLoaded() error {
	loadDLLOnce.Do(func() {
		candidates := []string{}
		if envPath := os.Getenv("STEEL_BRIDGE_DLL"); envPath != "" {
			candidates = append(candidates, envPath)
		}
		if _, file, _, ok := runtime.Caller(0); ok {
			repoDLL := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "native", "target", "release", "steel_bridge.dll"))
			candidates = append(candidates, repoDLL)
		}
		if wd, err := os.Getwd(); err == nil {
			candidates = append(candidates, filepath.Join(wd, "native", "target", "release", "steel_bridge.dll"))
			candidates = append(candidates, filepath.Join(wd, "steel_bridge.dll"))
		}
		if exe, err := os.Executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), "steel_bridge.dll"))
		}

		chosen := ""
		for _, c := range candidates {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				chosen = c
				break
			}
		}

		var cPath *C.char
		if chosen != "" {
			cPath = C.CString(chosen)
			defer C.free(unsafe.Pointer(cPath))
		}
		rc := C.lw_steel_load_dll(cPath)
		if rc != 0 {
			loadDLLErr = fmt.Errorf("failed to load steel_bridge native library (code %d, tried %q)", int(rc), chosen)
		}
	})
	return loadDLLErr
}

// openContext acquires the native generation handle for a seed.
func openContext(seed int64, workers, cacheChunks uint32, deterministic bool) (*nativeContext, error) {
	if err := ensureNativeLoaded(); err != nil {
		return nil, err
	}
	var errBuf C.SteelBuffer
	mode := C.uint8_t(0)
	if deterministic {
		mode = 1
	}
	ctx := C.lw_steel_context_new(C.int64_t(seed), C.uint32_t(workers), C.uint32_t(cacheChunks), mode, &errBuf)
	if errBuf.failed != 0 {
		defer C.lw_steel_buffer_free(errBuf)
		return nil, bufferError(errBuf, "open context failed")
	}
	if ctx == nil {
		return nil, errors.New("open context returned nil")
	}
	return ctx, nil
}

// closeContext releases one native handle.
func closeContext(ctx *nativeContext) {
	C.lw_steel_context_free(ctx)
}

// generateEncoded runs native generation and returns an owned wire buffer.
func generateEncoded(ctx *nativeContext, cx, cz int32) (nativeBuffer, error) {
	buf := C.lw_steel_generate_chunk(ctx, C.int32_t(cx), C.int32_t(cz))
	if buf.failed != 0 {
		defer C.lw_steel_buffer_free(buf)
		return nativeBuffer{}, bufferError(buf, "generate chunk failed")
	}
	if buf.data == nil || buf.len == 0 {
		return nativeBuffer{}, errors.New("empty chunk buffer")
	}
	return nativeBuffer{raw: buf}, nil
}

// bytes views the Rust buffer without copying it.
func (b nativeBuffer) bytes() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(b.raw.data)), int(b.raw.len))
}

// free releases the Rust buffer.
func (b nativeBuffer) free() {
	C.lw_steel_buffer_free(b.raw)
}

// bufferError copies a failed native buffer into a Go error.
func bufferError(buf C.SteelBuffer, fallback string) error {
	if buf.data == nil || buf.len == 0 {
		return errors.New(fallback)
	}
	return errors.New(C.GoStringN((*C.char)(unsafe.Pointer(buf.data)), C.int(buf.len)))
}
