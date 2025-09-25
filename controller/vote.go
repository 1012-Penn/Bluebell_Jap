// Package controller 控制器层
//
// 负责处理HTTP请求和响应，是系统的入口层
// 主要职责：
// 1. 接收HTTP请求
// 2. 参数验证和绑定
// 3. 调用业务逻辑层
// 4. 返回HTTP响应
// 5. 错误处理和日志记录
package controller

import (
	"bluebell/logic"
	"bluebell/models"

	"go.uber.org/zap"

	"github.com/go-playground/validator/v10"

	"github.com/gin-gonic/gin"
)

// PostVoteController 帖子投票控制器
//
// 功能说明：
// 1. 接收用户对帖子的投票请求
// 2. 验证投票参数（帖子ID、投票方向）
// 3. 从JWT中获取当前用户ID
// 4. 调用业务逻辑层处理投票
// 5. 返回投票结果
//
// 请求方式：POST /api/v1/vote
// 请求头：Authorization: Bearer <JWT_TOKEN>
// 请求参数：{"post_id": "123", "direction": 1}
// 参数说明：
// - post_id: 帖子ID
// - direction: 投票方向，1表示赞成，-1表示反对，0表示取消投票
// 响应格式：{"code": 1000, "msg": "success", "data": null}
//
// 技术亮点：
// - JWT身份认证
// - 参数自动验证
// - 投票业务逻辑处理
// - 统一的错误处理
//
// 投票数据结构（已注释，使用models.ParamVoteData）
//
//	type VoteData struct {
//		// UserID 从请求中获取当前的用户
//		PostID    int64 `json:"post_id,string"`   // 贴子id
//		Direction int   `json:"direction,string"` // 赞成票(1)还是反对票(-1)
//	}
func PostVoteController(c *gin.Context) {
	// 1. 参数校验
	// 技术亮点：使用ShouldBindJSON自动绑定JSON请求体到ParamVoteData结构体
	// 同时触发validator标签验证，确保参数格式正确
	p := new(models.ParamVoteData)
	if err := c.ShouldBindJSON(p); err != nil {
		// 类型断言，判断是否为验证错误
		// 技术亮点：区分不同类型的错误，提供更精确的错误信息
		errs, ok := err.(validator.ValidationErrors)
		if !ok {
			// 非验证错误，返回通用参数错误
			ResponseError(c, CodeInvalidParam)
			return
		}
		// 翻译并去除掉错误提示中的结构体标识
		// 技术亮点：提供用户友好的错误提示，去除技术细节
		errData := removeTopStruct(errs.Translate(trans))
		ResponseErrorWithMsg(c, CodeInvalidParam, errData)
		return
	}

	// 2. 获取当前请求的用户ID
	// 技术亮点：通过JWT中间件获取用户身份，确保只有登录用户才能投票
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}

	// 3. 调用业务逻辑层处理投票
	// 技术亮点：分层架构，Controller层只负责请求处理，投票逻辑交给Logic层
	// 投票逻辑包括：检查投票状态、更新投票记录、更新帖子分数等
	if err := logic.VoteForPost(userID, p); err != nil {
		zap.L().Error("logic.VoteForPost() failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	// 4. 返回成功响应
	// 技术亮点：统一的响应格式，便于前端处理
	ResponseSuccess(c, nil)
}
