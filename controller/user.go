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
	"bluebell/dao/mysql"
	"bluebell/logic"
	"bluebell/models"
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"

	"github.com/gin-gonic/gin"
)

// SignUpHandler 用户注册处理器
//
// 功能说明：
// 1. 接收用户注册请求
// 2. 验证请求参数（用户名、密码、确认密码）
// 3. 调用业务逻辑层处理注册
// 4. 返回注册结果
//
// 请求方式：POST /api/v1/signup
// 请求参数：{"username": "testuser", "password": "123456", "confirm_password": "123456"}
// 响应格式：{"code": 1000, "msg": "success", "data": null}
//
// 技术亮点：
// - 使用validator进行参数验证
// - 统一的错误处理和响应格式
// - 结构化日志记录
func SignUpHandler(c *gin.Context) {
	// 1. 获取参数和参数校验
	// 技术亮点：使用ShouldBindJSON自动绑定JSON请求体到结构体
	// 同时触发validator标签验证，减少手动验证代码
	p := new(models.ParamSignUp)
	if err := c.ShouldBindJSON(p); err != nil {
		// 记录错误日志，便于调试和监控
		zap.L().Error("SignUp with invalid param", zap.Error(err))

		// 类型断言，判断是否为验证错误
		// 技术亮点：区分不同类型的错误，提供更精确的错误信息
		errs, ok := err.(validator.ValidationErrors)
		if !ok {
			// 非验证错误，返回通用参数错误
			ResponseError(c, CodeInvalidParam)
			return
		}
		// 验证错误，返回具体的字段错误信息
		// 技术亮点：翻译验证错误信息，提供用户友好的错误提示
		ResponseErrorWithMsg(c, CodeInvalidParam, removeTopStruct(errs.Translate(trans)))
		return
	}

	// 2. 调用业务逻辑层处理注册
	// 技术亮点：分层架构，Controller层只负责请求处理，不包含业务逻辑
	if err := logic.SignUp(p); err != nil {
		zap.L().Error("logic.SignUp failed", zap.Error(err))

		// 根据具体错误类型返回不同的错误码
		// 技术亮点：细粒度的错误处理，提高用户体验
		if errors.Is(err, mysql.ErrorUserExist) {
			ResponseError(c, CodeUserExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}

	// 3. 返回成功响应
	// 技术亮点：统一的响应格式，便于前端处理
	ResponseSuccess(c, nil)
}

// LoginHandler 用户登录处理器
//
// 功能说明：
// 1. 接收用户登录请求
// 2. 验证请求参数（用户名、密码）
// 3. 调用业务逻辑层验证用户身份
// 4. 生成JWT令牌
// 5. 返回用户信息和令牌
//
// 请求方式：POST /api/v1/login
// 请求参数：{"username": "testuser", "password": "123456"}
// 响应格式：{"code": 1000, "msg": "success", "data": {"user_id": "123", "user_name": "testuser", "token": "..."}}
//
// 技术亮点：
// - JWT无状态认证
// - 密码安全验证
// - 用户ID转换为字符串避免精度丢失
func LoginHandler(c *gin.Context) {
	// 1. 获取请求参数及参数校验
	// 技术亮点：复用参数验证逻辑，保持代码一致性
	p := new(models.ParamLogin)
	if err := c.ShouldBindJSON(p); err != nil {
		// 记录错误日志
		zap.L().Error("Login with invalid param", zap.Error(err))

		// 处理验证错误
		errs, ok := err.(validator.ValidationErrors)
		if !ok {
			ResponseError(c, CodeInvalidParam)
			return
		}
		// 技术亮点：翻译验证错误信息，提供用户友好的错误提示
		ResponseErrorWithMsg(c, CodeInvalidParam, removeTopStruct(errs.Translate(trans)))
		return
	}

	// 2. 调用业务逻辑层处理登录
	// 技术亮点：业务逻辑层处理密码验证和JWT生成
	user, err := logic.Login(p)
	if err != nil {
		// 记录登录失败日志，包含用户名便于安全审计
		zap.L().Error("logic.Login failed", zap.String("username", p.Username), zap.Error(err))

		// 根据错误类型返回不同错误码
		// 技术亮点：区分用户名不存在和密码错误，提高安全性
		if errors.Is(err, mysql.ErrorUserNotExist) {
			ResponseError(c, CodeUserNotExist)
			return
		}
		ResponseError(c, CodeInvalidPassword)
		return
	}

	// 3. 返回登录成功响应
	// 技术亮点：用户ID转换为字符串，避免前端精度丢失问题
	// JavaScript中Number类型最大安全整数是2^53-1，超过此值会丢失精度
	ResponseSuccess(c, gin.H{
		"user_id":   fmt.Sprintf("%d", user.UserID), // 转换为字符串避免精度丢失
		"user_name": user.Username,                  // 用户名
		"token":     user.Token,                     // JWT令牌，用于后续请求认证
	})
}
