# 简化版Kafka通知系统

这是一个极简的Kafka消息队列实现，只有100多行代码，满足基本的异步通知需求。

## 核心功能

- **发布通知**：将点赞、评论等事件发送到Kafka
- **消费消息**：从Kafka消费消息并写入数据库
- **自动重试**：消息发送失败时自动重试
- **简单易用**：只需要几行代码就能使用

## 快速开始

### 1. 启动Kafka
```bash
# 使用Docker启动Kafka
docker run -d --name kafka -p 9092:9092 apache/kafka:latest

# 创建主题
docker exec kafka kafka-topics.sh --create --topic bluebell-notifications --bootstrap-server localhost:9092 --partitions 1 --replication-factor 1
```

### 2. 初始化系统
```go
// 在main函数中初始化
if err := logic.InitSimpleNotificationSystem(); err != nil {
    log.Fatal(err)
}
defer logic.CloseSimpleNotificationSystem()
```

### 3. 发布通知
```go
// 发布点赞通知
err := logic.PublishSimpleLikeNotification(ctx, 1001, 2001, 1002)
// 参数：接收用户ID, 帖子ID, 操作者ID

// 发布评论通知  
err := logic.PublishSimpleCommentNotification(ctx, 1001, 2001, 3001, 1003)
// 参数：接收用户ID, 帖子ID, 评论ID, 操作者ID
```

## 代码结构

```
pkg/simple_kafka/
├── notify.go          # 核心实现（100行代码）
logic/
├── simple_notify.go   # 业务逻辑封装
examples/
├── simple_kafka_example.go  # 使用示例
```

## 核心代码解析

### 生产者
```go
type SimpleNotificationProducer struct {
    producer sarama.SyncProducer
    topic    string
}

func (p *SimpleNotificationProducer) PublishNotification(ctx context.Context, event *models.NotificationEvent) error {
    // 1. 生成ID
    if event.ID == 0 {
        event.ID = snowflake.GenID()
    }
    
    // 2. 序列化消息
    messageData, err := json.Marshal(event)
    if err != nil {
        return err
    }
    
    // 3. 发送到Kafka
    msg := &sarama.ProducerMessage{
        Topic: p.topic,
        Key:   sarama.StringEncoder(fmt.Sprintf("user_%d", event.ReceiverID)),
        Value: sarama.ByteEncoder(messageData),
    }
    
    _, _, err = p.producer.SendMessage(msg)
    return err
}
```

### 消费者
```go
type SimpleNotificationHandler struct{}

func (h *SimpleNotificationHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
    for {
        select {
        case message := <-claim.Messages():
            // 1. 解析消息
            var event models.NotificationEvent
            json.Unmarshal(message.Value, &event)
            
            // 2. 写入数据库
            mysql.InsertNotification(&event)
            
            // 3. 标记已处理
            session.MarkMessage(message.Topic, message.Partition, message.Offset, "")
        }
    }
}
```

## 配置说明

只需要配置3个参数：
- `brokers`: Kafka地址列表
- `topic`: 主题名称
- `groupID`: 消费者组ID

```go
brokers := []string{"localhost:9092"}
topic := "bluebell-notifications"  
groupID := "bluebell-group"
```

## 优势

1. **极简设计**：只有100多行核心代码
2. **零配置**：开箱即用，无需复杂配置
3. **自动处理**：自动重试、自动消费
4. **易于理解**：代码逻辑清晰，容易维护
5. **满足需求**：完全满足基本的异步通知需求

## 使用场景

- 点赞、评论等简单通知
- 不需要复杂路由和过滤
- 对性能要求不是特别高
- 希望快速实现异步处理

## 注意事项

1. 确保Kafka服务正常运行
2. 主题需要提前创建
3. 消费者组ID要唯一
4. 数据库连接要正常

## 总结

这个简化版本去掉了所有复杂的配置和功能，只保留核心的消息队列功能。对于大多数应用场景来说，这已经足够了。如果你需要更高级的功能（如消息过滤、优先级、监控等），再考虑使用完整版本。
