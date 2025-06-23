//go:build load
// +build load

package cmd

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/testutil"
)

func TestSelector_LoadTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	tdb := testutil.NewTestDB(t)
	defer tdb.Close()
	defer tdb.CleanupTestData(t)

	factory := testutil.NewDataFactory(tdb)
	logger := testutil.NewTestLogger(t)

	// Create selector
	sel := &selector{
		ctx:     tdb.Context(),
		db:      ntpdb.New(tdb.DB),
		log:     logger.Logger(),
		dryRun:  false,
	}

	t.Run("HighVolumeServers", func(t *testing.T) {
		// Generate large dataset
		numAccounts := 100
		numMonitors := 1000
		numServers := 10000

		t.Logf("Setting up load test data: %d accounts, %d monitors, %d servers",
			numAccounts, numMonitors, numServers)

		setupStart := time.Now()
		setupLargeDataset(t, factory, numAccounts, numMonitors, numServers)
		setupTime := time.Since(setupStart)
		t.Logf("Data setup completed in %v", setupTime)

		// Add servers to review queue
		batchSize := 100
		totalBatches := numServers / batchSize

		t.Logf("Processing %d servers in %d batches of %d", numServers, totalBatches, batchSize)

		overallStart := time.Now()
		processedCount := 0
		errorCount := 0

		for batch := 0; batch < totalBatches; batch++ {
			batchStart := time.Now()
			start := 10000 + batch*batchSize
			end := start + batchSize

			// Process batch of servers
			var wg sync.WaitGroup
			results := make(chan error, batchSize)

			for serverID := start; serverID < end; serverID++ {
				// Add to review queue
				_, err := tdb.ExecContext(tdb.Context(),
					"INSERT INTO servers_monitor_review (server_id, next_review) VALUES (?, NOW())",
					serverID)
				if err != nil {
					t.Logf("Failed to add server %d to queue: %v", serverID, err)
					continue
				}

				wg.Add(1)
				go func(sid int) {
					defer wg.Done()
					_, err := sel.processServerNew(uint32(sid))
					results <- err
				}(serverID)
			}

			// Wait for batch completion
			go func() {
				wg.Wait()
				close(results)
			}()

			// Collect results
			batchErrors := 0
			for err := range results {
				processedCount++
				if err != nil {
					batchErrors++
					errorCount++
					if batchErrors < 5 { // Log first few errors only
						t.Logf("Processing error: %v", err)
					}
				}
			}

			batchTime := time.Since(batchStart)
			serversPerSec := float64(batchSize) / batchTime.Seconds()

			t.Logf("Batch %d/%d: %d servers in %v (%.1f servers/sec, %d errors)",
				batch+1, totalBatches, batchSize, batchTime, serversPerSec, batchErrors)

			// Brief pause between batches to avoid overwhelming the database
			time.Sleep(100 * time.Millisecond)
		}

		overallTime := time.Since(overallStart)
		overallRate := float64(processedCount) / overallTime.Seconds()

		t.Logf("Load test completed: %d servers processed in %v (%.1f servers/sec overall)",
			processedCount, overallTime, overallRate)
		t.Logf("Error rate: %d/%d (%.2f%%)", errorCount, processedCount,
			float64(errorCount)/float64(processedCount)*100)

		// Performance assertions
		testutil.AssertTrue(t, overallRate > 1.0, "Expected >1 server/sec, got %.2f", overallRate)
		testutil.AssertTrue(t, float64(errorCount)/float64(processedCount) < 0.05,
			"Error rate too high: %.2f%%", float64(errorCount)/float64(processedCount)*100)
	})

	t.Run("ConcurrentServerProcessing", func(t *testing.T) {
		// Test concurrent processing of many servers
		numServers := 1000
		concurrency := 50

		// Setup servers
		baseID := 20000
		for i := 0; i < numServers; i++ {
			serverID := uint32(baseID + i)
			factory.CreateTestServer(t, serverID, fmt.Sprintf("203.0.2.%d", i%255), "v4", nil)

			_, err := tdb.ExecContext(tdb.Context(),
				"INSERT INTO servers_monitor_review (server_id, next_review) VALUES (?, NOW())",
				serverID)
			testutil.AssertNoError(t, err, "Failed to add server to queue")
		}

		t.Logf("Testing concurrent processing: %d servers with %d concurrent workers",
			numServers, concurrency)

		start := time.Now()

		// Channel for work distribution
		serverChan := make(chan uint32, numServers)
		results := make(chan error, numServers)

		// Add servers to work channel
		for i := 0; i < numServers; i++ {
			serverChan <- uint32(baseID + i)
		}
		close(serverChan)

		// Start workers
		var wg sync.WaitGroup
		for w := 0; w < concurrency; w++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				processed := 0
				for serverID := range serverChan {
					_, err := sel.processServerNew(serverID)
					results <- err
					processed++
				}
				t.Logf("Worker %d processed %d servers", workerID, processed)
			}(w)
		}

		// Wait for completion
		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect results
		processedCount := 0
		errorCount := 0
		for err := range results {
			processedCount++
			if err != nil {
				errorCount++
			}
		}

		duration := time.Since(start)
		rate := float64(processedCount) / duration.Seconds()

		t.Logf("Concurrent processing: %d servers in %v (%.1f servers/sec)",
			processedCount, duration, rate)
		t.Logf("Errors: %d/%d (%.2f%%)", errorCount, processedCount,
			float64(errorCount)/float64(processedCount)*100)

		// Performance assertions
		testutil.AssertTrue(t, rate > 5.0, "Expected >5 servers/sec with concurrency, got %.2f", rate)
		testutil.AssertEqual(t, numServers, processedCount, "Not all servers processed")
	})

	t.Run("MemoryUsageStability", func(t *testing.T) {
		// Test memory usage during extended operation
		numIterations := 100
		serversPerIteration := 50

		var maxMemory uint64

		for i := 0; i < numIterations; i++ {
			// Process batch of servers
			baseID := 30000 + i*serversPerIteration
			for j := 0; j < serversPerIteration; j++ {
				serverID := uint32(baseID + j)
				factory.CreateTestServer(t, serverID, fmt.Sprintf("204.0.%d.%d", i%255, j%255), "v4", nil)

				_, err := tdb.ExecContext(tdb.Context(),
					"INSERT INTO servers_monitor_review (server_id, next_review) VALUES (?, NOW())",
					serverID)
				testutil.AssertNoError(t, err, "Failed to add server to queue")

				_, err = sel.processServerNew(serverID)
				testutil.AssertNoError(t, err, "Processing failed")
			}

			// Check memory usage periodically
			if i%10 == 0 {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				currentMem := m.Alloc

				if currentMem > maxMemory {
					maxMemory = currentMem
				}

				t.Logf("Iteration %d: Memory usage: %d MB", i, currentMem/(1024*1024))
			}

			// Force garbage collection periodically
			if i%20 == 0 {
				runtime.GC()
			}
		}

		t.Logf("Maximum memory usage: %d MB", maxMemory/(1024*1024))

		// Memory should not exceed reasonable limits
		maxAllowedMB := uint64(500) // 500MB limit
		testutil.AssertTrue(t, maxMemory/(1024*1024) < maxAllowedMB,
			"Memory usage too high: %d MB (limit: %d MB)", maxMemory/(1024*1024), maxAllowedMB)
	})
}

