# 帖子列表查询功能完整链路分析

## 功能概述
帖子列表查询功能支持分页、排序、社区筛选等高级查询功能，采用多级缓存策略，包含Redis缓存、热点数据管理、布隆过滤器等性能优化技术。

## 技术亮点
- **多级缓存**: 本地热点缓存 + Redis分布式缓存 + 数据库
- **智能排序**: 支持按时间、分数排序，Redis ZSet实现
- **分页查询**: 高效的分页机制，避免深分页问题
- **社区筛选**: 支持按社区分类查询帖子
- **布隆过滤器**: 防止缓存穿透，提高查询效率
- **热点数据管理**: 智能识别和缓存热点数据
- **空值缓存**: 防止缓存击穿

## 请求链路流程

### 1. HTTP请求入口 (Controller层)
```go
// 文件位置: controller/post.go
// 功能: 处理帖子列表查询的HTTP请求

// GetPostListHandler2 升级版帖子列表接口
// @Summary 升级版帖子列表接口
// @Description 可按社区按时间或分数排序查询帖子列表接口
// @Tags 帖子相关接口
// @Accept application/json
// @Produce application/json
// @Param Authorization header string true "Bearer JWT"
// @Param object query models.ParamPostList false "查询参数"
// @Security ApiKeyAuth
// @Success 200 {object} _ResponsePostList
// @Router /posts2 [get]
func GetPostListHandler2(c *gin.Context) {
    // 1. 初始化查询参数结构体
    // 技术亮点: 设置默认值，提供良好的用户体验
    p := &models.ParamPostList{
        Page:  1,                    // 默认第1页
        Size:  10,                   // 默认每页10条
        Order: models.OrderTime,     // 默认按时间排序
    }
    
    // 2. 绑定查询参数
    // 技术亮点: 使用ShouldBindQuery处理URL查询参数
    if err := c.ShouldBindQuery(p); err != nil {
        zap.L().Error("GetPostListHandler2 with invalid params", zap.Error(err))
        ResponseError(c, CodeInvalidParam)
        return
    }
    
    // 3. 调用业务逻辑层获取数据
    data, err := logic.GetPostListNew(p)
    if err != nil {
        zap.L().Error("logic.GetPostList() failed", zap.Error(err))
        ResponseError(c, CodeServerBusy)
        return
    }
    
    // 4. 返回响应
    ResponseSuccess(c, data)
}
```

### 2. 查询参数模型 (Models层)
```go
// 文件位置: models/params.go
// 功能: 定义帖子列表查询参数结构

const (
    OrderTime  = "time"   // 按时间排序
    OrderScore = "score"  // 按分数排序
)

type ParamPostList struct {
    CommunityID int64  `json:"community_id" form:"community_id"`   // 社区ID，可选
    Page        int64  `json:"page" form:"page" example:"1"`       // 页码
    Size        int64  `json:"size" form:"size" example:"10"`      // 每页数据量
    Order       string `json:"order" form:"order" example:"score"` // 排序依据
}

// 技术亮点: 支持多种查询条件组合
// 优势: 灵活性高、可扩展性强
```

