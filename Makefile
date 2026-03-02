PREFIX = /usr
BIN_DIR = ${PREFIX}/sbin
PKG_NAME = deepin-power-control
BINARY_NAME := deepin-power-control
# Full name for directories
SHARE_DIR = ${PREFIX}/share/${PKG_NAME}
DATA_DIR = ${PREFIX}/share/${PKG_NAME}
CONF_DIR = /etc/${PKG_NAME}

# Environment settings
export GOCACHE := /tmp/gocache
export GO111MODULE := off
GOPATH = /usr/share/gocode

GOPATH_DIR = gopath
GOPKG_PREFIX = ${PKG_NAME}
GOCMD = go

# Hardening flags for distribution
ifneq ($(filter $(DEB_BUILD_ARCH), amd64 arm64),)
    DSPC_GO_BUILD_OPTIONS += -buildmode=pie -ldflags '-linkmode=external -extldflags "-Wl,-z,noexecstack,-z,relro,-z,now"'
    DSPC_CGO_FLAGS += CGO_CFLAGS="-fstack-protector-strong -D_FORTIFY_SOURCE=2 -O2 -ftrapv"
endif

GOBUILD = $(GOCMD) build $(GO_BUILD_FLAGS)
GOCLEAN = $(GOCMD) clean

all: build

prepare:
	@echo "> Preparing build environment..."
	@mkdir -p out
	@mkdir -p ${GOPATH_DIR}/src/$(dir ${GOPKG_PREFIX})
	@ln -snf ../.. ${GOPATH_DIR}/src/${GOPKG_PREFIX}

	@if [ "${DEB_BUILD_ARCH}" = "loongarch64" ] && [ -d "vendor" ]; then \
		echo "> Linking dependencies from vendor..."; \
		for author in $$(ls vendor/github.com/); do \
			mkdir -p ${GOPATH_DIR}/src/github.com/$$author; \
			ln -snf ${CURDIR}/vendor/github.com/$$author/* ${GOPATH_DIR}/src/github.com/$$author/; \
		done \
	fi

fmt:
	@echo "> Formatting code..."
	env GOPATH="${CURDIR}/${GOPATH_DIR}:${GOPATH}" $(GOCMD) fmt ./...

build: prepare out/${BINARY_NAME}

out/${BINARY_NAME}: prepare
	@echo "> Compiling ${BINARY_NAME} binary..."
	env GOPATH="${CURDIR}/${GOPATH_DIR}:${GOPATH}" ${DSPC_CGO_FLAGS} \
	$(GOBUILD) -o out/${BINARY_NAME} ${DSPC_GO_BUILD_OPTIONS} ./cmd/${PKG_NAME}

install:
	@echo "> Installing files..."
	# Install binary
	mkdir -p ${DESTDIR}${BIN_DIR}
	install -m 755 out/${BINARY_NAME} ${DESTDIR}${BIN_DIR}/${BINARY_NAME}

	# 2. Install config structure to /usr/share/${PACKAGE}
	# Use cp -r to preserve directory tree (including override and its sub-dirs)
	mkdir -p ${DESTDIR}${SHARE_DIR}/configs
	# Ensure correct permissions after recursive copy
	if [ -d "configs" ]; then \
		cp -rf configs/* ${DESTDIR}${SHARE_DIR}/configs/; \
		find ${DESTDIR}${SHARE_DIR}/configs -type d -exec chmod 755 {} +; \
		find ${DESTDIR}${SHARE_DIR}/configs -type f -exec chmod 644 {} +; \
	fi

clean:
	@echo "> Cleaning up..."
	@rm -rf out/*
	@rm -rf ${GOPATH_DIR}
	@${GOCLEAN}

test: prepare
	@echo "> Running tests..."
	env GOPATH="${CURDIR}/${GOPATH_DIR}:${GOPATH}" $(GOCMD) test ./...

.PHONY: all prepare fmt build install clean test