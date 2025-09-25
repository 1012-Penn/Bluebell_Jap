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
	"database/sql"
	"errors"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/hotspot"
	"bluebell/pkg/snowflake"

	redisv8 "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// hotManager 热点数据管理器
// 技术亮点：使用单例模式管理热点数据，包括本地缓存、Redis缓存、布隆过滤器等
var hotManager = hotspot.GetManager()

// CreatePost 创建帖子业务逻辑
//
// 功能说明：
// 1. 生成全局唯一帖子ID
// 2. 保存帖子到MySQL数据库
// 3. 同步帖子ID到Redis缓存
// 4. 将帖子ID加入布隆过滤器，防止缓存穿透
//
// 参数说明：
// - p: 帖子对象，包含标题、内容、作者ID、社区ID等
//
// 返回值：
// - error: 创建过程中的错误
//
// 技术亮点：
// - 使用雪花算法生成分布式唯一ID
// - 数据库和缓存双写，保证数据一致性
// - 布隆过滤器防止缓存穿透
// - 分层架构，业务逻辑与数据访问分离
func CreatePost(p *models.Post) (err error) {
	// 1. 生成全局唯一帖子ID
	// 技术亮点：使用雪花算法生成分布式唯一ID
	// 优势：全局唯一、趋势递增、包含时间信息、高性能
	p.ID = snowflake.GenID()

	// 2. 保存帖子到MySQL数据库
	// 技术亮点：先保存到数据库，确保数据持久化
	err = mysql.CreatePost(p)
	if err != nil {
		return err
	}

	// 3. 同步帖子ID到Redis缓存
	// 技术亮点：数据库和缓存双写，保证数据一致性
	// Redis中存储帖子ID列表，支持排序和分页查询
	err = redis.CreatePost(p.ID, p.CommunityID)
	if err == nil {
		// 帖子创建成功后立即加入布隆过滤器，减少缓存穿透
		// 技术亮点：布隆过滤器可以快速判断帖子是否存在，避免无效查询
		hotManager.Cache().AddToBloom(p.ID)
	}
	return
}

// GetPostById 根据帖子ID查询帖子详情数据
//
// 功能说明：
// 1. 通过多级缓存策略获取帖子详情
// 2. 包含作者信息、社区信息等关联数据
// 3. 支持浏览量统计
// 4. 使用布隆过滤器防止缓存穿透
//
// 参数说明：
// - pid: 帖子ID
//
// 返回值：
// - data: 帖子详情，包含作者、社区等关联信息
// - err: 查询过程中的错误
//
// 技术亮点：
// - 多级缓存策略：本地缓存 -> Redis缓存 -> 数据库
// - 布隆过滤器防止缓存穿透
// - 热点数据管理
// - 浏览量统计
func GetPostById(pid int64) (data *models.ApiPostDetail, err error) {
	ctx := context.Background()
	// 技术亮点：使用多级缓存策略获取帖子详情，withView=true表示需要统计浏览量
	data, err = getPostDetail(ctx, pid, true)
	if err != nil {
		// 只记录非"帖子不存在"的错误，避免日志污染
		if !errors.Is(err, hotspot.ErrPostNotExist) {
			zap.L().Error("getPostDetail failed",
				zap.Int64("pid", pid),
				zap.Error(err))
		}
	}
	return
}

// GetPostList 获取帖子列表
func GetPostList(page, size int64) (data []*models.ApiPostDetail, err error) {
	posts, err := mysql.GetPostList(page, size)
	if err != nil {
		return nil, err
	}
	data = make([]*models.ApiPostDetail, 0, len(posts))

	ctx := context.Background()
	for _, post := range posts {
		postDetail, assembleErr := assemblePostDetail(post)
		if assembleErr != nil {
			zap.L().Error("assemblePostDetail failed",
				zap.Int64("pid", post.ID),
				zap.Error(assembleErr))
			continue
		}
		// 列表场景只做缓存填充，不增加浏览量
		hotManager.ObservePost(ctx, postDetail, false)
		_ = hotManager.Cache().SaveDetail(ctx, postDetail)
		data = append(data, postDetail)
	}
	return
}

func GetPostList2(p *models.ParamPostList) (data []*models.ApiPostDetail, err error) {
	// 2. 去redis查询id列表
	ids, err := redis.GetPostIDsInOrder(p)
	if err != nil {
		return
	}
	if len(ids) == 0 {
		zap.L().Warn("redis.GetPostIDsInOrder(p) return 0 data")
		return
	}
	zap.L().Debug("GetPostList2", zap.Any("ids", ids))
	ctx := context.Background()
	// 3. 根据id去MySQL数据库查询帖子详细信息
	// 返回的数据还要按照我给定的id的顺序返回
	posts, err := mysql.GetPostListByIDs(ids)
	if err != nil {
		return
	}
	zap.L().Debug("GetPostList2", zap.Any("posts", posts))
	// 提前查询好每篇帖子的投票数
	voteData, err := redis.GetPostVoteData(ids)
	if err != nil {
		return
	}

	// 将帖子的作者及分区信息查询出来填充到帖子中
	for idx, post := range posts {
		postDetail, assembleErr := assemblePostDetail(post)
		if assembleErr != nil {
			zap.L().Error("assemblePostDetail failed",
				zap.Int64("pid", post.ID),
				zap.Error(assembleErr))
			continue
		}
		postDetail.VoteNum = voteData[idx]
		hotManager.ObservePost(ctx, postDetail, false)
		_ = hotManager.Cache().SaveDetail(ctx, postDetail)
		data = append(data, postDetail)
	}
	return

}

