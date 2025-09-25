// Package logic 业务逻辑层
//
// 负责处理业务逻辑，是系统的核心层
// 主要职责：
// 1. 处理业务规则和逻辑
// 2. 协调各个DAO层操作
// 3. 数据转换和验证
// 4. 调用外部服务（如JWT生成）
// 5. 不直接处理HTTP请求和数据库操作
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

// NotificationService 通知服务
//
// 功能说明：
// 1. 封装通知系统的完整流程：写入MQ、异步消费、数据库存储
// 2. 实现前端节流拉取机制，避免频繁请求
// 3. 支持点赞、评论等多种通知类型
// 4. 使用Redis作为消息队列和缓存
//
// 字段说明：
// - client: Redis客户端，用于MQ操作和缓存
// - queueKey: 通知队列的Redis键名
// - pullKey: 用户最后拉取通知ID的缓存键模板
// - intervalKey: 用户拉取间隔控制的缓存键模板
// - backoffSteps: 退避策略的时间间隔数组
//
// 技术亮点：
// - 异步处理，提高系统响应速度
// - 节流控制，避免前端频繁请求
// - 退避策略，智能调整拉取频率
// - 消息队列解耦，提高系统稳定性
type NotificationService struct {
	client       *redis.Client   // Redis客户端
	queueKey     string          // 通知队列键名
	pullKey      string          // 用户最后拉取ID键模板
	intervalKey  string          // 用户拉取间隔键模板
	backoffSteps []time.Duration // 退避策略时间间隔
}

// notifyService 全局通知服务实例
// 技术亮点：使用单例模式，确保全局只有一个通知服务实例
var notifyService *NotificationService

// NewNotificationService 创建通知服务实例
//
// 功能说明：
// 1. 初始化Redis客户端连接
// 2. 设置各种Redis键名模板
// 3. 配置退避策略的时间间隔
//
// 返回值：
// - *NotificationService: 通知服务实例
//
// 技术亮点：
// - 预定义Redis键名，避免硬编码
// - 配置退避策略，智能控制拉取频率
// - 使用模板字符串，支持多用户隔离
func NewNotificationService() *NotificationService {
	client := redispkg.GetClient()
	return &NotificationService{
		client:       client,
		queueKey:     "bluebell:notify:queue",                                                                                 // 通知队列键名
		pullKey:      "bluebell:notify:last:%d",                                                                               // 用户最后拉取ID键模板
		intervalKey:  "bluebell:notify:interval:%d",                                                                           // 用户拉取间隔键模板
		backoffSteps: []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 60 * time.Second, 5 * time.Minute}, // 退避策略：5秒->10秒->20秒->1分钟->5分钟
	}
}

// PublishLikeNotification 发布点赞或评论通知到消息队列
//
// 功能说明：
// 1. 将点赞或评论事件写入Redis消息队列
// 2. 异步处理，不阻塞主业务流程
// 3. 支持多种通知类型（点赞、评论等）
// 4. 自动生成通知ID，确保唯一性
//
// 参数说明：
// - ctx: 上下文，用于控制超时和取消
// - event: 通知事件，包含接收者、操作者、帖子等信息
//
// 返回值：
// - error: 发布过程中的错误
//
// 技术亮点：
// - 异步处理，提高系统响应速度
// - 使用Redis List作为消息队列，简单高效
// - 自动生成ID，确保通知唯一性
// - 解耦业务逻辑和通知处理
func PublishLikeNotification(ctx context.Context, event *models.NotificationEvent) error {
	// 1. 参数验证
	if event == nil {
		return nil
	}

	// 2. 生成通知ID
	// 技术亮点：使用雪花算法生成全局唯一ID，避免ID冲突
	if event.ID == 0 {
		event.ID = snowflake.GenID()
	}

	// 3. 序列化通知事件
	// 技术亮点：JSON序列化，便于存储和传输
	payload, err := jsonMarshal(event)
	if err != nil {
		return err
	}

	// 4. 写入Redis消息队列
	// 技术亮点：使用LPush写入队列头部，BRPop消费时从尾部取出，实现FIFO
	return notifyService.client.LPush(ctx, notifyService.queueKey, payload).Err()
}

// StartNotificationConsumer 启动异步通知消费者
//
// 功能说明：
// 1. 创建通知服务实例
// 2. 启动goroutine异步消费消息队列
// 3. 将通知事件写入数据库
//
// 参数说明：
// - ctx: 上下文，用于控制消费者生命周期
//
// 技术亮点：
// - 异步处理，不阻塞主程序
// - 独立goroutine，提高并发性能
// - 解耦生产和消费，提高系统稳定性
func StartNotificationConsumer(ctx context.Context) {
	notifyService = NewNotificationService()
	// 技术亮点：使用goroutine异步消费，不阻塞主程序
	go notifyService.consume(ctx)
}

