package kafka

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// Manager Kafka管理器
type Manager struct {
	client   *Client
	producer *Producer
	consumer *Consumer
	mu       sync.RWMutex
	closed   bool
}

// GlobalManager 全局Kafka管理器
var GlobalManager *Manager

// InitKafkaManager 初始化Kafka管理器
func InitKafkaManager(config *Config) error {
	// 创建客户端
	client, err := NewClient(config)
	if err != nil {
		return fmt.Errorf("创建Kafka客户端失败: %w", err)
	}

	// 创建生产者
	producer := NewProducer(client, "bluebell-notifications")

	// 创建管理器
	manager := &Manager{
		client:   client,
		producer: producer,
	}

	GlobalManager = manager

	zap.L().Info("Kafka管理器初始化成功")
	return nil
}

// GetProducer 获取生产者
func (m *Manager) GetProducer() *Producer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.producer
}

// GetConsumer 获取消费者
func (m *Manager) GetConsumer() *Consumer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.consumer
}

// StartConsumer 启动消费者
func (m *Manager) StartConsumer(handler MessageHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("管理器已关闭")
	}

	if m.consumer != nil {
		return fmt.Errorf("消费者已在运行")
	}

	// 创建消费者
	m.consumer = NewConsumer(m.client, "bluebell-notifications", handler)

	// 启动消费者
	if err := m.consumer.Start(); err != nil {
		return fmt.Errorf("启动消费者失败: %w", err)
	}

	zap.L().Info("Kafka消费者已启动")
	return nil
}

// StopConsumer 停止消费者
func (m *Manager) StopConsumer() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.consumer == nil {
		return nil
	}

	if err := m.consumer.Stop(); err != nil {
		return fmt.Errorf("停止消费者失败: %w", err)
	}

	m.consumer = nil
	zap.L().Info("Kafka消费者已停止")
	return nil
}

// Close 关闭管理器
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	// 停止消费者
	if m.consumer != nil {
		if err := m.consumer.Stop(); err != nil {
			zap.L().Error("停止消费者失败", zap.Error(err))
		}
		m.consumer = nil
	}

	// 关闭客户端
	if m.client != nil {
		if err := m.client.Close(); err != nil {
			zap.L().Error("关闭客户端失败", zap.Error(err))
		}
	}

	zap.L().Info("Kafka管理器已关闭")
	return nil
}

// IsClosed 检查管理器是否已关闭
func (m *Manager) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// HealthCheck 健康检查
func (m *Manager) HealthCheck() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("管理器已关闭")
	}

	if m.client == nil {
		return fmt.Errorf("客户端未初始化")
	}

	return m.client.HealthCheck()
}

// GetStats 获取统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]interface{}{
		"manager_status": "running",
		"closed":         m.closed,
		"producer_ready": m.producer != nil,
		"consumer_ready": m.consumer != nil,
	}

	if m.consumer != nil {
		stats["consumer_running"] = m.consumer.IsRunning()
		stats["topic"] = m.consumer.GetTopic()
		stats["group_id"] = m.consumer.GetGroupID()
	}

	if m.client != nil {
		if err := m.client.HealthCheck(); err != nil {
			stats["client_health"] = err.Error()
		} else {
			stats["client_health"] = "healthy"
		}
	}

	return stats
}

// GetGlobalManager 获取全局管理器
func GetGlobalManager() *Manager {
	return GlobalManager
}
