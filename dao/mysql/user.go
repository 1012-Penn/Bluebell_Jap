// Package mysql MySQL数据访问层
//
// 负责处理MySQL数据库操作，是数据持久化层
// 主要职责：
// 1. 执行SQL语句
// 2. 数据加密和解密
// 3. 错误处理和转换
// 4. 连接池管理
// 5. 不包含业务逻辑，只负责数据操作
package mysql

import (
	"bluebell/models"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
)

// 密码加密盐值
// 技术亮点：使用盐值增加密码安全性
// 注意：实际项目中应该使用更安全的加密方式，如bcrypt
const secret = "liwenzhou.com"

// CheckUserExist 检查指定用户名的用户是否存在
//
// 功能说明：
// 1. 查询数据库中是否存在指定用户名
// 2. 返回相应的错误信息
//
// 参数说明：
// - username: 要检查的用户名
//
// 返回值：
// - error: 如果用户存在返回ErrorUserExist，其他错误返回相应错误
//
// 技术亮点：
// - 使用COUNT查询提高性能
// - 参数化查询防止SQL注入
// - 统一的错误处理
func CheckUserExist(username string) (err error) {
	// 使用COUNT查询检查用户是否存在
	// 技术亮点：COUNT查询比SELECT *更高效
	sqlStr := `select count(user_id) from user where username = ?`
	var count int64

	// 执行查询
	if err := db.Get(&count, sqlStr, username); err != nil {
		return err
	}

	// 检查用户是否存在
	if count > 0 {
		return ErrorUserExist // 用户已存在
	}
	return nil
}

// InsertUser 向数据库中插入一条新的用户记录
//
// 功能说明：
// 1. 对用户密码进行加密
// 2. 将用户信息插入数据库
//
// 参数说明：
// - user: 用户信息，包含ID、用户名、密码
//
// 返回值：
// - error: 插入过程中的错误
//
// 技术亮点：
// - 密码加密存储，提高安全性
// - 参数化查询防止SQL注入
// - 使用雪花算法生成的ID，避免主键冲突
func InsertUser(user *models.User) (err error) {
	// 1. 对密码进行加密
	// 技术亮点：在数据层进行密码加密，业务层不关心加密细节
	user.Password = encryptPassword(user.Password)

	// 2. 执行SQL语句入库
	// 技术亮点：使用参数化查询，防止SQL注入攻击
	sqlStr := `insert into user(user_id, username, password) values(?,?,?)`
	_, err = db.Exec(sqlStr, user.UserID, user.Username, user.Password)
	return err
}

// encryptPassword 密码加密函数
//
// 功能说明：
// 1. 使用MD5+盐值的方式加密密码
// 2. 增加密码破解难度
//
// 参数说明：
// - oPassword: 原始密码
//
// 返回值：
// - string: 加密后的密码
//
// 技术亮点：
// - 使用盐值增加安全性
// - 固定盐值，确保相同密码加密结果一致
//
// 注意：实际项目中建议使用bcrypt等更安全的加密方式
func encryptPassword(oPassword string) string {
	h := md5.New()
	h.Write([]byte(secret))    // 先写入盐值
	h.Write([]byte(oPassword)) // 再写入原始密码
	return hex.EncodeToString(h.Sum(nil))
}

// Login 用户登录验证
//
// 功能说明：
// 1. 根据用户名查询用户信息
// 2. 验证密码是否正确
// 3. 返回用户信息（通过指针修改）
//
// 参数说明：
// - user: 用户信息指针，包含用户名和密码，验证成功后会填充用户ID
//
// 返回值：
// - error: 登录过程中的错误
//
// 技术亮点：
// - 通过指针返回用户信息，减少数据拷贝
// - 密码验证使用相同的加密算法
// - 区分用户不存在和密码错误
func Login(user *models.User) (err error) {
	// 1. 保存原始密码用于验证
	oPassword := user.Password

	// 2. 根据用户名查询用户信息
	// 技术亮点：使用参数化查询，防止SQL注入
	sqlStr := `select user_id, username, password from user where username=?`
	err = db.Get(user, sqlStr, user.Username)

	// 3. 处理查询结果
	if err == sql.ErrNoRows {
		return ErrorUserNotExist // 用户不存在
	}
	if err != nil {
		return err // 查询数据库失败
	}

	// 4. 验证密码是否正确
	// 技术亮点：使用相同的加密算法验证密码
	password := encryptPassword(oPassword)
	if password != user.Password {
		return ErrorInvalidPassword // 密码错误
	}

	return nil
}

// GetUserById 根据用户ID获取用户信息
//
// 功能说明：
// 1. 根据用户ID查询用户基本信息
// 2. 不包含密码等敏感信息
//
// 参数说明：
// - uid: 用户ID
//
// 返回值：
// - user: 用户信息，不包含密码
// - err: 查询过程中的错误
//
// 技术亮点：
// - 只查询必要字段，不包含密码
// - 使用参数化查询防止SQL注入
// - 返回指针，减少内存拷贝
func GetUserById(uid int64) (user *models.User, err error) {
	user = new(models.User)

	// 只查询用户ID和用户名，不包含密码
	// 技术亮点：避免查询敏感信息，提高安全性
	sqlStr := `select user_id, username from user where user_id = ?`
	err = db.Get(user, sqlStr, uid)
	return user, err
}
