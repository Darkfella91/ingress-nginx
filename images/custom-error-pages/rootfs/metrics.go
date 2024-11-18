package main

import (
    "github.com/prometheus/client_golang/prometheus"
)

var (
    requestCount = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"proto"},
    )
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "Duration of HTTP requests",
            Buckets: prometheus.DefBuckets,
        },
        []string{"proto"},
    )
)

func init() {
    prometheus.MustRegister(requestCount)
    prometheus.MustRegister(requestDuration)
}
