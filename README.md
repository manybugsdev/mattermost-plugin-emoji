# Mattermost Emoji Plugin

A comprehensive emoji manager plugin for Mattermost that allows users to manage custom emoji through slash commands.

## Description

This plugin provides a `/emoji` slash command that enables users to add, list, and remove custom emoji stored in the plugin's key-value store. Emoji can be added from image URLs or as text representations.

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

## Usage

Once installed and enabled, use the `/emoji` slash command to manage custom emoji:

### Commands

- **`/emoji ls`** - List all custom emoji
  ```
  /emoji ls
  ```
  Shows all custom emoji stored in the plugin with their names, content, and types.

- **`/emoji add [key] [URL]`** - Add emoji from an image URL
  ```
  /emoji add myemoji https://example.com/image.png
  ```
  Adds a custom emoji from an image URL. The URL must be accessible and point to a valid image file.

- **`/emoji add [key] [text]`** - Add emoji from text
  ```
  /emoji add textmoji Hello World!
  ```
  Adds a custom emoji with a text representation.

- **`/emoji rm [key]`** - Remove an emoji
  ```
  /emoji rm myemoji
  ```
  Removes the specified custom emoji.

### Features

- **KVStore Storage**: Custom emoji are stored in the plugin's key-value store
- **URL Validation**: Image URLs are validated to ensure they're accessible and point to valid images
- **SSRF Protection**: URLs are checked to prevent access to internal/private networks
- **Input Validation**: Emoji names must contain only letters, numbers, hyphens, and underscores
- **Security Features**:
  - HTTP timeouts (10s) to prevent hanging requests
  - DNS timeouts (5s) to prevent DNS rebinding attacks
  - Image size limit (5MB) to prevent DoS attacks
  - Redirect limit (max 10) to prevent redirect loops
- **HTTP Endpoint**: Serves emoji images via HTTP at `/plugins/com.mattermost.emoji-plugin/[emoji-name]`

### Testing

The plugin's HTTP endpoint is available at:
```
http://your-mattermost-server/plugins/com.mattermost.emoji-plugin
```

Individual emoji can be accessed at:
```
http://your-mattermost-server/plugins/com.mattermost.emoji-plugin/[emoji-name]
```

## Structure

- `plugin.json` - Plugin manifest file
- `server/plugin.go` - Main server-side plugin code
- `go.mod` - Go module dependencies
- `Makefile` - Build configuration

## License

MIT License - see LICENSE file for details
