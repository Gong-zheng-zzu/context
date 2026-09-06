package services

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/models"
	"github.com/gorilla/websocket"
)

// WebSocket连接管理器
type WebSocketManager struct {
	connections         map[string][]*websocket.Conn          // 🔥 修改：connectionID -> []WebSocket连接（支持多端共享）
	userToConnections   map[string][]string                   // userID -> []connectionID (支持一个用户多个连接)
	sessionToConnection map[string]string                     // sessionID -> connectionID (精确定向推送)
	callbacks           map[string]chan models.CallbackResult // callbackID -> 结果通道
	mutex               sync.RWMutex
}

// 全局WebSocket管理器实例
var GlobalWSManager = &WebSocketManager{
	connections:         make(map[string][]*websocket.Conn), // 🔥 修改：支持多个连接
	userToConnections:   make(map[string][]string),
	sessionToConnection: make(map[string]string),
	callbacks:           make(map[string]chan models.CallbackResult),
}

// 用户连接注册 - 支持工作空间级别的连接隔离，支持多端共享
func (wsm *WebSocketManager) RegisterUser(connectionID string, conn *websocket.Conn) {
	wsm.mutex.Lock()
	defer wsm.mutex.Unlock()

	log.Printf("🔗 [连接注册] ===== 开始注册用户连接 =====")
	log.Printf("🔗 [连接注册] 输入参数: connectionID=%s", connectionID)

	// 提取用户ID
	userID := wsm.extractUserIDFromConnectionID(connectionID)
	log.Printf("🔗 [连接注册] 从连接ID提取用户ID: %s", userID)

	// 🔥 核心改动：追加连接到数组，不是替换
	if existingConns, exists := wsm.connections[connectionID]; exists {
		// 检查是否重复添加同一个连接
		for _, existingConn := range existingConns {
			if existingConn == conn {
				log.Printf("🔗 [连接注册] ⚠️ 连接已存在，跳过重复注册")
				return
			}
		}

		// 追加新连接到数组
		wsm.connections[connectionID] = append(existingConns, conn)
		log.Printf("🔗 [连接注册] ➕ 追加连接: %s (用户: %s, 当前共 %d 个连接)",
			connectionID, userID, len(wsm.connections[connectionID]))
	} else {
		// 首次连接，创建新数组
		wsm.connections[connectionID] = []*websocket.Conn{conn}
		log.Printf("🔗 [连接注册] 🆕 首次连接: %s (用户: %s)", connectionID, userID)
	}

	// 更新用户到连接的映射
	if wsm.userToConnections[userID] == nil {
		wsm.userToConnections[userID] = []string{}
		log.Printf("🔗 [连接注册] 🆕 为用户 %s 创建新的连接映射", userID)
	}

	// 检查是否已经存在这个连接ID
	found := false
	for _, cid := range wsm.userToConnections[userID] {
		if cid == connectionID {
			found = true
			break
		}
	}

	if !found {
		wsm.userToConnections[userID] = append(wsm.userToConnections[userID], connectionID)
		log.Printf("🔗 [连接注册] ✅ 添加连接到用户映射: %s → %s", userID, connectionID)
	}

	// 统计总连接数
	totalConns := 0
	for _, conns := range wsm.connections {
		totalConns += len(conns)
	}

	log.Printf("🔗 [连接注册] ✅ 连接 %s 已注册 (用户 %s, 该工作空间 %d 个连接, 系统总连接数: %d)",
		connectionID, userID, len(wsm.connections[connectionID]), totalConns)
	log.Printf("🔗 [连接注册] ===== 用户连接注册完成，启动连接监听 =====")

	// 🔥 关键：为每个连接启动独立的监听goroutine
	go wsm.handleConnection(connectionID, conn)
}

// 🔥 简化：从连接ID中提取用户ID
func (wsm *WebSocketManager) extractUserIDFromConnectionID(connectionID string) string {
	// 🔥 新逻辑：支持两种格式
	// 格式1: userId (简单用户ID)
	// 格式2: userId_ws_workspaceHash (带工作空间的连接ID)
	parts := strings.Split(connectionID, "_ws_")
	if len(parts) >= 2 {
		return parts[0] // 返回用户ID部分
	}
	// 如果不是工作空间连接ID格式，直接返回原值（就是用户ID）
	return connectionID
}

