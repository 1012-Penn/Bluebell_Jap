package models

// User 用户数据模型
type User struct {
	UserID   int64  `db:"user_id"`  // 用户ID，对应数据库字段
	Username string `db:"username"` // 用户名，对应数据库字段
	Password string `db:"password"` // 密码，对应数据库字段（加密存储）
	Token    string // JWT令牌，不存储到数据库
}
