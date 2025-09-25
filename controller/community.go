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
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CommunityHandler 获取社区列表处理器
//
// 功能说明：
// 1. 获取所有社区的基本信息
// 2. 返回社区ID和社区名称列表
// 3. 用于前端显示社区选择器
//
// 请求方式：GET /api/v1/community
// 响应格式：{"code": 1000, "msg": "success", "data": [{"id": 1, "name": "技术社区"}, ...]}
//
// 技术亮点：
// - 简单的列表查询
// - 统一的错误处理
// - 不暴露服务端错误信息
func CommunityHandler(c *gin.Context) {
	// 查询到所有的社区（community_id, community_name) 以列表的形式返回
	// 技术亮点：调用业务逻辑层获取社区列表，保持分层架构
	data, err := logic.GetCommunityList()
	if err != nil {
		// 记录错误日志，便于问题追踪
		zap.L().Error("logic.GetCommunityList() failed", zap.Error(err))
		// 不轻易把服务端报错暴露给外面，提高安全性
		ResponseError(c, CodeServerBusy)
		return
	}

	// 返回成功响应
	// 技术亮点：统一的响应格式，便于前端处理
	ResponseSuccess(c, data)
}

// CommunityDetailHandler 社区详情处理器
//
// 功能说明：
// 1. 从URL路径参数中获取社区ID
// 2. 验证社区ID格式是否正确
// 3. 调用业务逻辑层获取社区详细信息
// 4. 返回完整的社区信息（包含介绍、创建时间等）
//
// 请求方式：GET /api/v1/community/:id
// 路径参数：id - 社区ID
// 响应格式：{"code": 1000, "msg": "success", "data": {"id": 1, "name": "技术社区", "introduction": "...", "create_time": "..."}}
//
// 技术亮点：
// - URL路径参数解析
// - 字符串到数字的类型转换
// - 详细的社区信息查询
// - 统一的错误处理
func CommunityDetailHandler(c *gin.Context) {
	// 1. 获取社区ID
	// 技术亮点：使用c.Param()从URL路径中提取参数
	// 例如：/api/v1/community/1 中的 "1" 就是id参数
	idStr := c.Param("id")

	// 将字符串转换为int64类型
	// 技术亮点：strconv.ParseInt参数说明
	// - idStr: 要转换的字符串
	// - 10: 进制（10进制）
	// - 64: 位数（64位）
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		// 参数转换失败，返回参数错误
		ResponseError(c, CodeInvalidParam)
		return
	}

	// 2. 根据ID获取社区详情
	// 技术亮点：调用业务逻辑层，获取包含详细信息的社区数据
	data, err := logic.GetCommunityDetail(id)
	if err != nil {
		// 记录错误日志，便于问题追踪
		zap.L().Error("logic.GetCommunityList() failed", zap.Error(err))
		// 不轻易把服务端报错暴露给外面，提高安全性
		ResponseError(c, CodeServerBusy)
		return
	}

	// 返回成功响应
	// 技术亮点：返回完整的社区详情信息
	ResponseSuccess(c, data)
}
