package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// Consumer Kafka消费者
type Consumer struct {
	client    *Client
	topic     string
	handler   MessageHandler
	mu        sync.RWMutex
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// MessageHandler 消息处理接口
type MessageHandler interface {
	// HandleMessage 处理单条消息
	HandleMessage(ctx context.Context, message *ConsumedMessage) error
	
	// HandleBatch 处理批量消息（可选）
	HandleBatch(ctx context.Context, messages []*ConsumedMessage) error
}

// ConsumedMessage 消费的消息结构
type ConsumedMessage struct {
	Topic     string                 `json:"topic"`     // 主题
	Partition int32                  `json:"partition"` // 分区
	Offset    int64                  `json:"offset"`    // 偏移量
	Key       string                 `json:"key"`       // 消息键
	Value     []byte                 `json:"value"`     // 消息值
	Headers   map[string]string      `json:"headers"`   // 消息头
	Timestamp time.Time              `json:"timestamp"` // 时间戳
	Metadata  map[string]interface{} `json:"metadata"`  // 元数据
}

// NewConsumer 创建消费者
func NewConsumer(client *Client, topic string, handler MessageHandler) *Consumer {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Consumer{
		client:  client,
		topic:   topic,
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动消费者
func (c *Consumer) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.running {
		return fmt.Errorf("消费者已在运行")
	}
	
	if c.client.IsClosed() {
		return fmt.Errorf("客户端已关闭")
	}
	
	consumer := c.client.GetConsumer()
	if consumer == nil {
		return fmt.Errorf("消费者未初始化")
	}
	
	c.running = true
	c.wg.Add(1)
	
	go c.consume()
	
	zap.L().Info("消费者已启动",
		zap.String("topic", c.topic),
		zap.String("group_id", c.client.config.Consumer.GroupID))
	
	return nil
}

// Stop 停止消费者
func (c *Consumer) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if !c.running {
		return nil
	}
	
	c.running = false
	c.cancel()
	
	// 等待消费完成
	c.wg.Wait()
	
	zap.L().Info("消费者已停止",
		zap.String("topic", c.topic))
	
	return nil
}

// IsRunning 检查消费者是否在运行
func (c *Consumer) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// consume 消费消息
func (c *Consumer) consume() {
	defer c.wg.Done()
	
	consumer := c.client.GetConsumer()
	if consumer == nil {
		zap.L().Error("消费者未初始化")
		return
	}
	
	// 创建消费者组会话
	session := &ConsumerGroupSession{
		consumer: c,
	}
	
	// 开始消费
	for {
		select {
		case <-c.ctx.Done():
			zap.L().Info("消费者收到停止信号")
			return
		default:
			// 消费消息
			err := consumer.Consume(c.ctx, []string{c.topic}, session)
			if err != nil {
				zap.L().Error("消费消息失败",
					zap.String("topic", c.topic),
					zap.Error(err))
				
				// 短暂等待后重试
				time.Sleep(time.Second)
				continue
			}
		}
	}
}

// ConsumerGroupSession 消费者组会话
type ConsumerGroupSession struct {
	consumer *Consumer
}

// Setup 会话设置
func (s *ConsumerGroupSession) Setup(sarama.ConsumerGroupSession) error {
	zap.L().Info("消费者组会话已建立",
		zap.String("topic", s.consumer.topic))
	return nil
}

// Cleanup 会话清理
func (s *ConsumerGroupSession) Cleanup(sarama.ConsumerGroupSession) error {
	zap.L().Info("消费者组会话已清理",
		zap.String("topic", s.consumer.topic))
	return nil
}

// ConsumeClaim 消费消息
func (s *ConsumerGroupSession) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	zap.L().Info("开始消费分区",
		zap.String("topic", claim.Topic()),
		zap.Int32("partition", claim.Partition()),
		zap.Int64("initial_offset", claim.InitialOffset()))
	
	// 批量处理消息
	batchSize := s.consumer.client.config.Consumer.MaxPollRecords
	if batchSize <= 0 {
		batchSize = 100 // 默认批量大小
	}
	
	messages := make([]*ConsumedMessage, 0, batchSize)
	
	for {
		select {
		case <-s.consumer.ctx.Done():
			// 处理剩余消息
			if len(messages) > 0 {
				s.processBatch(session, messages)
			}
			return nil
		case message := <-claim.Messages():
			if message == nil {
				// 处理当前批次
				if len(messages) > 0 {
					s.processBatch(session, messages)
					messages = messages[:0] // 重置切片
				}
				continue
			}
			
			// 构建消费消息
			consumedMsg := &ConsumedMessage{
				Topic:     message.Topic,
				Partition: message.Partition,
				Offset:    message.Offset,
				Key:       string(message.Key),
				Value:     message.Value,
				Headers:   make(map[string]string),
				Timestamp: message.Timestamp,
				Metadata: map[string]interface{}{
					"topic":     message.Topic,
					"partition": message.Partition,
					"offset":    message.Offset,
				},
			}
			
			// 解析消息头
			for _, header := range message.Headers {
				consumedMsg.Headers[string(header.Key)] = string(header.Value)
			}
			
			messages = append(messages, consumedMsg)
			
			// 达到批量大小时处理
			if len(messages) >= batchSize {
				s.processBatch(session, messages)
				messages = messages[:0] // 重置切片
			}
		}
	}
}

