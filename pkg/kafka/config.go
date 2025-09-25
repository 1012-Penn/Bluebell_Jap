package kafka

import (
	"time"
)

// Config Kafka配置结构
type Config struct {
	// 基础配置
	Brokers []string `json:"brokers"` // Kafka broker地址列表

	// 生产者配置
	Producer ProducerConfig `json:"producer"`

	// 消费者配置
	Consumer ConsumerConfig `json:"consumer"`

	// 通用配置
	ClientID string `json:"client_id"` // 客户端ID
	Timeout  int    `json:"timeout"`   // 超时时间（秒）
}

// ProducerConfig 生产者配置
type ProducerConfig struct {
	// 重试配置
	MaxRetries int `json:"max_retries"` // 最大重试次数
	RetryDelay int `json:"retry_delay"` // 重试延迟（毫秒）

	// 批处理配置
	BatchSize    int `json:"batch_size"`    // 批处理大小
	BatchTimeout int `json:"batch_timeout"` // 批处理超时（毫秒）

	// 压缩配置
	Compression string `json:"compression"` // 压缩算法：none, gzip, snappy, lz4, zstd

	// 确认配置
	RequiredAcks int `json:"required_acks"` // 需要确认的副本数：0, 1, -1

	// 超时配置
	RequestTimeout int `json:"request_timeout"` // 请求超时（毫秒）
}

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	// 消费者组
	GroupID string `json:"group_id"` // 消费者组ID

	// 偏移量配置
	AutoOffsetReset string `json:"auto_offset_reset"` // 偏移量重置策略：earliest, latest

	// 提交配置
	EnableAutoCommit   bool `json:"enable_auto_commit"`   // 是否自动提交偏移量
	AutoCommitInterval int  `json:"auto_commit_interval"` // 自动提交间隔（毫秒）

	// 会话配置
	SessionTimeout    int `json:"session_timeout"`    // 会话超时（毫秒）
	HeartbeatInterval int `json:"heartbeat_interval"` // 心跳间隔（毫秒）

	// 拉取配置
	MaxPollRecords int `json:"max_poll_records"` // 单次拉取最大记录数
	FetchMinBytes  int `json:"fetch_min_bytes"`  // 最小拉取字节数
	FetchMaxWait   int `json:"fetch_max_wait"`   // 最大等待时间（毫秒）
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Brokers:  []string{"localhost:9092"},
		ClientID: "bluebell-notification",
		Timeout:  30,
		Producer: ProducerConfig{
			MaxRetries:     3,
			RetryDelay:     100,
			BatchSize:      16384,
			BatchTimeout:   10,
			Compression:    "gzip",
			RequiredAcks:   1,
			RequestTimeout: 30000,
		},
		Consumer: ConsumerConfig{
			GroupID:            "bluebell-notification-group",
			AutoOffsetReset:    "latest",
			EnableAutoCommit:   true,
			AutoCommitInterval: 1000,
			SessionTimeout:     30000,
			HeartbeatInterval:  3000,
			MaxPollRecords:     500,
			FetchMinBytes:      1,
			FetchMaxWait:       500,
		},
	}
}

// GetTimeout 获取超时时间
func (c *Config) GetTimeout() time.Duration {
	return time.Duration(c.Timeout) * time.Second
}

// GetProducerRetryDelay 获取生产者重试延迟
func (c *Config) GetProducerRetryDelay() time.Duration {
	return time.Duration(c.Producer.RetryDelay) * time.Millisecond
}

// GetProducerBatchTimeout 获取生产者批处理超时
func (c *Config) GetProducerBatchTimeout() time.Duration {
	return time.Duration(c.Producer.BatchTimeout) * time.Millisecond
}

// GetProducerRequestTimeout 获取生产者请求超时
func (c *Config) GetProducerRequestTimeout() time.Duration {
	return time.Duration(c.Producer.RequestTimeout) * time.Millisecond
}

// GetConsumerSessionTimeout 获取消费者会话超时
func (c *Config) GetConsumerSessionTimeout() time.Duration {
	return time.Duration(c.Consumer.SessionTimeout) * time.Millisecond
}

// GetConsumerHeartbeatInterval 获取消费者心跳间隔
func (c *Config) GetConsumerHeartbeatInterval() time.Duration {
	return time.Duration(c.Consumer.HeartbeatInterval) * time.Millisecond
}

// GetConsumerAutoCommitInterval 获取消费者自动提交间隔
func (c *Config) GetConsumerAutoCommitInterval() time.Duration {
	return time.Duration(c.Consumer.AutoCommitInterval) * time.Millisecond
}

// GetConsumerFetchMaxWait 获取消费者最大等待时间
func (c *Config) GetConsumerFetchMaxWait() time.Duration {
	return time.Duration(c.Consumer.FetchMaxWait) * time.Millisecond
}
