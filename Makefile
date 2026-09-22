export GO111MODULE=on

VERSION=$(shell date +"%Y.%m.%d")
BUILD=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BASEDIR=./builds

LDFLAGS=-ldflags "-s -w -X main.build=${BUILD} -buildid=${BUILD}"
GCFLAGS=-gcflags=all=-trimpath=$(shell echo ${HOME})
ASMFLAGS=-asmflags=all=-trimpath=$(shell echo ${HOME})
BUILDFLAGS=-trimpath ${LDFLAGS} ${GCFLAGS} ${ASMFLAGS}

GOFILES=`go list -buildvcs=false ./...`
GOFILESNOTEST=`go list -buildvcs=false ./... | grep -v test`

$(shell mkdir -p ${BASEDIR})

all: proxy agents
	@echo ""
	@echo "Build complete:"
	@ls -lhS ${BASEDIR}/

proxy: lint proxy-linux-amd64 proxy-linux-arm64

proxy-linux-amd64:
	@echo "Building proxy linux/amd64..."
	@env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags tailcat ${BUILDFLAGS} -o ${BASEDIR}/ligotail-proxy-linux_amd64 ./cmd/proxy/

proxy-linux-arm64:
	@echo "Building proxy linux/arm64..."
	@env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags tailcat ${BUILDFLAGS} -o ${BASEDIR}/ligotail-proxy-linux_arm64 ./cmd/proxy/

agents: lint agents-linux agents-windows
	@chmod +x ${BASEDIR}/*agent*

agents-linux:
	@for arch in amd64 386 arm64; do \
		echo "Building agents linux/$$arch..."; \
		env CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build ${BUILDFLAGS} -o ${BASEDIR}/ligolo-native-agent-linux_$$arch ./cmd/agent/; \
		env CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -tags tailcat ${BUILDFLAGS} -o ${BASEDIR}/ligotail-agent-linux_$$arch ./cmd/agent/; \
	done
	@echo "Building agents linux/arm (v7)..."
	@env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build ${BUILDFLAGS} -o ${BASEDIR}/ligolo-native-agent-linux_armv7 ./cmd/agent/
	@env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -tags tailcat ${BUILDFLAGS} -o ${BASEDIR}/ligotail-agent-linux_armv7 ./cmd/agent/
	@echo "Building agents linux/arm (v6)..."
	@env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build ${BUILDFLAGS} -o ${BASEDIR}/ligolo-native-agent-linux_armv6 ./cmd/agent/
	@env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build -tags tailcat ${BUILDFLAGS} -o ${BASEDIR}/ligotail-agent-linux_armv6 ./cmd/agent/
	@for arch in mips mipsle mips64 mips64le; do \
		echo "Building agents linux/$$arch (softfloat)..."; \
		env CGO_ENABLED=0 GOOS=linux GOARCH=$$arch GOMIPS=softfloat GOMIPS64=softfloat go build ${BUILDFLAGS} -o ${BASEDIR}/ligolo-native-agent-linux_$$arch ./cmd/agent/; \
		env CGO_ENABLED=0 GOOS=linux GOARCH=$$arch GOMIPS=softfloat GOMIPS64=softfloat go build -tags tailcat ${BUILDFLAGS} -o ${BASEDIR}/ligotail-agent-linux_$$arch ./cmd/agent/; \
	done

agents-windows:
	@for arch in amd64 386 arm64; do \
		echo "Building agents windows/$$arch..."; \
		env CGO_ENABLED=0 GOOS=windows GOARCH=$$arch go build ${BUILDFLAGS} -o ${BASEDIR}/ligolo-native-agent-windows_$$arch.exe ./cmd/agent/; \
		env CGO_ENABLED=0 GOOS=windows GOARCH=$$arch go build -tags tailcat ${BUILDFLAGS} -o ${BASEDIR}/ligotail-agent-windows_$$arch.exe ./cmd/agent/; \
	done

tidy:
	@go mod tidy

update: tidy
	@go get -v -d ./...
	@go get -u all

dep:
	@go install github.com/goreleaser/goreleaser
	@go install github.com/securego/gosec/v2/cmd/gosec@latest

lint:
	@env CGO_ENABLED=0 go fmt ${GOFILES}
	@env CGO_ENABLED=0 go vet ${GOFILESNOTEST}

security:
	@gosec -tests ./...

clean:
	@rm -rf ${BASEDIR}

.PHONY: all proxy proxy-linux-amd64 proxy-linux-arm64 agents agents-linux agents-windows tidy update dep lint security clean
