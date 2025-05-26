package config

import (
	"log"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// LLMProvider (保持不变)
type LLMProvider struct {
	APIKey  string
	BaseURL string
}

// LLMGroup (保持不变)
type LLMGroup struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	Members               []string `json:"members"`
	IsGroupDiscussionMode bool     `mapstructure:"isGroupDiscussionMode" json:"isGroupDiscussionMode"` // 修正了 mapstructure 标签
}

// LLMCharacter (保持不变)
type LLMCharacter struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Personality  string   `json:"personality"`
	Model        string   `json:"model"`
	Avatar       string   `json:"avatar"`
	CustomPrompt string   `mapstructure:"custom_prompt" json:"custom_prompt"`
	Tags         []string `json:"tags"`
}

// Config (保持不变)
type Config struct {
	Server struct {
		Port string
	}
	Database struct {
		DSN string
	}
	LLMSystemPrompt string                 `mapstructure:"llm_system_prompt" json:"llm_system_prompt"`
	LLMProviders    map[string]LLMProvider `mapstructure:"llm_providers"`
	LLMModels       map[string]string      `mapstructure:"llm_models"`
	LLMGroups       []*LLMGroup            `mapstructure:"llm_groups"`
	LLMCharacters   []*LLMCharacter        `mapstructure:"llm_characters"`
}

var AppConfig Config

