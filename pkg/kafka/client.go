package kafka

import (
	"context"
	"fmt"
	"sync"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// Client Kafka客户端管理器
type Client struct {
	config     *Config
	producer   sarama.SyncProducer
	consumer   sarama.ConsumerGroup
	mu         sync.RWMutex
	closed     bool
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewClient 创建Kafka客户端
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())
	
	client := &Client{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}

	// 初始化生产者
	if err := client.initProducer(); err != nil {
		cancel()
		return nil, fmt.Errorf("初始化生产者失败: %w", err)
	}

	// 初始化消费者
	if err := client.initConsumer(); err != nil {
		cancel()
		return nil, fmt.Errorf("初始化消费者失败: %w", err)
	}

	return client, nil
}

// initProducer 初始化生产者
func (c *Client) initProducer() error {
	config := sarama.NewConfig()
	
	// 基础配置
	config.ClientID = c.config.ClientID
	config.Net.DialTimeout = c.config.GetTimeout()
	config.Net.ReadTimeout = c.config.GetTimeout()
	config.Net.WriteTimeout = c.config.GetTimeout()
	
	// 生产者配置
	config.Producer.RequiredAcks = sarama.RequiredAcks(c.config.Producer.RequiredAcks)
	config.Producer.Retry.Max = c.config.Producer.MaxRetries
	config.Producer.Retry.Backoff = c.config.GetProducerRetryDelay()
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	config.Producer.Timeout = c.config.GetProducerRequestTimeout()
	
	// 批处理配置
	config.Producer.Flush.Messages = c.config.Producer.BatchSize
	config.Producer.Flush.Frequency = c.config.GetProducerBatchTimeout()
	
	// 压缩配置
	switch c.config.Producer.Compression {
	case "gzip":
		config.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		config.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		config.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		config.Producer.Compression = sarama.CompressionZSTD
	default:
		config.Producer.Compression = sarama.CompressionNone
	}

	producer, err := sarama.NewSyncProducer(c.config.Brokers, config)
	if err != nil {
		return err
	}

	c.producer = producer
	return nil
}

// initConsumer 初始化消费者
func (c *Client) initConsumer() error {
	config := sarama.NewConfig()
	
	// 基础配置
	config.ClientID = c.config.ClientID
	config.Net.DialTimeout = c.config.GetTimeout()
	config.Net.ReadTimeout = c.config.GetTimeout()
	config.Net.WriteTimeout = c.config.GetTimeout()
	
	// 消费者配置
	config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	if c.config.Consumer.AutoOffsetReset == "earliest" {
		config.Consumer.Offsets.Initial = sarama.OffsetOldest
	}
	
	// 会话和心跳配置
	config.Consumer.Group.Session.Timeout = c.config.GetConsumerSessionTimeout()
	config.Consumer.Group.Heartbeat.Interval = c.config.GetConsumerHeartbeatInterval()
	
	// 自动提交配置
	config.Consumer.Offsets.AutoCommit.Enable = c.config.Consumer.EnableAutoCommit
	config.Consumer.Offsets.AutoCommit.Interval = c.config.GetConsumerAutoCommitInterval()
	
	// 拉取配置
	config.Consumer.Fetch.Max = int32(c.config.Consumer.MaxPollRecords)
	config.Consumer.Fetch.Min = int32(c.config.Consumer.FetchMinBytes)
	config.Consumer.Fetch.Default = 1024 * 1024 // 1MB
	config.Consumer.MaxWaitTime = c.config.GetConsumerFetchMaxWait()
	
	// 错误处理
	config.Consumer.Return.Errors = true

	consumer, err := sarama.NewConsumerGroup(c.config.Brokers, c.config.Consumer.GroupID, config)
	if err != nil {
		return err
	}

	c.consumer = consumer
	return nil
}

// GetProducer 获取生产者
func (c *Client) GetProducer() sarama.SyncProducer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.producer
}

// GetConsumer 获取消费者
func (c *Client) GetConsumer() sarama.ConsumerGroup {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.consumer
}

// Close 关闭客户端
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.closed {
		return nil
	}
	
	c.closed = true
	c.cancel()
	
	var errs []error
	
	// 关闭生产者
	if c.producer != nil {
		if err := c.producer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭生产者失败: %w", err))
		}
	}
	
	// 关闭消费者
	if c.consumer != nil {
		if err := c.consumer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭消费者失败: %w", err))
		}
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("关闭客户端时发生错误: %v", errs)
	}
	
	zap.L().Info("Kafka客户端已关闭")
	return nil
}

// IsClosed 检查客户端是否已关闭
func (c *Client) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// HealthCheck 健康检查
func (c *Client) HealthCheck() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if c.closed {
		return fmt.Errorf("客户端已关闭")
	}
	
	if c.producer == nil {
		return fmt.Errorf("生产者未初始化")
	}
	
	if c.consumer == nil {
		return fmt.Errorf("消费者未初始化")
	}
	
	return nil
}
