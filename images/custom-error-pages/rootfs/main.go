/*
Copyright 2017 The Kubernetes Authors.

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
	"sort"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// FormatHeader name of the header used to extract the format
	FormatHeader = "X-Format"

	// CodeHeader name of the header used as source of the HTTP status code to return
	CodeHeader = "X-Code"

	// ContentType name of the header that defines the format of the reply
	ContentType = "Content-Type"

	// OriginalURI name of the header with the original URL from NGINX
	OriginalURI = "X-Original-URI"

	// Namespace name of the header that contains information about the Ingress namespace
	Namespace = "X-Namespace"

	// IngressName name of the header that contains the matched Ingress
	IngressName = "X-Ingress-Name"

	// ServiceName name of the header that contains the matched Service in the Ingress
	ServiceName = "X-Service-Name"

	// ServicePort name of the header that contains the matched Service port in the Ingress
	ServicePort = "X-Service-Port"

	// RequestId is a unique ID that identifies the request - same as for backend service
	RequestId = "X-Request-ID"

	// ErrFilesPathVar is the name of the environment variable indicating
	// the location on disk of files served by the handler.
	ErrFilesPathVar = "ERROR_FILES_PATH"

	// DefaultFormatVar is the name of the environment variable indicating
	// the default error MIME type that should be returned if either the
	// client does not specify an Accept header, or the Accept header provided
	// cannot be mapped to a file extension.
	DefaultFormatVar = "DEFAULT_RESPONSE_FORMAT"
)

func init() {
	prometheus.MustRegister(requestCount)
	prometheus.MustRegister(requestDuration)
}

func main() {
	errFilesPath := "/www"
	if os.Getenv(ErrFilesPathVar) != "" {
		errFilesPath = os.Getenv(ErrFilesPathVar)
	}

	defaultFormat := "text/html"
	if os.Getenv(DefaultFormatVar) != "" {
		defaultFormat = os.Getenv(DefaultFormatVar)
	}

	http.HandleFunc("/", errorHandler(errFilesPath, defaultFormat))

	http.Handle("/metrics", promhttp.Handler())

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	http.ListenAndServe(fmt.Sprintf(":8080"), nil)
}

func parseAcceptHeader(header string) []string {
    parts := strings.Split(header, ",")
    typeQualityPairs := make([]struct {
        mediaType string
        quality   float64
    }, len(parts))

    for i, part := range parts {
        part = strings.TrimSpace(part)
        mediaType := part
        quality := 1.0

        if qIndex := strings.Index(part, ";q="); qIndex != -1 {
            mediaType = part[:qIndex]
            qValue := part[qIndex+3:]
            if q, err := strconv.ParseFloat(qValue, 64); err == nil {
                quality = q
            }
        }

        typeQualityPairs[i] = struct {
            mediaType string
            quality   float64
        }{mediaType, quality}
    }

    sort.Slice(typeQualityPairs, func(i, j int) bool {
        return typeQualityPairs[i].quality > typeQualityPairs[j].quality
    })

    mediaTypes := make([]string, len(typeQualityPairs))
    for i, pair := range typeQualityPairs {
        mediaTypes[i] = pair.mediaType
    }

    return mediaTypes
}

func selectFormat(acceptHeader string, defaultFormat string) (string, string) {
    mediaTypes := parseAcceptHeader(acceptHeader)
    for _, mediaType := range mediaTypes {
        if mediaType == "application/json" || mediaType == "text/html" {
            cext, _ := mime.ExtensionsByType(mediaType)
            if len(cext) > 0 {
                return mediaType, cext[0]
            }
        }
    }
    cext, _ := mime.ExtensionsByType(defaultFormat)
    return defaultFormat, cext[0]
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
			w.Header().Set(FormatHeader, r.Header.Get(FormatHeader))
			w.Header().Set(CodeHeader, r.Header.Get(CodeHeader))
			w.Header().Set(ContentType, r.Header.Get(ContentType))
			w.Header().Set(OriginalURI, r.Header.Get(OriginalURI))
			w.Header().Set(Namespace, r.Header.Get(Namespace))
			w.Header().Set(IngressName, r.Header.Get(IngressName))
			w.Header().Set(ServiceName, r.Header.Get(ServiceName))
			w.Header().Set(ServicePort, r.Header.Get(ServicePort))
			w.Header().Set(RequestId, r.Header.Get(RequestId))
		}

		format := r.Header.Get(FormatHeader)
	        var ext string
	        if format == "" {
	            acceptHeader := r.Header.Get("Accept")
	            format, ext = selectFormat(acceptHeader, defaultFormat)
	            log.Printf("Selected format: %v, extension: %v", format, ext)
	        } else {
	            cext, _ := mime.ExtensionsByType(format)
	            if len(cext) > 0 {
	                ext = cext[0]
	            } else {
	                format = defaultFormat
	                cext, _ = mime.ExtensionsByType(defaultFormat)
	                ext = cext[0]
	            }
	        }
		
		w.Header().Set(ContentType, format)

		codeStr := r.Header.Get(CodeHeader)
	        if codeStr == "" {
	            codeStr = "404"
	        }
		
		code, err := strconv.Atoi(codeStr)
		if err != nil {
			code = 404
			log.Printf("unexpected error reading return code: %v. Using %v", err, code)
		}
		w.WriteHeader(code)

		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		// special case for compatibility
		if ext == ".htm" {
			ext = ".html"
		}
		file := fmt.Sprintf("%v/%v%v", path, code, ext)
		f, err := os.Open(file)
		if err != nil {
			log.Printf("unexpected error opening file: %v", err)
			scode := strconv.Itoa(code)
			file := fmt.Sprintf("%v/%cxx%v", path, scode[0], ext)
			f, err := os.Open(file)
			if err != nil {
				log.Printf("unexpected error opening file: %v", err)
				http.NotFound(w, r)
				return
			}
			defer f.Close()
			log.Printf("serving custom error response for code %v and format %v from file %v", code, format, file)
			io.Copy(w, f)
			return
		}
		defer f.Close()
		log.Printf("serving custom error response for code %v and format %v from file %v", code, format, file)
		io.Copy(w, f)

		duration := time.Now().Sub(start).Seconds()

		proto := strconv.Itoa(r.ProtoMajor)
		proto = fmt.Sprintf("%s.%s", proto, strconv.Itoa(r.ProtoMinor))

		requestCount.WithLabelValues(proto).Inc()
		requestDuration.WithLabelValues(proto).Observe(duration)
	}
}
