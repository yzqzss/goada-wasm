# goada-wasm

Drop-in CGO-free replacement for https://github.com/ada-url/goada.

NOTE:
- `goada.New()` and `goada.NewWithBase()` are concurrency-safe functions.
However, the returned Url objects are not concurrency-safe.
- For best performance, call .Free() on returned url when you are done with them.

```go
Url, err := goadawasm.New("https://...")
if err != nil {
    return err
}
defer Url.Free()
```
