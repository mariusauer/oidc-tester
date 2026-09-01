APP_NAME := oidc-tester
DIST_DIR := dist
LDFLAGS := -s -w

.PHONY: build test linux windows clean

build: linux windows

test:
	go test ./...

linux:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 .

windows:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe .

clean:
	rm -rf $(DIST_DIR)
