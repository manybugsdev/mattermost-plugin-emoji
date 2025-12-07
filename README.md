# Mattermost Emoji Plugin

A simple hello-world emoji manager plugin for Mattermost.

## Description

This plugin demonstrates the basic structure of a Mattermost server plugin. It implements a simple HTTP endpoint that responds with "Hello, world!" when accessed.

## Installation

1. Download the latest release from the releases page
2. Upload the `.tar.gz` file to your Mattermost server via **System Console > Plugins > Plugin Management**
3. Enable the plugin

## Building

To build the plugin:

```bash
make
```

This will:
- Build the server executable for Linux, macOS, and Windows
- Package everything into a distributable `.tar.gz` file in the `dist/` directory

## Development

### Prerequisites

- Go 1.19 or higher
- Make

### Building for a specific platform

```bash
# Linux
cd server && GOOS=linux GOARCH=amd64 go build -o plugin-linux-amd64 .

# macOS
cd server && GOOS=darwin GOARCH=amd64 go build -o plugin-darwin-amd64 .

# Windows
cd server && GOOS=windows GOARCH=amd64 go build -o plugin-windows-amd64.exe .
```

### Testing

Once installed and enabled, the plugin's HTTP endpoint will be available at:
```
http://your-mattermost-server/plugins/com.mattermost.emoji-plugin
```

This endpoint will respond with "Hello, world!"

## Structure

- `plugin.json` - Plugin manifest file
- `server/plugin.go` - Main server-side plugin code
- `go.mod` - Go module dependencies
- `Makefile` - Build configuration

## License

MIT License - see LICENSE file for details
