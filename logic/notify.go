package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"bluebell/dao/mysql"
	redispkg "bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/snowflake"

	redis "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// NotificationService 封装通知写入 MQ、落库以及前端节流拉取的流程。
type NotificationService struct {
	client       *redis.Client
	queueKey     string
	pullKey      string
	intervalKey  string
	backoffSteps []time.Duration
}

var notifyService *NotificationService

// NewNotificationService 创建通知服务实例。
func NewNotificationService() *NotificationService {
	client := redispkg.GetClient()
	return &NotificationService{
		client:       client,
		queueKey:     "bluebell:notify:queue",
		pullKey:      "bluebell:notify:last:%d",
		intervalKey:  "bluebell:notify:interval:%d",
		backoffSteps: []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 60 * time.Second, 5 * time.Minute},
	}
}

// PublishLikeNotification 点赞或评论动作写入 MQ。
func PublishLikeNotification(ctx context.Context, event *models.NotificationEvent) error {
	if event == nil {
		return nil
	}
	if event.ID == 0 {
		event.ID = snowflake.GenID()
	}
	payload, err := jsonMarshal(event)
	if err != nil {
		return err
	}
	return notifyService.client.LPush(ctx, notifyService.queueKey, payload).Err()
}

// StartNotificationConsumer 异步消费 MQ 并写入数据库。
func StartNotificationConsumer(ctx context.Context) {
	notifyService = NewNotificationService()
	go notifyService.consume(ctx)
}

func (s *NotificationService) consume(ctx context.Context) {
	for {
		result, err := s.client.BRPop(ctx, time.Minute, s.queueKey).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			zap.L().Error("notification BRPop failed", zap.Error(err))
			continue
		}
		if len(result) != 2 {
			continue
		}
		event, err := jsonUnmarshalNotification([]byte(result[1]))
		if err != nil {
			zap.L().Error("notification unmarshal failed", zap.Error(err))
			continue
		}
		if err := mysql.InsertNotification(event); err != nil {
			zap.L().Error("InsertNotification failed", zap.Error(err))
			continue
		}
	}
}

// FetchNotifications 结合前端的 lastID 控制拉取范围与频率。
func FetchNotifications(ctx context.Context, userID int64, param *models.NotificationPullParam) ([]*models.NotificationEvent, time.Duration, error) {
	if param == nil {
		param = &models.NotificationPullParam{}
	}
	limit := param.Limit
	if limit <= 0 {
		limit = 20
	}

	lastID := param.LastID
	if lastID == 0 {
		cached := notifyService.getLastPulledID(ctx, userID)
		if cached != 0 {
			lastID = cached
		}
	}

	var (
		events []*models.NotificationEvent
		err    error
	)

	if lastID == 0 {
		events, err = mysql.ListLatestNotifications(userID, limit)
	} else {
		events, err = mysql.ListNotificationsAfter(userID, lastID, limit)
	}
	if err != nil {
		return nil, notifyService.nextInterval(ctx, userID, false), err
	}

	if len(events) > 0 {
		notifyService.recordPull(ctx, userID, events[len(events)-1].ID)
	}

	delay := notifyService.nextInterval(ctx, userID, len(events) > 0)
	return events, delay, nil
}

func (s *NotificationService) recordPull(ctx context.Context, userID, lastID int64) {
	key := fmt.Sprintf(s.pullKey, userID)
	_ = s.client.Set(ctx, key, lastID, 24*time.Hour).Err()
}

func (s *NotificationService) getLastPulledID(ctx context.Context, userID int64) int64 {
	key := fmt.Sprintf(s.pullKey, userID)
	val, err := s.client.Get(ctx, key).Int64()
	if err != nil {
		return 0
	}
	return val
}

func (s *NotificationService) nextInterval(ctx context.Context, userID int64, hasNew bool) time.Duration {
	key := fmt.Sprintf(s.intervalKey, userID)
	if hasNew {
		_ = s.client.Set(ctx, key, int64(0), 24*time.Hour).Err()
		return s.backoffSteps[0]
	}
	current, err := s.client.Get(ctx, key).Int64()
	index := int(current)
	if err != nil || index <= 0 {
		index = 1
	} else if index >= len(s.backoffSteps)-1 {
		index = len(s.backoffSteps) - 1
	}
	_ = s.client.Set(ctx, key, int64(index+1), 24*time.Hour).Err()
	return s.backoffSteps[index]
}

func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func jsonUnmarshalNotification(data []byte) (*models.NotificationEvent, error) {
	var event models.NotificationEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	return &event, nil
}
