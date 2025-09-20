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
	sqlStr := `INSERT INTO user_notification (id, receiver_id, actor_id, post_id, comment_id, type, message, create_time)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(sqlStr, event.ID, event.ReceiverID, event.ActorID, event.PostID, event.CommentID, event.Type, event.Message, event.CreatedAt)
	return err
}

// ListNotificationsAfter 获取指定 ID 之后的通知，避免深度分页。
func ListNotificationsAfter(userID, lastID int64, limit int) ([]*models.NotificationEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	sqlStr := `SELECT id, receiver_id, actor_id, post_id, comment_id, type, message, create_time
FROM user_notification
WHERE receiver_id = ? AND id > ?
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
	sqlStr := `SELECT id, receiver_id, actor_id, post_id, comment_id, type, message, create_time
FROM user_notification
WHERE receiver_id = ?
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
	sqlStr := `SELECT id FROM user_notification WHERE receiver_id = ? ORDER BY id DESC LIMIT 1`
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
