package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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

func TestWalletIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	t.Log("")
	t.Log("======================================================================")
	t.Log("  INTEGRATION TESTS - Checking that the service works correctly")
	t.Log("======================================================================")

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(baseURL + "/api/v1/wallets")
	if err != nil {
		t.Skip("Server not running, skipping integration tests")
	}
	resp.Body.Close()

	t.Run("Full wallet lifecycle: create -> deposit -> withdraw -> check", func(t *testing.T) {
		t.Log("")
		t.Log("----------------------------------------------------------------------")
		t.Log("  TEST 1: Full wallet lifecycle")
		t.Log("----------------------------------------------------------------------")

		walletID := createWallet(t, client)
		t.Logf("  OK: Wallet created, ID: %s", walletID)

		depositAndCheck(t, client, walletID, 1000, 1000)
		t.Log("  OK: Deposit 1000 successful, balance: 1000")

		withdrawAndCheck(t, client, walletID, 300, 700)
		t.Log("  OK: Withdraw 300 successful, balance: 700")

		tryOverdraw(t, client, walletID)
		t.Log("  OK: Overdraw protection works, attempt to withdraw 10000 rejected")

		finalBalance := getBalance(t, client, walletID)
		assert.Equal(t, float64(700), finalBalance)
		t.Logf("  OK: Final balance: %.0f", finalBalance)
	})

	t.Run("50 concurrent requests to one wallet", func(t *testing.T) {
		t.Log("")
		t.Log("----------------------------------------------------------------------")
		t.Log("  TEST 2: Concurrent operations (50 requests)")
		t.Log("----------------------------------------------------------------------")

		walletID := createWallet(t, client)
		t.Logf("  OK: Wallet created, ID: %s", walletID)

		concurrentRequests := 50
		amountPerRequest := 10
		expectedTotal := concurrentRequests * amountPerRequest

		t.Logf("  INFO: Sending %d requests of %d each, expected balance: %d", 
			concurrentRequests, amountPerRequest, expectedTotal)

		done := make(chan bool, concurrentRequests)
		successCount := 0
		var mu sync.Mutex

		start := time.Now()
		for i := 0; i < concurrentRequests; i++ {
			go func() {
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
			}()
		}

		for i := 0; i < concurrentRequests; i++ {
			<-done
		}
		duration := time.Since(start)

		t.Logf("  INFO: Time: %v", duration)
		t.Logf("  OK: All requests completed, success: %d, failed: %d", 
			successCount, concurrentRequests-successCount)

		finalBalance := getBalance(t, client, walletID)
		assert.Equal(t, float64(expectedTotal), finalBalance)
		t.Logf("  OK: Final balance: %.0f (expected: %d)", finalBalance, expectedTotal)
	})

	t.Run("Error handling", func(t *testing.T) {
		t.Log("")
		t.Log("----------------------------------------------------------------------")
		t.Log("  TEST 3: Error handling")
		t.Log("----------------------------------------------------------------------")

		t.Log("  SCENARIO 1: Non-existent wallet")
		fakeID := uuid.New().String()
		resp := doGet(t, client, "/api/v1/wallets/"+fakeID)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var errResp ErrorResponse
		json.NewDecoder(resp.Body).Decode(&errResp)
		assert.Equal(t, "wallet not found", errResp.Error)
		t.Logf("  OK: 404 error: '%s'", errResp.Error)

		t.Log("  SCENARIO 2: Invalid UUID format")
		invalid := map[string]interface{}{
			"walletId":      "invalid-uuid",
			"operationType": "DEPOSIT",
			"amount":        100,
		}
		resp = doPost(t, client, "/api/v1/wallets/transaction", invalid)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		t.Log("  OK: 400 error: Invalid request rejected")
	})
}

