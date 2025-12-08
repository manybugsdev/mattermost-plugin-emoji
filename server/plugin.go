package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/plugin"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin
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
	emojiKeyPrefix = "emoji_"
)

// OnActivate is called when the plugin is activated
func (p *Plugin) OnActivate() error {
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
	keys, err := p.API.KVList(0, 1000)
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
		
		// Validate URL is accessible
		resp, err := http.Get(value)
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
		resp, err := http.Get(emoji.Content)
		if err != nil {
			http.Error(w, "Error fetching emoji image", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		// Copy content type
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		io.Copy(w, resp.Body)
		return
	}

	// Otherwise return text representation
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, emoji.Content)
}

func main() {
	plugin.ClientMain(&Plugin{})
}
