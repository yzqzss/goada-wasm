package goadawasm

import (
	"context"
	_ "embed"
	"errors"
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

// Global WASM runtime and compiled module - initialized once and shared
var (
	globalRuntime  wazero.Runtime
	compiledModule wazero.CompiledModule
	initOnce       sync.Once
	globalCtx      context.Context
)

// Parser represents a WASM module instance for concurrent URL parsing
type Parser struct {
	ctx       context.Context
	module    api.Module
	funcCache map[string]api.Function
	mutex     sync.RWMutex
}

// Initialize the global WASM runtime and compiled module (done once)
func initGlobalWasm() {
	globalCtx = context.Background()
	globalRuntime = wazero.NewRuntime(globalCtx)

	// Instantiate WASI
	wasi_snapshot_preview1.MustInstantiate(globalCtx, globalRuntime)

	// Compile the Ada WASM module once for reuse
	var err error
	compiledModule, err = globalRuntime.CompileModule(globalCtx, adaWasm)
	if err != nil {
		panic("failed to compile Ada WASM module: " + err.Error())
	}
}

// ensureGlobalInit ensures the global WASM runtime is initialized
func ensureGlobalInit() {
	initOnce.Do(initGlobalWasm)
}

var c = 0

// NewParser creates a new Parser with its own WASM module instance
func NewParser() (*Parser, error) {
	ensureGlobalInit()

	// Create a new module instance from the compiled module
	module, err := globalRuntime.InstantiateModule(globalCtx, compiledModule, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return nil, errors.New("failed to instantiate Ada WASM module: " + err.Error())
	}

	parser := &Parser{
		ctx:       globalCtx,
		module:    module,
		funcCache: make(map[string]api.Function),
	}
	runtime.SetFinalizer(parser, (*Parser).Close)
	return parser, nil
}

// Close closes the parser and releases its WASM module instance
func (p *Parser) Close() error {
	if p.module != nil {
		return p.module.Close(p.ctx)
	}
	return nil
}

// getFunction gets a cached function or loads it from the module
func (p *Parser) getFunction(name string) api.Function {
	p.mutex.RLock()
	fn, ok := p.funcCache[name]
	p.mutex.RUnlock()
	if ok {
		return fn
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Double-check after acquiring write lock
	if fn, ok := p.funcCache[name]; ok {
		return fn
	}

	fn = p.module.ExportedFunction(name)
	if fn != nil {
		p.funcCache[name] = fn
	}
	return fn
}

// URL represents a parsed URL backed by Ada WASM implementation
type Url struct {
	parser   *Parser // Reference to the parser that created this URL
	cpointer uint32  // Pointer to ada_url object in WASM memory
}

// Helper function to allocate memory in WASM
func (p *Parser) wasmMalloc(size uint32) (uint32, error) {
	malloc := p.getFunction("malloc")
	if malloc == nil {
		return 0, errors.New("malloc function not found")
	}

	results, err := malloc.Call(p.ctx, uint64(size))
	if err != nil {
		return 0, err
	}

	return uint32(results[0]), nil
}

// Helper function to free memory in WASM
func (p *Parser) wasmFree(ptr uint32) error {
	free := p.getFunction("free")
	if free == nil {
		return errors.New("free function not found")
	}

	_, err := free.Call(p.ctx, uint64(ptr))
	return err
}

// Helper function to write string to WASM memory and return pointer
func (p *Parser) writeStringToWasm(s string) (uint32, error) {
	if len(s) == 0 {
		return 0, nil
	}

	ptr, err := p.wasmMalloc(uint32(len(s)))
	if err != nil {
		return 0, err
	}

	ok := p.module.Memory().Write(ptr, []byte(s))
	if !ok {
		p.wasmFree(ptr)
		return 0, errors.New("failed to write string to WASM memory")
	}

	return ptr, nil
}

// Helper function to read ada_string from WASM memory
func (p *Parser) readAdaString(fn api.Function, urlPtr uint32) (string, error) {
	// Allocate memory for the ada_string result struct
	resultPtr, err := p.wasmMalloc(8) // 8 bytes for ada_string struct
	if err != nil {
		return "", err
	}
	defer p.wasmFree(resultPtr)

	// Call the function with WASM calling convention for struct returns
	_, err = fn.Call(p.ctx, uint64(resultPtr), uint64(urlPtr))
	if err != nil {
		return "", err
	}

	// Read the ada_string result from memory
	resultBytes, ok := p.module.Memory().Read(resultPtr, 8)
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
	stringBytes, ok := p.module.Memory().Read(bufferPtr, length)
	if !ok {
		return "", errors.New("failed to read string from memory")
	}

	return string(stringBytes), nil
}

// Helper function to call a boolean-returning Ada function
func (p *Parser) callAdaBoolFunction(funcName string, urlPtr uint32) bool {
	fn := p.getFunction(funcName)
	if fn == nil {
		return false
	}

	results, err := fn.Call(p.ctx, uint64(urlPtr))
	if err != nil {
		return false
	}

	return results[0] != 0
}

// ada_free frees the URL object in WASM memory
func (u *Url) ada_free() {
	if u.cpointer != 0 {
		adaFree := u.parser.getFunction("ada_free")
		if adaFree != nil {
			adaFree.Call(u.parser.ctx, uint64(u.cpointer))
		}
		u.cpointer = 0
	}
}

// New parses the given string into a URL using the parser
func (p *Parser) New(urlstring string) (*Url, error) {
	if len(urlstring) == 0 {
		return nil, ErrEmptyString
	}

	// Write URL string to WASM memory
	urlPtr, err := p.writeStringToWasm(urlstring)
	if err != nil {
		return nil, err
	}
	defer p.wasmFree(urlPtr)

	// Call ada_parse
	parseFunc := p.getFunction("ada_parse")
	if parseFunc == nil {
		return nil, errors.New("ada_parse function not found")
	}

	results, err := parseFunc.Call(p.ctx, uint64(urlPtr), uint64(len(urlstring)))
	if err != nil {
		return nil, err
	}

	urlObjPtr := uint32(results[0])
	if urlObjPtr == 0 {
		return nil, ErrInvalidUrl
	}

	// Check if the URL is valid
	if !p.callAdaBoolFunction("ada_is_valid", urlObjPtr) {
		adaFree := p.getFunction("ada_free")
		if adaFree != nil {
			adaFree.Call(p.ctx, uint64(urlObjPtr))
		}
		return nil, ErrInvalidUrl
	}

	url := &Url{parser: p, cpointer: urlObjPtr}
	runtime.SetFinalizer(url, (*Url).ada_free)
	return url, nil
}

// NewWithBase parses the given strings into a URL with a base URL using the parser
func (p *Parser) NewWithBase(urlstring, basestring string) (*Url, error) {
	if len(urlstring) == 0 || len(basestring) == 0 {
		return nil, ErrEmptyString
	}

	// Write URL and base strings to WASM memory
	urlPtr, err := p.writeStringToWasm(urlstring)
	if err != nil {
		return nil, err
	}
	defer p.wasmFree(urlPtr)

	basePtr, err := p.writeStringToWasm(basestring)
	if err != nil {
		return nil, err
	}
	defer p.wasmFree(basePtr)

	// Call ada_parse_with_base
	parseFunc := p.getFunction("ada_parse_with_base")
	if parseFunc == nil {
		return nil, errors.New("ada_parse_with_base function not found")
	}

	results, err := parseFunc.Call(p.ctx, uint64(urlPtr), uint64(len(urlstring)), uint64(basePtr), uint64(len(basestring)))
	if err != nil {
		return nil, err
	}

	urlObjPtr := uint32(results[0])
	if urlObjPtr == 0 {
		return nil, ErrInvalidUrl
	}

	// Check if the URL is valid
	if !p.callAdaBoolFunction("ada_is_valid", urlObjPtr) {
		adaFree := p.getFunction("ada_free")
		if adaFree != nil {
			adaFree.Call(p.ctx, uint64(urlObjPtr))
		}
		return nil, ErrInvalidUrl
	}

	url := &Url{parser: p, cpointer: urlObjPtr}
	runtime.SetFinalizer(url, (*Url).ada_free)
	return url, nil
}

// Free manually frees the URL object
func (u *Url) Free() {
	runtime.SetFinalizer(u, nil)
	u.ada_free()
}

// Valid checks if the URL is valid
func (u *Url) Valid() bool {
	return u.parser.callAdaBoolFunction("ada_is_valid", u.cpointer)
}

// HasCredentials checks if the URL has credentials
func (u *Url) HasCredentials() bool {
	return u.parser.callAdaBoolFunction("ada_has_credentials", u.cpointer)
}

// HasEmptyHostname checks if the URL has an empty hostname
func (u *Url) HasEmptyHostname() bool {
	return u.parser.callAdaBoolFunction("ada_has_empty_hostname", u.cpointer)
}

// HasHostname checks if the URL has a hostname
func (u *Url) HasHostname() bool {
	return u.parser.callAdaBoolFunction("ada_has_hostname", u.cpointer)
}

// HasNonEmptyUsername checks if the URL has a non-empty username
func (u *Url) HasNonEmptyUsername() bool {
	return u.parser.callAdaBoolFunction("ada_has_non_empty_username", u.cpointer)
}

// HasNonEmptyPassword checks if the URL has a non-empty password
func (u *Url) HasNonEmptyPassword() bool {
	return u.parser.callAdaBoolFunction("ada_has_non_empty_password", u.cpointer)
}

// HasPort checks if the URL has a port
func (u *Url) HasPort() bool {
	return u.parser.callAdaBoolFunction("ada_has_port", u.cpointer)
}

// HasPassword checks if the URL has a password
func (u *Url) HasPassword() bool {
	return u.parser.callAdaBoolFunction("ada_has_password", u.cpointer)
}

// HasHash checks if the URL has a hash
func (u *Url) HasHash() bool {
	return u.parser.callAdaBoolFunction("ada_has_hash", u.cpointer)
}

// HasSearch checks if the URL has a search/query string
func (u *Url) HasSearch() bool {
	return u.parser.callAdaBoolFunction("ada_has_search", u.cpointer)
}

// Href returns the full URL string
func (u *Url) Href() string {
	fn := u.parser.getFunction("ada_get_href")
	if fn == nil {
		return ""
	}
	result, _ := u.parser.readAdaString(fn, u.cpointer)
	return result
}

// Username returns the username
func (u *Url) Username() string {
	fn := u.parser.getFunction("ada_get_username")
	if fn == nil {
		return ""
	}
	result, _ := u.parser.readAdaString(fn, u.cpointer)
	return result
}

// Password returns the password
func (u *Url) Password() string {
	fn := u.parser.getFunction("ada_get_password")
	if fn == nil {
		return ""
	}
	result, _ := u.parser.readAdaString(fn, u.cpointer)
	return result
}

// Port returns the port
func (u *Url) Port() string {
	fn := u.parser.getFunction("ada_get_port")
	if fn == nil {
		return ""
	}
	result, _ := u.parser.readAdaString(fn, u.cpointer)
	return result
}

// Hash returns the hash/fragment
func (u *Url) Hash() string {
	fn := u.parser.getFunction("ada_get_hash")
	if fn == nil {
		return ""
	}
	result, _ := u.parser.readAdaString(fn, u.cpointer)
	return result
}

// Host returns the host (hostname + port)
func (u *Url) Host() string {
	fn := u.parser.getFunction("ada_get_host")
	if fn == nil {
		return ""
	}
	result, _ := u.parser.readAdaString(fn, u.cpointer)
	return result
}

// Hostname returns the hostname
func (u *Url) Hostname() string {
	fn := u.parser.getFunction("ada_get_hostname")
	if fn == nil {
		return ""
	}
	result, _ := u.parser.readAdaString(fn, u.cpointer)
	return result
}

// Pathname returns the pathname
func (u *Url) Pathname() string {
	fn := u.parser.getFunction("ada_get_pathname")
	if fn == nil {
		return ""
	}
	result, _ := u.parser.readAdaString(fn, u.cpointer)
	return result
}

// Search returns the search/query string
func (u *Url) Search() string {
	fn := u.parser.getFunction("ada_get_search")
	if fn == nil {
		return ""
	}
	result, _ := u.parser.readAdaString(fn, u.cpointer)
	return result
}

// Protocol returns the protocol/scheme
func (u *Url) Protocol() string {
	fn := u.parser.getFunction("ada_get_protocol")
	if fn == nil {
		return ""
	}
	result, _ := u.parser.readAdaString(fn, u.cpointer)
	return result
}

// Helper function to call setter functions that return bool
func (u *Url) callSetterBool(funcName, value string) bool {
	fn := u.parser.getFunction(funcName)
	if fn == nil {
		return false
	}

	valuePtr, err := u.parser.writeStringToWasm(value)
	if err != nil {
		return false
	}
	defer u.parser.wasmFree(valuePtr)

	results, err := fn.Call(u.parser.ctx, uint64(u.cpointer), uint64(valuePtr), uint64(len(value)))
	if err != nil {
		return false
	}

	return results[0] != 0
}

// Helper function to call setter functions that return void
func (u *Url) callSetterVoid(funcName, value string) {
	fn := u.parser.getFunction(funcName)
	if fn == nil {
		return
	}

	valuePtr, err := u.parser.writeStringToWasm(value)
	if err != nil {
		return
	}
	defer u.parser.wasmFree(valuePtr)

	fn.Call(u.parser.ctx, uint64(u.cpointer), uint64(valuePtr), uint64(len(value)))
}

// SetHref sets the full URL
func (u *Url) SetHref(s string) bool {
	return u.callSetterBool("ada_set_href", s)
}

// SetHost sets the host
func (u *Url) SetHost(s string) bool {
	return u.callSetterBool("ada_set_host", s)
}

// SetHostname sets the hostname
func (u *Url) SetHostname(s string) bool {
	return u.callSetterBool("ada_set_hostname", s)
}

// SetProtocol sets the protocol
func (u *Url) SetProtocol(s string) bool {
	return u.callSetterBool("ada_set_protocol", s)
}

// SetUsername sets the username
func (u *Url) SetUsername(s string) bool {
	return u.callSetterBool("ada_set_username", s)
}

// SetPassword sets the password
func (u *Url) SetPassword(s string) bool {
	return u.callSetterBool("ada_set_password", s)
}

// SetPort sets the port
func (u *Url) SetPort(s string) bool {
	return u.callSetterBool("ada_set_port", s)
}

// SetPathname sets the pathname
func (u *Url) SetPathname(s string) bool {
	return u.callSetterBool("ada_set_pathname", s)
}

// SetSearch sets the search/query string
func (u *Url) SetSearch(s string) {
	u.callSetterVoid("ada_set_search", s)
}

// SetHash sets the hash/fragment
func (u *Url) SetHash(s string) {
	u.callSetterVoid("ada_set_hash", s)
}
