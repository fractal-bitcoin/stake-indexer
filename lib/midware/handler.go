package midware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	isMetricsOn = true
)

type Response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Tag  string `json:"tag,omitempty"`
}

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r responseBodyWriter) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/health/") || strings.HasPrefix(path, "/metrics") {
			// Do not record metrics for health/metrics endpoints.
			c.Next() // Process request
			return
		}

		start := time.Now() // Start timer
		wrapWriter := &responseBodyWriter{body: &bytes.Buffer{}, ResponseWriter: c.Writer}
		c.Writer = wrapWriter // duplicate response body

		ServiceMetrics.Inc("flight", "in") // Count in-flight requests.
		defer ServiceMetrics.Dec("flight", "in")

		c.Next() // Process request

		// get response info
		var bizCode int
		var responseObj Response
		responseBodyJson := wrapWriter.body.Bytes()
		if e := json.Unmarshal(responseBodyJson, &responseObj); e == nil {
			bizCode = responseObj.Code
		}

		latency := time.Now().Sub(start)
		statusCode := c.Writer.Status()

		// Update metrics.
		if isMetricsOn {
			ServiceMetrics.Observe("all", "visit", latency)                       // Count all incoming requests.
			ServiceMetrics.Observe(strconv.Itoa(statusCode), "httpcode", latency) // Count requests by HTTP status code.
			ServiceMetrics.Observe(strconv.Itoa(bizCode), "bizcode", latency)     // Count requests by business code.
			if statusCode != 404 {
				urlPattern := c.FullPath()
				ServiceMetrics.Observe(urlPattern, "url", latency) // Count requests by HTTP route.
				ServiceMetrics.Observe(strconv.Itoa(bizCode)+urlPattern, "bizcode-url", latency)
			}
		}
	}
}

func CreateMetricsEndpoint(adminGinWeb gin.IRouter) {
	adminGinWeb.GET("/metrics", fetchMetricsSummary)
	adminGinWeb.GET("/metrics/:time/:type/:stage", fetchMetrics)
}

func fetchMetricsSummary(c *gin.Context) {
	timeFilter := "minute"
	typeFilter := "url"
	stageFilter := "past"

	metrics := ServiceMetrics.Dump(timeFilter, typeFilter, stageFilter)
	c.String(http.StatusOK, metrics)
}

func fetchMetrics(c *gin.Context) {
	timeFilter := c.Param("time")
	typeFilter := c.Param("type")
	stageFilter := c.Param("stage")

	metrics := ServiceMetrics.Dump(timeFilter, typeFilter, stageFilter)
	c.String(http.StatusOK, metrics)
}
