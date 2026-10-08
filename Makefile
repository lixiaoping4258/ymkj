# xTravel -> Kratos 重构工程
#
# Windows 上如果没装 make，用 scripts/gen.ps1 等价完成代码生成，
# 其余目标都是 go 命令，直接敲也一样。

GOHOSTOS     := $(shell go env GOHOSTOS)
VERSION      := $(shell git describe --tags --always 2>/dev/null || echo dev)
GitCommit    := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS      := -X main.Version=$(VERSION)

.PHONY: help
help:
	@echo "make init      安装 protoc 插件和工具链"
	@echo "make api       生成 proto 代码（Windows 用 scripts/gen.ps1）"
	@echo "make wire      生成依赖注入代码"
	@echo "make build     编译到 bin/"
	@echo "make run       本地运行（用 configs/config.local.yaml）"
	@echo "make test      跑单元测试"
	@echo "make lint      静态检查"
	@echo "make fmt       格式化"

.PHONY: init
init:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
	go install github.com/google/wire/cmd/wire@latest

# 注意：Windows 上 protoc 用 --plugin=<绝对路径> 会报
# "filename, directory name, or volume label syntax is incorrect"，
# 必须让 protoc 从 PATH 找插件。scripts/gen.ps1 已经处理了这点。
.PHONY: api
api:
ifeq ($(GOHOSTOS), windows)
	powershell -ExecutionPolicy Bypass -File ./scripts/gen.ps1
else
	protoc --proto_path=. --proto_path=./api --proto_path=./third_party \
	  --go_out=paths=source_relative:. \
	  --go-http_out=paths=source_relative:. \
	  --go-grpc_out=paths=source_relative:. \
	  $(shell find api -name '*.proto')
	protoc --proto_path=. --proto_path=./third_party \
	  --go_out=paths=source_relative:. internal/conf/conf.proto
endif

.PHONY: wire
wire:
	wire ./cmd/xtravel

.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o bin/xtravel ./cmd/xtravel

.PHONY: build-all
build-all:
	go build ./...

.PHONY: run
run:
	go run ./cmd/xtravel -conf configs/config.local.yaml

.PHONY: test
test:
	go test ./... -count=1

.PHONY: lint
lint:
	go vet ./...

.PHONY: fmt
fmt:
	gofmt -s -w .
	go mod tidy

.PHONY: clean
clean:
	rm -rf bin/ logs/
