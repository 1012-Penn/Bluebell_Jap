package models

import "time"

// 定义请求的参数结构体

const (
	OrderTime  = "time"
	OrderScore = "score"
)

// ParamSignUp 注册请求参数
type ParamSignUp struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	RePassword string `json:"confirm_password" binding:"required,eqfield=Password"`
}

// ParamLogin 登录请求参数
type ParamLogin struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// ParamVoteData 投票数据
type ParamVoteData struct {
	// UserID 从请求中获取当前的用户
	PostID    string `json:"post_id" binding:"required"`               // 贴子id
	Direction int8   `json:"direction,string" binding:"oneof=1 0 -1" ` // 赞成票(1)还是反对票(-1)取消投票(0)
	// 使用int8是因为int8的取值范围是-128到127,而int的取值范围是-2147483648到2147483647,所以使用int8可以节省内存
}

// ParamPostList 获取帖子列表query string参数
type ParamPostList struct {
	CommunityID int64  `json:"community_id" form:"community_id"`   // 可以为空
	Page        int64  `json:"page" form:"page" example:"1"`       // 页码
	Size        int64  `json:"size" form:"size" example:"10"`      // 每页数据量
	Order       string `json:"order" form:"order" example:"score"` // 排序依据
}

// NotificationEvent 消息通知事件在 MQ 中的载体。
type NotificationEvent struct {
	ID         int64     `json:"id"`
	ReceiverID int64     `json:"receiver_id"`
	ActorID    int64     `json:"actor_id"`
	PostID     int64     `json:"post_id"`
	CommentID  int64     `json:"comment_id"`
	Type       string    `json:"type"` // like/comment
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"created_at"`
}

// NotificationPullParam 前端拉取通知时的查询参数。
type NotificationPullParam struct {
	LastID int64 `json:"last_id" form:"last_id"`
	Limit  int   `json:"limit" form:"limit"`
}
