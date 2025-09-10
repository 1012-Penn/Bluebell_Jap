# 📦 Models 包说明文档

## 🎯 包的作用

`models` 包是 Bluebell 项目中的数据模型层，主要负责**定义和封装数据结构**，它是整个项目架构中的基础层。

## 🏗️ 主要职责

### 1. **数据结构定义**
- 定义用户、帖子、评论等业务实体的数据结构
- 映射数据库表结构到 Go 结构体
- 提供类型安全的数据容器

### 2. **数据验证规则**
- 定义请求参数的验证规则
- 使用标签（如 `binding`、`json`、`db`）来规范数据格式
- 确保数据的完整性和有效性

### 3. **数据转换和映射**
- 在数据库字段和 Go 结构体之间建立映射关系
- 处理不同数据格式之间的转换
- 支持 JSON 序列化和反序列化

## 📁 文件结构

```
models/
├── README.md           # 包说明文档（本文件）
├── user.go            # 用户数据模型
├── post.go            # 帖子数据模型
├── community.go       # 社区数据模型
├── params.go          # 请求参数模型
├── create_table.sql   # 数据库表结构
└── struct_test.go     # 结构体测试文件
```

## 🔍 核心模型详解

### 用户模型 (`user.go`)
```go
type User struct {
    UserID   int64  `db:"user_id"`      // 数据库字段映射
    Username string `db:"username"`      // 数据库字段映射
    Password string `db:"password"`      // 数据库字段映射
    Token    string                      // 业务逻辑字段
}
```

**设计说明**:
- `db` 标签：映射数据库字段名
- `UserID` 使用 `int64` 类型，支持大数据量
- `Token` 字段用于存储 JWT 令牌，支持会话管理

### 参数模型 (`params.go`)
```go
type ParamLogin struct {
    Username string `json:"username" binding:"required"`  // JSON标签 + 验证规则
    Password string `json:"password" binding:"required"`  // JSON标签 + 验证规则
}
```

**设计说明**:
- `json` 标签：定义 JSON 序列化字段名
- `binding:"required"`：使用 Gin 验证器确保字段必填
- 结构简单，只包含登录必需的信息

### 帖子模型 (`post.go`)
```go
type Post struct {
    ID          int64     `db:"id" json:"id"`
    Title       string    `db:"title" json:"title"`
    Content     string    `db:"content" json:"content"`
    AuthorID    int64     `db:"author_id" json:"author_id"`
    CommunityID int64     `db:"community_id" json:"community_id"`
    Status      int       `db:"status" json:"status"`
    CreateTime  time.Time `db:"create_time" json:"create_time"`
    UpdateTime  time.Time `db:"update_time" json:"update_time"`
}
```

**设计说明**:
- 包含完整的 CRUD 操作所需字段
- 使用 `time.Time` 类型处理时间
- 支持软删除和状态管理

## 🏗️ 在架构中的位置

```
┌─────────────────────────────────────┐
│           表现层 (Controller)        │
├─────────────────────────────────────┤
│           业务层 (Logic)             │
├─────────────────────────────────────┤
│           数据层 (DAO)               │
├─────────────────────────────────────┤
│         Models 包 (数据模型)         │ ← 这里
└─────────────────────────────────────┘
```

## 💡 设计优势

### 1. **类型安全**
- 使用 Go 的强类型系统确保数据安全
- 编译时就能发现类型错误

### 2. **代码复用**
- 同一数据结构在多个层中被使用
- 避免重复定义，提高代码一致性

### 3. **维护性**
- 数据结构变更只需在一个地方修改
- 降低代码耦合度

### 4. **可读性**
- 清晰的数据结构定义
- 标签提供了丰富的元数据信息

## 🔄 与其他层的交互

- **Controller 层**: 使用 models 接收和验证请求参数
- **Logic 层**: 使用 models 进行业务逻辑处理
- **DAO 层**: 使用 models 进行数据库操作
- **Response**: 使用 models 构建响应数据

## 🏷️ 标签使用规范

### 1. **数据库标签 (`db`)**
```go
type User struct {
    UserID int64 `db:"user_id"`  // 映射数据库字段名
}
```

### 2. **JSON 标签 (`json`)**
```go
type Response struct {
    Code int         `json:"code"`    // 响应状态码
    Msg  string      `json:"msg"`     // 响应消息
    Data interface{} `json:"data"`    // 响应数据
}
```

### 3. **验证标签 (`binding`)**
```go
type Param struct {
    Username string `binding:"required"`           // 必填字段
    Age      int    `binding:"gte=0,lte=150"`     // 数值范围验证
    Email    string `binding:"required,email"`     // 必填 + 邮箱格式验证
}
```

## 📊 数据验证规则

### 常用验证器
- `required`: 字段必填
- `email`: 邮箱格式验证
- `url`: URL 格式验证
- `min`: 最小长度/值
- `max`: 最大长度/值
- `gte`: 大于等于
- `lte`: 小于等于

### 自定义验证器
```go
// 可以注册自定义验证器
if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
    v.RegisterValidation("custom_rule", customValidator)
}
```

## 🔒 安全考虑

### 1. **数据脱敏**
- 密码字段不参与 JSON 序列化
- 敏感信息在响应中过滤

### 2. **输入验证**
- 严格的参数验证规则
- 防止恶意数据注入

### 3. **类型安全**
- 强类型系统防止类型错误
- 编译时错误检查

## 📝 最佳实践

### 1. **命名规范**
- 结构体名使用 PascalCase
- 字段名使用 PascalCase
- 标签值使用 snake_case

### 2. **文档注释**
- 为每个结构体添加注释
- 说明字段的用途和约束

### 3. **版本兼容**
- 向后兼容的字段变更
- 废弃字段的标记和处理

## 🚀 扩展建议

### 1. **缓存支持**
```go
type Cacheable interface {
    CacheKey() string
    CacheTTL() time.Duration
}
```

### 2. **事件支持**
```go
type EventEmitter interface {
    Emit(event string, data interface{})
    On(event string, handler func(interface{}))
}
```

### 3. **验证扩展**
```go
type Validator interface {
    Validate() error
    ValidateField(field string) error
}
```

## 📝 总结

`models` 包是 Bluebell 项目的**数据契约层**，它定义了系统中所有数据的结构和规则，为上层业务逻辑提供了统一的数据接口。通过良好的模型设计，确保了数据的类型安全、验证规则和映射关系，是整个项目架构稳定性的重要保障。

### 核心价值
1. **数据一致性**: 统一的数据结构定义
2. **类型安全**: 编译时错误检查
3. **可维护性**: 集中管理数据结构
4. **可扩展性**: 支持新功能的无缝集成

---

*最后更新: 2024年*
*维护者: Bluebell 开发团队*
