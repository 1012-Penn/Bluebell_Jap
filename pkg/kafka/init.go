package kafka

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
	"go.uber.org/zap"
)

// LoadConfigFromFile 从文件加载配置
func LoadConfigFromFile(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config struct {
		Kafka Config `yaml:"kafka"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &config.Kafka, nil
}

// LoadConfigFromEnv 从环境变量加载配置
func LoadConfigFromEnv() *Config {
	config := DefaultConfig()

	// 从环境变量读取配置
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		// 假设brokers用逗号分隔
		config.Brokers = []string{brokers}
	}

	if clientID := os.Getenv("KAFKA_CLIENT_ID"); clientID != "" {
		config.ClientID = clientID
	}

	if groupID := os.Getenv("KAFKA_GROUP_ID"); groupID != "" {
		config.Consumer.GroupID = groupID
	}

	return config
}

// InitFromFile 从配置文件初始化
func InitFromFile(filename string) error {
	config, err := LoadConfigFromFile(filename)
	if err != nil {
		return err
	}

	return InitKafkaManager(config)
}

// InitFromEnv 从环境变量初始化
func InitFromEnv() error {
	config := LoadConfigFromEnv()
	return InitKafkaManager(config)
}

// InitDefault 使用默认配置初始化
func InitDefault() error {
	config := DefaultConfig()
	return InitKafkaManager(config)
}

// Shutdown 关闭Kafka管理器
func Shutdown() error {
	if GlobalManager == nil {
		return nil
	}

	return GlobalManager.Close()
}