// 🔥 导出：公开方法供外部调用
func (wsm *WebSocketManager) ExtractUserIDFromConnectionID(connectionID string) string {
	return wsm.extractUserIDFromConnectionID(connectionID)
}

// 🔥 保留但简化：从连接ID中提取工作空间哈希（向后兼容）
func (wsm *WebSocketManager) extractWorkspaceHashFromConnectionID(connectionID string) string {
	// connectionID格式: userId_ws_workspaceHash
	// 例如: user_1703123456_ws_a1b2c3d4
	parts := strings.Split(connectionID, "_ws_")
	if len(parts) >= 2 {
		return parts[1]
	}
	// 如果不是工作空间连接ID格式，返回空字符串
	return ""
}

// 🔥 导出：公开方法供外部调用（向后兼容）
func (wsm *WebSocketManager) ExtractWorkspaceHashFromConnectionID(connectionID string) string {
	return wsm.extractWorkspaceHashFromConnectionID(connectionID)
}

// 连接注销 - 支持工作空间级别的连接管理，精确移除单个连接
// 🔥 重要：增加conn参数，精确定位要移除的连接
func (wsm *WebSocketManager) UnregisterUser(connectionID string, conn *websocket.Conn) {
	wsm.mutex.Lock()
	defer wsm.mutex.Unlock()

	// 提取用户ID
	userID := wsm.extractUserIDFromConnectionID(connectionID)

	log.Printf("🔗 [连接注销] 开始注销: connectionID=%s, userID=%s", connectionID, userID)

	if existingConns, exists := wsm.connections[connectionID]; exists {
		// 🔥 核心逻辑：从数组中移除特定连接（通过指针比较）
		newConns := []*websocket.Conn{}
		removed := false

		for _, existingConn := range existingConns {
			if existingConn == conn {
				// 找到要移除的连接，不添加到newConns
				removed = true
				log.Printf("🔗 [连接注销] 🎯 找到要移除的连接")
			} else {
				// 保留其他连接
				newConns = append(newConns, existingConn)
			}
		}

		if !removed {
			log.Printf("🔗 [连接注销] ⚠️ 连接不存在于数组中: %s", connectionID)
			return
		}

		// 🔥 判断：是否还有其他连接
		if len(newConns) > 0 {
			// 🔥 关键：还有其他连接，更新数组，不删除key
			wsm.connections[connectionID] = newConns
			log.Printf("🔗 [连接注销] ➖ 移除一个连接: %s (剩余 %d 个连接)",
				connectionID, len(newConns))
		} else {
			// 🔥 所有连接都断开了，删除整个key
			delete(wsm.connections, connectionID)
			log.Printf("🔗 [连接注销] ❌ 所有连接已断开: %s", connectionID)

			// 🔥 清理会话映射（只在所有连接都断开时才清理）
			sessionsToRemove := []string{}
			for sessionID, cid := range wsm.sessionToConnection {
				if cid == connectionID {
					sessionsToRemove = append(sessionsToRemove, sessionID)
				}
			}
			for _, sessionID := range sessionsToRemove {
				delete(wsm.sessionToConnection, sessionID)
				log.Printf("🔗 [连接注销] 🗑️ 清理会话映射: sessionID=%s", sessionID)
			}

			// 清理用户映射
			if connections, userExists := wsm.userToConnections[userID]; userExists {
				newConnections := []string{}
				for _, cid := range connections {
					if cid != connectionID {
						newConnections = append(newConnections, cid)
					}
				}
				if len(newConnections) == 0 {
					delete(wsm.userToConnections, userID)
					log.Printf("🔗 [连接注销] 🗑️ 清理用户映射: %s", userID)
				} else {
					wsm.userToConnections[userID] = newConnections
				}
			}
		}

		// 🔥 关键：只关闭当前这个conn，不影响其他连接
		conn.Close()
		log.Printf("🔗 [连接注销] ✅ 连接已关闭")
	} else {
		log.Printf("🔗 [连接注销] ⚠️ connectionID不存在: %s", connectionID)
	}
}

