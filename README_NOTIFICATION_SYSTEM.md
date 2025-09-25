# 基于Kafka的智能通知系统

完全按照你描述的需求设计的通知系统，实现了异步处理、智能节流、增量拉取等核心功能。

## 核心特性

### 1. 异步通知处理
- **Kafka消息队列**：点赞、评论等操作异步发送到Kafka
- **削峰填谷**：利用Kafka的批处理和压缩功能
- **自动消费**：后台自动消费消息并写入数据库
- **用户分区**：使用用户ID作为分区键，保证同一用户消息有序

### 2. 智能增量拉取
- **首次拉取**：默认拉取最近100条通知，快速显示
- **增量拉取**：基于lastID增量拉取，避免深度分页
- **Redis缓存**：记录用户最后拉取的ID，支持缓存恢复
- **索引优化**：利用数据库索引，查询性能优异

### 3. 智能节流策略
- **动态调整**：根据是否有新通知动态调整拉取频率
- **退避算法**：5秒 → 10秒 → 20秒 → 1分钟 → 5分钟
- **重置机制**：有新通知时重置为最快频率
- **手动刷新**：支持手动刷新重新开始节流逻辑

## 系统架构

```
用户操作 → API接口 → Kafka生产者 → Kafka集群
                                    ↓
数据库 ← Kafka消费者 ← Kafka集群 ← 消息队列
  ↑
Redis缓存（记录拉取状态）
```

## 核心实现

### 1. 消息发布
```go
// 发布点赞通知
event := &models.NotificationEvent{
    ReceiverID: 1001,
    ActorID:    &1002,
    PostID:     &2001,
    Type:       models.NotificationTypeLike,
    Message:    "有人点赞了你的帖子",
    CreatedAt:  time.Now(),
}

notification.PublishNotification(ctx, event)
```

### 2. 智能拉取
```go
// 拉取通知
result, err := notification.PullNotifications(ctx, userID, lastID, limit)

// 返回结果
{
    "notifications": [...],     // 通知列表
    "next_last_id": 12345,      // 下次拉取起始ID
    "next_delay": 5000,         // 下次拉取延迟（毫秒）
    "has_more": true            // 是否还有更多数据
}
```

### 3. 节流策略
```go
// 退避策略实现
backoffSteps := []time.Duration{
    5 * time.Second,   // 有新通知时
    10 * time.Second,  // 无新通知第1次
    20 * time.Second,  // 无新通知第2次
    60 * time.Second,  // 无新通知第3次
    5 * time.Minute,   // 最大延迟
}
```

## API接口

### 拉取通知
```http
GET /api/v1/notifications/pull?last_id=0&limit=20
```

**参数说明：**
- `last_id`: 最后拉取的ID，0表示首次拉取
- `limit`: 拉取数量，默认20，最大100

**响应示例：**
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "notifications": [
            {
                "id": 12345,
                "user_id": 1001,
                "from_user_id": 1002,
                "post_id": 2001,
                "type": "like",
                "content": "有人点赞了你的帖子",
                "created_at": "2024-01-01T12:00:00Z"
            }
        ],
        "next_last_id": 12345,
        "next_delay": 5000,
        "has_more": false
    }
}
```

### 发布通知
```http
POST /api/v1/notifications/publish/like
{
    "user_id": 1001,
    "post_id": 2001,
    "actor_id": 1002
}
```

## 技术亮点

### 1. 削峰填谷
- **批处理**：Kafka自动批处理消息，减少网络开销
- **压缩**：使用GZIP压缩，减少网络传输
- **异步处理**：不阻塞主业务流程

### 2. 性能优化
- **分区策略**：用户ID作为分区键，保证消息有序
- **索引查询**：基于ID的增量查询，避免深度分页
- **缓存机制**：Redis缓存拉取状态，减少数据库查询

### 3. 智能节流
- **动态调整**：根据通知频率智能调整拉取间隔
- **资源节约**：无通知时减少请求频率
- **用户体验**：有新通知时快速响应

### 4. 容错机制
- **消息重试**：发送失败自动重试
- **缓存恢复**：前端缓存丢失时从Redis恢复
- **数据一致性**：确保通知不丢失不重复

## 使用场景

### 1. 点赞通知
```go
// 用户点赞时
notification.PublishNotification(ctx, &models.NotificationEvent{
    ReceiverID: postAuthorID,
    ActorID:    &userID,
    PostID:     &postID,
    Type:       models.NotificationTypeLike,
    Message:    "有人点赞了你的帖子",
})
```

### 2. 评论通知
```go
// 用户评论时
notification.PublishNotification(ctx, &models.NotificationEvent{
    ReceiverID: postAuthorID,
    ActorID:    &userID,
    PostID:     &postID,
    CommentID:  &commentID,
    Type:       models.NotificationTypeComment,
    Message:    "有人评论了你的帖子",
})
```

## 配置说明

### Kafka配置
```go
brokers := []string{"localhost:9092"}
topic := "bluebell-notifications"
groupID := "bluebell-notification-group"
```

### 节流配置
```go
// 退避策略时间间隔
backoffSteps := []time.Duration{
    5 * time.Second,   // 有新通知
    10 * time.Second,  // 无新通知1次
    20 * time.Second,  // 无新通知2次
    60 * time.Second,  // 无新通知3次
    5 * time.Minute,   // 最大延迟
}
```

## 监控指标

### 1. 消息队列指标
- 消息发送成功率
- 消息消费延迟
- 队列积压情况

### 2. 业务指标
- 通知发送量
- 用户拉取频率
- 缓存命中率

### 3. 性能指标
- 数据库查询时间
- Redis操作延迟
- API响应时间

## 部署建议

### 1. Kafka集群
- 至少3个broker节点
- 设置合适的副本数
- 配置监控告警

### 2. 数据库优化
- 为user_id和id字段创建复合索引
- 定期清理过期通知
- 考虑分表分库

### 3. Redis配置
- 设置合适的过期时间
- 配置持久化策略
- 监控内存使用

## 总结

这个通知系统完全按照你的需求设计，具有以下优势：

1. **高性能**：异步处理，支持高并发
2. **智能化**：动态节流，资源利用最优
3. **用户友好**：增量拉取，快速响应
4. **系统稳定**：削峰填谷，容错机制完善
5. **易于维护**：代码简洁，逻辑清晰

通过Kafka消息队列实现了你描述的所有功能，既保证了系统稳定性，又提供了良好的用户体验。
