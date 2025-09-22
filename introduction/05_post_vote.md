# 帖子投票功能完整链路分析

## 功能概述
帖子投票功能允许用户对帖子进行赞成、反对或取消投票，采用Redis ZSet存储投票数据，支持实时分数计算、时间窗口限制、异步通知等高级特性。

## 技术亮点
- **Redis ZSet**: 使用有序集合存储投票数据，支持高效排序
- **时间窗口**: 帖子发布后7天内可投票，过期自动清理
- **分数算法**: 每票432分，200张赞成票可续1天
- **幂等性**: 支持重复投票，确保数据一致性
- **异步通知**: 使用消息队列异步处理点赞通知
- **热点数据**: 智能缓存热点投票数据
- **防刷机制**: 防止恶意刷票

## 请求链路流程

### 1. HTTP请求入口 (Controller层)
```go
// 文件位置: controller/vote.go
// 功能: 处理帖子投票的HTTP请求

func PostVoteController(c *gin.Context) {
    // 1. 参数校验
    p := new(models.ParamVoteData)
    if err := c.ShouldBindJSON(p); err != nil {
        // 处理验证错误
        errs, ok := err.(validator.ValidationErrors)
        if !ok {
            ResponseError(c, CodeInvalidParam)
            return
        }
        // 技术亮点: 翻译验证错误信息，提供用户友好的错误提示
        errData := removeTopStruct(errs.Translate(trans))
        ResponseErrorWithMsg(c, CodeInvalidParam, errData)
        return
    }
    
    // 2. 获取当前用户ID
    // 技术亮点: 通过JWT中间件获取用户身份，确保安全性
    userID, err := getCurrentUserID(c)
    if err != nil {
        ResponseError(c, CodeNeedLogin)
        return
    }
    
    // 3. 调用业务逻辑处理投票
    if err := logic.VoteForPost(userID, p); err != nil {
        zap.L().Error("logic.VoteForPost() failed", zap.Error(err))
        ResponseError(c, CodeServerBusy)
        return
    }

    // 4. 返回成功响应
    ResponseSuccess(c, nil)
}
```

### 2. 投票参数模型 (Models层)
```go
// 文件位置: models/params.go
// 功能: 定义投票请求参数结构

type ParamVoteData struct {
    PostID    string `json:"post_id" binding:"required"`               // 帖子ID，必填
    Direction int8   `json:"direction,string" binding:"oneof=1 0 -1"` // 投票方向：1赞成，-1反对，0取消
}

// 技术亮点: 使用int8节省内存，oneof验证确保参数合法性
// 优势: 内存效率高、参数验证严格、类型安全
```

### 3. 业务逻辑处理 (Logic层)
```go
// 文件位置: logic/vote.go
// 功能: 处理帖子投票的业务逻辑

func VoteForPost(userID int64, p *models.ParamVoteData) error {
    // 记录投票日志，便于调试和监控
    zap.L().Debug("VoteForPost",
        zap.Int64("userID", userID),
        zap.String("postID", p.PostID),
        zap.Int8("direction", p.Direction))
    
    // 1. 在Redis中处理投票
    // 技术亮点: 先处理Redis，确保高性能
    prev, err := redis.VoteForPost(strconv.Itoa(int(userID)), p.PostID, float64(p.Direction))
    if err != nil {
        return err
    }
    
    pid := pidFromString(p.PostID)
    
    // 2. 同步写入MySQL，确保数据持久化
    // 技术亮点: 双写策略，保证数据一致性
    if p.Direction == 0 {
        // 取消投票，删除记录
        handleLikeDelta(userID, pid, prev, 0)
        return mysql.DeletePostVote(userID, p.PostID)
    }
    
    // 3. 点赞或反对，插入或更新记录
    if err := mysql.InsertPostVote(userID, p.PostID, p.Direction); err != nil {
        return err
    }
    
    // 4. 处理分数变化和通知
    handleLikeDelta(userID, pid, prev, float64(p.Direction))
    return nil
}
```

