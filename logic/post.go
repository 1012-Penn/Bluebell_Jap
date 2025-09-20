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

var hotManager = hotspot.GetManager()

func CreatePost(p *models.Post) (err error) {
	// 1. 生成post id
	p.ID = snowflake.GenID()
	// 2. 保存到数据库
	err = mysql.CreatePost(p)
	if err != nil {
		return err
	}
	err = redis.CreatePost(p.ID, p.CommunityID)
	if err == nil {
		// 帖子创建成功后立即加入布隆过滤器，减少缓存穿透
		hotManager.Cache().AddToBloom(p.ID)
	}
	return
	// 3. 返回
}

// GetPostById 根据帖子id查询帖子详情数据
func GetPostById(pid int64) (data *models.ApiPostDetail, err error) {
	ctx := context.Background()
	data, err = getPostDetail(ctx, pid, true)
	if err != nil {
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
