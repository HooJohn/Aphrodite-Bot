package config

import (
	"log"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// LLMProvider defines the structure for LLM provider configuration.
type LLMProvider struct {
	APIKey  string // Name of the environment variable holding the API key
	BaseURL string
}

// LLMGroup defines an AI agent group.
type LLMGroup struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	Members               []string `json:"members"`    // List of LLMCharacter IDs
	IsGroupDiscussionMode bool     `mapstructure:"isGroupDiscussionMode" json:"isGroupDiscussionMode"` // Corrected mapstructure tag
}

// LLMCharacter defines an AI agent's properties.
type LLMCharacter struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Personality  string   `json:"personality"` // Internal identifier for personality/role
	Model        string   `json:"model"`       // Model name (e.g., "qwen-plus")
	Avatar       string   `json:"avatar"`      // Path to avatar image
	CustomPrompt string   `mapstructure:"custom_prompt" json:"custom_prompt"`
	Tags         []string `json:"tags"`
}

// Config holds the application's configuration.
type Config struct {
	Server struct {
		Port string
	}
	Database struct {
		DSN string // Data Source Name (e.g., "memory" or file path for SQLite)
	}
	LLMSystemPrompt string                 `mapstructure:"llm_system_prompt" json:"llm_system_prompt"` // Global system prompt for LLMs
	LLMProviders    map[string]LLMProvider `mapstructure:"llm_providers"`                             // Map of provider key to provider config
	LLMModels       map[string]string      `mapstructure:"llm_models"`                                // Map of model name to provider key
	LLMGroups       []*LLMGroup            `mapstructure:"llm_groups"`
	LLMCharacters   []*LLMCharacter        `mapstructure:"llm_characters"`
	GuestChatQuota  int                    `mapstructure:"guest_chat_quota" json:"guest_chat_quota"`
}

// AppConfig is the global configuration instance.
var AppConfig Config

