package logic

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/hotspot"

	"go.uber.org/zap"
)

// 推荐阅读
// 基于用户投票的相关算法：http://www.ruanyifeng.com/blog/algorithm/

// 本项目使用简化版的投票分数
// 投一票就加432分   86400/200  --> 200张赞成票可以给你的帖子续一天

/* 投票的几种情况：
direction=1时，有两种情况：
        1. 之前没有投过票，现在投赞成票    --> 更新分数和投票记录
        2. 之前投反对票，现在改投赞成票    --> 更新分数和投票记录
direction=0时，有两种情况：
        1. 之前投过赞成票，现在要取消投票  --> 更新分数和投票记录
        2. 之前投过反对票，现在要取消投票  --> 更新分数和投票记录
direction=-1时，有两种情况：
        1. 之前没有投过票，现在投反对票    --> 更新分数和投票记录
        2. 之前投赞成票，现在改投反对票    --> 更新分数和投票记录

投票的限制：
每个贴子自发表之日起一个星期之内允许用户投票，超过一个星期就不允许再投票了。
        1. 到期之后将redis中保存的赞成票数及反对票数存储到mysql表中
        2. 到期之后删除那个 KeyPostVotedZSetPF
*/

// VoteForPost 为帖子投票的函数
func VoteForPost(userID int64, p *models.ParamVoteData) error {
	zap.L().Debug("VoteForPost",
		zap.Int64("userID", userID),
		zap.String("postID", p.PostID),
		zap.Int8("direction", p.Direction))
	prev, err := redis.VoteForPost(strconv.Itoa(int(userID)), p.PostID, float64(p.Direction))
	if err != nil {
		return err
	}
	pid := pidFromString(p.PostID)

	// 同步写入MySQL，做幂等性处理
	if p.Direction == 0 {
		// 取消点赞，删除记录
		handleLikeDelta(userID, pid, prev, 0)
		return mysql.DeletePostVote(userID, p.PostID)
	}
	// 点赞或反对，插入或更新记录
	if err := mysql.InsertPostVote(userID, p.PostID, p.Direction); err != nil {
		return err
	}
	handleLikeDelta(userID, pid, prev, float64(p.Direction))
	return nil
}

func pidFromString(pid string) int64 {
	value, _ := strconv.ParseInt(pid, 10, 64)
	return value
}

func handleLikeDelta(actorID, pid int64, prev, current float64) {
	if pid == 0 {
		return
	}
	var delta int64
	switch {
	case current == 1 && prev < 1:
		delta = 1
	case current <= 0 && prev > 0:
		delta = -1
	}
	if delta == 0 {
		return
	}
	ctx := context.Background()
	detail, err := getPostDetail(ctx, pid, false)
	var created time.Time
	if err == nil && detail != nil && detail.Post != nil {
		created = detail.Post.CreateTime
	}
	if created.IsZero() {
		created = time.Now()
	}
	hotspot.GetManager().HandleLikeEvent(ctx, pid, created, delta)

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
		if err := PublishLikeNotification(ctx, event); err != nil {
			zap.L().Error("PublishLikeNotification failed", zap.Error(err))
		}
	}
}
