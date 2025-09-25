package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/snowflake"

	"github.com/IBM/sarama"
	redisClient "github.com/go-redis/redis/v8"
)

// NotificationQueue 通知队列管理器
type NotificationQueue struct {
	producer sarama.SyncProducer
	consumer sarama.ConsumerGroup
	redis    *redisClient.Client
	topic    string
	groupID  string
}

// NotificationMessage 通知消息结构
type NotificationMessage struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`      // 接收用户ID
	FromUserID *int64    `json:"from_user_id"` // 发送用户ID
	PostID     *int64    `json:"post_id"`      // 帖子ID
	CommentID  *int64    `json:"comment_id"`   // 评论ID
	Type       string    `json:"type"`         // 通知类型
	Content    string    `json:"content"`      // 通知内容
	CreatedAt  time.Time `json:"created_at"`   // 创建时间
}

// PullResult 拉取结果
type PullResult struct {
	Notifications []*models.NotificationEvent `json:"notifications"`
	NextLastID    int64                       `json:"next_last_id"`
	NextDelay     int64                       `json:"next_delay"` // 下次拉取延迟（毫秒）
	HasMore       bool                        `json:"has_more"`
}

var globalQueue *NotificationQueue

// InitNotificationQueue 初始化通知队列
func InitNotificationQueue(brokers []string, topic, groupID string) error {
	// 创建生产者
	producerConfig := sarama.NewConfig()
	producerConfig.Producer.Return.Successes = true
	producerConfig.Producer.RequiredAcks = sarama.WaitForAll
	producerConfig.Producer.Retry.Max = 3
	producerConfig.Producer.Compression = sarama.CompressionGZIP // 压缩减少网络传输

	producer, err := sarama.NewSyncProducer(brokers, producerConfig)
	if err != nil {
		return fmt.Errorf("创建生产者失败: %w", err)
	}

	// 创建消费者
	consumerConfig := sarama.NewConfig()
	consumerConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	consumerConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	consumerConfig.Consumer.MaxProcessingTime = 500 * time.Millisecond
	consumerConfig.Consumer.Group.Session.Timeout = 10 * time.Second
	consumerConfig.Consumer.Group.Heartbeat.Interval = 3 * time.Second

	consumer, err := sarama.NewConsumerGroup(brokers, groupID, consumerConfig)
	if err != nil {
		producer.Close()
		return fmt.Errorf("创建消费者失败: %w", err)
	}

	globalQueue = &NotificationQueue{
		producer: producer,
		consumer: consumer,
		redis:    redis.GetClient(),
		topic:    topic,
		groupID:  groupID,
	}

	// 启动消费者
	go globalQueue.startConsumer()

	log.Println("通知队列已启动")
	return nil
}

// PublishNotification 发布通知到队列
func PublishNotification(ctx context.Context, event *models.NotificationEvent) error {
	if globalQueue == nil {
		return fmt.Errorf("队列未初始化")
	}

	// 生成通知ID
	if event.ID == 0 {
		event.ID = snowflake.GenID()
	}

	// 构建消息
	msg := &NotificationMessage{
		ID:         event.ID,
		UserID:     event.ReceiverID,
		FromUserID: event.ActorID,
		PostID:     event.PostID,
		CommentID:  event.CommentID,
		Type:       event.Type,
		Content:    event.Message,
		CreatedAt:  event.CreatedAt,
	}

	// 序列化消息
	messageData, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// 发送到Kafka（使用用户ID作为分区键，保证同一用户的消息有序）
	producerMsg := &sarama.ProducerMessage{
		Topic: globalQueue.topic,
		Key:   sarama.StringEncoder(fmt.Sprintf("user_%d", event.ReceiverID)),
		Value: sarama.ByteEncoder(messageData),
		Headers: []sarama.RecordHeader{
			{Key: []byte("type"), Value: []byte("notification")},
			{Key: []byte("user_id"), Value: []byte(fmt.Sprintf("%d", event.ReceiverID))},
		},
	}

	_, _, err = globalQueue.producer.SendMessage(producerMsg)
	if err != nil {
		log.Printf("发送通知失败: user_id=%d, error=%v", event.ReceiverID, err)
		return err
	}

	log.Printf("通知已发送: user_id=%d, type=%s", event.ReceiverID, event.Type)
	return nil
}

// startConsumer 启动消费者
func (q *NotificationQueue) startConsumer() {
	handler := &NotificationHandler{queue: q}

	for {
		err := q.consumer.Consume(context.Background(), []string{q.topic}, handler)
		if err != nil {
			log.Printf("消费消息失败: %v", err)
			time.Sleep(time.Second)
		}
	}
}

// StartConsumer 启动消费者（公共方法）
func StartConsumer(ctx context.Context) error {
	if globalQueue == nil {
		return fmt.Errorf("通知队列未初始化")
	}

	handler := &NotificationHandler{queue: globalQueue}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := globalQueue.consumer.Consume(ctx, []string{globalQueue.topic}, handler)
			if err != nil {
				log.Printf("消费消息失败: %v", err)
				time.Sleep(time.Second)
			}
		}
	}
}

// NotificationHandler 消息处理器
type NotificationHandler struct {
	queue *NotificationQueue
}

func (h *NotificationHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *NotificationHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *NotificationHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			// 解析消息
			var msg NotificationMessage
			if err := json.Unmarshal(message.Value, &msg); err != nil {
				log.Printf("解析消息失败: %v", err)
				continue
			}

			// 转换为数据库模型
			event := &models.NotificationEvent{
				ID:         msg.ID,
				ReceiverID: msg.UserID,
				ActorID:    msg.FromUserID,
				PostID:     msg.PostID,
				CommentID:  msg.CommentID,
				Type:       msg.Type,
				Message:    msg.Content,
				CreatedAt:  msg.CreatedAt,
			}

			// 写入数据库
			if err := mysql.InsertNotification(event); err != nil {
				log.Printf("写入数据库失败: %v", err)
				continue
			}

			// 标记消息已处理
			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}

// PullNotifications 拉取用户通知（实现你描述的方案）
func PullNotifications(ctx context.Context, userID int64, lastID int64, limit int) (*PullResult, error) {
	if globalQueue == nil {
		return nil, fmt.Errorf("队列未初始化")
	}

	// 设置默认值
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// 获取最后拉取的ID
	if lastID == 0 {
		// 从Redis获取用户最后拉取的ID
		cached := getLastPulledID(ctx, userID)
		if cached != 0 {
			lastID = cached
		}
	}

	var notifications []*models.NotificationEvent
	var err error

	// 根据lastID决定查询策略
	if lastID == 0 {
		// 首次拉取，获取最新的100条通知
		notifications, err = mysql.ListLatestNotifications(userID, 100)
	} else {
		// 增量拉取，获取lastID之后的通知
		notifications, err = mysql.ListNotificationsAfter(userID, lastID, limit)
	}

	if err != nil {
		// 即使查询失败也返回延迟时间
		delay := calculateNextDelay(ctx, userID, false)
		return &PullResult{
			Notifications: nil,
			NextLastID:    lastID,
			NextDelay:     int64(delay / time.Millisecond),
			HasMore:       false,
		}, err
	}

	// 计算下次拉取的起始ID
	nextLastID := lastID
	if len(notifications) > 0 {
		nextLastID = notifications[len(notifications)-1].ID
		// 记录用户最后拉取的ID到Redis
		recordLastPulledID(ctx, userID, nextLastID)
	}

	// 计算下次拉取的延迟时间（智能节流）
	delay := calculateNextDelay(ctx, userID, len(notifications) > 0)

	// 判断是否还有更多数据
	hasMore := len(notifications) == limit

	return &PullResult{
		Notifications: notifications,
		NextLastID:    nextLastID,
		NextDelay:     int64(delay / time.Millisecond),
		HasMore:       hasMore,
	}, nil
}

// getLastPulledID 从Redis获取用户最后拉取的ID
func getLastPulledID(ctx context.Context, userID int64) int64 {
	key := fmt.Sprintf("notify:last:%d", userID)
	val, err := globalQueue.redis.Get(ctx, key).Int64()
	if err != nil {
		return 0
	}
	return val
}

// recordLastPulledID 记录用户最后拉取的ID到Redis
func recordLastPulledID(ctx context.Context, userID, lastID int64) {
	key := fmt.Sprintf("notify:last:%d", userID)
	globalQueue.redis.Set(ctx, key, lastID, 24*time.Hour)
}

// calculateNextDelay 计算下次拉取的延迟时间（智能节流策略）
func calculateNextDelay(ctx context.Context, userID int64, hasNew bool) time.Duration {
	key := fmt.Sprintf("notify:interval:%d", userID)

	// 退避策略：5秒 -> 10秒 -> 20秒 -> 1分钟 -> 5分钟
	backoffSteps := []time.Duration{
		5 * time.Second,
		10 * time.Second,
		20 * time.Second,
		60 * time.Second,
		5 * time.Minute,
	}

	if hasNew {
		// 有新通知时，重置为最快频率（5秒）
		globalQueue.redis.Set(ctx, key, 0, 24*time.Hour)
		return backoffSteps[0]
	}

	// 无新通知时，增加延迟时间
	current, err := globalQueue.redis.Get(ctx, key).Int64()
	index := int(current)

	if err != nil || index <= 0 {
		index = 1 // 从第二个间隔开始（10秒）
	} else if index >= len(backoffSteps)-1 {
		index = len(backoffSteps) - 1 // 最大延迟（5分钟）
	}

	// 记录下次使用的索引
	globalQueue.redis.Set(ctx, key, index+1, 24*time.Hour)
	return backoffSteps[index]
}

// Close 关闭队列
func Close() error {
	if globalQueue == nil {
		return nil
	}

	var err error
	if globalQueue.producer != nil {
		if e := globalQueue.producer.Close(); e != nil {
			err = e
		}
	}
	if globalQueue.consumer != nil {
		if e := globalQueue.consumer.Close(); e != nil {
			err = e
		}
	}

	return err
}
