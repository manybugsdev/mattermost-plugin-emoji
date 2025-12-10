package main

import (
	"context"
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

const (
	httpTimeout     = 10 * time.Second
	maxImageSize    = 5 * 1024 * 1024 // 5MB
	maxEmojiPerPage = 200              // Maximum number of emoji to list per page
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
				Text:         "Usage: `/emoji add [name] [image URL]`",
			}, nil
		}
		name := split[2]
		imageURL := split[3]
		return p.handleAdd(c, args.UserId, name, imageURL)
	case "rm":
		if len(split) < 3 {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Usage: `/emoji rm [name]`",
			}, nil
		}
		name := split[2]
		return p.handleRemove(c, args.UserId, name)
	default:
		return p.respondWithHelp(), nil
	}
}

// respondWithHelp returns help text for the emoji command
func (p *Plugin) respondWithHelp() *model.CommandResponse {
	helpText := `#### Emoji Manager Commands
* **/emoji ls** - Show all custom emoji
* **/emoji add [name] [image URL]** - Add emoji from image URL
* **/emoji rm [name]** - Remove emoji`

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         helpText,
	}
}

// handleList lists all custom emoji
func (p *Plugin) handleList(userID string) (*model.CommandResponse, *model.AppError) {
	// Get all custom emoji using the plugin API
	emojiList, err := p.API.GetEmojiList("name", 0, maxEmojiPerPage)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error listing emoji: " + err.Error(),
		}, nil
	}

	if len(emojiList) == 0 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "No custom emoji found. Use `/emoji add [name] [URL]` to add one.",
		}, nil
	}

	var lines []string
	for _, emoji := range emojiList {
		lines = append(lines, fmt.Sprintf("* **:%s:** (ID: %s)", emoji.Name, emoji.Id))
	}

	text := "#### Custom Emoji\n" + strings.Join(lines, "\n")
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         text,
	}, nil
}

// handleAdd adds a new custom emoji from an image URL
func (p *Plugin) handleAdd(c *plugin.Context, userID, name, imageURL string) (*model.CommandResponse, *model.AppError) {
	// Validate emoji name
	if !isValidEmojiName(name) {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Invalid emoji name. Emoji names must contain only letters, numbers, hyphens, and underscores.",
		}, nil
	}

	// Check if emoji already exists
	existing, _ := p.API.GetEmojiByName(name)
	if existing != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Emoji `:%s:` already exists. Use `/emoji rm %s` to remove it first.", name, name),
		}, nil
	}

	// Validate URL to prevent SSRF attacks
	if err := validateURL(imageURL); err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Invalid URL: " + err.Error(),
		}, nil
	}

	// Download the image from URL
	resp, err := p.httpClient.Get(imageURL)
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

	// Read image data with size limit
	limitedReader := io.LimitReader(resp.Body, maxImageSize)
	imageData, err := io.ReadAll(limitedReader)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Failed to download image: " + err.Error(),
		}, nil
	}

	// Create API client with user's session
	client, err := p.createAPIClient(c)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error: " + err.Error(),
		}, nil
	}

	// Determine filename from content type
	filename := name
	if strings.Contains(contentType, "png") {
		filename += ".png"
	} else if strings.Contains(contentType, "jpeg") || strings.Contains(contentType, "jpg") {
		filename += ".jpg"
	} else if strings.Contains(contentType, "gif") {
		filename += ".gif"
	} else {
		filename += ".png" // default
	}

	// Create the emoji
	emoji := &model.Emoji{
		CreatorId: userID,
		Name:      name,
	}

	createdEmoji, _, err := client.CreateEmoji(emoji, imageData, filename)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Failed to create emoji: " + err.Error(),
		}, nil
	}

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         fmt.Sprintf("Successfully added emoji `:%s:` (ID: %s)", createdEmoji.Name, createdEmoji.Id),
	}, nil
}

// handleRemove removes a custom emoji
func (p *Plugin) handleRemove(c *plugin.Context, userID, name string) (*model.CommandResponse, *model.AppError) {
	// Get the emoji by name
	emoji, appErr := p.API.GetEmojiByName(name)
	if appErr != nil || emoji == nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Emoji `:%s:` not found.", name),
		}, nil
	}

	// Create API client with user's session
	client, err := p.createAPIClient(c)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error: " + err.Error(),
		}, nil
	}

	// Delete the emoji
	_, deleteErr := client.DeleteEmoji(emoji.Id)
	if deleteErr != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Failed to delete emoji: " + deleteErr.Error(),
		}, nil
	}

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         fmt.Sprintf("Successfully removed emoji `:%s:`", name),
	}, nil
}

// createAPIClient creates and configures an API client with the user's session token
func (p *Plugin) createAPIClient(c *plugin.Context) (*model.Client4, error) {
	// Get site URL
	config := p.API.GetConfig()
	if config.ServiceSettings.SiteURL == nil {
		return nil, fmt.Errorf("site URL is not configured")
	}

	// Create API client
	client := model.NewAPIv4Client(*config.ServiceSettings.SiteURL)

	// Get the session token from context
	session, appErr := p.API.GetSession(c.SessionId)
	if appErr != nil {
		return nil, fmt.Errorf("error getting user session: %s", appErr.Error())
	}
	client.SetToken(session.Token)

	return client, nil
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

// ServeHTTP handles HTTP requests
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Emoji Plugin - Use /emoji command to manage custom emoji")
}

func main() {
	plugin.ClientMain(&Plugin{})
}