// LoadConfig loads configuration from file and environment variables.
// It handles optimized loading for Tags (LLMCharacters) and Members (LLMGroups).
func LoadConfig() {
	viper.SetConfigName("config")    // Name of config file (without extension)
	viper.SetConfigType("yaml")      // REQUIRED if the config file does not have the extension in the name
	viper.AddConfigPath("./config")  // Path to look for the config file in
	viper.AddConfigPath(".")         // Optionally look for config in the working directory
	viper.AddConfigPath("../config") // For running from locations like tests

	viper.SetDefault("server.port", "8080")
	viper.SetDefault("guest_chat_quota", 20)
	// Set other critical defaults, especially if they might be missing from YAML

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("WARN: [Config] Configuration file (config.yaml) not found. Using environment variables and defaults.")
		} else {
			log.Fatalf("FATAL: [Config] Error reading configuration file: %v", err)
		}
	}

	if err := viper.Unmarshal(&AppConfig); err != nil {
		log.Fatalf("FATAL: [Config] Failed to unmarshal configuration into AppConfig struct: %v", err)
	}

	// Environment variable overrides
	if port := os.Getenv("SERVER_PORT"); port != "" {
		AppConfig.Server.Port = port
		log.Printf("INFO: [Config] Server port overridden by environment variable SERVER_PORT: %s", port)
	}

	// Load API keys for LLM providers from environment variables
	for providerKey, providerConfig := range AppConfig.LLMProviders {
		envVarNameForKey := providerConfig.APIKey // Assumes APIKey field stores the name of the environment variable
		if envValue := os.Getenv(envVarNameForKey); envValue != "" {
			updatedConfig := providerConfig
			updatedConfig.APIKey = envValue // Replace placeholder with actual key
			AppConfig.LLMProviders[providerKey] = updatedConfig
			log.Printf("INFO: [Config] Loaded API Key for provider '%s' from environment variable '%s'.", providerKey, envVarNameForKey)
		} else if providerConfig.APIKey != "" && !strings.HasSuffix(providerConfig.APIKey, "_KEY") {
			// This case means the APIKey field in YAML was likely a hardcoded key (not recommended)
			// and no corresponding environment variable was found to override it.
			log.Printf("WARN: [Config] API Key for provider '%s' is directly set in config.yaml and not overridden by env var '%s'. Consider using env vars for keys.", providerKey, envVarNameForKey)
		} else if providerConfig.APIKey == "" || strings.HasSuffix(providerConfig.APIKey, "_KEY") { // Placeholder or empty
			log.Printf("WARN: [Config] API Key for provider '%s' (env var '%s') is not set and not provided directly in config.", providerKey, envVarNameForKey)
		}
	}

	// --- Manual loading and correction for potential Viper Unmarshal issues ---
	// Correctly load LLMModels if viper.Unmarshal didn't populate map[string]string
	if len(AppConfig.LLMModels) == 0 && viper.IsSet("llm_models") {
		log.Println("INFO: [Config] LLMModels map is empty, attempting manual load from Viper.")
		AppConfig.LLMModels = viper.GetStringMapString("llm_models")
		if len(AppConfig.LLMModels) > 0 {
			log.Println("INFO: [Config] Successfully manually loaded LLMModels:", AppConfig.LLMModels)
		} else {
			log.Println("WARN: [Config] Failed to load llm_models from configuration.")
		}
	}

	// Correctly load LLMCharacters, especially ensuring 'Tags' are properly unmarshalled
	if viper.IsSet("llm_characters") {
		var charactersRaw []map[string]interface{}
		if err := viper.UnmarshalKey("llm_characters", &charactersRaw); err == nil && len(charactersRaw) > 0 {
			AppConfig.LLMCharacters = make([]*LLMCharacter, 0, len(charactersRaw))
			for _, charMap := range charactersRaw {
				var char LLMCharacter
				// Safely extract fields with type assertions
				if id, ok := charMap["id"].(string); ok { char.ID = id }
				if name, ok := charMap["name"].(string); ok { char.Name = name }
				if personality, ok := charMap["personality"].(string); ok { char.Personality = personality }
				if model, ok := charMap["model"].(string); ok { char.Model = model }
				if avatar, ok := charMap["avatar"].(string); ok { char.Avatar = avatar }
				if cp, ok := charMap["custom_prompt"].(string); ok { char.CustomPrompt = cp }
				
				if tagsVal, ok := charMap["tags"].([]interface{}); ok {
					for _, t := range tagsVal {
						if tagStr, ok := t.(string); ok {
							char.Tags = append(char.Tags, tagStr)
						}
					}
				}
				AppConfig.LLMCharacters = append(AppConfig.LLMCharacters, &char)
			}
			log.Printf("INFO: [Config] Successfully loaded and parsed %d LLMCharacters (including Tags).", len(AppConfig.LLMCharacters))
		} else if err != nil {
			log.Printf("WARN: [Config] Failed to parse 'llm_characters' using UnmarshalKey: %v. Will attempt generic Get.", err)
			// Fallback for more complex structures if UnmarshalKey fails
			charactersGeneric := viper.Get("llm_characters")
			if charactersSlice, ok := charactersGeneric.([]interface{}); ok && len(charactersSlice) > 0 {
				// This path indicates a more complex structure that might need careful handling if the above fails.
				// For now, we assume the UnmarshalKey approach with map[string]interface{} is robust enough.
				log.Printf("WARN: [Config] Loaded %d raw LLMCharacter records using viper.Get(). Tags and other fields might need specific parsing if UnmarshalKey failed.", len(charactersSlice))
			} else {
				log.Println("WARN: [Config] Failed to load 'llm_characters' or the format is incorrect.")
			}
		}
	}

	// Correctly load LLMGroups, especially ensuring 'Members' are properly unmarshalled
	if viper.IsSet("llm_groups") {
		var groupsRaw []map[string]interface{}
		if err := viper.UnmarshalKey("llm_groups", &groupsRaw); err == nil && len(groupsRaw) > 0 {
			AppConfig.LLMGroups = make([]*LLMGroup, 0, len(groupsRaw))
			for _, groupMap := range groupsRaw {
				var group LLMGroup
				if id, ok := groupMap["id"].(string); ok { group.ID = id }
				if name, ok := groupMap["name"].(string); ok { group.Name = name }
				if desc, ok := groupMap["description"].(string); ok { group.Description = desc }
				// Viper keys are case-insensitive by default for mapstructure
				if isGroupMode, ok := groupMap["isgroupdiscussionmode"].(bool); ok { group.IsGroupDiscussionMode = isGroupMode }
				
				if membersVal, ok := groupMap["members"].([]interface{}); ok {
					for _, m := range membersVal {
						if memberStr, ok := m.(string); ok {
							group.Members = append(group.Members, memberStr)
						}
					}
				}
				AppConfig.LLMGroups = append(AppConfig.LLMGroups, &group)
			}
			log.Printf("INFO: [Config] Successfully loaded and parsed %d LLMGroups (including Members).", len(AppConfig.LLMGroups))
		} else {
			log.Println("WARN: [Config] Failed to load 'llm_groups' or the format is incorrect.")
		}
	}

	// Handle special characters in model names (e.g., replacing "__" with ".")
	// This ensures consistency if model names in config use "__" for readability but actual API expects "."
	updatedLLMModels := make(map[string]string)
	for modelName, provider := range AppConfig.LLMModels {
		newModelName := strings.Replace(modelName, "__", ".", 1)
		updatedLLMModels[newModelName] = provider
	}
	AppConfig.LLMModels = updatedLLMModels

	for _, character := range AppConfig.LLMCharacters {
		if character != nil && character.Model != "" { // Added nil check for safety
			character.Model = strings.Replace(character.Model, "__", ".", 1)
		}
	}
	log.Println("INFO: [Config] Configuration loading complete.")
}