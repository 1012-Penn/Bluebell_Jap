package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"bluebell/models"
	"bluebell/pkg/notification"
)

func notificationExample() {
	// 1. 初始化通知队列
	brokers := []string{"localhost:9092"}
	topic := "bluebell-notifications"
	groupID := "bluebell-notification-group"

	if err := notification.InitNotificationQueue(brokers, topic, groupID); err != nil {
		log.Fatalf("初始化通知队列失败: %v", err)
	}
	defer notification.Close()

	log.Println("通知队列已启动，开始模拟用户操作...")

	// 2. 模拟用户点赞操作
	ctx := context.Background()

	// 模拟用户1001的帖子2001被用户1002点赞
	likeEvent := &models.NotificationEvent{
		ReceiverID: 1001,
		ActorID:    int64Ptr(1002),
		PostID:     int64Ptr(2001),
		Type:       models.NotificationTypeLike,
		Message:    "用户张三点赞了你的帖子",
		CreatedAt:  time.Now(),
	}

	if err := notification.PublishNotification(ctx, likeEvent); err != nil {
		log.Printf("发布点赞通知失败: %v", err)
	} else {
		log.Println("点赞通知已发送")
	}

	// 模拟用户1001的帖子2001被用户1003评论
	commentEvent := &models.NotificationEvent{
		ReceiverID: 1001,
		ActorID:    int64Ptr(1003),
		PostID:     int64Ptr(2001),
		CommentID:  int64Ptr(3001),
		Type:       models.NotificationTypeComment,
		Message:    "用户李四评论了你的帖子",
		CreatedAt:  time.Now(),
	}

	if err := notification.PublishNotification(ctx, commentEvent); err != nil {
		log.Printf("发布评论通知失败: %v", err)
	} else {
		log.Println("评论通知已发送")
	}

	// 3. 等待消息被消费
	time.Sleep(2 * time.Second)

	// 4. 模拟前端拉取通知
	log.Println("模拟前端拉取通知...")

	// 首次拉取（lastID=0）
	result, err := notification.PullNotifications(ctx, 1001, 0, 20)
	if err != nil {
		log.Printf("拉取通知失败: %v", err)
	} else {
		log.Printf("首次拉取结果:")
		log.Printf("  通知数量: %d", len(result.Notifications))
		log.Printf("  下次拉取ID: %d", result.NextLastID)
		log.Printf("  下次延迟: %d毫秒", result.NextDelay)
		log.Printf("  是否还有更多: %v", result.HasMore)
	}

	// 等待一段时间，模拟用户没有新通知
	time.Sleep(6 * time.Second)

	// 再次拉取（增量拉取）
	result, err = notification.PullNotifications(ctx, 1001, result.NextLastID, 20)
	if err != nil {
		log.Printf("增量拉取失败: %v", err)
	} else {
		log.Printf("增量拉取结果:")
		log.Printf("  通知数量: %d", len(result.Notifications))
		log.Printf("  下次拉取ID: %d", result.NextLastID)
		log.Printf("  下次延迟: %d毫秒", result.NextDelay)
		log.Printf("  是否还有更多: %v", result.HasMore)
	}

	// 5. 模拟发送更多通知，测试节流策略
	log.Println("发送更多通知，测试节流策略...")

	for i := 0; i < 3; i++ {
		event := &models.NotificationEvent{
			ReceiverID: 1001,
			ActorID:    int64Ptr(1004 + int64(i)),
			PostID:     int64Ptr(2001),
			Type:       models.NotificationTypeLike,
			Message:    fmt.Sprintf("用户%d点赞了你的帖子", 1004+i),
			CreatedAt:  time.Now(),
		}

		notification.PublishNotification(ctx, event)
		time.Sleep(1 * time.Second)
	}

	// 等待消息被消费
	time.Sleep(2 * time.Second)

	// 再次拉取，应该会有新通知
	result, err = notification.PullNotifications(ctx, 1001, result.NextLastID, 20)
	if err != nil {
		log.Printf("拉取新通知失败: %v", err)
	} else {
		log.Printf("拉取新通知结果:")
		log.Printf("  通知数量: %d", len(result.Notifications))
		log.Printf("  下次延迟: %d毫秒", result.NextDelay)
	}

	log.Println("示例完成！")
}

// int64Ptr 创建int64指针的辅助函数
func int64Ptr(i int64) *int64 {
	return &i
}

func main() {
	notificationExample()
}