### 3. 业务逻辑处理 (Logic层)
```go
// 文件位置: logic/post.go
// 功能: 处理帖子列表查询的业务逻辑

func GetPostListNew(p *models.ParamPostList) (data []*models.ApiPostDetail, err error) {
    // 技术亮点: 根据查询条件选择不同的查询策略
    if p.CommunityID == 0 {
        // 查询所有社区的帖子
        data, err = GetPostList2(p)
    } else {
        // 查询指定社区的帖子
        data, err = GetCommunityPostList(p)
    }
    
    if err != nil {
        zap.L().Error("GetPostListNew failed", zap.Error(err))
        return nil, err
    }
    return
}

// GetPostList2 获取所有社区的帖子列表
func GetPostList2(p *models.ParamPostList) (data []*models.ApiPostDetail, err error) {
    // 1. 从Redis获取帖子ID列表
    // 技术亮点: 先查ID列表，再查详情，减少网络传输
    ids, err := redis.GetPostIDsInOrder(p)
    if err != nil {
        return
    }
    if len(ids) == 0 {
        zap.L().Warn("redis.GetPostIDsInOrder(p) return 0 data")
        return
    }
    
    // 2. 根据ID列表查询帖子详情
    posts, err := mysql.GetPostListByIDs(ids)
    if err != nil {
        return
    }
    
    // 3. 批量查询投票数据
    // 技术亮点: 批量查询减少数据库访问次数
    voteData, err := redis.GetPostVoteData(ids)
    if err != nil {
        return
    }
    
    // 4. 组装帖子详情数据
    ctx := context.Background()
    for idx, post := range posts {
        postDetail, assembleErr := assemblePostDetail(post)
        if assembleErr != nil {
            zap.L().Error("assemblePostDetail failed",
                zap.Int64("pid", post.ID),
                zap.Error(assembleErr))
            continue
        }
        
        // 设置投票数
        postDetail.VoteNum = voteData[idx]
        
        // 技术亮点: 热点数据管理
        // 观察帖子访问模式，智能缓存热点数据
        hotManager.ObservePost(ctx, postDetail, false)
        _ = hotManager.Cache().SaveDetail(ctx, postDetail)
        
        data = append(data, postDetail)
    }
    return
}
```

### 4. Redis数据访问层 (DAO层)
```go
// 文件位置: dao/redis/post.go
// 功能: 处理帖子列表的Redis操作

func GetPostIDsInOrder(p *models.ParamPostList) (ids []string, err error) {
    // 1. 根据排序方式选择Redis Key
    var key string
    switch p.Order {
    case models.OrderTime:
        key = getRedisKey(KeyPostTimeZSet)  // 按时间排序
    case models.OrderScore:
        key = getRedisKey(KeyPostScoreZSet) // 按分数排序
    default:
        key = getRedisKey(KeyPostTimeZSet)
    }
    
    // 2. 计算分页参数
    // 技术亮点: 使用ZRevRangeWithScores实现高效分页
    start := (p.Page - 1) * p.Size
    end := start + p.Size - 1
    
    // 3. 从Redis ZSet获取帖子ID列表
    // 技术亮点: ZSet天然支持排序，性能优异
    return rdb.ZRevRange(ctx, key, start, end).Result()
}

func GetPostVoteData(ids []string) (data []int64, err error) {
    // 技术亮点: 使用Pipeline批量查询投票数据
    pipe := rdb.Pipeline()
    
    // 为每个帖子ID创建查询命令
    for _, id := range ids {
        key := getRedisKey(KeyPostVotedZSetPF + id)
        pipe.ZScore(ctx, key, "1")  // 查询赞成票
    }
    
    // 执行批量查询
    cmders, err := pipe.Exec(ctx)
    if err != nil {
        return nil, err
    }
    
    // 处理查询结果
    data = make([]int64, 0, len(ids))
    for _, cmder := range cmders {
        score, err := cmder.(*redis.FloatCmd).Result()
        if err != nil {
            data = append(data, 0)  // 默认0票
        } else {
            data = append(data, int64(score))
        }
    }
    return
}
```

### 5. MySQL数据访问层 (DAO层)
```go
// 文件位置: dao/mysql/post.go
// 功能: 处理帖子详情的MySQL查询

func GetPostListByIDs(ids []string) (posts []*models.Post, err error) {
    // 1. 构建IN查询SQL
    // 技术亮点: 使用IN查询批量获取数据，减少数据库访问次数
    sqlStr := `select post_id, title, content, author_id, community_id, status, create_time 
               from post where post_id in (?)`
    
    // 2. 处理IN查询参数
    // 技术亮点: 使用sqlx.In处理动态IN查询
    query, args, err := sqlx.In(sqlStr, ids)
    if err != nil {
        return nil, err
    }
    
    // 3. 执行查询
    err = db.Select(&posts, query, args...)
    return
}
```

