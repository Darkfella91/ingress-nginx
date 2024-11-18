/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Collect and display Prometheus metrics

package main

import (
    "github.com/prometheus/client_golang/prometheus"
)

const (
    namespace = "default_http_backend"
    subsystem = "http"
)

var (
    requestCount = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: namespace,
            Subsystem: subsystem,
            Name:      "request_count_total",
            Help:      "Total number of HTTP requests made.",
        },
        []string{"proto"},
    )

    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: namespace,
            Subsystem: subsystem,
            Name:      "request_duration_seconds",
            Help:      "Histogram of the duration (in seconds) of HTTP requests.",
            Buckets:   prometheus.DefBuckets,
        },
        []string{"proto"},
    )
)

func init() {
    // Register the metrics with Prometheus
    prometheus.MustRegister(requestCount)
    prometheus.MustRegister(requestDuration)
}
