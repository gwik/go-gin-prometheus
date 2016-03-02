package ginprometheus

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

var defaultMetricPath = "/metrics"

type Prometheus struct {
	reqCnt               *prometheus.CounterVec
	reqDur, reqSz, resSz prometheus.Summary

	MetricsPath string
}

func NewPrometheus(subsystem string) *Prometheus {
	p := &Prometheus{
		MetricsPath: defaultMetricPath,
	}

	p.registerMetrics(subsystem)

	return p
}

func Middleware(subsystem string) gin.HandlerFunc {
	return NewPrometheus(subsystem).handlerFunc()
}

func (p *Prometheus) registerMetrics(subsystem string) {
	p.reqCnt = prometheus.MustRegisterOrGet(prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "requests_total",
			Help:      "How many HTTP requests processed, partitioned by status code and HTTP method.",
		},
		[]string{"code", "method", "handler"},
	)).(*prometheus.CounterVec)

	p.reqDur = prometheus.MustRegisterOrGet(prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      "request_duration_microseconds",
			Help:      "The HTTP request latencies in microseconds.",
		},
	)).(prometheus.Summary)

	p.reqSz = prometheus.MustRegisterOrGet(prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      "request_size_bytes",
			Help:      "The HTTP request sizes in bytes.",
		},
	)).(prometheus.Summary)

	p.resSz = prometheus.MustRegisterOrGet(prometheus.NewSummary(
		prometheus.SummaryOpts{
			Subsystem: subsystem,
			Name:      "response_size_bytes",
			Help:      "The HTTP response sizes in bytes.",
		},
	)).(prometheus.Summary)
}

func (p *Prometheus) Use(e *gin.Engine) {
	e.Use(p.handlerFunc())
	e.GET(p.MetricsPath, prometheusHandler())
}

func (p *Prometheus) handlerFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.String() == p.MetricsPath {
			c.Next()
			return
		}

		start := time.Now()

		reqSz := make(chan float64)
		go computeRequestSize(c.Request, reqSz)

		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		elapsed := float64(time.Since(start)) / float64(time.Microsecond)
		resSz := float64(c.Writer.Size())

		p.reqDur.Observe(elapsed)
		p.reqCnt.WithLabelValues(status, c.Request.Method, c.HandlerName()).Inc()
		p.reqSz.Observe(<-reqSz)
		p.resSz.Observe(resSz)
	}
}

func prometheusHandler() gin.HandlerFunc {
	h := prometheus.UninstrumentedHandler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func computeRequestSize(r *http.Request, out chan float64) {
	c := &counter{}
	r.Write(c)
	out <- float64(c.size)
}

type counter struct {
	size int
}

func (c *counter) Write(p []byte) (n int, err error) {
	l := len(p)
	c.size += l

	return l, nil
}