// 🔥 新增：向指定connectionID的所有连接广播消息
func (wsm *WebSocketManager) BroadcastToConnectionID(connectionID string, message interface{}) error {
	wsm.mutex.RLock()
	conns, exists := wsm.connections[connectionID]
	wsm.mutex.RUnlock()

	if !exists || len(conns) == 0 {
		return fmt.Errorf("连接 %s 不存在或无活跃连接", connectionID)
	}

	log.Printf("📡 [消息广播] 向 %s 的 %d 个连接广播消息", connectionID, len(conns))

	// 遍历所有连接，发送消息
	var lastErr error
	successCount := 0
	failedConns := []*websocket.Conn{}

	for i, conn := range conns {
		if err := conn.WriteJSON(message); err != nil {
			log.Printf("❌ [消息广播] 连接 #%d 发送失败: %v", i, err)
			lastErr = err
			failedConns = append(failedConns, conn)
		} else {
			successCount++
			log.Printf("✅ [消息广播] 连接 #%d 发送成功", i)
		}
	}

	// 清理失败的连接（异步清理，不阻塞当前操作）
	if len(failedConns) > 0 {
		go func() {
			for _, failedConn := range failedConns {
				wsm.UnregisterUser(connectionID, failedConn)
			}
		}()
	}

	log.Printf("📡 [消息广播] 完成: 成功 %d/%d 个连接", successCount, len(conns))
	return lastErr
}

// 🔥 新增：注册会话到连接的映射
func (wsm *WebSocketManager) RegisterSession(sessionID, connectionID string) bool {
	wsm.mutex.Lock()
	defer wsm.mutex.Unlock()

	log.Printf("🔗 [会话注册] ===== 开始注册会话映射 =====")
	log.Printf("🔗 [会话注册] 输入参数: sessionID=%s, connectionID=%s", sessionID, connectionID)
	log.Printf("🔗 [会话注册] 当前连接数: %d", len(wsm.connections))
	log.Printf("🔗 [会话注册] 当前会话映射数: %d", len(wsm.sessionToConnection))

	// 检查连接是否存在
	if _, exists := wsm.connections[connectionID]; exists {
		log.Printf("🔗 [会话注册] ✅ 连接 %s 存在，可以注册会话", connectionID)

		// 检查是否已经存在旧的映射
		if oldConnectionID, oldExists := wsm.sessionToConnection[sessionID]; oldExists {
			log.Printf("🔗 [会话注册] ⚠️ 会话 %s 已存在映射到连接 %s，将覆盖", sessionID, oldConnectionID)
		}

		wsm.sessionToConnection[sessionID] = connectionID
		userID := wsm.extractUserIDFromConnectionID(connectionID)
		log.Printf("🔗 [会话注册] 📋 注册会话: %s → 连接: %s (用户: %s)",
			sessionID, connectionID, userID)
		log.Printf("🔗 [会话注册] ✅ 会话注册成功: %s，连接: %s",
			sessionID, connectionID)
		log.Printf("🔗 [会话注册] ===== 会话映射注册完成 =====")
		return true
	} else {
		log.Printf("🔗 [会话注册] ❌ 连接 %s 不存在", connectionID)
		log.Printf("🔗 [会话注册] ===== 会话映射注册失败 =====")
		activeConnections := make([]string, 0, len(wsm.connections))
		for connID := range wsm.connections {
			activeConnections = append(activeConnections, connID)
		}
		log.Printf("🔗 [会话注册变更] 当前活跃连接: %v", activeConnections)
		return false
	}
}

// 🔥 新增：注销会话映射
func (wsm *WebSocketManager) UnregisterSession(sessionID string) {
	wsm.mutex.Lock()
	defer wsm.mutex.Unlock()

	if connectionID, exists := wsm.sessionToConnection[sessionID]; exists {
		delete(wsm.sessionToConnection, sessionID)
		log.Printf("[WebSocket] 🗑️ 会话注销: sessionID=%s, connectionID=%s", sessionID, connectionID)
	}
}

