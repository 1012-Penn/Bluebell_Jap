package logic

import (
	"context"
	"fmt"
	"sync"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/kafka"
	"bluebell/pkg/snowflake"

	"go.uber.org/zap"
)

// KafkaNotificationService 基于Kafka的通知服务
type KafkaNotificationService struct {
	client     *kafka.Client
	producer   *kafka.Producer
	consumer   *kafka.Consumer
	mu         sync.RWMutex
	closed     bool
	ctx        context.Context
	cancel     context.CancelFunc
}

// NotificationHandler 通知消息处理器
type NotificationHandler struct {
	service *KafkaNotificationService
}

// kafkaNotifyService 全局Kafka通知服务实例
var kafkaNotifyService *KafkaNotificationService

// NewKafkaNotificationService 创建Kafka通知服务
func NewKafkaNotificationService(config *kafka.Config) (*KafkaNotificationService, error) {
	// 创建Kafka客户端
	client, err := kafka.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("创建Kafka客户端失败: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	
	service := &KafkaNotificationService{
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}

	// 创建生产者
	service.producer = kafka.NewProducer(client, "bluebell-notifications")

	// 创建消费者处理器
	handler := &NotificationHandler{service: service}
	service.consumer = kafka.NewConsumer(client, "bluebell-notifications", handler)

	return service, nil
}

// StartKafkaNotificationService 启动Kafka通知服务
func StartKafkaNotificationService(config *kafka.Config) error {
	service, err := NewKafkaNotificationService(config)
	if err != nil {
		return err
	}

	kafkaNotifyService = service

	// 启动消费者
	if err := service.consumer.Start(); err != nil {
		return fmt.Errorf("启动消费者失败: %w", err)
	}

	zap.L().Info("Kafka通知服务已启动")
	return nil
}

// PublishKafkaNotification 发布通知到Kafka
func PublishKafkaNotification(ctx context.Context, event *models.NotificationEvent) error {
	if kafkaNotifyService == nil {
		return fmt.Errorf("Kafka通知服务未初始化")
	}

	if event == nil {
		return nil
	}

	// 生成通知ID
	if event.ID == 0 {
		event.ID = snowflake.GenID()
	}

	// 构建通知消息
	notification := &kafka.NotificationMessage{
		ID:         event.ID,
		UserID:     event.ReceiverID,
		FromUserID: event.ActorID,
		PostID:     event.PostID,
		CommentID:  event.CommentID,
		Type:       event.Type,
		Content:    event.Message,
		CreatedAt:  event.CreatedAt,
		Priority:   getPriorityByType(event.Type),
		RetryCount: 0,
	}

	// 发送到Kafka
	return kafkaNotifyService.producer.SendNotification(ctx, notification)
}

// PublishKafkaBatchNotifications 批量发布通知到Kafka
func PublishKafkaBatchNotifications(ctx context.Context, events []*models.NotificationEvent) error {
	if kafkaNotifyService == nil {
		return fmt.Errorf("Kafka通知服务未初始化")
	}

	if len(events) == 0 {
		return nil
	}

	// 构建通知消息列表
	notifications := make([]*kafka.NotificationMessage, 0, len(events))
	
	for _, event := range events {
		if event == nil {
			continue
		}

		// 生成通知ID
		if event.ID == 0 {
			event.ID = snowflake.GenID()
		}

		notification := &kafka.NotificationMessage{
			ID:         event.ID,
			UserID:     event.ReceiverID,
			FromUserID: event.ActorID,
			PostID:     event.PostID,
			CommentID:  event.CommentID,
			Type:       event.Type,
			Content:    event.Message,
			CreatedAt:  event.CreatedAt,
			Priority:   getPriorityByType(event.Type),
			RetryCount: 0,
		}
		
		notifications = append(notifications, notification)
	}

	// 批量发送到Kafka
	return kafkaNotifyService.producer.SendBatchNotifications(ctx, notifications)
}

// HandleMessage 处理单条通知消息
func (h *NotificationHandler) HandleMessage(ctx context.Context, message *kafka.ConsumedMessage) error {
	// 解析通知消息
	notification, err := kafka.ParseNotificationMessage(message)
	if err != nil {
		zap.L().Error("解析通知消息失败",
			zap.String("topic", message.Topic),
			zap.Int32("partition", message.Partition),
			zap.Int64("offset", message.Offset),
			zap.Error(err))
		return err
	}

	// 转换为数据库模型
	event := &models.NotificationEvent{
		ID:         notification.ID,
		ReceiverID: notification.UserID,
		ActorID:    notification.FromUserID,
		PostID:     notification.PostID,
		CommentID:  notification.CommentID,
		Type:       notification.Type,
		Message:    notification.Content,
		CreatedAt:  notification.CreatedAt,
	}

	// 写入数据库
	if err := mysql.InsertNotification(event); err != nil {
		zap.L().Error("插入通知到数据库失败",
			zap.Int64("notification_id", notification.ID),
			zap.Int64("user_id", notification.UserID),
			zap.String("type", notification.Type),
			zap.Error(err))
		return err
	}

	zap.L().Info("通知处理成功",
		zap.Int64("notification_id", notification.ID),
		zap.Int64("user_id", notification.UserID),
		zap.String("type", notification.Type),
		zap.String("topic", message.Topic),
		zap.Int32("partition", message.Partition),
		zap.Int64("offset", message.Offset))

	return nil
}

// HandleBatch 处理批量通知消息
func (h *NotificationHandler) HandleBatch(ctx context.Context, messages []*kafka.ConsumedMessage) error {
	if len(messages) == 0 {
		return nil
	}

	// 解析所有通知消息
	events := make([]*models.NotificationEvent, 0, len(messages))
	
	for _, message := range messages {
		notification, err := kafka.ParseNotificationMessage(message)
		if err != nil {
			zap.L().Error("解析通知消息失败",
				zap.String("topic", message.Topic),
				zap.Int32("partition", message.Partition),
				zap.Int64("offset", message.Offset),
				zap.Error(err))
			continue
		}

		// 转换为数据库模型
		event := &models.NotificationEvent{
			ID:         notification.ID,
			ReceiverID: notification.UserID,
			ActorID:    notification.FromUserID,
			PostID:     notification.PostID,
			CommentID:  notification.CommentID,
			Type:       notification.Type,
			Message:    notification.Content,
			CreatedAt:  notification.CreatedAt,
		}
		
		events = append(events, event)
	}

	if len(events) == 0 {
		return nil
	}

	// 批量插入数据库
	if err := mysql.BatchInsertNotifications(events); err != nil {
		zap.L().Error("批量插入通知到数据库失败",
			zap.Int("count", len(events)),
			zap.Error(err))
		return err
	}

	zap.L().Info("批量通知处理成功",
		zap.Int("count", len(events)))

	return nil
}

// getPriorityByType 根据通知类型获取优先级
func getPriorityByType(notificationType string) int {
	switch notificationType {
	case models.NotificationTypeLike:
		return 1 // 点赞优先级较低
	case models.NotificationTypeComment:
		return 2 // 评论优先级中等
	case models.NotificationTypeReply:
		return 3 // 回复优先级较高
	case models.NotificationTypeSystem:
		return 4 // 系统通知优先级最高
	default:
		return 1
	}
}

// Close 关闭Kafka通知服务
func (s *KafkaNotificationService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.closed {
		return nil
	}
	
	s.closed = true
	s.cancel()
	
	// 停止消费者
	if s.consumer != nil {
		if err := s.consumer.Stop(); err != nil {
			zap.L().Error("停止消费者失败", zap.Error(err))
		}
	}
	
	// 关闭客户端
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			zap.L().Error("关闭Kafka客户端失败", zap.Error(err))
		}
	}
	
	zap.L().Info("Kafka通知服务已关闭")
	return nil
}

// HealthCheck 健康检查
func (s *KafkaNotificationService) HealthCheck() error {
	if s.closed {
		return fmt.Errorf("服务已关闭")
	}
	
	if s.client == nil {
		return fmt.Errorf("客户端未初始化")
	}
	
	return s.client.HealthCheck()
}

// GetStats 获取服务统计信息
func (s *KafkaNotificationService) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"service_status": "running",
		"closed":         s.closed,
		"consumer_running": s.consumer != nil && s.consumer.IsRunning(),
		"topic":          s.consumer.GetTopic(),
		"group_id":       s.consumer.GetGroupID(),
	}
	
	if s.client != nil {
		if err := s.client.HealthCheck(); err != nil {
			stats["client_health"] = err.Error()
		} else {
			stats["client_health"] = "healthy"
		}
	}
	
	return stats
}
