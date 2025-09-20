package hotspot

import (
	"context"
	"strconv"
	"sync"
	"time"

	"bluebell/dao/mysql"
	redispkg "bluebell/dao/redis"
	"bluebell/models"

	redis "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// Manager 将热点识别、点赞缓冲和排行榜维护组合在一起，对外提供统一接口。
type Manager struct {
	cache      *PostCache
	likeBuffer *likeBuffer
	ranking    *rankingManager
	client     *redis.Client

	decay       float64
	hotScore    float64
	flushTicker *time.Ticker
	once        sync.Once
	mu          sync.RWMutex
	hotPosts    map[int64]struct{}
}

// NewManager 构造热点管理器。
func NewManager() *Manager {
	client := redispkg.GetClient()
	m := &Manager{
		cache:      NewPostCache(512),
		likeBuffer: newLikeBuffer(50, 10*time.Second),
		ranking:    newRankingManager(200),
		client:     client,
		decay:      0.1,
		hotScore:   120,
		hotPosts:   make(map[int64]struct{}),
	}
	m.startFlushLoop()
	return m
}

func (m *Manager) startFlushLoop() {
	m.once.Do(func() {
		m.flushTicker = time.NewTicker(10 * time.Minute)
		go func() {
			for range m.flushTicker.C {
				m.FlushRanking(context.Background())
				m.FlushLikeBuffer(context.Background())
			}
		}()
	})
}

// Cache 返回帖子缓存组件，用于帖子查询逻辑。
func (m *Manager) Cache() *PostCache {
	return m.cache
}

// ObservePost 在帖子被访问后调用，记录浏览量并尝试识别热点。
func (m *Manager) ObservePost(ctx context.Context, detail *models.ApiPostDetail, withView bool) {
	if detail == nil || detail.Post == nil {
		return
	}
	pid := detail.Post.ID
	if withView {
		m.addMetric(ctx, pid, detail.Post.CreateTime, "views", 1)
	}
	score := m.calculateAndUpdate(ctx, pid, detail.Post.CreateTime)
	if score >= m.hotScore {
		m.markHot(detail, score)
	}
}

func (m *Manager) markHot(detail *models.ApiPostDetail, score float64) {
	pid := detail.Post.ID
	m.mu.Lock()
	_, alreadyHot := m.hotPosts[pid]
	m.hotPosts[pid] = struct{}{}
	m.mu.Unlock()

	if !alreadyHot {
		// 将帖子写入本地热点缓存，提高命中率
		m.cache.PromoteHot(pid, detail, 5*time.Minute)
	}
	m.ranking.Update(pid, score)
}

// FlushRanking 将本地排名快照批量写入 Redis ZSET。
func (m *Manager) FlushRanking(ctx context.Context) {
	snapshot := m.ranking.Snapshot()
	if len(snapshot) == 0 {
		return
	}
	members := make([]*redis.Z, 0, len(snapshot))
	for _, item := range snapshot {
		members = append(members, &redis.Z{Score: item.score, Member: item.key})
	}
	_ = m.client.ZAdd(ctx, "bluebell:hot:ranking", members...).Err()
}

// FlushLikeBuffer 将热点帖子的点赞增量批量写入 Redis 和数据库。
func (m *Manager) FlushLikeBuffer(ctx context.Context) {
	entries := m.likeBuffer.ForceFlush()
	for pid, delta := range entries {
		m.persistLikeDelta(ctx, pid, delta)
	}
}

// HandleLikeEvent 处理点赞增量，热点帖子触发缓冲策略。
func (m *Manager) HandleLikeEvent(ctx context.Context, pid int64, createdAt time.Time, delta int64) {
	score := m.addMetric(ctx, pid, createdAt, "likes", delta)
	if score >= m.hotScore {
		m.mu.Lock()
		m.hotPosts[pid] = struct{}{}
		m.mu.Unlock()
	}
	if m.isHot(pid) {
		if flush, value := m.likeBuffer.Add(pid, delta); flush {
			m.persistLikeDelta(ctx, pid, value)
		}
	} else {
		m.persistLikeDelta(ctx, pid, delta)
	}
}

// HandleCommentEvent 评论同样参与热度计算。
func (m *Manager) HandleCommentEvent(ctx context.Context, pid int64, createdAt time.Time, delta int64) {
	m.addMetric(ctx, pid, createdAt, "comments", delta)
}

func (m *Manager) isHot(pid int64) bool {
	m.mu.RLock()
	_, ok := m.hotPosts[pid]
	m.mu.RUnlock()
	return ok
}

func (m *Manager) addMetric(ctx context.Context, pid int64, createdAt time.Time, field string, delta int64) float64 {
	if pid == 0 {
		return 0
	}
	key := "bluebell:hot:metric:" + strconv.FormatInt(pid, 10)
	_ = m.client.HIncrBy(ctx, key, field, delta).Err()
	_ = m.client.Expire(ctx, key, 24*time.Hour).Err()
	return m.calculateAndUpdate(ctx, pid, createdAt)
}

func (m *Manager) calculateAndUpdate(ctx context.Context, pid int64, createdAt time.Time) float64 {
	key := "bluebell:hot:metric:" + strconv.FormatInt(pid, 10)
	data, err := m.client.HGetAll(ctx, key).Result()
	if err != nil {
		return 0
	}
	views := parseInt64(data["views"])
	likes := parseInt64(data["likes"])
	comments := parseInt64(data["comments"])
	score := HeatScore(views, likes, comments, createdAt, m.decay)
	m.ranking.Update(pid, score)
	return score
}

func parseInt64(v string) int64 {
	if v == "" {
		return 0
	}
	value, _ := strconv.ParseInt(v, 10, 64)
	return value
}

func (m *Manager) persistLikeDelta(ctx context.Context, pid int64, delta int64) {
	if delta == 0 {
		return
	}
	field := "bluebell:hot:likes:" + strconv.FormatInt(pid, 10)
	_ = m.client.IncrBy(ctx, field, delta).Err()
	_ = m.client.Expire(ctx, field, 24*time.Hour).Err()
	if err := mysql.UpsertPostLikeStat(pid, delta); err != nil {
		zap.L().Error("UpsertPostLikeStat failed", zap.Int64("pid", pid), zap.Error(err))
	}
}

// GlobalManager 为外部调用提供一个懒加载的单例，避免在不同逻辑中重复创建对象。
var (
	globalManager     *Manager
	globalManagerOnce sync.Once
)

// GetManager 返回全局热点管理器实例。
func GetManager() *Manager {
	globalManagerOnce.Do(func() {
		globalManager = NewManager()
	})
	return globalManager
}
