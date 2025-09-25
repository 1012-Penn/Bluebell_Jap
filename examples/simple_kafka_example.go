package main

import (
	"context"
	"log"
	"time"

	"bluebell/logic"
)

func main() {
	// 1. 初始化简化Kafka通知系统
	if err := logic.InitSimpleNotificationSystem(); err != nil {
		log.Fatalf("初始化失败: %v", err)
	}
	defer logic.CloseSimpleNotificationSystem()

	// 2. 发布通知
	ctx := context.Background()

	// 发布点赞通知
	if err := logic.PublishSimpleLikeNotification(ctx, 1001, 2001, 1002); err != nil {
		log.Printf("发布点赞通知失败: %v", err)
	} else {
		log.Println("点赞通知发布成功")
	}

	// 发布评论通知
	if err := logic.PublishSimpleCommentNotification(ctx, 1001, 2001, 3001, 1003); err != nil {
		log.Printf("发布评论通知失败: %v", err)
	} else {
		log.Println("评论通知发布成功")
	}

	// 等待消费
	time.Sleep(5 * time.Second)
	log.Println("示例完成")
}