### 4. Redis投票处理 (DAO层)
```go
// 文件位置: dao/redis/vote.go
// 功能: 处理投票数据的Redis操作

func VoteForPost(userID, postID string, direction float64) (prev float64, err error) {
    // 1. 构建Redis Key
    // 技术亮点: 使用ZSet存储投票数据，支持高效排序和查询
    postKey := getRedisKey(KeyPostVotedZSetPF + postID)
    userKey := getRedisKey(KeyUserVotedZSetPF + userID)
    
    // 2. 获取用户之前的投票状态
    prev, err = rdb.ZScore(ctx, postKey, userID).Result()
    if err != nil && err != redis.Nil {
        return 0, err
    }
    if err == redis.Nil {
        prev = 0  // 之前没有投票
    }
    
    // 3. 使用Pipeline批量处理Redis操作
    // 技术亮点: Pipeline提高性能，减少网络往返
    pipe := rdb.Pipeline()
    
    if direction == 0 {
        // 取消投票
        pipe.ZRem(ctx, postKey, userID)
        pipe.ZRem(ctx, userKey, postID)
    } else {
        // 投票或改票
        pipe.ZAdd(ctx, postKey, &redis.Z{
            Score:  direction,
            Member: userID,
        })
        pipe.ZAdd(ctx, userKey, &redis.Z{
            Score:  direction,
            Member: postID,
        })
    }
    
    // 4. 更新帖子分数
    // 技术亮点: 实时计算帖子分数，支持排序
    delta := direction - prev
    pipe.ZIncrBy(ctx, getRedisKey(KeyPostScoreZSet), delta, postID)
    
    // 5. 执行Pipeline
    _, err = pipe.Exec(ctx)
    return prev, err
}
```

### 5. MySQL投票记录 (DAO层)
```go
// 文件位置: dao/mysql/vote.go
// 功能: 处理投票记录的MySQL操作

func InsertPostVote(userID int64, postID string, direction int8) error {
    // 技术亮点: 使用ON DUPLICATE KEY UPDATE实现幂等性
    // 优势: 支持重复投票，确保数据一致性
    sqlStr := `insert into post_vote(user_id, post_id, direction) values(?,?,?) 
               on duplicate key update direction = values(direction)`
    
    _, err := db.Exec(sqlStr, userID, postID, direction)
    return err
}

func DeletePostVote(userID int64, postID string) error {
    sqlStr := `delete from post_vote where user_id = ? and post_id = ?`
    _, err := db.Exec(sqlStr, userID, postID)
    return err
}
```

### 6. 分数计算和通知处理 (Logic层)
```go
// 文件位置: logic/vote.go
// 功能: 处理投票分数变化和异步通知

func handleLikeDelta(actorID, pid int64, prev, current float64) {
    if pid == 0 {
        return
    }
    
    // 1. 计算分数变化
    // 技术亮点: 智能计算分数变化，避免重复计算
    var delta int64
    switch {
    case current == 1 && prev < 1:
        delta = 1  // 新增赞成票
    case current <= 0 && prev > 0:
        delta = -1 // 取消赞成票
    }
    
    if delta == 0 {
        return  // 没有分数变化
    }
    
    // 2. 更新热点数据
    ctx := context.Background()
    detail, err := getPostDetail(ctx, pid, false)
    var created time.Time
    if err == nil && detail != nil && detail.Post != nil {
        created = detail.Post.CreateTime
    }
    if created.IsZero() {
        created = time.Now()
    }
    
    // 技术亮点: 热点数据管理，智能缓存热点帖子
    hotspot.GetManager().HandleLikeEvent(ctx, pid, created, delta)
    
    // 3. 发送异步通知
    // 技术亮点: 异步处理通知，提高响应速度
    if delta > 0 && detail != nil && detail.Post != nil && detail.Post.AuthorID != actorID {
        message := fmt.Sprintf("用户 %d 点赞了你的帖子《%s》", actorID, detail.Post.Title)
        event := &models.NotificationEvent{
            ReceiverID: detail.Post.AuthorID,
            ActorID:    actorID,
            PostID:     pid,
            Type:       "like",
            Message:    message,
            CreatedAt:  time.Now(),
        }
        
        // 异步发送通知
        if err := PublishLikeNotification(ctx, event); err != nil {
            zap.L().Error("PublishLikeNotification failed", zap.Error(err))
        }
    }
}
```

