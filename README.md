# Mattermost Emoji Plugin

A comprehensive emoji manager plugin for Mattermost that allows users to manage custom emoji through slash commands.

## Description

This plugin provides a `/emoji` slash command that enables users to add, list, and remove custom emoji using Mattermost's native emoji system. Emoji are added from image URLs and stored in Mattermost's database.

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
  Shows all custom emoji in the Mattermost server with their names and IDs.

- **`/emoji add [name] [image URL]`** - Add emoji from an image URL
  ```
  /emoji add myemoji https://example.com/image.png
  ```
  Adds a custom emoji from an image URL. The URL must be accessible and point to a valid image file (PNG, JPG, or GIF).
  The emoji name should not include colons (e.g., use `myemoji` not `:myemoji:`).

- **`/emoji rm [name]`** - Remove an emoji
  ```
  /emoji rm myemoji
  ```
  Removes the specified custom emoji from Mattermost.

### Features

- **Native Mattermost Integration**: Uses Mattermost's built-in emoji system for storage and management
- **URL Validation**: Image URLs are validated to ensure they're accessible and point to valid images
- **SSRF Protection**: URLs are checked to prevent access to internal/private networks
- **Input Validation**: Emoji names must contain only letters, numbers, hyphens, and underscores
- **Security Features**:
  - HTTP timeouts (10s) to prevent hanging requests
  - DNS timeouts (5s) to prevent DNS rebinding attacks
  - Image size limit (5MB) to prevent DoS attacks
  - Redirect limit (max 10) to prevent redirect loops
- **User Session Management**: Uses the user's session to create/delete emoji with proper permissions

### Testing

The plugin's HTTP endpoint is available at:
```
http://your-mattermost-server/plugins/com.mattermost.emoji-plugin
```

Custom emoji added through the plugin will be available system-wide and can be used in any message with the `:emoji-name:` syntax.

## Structure

- `plugin.json` - Plugin manifest file
- `server/plugin.go` - Main server-side plugin code
- `go.mod` - Go module dependencies
- `Makefile` - Build configuration

## License

MIT License - see LICENSE file for details
