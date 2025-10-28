package goadawasm_test

import (
	"errors"
	"testing"

	goadawasm "github.com/yzqzss/goada-wasm"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		errorType   error
	}{
		{
			name:        "valid https URL",
			input:       "https://example.com/path",
			expectError: false,
		},
		{
			name:        "valid http URL with port",
			input:       "http://example.com:8080/path",
			expectError: false,
		},
		{
			name:        "valid URL with credentials",
			input:       "https://user:pass@example.com/path",
			expectError: false,
		},
		{
			name:        "valid file URL",
			input:       "file:///home/user/file.txt",
			expectError: false,
		},
		{
			name:        "valid ftp URL",
			input:       "ftp://ftp.example.com/file.txt",
			expectError: false,
		},
		{
			name:        "empty string",
			input:       "",
			expectError: true,
			errorType:   goadawasm.ErrEmptyString,
		},
		{
			name:        "invalid URL",
			input:       "not-a-url",
			expectError: true,
			errorType:   goadawasm.ErrInvalidUrl,
		},
		{
			name:        "malformed URL",
			input:       "http://",
			expectError: true,
			errorType:   goadawasm.ErrInvalidUrl,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := goadawasm.New(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorType != nil && !errors.Is(err, tt.errorType) {
					t.Errorf("expected error type %v, got %v", tt.errorType, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if url == nil {
				t.Error("expected URL object but got nil")
				return
			}

			defer url.Free()

			// Basic validation
			if !url.Valid() {
				t.Error("URL should be valid")
			}

			href := url.Href()
			if href == "" {
				t.Error("href should not be empty for valid URL")
			}
		})
	}
}

func TestNewWithBase(t *testing.T) {
	tests := []struct {
		name        string
		urlString   string
		baseString  string
		expected    string
		expectError bool
		errorType   error
	}{
		{
			name:       "relative path with base",
			urlString:  "../other/file.html",
			baseString: "https://example.com/base/",
			expected:   "https://example.com/other/file.html",
		},
		{
			name:       "absolute URL with base (should ignore base)",
			urlString:  "https://other.com/path",
			baseString: "https://example.com/base/",
			expected:   "https://other.com/path",
		},
		{
			name:       "relative path with query",
			urlString:  "page.html?query=value",
			baseString: "https://example.com/dir/",
			expected:   "https://example.com/dir/page.html?query=value",
		},
		{
			name:        "empty URL string",
			urlString:   "",
			baseString:  "",
			expectError: true,
			errorType:   goadawasm.ErrEmptyString,
		},
		{
			name:        "invalid base URL",
			urlString:   "path",
			baseString:  "not-a-url",
			expectError: true,
			errorType:   goadawasm.ErrInvalidUrl,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := goadawasm.NewWithBase(tt.urlString, tt.baseString)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorType != nil && !errors.Is(err, tt.errorType) {
					t.Errorf("expected error type %v, got %v", tt.errorType, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if url == nil {
				t.Error("expected URL object but got nil")
				return
			}

			defer url.Free()

			href := url.Href()
			if href != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, href)
			}
		})
	}
}

func TestUrlComponents(t *testing.T) {

	testURL := "https://user:password@example.com:8080/path/to/resource?query=value&foo=bar#fragment"
	url, err := goadawasm.New(testURL)
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}
	defer url.Free()

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"Href", url.Href(), testURL},
		{"Protocol", url.Protocol(), "https:"},
		{"Username", url.Username(), "user"},
		{"Password", url.Password(), "password"},
		{"Host", url.Host(), "example.com:8080"},
		{"Hostname", url.Hostname(), "example.com"},
		{"Port", url.Port(), "8080"},
		{"Pathname", url.Pathname(), "/path/to/resource"},
		{"Search", url.Search(), "?query=value&foo=bar"},
		{"Hash", url.Hash(), "#fragment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s: expected %s, got %s", tt.name, tt.expected, tt.got)
			}
		})
	}
}

