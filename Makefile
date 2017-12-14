.DEFAULT_GOAL := all
PROJECT_ROOT := src/

NAME := spotitube
CUR_DIR = $(shell pwd)
BINARY := $(CUR_DIR)/out/$(NAME)
BINARY_INSTALL_PATH := /usr/local/sbin
BINARY_INSTALL := $(BINARY_INSTALL_PATH)/$(NAME)
VERSION := $(shell awk -F'= ' '/VERSION / {print $$2}' src/system/constants.go)
PKG_NAME := $(BINARY)-v$(VERSION)
GOARCH := amd64

include Makefile.gobuild
include Makefile.packaging

install: bin
	@ ( \
		echo -en "Installing...\r"; \
		(test -d $(BINARY_INSTALL_PATH) || install -D -d -m 00755 $(BINARY_INSTALL_PATH)) && \
		install -m 00755 $(BINARY) $(BINARY_INSTALL_PATH)/ && \
		echo -e "\rInstalled at: $(BINARY_INSTALL)"; \
	);

clean:
	@ ( \
		echo -en "Cleaning...\r"; \
		(test ! -d $(CUR_DIR)/pkg || rm -rf $(CUR_DIR)/pkg) && \
		(test ! -d $(CUR_DIR)/out || rm -rf $(CUR_DIR)/out) && \
		rm -rf $(BINARY)* && \
		echo -e "\rCleaned workspace."; \
	);


all: deps bin
