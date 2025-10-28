# goada-wasm

Drop-in CGO-free replacement for https://github.com/ada-url/goada.

NOTE: For best performance, call .Free() on returned url when you are done with them.

```go
Url, err := goada.New("https://...")
if err != nil {
    return err
}
defer Url.Free()
```

This library is concurrency-safe.
