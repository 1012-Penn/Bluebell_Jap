package controller

import (
	"net/http"
	"strconv"
	"time"

	"bluebell/logic"
	"bluebell/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// PullNotifications 拉取用户通知
func PullNotifications(c *gin.Context) {
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

	// 解析参数
	param := &logic.PullParam{
		UserID: uid,
	}

	// 获取lastID参数
	if lastIDStr := c.Query("last_id"); lastIDStr != "" {
		if lastID, err := strconv.ParseInt(lastIDStr, 10, 64); err == nil {
			param.LastID = lastID
		}
	}

	// 获取limit参数
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			param.Limit = limit
		}
	}

	// 拉取通知
	service := logic.GetNotificationPullService()
	result, err := service.PullNotifications(c.Request.Context(), param)
	if err != nil {
		zap.L().Error("拉取通知失败",
			zap.Int64("user_id", uid),
			zap.Int64("last_id", param.LastID),
			zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	// 记录拉取时间
	service.RecordLastPulledTime(c.Request.Context(), uid)

	// 返回结果
	ResponseSuccess(c, gin.H{
		"notifications": result.Notifications,
		"next_last_id":  result.NextLastID,
		"next_delay":    int64(result.NextDelay / time.Millisecond), // 返回毫秒
		"has_more":      result.HasMore,
	})
}

// GetNotificationStats 获取通知统计信息
func GetNotificationStats(c *gin.Context) {
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

	// 获取统计信息
	service := logic.GetNotificationPullService()
	stats, err := service.GetNotificationStats(c.Request.Context(), uid)
	if err != nil {
		zap.L().Error("获取通知统计失败",
			zap.Int64("user_id", uid),
			zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	ResponseSuccess(c, stats)
}

// MarkAsRead 标记通知为已读
func MarkAsRead(c *gin.Context) {
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

	// 解析请求参数
	var req struct {
		NotificationID int64 `json:"notification_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		zap.L().Error("参数绑定失败", zap.Error(err))
		ResponseError(c, CodeInvalidParam)
		return
	}

	// 标记为已读
	service := logic.GetNotificationPullService()
	if err := service.MarkAsRead(c.Request.Context(), uid, req.NotificationID); err != nil {
		zap.L().Error("标记通知已读失败",
			zap.Int64("user_id", uid),
			zap.Int64("notification_id", req.NotificationID),
			zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	ResponseSuccess(c, nil)
}

// MarkAllAsRead 标记所有通知为已读
func MarkAllAsRead(c *gin.Context) {
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

	// 标记所有通知为已读
	service := logic.GetNotificationPullService()
	if err := service.MarkAllAsRead(c.Request.Context(), uid); err != nil {
		zap.L().Error("标记所有通知已读失败",
			zap.Int64("user_id", uid),
			zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	ResponseSuccess(c, nil)
}

// DeleteNotification 删除通知
func DeleteNotification(c *gin.Context) {
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

	// 解析请求参数
	var req struct {
		NotificationID int64 `json:"notification_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		zap.L().Error("参数绑定失败", zap.Error(err))
		ResponseError(c, CodeInvalidParam)
		return
	}

	// 删除通知
	service := logic.GetNotificationPullService()
	if err := service.DeleteNotification(c.Request.Context(), uid, req.NotificationID); err != nil {
		zap.L().Error("删除通知失败",
			zap.Int64("user_id", uid),
			zap.Int64("notification_id", req.NotificationID),
			zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	ResponseSuccess(c, nil)
}

// ClearNotifications 清空所有通知
func ClearNotifications(c *gin.Context) {
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

	// 清空通知
	service := logic.GetNotificationPullService()
	if err := service.ClearNotifications(c.Request.Context(), uid); err != nil {
		zap.L().Error("清空通知失败",
			zap.Int64("user_id", uid),
			zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	ResponseSuccess(c, nil)
}

// PublishNotification 发布通知（测试用）
func PublishNotification(c *gin.Context) {
	// 解析请求参数
	var req struct {
		UserID     int64  `json:"user_id" binding:"required"`
		Type       string `json:"type" binding:"required,oneof=like comment reply system"`
		Content    string `json:"content" binding:"required"`
		PostID     *int64 `json:"post_id"`
		CommentID  *int64 `json:"comment_id"`
		FromUserID *int64 `json:"from_user_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		zap.L().Error("参数绑定失败", zap.Error(err))
		ResponseError(c, CodeInvalidParam)
		return
	}

	// 构建通知事件
	event := &models.NotificationEvent{
		ReceiverID: req.UserID,
		ActorID:    req.FromUserID,
		PostID:     req.PostID,
		CommentID:  req.CommentID,
		Type:       req.Type,
		Message:    req.Content,
		CreatedAt:  time.Now(),
	}

	// 发布到Kafka
	if err := logic.PublishLikeNotification(c.Request.Context(), event); err != nil {
		zap.L().Error("发布通知失败",
			zap.Int64("user_id", req.UserID),
			zap.String("type", req.Type),
			zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	ResponseSuccess(c, gin.H{
		"message": "通知发布成功",
	})
}