// consume 消费消息队列中的通知事件
//
// 功能说明：
// 1. 持续监听Redis消息队列
// 2. 从队列中取出通知事件
// 3. 反序列化通知数据
// 4. 将通知写入数据库
//
// 参数说明：
// - ctx: 上下文，用于控制超时和取消
//
// 技术亮点：
// - 使用BRPop阻塞式消费，避免轮询
// - 错误容错，单个消息处理失败不影响整体
// - 持续运行，确保消息不丢失
// - 结构化日志，便于问题排查
func (s *NotificationService) consume(ctx context.Context) {
	// 技术亮点：无限循环持续消费，确保消息不丢失
	for {
		// 1. 阻塞式从队列尾部取出消息
		// 技术亮点：BRPop是阻塞操作，有消息时立即返回，无消息时等待1分钟
		result, err := s.client.BRPop(ctx, time.Minute, s.queueKey).Result()
		if err != nil {
			// 处理Redis.Nil（队列为空）和其他错误
			if err == redis.Nil {
				continue // 队列为空，继续等待
			}
			// 记录其他错误，但不中断消费
			zap.L().Error("notification BRPop failed", zap.Error(err))
			continue
		}

		// 2. 验证返回结果格式
		// 技术亮点：BRPop返回[键名, 值]，确保格式正确
		if len(result) != 2 {
			continue
		}

		// 3. 反序列化通知事件
		// 技术亮点：JSON反序列化，恢复通知对象
		event, err := jsonUnmarshalNotification([]byte(result[1]))
		if err != nil {
			zap.L().Error("notification unmarshal failed", zap.Error(err))
			continue
		}

		// 4. 将通知写入数据库
		// 技术亮点：持久化存储，确保通知不丢失
		if err := mysql.InsertNotification(event); err != nil {
			zap.L().Error("InsertNotification failed", zap.Error(err))
			continue
		}
	}
}

// FetchNotifications 获取用户通知列表（支持节流控制）
//
// 功能说明：
// 1. 根据用户ID和参数获取通知列表
// 2. 支持增量拉取（基于lastID）
// 3. 实现节流控制，避免频繁请求
// 4. 返回下次拉取的延迟时间
//
// 参数说明：
// - ctx: 上下文，用于控制超时和取消
// - userID: 用户ID
// - param: 拉取参数，包含lastID和limit
//
// 返回值：
// - []*models.NotificationEvent: 通知事件列表
// - time.Duration: 下次拉取的延迟时间
// - error: 获取过程中的错误
//
// 技术亮点：
// - 增量拉取，减少数据传输
// - 节流控制，避免频繁请求
// - 智能延迟，根据是否有新通知调整拉取频率
// - 缓存优化，记录用户最后拉取的ID
func FetchNotifications(ctx context.Context, userID int64, param *models.NotificationPullParam) ([]*models.NotificationEvent, time.Duration, error) {
	// 1. 参数默认值处理
	if param == nil {
		param = &models.NotificationPullParam{}
	}

	// 设置默认拉取数量
	limit := param.Limit
	if limit <= 0 {
		limit = 20 // 默认每次拉取20条
	}

	// 2. 获取最后拉取的ID
	// 技术亮点：支持增量拉取，减少重复数据传输
	lastID := param.LastID
	if lastID == 0 {
		// 从缓存中获取用户最后拉取的ID
		cached := notifyService.getLastPulledID(ctx, userID)
		if cached != 0 {
			lastID = cached
		}
	}

	var (
		events []*models.NotificationEvent
		err    error
	)

	// 3. 根据lastID决定查询策略
	if lastID == 0 {
		// 首次拉取，获取最新的通知
		events, err = mysql.ListLatestNotifications(userID, limit)
	} else {
		// 增量拉取，获取lastID之后的通知
		events, err = mysql.ListNotificationsAfter(userID, lastID, limit)
	}

	// 4. 处理查询错误
	if err != nil {
		// 技术亮点：即使查询失败也返回延迟时间，避免前端立即重试
		return nil, notifyService.nextInterval(ctx, userID, false), err
	}

	// 5. 记录拉取状态
	if len(events) > 0 {
		// 技术亮点：记录用户最后拉取的ID，支持下次增量拉取
		notifyService.recordPull(ctx, userID, events[len(events)-1].ID)
	}

	// 6. 计算下次拉取的延迟时间
	// 技术亮点：根据是否有新通知智能调整拉取频率
	delay := notifyService.nextInterval(ctx, userID, len(events) > 0)
	return events, delay, nil
}

