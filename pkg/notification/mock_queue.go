package notification

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/snowflake"

	redisClient "github.com/go-redis/redis/v8"
)

// MockNotificationQueue 模拟通知队列（用于测试）
type MockNotificationQueue struct {
	notifications []*models.NotificationEvent
	mu            sync.RWMutex
	redis         *redisClient.Client
}

var mockQueue *MockNotificationQueue

// InitMockNotificationQueue 初始化模拟通知队列
func InitMockNotificationQueue() error {
	mockQueue = &MockNotificationQueue{
		notifications: make([]*models.NotificationEvent, 0),
		redis:         redis.GetClient(),
	}

	// 启动模拟消费者
	go mockQueue.startMockConsumer()

	log.Println("模拟通知队列已启动")
	return nil
}

// PublishMockNotification 发布模拟通知
func PublishMockNotification(ctx context.Context, event *models.NotificationEvent) error {
	if mockQueue == nil {
		return fmt.Errorf("模拟队列未初始化")
	}

	// 生成通知ID
	if event.ID == 0 {
		event.ID = snowflake.GenID()
	}

	// 添加到模拟队列
	mockQueue.mu.Lock()
	mockQueue.notifications = append(mockQueue.notifications, event)
	mockQueue.mu.Unlock()

	log.Printf("模拟通知已发布: user_id=%d, type=%s", event.ReceiverID, event.Type)
	return nil
}

// startMockConsumer 启动模拟消费者
func (q *MockNotificationQueue) startMockConsumer() {
	for {
		time.Sleep(100 * time.Millisecond) // 模拟异步处理

		q.mu.Lock()
		if len(q.notifications) > 0 {
			// 处理第一个通知
			event := q.notifications[0]
			q.notifications = q.notifications[1:]

			// 写入数据库
			if err := mysql.InsertNotification(event); err != nil {
				log.Printf("写入数据库失败: %v", err)
			} else {
				log.Printf("模拟消费成功: user_id=%d, type=%s", event.ReceiverID, event.Type)
			}
		}
		q.mu.Unlock()
	}
}

// PullMockNotifications 拉取模拟通知
func PullMockNotifications(ctx context.Context, userID int64, lastID int64, limit int) (*PullResult, error) {
	if mockQueue == nil {
		return nil, fmt.Errorf("模拟队列未初始化")
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

// CloseMock 关闭模拟队列
func CloseMock() error {
	// 模拟队列不需要特殊关闭
	return nil
}
