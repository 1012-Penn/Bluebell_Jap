package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// Producer Kafka生产者
type Producer struct {
	client *Client
	topic  string
}

// NewProducer 创建生产者
func NewProducer(client *Client, topic string) *Producer {
	return &Producer{
		client: client,
		topic:  topic,
	}
}

// Message 消息结构
type Message struct {
	Key       string                 `json:"key"`       // 消息键
	Value     interface{}            `json:"value"`     // 消息值
	Headers   map[string]string      `json:"headers"`   // 消息头
	Timestamp time.Time              `json:"timestamp"` // 时间戳
	Metadata  map[string]interface{} `json:"metadata"`  // 元数据
}

// NotificationMessage 通知消息结构
type NotificationMessage struct {
	ID         int64     `json:"id"`          // 通知ID
	UserID     int64     `json:"user_id"`     // 接收用户ID
	FromUserID *int64    `json:"from_user_id"` // 发送用户ID
	PostID     *int64    `json:"post_id"`     // 帖子ID
	CommentID  *int64    `json:"comment_id"`  // 评论ID
	Type       string    `json:"type"`        // 通知类型
	Content    string    `json:"content"`     // 通知内容
	CreatedAt  time.Time `json:"created_at"`  // 创建时间
	Priority   int       `json:"priority"`    // 优先级
	RetryCount int       `json:"retry_count"` // 重试次数
}

// SendMessage 发送消息
func (p *Producer) SendMessage(ctx context.Context, msg *Message) error {
	if p.client.IsClosed() {
		return fmt.Errorf("客户端已关闭")
	}

	producer := p.client.GetProducer()
	if producer == nil {
		return fmt.Errorf("生产者未初始化")
	}

	// 序列化消息值
	valueBytes, err := json.Marshal(msg.Value)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	// 构建Sarama消息
	saramaMsg := &sarama.ProducerMessage{
		Topic:     p.topic,
		Key:       sarama.StringEncoder(msg.Key),
		Value:     sarama.ByteEncoder(valueBytes),
		Timestamp: msg.Timestamp,
	}

	// 添加消息头
	if msg.Headers != nil {
		for k, v := range msg.Headers {
			saramaMsg.Headers = append(saramaMsg.Headers, sarama.RecordHeader{
				Key:   []byte(k),
				Value: []byte(v),
			})
		}
	}

	// 发送消息
	partition, offset, err := producer.SendMessage(saramaMsg)
	if err != nil {
		zap.L().Error("发送消息失败",
			zap.String("topic", p.topic),
			zap.String("key", msg.Key),
			zap.Error(err))
		return fmt.Errorf("发送消息失败: %w", err)
	}

	zap.L().Info("消息发送成功",
		zap.String("topic", p.topic),
		zap.String("key", msg.Key),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset))

	return nil
}

// SendNotification 发送通知消息
func (p *Producer) SendNotification(ctx context.Context, notification *NotificationMessage) error {
	// 构建消息
	msg := &Message{
		Key:   fmt.Sprintf("user_%d", notification.UserID), // 使用用户ID作为分区键
		Value: notification,
		Headers: map[string]string{
			"type":      "notification",
			"user_id":   fmt.Sprintf("%d", notification.UserID),
			"priority":  fmt.Sprintf("%d", notification.Priority),
		},
		Timestamp: notification.CreatedAt,
		Metadata: map[string]interface{}{
			"notification_id": notification.ID,
			"notification_type": notification.Type,
		},
	}

	return p.SendMessage(ctx, msg)
}

// SendBatchMessages 批量发送消息
func (p *Producer) SendBatchMessages(ctx context.Context, messages []*Message) error {
	if len(messages) == 0 {
		return nil
	}

	if p.client.IsClosed() {
		return fmt.Errorf("客户端已关闭")
	}

	producer := p.client.GetProducer()
	if producer == nil {
		return fmt.Errorf("生产者未初始化")
	}

	// 构建Sarama消息列表
	saramaMessages := make([]*sarama.ProducerMessage, 0, len(messages))
	
	for _, msg := range messages {
		// 序列化消息值
		valueBytes, err := json.Marshal(msg.Value)
		if err != nil {
			zap.L().Error("序列化消息失败", zap.Error(err))
			continue
		}

		// 构建Sarama消息
		saramaMsg := &sarama.ProducerMessage{
			Topic:     p.topic,
			Key:       sarama.StringEncoder(msg.Key),
			Value:     sarama.ByteEncoder(valueBytes),
			Timestamp: msg.Timestamp,
		}

		// 添加消息头
		if msg.Headers != nil {
			for k, v := range msg.Headers {
				saramaMsg.Headers = append(saramaMsg.Headers, sarama.RecordHeader{
					Key:   []byte(k),
					Value: []byte(v),
				})
			}
		}

		saramaMessages = append(saramaMessages, saramaMsg)
	}

	// 批量发送
	err := producer.SendMessages(saramaMessages)
	if err != nil {
		zap.L().Error("批量发送消息失败",
			zap.String("topic", p.topic),
			zap.Int("count", len(saramaMessages)),
			zap.Error(err))
		return fmt.Errorf("批量发送消息失败: %w", err)
	}

	zap.L().Info("批量消息发送成功",
		zap.String("topic", p.topic),
		zap.Int("count", len(saramaMessages)))

	return nil
}

// SendBatchNotifications 批量发送通知消息
func (p *Producer) SendBatchNotifications(ctx context.Context, notifications []*NotificationMessage) error {
	if len(notifications) == 0 {
		return nil
	}

	messages := make([]*Message, 0, len(notifications))
	
	for _, notification := range notifications {
		msg := &Message{
			Key:   fmt.Sprintf("user_%d", notification.UserID),
			Value: notification,
			Headers: map[string]string{
				"type":      "notification",
				"user_id":   fmt.Sprintf("%d", notification.UserID),
				"priority":  fmt.Sprintf("%d", notification.Priority),
			},
			Timestamp: notification.CreatedAt,
			Metadata: map[string]interface{}{
				"notification_id": notification.ID,
				"notification_type": notification.Type,
			},
		}
		messages = append(messages, msg)
	}

	return p.SendBatchMessages(ctx, messages)
}

// GetTopic 获取主题名称
func (p *Producer) GetTopic() string {
	return p.topic
}

// Close 关闭生产者
func (p *Producer) Close() error {
	// 生产者由客户端管理，这里不需要单独关闭
	return nil
}