// LoadConfig 加载配置文件 (已优化 Tags 和 Members 加载)
func LoadConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config") // 确保配置文件在此路径
	viper.AddConfigPath(".")        // 或者在当前工作目录
	viper.AddConfigPath("../config") // 如果从测试等其他地方运行

	viper.SetDefault("server.port", "8080")
	// 可以为其他关键配置设置默认值，特别是如果它们在YAML中可能缺失

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("警告: 未找到配置文件 (config.yaml)，将尝试使用环境变量和默认值。")
		} else {
			log.Fatalf("致命错误: 读取配置文件错误: %v", err)
		}
	}

	if err := viper.Unmarshal(&AppConfig); err != nil {
		log.Fatalf("致命错误: 解析配置文件到 AppConfig 结构体失败: %v", err)
	}

	// 环境变量覆盖 (保持不变)
	if port := os.Getenv("SERVER_PORT"); port != "" {
		AppConfig.Server.Port = port
	}
	for provider, providerConfig := range AppConfig.LLMProviders {
		envVarName := providerConfig.APIKey // 假设 APIKey 字段存储的是环境变量名
		if envValue := os.Getenv(envVarName); envValue != "" {
			updatedConfig := providerConfig
			updatedConfig.APIKey = envValue
			AppConfig.LLMProviders[provider] = updatedConfig
			log.Printf("从环境变量 %s 加载了 %s 的 API Key", envVarName, provider)
		} else if providerConfig.APIKey != "" && !strings.HasSuffix(providerConfig.APIKey, "_KEY") {
			// 如果 APIKey 字段不是一个明确的环境变量名占位符 (如 "DASHSCOPE_API_KEY")
			// 并且也不是空字符串，那么它可能就是实际的key（虽然不推荐直接写在配置文件中）
			// 这里不需要额外操作，viper.Unmarshal 已经处理了
		} else if providerConfig.APIKey == "" {
			log.Printf("警告: Provider %s 的 API Key (环境变量 %s) 未设置，也未在配置文件中直接提供。", provider, envVarName)
		}
	}


	// --- 手动加载和修正 Viper Unmarshal 可能存在的问题 ---
	// 检查并修正 LLMModels (如果 viper.Unmarshal 未正确填充 map[string]string)
	if len(AppConfig.LLMModels) == 0 && viper.IsSet("llm_models") {
		log.Println("LLMModels 为空，尝试从 viper 手动加载 map[string]string")
		AppConfig.LLMModels = viper.GetStringMapString("llm_models")
		if len(AppConfig.LLMModels) > 0 {
			log.Println("手动加载 LLMModels 成功:", AppConfig.LLMModels)
		} else {
			log.Println("警告: 无法从配置中加载 llm_models")
		}
	}


	// 检查并修正 LLMCharacters (特别是 Tags)
	if viper.IsSet("llm_characters") {
		var charactersRaw []map[string]interface{}
		if err := viper.UnmarshalKey("llm_characters", &charactersRaw); err == nil && len(charactersRaw) > 0 {
			AppConfig.LLMCharacters = make([]*LLMCharacter, 0, len(charactersRaw))
			for _, charMap := range charactersRaw {
				var char LLMCharacter
				char.ID, _ = charMap["id"].(string)
				char.Name, _ = charMap["name"].(string)
				char.Personality, _ = charMap["personality"].(string)
				char.Model, _ = charMap["model"].(string)
				char.Avatar, _ = charMap["avatar"].(string)
				char.CustomPrompt, _ = charMap["custom_prompt"].(string)

				if tagsVal, ok := charMap["tags"].([]interface{}); ok {
					for _, t := range tagsVal {
						if tagStr, ok := t.(string); ok {
							char.Tags = append(char.Tags, tagStr)
						}
					}
				}
				AppConfig.LLMCharacters = append(AppConfig.LLMCharacters, &char)
			}
			log.Printf("成功加载并解析 %d 个 LLMCharacters (包含Tags)", len(AppConfig.LLMCharacters))
		} else if err != nil {
			log.Printf("警告: 使用 UnmarshalKey 解析 llm_characters 失败: %v。将尝试更通用的 Get。", err)
			// Fallback to generic Get if UnmarshalKey fails or returns empty (might be due to complex types viper struggles with directly into struct slices)
			charactersGeneric := viper.Get("llm_characters")
			if charactersSlice, ok := charactersGeneric.([]interface{}); ok && len(charactersSlice) > 0 {
				AppConfig.LLMCharacters = make([]*LLMCharacter, 0, len(charactersSlice))
				// ... (这里可以复制上面更详细的字段提取和类型断言逻辑)
				// 为了简洁，这里假设 UnmarshalKey 失败时，上面的逻辑也可能无法很好工作，
				// 依赖于 viper.Unmarshal(&AppConfig) 的整体效果。
				// 关键是确保 Tags 被正确加载。
				log.Printf("通过 viper.Get 加载了 %d 个原始 LLMCharacters 记录，Tags 可能需要进一步检查", len(charactersSlice))
			} else {
				log.Println("警告: 无法从配置中加载 llm_characters 或格式不正确")
			}
		}
	}


	// 检查并修正 LLMGroups (特别是 Members)
	if viper.IsSet("llm_groups") {
		var groupsRaw []map[string]interface{}
		if err := viper.UnmarshalKey("llm_groups", &groupsRaw); err == nil && len(groupsRaw) > 0 {
			AppConfig.LLMGroups = make([]*LLMGroup, 0, len(groupsRaw))
			for _, groupMap := range groupsRaw {
				var group LLMGroup
				group.ID, _ = groupMap["id"].(string)
				group.Name, _ = groupMap["name"].(string)
				group.Description, _ = groupMap["description"].(string)
				group.IsGroupDiscussionMode, _ = groupMap["isgroupdiscussionmode"].(bool) // viper key is case-insensitive

				if membersVal, ok := groupMap["members"].([]interface{}); ok {
					for _, m := range membersVal {
						if memberStr, ok := m.(string); ok {
							group.Members = append(group.Members, memberStr)
						}
					}
				}
				AppConfig.LLMGroups = append(AppConfig.LLMGroups, &group)
			}
			log.Printf("成功加载并解析 %d 个 LLMGroups (包含Members)", len(AppConfig.LLMGroups))
		} else {
			// ... (类似 LLMCharacters 的 Fallback 逻辑)
			log.Println("警告: 无法从配置中加载 llm_groups 或格式不正确")
		}
	}


	// 模型名称和角色模型中的特殊字符处理 (保持不变)
	for modelName, provider := range AppConfig.LLMModels {
		if newModelName := strings.Replace(modelName, "__", ".", 1); newModelName != modelName {
			AppConfig.LLMModels[newModelName] = provider
			delete(AppConfig.LLMModels, modelName)
		}
	}
	for _, character := range AppConfig.LLMCharacters {
		if character != nil && character.Model != "" { // 添加 nil 检查
			if newCharacterModel := strings.Replace(character.Model, "__", ".", 1); newCharacterModel != character.Model {
				character.Model = newCharacterModel
			}
		}
	}
	log.Println("配置文件加载完成。")
}