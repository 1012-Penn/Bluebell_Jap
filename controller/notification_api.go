package controller

import (
	"bluebell/models"
	"bluebell/pkg/notification"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// PullNotificationsAPI 拉取用户通知API
func PullNotificationsAPI(c *gin.Context) {
	// 获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		ResponseError(c, CodeNeedLogin)
		return
	}

	uid, ok := userID.(int64)
	if !ok {
		ResponseError(c, CodeInvalidParam)
		return
	}

	// 获取参数
	lastIDStr := c.DefaultQuery("last_id", "0")
	limitStr := c.DefaultQuery("limit", "20")

	lastID, err := strconv.ParseInt(lastIDStr, 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParam)
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}

	// 拉取通知
	result, err := notification.PullNotifications(c.Request.Context(), uid, lastID, limit)
	if err != nil {
		zap.L().Error("拉取通知失败",
			zap.Int64("user_id", uid),
			zap.Int64("last_id", lastID),
			zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	// 返回结果
	ResponseSuccess(c, gin.H{
		"notifications": result.Notifications,
		"next_last_id":  result.NextLastID,
		"next_delay":    result.NextDelay,
		"has_more":      result.HasMore,
	})
}

// PublishLikeNotification 发布点赞通知
func PublishLikeNotification(c *gin.Context) {
	// 解析参数
	var req struct {
		UserID  int64 `json:"user_id" binding:"required"`
		PostID  int64 `json:"post_id" binding:"required"`
		ActorID int64 `json:"actor_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseError(c, CodeInvalidParam)
		return
	}

	// 构建通知事件
	event := &models.NotificationEvent{
		ReceiverID: req.UserID,
		ActorID:    &req.ActorID,
		PostID:     &req.PostID,
		Type:       models.NotificationTypeLike,
		Message:    "有人点赞了你的帖子",
		CreatedAt:  time.Now(),
	}

	// 发布到队列
	if err := notification.PublishNotification(c.Request.Context(), event); err != nil {
		zap.L().Error("发布点赞通知失败",
			zap.Int64("user_id", req.UserID),
			zap.Int64("post_id", req.PostID),
			zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	ResponseSuccess(c, gin.H{
		"message": "点赞通知已发送",
	})
}

// PublishCommentNotification 发布评论通知
func PublishCommentNotification(c *gin.Context) {
	// 解析参数
	var req struct {
		UserID    int64 `json:"user_id" binding:"required"`
		PostID    int64 `json:"post_id" binding:"required"`
		CommentID int64 `json:"comment_id" binding:"required"`
		ActorID   int64 `json:"actor_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseError(c, CodeInvalidParam)
		return
	}

	// 构建通知事件
	event := &models.NotificationEvent{
		ReceiverID: req.UserID,
		ActorID:    &req.ActorID,
		PostID:     &req.PostID,
		CommentID:  &req.CommentID,
		Type:       models.NotificationTypeComment,
		Message:    "有人评论了你的帖子",
		CreatedAt:  time.Now(),
	}

	// 发布到队列
	if err := notification.PublishNotification(c.Request.Context(), event); err != nil {
		zap.L().Error("发布评论通知失败",
			zap.Int64("user_id", req.UserID),
			zap.Int64("post_id", req.PostID),
			zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	ResponseSuccess(c, gin.H{
		"message": "评论通知已发送",
	})
}
