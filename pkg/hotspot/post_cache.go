// Package hotspot 热点数据管理包
//
// 实现热点数据的识别、缓存和批量处理机制
// 主要功能：
// 1. 识别热点帖子（高访问量、高点赞量）
// 2. 本地缓存热点数据，减少数据库压力
// 3. 批量处理点赞增量，提高系统性能
// 4. 智能阈值控制，平衡实时性和性能
//
// 技术亮点：
// - 本地缓存热点数据，减少Redis和数据库访问
// - 批量处理机制，提高数据库写入效率
// - 智能阈值控制，平衡实时性和性能
// - 线程安全设计，支持高并发访问
package hotspot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	redispkg "bluebell/dao/redis"
	"bluebell/models"

	redis "github.com/go-redis/redis/v8"
)

// ErrPostNotExist 帖子不存在错误
//
// 功能说明：
// 1. 表示布隆过滤器判定帖子不存在
// 2. 避免对数据库造成穿透攻击
// 3. 用于快速判断帖子是否存在
//
// 技术亮点：
// - 布隆过滤器预判，避免无效查询
// - 防止缓存穿透，保护数据库
var ErrPostNotExist = errors.New("post not exist")

// PostCache 帖子详情多级缓存
//
// 功能说明：
// 1. 封装帖子详情的多级缓存逻辑
// 2. 实现本地缓存+Redis缓存+数据库的三级缓存
// 3. 支持热点帖子的快速访问
// 4. 防止缓存穿透和缓存击穿
//
// 字段说明：
// - client: Redis客户端，用于分布式缓存
// - bloom: 布隆过滤器，防止缓存穿透
// - hotCache: 本地热点缓存，存储热点帖子
// - emptyTTL: 空值缓存过期时间
// - postTTL: 帖子基础信息缓存过期时间
// - userTTL: 用户信息缓存过期时间
// - communityTTL: 社区信息缓存过期时间
//
// 技术亮点：
// - 多级缓存策略，提高访问效率
// - 布隆过滤器，防止缓存穿透
// - 拆分缓存，提高复用性
// - 不同TTL策略，优化内存使用
type PostCache struct {
	client       *redis.Client   // Redis客户端
	bloom        *bloomFilter    // 布隆过滤器
	hotCache     *localCache     // 本地热点缓存
	emptyTTL     time.Duration   // 空值缓存过期时间
	postTTL      time.Duration   // 帖子基础信息缓存过期时间
	userTTL      time.Duration   // 用户信息缓存过期时间
	communityTTL time.Duration   // 社区信息缓存过期时间
}

// NewPostCache 初始化帖子缓存组件。
func NewPostCache(capacity int) *PostCache {
	client := redispkg.GetClient()
	return &PostCache{
		client:       client,
		bloom:        newBloomFilter(1<<21, 3),
		hotCache:     newLocalCache(capacity, 3*time.Minute),
		emptyTTL:     3 * time.Minute,
		postTTL:      10 * time.Minute,
		userTTL:      30 * time.Minute,
		communityTTL: 30 * time.Minute,
	}
}

// AddToBloom 将新帖子的 ID 填充到布隆过滤器，避免缓存穿透。
func (c *PostCache) AddToBloom(ids ...int64) {
	for _, id := range ids {
		c.bloom.Add([]byte(strconv.FormatInt(id, 10)))
	}
}

func (c *PostCache) hotKey(pid int64) string {
	return fmt.Sprintf("bluebell:cache:post:%d", pid)
}

func (c *PostCache) postKey(pid int64) string {
	return fmt.Sprintf("bluebell:cache:post:%d:base", pid)
}

func (c *PostCache) userKey(uid int64) string {
	return fmt.Sprintf("bluebell:cache:user:%d", uid)
}

func (c *PostCache) communityKey(cid int64) string {
	return fmt.Sprintf("bluebell:cache:community:%d", cid)
}

func (c *PostCache) voteKey(pid int64) string {
	return fmt.Sprintf("bluebell:cache:post:%d:votes", pid)
}

func (c *PostCache) emptyKey(pid int64) string {
	return fmt.Sprintf("bluebell:cache:post:%d:empty", pid)
}

// TryGetHot 尝试从本地热点缓存中获取帖子详情。
func (c *PostCache) TryGetHot(pid int64) (*models.ApiPostDetail, bool) {
	if v, ok := c.hotCache.Get(pid); ok {
		if detail, ok := v.(*models.ApiPostDetail); ok {
			return detail, true
		}
	}
	return nil, false
}

