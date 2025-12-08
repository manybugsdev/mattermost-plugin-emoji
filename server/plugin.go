package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/plugin"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin
	httpClient *http.Client
}

// CustomEmoji represents a custom emoji stored in the plugin's KVStore
type CustomEmoji struct {
	Name      string `json:"name"`
	Content   string `json:"content"`
	Type      string `json:"type"` // "url" or "text"
	CreatorID string `json:"creator_id"`
	CreatedAt int64  `json:"created_at"`
}

const (
	emojiKeyPrefix      = "emoji_"
	httpTimeout         = 10 * time.Second
	maxImageSize        = 5 * 1024 * 1024 // 5MB
	maxKVListSize       = 1000
)

// OnActivate is called when the plugin is activated
func (p *Plugin) OnActivate() error {
	// Initialize HTTP client with timeout and security settings
	p.httpClient = &http.Client{
		Timeout: httpTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Limit redirects to 10
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          "emoji",
		AutoComplete:     true,
		AutoCompleteDesc: "Manage custom emoji",
		AutoCompleteHint: "[ls|add|rm] [arguments]",
		DisplayName:      "Emoji Manager",
		Description:      "Manage custom emoji in your workspace",
	}); err != nil {
		return err
	}
	return nil
}

// ExecuteCommand handles the /emoji command
func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	split := strings.Fields(args.Command)
	if len(split) < 2 {
		return p.respondWithHelp(), nil
	}

	subcommand := split[1]

	switch subcommand {
	case "ls":
		return p.handleList(args.UserId)
	case "add":
		if len(split) < 4 {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Usage: `/emoji add [key] [URL or text]`",
			}, nil
		}
		key := split[2]
		value := strings.Join(split[3:], " ")
		return p.handleAdd(args.UserId, key, value)
	case "rm":
		if len(split) < 3 {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Usage: `/emoji rm [key]`",
			}, nil
		}
		key := split[2]
		return p.handleRemove(args.UserId, key)
	default:
		return p.respondWithHelp(), nil
	}
}

// respondWithHelp returns help text for the emoji command
func (p *Plugin) respondWithHelp() *model.CommandResponse {
	helpText := `#### Emoji Manager Commands
* **/emoji ls** - Show all custom emoji
* **/emoji add [key] [URL]** - Add emoji from image URL
* **/emoji add [key] [text]** - Add emoji from text
* **/emoji rm [key]** - Remove emoji`

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         helpText,
	}
}

// handleList lists all custom emoji
func (p *Plugin) handleList(userID string) (*model.CommandResponse, *model.AppError) {
	keys, err := p.API.KVList(0, maxKVListSize)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error listing emoji: " + err.Error(),
		}, nil
	}

	var emojiList []string
	for _, key := range keys {
		if strings.HasPrefix(key, emojiKeyPrefix) {
			data, err := p.API.KVGet(key)
			if err != nil {
				continue
			}

			var emoji CustomEmoji
			if err := json.Unmarshal(data, &emoji); err != nil {
				continue
			}

			emojiList = append(emojiList, fmt.Sprintf("* **:%s:** - %s (%s)", emoji.Name, emoji.Content, emoji.Type))
		}
	}

	if len(emojiList) == 0 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "No custom emoji found. Use `/emoji add [key] [URL or text]` to add one.",
		}, nil
	}

	text := "#### Custom Emoji\n" + strings.Join(emojiList, "\n")
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         text,
	}, nil
}

// handleAdd adds a new custom emoji
func (p *Plugin) handleAdd(userID, key, value string) (*model.CommandResponse, *model.AppError) {
	// Validate emoji key
	if !isValidEmojiName(key) {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Invalid emoji name. Emoji names must contain only letters, numbers, hyphens, and underscores.",
		}, nil
	}

	// Check if emoji already exists
	existingKey := emojiKeyPrefix + key
	existingData, _ := p.API.KVGet(existingKey)
	if existingData != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Emoji `:%s:` already exists. Use `/emoji rm %s` to remove it first.", key, key),
		}, nil
	}

	// Determine if value is URL or text
	emojiType := "text"
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		emojiType = "url"
		
		// Validate URL to prevent SSRF attacks
		if err := validateURL(value); err != nil {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Invalid URL: " + err.Error(),
			}, nil
		}
		
		// Validate URL is accessible
		resp, err := p.httpClient.Get(value)
		if err != nil {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Failed to access URL: " + err.Error(),
			}, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         fmt.Sprintf("URL returned status code: %d", resp.StatusCode),
			}, nil
		}

		// Check if content type is an image
		contentType := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "URL must point to an image file.",
			}, nil
		}
	}

	// Create custom emoji
	emoji := CustomEmoji{
		Name:      key,
		Content:   value,
		Type:      emojiType,
		CreatorID: userID,
		CreatedAt: model.GetMillis(),
	}

	// Save to KVStore
	data, err := json.Marshal(emoji)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error saving emoji: " + err.Error(),
		}, nil
	}

	if err := p.API.KVSet(existingKey, data); err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error saving emoji: " + err.Error(),
		}, nil
	}

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         fmt.Sprintf("Successfully added emoji `:%s:` (%s)", key, emojiType),
	}, nil
}

