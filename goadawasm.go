package goadawasm

import (
	"context"
	_ "embed"
	"errors"
	"log"
	"runtime"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed embed/ada.wasm
var adaWasm []byte

var (
	ErrEmptyString = errors.New("empty url string")
	ErrInvalidUrl  = errors.New("invalid url")
)

// Global WASM runtime and module - initialized once
var (
	wasmRuntime wazero.Runtime
	wasmModule  api.Module
	wasmCtx     context.Context
	initOnce    sync.Once
	wasmMutex   sync.Mutex
)

// Initialize the WASM runtime and module
func initWasm() {
	wasmCtx = context.Background()
	wasmRuntime = wazero.NewRuntime(wasmCtx)

	// Instantiate WASI
	wasi_snapshot_preview1.MustInstantiate(wasmCtx, wasmRuntime)

	// Instantiate the Ada WASM module
	var err error
	wasmModule, err = wasmRuntime.Instantiate(wasmCtx, adaWasm)
	if err != nil {
		log.Panicf("failed to instantiate Ada WASM module: %v", err)
	}
}

// Ensure WASM is initialized
func ensureWasmInit() {
	initOnce.Do(initWasm)
}

// URL represents a parsed URL backed by Ada WASM implementation
type Url struct {
	cpointer uint32 // Pointer to ada_url object in WASM memory
}

// Helper function to allocate memory in WASM
func wasmMalloc(size uint32) (uint32, error) {
	malloc := wasmModule.ExportedFunction("malloc")
	if malloc == nil {
		return 0, errors.New("malloc function not found")
	}

	results, err := malloc.Call(wasmCtx, uint64(size))
	if err != nil {
		return 0, err
	}

	return uint32(results[0]), nil
}

// Helper function to free memory in WASM
func wasmFree(ptr uint32) error {
	free := wasmModule.ExportedFunction("free")
	if free == nil {
		return errors.New("free function not found")
	}

	_, err := free.Call(wasmCtx, uint64(ptr))
	return err
}

// Helper function to write string to WASM memory and return pointer
func writeStringToWasm(s string) (uint32, error) {
	if len(s) == 0 {
		return 0, nil
	}

	ptr, err := wasmMalloc(uint32(len(s)))
	if err != nil {
		return 0, err
	}

	ok := wasmModule.Memory().Write(ptr, []byte(s))
	if !ok {
		wasmFree(ptr)
		return 0, errors.New("failed to write string to WASM memory")
	}

	return ptr, nil
}

// Helper function to read ada_string from WASM memory
func readAdaString(fn api.Function, urlPtr uint32) (string, error) {
	// Allocate memory for the ada_string result struct
	resultPtr, err := wasmMalloc(8) // 8 bytes for ada_string struct
	if err != nil {
		return "", err
	}
	defer wasmFree(resultPtr)

	// Call the function with WASM calling convention for struct returns
	_, err = fn.Call(wasmCtx, uint64(resultPtr), uint64(urlPtr))
	if err != nil {
		return "", err
	}

	// Read the ada_string result from memory
	resultBytes, ok := wasmModule.Memory().Read(resultPtr, 8)
	if !ok {
		return "", errors.New("failed to read result struct from memory")
	}

	// Extract buffer pointer and length (little-endian, 32-bit)
	bufferPtr := uint32(resultBytes[0]) | uint32(resultBytes[1])<<8 | uint32(resultBytes[2])<<16 | uint32(resultBytes[3])<<24
	length := uint32(resultBytes[4]) | uint32(resultBytes[5])<<8 | uint32(resultBytes[6])<<16 | uint32(resultBytes[7])<<24

	if bufferPtr == 0 || length == 0 {
		return "", nil
	}

	// Read the string data from WASM memory
	stringBytes, ok := wasmModule.Memory().Read(bufferPtr, length)
	if !ok {
		return "", errors.New("failed to read string from memory")
	}

	return string(stringBytes), nil
}

// Helper function to call a boolean-returning Ada function
func callAdaBoolFunction(funcName string, urlPtr uint32) bool {
	fn := wasmModule.ExportedFunction(funcName)
	if fn == nil {
		return false
	}

	results, err := fn.Call(wasmCtx, uint64(urlPtr))
	if err != nil {
		return false
	}

	return results[0] != 0
}

// Free the URL object in WASM memory
func free(u *Url) {
	if u.cpointer != 0 {
		adaFree := wasmModule.ExportedFunction("ada_free")
		if adaFree != nil {
			adaFree.Call(wasmCtx, uint64(u.cpointer))
		}
		u.cpointer = 0
	}
}

// New parses the given string into a URL
func New(urlstring string) (*Url, error) {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()

	ensureWasmInit()

	if len(urlstring) == 0 {
		return nil, ErrEmptyString
	}

	// Write URL string to WASM memory
	urlPtr, err := writeStringToWasm(urlstring)
	if err != nil {
		return nil, err
	}
	defer wasmFree(urlPtr)

	// Call ada_parse
	parseFunc := wasmModule.ExportedFunction("ada_parse")
	if parseFunc == nil {
		return nil, errors.New("ada_parse function not found")
	}

	results, err := parseFunc.Call(wasmCtx, uint64(urlPtr), uint64(len(urlstring)))
	if err != nil {
		return nil, err
	}

	urlObjPtr := uint32(results[0])
	if urlObjPtr == 0 {
		return nil, ErrInvalidUrl
	}

	// Check if the URL is valid
	if !callAdaBoolFunction("ada_is_valid", urlObjPtr) {
		adaFree := wasmModule.ExportedFunction("ada_free")
		if adaFree != nil {
			adaFree.Call(wasmCtx, uint64(urlObjPtr))
		}
		return nil, ErrInvalidUrl
	}
	url := &Url{cpointer: urlObjPtr}

	runtime.SetFinalizer(url, free)
	return url, nil
}

// NewWithBase parses the given strings into a URL with a base URL
func NewWithBase(urlstring, basestring string) (*Url, error) {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()

	ensureWasmInit()

	if len(urlstring) == 0 || len(basestring) == 0 {
		return nil, ErrEmptyString
	}

	// Write URL and base strings to WASM memory
	urlPtr, err := writeStringToWasm(urlstring)
	if err != nil {
		return nil, err
	}
	defer wasmFree(urlPtr)

	basePtr, err := writeStringToWasm(basestring)
	if err != nil {
		return nil, err
	}
	defer wasmFree(basePtr)

	// Call ada_parse_with_base
	parseFunc := wasmModule.ExportedFunction("ada_parse_with_base")
	if parseFunc == nil {
		return nil, errors.New("ada_parse_with_base function not found")
	}

	results, err := parseFunc.Call(wasmCtx, uint64(urlPtr), uint64(len(urlstring)), uint64(basePtr), uint64(len(basestring)))
	if err != nil {
		return nil, err
	}

	urlObjPtr := uint32(results[0])
	if urlObjPtr == 0 {
		return nil, ErrInvalidUrl
	}

	// Check if the URL is valid
	if !callAdaBoolFunction("ada_is_valid", urlObjPtr) {
		adaFree := wasmModule.ExportedFunction("ada_free")
		if adaFree != nil {
			adaFree.Call(wasmCtx, uint64(urlObjPtr))
		}
		return nil, ErrInvalidUrl
	}

	url := &Url{cpointer: urlObjPtr}
	runtime.SetFinalizer(url, free)
	return url, nil
}

// Free manually frees the URL object
func (u *Url) Free() {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	runtime.SetFinalizer(u, nil)
	free(u)
}

// Valid checks if the URL is valid
func (u *Url) Valid() bool {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	return callAdaBoolFunction("ada_is_valid", u.cpointer)
}

// HasCredentials checks if the URL has credentials
func (u *Url) HasCredentials() bool {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	return callAdaBoolFunction("ada_has_credentials", u.cpointer)
}

// HasEmptyHostname checks if the URL has an empty hostname
func (u *Url) HasEmptyHostname() bool {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	return callAdaBoolFunction("ada_has_empty_hostname", u.cpointer)
}

// HasHostname checks if the URL has a hostname
func (u *Url) HasHostname() bool {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	return callAdaBoolFunction("ada_has_hostname", u.cpointer)
}

// HasNonEmptyUsername checks if the URL has a non-empty username
func (u *Url) HasNonEmptyUsername() bool {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	return callAdaBoolFunction("ada_has_non_empty_username", u.cpointer)
}

// HasNonEmptyPassword checks if the URL has a non-empty password
func (u *Url) HasNonEmptyPassword() bool {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	return callAdaBoolFunction("ada_has_non_empty_password", u.cpointer)
}

// HasPort checks if the URL has a port
func (u *Url) HasPort() bool {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	return callAdaBoolFunction("ada_has_port", u.cpointer)
}

// HasPassword checks if the URL has a password
func (u *Url) HasPassword() bool {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	return callAdaBoolFunction("ada_has_password", u.cpointer)
}

// HasHash checks if the URL has a hash
func (u *Url) HasHash() bool {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	return callAdaBoolFunction("ada_has_hash", u.cpointer)
}

// HasSearch checks if the URL has a search/query string
func (u *Url) HasSearch() bool {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	return callAdaBoolFunction("ada_has_search", u.cpointer)
}

// Href returns the full URL string
func (u *Url) Href() string {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction("ada_get_href")
	if fn == nil {
		return ""
	}
	result, _ := readAdaString(fn, u.cpointer)
	return result
}

// Username returns the username
func (u *Url) Username() string {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction("ada_get_username")
	if fn == nil {
		return ""
	}
	result, _ := readAdaString(fn, u.cpointer)
	return result
}

// Password returns the password
func (u *Url) Password() string {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction("ada_get_password")
	if fn == nil {
		return ""
	}
	result, _ := readAdaString(fn, u.cpointer)
	return result
}

// Port returns the port
func (u *Url) Port() string {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction("ada_get_port")
	if fn == nil {
		return ""
	}
	result, _ := readAdaString(fn, u.cpointer)
	return result
}

// Hash returns the hash/fragment
func (u *Url) Hash() string {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction("ada_get_hash")
	if fn == nil {
		return ""
	}
	result, _ := readAdaString(fn, u.cpointer)
	return result
}

// Host returns the host (hostname + port)
func (u *Url) Host() string {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction("ada_get_host")
	if fn == nil {
		return ""
	}
	result, _ := readAdaString(fn, u.cpointer)
	return result
}

// Hostname returns the hostname
func (u *Url) Hostname() string {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction("ada_get_hostname")
	if fn == nil {
		return ""
	}
	result, _ := readAdaString(fn, u.cpointer)
	return result
}

// Pathname returns the pathname
func (u *Url) Pathname() string {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction("ada_get_pathname")
	if fn == nil {
		return ""
	}
	result, _ := readAdaString(fn, u.cpointer)
	return result
}

// Search returns the search/query string
func (u *Url) Search() string {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction("ada_get_search")
	if fn == nil {
		return ""
	}
	result, _ := readAdaString(fn, u.cpointer)
	return result
}

// Protocol returns the protocol/scheme
func (u *Url) Protocol() string {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction("ada_get_protocol")
	if fn == nil {
		return ""
	}
	result, _ := readAdaString(fn, u.cpointer)
	return result
}

// Helper function to call setter functions that return bool
func (u *Url) allSetterBoolWithLock(funcName, value string) bool {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction(funcName)
	if fn == nil {
		return false
	}

	valuePtr, err := writeStringToWasm(value)
	if err != nil {
		return false
	}
	defer wasmFree(valuePtr)

	results, err := fn.Call(wasmCtx, uint64(u.cpointer), uint64(valuePtr), uint64(len(value)))
	if err != nil {
		return false
	}

	return results[0] != 0
}

// Helper function to call setter functions that return void
func (u *Url) callSetterVoidWithLock(funcName, value string) {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()
	fn := wasmModule.ExportedFunction(funcName)
	if fn == nil {
		return
	}

	valuePtr, err := writeStringToWasm(value)
	if err != nil {
		return
	}
	defer wasmFree(valuePtr)

	fn.Call(wasmCtx, uint64(u.cpointer), uint64(valuePtr), uint64(len(value)))
}

// SetHref sets the full URL
func (u *Url) SetHref(s string) bool {
	return u.allSetterBoolWithLock("ada_set_href", s)
}

// SetHost sets the host
func (u *Url) SetHost(s string) bool {
	return u.allSetterBoolWithLock("ada_set_host", s)
}

// SetHostname sets the hostname
func (u *Url) SetHostname(s string) bool {
	return u.allSetterBoolWithLock("ada_set_hostname", s)
}

// SetProtocol sets the protocol
func (u *Url) SetProtocol(s string) bool {
	return u.allSetterBoolWithLock("ada_set_protocol", s)
}

// SetUsername sets the username
func (u *Url) SetUsername(s string) bool {
	return u.allSetterBoolWithLock("ada_set_username", s)
}

// SetPassword sets the password
func (u *Url) SetPassword(s string) bool {
	return u.allSetterBoolWithLock("ada_set_password", s)
}

// SetPort sets the port
func (u *Url) SetPort(s string) bool {
	return u.allSetterBoolWithLock("ada_set_port", s)
}

// SetPathname sets the pathname
func (u *Url) SetPathname(s string) bool {
	return u.allSetterBoolWithLock("ada_set_pathname", s)
}

// SetSearch sets the search/query string
func (u *Url) SetSearch(s string) {
	u.callSetterVoidWithLock("ada_set_search", s)
}

// SetHash sets the hash/fragment
func (u *Url) SetHash(s string) {
	u.callSetterVoidWithLock("ada_set_hash", s)
}
