# Kafka通知系统

基于Kafka消息队列的异步通知系统，支持点赞、评论等多种通知类型，实现了增量拉取和智能节流控制。

## 功能特性

### 核心功能
- **异步通知处理**：使用Kafka消息队列实现异步通知，提高系统响应速度
- **增量拉取**：支持基于ID的增量拉取，避免深度分页问题
- **智能节流**：根据是否有新通知动态调整拉取频率，减少无效请求
- **批量处理**：支持批量发送和消费通知，提高处理效率
- **优先级支持**：不同通知类型支持不同优先级处理

### 技术亮点
- **削峰填谷**：利用Kafka的批处理和压缩功能，有效处理高并发场景
- **容错机制**：支持消息重试和错误处理，确保通知不丢失
- **监控友好**：提供健康检查和统计信息接口
- **配置灵活**：支持文件配置和环境变量配置

## 系统架构

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   前端应用   │───▶│   API接口   │───▶│  Kafka生产者 │───▶│  Kafka集群  │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
                                                              │
┌─────────────┐    ┌─────────────┐    ┌─────────────┐        │
│   数据库     │◀───│  Kafka消费者 │◀───│  Kafka集群  │────────┘
└─────────────┘    └─────────────┘    └─────────────┘
```

## 快速开始

### 1. 环境准备

```bash
# 安装Kafka（使用Docker）
docker run -d --name kafka -p 9092:9092 apache/kafka:latest

# 创建主题
docker exec kafka kafka-topics.sh --create --topic bluebell-notifications --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1
```

### 2. 配置Kafka

```yaml
# config/kafka.yaml
kafka:
  brokers:
    - "localhost:9092"
  client_id: "bluebell-notification"
  producer:
    max_retries: 3
    batch_size: 16384
    compression: "gzip"
  consumer:
    group_id: "bluebell-notification-group"
    auto_offset_reset: "latest"
    max_poll_records: 500
```

### 3. 初始化服务

```go
// 从配置文件初始化
if err := kafka.InitFromFile("config/kafka.yaml"); err != nil {
    log.Fatal(err)
}

// 启动通知服务
if err := logic.StartKafkaNotificationService(config); err != nil {
    log.Fatal(err)
}
```

### 4. 发布通知

```go
// 发布点赞通知
event := &models.NotificationEvent{
    ReceiverID: 1001,
    ActorID:    int64Ptr(1002),
    PostID:     int64Ptr(2001),
    Type:       models.NotificationTypeLike,
    Message:    "用户张三点赞了你的帖子",
    CreatedAt:  time.Now(),
}

err := logic.PublishLikeNotification(ctx, event)
```

### 5. 拉取通知

```go
// 拉取通知
pullService := logic.GetNotificationPullService()
result, err := pullService.PullNotifications(ctx, &logic.PullParam{
    UserID: 1001,
    LastID: 0,  // 首次拉取
    Limit:  20,
})
```

## API接口

### 拉取通知
```http
GET /api/v1/notifications/pull?last_id=0&limit=20
```

**参数：**
- `last_id`: 最后拉取的ID，0表示首次拉取
- `limit`: 拉取数量，默认20，最大100

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "notifications": [...],
    "next_last_id": 12345,
    "next_delay": 5000,
    "has_more": true
  }
}
```

### 获取统计信息
```http
GET /api/v1/notifications/stats
```

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "user_id": 1001,
    "unread_count": 5,
    "total_count": 100,
    "last_pulled_id": 12345,
    "last_pulled_time": "2024-01-01T12:00:00Z"
  }
}
```

### 标记已读
```http
POST /api/v1/notifications/mark-read
{
  "notification_id": 12345
}
```

## 节流策略

系统实现了智能节流控制，根据是否有新通知动态调整拉取频率：

1. **有新通知时**：重置为最快频率（5秒）
2. **无新通知时**：逐渐增加延迟时间
   - 5秒 → 10秒 → 20秒 → 1分钟 → 5分钟
3. **最大间隔**：5分钟，避免过度延迟

## 配置说明

### Kafka配置
```yaml
kafka:
  brokers: ["localhost:9092"]        # Kafka broker地址
  client_id: "bluebell-notification" # 客户端ID
  timeout: 30                        # 超时时间（秒）
  
  producer:
    max_retries: 3                   # 最大重试次数
    batch_size: 16384                # 批处理大小
    compression: "gzip"              # 压缩算法
    required_acks: 1                 # 确认副本数
    
  consumer:
    group_id: "bluebell-notification-group" # 消费者组ID
    auto_offset_reset: "latest"             # 偏移量重置策略
    max_poll_records: 500                   # 单次拉取最大记录数
```

### 通知配置
```yaml
notification:
  topic: "bluebell-notifications"    # 主题名称
  pull:
    default_limit: 20                # 默认拉取数量
    max_limit: 100                   # 最大拉取数量
    first_pull_limit: 100            # 首次拉取数量
  throttle:
    backoff_steps: [5, 10, 20, 60, 300] # 退避策略时间间隔（秒）
    max_interval: 300                # 最大间隔（秒）
```

## 监控和运维

### 健康检查
```go
// 检查Kafka管理器状态
manager := kafka.GetGlobalManager()
if err := manager.HealthCheck(); err != nil {
    log.Printf("Kafka健康检查失败: %v", err)
}

// 获取统计信息
stats := manager.GetStats()
log.Printf("Kafka统计信息: %+v", stats)
```

### 日志监控
系统提供详细的日志记录，包括：
- 消息发送和消费日志
- 错误和重试日志
- 性能统计日志

### 性能优化建议
1. **批处理大小**：根据消息量调整`batch_size`
2. **压缩算法**：使用`gzip`或`snappy`减少网络传输
3. **分区数量**：根据并发量调整主题分区数
4. **消费者组**：合理设置消费者组大小

## 故障处理

### 常见问题
1. **消息丢失**：检查Kafka集群状态和网络连接
2. **消费延迟**：检查消费者组状态和分区分配
3. **内存泄漏**：检查消费者配置和批处理大小

### 恢复策略
1. **自动重试**：生产者自动重试失败的消息
2. **偏移量重置**：消费者可以从最新位置重新开始
3. **数据补偿**：通过数据库查询补偿丢失的通知

## 扩展功能

### 自定义处理器
```go
type CustomHandler struct{}

func (h *CustomHandler) HandleMessage(ctx context.Context, message *kafka.ConsumedMessage) error {
    // 自定义处理逻辑
    return nil
}

func (h *CustomHandler) HandleBatch(ctx context.Context, messages []*kafka.ConsumedMessage) error {
    // 自定义批量处理逻辑
    return nil
}
```

### 消息过滤
```go
// 在消费者中添加消息过滤逻辑
if notification.Type == models.NotificationTypeLike {
    // 只处理点赞通知
    return h.service.HandleMessage(ctx, message)
}
```

## 总结

Kafka通知系统提供了完整的异步通知解决方案，具有以下优势：

1. **高性能**：异步处理，支持高并发
2. **高可靠**：消息持久化，支持重试
3. **易扩展**：支持水平扩展和自定义处理
4. **易监控**：提供丰富的监控和统计信息
5. **易维护**：配置灵活，日志详细

通过合理配置和使用，可以满足大规模应用的通知需求。
