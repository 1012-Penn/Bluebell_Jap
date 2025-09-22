// Package logic 业务逻辑层
//
// 负责处理业务逻辑，是系统的核心层
// 主要职责：
// 1. 处理业务规则和逻辑
// 2. 协调各个DAO层操作
// 3. 数据转换和验证
// 4. 调用外部服务（如JWT生成）
// 5. 不直接处理HTTP请求和数据库操作
package logic

import (
	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/jwt"
	"bluebell/pkg/snowflake"
)

// SignUp 用户注册业务逻辑
//
// 功能说明：
// 1. 检查用户名是否已存在
// 2. 生成全局唯一用户ID
// 3. 构造用户对象
// 4. 保存用户到数据库
//
// 参数说明：
// - p: 用户注册参数，包含用户名、密码、确认密码
//
// 返回值：
// - error: 注册过程中的错误
//
// 技术亮点：
// - 使用雪花算法生成分布式唯一ID
// - 提前检查用户名唯一性，避免无效操作
// - 分层架构，业务逻辑与数据访问分离
func SignUp(p *models.ParamSignUp) (err error) {
	// 1. 检查用户名是否已存在
	// 技术亮点：提前检查，避免无效操作，提高性能
	if err := mysql.CheckUserExist(p.Username); err != nil {
		return err
	}

	// 2. 生成全局唯一用户ID
	// 技术亮点：使用雪花算法生成分布式唯一ID
	// 优势：全局唯一、趋势递增、包含时间信息、高性能
	userID := snowflake.GenID()

	// 3. 构造用户对象
	// 技术亮点：在业务层构造数据对象，DAO层专注数据操作
	user := &models.User{
		UserID:   userID,     // 雪花算法生成的唯一ID
		Username: p.Username, // 用户名
		Password: p.Password, // 原始密码（将在DAO层加密）
	}

	// 4. 保存用户到数据库
	// 技术亮点：密码加密在DAO层处理，业务层不关心加密细节
	return mysql.InsertUser(user)
}

// Login 用户登录业务逻辑
//
// 功能说明：
// 1. 构造用户对象用于查询
// 2. 验证用户身份和密码
// 3. 生成JWT令牌
// 4. 返回用户信息和令牌
//
// 参数说明：
// - p: 用户登录参数，包含用户名和密码
//
// 返回值：
// - user: 用户信息，包含ID、用户名和JWT令牌
// - err: 登录过程中的错误
//
// 技术亮点：
// - JWT无状态认证，支持分布式部署
// - 密码验证在DAO层处理，确保安全性
// - 返回完整的用户信息，便于后续使用
func Login(p *models.ParamLogin) (user *models.User, err error) {
	// 1. 构造用户对象用于查询
	// 技术亮点：复用User结构体，减少代码重复
	user = &models.User{
		Username: p.Username,
		Password: p.Password,
	}

	// 2. 验证用户身份和密码
	// 技术亮点：传递指针，DAO层可以修改user对象，获取用户ID
	// 密码验证在DAO层进行，使用bcrypt安全比较
	if err := mysql.Login(user); err != nil {
		return nil, err
	}

	// 3. 生成JWT令牌
	// 技术亮点：使用JWT实现无状态认证
	// 优势：支持分布式部署、减少服务器存储、便于扩展
	token, err := jwt.GenToken(user.UserID, user.Username)
	if err != nil {
		return nil, err
	}

	// 4. 设置令牌到用户对象
	user.Token = token
	return user, nil
}
