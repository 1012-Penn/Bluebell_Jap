package logic

import (
	"context"
	"fmt"
	"sync"
	"time"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"

	redisClient "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// NotificationPullService 通知拉取服务
type NotificationPullService struct {
	redisClient *redisClient.Client
	mu          sync.RWMutex
}

// PullParam 拉取参数
type PullParam struct {
	UserID int64 `json:"user_id"` // 用户ID
	LastID int64 `json:"last_id"` // 最后拉取的ID，0表示首次拉取
	Limit  int   `json:"limit"`   // 拉取数量限制
}

// PullResult 拉取结果
type PullResult struct {
	Notifications []*models.NotificationEvent `json:"notifications"` // 通知列表
	NextLastID    int64                       `json:"next_last_id"`  // 下次拉取的起始ID
	NextDelay     time.Duration               `json:"next_delay"`    // 下次拉取的延迟时间
	HasMore       bool                        `json:"has_more"`      // 是否还有更多数据
}

// notificationPullService 全局通知拉取服务实例
var notificationPullService *NotificationPullService

// NewNotificationPullService 创建通知拉取服务
func NewNotificationPullService() *NotificationPullService {
	return &NotificationPullService{
		redisClient: redis.GetClient(),
	}
}

// GetNotificationPullService 获取通知拉取服务实例
func GetNotificationPullService() *NotificationPullService {
	if notificationPullService == nil {
		notificationPullService = NewNotificationPullService()
	}
	return notificationPullService
}

// PullNotifications 拉取用户通知
func (s *NotificationPullService) PullNotifications(ctx context.Context, param *PullParam) (*PullResult, error) {
	if param == nil {
		return nil, fmt.Errorf("参数不能为空")
	}

	// 设置默认值
	if param.Limit <= 0 {
		param.Limit = 20 // 默认每次拉取20条
	}
	if param.Limit > 100 {
		param.Limit = 100 // 最大限制100条
	}

	// 获取最后拉取的ID
	lastID := param.LastID
	if lastID == 0 {
		// 从缓存中获取用户最后拉取的ID
		cached := s.getLastPulledID(ctx, param.UserID)
		if cached != 0 {
			lastID = cached
		}
	}

	var (
		notifications []*models.NotificationEvent
		err           error
	)

	// 根据lastID决定查询策略
	if lastID == 0 {
		// 首次拉取，获取最新的通知
		notifications, err = mysql.ListLatestNotifications(param.UserID, param.Limit)
	} else {
		// 增量拉取，获取lastID之后的通知
		notifications, err = mysql.ListNotificationsAfter(param.UserID, lastID, param.Limit)
	}

	if err != nil {
		// 即使查询失败也返回延迟时间，避免前端立即重试
		delay := s.calculateNextDelay(ctx, param.UserID, false)
		return &PullResult{
			Notifications: nil,
			NextLastID:    lastID,
			NextDelay:     delay,
			HasMore:       false,
		}, err
	}

	// 计算下次拉取的起始ID
	nextLastID := lastID
	if len(notifications) > 0 {
		nextLastID = notifications[len(notifications)-1].ID
		// 记录用户最后拉取的ID
		s.recordLastPulledID(ctx, param.UserID, nextLastID)
	}

	// 计算下次拉取的延迟时间
	delay := s.calculateNextDelay(ctx, param.UserID, len(notifications) > 0)

	// 判断是否还有更多数据
	hasMore := len(notifications) == param.Limit

	return &PullResult{
		Notifications: notifications,
		NextLastID:    nextLastID,
		NextDelay:     delay,
		HasMore:       hasMore,
	}, nil
}

// getLastPulledID 获取用户最后拉取的通知ID
func (s *NotificationPullService) getLastPulledID(ctx context.Context, userID int64) int64 {
	key := fmt.Sprintf("bluebell:notify:last:%d", userID)
	val, err := s.redisClient.Get(ctx, key).Int64()
	if err != nil {
		return 0 // 缓存未命中，返回0表示首次拉取
	}
	return val
}

// recordLastPulledID 记录用户最后拉取的通知ID
func (s *NotificationPullService) recordLastPulledID(ctx context.Context, userID, lastID int64) {
	key := fmt.Sprintf("bluebell:notify:last:%d", userID)
	// 设置24小时过期时间
	_ = s.redisClient.Set(ctx, key, lastID, 24*time.Hour).Err()
}

// calculateNextDelay 计算下次拉取的延迟时间（退避策略）
func (s *NotificationPullService) calculateNextDelay(ctx context.Context, userID int64, hasNew bool) time.Duration {
	key := fmt.Sprintf("bluebell:notify:interval:%d", userID)
	
	// 退避策略的时间间隔
	backoffSteps := []time.Duration{
		5 * time.Second,   // 5秒
		10 * time.Second,  // 10秒
		20 * time.Second,  // 20秒
		60 * time.Second,  // 1分钟
		5 * time.Minute,   // 5分钟
	}

	if hasNew {
		// 有新通知时，重置为最快频率（5秒）
		_ = s.redisClient.Set(ctx, key, int64(0), 24*time.Hour).Err()
		return backoffSteps[0] // 5秒
	}

	// 无新通知时，增加延迟时间
	current, err := s.redisClient.Get(ctx, key).Int64()
	index := int(current)

	// 处理索引边界
	if err != nil || index <= 0 {
		index = 1 // 从第二个间隔开始（10秒）
	} else if index >= len(backoffSteps)-1 {
		index = len(backoffSteps) - 1 // 最大延迟（5分钟）
	}

	// 记录下次使用的索引
	_ = s.redisClient.Set(ctx, key, int64(index+1), 24*time.Hour).Err()
	return backoffSteps[index]
}

// GetNotificationStats 获取用户通知统计信息
func (s *NotificationPullService) GetNotificationStats(ctx context.Context, userID int64) (map[string]interface{}, error) {
	// 获取未读通知数量
	unreadCount, err := mysql.GetUnreadNotificationCount(userID)
	if err != nil {
		return nil, err
	}

	// 获取总通知数量
	totalCount, err := mysql.GetTotalNotificationCount(userID)
	if err != nil {
		return nil, err
	}

	// 获取最后拉取的ID
	lastPulledID := s.getLastPulledID(ctx, userID)

	// 获取最后拉取时间
	lastPulledTime := s.getLastPulledTime(ctx, userID)

	return map[string]interface{}{
		"user_id":          userID,
		"unread_count":     unreadCount,
		"total_count":      totalCount,
		"last_pulled_id":   lastPulledID,
		"last_pulled_time": lastPulledTime,
	}, nil
}

// getLastPulledTime 获取用户最后拉取时间
func (s *NotificationPullService) getLastPulledTime(ctx context.Context, userID int64) *time.Time {
	key := fmt.Sprintf("bluebell:notify:time:%d", userID)
	val, err := s.redisClient.Get(ctx, key).Result()
	if err != nil {
		return nil
	}
	
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return &t
	}
	return nil
}

// RecordLastPulledTime 记录用户最后拉取时间
func (s *NotificationPullService) RecordLastPulledTime(ctx context.Context, userID int64) {
	key := fmt.Sprintf("bluebell:notify:time:%d", userID)
	now := time.Now().Format(time.RFC3339)
	_ = s.redisClient.Set(ctx, key, now, 24*time.Hour).Err()
}

// MarkAsRead 标记通知为已读
func (s *NotificationPullService) MarkAsRead(ctx context.Context, userID, notificationID int64) error {
	return mysql.MarkNotificationAsRead(userID, notificationID)
}

// MarkAllAsRead 标记所有通知为已读
func (s *NotificationPullService) MarkAllAsRead(ctx context.Context, userID int64) error {
	return mysql.MarkAllNotificationsAsRead(userID)
}

// DeleteNotification 删除通知
func (s *NotificationPullService) DeleteNotification(ctx context.Context, userID, notificationID int64) error {
	return mysql.DeleteNotification(userID, notificationID)
}

// ClearNotifications 清空用户所有通知
func (s *NotificationPullService) ClearNotifications(ctx context.Context, userID int64) error {
	return mysql.ClearUserNotifications(userID)
}
