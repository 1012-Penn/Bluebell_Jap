package routes

import (
	"bluebell/controller"
	"bluebell/middleware"

	"github.com/gin-gonic/gin"
)

// SetupNotificationRoutes 设置通知相关路由
func SetupNotificationRoutes(r *gin.Engine) {
	// 通知相关路由组
	notificationGroup := r.Group("/api/v1/notifications")
	notificationGroup.Use(middleware.JWTAuthMiddleware()) // 需要登录

	{
		// 拉取通知
		notificationGroup.GET("/pull", controller.PullNotifications)
		
		// 获取通知统计
		notificationGroup.GET("/stats", controller.GetNotificationStats)
		
		// 标记通知为已读
		notificationGroup.POST("/mark-read", controller.MarkAsRead)
		
		// 标记所有通知为已读
		notificationGroup.POST("/mark-all-read", controller.MarkAllAsRead)
		
		// 删除通知
		notificationGroup.DELETE("/delete", controller.DeleteNotification)
		
		// 清空所有通知
		notificationGroup.DELETE("/clear", controller.ClearNotifications)
	}

	// 管理员路由组（用于测试）
	adminGroup := r.Group("/api/v1/admin/notifications")
	adminGroup.Use(middleware.JWTAuthMiddleware()) // 需要登录

	{
		// 发布通知（测试用）
		adminGroup.POST("/publish", controller.PublishNotification)
	}
}
