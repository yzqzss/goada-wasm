package goadawasm_test

import (
	"fmt"
	"sync"
	"testing"

	goadawasm "github.com/yzqzss/goada-wasm"
)

// TestConcurrentMemoryStress tests memory handling under concurrent load
func TestConcurrentMemoryStress(t *testing.T) {
	t.Run("concurrent_url_creation", func(t *testing.T) {
		const goroutines = 10
		const iterations = 200

		var wg sync.WaitGroup
		errors := make(chan error, goroutines*iterations)

		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				parser, _ := goadawasm.NewParser()

				for i := 0; i < iterations; i++ {
					url, err := parser.New("https://example.com/concurrent/test")
					if err != nil {
						errors <- err
						return
					}

					href := url.Href()
					if href == "" {
						errors <- fmt.Errorf("goroutine %d, iteration %d: empty href", goroutineID, i)
						url.Free()
						return
					}
					url.SetPassword("pass")
					url.Free()
				}
			}(g)
		}

		wg.Wait()
		close(errors)

		errorCount := 0
		for err := range errors {
			t.Errorf("Concurrent test error: %v", err)
			errorCount++
		}

		if errorCount == 0 {
			t.Logf("Successfully completed %d concurrent goroutines with %d iterations each", goroutines, iterations)
		}
	})
}