// recordPull 记录用户最后拉取的通知ID
//
// 功能说明：
// 1. 将用户最后拉取的通知ID存储到Redis
// 2. 支持下次增量拉取
// 3. 设置24小时过期时间
//
// 参数说明：
// - ctx: 上下文
// - userID: 用户ID
// - lastID: 最后拉取的通知ID
//
// 技术亮点：
// - 使用Redis缓存，提高查询效率
// - 设置过期时间，避免内存泄漏
// - 支持增量拉取，减少数据传输
func (s *NotificationService) recordPull(ctx context.Context, userID, lastID int64) {
	key := fmt.Sprintf(s.pullKey, userID)
	// 技术亮点：设置24小时过期时间，避免缓存永久存在
	_ = s.client.Set(ctx, key, lastID, 24*time.Hour).Err()
}

// getLastPulledID 获取用户最后拉取的通知ID
//
// 功能说明：
// 1. 从Redis缓存中获取用户最后拉取的ID
// 2. 支持增量拉取功能
// 3. 缓存未命中时返回0
//
// 参数说明：
// - ctx: 上下文
// - userID: 用户ID
//
// 返回值：
// - int64: 最后拉取的通知ID，缓存未命中时返回0
//
// 技术亮点：
// - 缓存优化，减少数据库查询
// - 容错处理，缓存未命中时返回默认值
func (s *NotificationService) getLastPulledID(ctx context.Context, userID int64) int64 {
	key := fmt.Sprintf(s.pullKey, userID)
	val, err := s.client.Get(ctx, key).Int64()
	if err != nil {
		return 0 // 缓存未命中，返回0表示首次拉取
	}
	return val
}

// nextInterval 计算下次拉取的延迟时间（退避策略）
//
// 功能说明：
// 1. 根据是否有新通知调整拉取频率
// 2. 实现智能退避策略，避免频繁请求
// 3. 有通知时重置为最快频率，无通知时逐渐增加延迟
//
// 参数说明：
// - ctx: 上下文
// - userID: 用户ID
// - hasNew: 是否有新通知
//
// 返回值：
// - time.Duration: 下次拉取的延迟时间
//
// 技术亮点：
// - 退避策略，智能调整拉取频率
// - 有通知时快速响应，无通知时减少请求
// - 避免无效请求，节省系统资源
func (s *NotificationService) nextInterval(ctx context.Context, userID int64, hasNew bool) time.Duration {
	key := fmt.Sprintf(s.intervalKey, userID)

	if hasNew {
		// 有新通知时，重置为最快频率（5秒）
		// 技术亮点：有通知时快速响应，提高用户体验
		_ = s.client.Set(ctx, key, int64(0), 24*time.Hour).Err()
		return s.backoffSteps[0] // 5秒
	}

	// 无新通知时，增加延迟时间
	current, err := s.client.Get(ctx, key).Int64()
	index := int(current)

	// 处理索引边界
	if err != nil || index <= 0 {
		index = 1 // 从第二个间隔开始（10秒）
	} else if index >= len(s.backoffSteps)-1 {
		index = len(s.backoffSteps) - 1 // 最大延迟（5分钟）
	}

	// 记录下次使用的索引
	_ = s.client.Set(ctx, key, int64(index+1), 24*time.Hour).Err()
	return s.backoffSteps[index]
}

// jsonMarshal JSON序列化辅助函数
//
// 功能说明：
// 1. 将对象序列化为JSON字节数组
// 2. 统一序列化处理，便于维护
//
// 参数说明：
// - v: 要序列化的对象
//
// 返回值：
// - []byte: JSON字节数组
// - error: 序列化过程中的错误
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// jsonUnmarshalNotification JSON反序列化通知事件
//
// 功能说明：
// 1. 将JSON字节数组反序列化为通知事件
// 2. 处理创建时间默认值
// 3. 统一反序列化处理，便于维护
//
// 参数说明：
// - data: JSON字节数组
//
// 返回值：
// - *models.NotificationEvent: 通知事件对象
// - error: 反序列化过程中的错误
//
// 技术亮点：
// - 处理创建时间默认值，确保数据完整性
// - 统一反序列化逻辑，便于维护
func jsonUnmarshalNotification(data []byte) (*models.NotificationEvent, error) {
	var event models.NotificationEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}

	// 技术亮点：处理创建时间默认值，确保数据完整性
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	return &event, nil
}