// 🔥 新增：基于sessionId推送指令（支持多端广播）
func (wsm *WebSocketManager) PushInstructionToSession(sessionID string, instruction models.LocalInstruction) (chan models.CallbackResult, error) {
	wsm.mutex.RLock()

	// 根据sessionID查找对应的connectionID
	connectionID, sessionExists := wsm.sessionToConnection[sessionID]
	if !sessionExists {
		wsm.mutex.RUnlock()
		log.Printf("[指令推送] ⚠️ 会话 %s 未注册", sessionID)
		return nil, fmt.Errorf("会话 %s 未注册", sessionID)
	}

	// 🔥 修改：检查该connectionID下是否有活跃连接
	conns, connExists := wsm.connections[connectionID]
	if !connExists || len(conns) == 0 {
		wsm.mutex.RUnlock()
		// 清理无效的会话映射
		wsm.mutex.Lock()
		delete(wsm.sessionToConnection, sessionID)
		wsm.mutex.Unlock()
		log.Printf("[指令推送] ⚠️ 会话 %s 对应的连接 %s 已断开", sessionID, connectionID)
		return nil, fmt.Errorf("会话 %s 对应的连接已断开", sessionID)
	}

	wsm.mutex.RUnlock()

	// 创建回调通道
	callbackChan := make(chan models.CallbackResult, 1)
	wsm.mutex.Lock()
	wsm.callbacks[instruction.CallbackID] = callbackChan
	wsm.mutex.Unlock()

	// 构建消息
	message := map[string]interface{}{
		"type": "instruction",
		"data": instruction,
	}

	userID := wsm.extractUserIDFromConnectionID(connectionID)
	log.Printf("📡 [指令推送] sessionID=%s → connectionID=%s (用户: %s, 连接数: %d)",
		sessionID, connectionID, userID, len(conns))
	log.Printf("📋 [指令推送] 指令详情: type=%s, callbackId=%s, target=%s",
		instruction.Type, instruction.CallbackID, instruction.Target)

	// 🔥 核心改动：使用广播替代单播
	if err := wsm.BroadcastToConnectionID(connectionID, message); err != nil {
		wsm.mutex.Lock()
		delete(wsm.callbacks, instruction.CallbackID)
		wsm.mutex.Unlock()
		close(callbackChan)
		log.Printf("❌ [指令推送] 广播失败: %v", err)
		return nil, fmt.Errorf("推送指令失败: %v", err)
	}

	log.Printf("✅ [指令推送] 指令已广播到 %d 个连接 (等待回调: %s)",
		len(conns), instruction.CallbackID)
	return callbackChan, nil
}

// 推送指令给指定用户 - 支持多工作空间连接（支持多端广播）
func (wsm *WebSocketManager) PushInstruction(userID string, instruction models.LocalInstruction) (chan models.CallbackResult, error) {
	wsm.mutex.RLock()

	// 查找用户的所有connectionID
	connectionIDs, userExists := wsm.userToConnections[userID]
	if !userExists || len(connectionIDs) == 0 {
		wsm.mutex.RUnlock()
		log.Printf("[WebSocket] ⚠️ 推送失败：用户 %s 未连接", userID)
		return nil, fmt.Errorf("用户 %s 未连接", userID)
	}

	// 🔥 策略：推送到用户的第一个活跃connectionID（主要工作空间）
	var targetConnectionID string
	var targetConns []*websocket.Conn

	for _, connectionID := range connectionIDs {
		if conns, exists := wsm.connections[connectionID]; exists && len(conns) > 0 {
			targetConnectionID = connectionID
			targetConns = conns
			break
		}
	}

	wsm.mutex.RUnlock()

	if targetConnectionID == "" || len(targetConns) == 0 {
		log.Printf("[WebSocket] ⚠️ 推送失败：用户 %s 的所有连接都不可用", userID)
		return nil, fmt.Errorf("用户 %s 的连接不可用", userID)
	}

	// 创建回调通道
	callbackChan := make(chan models.CallbackResult, 1)
	wsm.mutex.Lock()
	wsm.callbacks[instruction.CallbackID] = callbackChan
	wsm.mutex.Unlock()

	// 构建消息
	message := map[string]interface{}{
		"type": "instruction",
		"data": instruction,
	}

	log.Printf("[WebSocket] 📤 开始推送指令到用户 %s (连接: %s, 连接数: %d)", userID, targetConnectionID, len(targetConns))
	log.Printf("[WebSocket] 📋 指令详情: type=%s, callbackId=%s, target=%s",
		instruction.Type, instruction.CallbackID, instruction.Target)

	// 🔥 核心改动：使用广播替代单播
	if err := wsm.BroadcastToConnectionID(targetConnectionID, message); err != nil {
		wsm.mutex.Lock()
		delete(wsm.callbacks, instruction.CallbackID)
		wsm.mutex.Unlock()
		close(callbackChan)
		log.Printf("[WebSocket] ❌ 推送指令失败: %v", err)
		return nil, fmt.Errorf("发送指令失败: %v", err)
	}

	log.Printf("[WebSocket] ✅ 指令已广播到用户 %s 的 %d 个连接: %s (等待回调: %s)",
		userID, len(targetConns), instruction.Type, instruction.CallbackID)
	return callbackChan, nil
}

