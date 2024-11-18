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

package main

import (
    "fmt"
    "io"
    "log"
    "mime"
    "net/http"
    "os"
    "strconv"
    "strings"
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
    FormatHeader       = "X-Format"
    CodeHeader         = "X-Code"
    ContentType        = "Content-Type"
    OriginalURI        = "X-Original-URI"
    Namespace          = "X-Namespace"
    IngressName        = "X-Ingress-Name"
    ServiceName        = "X-Service-Name"
    ServicePort        = "X-Service-Port"
    RequestId          = "X-Request-ID"
    ErrFilesPathVar    = "ERROR_FILES_PATH"
    DefaultFormatVar   = "DEFAULT_RESPONSE_FORMAT"
)

var (
    requestCount    = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "default_http_backend",
            Subsystem: "http",
            Name:      "request_count_total",
            Help:      "Total number of HTTP requests made.",
        },
        []string{"proto"},
    )
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "default_http_backend",
            Subsystem: "http",
            Name:      "request_duration_seconds",
            Help:      "Histogram of the duration (in seconds) of HTTP requests.",
            Buckets:   prometheus.DefBuckets,
        },
        []string{"proto"},
    )
)

func init() {
    prometheus.MustRegister(requestCount)
    prometheus.MustRegister(requestDuration)
}

func main() {
    errFilesPath := getEnv(ErrFilesPathVar, "/www")
    defaultFormat := getEnv(DefaultFormatVar, "text/html")

    http.HandleFunc("/", errorHandler(errFilesPath, defaultFormat))
    http.Handle("/metrics", promhttp.Handler())
    http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    log.Fatal(http.ListenAndServe(":8080", nil))
}

func getEnv(key, fallback string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return fallback
}

func errorHandler(path, defaultFormat string) func(http.ResponseWriter, *http.Request) {
    defaultExts, err := mime.ExtensionsByType(defaultFormat)
    if err != nil || len(defaultExts) == 0 {
        panic("couldn't get file extension for default format")
    }
    defaultExt := defaultExts[0]

    return func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        ext := defaultExt

        if os.Getenv("DEBUG") != "" {
            copyHeaders(w, r)
        }

        format := r.Header.Get(FormatHeader)
        if format == "" {
            format = defaultFormat
            log.Printf("format not specified. Using %v", format)
        }

        cext, err := mime.ExtensionsByType(format)
        if err != nil || len(cext) == 0 {
            log.Printf("unexpected error reading media type extension: %v. Using %v", err, ext)
            format = defaultFormat
        } else {
            ext = cext[0]
        }
        w.Header().Set(ContentType, format)

        code, err := strconv.Atoi(r.Header.Get(CodeHeader))
        if err != nil {
            code = 404
            log.Printf("unexpected error reading return code: %v. Using %v", err, code)
        }
        w.WriteHeader(code)

        if !strings.HasPrefix(ext, ".") {
            ext = "." + ext
        }
        if ext == ".htm" {
            ext = ".html"
        }

        file := fmt.Sprintf("%v/%v%v", path, code, ext)
        if err := serveFile(w, r, file, code, format); err != nil {
            scode := strconv.Itoa(code)
            file := fmt.Sprintf("%v/%cxx%v", path, scode[0], ext)
            if err := serveFile(w, r, file, code, format); err != nil {
                http.NotFound(w, r)
            }
        }

        duration := time.Since(start).Seconds()
        proto := fmt.Sprintf("%d.%d", r.ProtoMajor, r.ProtoMinor)
        requestCount.WithLabelValues(proto).Inc()
        requestDuration.WithLabelValues(proto).Observe(duration)
    }
}

func copyHeaders(w http.ResponseWriter, r *http.Request) {
    headers := []string{FormatHeader, CodeHeader, ContentType, OriginalURI, Namespace, IngressName, ServiceName, ServicePort, RequestId}
    for _, header := range headers {
        w.Header().Set(header, r.Header.Get(header))
    }
}

func serveFile(w http.ResponseWriter, r *http.Request, file string, code int, format string) error {
    f, err := os.Open(file)
    if err != nil {
        log.Printf("unexpected error opening file: %v", err)
        return err
    }
    defer f.Close()
    log.Printf("serving custom error response for code %v and format %v from file %v", code, format, file)
    io.Copy(w, f)
    return nil
}
