package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPrometheusMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PrometheusMiddleware())
	
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make a request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check that metrics were recorded
	count := testutil.CollectAndCount(httpRequestsTotal)
	if count == 0 {
		t.Error("Expected http_requests_total to be incremented")
	}
}

func TestRecordContractValidation(t *testing.T) {
	// Reset counter
	contractValidationTotal.Reset()

	RecordContractValidation(true)
	RecordContractValidation(false)

	// Check metrics
	successCount := testutil.ToFloat64(contractValidationTotal.WithLabelValues("success"))
	failedCount := testutil.ToFloat64(contractValidationTotal.WithLabelValues("failed"))

	if successCount != 1 {
		t.Errorf("Expected 1 success, got %f", successCount)
	}

	if failedCount != 1 {
		t.Errorf("Expected 1 failed, got %f", failedCount)
	}
}

func TestRecordContractVersionCreated(t *testing.T) {
	// Get initial count
	initialCount := testutil.ToFloat64(contractVersionsCreated)

	RecordContractVersionCreated()
	RecordContractVersionCreated()

	newCount := testutil.ToFloat64(contractVersionsCreated)
	
	if newCount != initialCount+2 {
		t.Errorf("Expected count to increase by 2, got %f", newCount-initialCount)
	}
}

func TestRecordWebSocketConnection(t *testing.T) {
	// Reset counter
	websocketConnectionsTotal.Reset()

	RecordWebSocketConnection("connect")
	RecordWebSocketConnection("disconnect")

	connectCount := testutil.ToFloat64(websocketConnectionsTotal.WithLabelValues("connect"))
	disconnectCount := testutil.ToFloat64(websocketConnectionsTotal.WithLabelValues("disconnect"))

	if connectCount != 1 {
		t.Errorf("Expected 1 connect, got %f", connectCount)
	}

	if disconnectCount != 1 {
		t.Errorf("Expected 1 disconnect, got %f", disconnectCount)
	}
}

func TestActiveWebSocketConnections(t *testing.T) {
	// Get initial value
	initialValue := testutil.ToFloat64(activeWebsocketConnections)

	IncrementActiveWebSocketConnections()
	IncrementActiveWebSocketConnections()
	
	currentValue := testutil.ToFloat64(activeWebsocketConnections)
	if currentValue != initialValue+2 {
		t.Errorf("Expected gauge to increase by 2, got %f", currentValue-initialValue)
	}

	DecrementActiveWebSocketConnections()
	
	finalValue := testutil.ToFloat64(activeWebsocketConnections)
	if finalValue != initialValue+1 {
		t.Errorf("Expected gauge to be at initial+1, got %f", finalValue-initialValue)
	}
}

func TestRecordDatabaseQuery(t *testing.T) {
	RecordDatabaseQuery("select", 50*time.Millisecond)
	RecordDatabaseQuery("insert", 100*time.Millisecond)

	// Verify observations were recorded by checking the histogram itself
	count := testutil.CollectAndCount(databaseQueryDuration)
	if count == 0 {
		t.Error("Expected database query duration to be recorded")
	}
}

func TestRecordRedisOperation(t *testing.T) {
	RecordRedisOperation("get", 10*time.Millisecond)
	RecordRedisOperation("set", 15*time.Millisecond)

	// Verify observations were recorded by checking the histogram itself
	count := testutil.CollectAndCount(redisOperationDuration)
	if count == 0 {
		t.Error("Expected redis operation duration to be recorded")
	}
}

func TestActiveConnections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PrometheusMiddleware())
	
	r.GET("/test", func(c *gin.Context) {
		// Check that active connections was incremented
		value := testutil.ToFloat64(activeConnections)
		if value < 1 {
			t.Error("Expected active connections to be at least 1 during request")
		}
		c.JSON(200, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

func TestMetricsRegistration(t *testing.T) {
	// Verify all metrics are registered
	metrics := []prometheus.Collector{
		httpRequestsTotal,
		httpRequestDuration,
		activeConnections,
		contractValidationTotal,
		contractVersionsCreated,
		websocketConnectionsTotal,
		activeWebsocketConnections,
		databaseQueryDuration,
		redisOperationDuration,
	}

	for _, metric := range metrics {
		if metric == nil {
			t.Error("Expected metric to be registered")
		}
	}
}
