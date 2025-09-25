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
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	// swagger 嵌入文件
)

// CreatePostHandler 创建帖子的处理函数
//
// 功能说明：
// 1. 接收创建帖子的HTTP请求
// 2. 验证请求参数（标题、内容、社区ID等）
// 3. 从JWT中获取当前用户ID
// 4. 调用业务逻辑层创建帖子
// 5. 返回创建结果
//
// 请求方式：POST /api/v1/post
// 请求头：Authorization: Bearer <JWT_TOKEN>
// 请求参数：{"title": "帖子标题", "content": "帖子内容", "community_id": 1}
// 响应格式：{"code": 1000, "msg": "success", "data": null}
//
// 技术亮点：
// - JWT身份认证
// - 参数自动验证
// - 统一的错误处理
// - 结构化日志记录
func CreatePostHandler(c *gin.Context) {
	// 1. 获取参数及参数的校验
	// 技术亮点：使用ShouldBindJSON自动绑定JSON请求体到Post结构体
	// 同时触发validator标签验证，确保必填字段不为空
	p := new(models.Post)
	if err := c.ShouldBindJSON(p); err != nil {
		// 记录调试日志，包含具体错误信息
		zap.L().Debug("c.ShouldBindJSON(p) error", zap.Any("err", err))
		// 记录错误日志，便于问题追踪
		zap.L().Error("create post with invalid param")
		ResponseError(c, CodeInvalidParam)
		return
	}

	// 从JWT中获取当前发请求的用户ID
	// 技术亮点：通过JWT中间件获取用户身份，确保只有登录用户才能创建帖子
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p.AuthorID = userID // 设置帖子作者ID

	// 2. 调用业务逻辑层创建帖子
	// 技术亮点：分层架构，Controller层只负责请求处理，业务逻辑交给Logic层
	if err := logic.CreatePost(p); err != nil {
		zap.L().Error("logic.CreatePost(p) failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	// 3. 返回成功响应
	// 技术亮点：统一的响应格式，便于前端处理
	ResponseSuccess(c, nil)
}

// GetPostDetailHandler 获取帖子详情的处理函数
//
// 功能说明：
// 1. 从URL路径参数中获取帖子ID
// 2. 验证帖子ID格式是否正确
// 3. 调用业务逻辑层获取帖子详情
// 4. 返回完整的帖子信息（包含作者、社区等关联信息）
//
// 请求方式：GET /api/v1/post/:id
// 路径参数：id - 帖子ID
// 响应格式：{"code": 1000, "msg": "success", "data": {"id": "123", "title": "...", "author_name": "...", "community": {...}}}
//
// 技术亮点：
// - URL路径参数解析
// - 字符串到数字的类型转换
// - 关联数据查询和组装
// - 统一的错误处理
func GetPostDetailHandler(c *gin.Context) {
	// 1. 获取参数（从URL中获取帖子的id）
	// 技术亮点：使用c.Param()从URL路径中提取参数
	// 例如：/api/v1/post/123 中的 "123" 就是id参数
	pidStr := c.Param("id")

	// 将字符串转换为int64类型
	// 技术亮点：strconv.ParseInt参数说明
	// - pidStr: 要转换的字符串
	// - 10: 进制（10进制）
	// - 64: 位数（64位）
	pid, err := strconv.ParseInt(pidStr, 10, 64)
	if err != nil {
		// 记录参数转换失败的日志
		zap.L().Error("get post detail with invalid param", zap.Error(err))
		ResponseError(c, CodeInvalidParam)
		return
	}

	// 2. 根据id取出帖子数据（查数据库）
	// 技术亮点：调用业务逻辑层，获取包含关联信息的完整帖子数据
	data, err := logic.GetPostById(pid)
	if err != nil {
		zap.L().Error("logic.GetPostById(pid) failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	// 3. 返回响应
	// 技术亮点：返回完整的帖子详情，包含作者、社区等关联信息
	ResponseSuccess(c, data)
}

// GetPostListHandler 获取帖子列表的处理函数
//
// 功能说明：
// 1. 从查询参数中获取分页信息
// 2. 调用业务逻辑层获取帖子列表
// 3. 返回分页的帖子列表数据
//
// 请求方式：GET /api/v1/posts?page=1&size=10
// 查询参数：
// - page: 页码，默认为1
// - size: 每页大小，默认为10
// 响应格式：{"code": 1000, "msg": "success", "data": [{"id": "123", "title": "...", ...}]}
//
// 技术亮点：
// - 分页参数处理
// - 列表数据查询
// - 统一的响应格式
func GetPostListHandler(c *gin.Context) {
	// 获取分页参数
	// 技术亮点：从查询参数中提取分页信息，提供默认值
	page, size := getPageInfo(c)

	// 调用业务逻辑层获取帖子列表数据
	// 技术亮点：分层架构，Controller层只负责参数处理，业务逻辑交给Logic层
	data, err := logic.GetPostList(page, size)
	if err != nil {
		zap.L().Error("logic.GetPostList() failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	// 返回成功响应
	// 技术亮点：统一的响应格式，便于前端处理
	ResponseSuccess(c, data)
}

// GetPostListHandler2 升级版帖子列表接口
//
// 功能说明：
// 1. 支持按社区筛选帖子
// 2. 支持按时间或分数排序
// 3. 支持分页查询
// 4. 使用Redis缓存提高性能
// 5. 返回包含投票数的完整帖子信息
//
// 请求方式：GET /api/v1/posts2?page=1&size=10&order=time&community_id=1
// 查询参数：
// - page: 页码，默认为1
// - size: 每页大小，默认为10
// - order: 排序方式，time(按时间)或score(按分数)
// - community_id: 社区ID，可选，用于筛选特定社区的帖子
// 响应格式：{"code": 1000, "msg": "success", "data": [{"id": "123", "title": "...", "vote_num": 10, ...}]}
//
// 技术亮点：
// - 多条件查询支持
// - Redis缓存优化
// - 排序和分页
// - 投票数据集成
//
// Swagger文档注释
// @Summary 升级版帖子列表接口
// @Description 可按社区按时间或分数排序查询帖子列表接口
// @Tags 帖子相关接口(api分组展示使用的)
// @Accept application/json
// @Produce application/json
// @Param Authorization header string true "Bearer JWT"
// @Param object query models.ParamPostList false "查询参数"
// @Security ApiKeyAuth
// @Success 200 {object} _ResponsePostList
// @Router /posts2 [get]
func GetPostListHandler2(c *gin.Context) {
	// GET请求参数(query string)：/api/v1/posts2?page=1&size=10&order=time
	// 初始化结构体时指定初始参数
	// 技术亮点：提供合理的默认值，提高用户体验
	p := &models.ParamPostList{
		Page:  1,                // 默认第1页
		Size:  10,               // 默认每页10条
		Order: models.OrderTime, // 默认按时间排序
	}

	// 技术亮点：使用ShouldBindQuery绑定查询参数
	// c.ShouldBind() 根据请求的数据类型选择相应的方法去获取数据
	// c.ShouldBindJSON() 如果请求中携带的是json格式的数据，才能用这个方法获取到数据
	// c.ShouldBindQuery() 用于绑定URL查询参数
	if err := c.ShouldBindQuery(p); err != nil {
		zap.L().Error("GetPostListHandler2 with invalid params", zap.Error(err))
		ResponseError(c, CodeInvalidParam)
		return
	}

	// 调用升级版业务逻辑层获取帖子列表
	// 技术亮点：使用Redis缓存和排序功能，性能更优
	data, err := logic.GetPostListNew(p)
	if err != nil {
		zap.L().Error("logic.GetPostList() failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	// 返回成功响应
	// 技术亮点：返回包含投票数的完整帖子信息
	ResponseSuccess(c, data)
}

// 根据社区去查询帖子列表
//func GetCommunityPostListHandler(c *gin.Context) {
//	// 初始化结构体时指定初始参数
//	p := &models.ParamCommunityPostList{
//		ParamPostList: &models.ParamPostList{
//			Page:  1,
//			Size:  10,
//			Order: models.OrderTime,
//		},
//	}
//	//c.ShouldBind()  根据请求的数据类型选择相应的方法去获取数据
//	//c.ShouldBindJSON() 如果请求中携带的是json格式的数据，才能用这个方法获取到数据
//	if err := c.ShouldBindQuery(p); err != nil {
//		zap.L().Error("GetCommunityPostListHandler with invalid params", zap.Error(err))
//		ResponseError(c, CodeInvalidParam)
//		return
//	}
//
//	// 获取数据
//	data, err := logic.GetCommunityPostList(p)
//	if err != nil {
//		zap.L().Error("logic.GetPostList() failed", zap.Error(err))
//		ResponseError(c, CodeServerBusy)
//		return
//	}
//	ResponseSuccess(c, data)
//}