// 处理回调结果
func (wsm *WebSocketManager) HandleCallback(callbackID string, result models.CallbackResult) {
	wsm.mutex.RLock()
	callbackChan, exists := wsm.callbacks[callbackID]
	wsm.mutex.RUnlock()

	if !exists {
		log.Printf("[WebSocket] ⚠️ 收到未知回调ID: %s", callbackID)
		return
	}

	log.Printf("[WebSocket] 📥 处理回调: %s, success=%t, message=%s",
		callbackID, result.Success, result.Message)

	// 发送结果并清理
	select {
	case callbackChan <- result:
		log.Printf("[WebSocket] ✅ 回调已处理: %s", callbackID)
	case <-time.After(1 * time.Second):
		log.Printf("[WebSocket] ⏰ 回调处理超时: %s", callbackID)
	}

	wsm.mutex.Lock()
	delete(wsm.callbacks, callbackID)
	wsm.mutex.Unlock()
	close(callbackChan)
}

// 处理WebSocket连接
func (wsm *WebSocketManager) handleConnection(connectionID string, conn *websocket.Conn) {
	// 🔥 重要：传入具体的conn引用，确保注销时精确移除
	defer wsm.UnregisterUser(connectionID, conn)

	userID := wsm.extractUserIDFromConnectionID(connectionID)
	log.Printf("[WebSocket] 🚀 开始处理连接 %s (用户: %s)", connectionID, userID)

	// 🔥 新增：立即发送连接成功确认消息
	confirmMessage := map[string]interface{}{
		"type":         "connected",
		"connectionId": connectionID,
		"userId":       userID,
		"message":      "WebSocket连接已建立",
		"timestamp":    time.Now().Format(time.RFC3339),
	}
	if err := conn.WriteJSON(confirmMessage); err != nil {
		log.Printf("[WebSocket] ❌ 发送连接确认消息失败: %v", err)
		return
	}
	log.Printf("[WebSocket] ✅ 已发送连接确认消息: connectionID=%s", connectionID)

	// 设置读取超时 - 调整为更宽松的超时时间
	conn.SetReadDeadline(time.Now().Add(90 * time.Second)) // 从60秒调整为90秒

	// 🔥 修复：在心跳Pong处理中添加会话保活逻辑
	conn.SetPongHandler(func(string) error {
		log.Printf("[WebSocket] 💓 收到连接 %s 的Pong (用户: %s)", connectionID, userID)
		conn.SetReadDeadline(time.Now().Add(90 * time.Second)) // 从60秒调整为90秒

		// 🔥 新增：心跳保活 - 更新关联会话的时间戳
		wsm.updateSessionActivityByConnection(connectionID, userID)

		return nil
	})

	// 启动心跳 - 调整心跳间隔
	ticker := time.NewTicker(45 * time.Second) // 从30秒调整为45秒，给客户端更多响应时间
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				log.Printf("[WebSocket] 💓 发送心跳到连接 %s (用户: %s)", connectionID, userID)
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					log.Printf("[WebSocket] ❌ 心跳失败，连接 %s 异常: %v", connectionID, err)
					return
				}
			}
		}
	}()

	// 消息处理循环
	for {
		var message map[string]interface{}
		if err := conn.ReadJSON(&message); err != nil {
			log.Printf("[WebSocket] ❌ 连接 %s 读取消息失败: %v", connectionID, err)
			break
		}

		log.Printf("[WebSocket] 📥 收到连接 %s 的消息: %+v", connectionID, message)

		// 处理回调消息
		if msgType, ok := message["type"].(string); ok && msgType == "callback" {
			if callbackID, ok := message["callbackId"].(string); ok {
				success, _ := message["success"].(bool)
				messageStr, _ := message["message"].(string)

				result := models.CallbackResult{
					Success:   success,
					Message:   messageStr,
					Data:      message["data"],
					Timestamp: time.Now(),
				}

				log.Printf("[WebSocket] 🎯 处理回调消息: callbackId=%s, success=%t", callbackID, success)
				wsm.HandleCallback(callbackID, result)
			} else {
				log.Printf("[WebSocket] ⚠️ 回调消息缺少callbackId: %+v", message)
			}
		} else {
			log.Printf("[WebSocket] 📨 收到其他类型消息: type=%s", msgType)
		}
	}

	log.Printf("[WebSocket] 🔚 连接 %s 处理结束 (用户: %s)", connectionID, userID)
}

