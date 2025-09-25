package simple_kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/snowflake"

	"github.com/IBM/sarama"
)

// SimpleNotificationProducer 简化的通知生产者
type SimpleNotificationProducer struct {
	producer sarama.SyncProducer
	topic    string
}

// SimpleNotificationConsumer 简化的通知消费者
type SimpleNotificationConsumer struct {
	consumer sarama.ConsumerGroup
	topic    string
	groupID  string
}

// NewSimpleProducer 创建简化的生产者
func NewSimpleProducer(brokers []string, topic string) (*SimpleNotificationProducer, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	config.Producer.RequiredAcks = sarama.WaitForOne
	config.Producer.Retry.Max = 3

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, err
	}

	return &SimpleNotificationProducer{
		producer: producer,
		topic:    topic,
	}, nil
}

// NewSimpleConsumer 创建简化的消费者
func NewSimpleConsumer(brokers []string, topic, groupID string) (*SimpleNotificationConsumer, error) {
	config := sarama.NewConfig()
	config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	config.Consumer.Offsets.Initial = sarama.OffsetNewest

	consumer, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		return nil, err
	}

	return &SimpleNotificationConsumer{
		consumer: consumer,
		topic:    topic,
		groupID:  groupID,
	}, nil
}

// PublishNotification 发布通知
func (p *SimpleNotificationProducer) PublishNotification(ctx context.Context, event *models.NotificationEvent) error {
	// 生成ID
	if event.ID == 0 {
		event.ID = snowflake.GenID()
	}

	// 序列化消息
	messageData, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// 发送消息
	msg := &sarama.ProducerMessage{
		Topic: p.topic,
		Key:   sarama.StringEncoder(fmt.Sprintf("user_%d", event.ReceiverID)),
		Value: sarama.ByteEncoder(messageData),
	}

	_, _, err = p.producer.SendMessage(msg)
	return err
}

// StartConsuming 开始消费消息
func (c *SimpleNotificationConsumer) StartConsuming(ctx context.Context) error {
	handler := &SimpleNotificationHandler{}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				err := c.consumer.Consume(ctx, []string{c.topic}, handler)
				if err != nil {
					log.Printf("消费消息失败: %v", err)
					time.Sleep(time.Second)
				}
			}
		}
	}()

	return nil
}

// Close 关闭生产者
func (p *SimpleNotificationProducer) Close() error {
	return p.producer.Close()
}

// Close 关闭消费者
func (c *SimpleNotificationConsumer) Close() error {
	return c.consumer.Close()
}

// SimpleNotificationHandler 简化的消息处理器
type SimpleNotificationHandler struct{}

// Setup 会话设置
func (h *SimpleNotificationHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

// Cleanup 会话清理
func (h *SimpleNotificationHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim 消费消息
func (h *SimpleNotificationHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			// 解析消息
			var event models.NotificationEvent
			if err := json.Unmarshal(message.Value, &event); err != nil {
				log.Printf("解析消息失败: %v", err)
				continue
			}

			// 写入数据库
			if err := mysql.InsertNotification(&event); err != nil {
				log.Printf("写入数据库失败: %v", err)
				continue
			}

			// 标记消息已处理
			session.MarkMessage(message.Topic, message.Partition, message.Offset, "")

		case <-session.Context().Done():
			return nil
		}
	}
}

// 全局变量
var (
	globalProducer *SimpleNotificationProducer
	globalConsumer *SimpleNotificationConsumer
)

// InitSimpleKafka 初始化简化的Kafka
func InitSimpleKafka(brokers []string, topic, groupID string) error {
	// 创建生产者
	producer, err := NewSimpleProducer(brokers, topic)
	if err != nil {
		return fmt.Errorf("创建生产者失败: %w", err)
	}

	// 创建消费者
	consumer, err := NewSimpleConsumer(brokers, topic, groupID)
	if err != nil {
		producer.Close()
		return fmt.Errorf("创建消费者失败: %w", err)
	}

	globalProducer = producer
	globalConsumer = consumer

	// 启动消费者
	ctx := context.Background()
	if err := consumer.StartConsuming(ctx); err != nil {
		producer.Close()
		consumer.Close()
		return fmt.Errorf("启动消费者失败: %w", err)
	}

	log.Println("简化Kafka通知系统已启动")
	return nil
}

// PublishSimpleNotification 发布通知（全局函数）
func PublishSimpleNotification(ctx context.Context, event *models.NotificationEvent) error {
	if globalProducer == nil {
		return fmt.Errorf("生产者未初始化")
	}
	return globalProducer.PublishNotification(ctx, event)
}

// CloseSimpleKafka 关闭Kafka
func CloseSimpleKafka() error {
	var err error
	if globalProducer != nil {
		if e := globalProducer.Close(); e != nil {
			err = e
		}
	}
	if globalConsumer != nil {
		if e := globalConsumer.Close(); e != nil {
			err = e
		}
	}
	return err
}