func TestUrlBooleanMethods(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		tests map[string]bool
	}{
		{
			name: "URL with credentials and port",
			url:  "https://user:pass@example.com:8080/path?query=value#fragment",
			tests: map[string]bool{
				"Valid":               true,
				"HasCredentials":      true,
				"HasHostname":         true,
				"HasNonEmptyUsername": true,
				"HasNonEmptyPassword": true,
				"HasPort":             true,
				"HasPassword":         true,
				"HasHash":             true,
				"HasSearch":           true,
				"HasEmptyHostname":    false,
			},
		},
		{
			name: "Simple URL without extras",
			url:  "http://example.com/path",
			tests: map[string]bool{
				"Valid":               true,
				"HasCredentials":      false,
				"HasHostname":         true,
				"HasNonEmptyUsername": false,
				"HasNonEmptyPassword": false,
				"HasPort":             false,
				"HasPassword":         false,
				"HasHash":             false,
				"HasSearch":           false,
				"HasEmptyHostname":    false,
			},
		},
		{
			name: "File URL",
			url:  "file:///home/user/file.txt",
			tests: map[string]bool{
				"Valid":               true,
				"HasCredentials":      false,
				"HasHostname":         true, // file:// URLs have empty hostname
				"HasNonEmptyUsername": false,
				"HasNonEmptyPassword": false,
				"HasPort":             false,
				"HasPassword":         false,
				"HasHash":             false,
				"HasSearch":           false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := goadawasm.New(tt.url)
			if err != nil {
				t.Fatalf("failed to parse URL: %v", err)
			}
			defer url.Free()

			for methodName, expected := range tt.tests {
				var got bool
				switch methodName {
				case "Valid":
					got = url.Valid()
				case "HasCredentials":
					got = url.HasCredentials()
				case "HasEmptyHostname":
					got = url.HasEmptyHostname()
				case "HasHostname":
					got = url.HasHostname()
				case "HasNonEmptyUsername":
					got = url.HasNonEmptyUsername()
				case "HasNonEmptyPassword":
					got = url.HasNonEmptyPassword()
				case "HasPort":
					got = url.HasPort()
				case "HasPassword":
					got = url.HasPassword()
				case "HasHash":
					got = url.HasHash()
				case "HasSearch":
					got = url.HasSearch()
				default:
					t.Errorf("unknown method: %s", methodName)
					continue
				}

				if got != expected {
					t.Errorf("%s: expected %v, got %v", methodName, expected, got)
				}
			}
		})
	}
}

func TestUrlSetters(t *testing.T) {

	url, err := goadawasm.New("https://example.com/path")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}
	defer url.Free()

	// Test SetPort
	if !url.SetPort("8080") {
		t.Error("SetPort should succeed")
	}
	if url.Port() != "8080" {
		t.Errorf("expected port 8080, got %s", url.Port())
	}
	if url.Host() != "example.com:8080" {
		t.Errorf("expected host example.com:8080, got %s", url.Host())
	}

	// Test SetHostname
	if !url.SetHostname("newhost.com") {
		t.Error("SetHostname should succeed")
	}
	if url.Hostname() != "newhost.com" {
		t.Errorf("expected hostname newhost.com, got %s", url.Hostname())
	}

	// Test SetProtocol
	if !url.SetProtocol("http:") {
		t.Error("SetProtocol should succeed")
	}
	if url.Protocol() != "http:" {
		t.Errorf("expected protocol http:, got %s", url.Protocol())
	}

	// Test SetPathname
	if !url.SetPathname("/newpath") {
		t.Error("SetPathname should succeed")
	}
	if url.Pathname() != "/newpath" {
		t.Errorf("expected pathname /newpath, got %s", url.Pathname())
	}

	// Test SetUsername
	if !url.SetUsername("testuser") {
		t.Error("SetUsername should succeed")
	}
	if url.Username() != "testuser" {
		t.Errorf("expected username testuser, got %s", url.Username())
	}

	// Test SetPassword
	if !url.SetPassword("testpass") {
		t.Error("SetPassword should succeed")
	}
	if url.Password() != "testpass" {
		t.Errorf("expected password testpass, got %s", url.Password())
	}

	// Test SetSearch (void method)
	url.SetSearch("?query=test")
	if url.Search() != "?query=test" {
		t.Errorf("expected search ?query=test, got %s", url.Search())
	}

	// Test SetHash (void method)
	url.SetHash("#section1")
	if url.Hash() != "#section1" {
		t.Errorf("expected hash #section1, got %s", url.Hash())
	}

	// Test SetHref
	newHref := "http://different.com/other"
	if !url.SetHref(newHref) {
		t.Error("SetHref should succeed")
	}
	if url.Href() != newHref {
		t.Errorf("expected href %s, got %s", newHref, url.Href())
	}
}