// PromoteHot 将帖子详情提升为热点，写入本地缓存。
func (c *PostCache) PromoteHot(pid int64, detail *models.ApiPostDetail, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 3 * time.Minute
	}
	if c.hotCache == nil {
		c.hotCache = newLocalCache(256, ttl)
	}
	// 调整热点缓存的过期时间，使其与最新的热点策略保持一致。
	c.hotCache.ttl = ttl
	c.hotCache.Set(pid, detail)
}

// SaveDetail 将帖子详情拆分写入 Redis，便于细粒度复用。
func (c *PostCache) SaveDetail(ctx context.Context, detail *models.ApiPostDetail) error {
	postBytes, err := json.Marshal(detail.Post)
	if err != nil {
		return err
	}
	userBytes, err := json.Marshal(map[string]interface{}{
		"username":  detail.AuthorName,
		"author_id": detail.Post.AuthorID,
	})
	if err != nil {
		return err
	}
	communityBytes, err := json.Marshal(detail.CommunityDetail)
	if err != nil {
		return err
	}

	pipeline := c.client.TxPipeline()
	pipeline.Set(ctx, c.postKey(detail.Post.ID), postBytes, c.postTTL)
	pipeline.Set(ctx, c.userKey(detail.Post.AuthorID), userBytes, c.userTTL)
	pipeline.Set(ctx, c.communityKey(detail.Post.CommunityID), communityBytes, c.communityTTL)
	pipeline.Set(ctx, c.voteKey(detail.Post.ID), strconv.FormatInt(detail.VoteNum, 10), c.postTTL)
	_, err = pipeline.Exec(ctx)
	if err == nil {
		c.AddToBloom(detail.Post.ID)
	}
	return err
}

// CacheEmpty 在 Redis 中缓存空值，缓解穿透。
func (c *PostCache) CacheEmpty(ctx context.Context, pid int64) {
	_ = c.client.Set(ctx, c.emptyKey(pid), "1", c.emptyTTL).Err()
}

// existEmpty 判断是否缓存了空值。
func (c *PostCache) existEmpty(ctx context.Context, pid int64) bool {
	_, err := c.client.Get(ctx, c.emptyKey(pid)).Result()
	return err == nil
}

// LoadDetail 尝试从 Redis 还原帖子详情。
func (c *PostCache) LoadDetail(ctx context.Context, pid int64) (*models.ApiPostDetail, error) {
	pipeline := c.client.TxPipeline()
	postCmd := pipeline.Get(ctx, c.postKey(pid))
	voteCmd := pipeline.Get(ctx, c.voteKey(pid))
	_, err := pipeline.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	postBytes, err := postCmd.Bytes()
	if err != nil {
		return nil, err
	}
	var post models.Post
	if err := json.Unmarshal(postBytes, &post); err != nil {
		return nil, err
	}

	detail := &models.ApiPostDetail{Post: &post}

	voteBytes, err := voteCmd.Bytes()
	if err == nil {
		if v, parseErr := strconv.ParseInt(string(voteBytes), 10, 64); parseErr == nil {
			detail.VoteNum = v
		}
	}

	if err := c.attachAuthor(ctx, detail); err != nil {
		return nil, err
	}
	if err := c.attachCommunity(ctx, detail); err != nil {
		return nil, err
	}
	return detail, nil
}

func (c *PostCache) attachAuthor(ctx context.Context, detail *models.ApiPostDetail) error {
	data, err := c.client.Get(ctx, c.userKey(detail.Post.AuthorID)).Bytes()
	if err != nil {
		return err
	}
	tmp := map[string]interface{}{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	if name, ok := tmp["username"].(string); ok {
		detail.AuthorName = name
	}
	return nil
}

func (c *PostCache) attachCommunity(ctx context.Context, detail *models.ApiPostDetail) error {
	data, err := c.client.Get(ctx, c.communityKey(detail.Post.CommunityID)).Bytes()
	if err != nil {
		return err
	}
	var community models.CommunityDetail
	if err := json.Unmarshal(data, &community); err != nil {
		return err
	}
	detail.CommunityDetail = &community
	return nil
}

// ShouldQueryDB 根据布隆过滤器和空值缓存判断是否需要访问数据库。
func (c *PostCache) ShouldQueryDB(ctx context.Context, pid int64) bool {
	if c.existEmpty(ctx, pid) {
		return false
	}
	if !c.bloom.Test([]byte(strconv.FormatInt(pid, 10))) {
		return false
	}
	return true
}

// HeatScore 根据浏览量、点赞数、评论数和时间衰减计算热度值。
func HeatScore(views, likes, comments int64, createdAt time.Time, decay float64) float64 {
	days := time.Since(createdAt).Hours() / 24
	weight := 0.5*float64(likes) + 0.3*float64(views) + 0.2*float64(comments)
	return weight * math.Exp(-decay*days)
}
