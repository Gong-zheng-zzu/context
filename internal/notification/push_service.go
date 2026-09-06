package notification

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/bigdata"
	"github.com/gorilla/websocket"
)

// NotificationType 通知类型
type NotificationType string

const (
	NotificationAlert          NotificationType = "alert"          // 告警
	NotificationRecommendation NotificationType = "recommendation" // 推荐
	NotificationMessage        NotificationType = "message"        // 消息
	NotificationCall           NotificationType = "call"           // 呼叫
)

// Notification 通知消息
type Notification struct {
	ID        string                 `json:"id"`
	Type      NotificationType       `json:"type"`
	Title     string                 `json:"title"`
	Content   string                 `json:"content"`
	Level     string                 `json:"level"`     // info/warning/critical
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	Read      bool                   `json:"read"`
}

// Client WebSocket客户端
type Client struct {
	UserID string
	Role   string
	Conn   *websocket.Conn
	Send   chan []byte
}

// PushService 推送服务
type PushService struct {
	clients    map[string][]*Client // key: userID, value: 客户端列表
	register   chan *Client
	unregister chan *Client
	broadcast  chan *BroadcastMessage
	mutex      sync.RWMutex
}

// BroadcastMessage 广播消息
type BroadcastMessage struct {
	UserID       string
	Role         string
	Notification *Notification
}

// NewPushService 创建推送服务
func NewPushService() *PushService {
	return &PushService{
		clients:    make(map[string][]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *BroadcastMessage, 256),
	}
}

// Run 运行推送服务
func (s *PushService) Run() {
	for {
		select {
		case client := <-s.register:
			s.mutex.Lock()
			s.clients[client.UserID] = append(s.clients[client.UserID], client)
			s.mutex.Unlock()
			log.Printf("[推送服务] 客户端连接: user_id=%s, role=%s", client.UserID, client.Role)

		case client := <-s.unregister:
			s.mutex.Lock()
			if clients, ok := s.clients[client.UserID]; ok {
				// 从列表中移除该客户端
				for i, c := range clients {
					if c == client {
						s.clients[client.UserID] = append(clients[:i], clients[i+1:]...)
						close(client.Send)
						break
					}
				}
				// 如果该用户没有客户端了，删除key
				if len(s.clients[client.UserID]) == 0 {
					delete(s.clients, client.UserID)
				}
			}
			s.mutex.Unlock()
			log.Printf("[推送服务] 客户端断开: user_id=%s, role=%s", client.UserID, client.Role)

		case message := <-s.broadcast:
			s.mutex.RLock()
			clients := s.clients[message.UserID]
			s.mutex.RUnlock()

			// 序列化通知
			data, err := json.Marshal(message.Notification)
			if err != nil {
				log.Printf("[推送服务] 序列化失败: %v", err)
				continue
			}

			// 发送给该用户的所有客户端
			for _, client := range clients {
				// 检查角色是否匹配
				if message.Role == "" || client.Role == message.Role {
					select {
					case client.Send <- data:
					default:
						// 发送失败，关闭客户端
						close(client.Send)
						s.mutex.Lock()
						if clients, ok := s.clients[client.UserID]; ok {
							for i, c := range clients {
								if c == client {
									s.clients[client.UserID] = append(clients[:i], clients[i+1:]...)
									break
								}
							}
						}
						s.mutex.Unlock()
					}
				}
			}
		}
	}
}

// RegisterClient 注册客户端
func (s *PushService) RegisterClient(client *Client) {
	s.register <- client
}

// UnregisterClient 注销客户端
func (s *PushService) UnregisterClient(client *Client) {
	s.unregister <- client
}

// PushNotification 推送通知
func (s *PushService) PushNotification(userID, role string, notification *Notification) {
	s.broadcast <- &BroadcastMessage{
		UserID:       userID,
		Role:         role,
		Notification: notification,
	}
}

// PushAlert 推送告警
func (s *PushService) PushAlert(userID, role string, alert *bigdata.Alert) {
	notification := &Notification{
		ID:        alert.ID,
		Type:      NotificationAlert,
		Title:     alert.Title,
		Content:   alert.Message,
		Level:     string(alert.Level),
		Timestamp: alert.Timestamp,
		Data: map[string]interface{}{
			"elder_id":   alert.ElderID,
			"elder_name": alert.ElderName,
			"alert_type": alert.Type,
		},
		Read: false,
	}
	s.PushNotification(userID, role, notification)
}

// PushRecommendation 推送推荐
func (s *PushService) PushRecommendation(userID, role string, recommendation *bigdata.Recommendation) {
	level := "info"
	if recommendation.Priority >= 4 {
		level = "warning"
	}

	notification := &Notification{
		ID:        recommendation.ID,
		Type:      NotificationRecommendation,
		Title:     recommendation.Title,
		Content:   recommendation.Content,
		Level:     level,
		Timestamp: recommendation.Timestamp,
		Data: map[string]interface{}{
			"elder_id":   recommendation.ElderID,
			"elder_name": recommendation.ElderName,
			"rec_type":   recommendation.Type,
			"priority":   recommendation.Priority,
		},
		Read: false,
	}
	s.PushNotification(userID, role, notification)
}

// PushMessage 推送消息
func (s *PushService) PushMessage(userID, role, title, content string) {
	notification := &Notification{
		ID:        generateID(),
		Type:      NotificationMessage,
		Title:     title,
		Content:   content,
		Level:     "info",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
		Read:      false,
	}
	s.PushNotification(userID, role, notification)
}

// PushCall 推送呼叫通知
func (s *PushService) PushCall(userID, role, elderID, elderName, reason string) {
	notification := &Notification{
		ID:        generateID(),
		Type:      NotificationCall,
		Title:     "呼叫通知",
		Content:   elderName + "正在呼叫",
		Level:     "warning",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"elder_id":   elderID,
			"elder_name": elderName,
			"reason":     reason,
		},
		Read: false,
	}
	s.PushNotification(userID, role, notification)
}

// GetConnectedClients 获取在线客户端数量
func (s *PushService) GetConnectedClients() map[string]int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	result := make(map[string]int)
	for userID, clients := range s.clients {
		result[userID] = len(clients)
	}
	return result
}

// WritePump 写入泵（处理客户端发送）
func (c *Client) WritePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// 通道关闭
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 批量发送队列中的其他消息
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ReadPump 读取泵（处理客户端接收）
func (c *Client) ReadPump(service *PushService) {
	defer func() {
		service.UnregisterClient(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[推送服务] WebSocket错误: %v", err)
			}
			break
		}
	}
}

// generateID 生成唯一ID
func generateID() string {
	return time.Now().Format("20060102150405") + "_" + randomString(6)
}

// randomString 生成随机字符串
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}

// EmailService 邮件服务（占位符）
type EmailService struct{}

// SendEmail 发送邮件
func (s *EmailService) SendEmail(to, subject, body string) error {
	// TODO: 实现邮件发送
	log.Printf("[邮件服务] 发送邮件: to=%s, subject=%s", to, subject)
	return nil
}

// SMSService 短信服务（占位符）
type SMSService struct{}

// SendSMS 发送短信
func (s *SMSService) SendSMS(phone, message string) error {
	// TODO: 实现短信发送
	log.Printf("[短信服务] 发送短信: phone=%s, message=%s", phone, message)
	return nil
}