func GetCommunityPostList(p *models.ParamPostList) (data []*models.ApiPostDetail, err error) {
	// 2. 去redis查询id列表
	ids, err := redis.GetCommunityPostIDsInOrder(p)
	if err != nil {
		return
	}
	if len(ids) == 0 {
		zap.L().Warn("redis.GetPostIDsInOrder(p) return 0 data")
		return
	}
	zap.L().Debug("GetCommunityPostIDsInOrder", zap.Any("ids", ids))
	// 3. 根据id去MySQL数据库查询帖子详细信息
	// 返回的数据还要按照我给定的id的顺序返回
	posts, err := mysql.GetPostListByIDs(ids)
	if err != nil {
		return
	}
	zap.L().Debug("GetPostList2", zap.Any("posts", posts))
	// 提前查询好每篇帖子的投票数
	voteData, err := redis.GetPostVoteData(ids)
	if err != nil {
		return
	}

	ctx := context.Background()
	// 将帖子的作者及分区信息查询出来填充到帖子中
	for idx, post := range posts {
		postDetail, assembleErr := assemblePostDetail(post)
		if assembleErr != nil {
			zap.L().Error("assemblePostDetail failed",
				zap.Int64("pid", post.ID),
				zap.Error(assembleErr))
			continue
		}
		postDetail.VoteNum = voteData[idx]
		hotManager.ObservePost(ctx, postDetail, false)
		_ = hotManager.Cache().SaveDetail(ctx, postDetail)
		data = append(data, postDetail)
	}
	return
}

func getPostDetail(ctx context.Context, pid int64, withView bool) (*models.ApiPostDetail, error) {
	// 1. 本地热点缓存
	if detail, ok := hotManager.Cache().TryGetHot(pid); ok {
		hotManager.ObservePost(ctx, detail, withView)
		return detail, nil
	}

	// 2. Redis 拆分缓存
	if detail, err := hotManager.Cache().LoadDetail(ctx, pid); err == nil {
		hotManager.ObservePost(ctx, detail, withView)
		return detail, nil
	} else if !errors.Is(err, redisv8.Nil) && err != nil {
		zap.L().Warn("LoadDetail failed", zap.Int64("pid", pid), zap.Error(err))
	}

	// 3. 布隆过滤器与空值缓存兜底
	if !hotManager.Cache().ShouldQueryDB(ctx, pid) {
		hotManager.Cache().CacheEmpty(ctx, pid)
		return nil, hotspot.ErrPostNotExist
	}

	// 4. 读取数据库并写入缓存
	detail, err := loadPostDetailFromDB(pid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			hotManager.Cache().CacheEmpty(ctx, pid)
		}
		return nil, err
	}
	if saveErr := hotManager.Cache().SaveDetail(ctx, detail); saveErr != nil {
		zap.L().Warn("SaveDetail failed", zap.Int64("pid", pid), zap.Error(saveErr))
	}
	hotManager.ObservePost(ctx, detail, withView)
	return detail, nil
}

func loadPostDetailFromDB(pid int64) (*models.ApiPostDetail, error) {
	post, err := mysql.GetPostById(pid)
	if err != nil {
		return nil, err
	}
	return assemblePostDetail(post)
}

func assemblePostDetail(post *models.Post) (*models.ApiPostDetail, error) {
	if post == nil {
		return nil, errors.New("post is nil")
	}
	user, err := mysql.GetUserById(post.AuthorID)
	if err != nil {
		return nil, err
	}
	community, err := mysql.GetCommunityDetailByID(post.CommunityID)
	if err != nil {
		return nil, err
	}
	return &models.ApiPostDetail{
		AuthorName:      user.Username,
		Post:            post,
		CommunityDetail: community,
	}, nil
}

// GetPostListNew  将两个查询帖子列表逻辑合二为一的函数
func GetPostListNew(p *models.ParamPostList) (data []*models.ApiPostDetail, err error) {
	// 根据请求参数的不同，执行不同的逻辑。
	if p.CommunityID == 0 {
		// 查所有
		data, err = GetPostList2(p)
	} else {
		// 根据社区id查询
		data, err = GetCommunityPostList(p)
	}
	if err != nil {
		zap.L().Error("GetPostListNew failed", zap.Error(err))
		return nil, err
	}
	return
}
