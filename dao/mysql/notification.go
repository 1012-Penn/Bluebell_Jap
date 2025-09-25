package mysql

import (
	"database/sql"
	"time"

	"bluebell/models"
)

// InsertNotification 将通知事件写入数据库。
func InsertNotification(event *models.NotificationEvent) error {
	if event == nil {
		return nil
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	sqlStr := `INSERT INTO bluebell_notification (id, user_id, from_user_id, post_id, comment_id, type, content, create_time)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(sqlStr, event.ID, event.ReceiverID, event.ActorID, event.PostID, event.CommentID, event.Type, event.Message, event.CreatedAt)
	return err
}

// ListNotificationsAfter 获取指定 ID 之后的通知，避免深度分页。
func ListNotificationsAfter(userID, lastID int64, limit int) ([]*models.NotificationEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	sqlStr := `SELECT id, user_id, from_user_id, post_id, comment_id, type, content, create_time
FROM bluebell_notification
WHERE user_id = ? AND id > ?
ORDER BY id ASC
LIMIT ?`
	rows, err := db.Query(sqlStr, userID, lastID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*models.NotificationEvent, 0, limit)
	for rows.Next() {
		item := new(models.NotificationEvent)
		if err := rows.Scan(&item.ID, &item.ReceiverID, &item.ActorID, &item.PostID, &item.CommentID, &item.Type, &item.Message, &item.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

// ListLatestNotifications 首次拉取时获取固定数量的数据。
func ListLatestNotifications(userID int64, limit int) ([]*models.NotificationEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	sqlStr := `SELECT id, user_id, from_user_id, post_id, comment_id, type, content, create_time
FROM bluebell_notification
WHERE user_id = ?
ORDER BY id DESC
LIMIT ?`
	rows, err := db.Query(sqlStr, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*models.NotificationEvent, 0, limit)
	for rows.Next() {
		item := new(models.NotificationEvent)
		if err := rows.Scan(&item.ID, &item.ReceiverID, &item.ActorID, &item.PostID, &item.CommentID, &item.Type, &item.Message, &item.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	// 逆序返回，保持时间顺序
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	return list, rows.Err()
}

// GetLastNotificationID 查询某个用户最新的通知 ID。
func GetLastNotificationID(userID int64) (int64, error) {
	sqlStr := `SELECT id FROM bluebell_notification WHERE user_id = ? ORDER BY id DESC LIMIT 1`
	var id sql.NullInt64
	err := db.QueryRow(sqlStr, userID).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	if id.Valid {
		return id.Int64, nil
	}
	return 0, nil
}

// BatchInsertNotifications 批量插入通知事件
func BatchInsertNotifications(events []*models.NotificationEvent) error {
	if len(events) == 0 {
		return nil
	}
	
	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	// 准备批量插入语句
	sqlStr := `INSERT INTO bluebell_notification (id, user_id, from_user_id, post_id, comment_id, type, content, create_time)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	
	stmt, err := tx.Prepare(sqlStr)
	if err != nil {
		return err
	}
	defer stmt.Close()
	
	// 批量插入
	for _, event := range events {
		if event == nil {
			continue
		}
		
		// 设置默认创建时间
		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now()
		}
		
		_, err := stmt.Exec(event.ID, event.ReceiverID, event.ActorID, event.PostID, event.CommentID, event.Type, event.Message, event.CreatedAt)
		if err != nil {
			return err
		}
	}
	
	// 提交事务
	return tx.Commit()
}

// GetUnreadNotificationCount 获取用户未读通知数量
func GetUnreadNotificationCount(userID int64) (int, error) {
	sqlStr := `SELECT COUNT(*) FROM bluebell_notification WHERE user_id = ? AND is_read = false`
	var count int
	err := db.QueryRow(sqlStr, userID).Scan(&count)
	return count, err
}

// GetTotalNotificationCount 获取用户总通知数量
func GetTotalNotificationCount(userID int64) (int, error) {
	sqlStr := `SELECT COUNT(*) FROM bluebell_notification WHERE user_id = ?`
	var count int
	err := db.QueryRow(sqlStr, userID).Scan(&count)
	return count, err
}

// MarkNotificationAsRead 标记通知为已读
func MarkNotificationAsRead(userID, notificationID int64) error {
	sqlStr := `UPDATE bluebell_notification SET is_read = true, read_time = NOW() WHERE user_id = ? AND id = ?`
	_, err := db.Exec(sqlStr, userID, notificationID)
	return err
}

// MarkAllNotificationsAsRead 标记所有通知为已读
func MarkAllNotificationsAsRead(userID int64) error {
	sqlStr := `UPDATE bluebell_notification SET is_read = true, read_time = NOW() WHERE user_id = ? AND is_read = false`
	_, err := db.Exec(sqlStr, userID)
	return err
}

// DeleteNotification 删除通知
func DeleteNotification(userID, notificationID int64) error {
	sqlStr := `UPDATE bluebell_notification SET is_deleted = true WHERE user_id = ? AND id = ?`
	_, err := db.Exec(sqlStr, userID, notificationID)
	return err
}

// ClearUserNotifications 清空用户所有通知
func ClearUserNotifications(userID int64) error {
	sqlStr := `UPDATE bluebell_notification SET is_deleted = true WHERE user_id = ?`
	_, err := db.Exec(sqlStr, userID)
	return err
}