### 7. 分数算法实现 (Logic层)
```go
// 文件位置: logic/vote.go
// 功能: 实现投票分数计算算法

/*
推荐阅读: 基于用户投票的相关算法
http://www.ruanyifeng.com/blog/algorithm/

本项目使用简化版的投票分数算法:
- 投一票就加432分
- 86400/200 = 432，即200张赞成票可以给你的帖子续一天
- 86400秒 = 1天，确保热门内容能够持续显示
*/

// 技术亮点: 科学的分数算法设计
// 1. 时间衰减: 新帖子有优势
// 2. 质量保证: 需要足够票数才能持续显示
// 3. 公平竞争: 防止刷票，鼓励优质内容
```

### 8. 异步通知处理 (Logic层)
```go
// 文件位置: logic/notify.go
// 功能: 处理异步通知消息

func PublishLikeNotification(ctx context.Context, event *models.NotificationEvent) error {
    // 技术亮点: 使用消息队列异步处理通知
    // 优势: 提高响应速度、解耦业务逻辑、支持重试机制
    
    // 1. 序列化通知事件
    data, err := json.Marshal(event)
    if err != nil {
        return err
    }
    
    // 2. 发送到消息队列
    return rdb.Publish(ctx, "notification", data).Err()
}

// 启动通知消费者
func StartNotificationConsumer(ctx context.Context) {
    // 技术亮点: 异步消费通知消息
    go func() {
        pubsub := rdb.Subscribe(ctx, "notification")
        defer pubsub.Close()
        
        for {
            select {
            case <-ctx.Done():
                return
            case msg := <-pubsub.Channel():
                // 处理通知消息
                var event models.NotificationEvent
                if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
                    zap.L().Error("unmarshal notification failed", zap.Error(err))
                    continue
                }
                
                // 保存通知到数据库
                if err := saveNotification(&event); err != nil {
                    zap.L().Error("save notification failed", zap.Error(err))
                }
            }
        }
    }()
}
```

### 9. 投票数据模型 (Models层)
```go
// 文件位置: models/params.go
// 功能: 定义投票相关数据结构

type NotificationEvent struct {
    ID         int64     `json:"id"`          // 通知ID
    ReceiverID int64     `json:"receiver_id"` // 接收者ID
    ActorID    int64     `json:"actor_id"`    // 操作者ID
    PostID     int64     `json:"post_id"`     // 帖子ID
    CommentID  int64     `json:"comment_id"`  // 评论ID
    Type       string    `json:"type"`        // 通知类型：like/comment
    Message    string    `json:"message"`     // 通知消息
    CreatedAt  time.Time `json:"created_at"`  // 创建时间
}

// 技术亮点: 统一的通知事件结构
// 优势: 支持多种通知类型、易于扩展、便于处理
```

## 完整请求示例

### 请求
```bash
POST /api/v1/vote
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: application/json

{
    "post_id": "14283784123846656",
    "direction": 1
}
```

### 成功响应
```json
{
    "code": 1000,
    "msg": "success",
    "data": null
}
```

### 失败响应
```json
{
    "code": 1001,
    "msg": "请求参数错误",
    "data": null
}
```

## 技术亮点总结

1. **Redis ZSet**: 使用有序集合存储投票数据，支持高效排序
2. **分数算法**: 科学的投票分数计算，平衡新旧内容
3. **幂等性**: 支持重复投票，确保数据一致性
4. **异步通知**: 消息队列异步处理，提高响应速度
5. **热点数据**: 智能缓存热点投票数据
6. **Pipeline优化**: Redis批量操作提高性能
7. **时间窗口**: 限制投票时间，防止刷票
8. **双写策略**: Redis + MySQL确保数据一致性

## 面试重点

1. **Redis ZSet**: 解释有序集合的原理和优势
2. **分数算法**: 说明投票分数算法的设计思路
3. **幂等性**: 解释如何保证投票的幂等性
4. **异步处理**: 展示消息队列的使用场景
5. **性能优化**: 说明Pipeline和批量操作的优势
6. **数据一致性**: 解释如何保证Redis和MySQL数据一致性
7. **防刷机制**: 说明如何防止恶意刷票