// 🔥 新增：通过连接ID更新会话活跃度
func (wsm *WebSocketManager) updateSessionActivityByConnection(connectionID, userID string) {
	wsm.mutex.RLock()

	// 查找该连接关联的所有会话
	var associatedSessions []string
	for sessionID, connID := range wsm.sessionToConnection {
		if connID == connectionID {
			associatedSessions = append(associatedSessions, sessionID)
		}
	}

	wsm.mutex.RUnlock()

	if len(associatedSessions) == 0 {
		log.Printf("[WebSocket] 💓 心跳保活: 连接 %s 未关联任何会话", connectionID)
		return
	}

	// 更新所有关联会话的活跃时间
	for _, sessionID := range associatedSessions {
		// 🔥 关键：调用会话时间戳更新逻辑
		if globalHandler != nil {
			globalHandler.UpdateSessionActivity(sessionID)
			log.Printf("[WebSocket] 💓 心跳保活: 已更新会话 %s 的活跃时间 (连接: %s)", sessionID, connectionID)
		} else {
			log.Printf("[WebSocket] ⚠️ 心跳保活: 无法更新会话 %s，全局处理器不可用", sessionID)
		}
	}
}

// 🔥 新增：全局处理器引用，用于调用会话更新方法
var globalHandler interface {
	UpdateSessionActivity(sessionID string)
}

// 🔥 新增：设置全局处理器引用
func SetGlobalHandler(handler interface{ UpdateSessionActivity(sessionID string) }) {
	globalHandler = handler
}

// 获取在线用户数 - 返回有连接的用户列表
func (wsm *WebSocketManager) GetOnlineUsers() []string {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()

	users := make([]string, 0, len(wsm.userToConnections))
	for userID := range wsm.userToConnections {
		users = append(users, userID)
	}

	log.Printf("[WebSocket] 📊 当前在线用户: %v (总连接数: %d)", users, len(wsm.connections))
	return users
}

// 🔥 新增：获取详细连接信息
func (wsm *WebSocketManager) GetConnectionStats() map[string]interface{} {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()

	stats := map[string]interface{}{
		"total_connections": len(wsm.connections),
		"online_users":      len(wsm.userToConnections),
		"user_connections":  make(map[string]int),
	}

	for userID, connections := range wsm.userToConnections {
		stats["user_connections"].(map[string]int)[userID] = len(connections)
	}

	return stats
}

// 🔥 新增：GetUserConnections 获取指定用户的所有连接ID
func (wsm *WebSocketManager) GetUserConnections(userID string) []string {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()

	connections, exists := wsm.userToConnections[userID]
	if !exists {
		return []string{}
	}

	// 返回连接ID的副本，避免外部修改
	result := make([]string, len(connections))
	copy(result, connections)
	return result
}
