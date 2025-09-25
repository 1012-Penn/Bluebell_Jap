package logic

import (
	"context"
	"time"

	"bluebell/models"
	"bluebell/pkg/simple_kafka"
)

// PublishSimpleLikeNotification 发布点赞通知（简化版）
func PublishSimpleLikeNotification(ctx context.Context, userID, postID, actorID int64) error {
	event := &models.NotificationEvent{
		ReceiverID: userID,
		ActorID:    &actorID,
		PostID:     &postID,
		Type:       models.NotificationTypeLike,
		Message:    "有人点赞了你的帖子",
		CreatedAt:  time.Now(),
	}

	return simple_kafka.PublishSimpleNotification(ctx, event)
}

// PublishSimpleCommentNotification 发布评论通知（简化版）
func PublishSimpleCommentNotification(ctx context.Context, userID, postID, commentID, actorID int64) error {
	event := &models.NotificationEvent{
		ReceiverID: userID,
		ActorID:    &actorID,
		PostID:     &postID,
		CommentID:  &commentID,
		Type:       models.NotificationTypeComment,
		Message:    "有人评论了你的帖子",
		CreatedAt:  time.Now(),
	}

	return simple_kafka.PublishSimpleNotification(ctx, event)
}

// InitSimpleNotificationSystem 初始化简化通知系统
func InitSimpleNotificationSystem() error {
	brokers := []string{"localhost:9092"}
	topic := "bluebell-notifications"
	groupID := "bluebell-group"

	return simple_kafka.InitSimpleKafka(brokers, topic, groupID)
}

// CloseSimpleNotificationSystem 关闭简化通知系统
func CloseSimpleNotificationSystem() error {
	return simple_kafka.CloseSimpleKafka()
}
