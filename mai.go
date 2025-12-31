// cmd/fluxcache/main.go
package main

import (
    "crypto/sha256"
    "encoding/hex"
    "flag"
    "fmt"
    "io"
    "log"
    "net/http"
    "net/url"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"
)

var (
    cacheDir   = flag.String("cache", "./fluxcache", "Thư mục cache module")
    port       = flag.Int("port", 8080, "Port proxy listen")
    upstream   = flag.String("upstream", "https://proxy.golang.org", "Upstream proxy (fallback)")
    verbose    = flag.Bool("v", false, "Verbose log")
    cacheMutex sync.RWMutex
)

type FluxCache struct {
    cacheRoot string
}

func NewFluxCache(root string) *FluxCache {
    os.MkdirAll(root, 0755)
    return &FluxCache{cacheRoot: root}
}

// hashPath tạo path cache từ URL
func (c *FluxCache) cachePath(modulePath string) string {
    h := sha256.Sum256([]byte(modulePath))
    hash := hex.EncodeToString(h[:])
    return filepath.Join(c.cacheRoot, hash[:2], hash[2:4], hash)
}

// serveCached phục vụ từ cache nếu có
func (c *FluxCache) serveFromCache(w http.ResponseWriter, modulePath string) bool {
    cachePath := c.cachePath(modulePath)
    if data, err := os.ReadFile(cachePath); err == nil {
        if *verbose {
            log.Printf("CACHE HIT: %s", modulePath)
        }
        http.ServeContent(w, nil, filepath.Base(modulePath), time.Now(), strings.NewReader(data))
        return true
    }
    return false
}

// fetchAndCache lấy từ upstream + lưu cache
func (c *FluxCache) fetchAndCache(w http.ResponseWriter, r *http.Request, modulePath string) {
    upstreamURL := *upstream + r.URL.Path + "?" + r.URL.RawQuery
    if *verbose {
        log.Printf("CACHE MISS → FETCH: %s", upstreamURL)
    }

    resp, err := http.Get(upstreamURL)
    if err != nil || resp.StatusCode != 200 {
        http.Error(w, "upstream error", http.StatusBadGateway)
        return
    }
    defer resp.Body.Close()

    data, err := io.ReadAll(resp.Body)
    if err != nil {
        http.Error(w, "read error", http.StatusInternalServerError)
        return
    }

    // Lưu cache
    cachePath := c.cachePath(modulePath)
    os.MkdirAll(filepath.Dir(cachePath), 0755)
    os.WriteFile(cachePath, data, 0644)

    // Serve client
    for k, v := range resp.Header {
        w.Header()[k] = v
    }
    w.WriteHeader(resp.StatusCode)
    w.Write(data)
}

func main() {
    flag.Parse()

    cache := NewFluxCache(*cacheDir)
    log.Printf("FluxCache starting on :%d | cache: %s | upstream: %s", *port, *cacheDir, *upstream)

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        path := r.URL.Path
        if strings.HasPrefix(path, "/") {
            path = path[1:]
        }

        // Decode module path (Go module proxy format)
        modulePath, err := url.PathUnescape(path)
        if err != nil {
            http.Error(w, "bad path", http.StatusBadRequest)
            return
        }

        // Cache hit?
        if cache.serveFromCache(w, modulePath) {
            return
        }

        // Cache miss → fetch upstream
        cache.fetchAndCache(w, r, modulePath)
    })

    log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