// processBatch 处理批量消息
func (s *ConsumerGroupSession) processBatch(session sarama.ConsumerGroupSession, messages []*ConsumedMessage) {
	if len(messages) == 0 {
		return
	}
	
	// 尝试批量处理
	if s.consumer.handler.HandleBatch != nil {
		if err := s.consumer.handler.HandleBatch(s.consumer.ctx, messages); err != nil {
			zap.L().Error("批量处理消息失败",
				zap.Int("count", len(messages)),
				zap.Error(err))
			
			// 批量处理失败，回退到单条处理
			s.processMessagesIndividually(session, messages)
		} else {
			// 批量处理成功，标记消息已处理
			s.markMessagesProcessed(session, messages)
		}
	} else {
		// 没有批量处理函数，单条处理
		s.processMessagesIndividually(session, messages)
	}
}

// processMessagesIndividually 单条处理消息
func (s *ConsumerGroupSession) processMessagesIndividually(session sarama.ConsumerGroupSession, messages []*ConsumedMessage) {
	for _, msg := range messages {
		if err := s.consumer.handler.HandleMessage(s.consumer.ctx, msg); err != nil {
			zap.L().Error("处理消息失败",
				zap.String("topic", msg.Topic),
				zap.Int32("partition", msg.Partition),
				zap.Int64("offset", msg.Offset),
				zap.String("key", msg.Key),
				zap.Error(err))
			
			// 可以选择重试或跳过
			continue
		}
		
		// 标记消息已处理
		session.MarkMessage(msg.Topic, msg.Partition, msg.Offset, "")
	}
}

// markMessagesProcessed 标记消息已处理
func (s *ConsumerGroupSession) markMessagesProcessed(session sarama.ConsumerGroupSession, messages []*ConsumedMessage) {
	for _, msg := range messages {
		session.MarkMessage(msg.Topic, msg.Partition, msg.Offset, "")
	}
}

// ParseNotificationMessage 解析通知消息
func ParseNotificationMessage(msg *ConsumedMessage) (*NotificationMessage, error) {
	var notification NotificationMessage
	if err := json.Unmarshal(msg.Value, &notification); err != nil {
		return nil, fmt.Errorf("解析通知消息失败: %w", err)
	}
	return &notification, nil
}

// GetTopic 获取主题名称
func (c *Consumer) GetTopic() string {
	return c.topic
}

// GetGroupID 获取消费者组ID
func (c *Consumer) GetGroupID() string {
	return c.client.config.Consumer.GroupID
}
