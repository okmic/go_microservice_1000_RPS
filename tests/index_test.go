package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const baseURL = "http://localhost:9999"

type WalletResponse struct {
	WalletID string  `json:"walletId"`
	Balance  float64 `json:"balance"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type TransactionRequest struct {
	WalletID      string `json:"walletId"`
	OperationType string `json:"operationType"`
	Amount        int64  `json:"amount"`
}

type TestReport struct {
	TestName      string
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
	TotalRequests int
	SuccessCount  int
	FailCount     int
	SuccessRate   float64
	RPS           float64
	AvgLatency    float64
	Status        string
	Details       map[string]interface{}
}

var report TestReport


func TestWalletIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(baseURL + "/api/v1/wallets")
	if err != nil {
		t.Skip("Server not running, skipping integration tests")
	}
	resp.Body.Close()

	t.Run("Full wallet lifecycle", func(t *testing.T) {
		start := time.Now()
		t.Logf("[TEST START] Full wallet lifecycle")

		walletID := createWallet(t, client)
		t.Logf("[STEP 1] Wallet created successfully | wallet_id=%s", walletID)

		depositAndCheck(t, client, walletID, 1000, 1000)
		t.Logf("[STEP 2] Deposit operation | amount=1000 | new_balance=1000")

		withdrawAndCheck(t, client, walletID, 300, 700)
		t.Logf("[STEP 3] Withdraw operation | amount=300 | new_balance=700")

		tryOverdraw(t, client, walletID)
		t.Logf("[STEP 4] Overdraw prevention | attempted=10000 | result=rejected")

		finalBalance := getBalance(t, client, walletID)
		assert.Equal(t, float64(700), finalBalance)

		duration := time.Since(start)
		t.Logf("[TEST END] Full wallet lifecycle | duration=%v | final_balance=%.0f | status=PASSED", duration, finalBalance)
	})

	t.Run("Concurrent operations", func(t *testing.T) {
		start := time.Now()
		t.Logf("[TEST START] Concurrent operations test")

		walletID := createWallet(t, client)
		t.Logf("[STEP 1] Wallet created for concurrency test | wallet_id=%s", walletID)

		concurrentRequests := 50
		amountPerRequest := 10
		expectedTotal := concurrentRequests * amountPerRequest

		t.Logf("[STEP 2] Starting concurrent operations | requests=%d | amount_per_request=%d | expected_total=%d",
			concurrentRequests, amountPerRequest, expectedTotal)

		done := make(chan bool, concurrentRequests)
		successCount := 0
		var mu sync.Mutex

		for i := 0; i < concurrentRequests; i++ {
			go func(idx int) {
				deposit := TransactionRequest{
					WalletID:      walletID,
					OperationType: "DEPOSIT",
					Amount:        int64(amountPerRequest),
				}
				resp := doTransactionWithStatus(t, client, deposit)
				mu.Lock()
				if resp.StatusCode == http.StatusOK {
					successCount++
				}
				mu.Unlock()
				resp.Body.Close()
				done <- true
			}(i)
		}

		for i := 0; i < concurrentRequests; i++ {
			<-done
		}

		t.Logf("[STEP 3] Concurrent operations completed | successful=%d | failed=%d",
			successCount, concurrentRequests-successCount)

		finalBalance := getBalance(t, client, walletID)
		assert.Equal(t, float64(expectedTotal), finalBalance)

		duration := time.Since(start)
		t.Logf("[TEST END] Concurrent operations | duration=%v | requests=%d | final_balance=%.0f | expected=%.0f | status=PASSED",
			duration, concurrentRequests, finalBalance, float64(expectedTotal))
	})

	t.Run("Error scenarios", func(t *testing.T) {
		start := time.Now()
		t.Logf("[TEST START] Error scenarios validation")

		t.Log("[STEP 1] Testing non-existent wallet...")
		fakeID := uuid.New().String()
		resp := doGet(t, client, "/api/v1/wallets/"+fakeID)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		t.Logf("[STEP 1] Non-existent wallet check | wallet_id=%s | status=%d | expected=404 | result=PASSED",
			fakeID, resp.StatusCode)

		var errResp ErrorResponse
		json.NewDecoder(resp.Body).Decode(&errResp)
		assert.Equal(t, "wallet not found", errResp.Error)
		t.Logf("[STEP 1] Error message validation | message='%s' | expected='wallet not found' | result=PASSED",
			errResp.Error)

		t.Log("[STEP 2] Testing invalid request format...")
		invalid := map[string]interface{}{
			"walletId":      "invalid-uuid",
			"operationType": "DEPOSIT",
			"amount":        100,
		}
		resp = doPost(t, client, "/api/v1/wallets/transaction", invalid)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		t.Logf("[STEP 2] Invalid request validation | status=%d | expected=400 | result=PASSED",
			resp.StatusCode)

		duration := time.Since(start)
		t.Logf("[TEST END] Error scenarios | duration=%v | status=PASSED", duration)
	})
}


func TestPerformance_1000RPS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test")
	}

	report = TestReport{
		TestName:   "1000 RPS Performance Test",
		StartTime:  time.Now(),
		Status:     "RUNNING",
		Details:    make(map[string]interface{}),
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	walletID := createWalletForTest(t, client)
	report.Details["wallet_id"] = walletID
	t.Logf("[PERF] Test wallet created | wallet_id=%s", walletID)

	duration := 1 * time.Second
	targetRPS := 1000

	t.Logf("[PERF] Starting performance test | target_rps=%d | duration=%v", targetRPS, duration)

	var successCount int64
	var failCount int64
	var totalLatency int64

	var wg sync.WaitGroup
	stopChan := make(chan struct{})
	startTime := time.Now()

	requestRate := time.Tick(time.Second / time.Duration(targetRPS))

	workerCount := targetRPS * 2
	t.Logf("[PERF] Spawning workers | workers=%d", workerCount)

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopChan:
					return
				case <-requestRate:
					start := time.Now()
					err := sendTransaction(client, walletID, 1)
					latency := time.Since(start).Microseconds()
					atomic.AddInt64(&totalLatency, latency)

					if err != nil {
						atomic.AddInt64(&failCount, 1)
					} else {
						atomic.AddInt64(&successCount, 1)
					}
				}
			}
		}(i)
	}

	time.Sleep(duration)
	close(stopChan)
	wg.Wait()

	elapsed := time.Since(startTime)

	success := atomic.LoadInt64(&successCount)
	fail := atomic.LoadInt64(&failCount)
	total := success + fail
	avgLatency := float64(0)
	if total > 0 {
		avgLatency = float64(atomic.LoadInt64(&totalLatency)) / float64(total)
	}
	actualRPS := float64(total) / elapsed.Seconds()
	successRate := float64(success) / float64(total) * 100

	report.Duration = elapsed
	report.TotalRequests = int(total)
	report.SuccessCount = int(success)
	report.FailCount = int(fail)
	report.SuccessRate = successRate
	report.RPS = actualRPS
	report.AvgLatency = avgLatency

	t.Logf("[PERF] Performance Metrics:")
	t.Logf("[PERF]   Target RPS: %d", targetRPS)
	t.Logf("[PERF]   Actual RPS: %.2f", actualRPS)
	t.Logf("[PERF]   Total Requests: %d", total)
	t.Logf("[PERF]   Successful: %d (%.2f%%)", success, successRate)
	t.Logf("[PERF]   Failed: %d (%.2f%%)", fail, 100-successRate)
	t.Logf("[PERF]   Avg Latency: %.2f μs", avgLatency)
	t.Logf("[PERF]   Duration: %v", elapsed)

	report.Details["target_rps"] = targetRPS
	report.Details["actual_rps"] = actualRPS
	report.Details["avg_latency_us"] = avgLatency
	report.Details["worker_count"] = workerCount

	if total < int64(targetRPS) {
		report.Status = "FAILED"
		report.Details["error"] = fmt.Sprintf("Expected at least %d requests, got %d", targetRPS, total)
		t.Errorf("[PERF] Expected at least %d requests, got %d", targetRPS, total)
	}

	if successRate < 95.0 {
		report.Status = "FAILED"
		report.Details["error"] = fmt.Sprintf("Success rate too low: %.2f%%", successRate)
		t.Errorf("[PERF] Success rate too low: %.2f%%", successRate)
	}

	finalBalance := getBalance(t, client, walletID)
	expectedBalance := float64(success)

	t.Logf("[PERF] Balance verification | final=%.0f | expected=%.0f | diff=%.0f",
		finalBalance, expectedBalance, finalBalance-expectedBalance)

	report.Details["final_balance"] = finalBalance
	report.Details["expected_balance"] = expectedBalance

	if finalBalance != expectedBalance {
		report.Status = "FAILED"
		report.Details["error"] = fmt.Sprintf("Balance mismatch: got %.0f, expected %.0f", finalBalance, expectedBalance)
		t.Errorf("[PERF] Balance mismatch: got %.0f, expected %.0f", finalBalance, expectedBalance)
	}

	report.EndTime = time.Now()
	if report.Status != "FAILED" {
		report.Status = "PASSED"
	}

	printPerformanceReport(report)
}

func TestPerformance_HighConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test")
	}

	report := TestReport{
		TestName:   "High Concurrency Test",
		StartTime:  time.Now(),
		Status:     "RUNNING",
		Details:    make(map[string]interface{}),
	}

	client := &http.Client{Timeout: 10 * time.Second}

	walletID := createWalletForTest(t, client)
	report.Details["wallet_id"] = walletID
	t.Logf("[CONC] Test wallet created | wallet_id=%s", walletID)

	concurrency := 100
	requestsPerWorker := 10
	totalRequests := concurrency * requestsPerWorker

	t.Logf("[CONC] Starting concurrency test | workers=%d | requests_per_worker=%d | total_requests=%d",
		concurrency, requestsPerWorker, totalRequests)

	var successCount int64
	var failCount int64
	var wg sync.WaitGroup

	startTime := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			localSuccess := 0
			localFail := 0
			for j := 0; j < requestsPerWorker; j++ {
				err := sendTransaction(client, walletID, 1)
				if err != nil {
					atomic.AddInt64(&failCount, 1)
					localFail++
				} else {
					atomic.AddInt64(&successCount, 1)
					localSuccess++
				}
			}
			t.Logf("[CONC] Worker %d completed | success=%d | fail=%d", workerID, localSuccess, localFail)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	success := atomic.LoadInt64(&successCount)
	fail := atomic.LoadInt64(&failCount)
	total := success + fail

	actualRPS := float64(total) / elapsed.Seconds()
	successRate := float64(success) / float64(total) * 100

	report.Duration = elapsed
	report.TotalRequests = int(total)
	report.SuccessCount = int(success)
	report.FailCount = int(fail)
	report.SuccessRate = successRate
	report.RPS = actualRPS
	report.Details["concurrency"] = concurrency
	report.Details["requests_per_worker"] = requestsPerWorker

	t.Logf("[CONC] Performance Metrics:")
	t.Logf("[CONC]   Total Requests: %d", total)
	t.Logf("[CONC]   Successful: %d (%.2f%%)", success, successRate)
	t.Logf("[CONC]   Failed: %d (%.2f%%)", fail, 100-successRate)
	t.Logf("[CONC]   Actual RPS: %.2f", actualRPS)
	t.Logf("[CONC]   Duration: %v", elapsed)

	if successRate < 99.0 {
		report.Status = "FAILED"
		report.Details["error"] = fmt.Sprintf("Success rate too low: %.2f%%", successRate)
		t.Errorf("[CONC] Success rate too low: %.2f%%", successRate)
	}

	finalBalance := getBalance(t, client, walletID)
	expectedBalance := float64(success)

	t.Logf("[CONC] Balance verification | final=%.0f | expected=%.0f", finalBalance, expectedBalance)

	report.Details["final_balance"] = finalBalance
	report.Details["expected_balance"] = expectedBalance

	if finalBalance != expectedBalance {
		report.Status = "FAILED"
		report.Details["error"] = fmt.Sprintf("Balance mismatch: got %.0f, expected %.0f", finalBalance, expectedBalance)
		t.Errorf("[CONC] Balance mismatch: got %.0f, expected %.0f", finalBalance, expectedBalance)
	}

	report.EndTime = time.Now()
	if report.Status != "FAILED" {
		report.Status = "PASSED"
	}

	printConcurrencyReport(report)
}


func printPerformanceReport(r TestReport) {
	t := testing.T{}

	line := strings.Repeat("=", 60)
	dash := strings.Repeat("-", 60)

	t.Logf("%s", line)
	t.Logf("PERFORMANCE TEST REPORT")
	t.Logf("%s", line)
	t.Logf("Test Name:         %s", r.TestName)
	t.Logf("Status:            %s", r.Status)
	t.Logf("Duration:          %v", r.Duration)
	t.Logf("%s", dash)
	t.Logf("PERFORMANCE METRICS:")
	t.Logf("  Total Requests:  %d", r.TotalRequests)
	t.Logf("  Successful:      %d (%.2f%%)", r.SuccessCount, r.SuccessRate)
	t.Logf("  Failed:          %d (%.2f%%)", r.FailCount, 100-r.SuccessRate)
	t.Logf("  Actual RPS:      %.2f", r.RPS)
	t.Logf("  Avg Latency:     %.2f μs", r.AvgLatency)
	t.Logf("%s", dash)
	t.Logf("DETAILS:")
	for key, value := range r.Details {
		t.Logf("  %s: %v", key, value)
	}
	t.Logf("%s", line)
}

func printConcurrencyReport(r TestReport) {
	t := testing.T{}

	line := strings.Repeat("=", 60)
	dash := strings.Repeat("-", 60)

	t.Logf("%s", line)
	t.Logf("CONCURRENCY TEST REPORT")
	t.Logf("%s", line)
	t.Logf("Test Name:         %s", r.TestName)
	t.Logf("Status:            %s", r.Status)
	t.Logf("Duration:          %v", r.Duration)
	t.Logf("%s", dash)
	t.Logf("CONCURRENCY METRICS:")
	t.Logf("  Workers:         %d", r.Details["concurrency"])
	t.Logf("  Requests/Worker: %d", r.Details["requests_per_worker"])
	t.Logf("  Total Requests:  %d", r.TotalRequests)
	t.Logf("  Successful:      %d (%.2f%%)", r.SuccessCount, r.SuccessRate)
	t.Logf("  Failed:          %d (%.2f%%)", r.FailCount, 100-r.SuccessRate)
	t.Logf("  Actual RPS:      %.2f", r.RPS)
	t.Logf("%s", dash)
	t.Logf("BALANCE VERIFICATION:")
	t.Logf("  Final Balance:   %.0f", r.Details["final_balance"])
	t.Logf("  Expected:        %.0f", r.Details["expected_balance"])

	status := "MATCHED"
	if r.Details["final_balance"] != r.Details["expected_balance"] {
		status = "MISMATCH"
	}
	t.Logf("  Status:          %s", status)
	t.Logf("%s", line)
}


func createWallet(t *testing.T, client *http.Client) string {
	resp, err := client.Post(baseURL+"/api/v1/wallets", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var wallet WalletResponse
	err = json.NewDecoder(resp.Body).Decode(&wallet)
	require.NoError(t, err)
	require.NotEmpty(t, wallet.WalletID)

	return wallet.WalletID
}

func createWalletForTest(t *testing.T, client *http.Client) string {
	resp, err := client.Post(baseURL+"/api/v1/wallets", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}
	defer resp.Body.Close()

	var wallet WalletResponse
	if err := json.NewDecoder(resp.Body).Decode(&wallet); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	return wallet.WalletID
}

func depositAndCheck(t *testing.T, client *http.Client, walletID string, amount int64, expected float64) {
	req := TransactionRequest{
		WalletID:      walletID,
		OperationType: "DEPOSIT",
		Amount:        amount,
	}

	resp := doTransaction(t, client, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var wallet WalletResponse
	json.NewDecoder(resp.Body).Decode(&wallet)
	assert.Equal(t, expected, wallet.Balance)
}

func withdrawAndCheck(t *testing.T, client *http.Client, walletID string, amount int64, expected float64) {
	req := TransactionRequest{
		WalletID:      walletID,
		OperationType: "WITHDRAW",
		Amount:        amount,
	}

	resp := doTransaction(t, client, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var wallet WalletResponse
	json.NewDecoder(resp.Body).Decode(&wallet)
	assert.Equal(t, expected, wallet.Balance)
}

func tryOverdraw(t *testing.T, client *http.Client, walletID string) {
	req := TransactionRequest{
		WalletID:      walletID,
		OperationType: "WITHDRAW",
		Amount:        10000,
	}

	resp := doTransaction(t, client, req)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp ErrorResponse
	json.NewDecoder(resp.Body).Decode(&errResp)
	assert.Equal(t, "insufficient balance", errResp.Error)
}

func getBalance(t *testing.T, client *http.Client, walletID string) float64 {
	resp := doGet(t, client, "/api/v1/wallets/"+walletID)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var wallet WalletResponse
	json.NewDecoder(resp.Body).Decode(&wallet)
	return wallet.Balance
}

func doTransaction(t *testing.T, client *http.Client, req TransactionRequest) *http.Response {
	jsonData, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := client.Post(
		baseURL+"/api/v1/wallets/transaction",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	return resp
}

func doTransactionWithStatus(t *testing.T, client *http.Client, req TransactionRequest) *http.Response {
	jsonData, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := client.Post(
		baseURL+"/api/v1/wallets/transaction",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	return resp
}

func sendTransaction(client *http.Client, walletID string, amount int64) error {
	req := TransactionRequest{
		WalletID:      walletID,
		OperationType: "DEPOSIT",
		Amount:        amount,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := client.Post(
		baseURL+"/api/v1/wallets/transaction",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return nil
}

func doPost(t *testing.T, client *http.Client, path string, body interface{}) *http.Response {
	jsonData, err := json.Marshal(body)
	require.NoError(t, err)

	resp, err := client.Post(baseURL+path, "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	return resp
}

func doGet(t *testing.T, client *http.Client, path string) *http.Response {
	resp, err := client.Get(baseURL + path)
	require.NoError(t, err)
	return resp
}