func BenchmarkSelector_ProcessServer(b *testing.B) {
	tdb := testutil.NewTestDB(b)
	defer tdb.Close()
	defer tdb.CleanupTestData(b)

	factory := testutil.NewDataFactory(tdb)
	logger := testutil.NewTestLogger(b)

	// Setup benchmark data
	setupBenchmarkData(b, factory)

	sel := &selector{
		ctx:     tdb.Context(),
		db:      ntpdb.New(tdb.DB),
		log:     logger.Logger(),
		dryRun:  false,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		serverID := uint32(40000 + i%1000) // Rotate through 1000 servers

		// Add to review queue
		tdb.ExecContext(tdb.Context(),
			"INSERT INTO servers_monitor_review (server_id, next_review) VALUES (?, NOW()) ON DUPLICATE KEY UPDATE next_review = NOW()",
			serverID)

		// Benchmark the processing
		_, err := sel.processServerNew(serverID)
		if err != nil {
			b.Fatalf("Processing failed: %v", err)
		}
	}
}

// setupLargeDataset creates a large dataset for load testing
func setupLargeDataset(t *testing.T, factory *testutil.DataFactory, numAccounts, numMonitors, numServers int) {
	// Create accounts
	t.Logf("Creating %d accounts...", numAccounts)
	for i := 0; i < numAccounts; i++ {
		accountID := uint32(2000 + i)
		factory.CreateTestAccount(t, accountID, fmt.Sprintf("loadtest%d@example.com", i))
	}

	// Create monitors with diverse networks
	t.Logf("Creating %d monitors...", numMonitors)
	for i := 0; i < numMonitors; i++ {
		monitorID := uint32(5000 + i)
		accountID := uint32(2000 + i%numAccounts)

		// Distribute across different /16 networks
		network := i % 256
		host := (i / 256) % 256
		ip := fmt.Sprintf("172.%d.%d.1", network, host)

		status := "active"
		if i%10 == 0 {
			status = "testing"
		} else if i%20 == 0 {
			status = "pending"
		}

		factory.CreateTestMonitor(t, monitorID, fmt.Sprintf("load-mon-%d.test", i), accountID, ip, status)
	}

	// Create servers
	t.Logf("Creating %d servers...", numServers)
	for i := 0; i < numServers; i++ {
		serverID := uint32(10000 + i)

		// Distribute across different networks
		network := i % 256
		host := (i / 256) % 256
		ip := fmt.Sprintf("192.%d.%d.1", network, host)

		factory.CreateTestServer(t, serverID, ip, "v4", nil)
	}

	// Add some performance data for realistic testing
	t.Logf("Adding performance data...")
	now := time.Now()
	for i := 0; i < numMonitors && i < 100; i++ { // Limit to first 100 monitors for performance
		monitorID := uint32(5000 + i)
		for j := 0; j < 10 && j < numServers; j++ { // First 10 servers for each monitor
			serverID := uint32(10000 + j)

			// Add some historical log scores
			for k := 0; k < 5; k++ {
				score := 20.0 + float64(k%10)/10
				step := 0.8 + float64(k%5)/10
				rtt := int32(50 + k*10)
				ts := now.Add(-time.Duration(k) * time.Hour)

				factory.CreateTestLogScore(t, serverID, monitorID, score, step, &rtt, ts)
			}
		}
	}

	t.Logf("Large dataset setup completed")
}

