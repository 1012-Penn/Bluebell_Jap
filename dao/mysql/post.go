// Package mysql MySQL数据访问层
//
// 负责处理MySQL数据库操作，是数据持久化层
// 主要职责：
// 1. 执行SQL语句
// 2. 数据查询和插入
// 3. 错误处理和转换
// 4. 连接池管理
// 5. 不包含业务逻辑，只负责数据操作
package mysql

import (
	"bluebell/models"
	"strings"

	"github.com/jmoiron/sqlx"
)

// CreatePost 创建帖子
//
// 功能说明：
// 1. 向post表插入新的帖子记录
// 2. 包含帖子ID、标题、内容、作者ID、社区ID等信息
//
// 参数说明：
// - p: 帖子对象，包含所有必要字段
//
// 返回值：
// - error: 插入过程中的错误
//
// 技术亮点：
// - 使用参数化查询防止SQL注入
// - 明确的字段列表，避免表结构变更影响
// - 使用雪花算法生成的ID，避免主键冲突
func CreatePost(p *models.Post) (err error) {
	// 插入帖子记录
	// 技术亮点：使用参数化查询，防止SQL注入攻击
	sqlStr := `insert into post(
	post_id, title, content, author_id, community_id)
	values (?, ?, ?, ?, ?)
	`
	_, err = db.Exec(sqlStr, p.ID, p.Title, p.Content, p.AuthorID, p.CommunityID)
	return
}

// GetPostById 根据ID查询单个帖子数据
//
// 功能说明：
// 1. 根据帖子ID查询帖子基本信息
// 2. 包含标题、内容、作者ID、社区ID、创建时间等字段
//
// 参数说明：
// - pid: 帖子ID
//
// 返回值：
// - post: 帖子信息
// - err: 查询过程中的错误
//
// 技术亮点：
// - 使用参数化查询防止SQL注入
// - 只查询必要字段，提高查询效率
// - 使用db.Get查询单条记录
func GetPostById(pid int64) (post *models.Post, err error) {
	post = new(models.Post)
	// 查询帖子基本信息
	// 技术亮点：明确指定查询字段，避免查询不必要的数据
	sqlStr := `select
	post_id, title, content, author_id, community_id, create_time
	from post
	where post_id = ?
	`
	err = db.Get(post, sqlStr, pid)
	return
}

// GetPostList 查询帖子列表函数
func GetPostList(page, size int64) (posts []*models.Post, err error) {
	sqlStr := `select 
	post_id, title, content, author_id, community_id, create_time
	from post
	ORDER BY create_time
	DESC
	limit ?,?
	`
	posts = make([]*models.Post, 0, 2) // 不要写成make([]*models.Post, 2),理由是预估返回的帖子数量为2
	err = db.Select(&posts, sqlStr, (page-1)*size, size)
	return
}

// GetPostListByIDs 根据给定的id列表查询帖子数据
func GetPostListByIDs(ids []string) (postList []*models.Post, err error) {
	sqlStr := `select post_id, title, content, author_id, community_id, create_time
	from post
	where post_id in (?)
	order by FIND_IN_SET(post_id, ?)
	`
	// https: //www.liwenzhou.com/posts/Go/sqlx/
	query, args, err := sqlx.In(sqlStr, ids, strings.Join(ids, ","))
	if err != nil {
		return nil, err
	}
	query = db.Rebind(query)
	err = db.Select(&postList, query, args...) // !!!!!!
	return
}

// InsertPostVote 插入点赞记录
func InsertPostVote(userID int64, postID string, voteType int8) error {
	// 使用 INSERT ... ON DUPLICATE KEY UPDATE 实现"插入或更新"的幂等性操作
	// 1. 如果 (user_id, post_id) 组合不存在，则插入新记录
	// 2. 如果 (user_id, post_id) 组合已存在（主键冲突），则更新现有记录
	// VALUES(vote_type) 表示使用 INSERT 语句中的 vote_type 值来更新
	// CURRENT_TIMESTAMP 更新创建时间为当前时间
	sqlStr := `INSERT INTO post_vote (user_id, post_id, vote_type) VALUES (?, ?, ?)
	ON DUPLICATE KEY UPDATE vote_type = VALUES(vote_type), create_time = CURRENT_TIMESTAMP`
	_, err := db.Exec(sqlStr, userID, postID, voteType)
	return err
}

// DeletePostVote 删除点赞记录（如果需要取消点赞时删除记录，可选）
func DeletePostVote(userID int64, postID string) error {
	sqlStr := `DELETE FROM post_vote WHERE user_id = ? AND post_id = ?`
	_, err := db.Exec(sqlStr, userID, postID)
	return err
}

// UpsertPostLikeStat 将热点帖子累计的点赞量增量写入数据库。
//
// 表结构示例：
// CREATE TABLE IF NOT EXISTS post_like_stat (
//
//	post_id BIGINT PRIMARY KEY,
//	like_count BIGINT NOT NULL DEFAULT 0
//
// );
func UpsertPostLikeStat(postID int64, delta int64) error {
	if delta == 0 {
		return nil
	}
	sqlStr := `INSERT INTO post_like_stat (post_id, like_count) VALUES (?, ?)
ON DUPLICATE KEY UPDATE like_count = GREATEST(0, like_count + VALUES(like_count))`
	_, err := db.Exec(sqlStr, postID, delta)
	return err
}
