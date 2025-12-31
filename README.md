# Run FluxCache proxy
go run main.go -port 8080 -cache ./mycache

# Set GOPROXY
export GOPROXY=http://localhost:8080,direct

# go get bất kỳ module → cache hit lần sau → nhanh vl
go get github.com/gin-gonic/gin@latest