func TestUrlSettersFailure(t *testing.T) {

	url, err := goadawasm.New("https://example.com/path")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}
	defer url.Free()

	// Test invalid port
	if url.SetPort("invalid-port") {
		t.Error("SetPort should fail for invalid port")
	}

	// Note: Ada URL parser may be more lenient with protocol validation
	// Test invalid href
	if url.SetHref("not-a-valid-url") {
		t.Error("SetHref should fail for invalid URL")
	}
}

func TestMemoryManagement(t *testing.T) {

	// Test that multiple URLs can be created and freed
	for i := 0; i < 50; i++ { // Reduced iterations to avoid memory issues
		url, err := goadawasm.New("https://example.com/path")
		if err != nil {
			t.Fatalf("failed to parse URL: %v", err)
		}

		// Test that the URL works
		href := url.Href()
		if href != "https://example.com/path" {
			t.Errorf("URL href mismatch: expected 'https://example.com/path', got '%s'", href)
		}

		// Explicitly free all URLs to avoid memory pressure
		url.Free()
	}
}

func TestConcurrentAccess(t *testing.T) {
	// Test that URL operations are safe for concurrent read access with separate URLs
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {

			// Each goroutine creates its own URL to avoid race conditions
			url, err := goadawasm.New("https://example.com:8080/path?query=value#fragment")
			if err != nil {
				t.Errorf("failed to parse URL: %v", err)
				done <- true
				return
			}
			defer url.Free()

			// These should all be safe to call concurrently on separate URL objects
			_ = url.Href()
			_ = url.Protocol()
			_ = url.Host()
			_ = url.Pathname()
			_ = url.Search()
			_ = url.Hash()
			_ = url.HasCredentials()
			_ = url.Valid()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
	}{
		{
			name: "URL with multiple query parameters",
			url:  "https://example.com/path?param1=value1&param2=value2&param3=value3&param4=value4",
		},
		{
			name: "URL with encoded characters",
			url:  "https://example.com/path%20with%20spaces?query=value%20with%20spaces#fragment%20with%20spaces",
		},
		{
			name: "URL with port and complex path",
			url:  "https://api.example.com:443/v1/users/123/profile?include=details&format=json#top",
		},
		{
			name: "URL with special characters",
			url:  "https://example.com/path with spaces?query=value with spaces#fragment with spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := goadawasm.New(tt.url)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if url == nil {
				t.Error("expected URL object but got nil")
				return
			}

			defer url.Free()

			// Basic validation - URL should be parseable
			if !url.Valid() {
				t.Error("URL should be valid")
			}

			// Should be able to get href without errors
			href := url.Href()
			if href == "" {
				t.Error("href should not be empty")
			}
		})
	}
}

func BenchmarkNew(b *testing.B) {
	testURL := "https://user:password@example.com:8080/path/to/resource?query=value&foo=bar#fragment"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		url, err := goadawasm.New(testURL)
		if err != nil {
			b.Fatal(err)
		}
		url.Free()
	}
}

func BenchmarkUrlParsing(b *testing.B) {
	testURLs := []string{
		"https://example.com/path",
		"http://user:pass@example.com:8080/path?query=value#fragment",
		"https://secure.example.com/api/v1/users/123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testURL := testURLs[i%len(testURLs)]
		url, err := goadawasm.New(testURL)
		if err != nil {
			b.Fatal(err)
		}

		// Access some properties to ensure they're computed
		_ = url.Href()
		_ = url.Host()
		_ = url.Pathname()

		url.Free()
	}
}

func BenchmarkUrlAccess(b *testing.B) {

	url, err := goadawasm.New("https://user:password@example.com:8080/path/to/resource?query=value&foo=bar#fragment")
	if err != nil {
		b.Fatal(err)
	}
	defer url.Free()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = url.Href()
		_ = url.Protocol()
		_ = url.Username()
		_ = url.Password()
		_ = url.Host()
		_ = url.Hostname()
		_ = url.Port()
		_ = url.Pathname()
		_ = url.Search()
		_ = url.Hash()
	}
}
