package models

import "time"

// 排序常量定义
// 技术亮点：使用常量避免魔法字符串，提高代码可维护性
const (
	OrderTime  = "time"  // 按时间排序
	OrderScore = "score" // 按分数排序
)

// 通知类型常量
const (
	NotificationTypeLike    = "like"    // 点赞
	NotificationTypeComment = "comment" // 评论
	NotificationTypeReply   = "reply"   // 回复
	NotificationTypeSystem  = "system"  // 系统通知
)

// ParamSignUp 用户注册请求参数
//
// 功能说明：
// 1. 定义用户注册接口的请求参数结构
// 2. 支持JSON绑定和参数验证
// 3. 包含密码确认验证
//
// 字段说明：
// - Username: 用户名，必填
// - Password: 密码，必填
// - RePassword: 确认密码，必填且必须与密码相同
//
// 技术亮点：
// - 使用validator标签进行参数验证
// - 支持密码确认验证（eqfield=Password）
// - JSON标签支持自动绑定
type ParamSignUp struct {
	Username   string `json:"username" binding:"required"`                          // 用户名，必填
	Password   string `json:"password" binding:"required"`                          // 密码，必填
	RePassword string `json:"confirm_password" binding:"required,eqfield=Password"` // 确认密码，必填且必须与密码相同
}

// ParamLogin 用户登录请求参数
//
// 功能说明：
// 1. 定义用户登录接口的请求参数结构
// 2. 支持JSON绑定和参数验证
// 3. 包含基本的用户名和密码验证
//
// 字段说明：
// - Username: 用户名，必填
// - Password: 密码，必填
//
// 技术亮点：
// - 简洁的参数结构，只包含必要字段
// - 使用validator标签进行必填验证
// - JSON标签支持自动绑定
type ParamLogin struct {
	Username string `json:"username" binding:"required"` // 用户名，必填
	Password string `json:"password" binding:"required"` // 密码，必填
}

// ParamVoteData 帖子投票请求参数
//
// 功能说明：
// 1. 定义帖子投票接口的请求参数结构
// 2. 支持赞成、反对、取消投票三种操作
// 3. 包含严格的参数验证
//
// 字段说明：
// - PostID: 帖子ID，必填
// - Direction: 投票方向，1表示赞成，-1表示反对，0表示取消投票
//
// 技术亮点：
// - 使用int8节省内存，提高性能
// - 使用oneof验证确保参数合法性
// - 支持字符串到int8的自动转换
type ParamVoteData struct {
	PostID    string `json:"post_id" binding:"required"`              // 帖子ID，必填
	Direction int8   `json:"direction,string" binding:"oneof=1 0 -1"` // 投票方向：1赞成，-1反对，0取消
	// 技术亮点：使用int8节省内存，int8取值范围-128到127，满足投票需求
}

// ParamPostList 帖子列表查询参数
//
// 功能说明：
// 1. 定义帖子列表查询接口的请求参数结构
// 2. 支持分页、排序、社区筛选等高级查询
// 3. 支持URL查询参数和JSON参数绑定
//
// 字段说明：
// - CommunityID: 社区ID，可选，用于筛选特定社区的帖子
// - Page: 页码，从1开始
// - Size: 每页数据量
// - Order: 排序依据，支持按时间或分数排序
//
// 技术亮点：
// - 支持多种参数绑定方式（form和json）
// - 提供默认值示例，便于API文档生成
// - 灵活的查询条件组合
type ParamPostList struct {
	CommunityID int64  `json:"community_id" form:"community_id"`   // 社区ID，可选
	Page        int64  `json:"page" form:"page" example:"1"`       // 页码，从1开始
	Size        int64  `json:"size" form:"size" example:"10"`      // 每页数据量
	Order       string `json:"order" form:"order" example:"score"` // 排序依据：time或score
}

// NotificationEvent 消息通知事件
//
// 功能说明：
// 1. 定义消息队列中的通知事件结构
// 2. 支持点赞、评论等多种通知类型
// 3. 包含完整的通知上下文信息
//
// 字段说明：
// - ID: 通知ID
// - ReceiverID: 接收者ID
// - ActorID: 操作者ID（指针类型，支持nil）
// - PostID: 相关帖子ID（指针类型，支持nil）
// - CommentID: 相关评论ID（指针类型，支持nil）
// - Type: 通知类型（like/comment）
// - Message: 通知消息内容
// - CreatedAt: 创建时间
//
// 技术亮点：
// - 统一的通知事件结构，支持多种通知类型
// - 包含完整的上下文信息，便于处理
// - 支持时间戳，便于排序和去重
// - 使用指针类型支持可选字段
type NotificationEvent struct {
	ID         int64     `json:"id"`          // 通知ID
	ReceiverID int64     `json:"receiver_id"` // 接收者ID
	ActorID    *int64    `json:"actor_id"`    // 操作者ID（指针类型，支持nil）
	PostID     *int64    `json:"post_id"`     // 相关帖子ID（指针类型，支持nil）
	CommentID  *int64    `json:"comment_id"`  // 相关评论ID（指针类型，支持nil）
	Type       string    `json:"type"`        // 通知类型：like/comment
	Message    string    `json:"message"`     // 通知消息内容
	CreatedAt  time.Time `json:"created_at"`  // 创建时间
}

// NotificationPullParam 通知拉取参数
//
// 功能说明：
// 1. 定义前端拉取通知时的查询参数
// 2. 支持分页和增量拉取
// 3. 优化通知加载性能
//
// 字段说明：
// - LastID: 最后一条通知的ID，用于增量拉取
// - Limit: 拉取数量限制
//
// 技术亮点：
// - 支持增量拉取，减少网络传输
// - 限制拉取数量，防止一次性加载过多数据
// - 支持多种参数绑定方式
type NotificationPullParam struct {
	LastID int64 `json:"last_id" form:"last_id"` // 最后一条通知ID，用于增量拉取
	Limit  int   `json:"limit" form:"limit"`     // 拉取数量限制
}
