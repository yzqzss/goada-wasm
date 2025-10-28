#!/bin/bash

set -e

# Download WASI SDK if not exists
if [ ! -d "wasi-sdk-27.0-x86_64-linux" ]; then
    echo "Downloading WASI SDK..."
    wcurl https://github.com/WebAssembly/wasi-sdk/releases/download/wasi-sdk-27/wasi-sdk-27.0-x86_64-linux.tar.gz
    tar xf wasi-sdk-27.0-x86_64-linux.tar.gz
    rm wasi-sdk-27.0-x86_64-linux.tar.gz
    echo "WASI SDK downloaded and extracted."
fi

export PATH=$PATH:`pwd`/wasi-sdk-27.0-x86_64-linux/bin

# Clean up old ada files
rm -f ada.cpp ada.h ada.wasm

# Download Ada library if not exists
echo "Downloading Ada library v3.3.0..."
wcurl https://github.com/ada-url/ada/releases/download/v3.3.0/ada.cpp
wcurl https://github.com/ada-url/ada/releases/download/v3.3.0/ada.h

# Compile Ada library to WASM
echo "Compiling Ada library to WASM..."
wasm32-wasi-clang++ -O3 -std=c++20 -fno-exceptions -Wl,--export-all -o ./ada.wasm ./ada.cpp ./main.cpp

echo "Build completed! Generated ada.wasm"
echo "File size: $(du -h ada.wasm | cut -f1)"