func TestPerformance_1000RPS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test")
	}

	t.Log("")
	t.Log("======================================================================")
	t.Log("  PERFORMANCE TEST: 1000 RPS")
	t.Log("======================================================================")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	walletID := createWalletForTest(t, client)
	t.Logf("  OK: Wallet created, ID: %s", walletID)

	targetRPS := 1000
	duration := 1 * time.Second

	t.Logf("  INFO: Target: %d requests per second", targetRPS)
	t.Logf("  INFO: Duration: %v", duration)

	var successCount int64
	var failCount int64
	var totalLatency int64

	var wg sync.WaitGroup
	stopChan := make(chan struct{})
	startTime := time.Now()

	requestRate := time.Tick(time.Second / time.Duration(targetRPS))

	for i := 0; i < targetRPS; i++ {
		wg.Add(1)
		go func() {
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
		}()
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

	t.Log("")
	t.Log("  RESULTS:")
	t.Logf("    Target RPS:      %d", targetRPS)
	t.Logf("    Actual RPS:      %.2f", actualRPS)
	t.Logf("    Total requests:  %d", total)
	t.Logf("    Successful:      %d (%.2f%%)", success, successRate)
	t.Logf("    Failed:          %d (%.2f%%)", fail, 100-successRate)
	t.Logf("    Avg latency:     %.2f μs", avgLatency)
	t.Logf("    Duration:        %v", elapsed)

	if total >= int64(targetRPS) {
		t.Logf("  OK: Target achieved: %d RPS", targetRPS)
	} else {
		t.Logf("  FAIL: Target not achieved: got %d, expected %d", total, targetRPS)
	}

	if successRate >= 95.0 {
		t.Logf("  OK: Quality: %.2f%% successful requests", successRate)
	} else {
		t.Logf("  FAIL: Low quality: %.2f%% successful requests", successRate)
	}

	finalBalance := getBalance(t, client, walletID)
	expectedBalance := float64(success)
	t.Logf("  INFO: Balance check: final %.0f, expected %.0f", finalBalance, expectedBalance)

	if finalBalance == expectedBalance {
		t.Logf("  OK: Balance is correct: %.0f", finalBalance)
	} else {
		t.Logf("  FAIL: Balance mismatch: got %.0f, expected %.0f", finalBalance, expectedBalance)
	}
}

func TestPerformance_HighConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test")
	}

	t.Log("")
	t.Log("======================================================================")
	t.Log("  PERFORMANCE TEST: High Concurrency")
	t.Log("======================================================================")

	client := &http.Client{Timeout: 10 * time.Second}

	walletID := createWalletForTest(t, client)
	t.Logf("  OK: Wallet created, ID: %s", walletID)

	concurrency := 100
	requestsPerWorker := 10
	totalRequests := concurrency * requestsPerWorker

	t.Logf("  INFO: %d workers x %d requests = %d total", 
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
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	success := atomic.LoadInt64(&successCount)
	fail := atomic.LoadInt64(&failCount)
	total := success + fail

	actualRPS := float64(total) / elapsed.Seconds()
	successRate := float64(success) / float64(total) * 100

	t.Log("")
	t.Log("  RESULTS:")
	t.Logf("    Total requests:  %d", total)
	t.Logf("    Successful:      %d (%.2f%%)", success, successRate)
	t.Logf("    Failed:          %d (%.2f%%)", fail, 100-successRate)
	t.Logf("    Actual RPS:      %.2f", actualRPS)
	t.Logf("    Duration:        %v", elapsed)

	if successRate >= 99.0 {
		t.Logf("  OK: Quality: %.2f%% successful requests", successRate)
	} else {
		t.Logf("  FAIL: Low quality: %.2f%% successful requests", successRate)
	}

	finalBalance := getBalance(t, client, walletID)
	expectedBalance := float64(success)

	t.Logf("  INFO: Balance check: final %.0f, expected %.0f", finalBalance, expectedBalance)

	if finalBalance == expectedBalance {
		t.Logf("  OK: Balance is correct: %.0f", finalBalance)
	} else {
		t.Logf("  FAIL: Balance mismatch: got %.0f, expected %.0f", finalBalance, expectedBalance)
	}
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