// handleRemove removes a custom emoji
func (p *Plugin) handleRemove(userID, key string) (*model.CommandResponse, *model.AppError) {
	emojiKey := emojiKeyPrefix + key
	data, err := p.API.KVGet(emojiKey)
	if err != nil || data == nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Emoji `:%s:` not found.", key),
		}, nil
	}

	if err := p.API.KVDelete(emojiKey); err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error removing emoji: " + err.Error(),
		}, nil
	}

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         fmt.Sprintf("Successfully removed emoji `:%s:`", key),
	}, nil
}

// isValidEmojiName checks if an emoji name is valid
func isValidEmojiName(name string) bool {
	if name == "" {
		return false
	}
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || 
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

// validateURL checks if a URL is valid and not pointing to internal/private networks
func validateURL(urlStr string) error {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %v", err)
	}

	// Only allow http and https schemes
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("only http and https schemes are allowed")
	}

	// Check if hostname resolves to a private IP
	host := parsedURL.Hostname()
	if host == "" {
		return fmt.Errorf("invalid hostname")
	}

	// Resolve the hostname to IP addresses with timeout
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: 5 * time.Second,
			}
			return d.DialContext(ctx, network, address)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	ips, err := resolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname: %v", err)
	}

	// Check if any resolved IP is private
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("URLs pointing to private/internal networks are not allowed")
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is private, loopback, or link-local
func isPrivateIP(ip net.IP) bool {
	// Check for loopback addresses
	if ip.IsLoopback() {
		return true
	}

	// Check for link-local addresses
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private IPv4 ranges
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
		// 169.254.0.0/16 (link-local)
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
	}

	// Check for private IPv6 ranges
	if ip.To4() == nil {
		// fc00::/7 (Unique Local Addresses)
		if len(ip) >= 1 && (ip[0]&0xfe) == 0xfc {
			return true
		}
	}

	return false
}

// ServeHTTP demonstrates a plugin that handles HTTP requests
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	// Extract emoji name from path
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		fmt.Fprint(w, "Emoji Plugin - Use /emoji command to manage custom emoji")
		return
	}

	// Get emoji from KVStore
	emojiKey := emojiKeyPrefix + path
	data, err := p.API.KVGet(emojiKey)
	if err != nil || data == nil {
		http.Error(w, "Emoji not found", http.StatusNotFound)
		return
	}

	var emoji CustomEmoji
	if err := json.Unmarshal(data, &emoji); err != nil {
		http.Error(w, "Error parsing emoji data", http.StatusInternalServerError)
		return
	}

	// If emoji is a URL, fetch and return the image
	if emoji.Type == "url" {
		// Validate URL before fetching
		if err := validateURL(emoji.Content); err != nil {
			http.Error(w, "Invalid emoji URL", http.StatusBadRequest)
			return
		}

		resp, err := p.httpClient.Get(emoji.Content)
		if err != nil {
			http.Error(w, "Error fetching emoji image", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		// Copy content type
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		
		// Limit the amount of data read to prevent DoS
		limitedReader := io.LimitReader(resp.Body, maxImageSize)
		if _, err := io.Copy(w, limitedReader); err != nil {
			p.API.LogError("Failed to copy emoji image", "error", err.Error())
			http.Error(w, "Error serving emoji image", http.StatusInternalServerError)
		}
		return
	}

	// Otherwise return text representation
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, emoji.Content)
}

func main() {
	plugin.ClientMain(&Plugin{})
}