// setupBenchmarkData creates optimized data for benchmarking
func setupBenchmarkData(b *testing.B, factory *testutil.DataFactory) {
	// Create 100 accounts
	for i := 0; i < 100; i++ {
		accountID := uint32(3000 + i)
		factory.CreateTestAccount(b, accountID, fmt.Sprintf("bench%d@example.com", i))
	}

	// Create 500 monitors
	for i := 0; i < 500; i++ {
		monitorID := uint32(6000 + i)
		accountID := uint32(3000 + i%100)
		ip := fmt.Sprintf("173.%d.%d.1", i%256, (i/256)%256)

		factory.CreateTestMonitor(b, monitorID, fmt.Sprintf("bench-mon-%d.test", i), accountID, ip, "active")
	}

	// Create 1000 servers
	for i := 0; i < 1000; i++ {
		serverID := uint32(40000 + i)
		ip := fmt.Sprintf("193.%d.%d.1", i%256, (i/256)%256)

		factory.CreateTestServer(b, serverID, ip, "v4", nil)
	}

	// Add minimal performance data
	now := time.Now()
	for i := 0; i < 50; i++ { // First 50 monitors
		monitorID := uint32(6000 + i)
		for j := 0; j < 20; j++ { // First 20 servers
			serverID := uint32(40000 + j)

			score := 20.0
			step := 0.9
			rtt := int32(50)

			factory.CreateTestLogScore(b, serverID, monitorID, score, step, &rtt, now.Add(-time.Hour))
		}
	}
}
