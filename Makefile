# Makefile for building the Mattermost emoji plugin

PLUGIN_ID = com.mattermost.emoji-plugin
PLUGIN_VERSION = 0.1.0

.PHONY: all
all: dist

.PHONY: dist
dist: server
	mkdir -p dist/$(PLUGIN_ID)
	cp plugin.json dist/$(PLUGIN_ID)/
	mkdir -p dist/$(PLUGIN_ID)/server
	cp server/plugin-* dist/$(PLUGIN_ID)/server/ 2>/dev/null || true
	cd dist && tar -czvf $(PLUGIN_ID)-$(PLUGIN_VERSION).tar.gz $(PLUGIN_ID)/
	rm -rf dist/$(PLUGIN_ID)

.PHONY: server
server:
	cd server && env GOOS=linux GOARCH=amd64 go build -o plugin-linux-amd64 .
	cd server && env GOOS=darwin GOARCH=amd64 go build -o plugin-darwin-amd64 .
	cd server && env GOOS=windows GOARCH=amd64 go build -o plugin-windows-amd64.exe .

.PHONY: clean
clean:
	rm -rf dist
	rm -f server/plugin-*
