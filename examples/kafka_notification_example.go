package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"bluebell/logic"
	"bluebell/models"
	"bluebell/pkg/kafka"
)

func main() {
	// 1. 初始化Kafka配置
	config := kafka.DefaultConfig()
	config.Brokers = []string{"localhost:9092"}
	config.Consumer.GroupID = "bluebell-notification-example"

	// 2. 初始化Kafka管理器
	if err := kafka.InitKafkaManager(config); err != nil {
		log.Fatalf("初始化Kafka管理器失败: %v", err)
	}
	defer kafka.Shutdown()

	// 3. 启动通知服务
	if err := logic.StartKafkaNotificationService(config); err != nil {
		log.Fatalf("启动通知服务失败: %v", err)
	}

	// 4. 发布通知示例
	ctx := context.Background()
	
	// 发布点赞通知
	likeEvent := &models.NotificationEvent{
		ReceiverID: 1001,
		ActorID:    int64Ptr(1002),
		PostID:     int64Ptr(2001),
		Type:       models.NotificationTypeLike,
		Message:    "用户张三点赞了你的帖子",
		CreatedAt:  time.Now(),
	}

	if err := logic.PublishLikeNotification(ctx, likeEvent); err != nil {
		log.Printf("发布点赞通知失败: %v", err)
	} else {
		log.Println("点赞通知发布成功")
	}

	// 发布评论通知
	commentEvent := &models.NotificationEvent{
		ReceiverID: 1001,
		ActorID:    int64Ptr(1003),
		PostID:     int64Ptr(2001),
		CommentID:  int64Ptr(3001),
		Type:       models.NotificationTypeComment,
		Message:    "用户李四评论了你的帖子",
		CreatedAt:  time.Now(),
	}

	if err := logic.PublishLikeNotification(ctx, commentEvent); err != nil {
		log.Printf("发布评论通知失败: %v", err)
	} else {
		log.Println("评论通知发布成功")
	}

	// 5. 拉取通知示例
	pullService := logic.GetNotificationPullService()
	
	// 首次拉取
	pullParam := &logic.PullParam{
		UserID: 1001,
		LastID: 0, // 首次拉取
		Limit:  20,
	}

	result, err := pullService.PullNotifications(ctx, pullParam)
	if err != nil {
		log.Printf("拉取通知失败: %v", err)
	} else {
		log.Printf("拉取到 %d 条通知", len(result.Notifications))
		log.Printf("下次拉取ID: %d", result.NextLastID)
		log.Printf("下次拉取延迟: %v", result.NextDelay)
		log.Printf("是否还有更多: %v", result.HasMore)
	}

	// 6. 获取统计信息
	stats, err := pullService.GetNotificationStats(ctx, 1001)
	if err != nil {
		log.Printf("获取统计信息失败: %v", err)
	} else {
		log.Printf("统计信息: %+v", stats)
	}

	// 7. 等待一段时间让消费者处理消息
	time.Sleep(5 * time.Second)

	// 8. 再次拉取（增量拉取）
	pullParam.LastID = result.NextLastID
	result, err = pullService.PullNotifications(ctx, pullParam)
	if err != nil {
		log.Printf("增量拉取通知失败: %v", err)
	} else {
		log.Printf("增量拉取到 %d 条通知", len(result.Notifications))
	}

	log.Println("示例运行完成")
}

// int64Ptr 创建int64指针的辅助函数
func int64Ptr(i int64) *int64 {
	return &i
}