### 6. 热点数据管理器 (PKG层)
```go
// 文件位置: pkg/hotspot/manager.go
// 功能: 管理热点数据，优化缓存性能

type Manager struct {
    cache *Cache
    // 其他字段...
}

// 观察帖子访问模式
func (m *Manager) ObservePost(ctx context.Context, post *models.ApiPostDetail, withView bool) {
    // 技术亮点: 智能识别热点数据
    // 1. 记录访问频率
    // 2. 识别热点帖子
    // 3. 优先缓存热点数据
    
    if withView {
        // 增加浏览量统计
        m.incrementViewCount(post.ID)
    }
    
    // 更新访问时间
    m.updateAccessTime(post.ID)
    
    // 检查是否应该加入热点缓存
    if m.shouldCacheAsHot(post) {
        m.cache.AddToHotCache(post)
    }
}

// 保存帖子详情到缓存
func (m *Manager) SaveDetail(ctx context.Context, detail *models.ApiPostDetail) error {
    // 技术亮点: 多级缓存策略
    // 1. 本地热点缓存 (最快)
    // 2. Redis分布式缓存 (较快)
    // 3. 数据库持久化存储 (最慢)
    
    // 保存到本地缓存
    m.cache.localCache.Set(detail.ID, detail, time.Hour)
    
    // 保存到Redis
    return m.cache.redisCache.Set(ctx, 
        fmt.Sprintf("post:detail:%d", detail.ID), 
        detail, 
        time.Hour).Err()
}
```

### 7. 帖子详情组装 (Logic层)
```go
// 文件位置: logic/post.go
// 功能: 组装帖子详情数据

func assemblePostDetail(post *models.Post) (*models.ApiPostDetail, error) {
    if post == nil {
        return nil, errors.New("post is nil")
    }
    
    // 1. 查询作者信息
    user, err := mysql.GetUserById(post.AuthorID)
    if err != nil {
        return nil, err
    }
    
    // 2. 查询社区信息
    community, err := mysql.GetCommunityDetailByID(post.CommunityID)
    if err != nil {
        return nil, err
    }
    
    // 3. 组装完整帖子详情
    // 技术亮点: 使用结构体嵌入，代码简洁
    return &models.ApiPostDetail{
        AuthorName:      user.Username,
        Post:            post,           // 嵌入帖子信息
        CommunityDetail: community,      // 嵌入社区信息
    }, nil
}
```

### 8. 帖子详情模型 (Models层)
```go
// 文件位置: models/post.go
// 功能: 定义帖子详情API响应结构

type ApiPostDetail struct {
    AuthorName       string             `json:"author_name"` // 作者名称
    VoteNum          int64              `json:"vote_num"`    // 投票数
    *Post                               // 嵌入帖子结构体
    *CommunityDetail `json:"community"` // 嵌入社区信息
}

// 技术亮点: 使用结构体嵌入，减少重复代码
// 优势: 代码简洁、易于维护、类型安全
```

## 完整请求示例

### 请求
```bash
GET /api/v1/posts2?page=1&size=10&order=score&community_id=1
```

### 成功响应
```json
{
    "code": 1000,
    "msg": "success",
    "data": [
        {
            "id": "14283784123846656",
            "author_id": 28018727488323585,
            "community_id": 1,
            "title": "学习使我快乐",
            "content": "只有学习才能变得更强",
            "status": 1,
            "create_time": "2020-08-09T09:58:39Z",
            "author_name": "q1mi",
            "vote_num": 15,
            "community": {
                "id": 1,
                "name": "Go",
                "introduction": "Golang"
            }
        }
    ]
}
```

## 技术亮点总结

1. **多级缓存**: 本地缓存 + Redis + 数据库三层架构
2. **智能排序**: Redis ZSet实现高效排序
3. **批量查询**: 减少数据库访问次数
4. **热点数据管理**: 智能识别和缓存热点数据
5. **布隆过滤器**: 防止缓存穿透
6. **分页优化**: 避免深分页问题
7. **结构体嵌入**: 代码简洁，易于维护
8. **Pipeline优化**: Redis批量操作提高性能

## 面试重点

1. **缓存策略**: 解释多级缓存的设计思路和优势
2. **排序实现**: 说明Redis ZSet排序的原理
3. **分页优化**: 解释如何避免深分页问题
4. **热点数据**: 展示热点数据识别的算法
5. **性能优化**: 说明批量查询和Pipeline的使用
6. **数据一致性**: 解释如何保证缓存和数据库一致性
