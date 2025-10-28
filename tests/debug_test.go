package goadawasm_test

import (
	"testing"

	goadawasm "github.com/yzqzss/goada-wasm"
)

func TestDebugProtocolBehavior(t *testing.T) {
	url, err := goadawasm.New("https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	defer url.Free()

	t.Logf("Original protocol: %s", url.Protocol())

	// Test various protocol values
	protocols := []string{
		"http:",
		"https:",
		"http",     // Missing colon
		"http123:", // Invalid characters
		"",         // Empty
		"ht tp:",   // With spaces
		"HTTP:",    // Uppercase
	}

	for _, proto := range protocols {
		// Create a fresh URL for each test
		testURL, err := goadawasm.New("https://example.com/")
		if err != nil {
			t.Errorf("Failed to create URL: %v", err)
			continue
		}

		result := testURL.SetProtocol(proto)
		actualProto := testURL.Protocol()
		t.Logf("SetProtocol(%q) -> result: %v, actual protocol: %q", proto, result, actualProto)

		testURL.Free()
	}

	// Test null byte handling
	t.Log("Testing null byte handling:")
	nullURL, err := goadawasm.New("https://example.com/path\x00")
	if err != nil {
		t.Logf("URL with null byte correctly rejected: %v", err)
	} else {
		t.Logf("URL with null byte accepted - this might be a problem")
		t.Logf("Href: %q", nullURL.Href())
		nullURL.Free()
	}
}
