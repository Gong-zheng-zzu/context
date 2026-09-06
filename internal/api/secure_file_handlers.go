package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/security"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// 全局审计日志记录器
var (
	auditLogger     *security.AuditLogger
	auditLoggerOnce sync.Once
)

// 初始化审计日志记录器
func initAuditLogger() {
	auditLoggerOnce.Do(func() {
		var err error
		auditLogger, err = security.NewAuditLogger("./data/audit_logs")
		if err != nil {
			fmt.Printf("⚠️ 审计日志初始化失败: %v\n", err)
		} else {
			fmt.Println("✅ 审计日志记录器已初始化")
		}
	})
}

// SecureFileUpload 安全文件上传处理器（带加密+脱敏+审计）
func SecureFileUpload(c *gin.Context) {
	initAuditLogger()

	// 获取用户ID
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		return
	}

	fmt.Printf("✅ [文件上传] 开始处理文件上传，用户ID: %s\n", userID)

	// 获取客户端IP
	clientIP := c.ClientIP()

	// 获取上传的文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		fmt.Printf("❌ [文件上传] 获取文件失败: %v\n", err)
		if auditLogger != nil {
			auditLogger.Log(security.AuditEvent{
				Timestamp: time.Now(),
				Level:     security.AuditLevelWarning,
				UserID:    userID,
				EventType: "file_upload_failed",
				Message:   "Failed to get file from request",
				Metadata:  map[string]interface{}{"client_ip": clientIP, "error": err.Error()},
			})
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to get file: %v", err),
		})
		return
	}
	defer file.Close()

	// 验证文件大小
	if header.Size > MaxFileSize {
		errMsg := fmt.Sprintf("File size exceeds maximum limit of %d MB", MaxFileSize/(1024*1024))
		fmt.Printf("❌ [文件上传] 文件大小超限: %s, 大小: %d bytes\n", header.Filename, header.Size)
		if auditLogger != nil {
			auditLogger.Log(security.AuditEvent{
				Timestamp: time.Now(),
				Level:     security.AuditLevelWarning,
				UserID:    userID,
				EventType: "file_upload_failed",
				Message:   errMsg,
				Metadata:  map[string]interface{}{"client_ip": clientIP, "filename": header.Filename, "size": header.Size},
			})
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   errMsg,
		})
		return
	}

	fmt.Printf("✅ [文件上传] 文件大小验证通过: %s, 大小: %d bytes\n", header.Filename, header.Size)

	// 验证文件类型
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowedTypes := AllowedImageTypes + "," + AllowedDocTypes
	if !strings.Contains(allowedTypes, ext) {
		errMsg := fmt.Sprintf("File type %s not allowed. Allowed types: %s", ext, allowedTypes)
		fmt.Printf("❌ [文件上传] 文件类型不支持: %s\n", ext)
		if auditLogger != nil {
			auditLogger.Log(security.AuditEvent{
				Timestamp: time.Now(),
				Level:     security.AuditLevelWarning,
				UserID:    userID,
				EventType: "file_upload_failed",
				Message:   errMsg,
				Metadata:  map[string]interface{}{"client_ip": clientIP, "filename": header.Filename, "size": header.Size},
			})
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   errMsg,
		})
		return
	}

	// 读取文件内容
	fileContent, err := io.ReadAll(file)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to read file: %v", err)
		if auditLogger != nil {
			auditLogger.Log(security.AuditEvent{
				Timestamp: time.Now(),
				Level:     security.AuditLevelWarning,
				UserID:    userID,
				EventType: "file_upload_failed",
				Message:   errMsg,
				Metadata:  map[string]interface{}{"client_ip": clientIP, "filename": header.Filename, "size": header.Size},
			})
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   errMsg,
		})
		return
	}

	// 🔐 步骤1: 加密文件内容
	encryption := security.NewFileEncryption(userID)
	encryptedContent, err := encryption.Encrypt(fileContent)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to encrypt file: %v", err)
		if auditLogger != nil {
			auditLogger.Log(security.AuditEvent{
				Timestamp: time.Now(),
				Level:     security.AuditLevelWarning,
				UserID:    userID,
				EventType: "file_upload_failed",
				Message:   errMsg,
				Metadata:  map[string]interface{}{"client_ip": clientIP, "filename": header.Filename, "size": header.Size},
			})
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   errMsg,
		})
		return
	}

	// 生成文件ID和路径
	fileID := uuid.New().String()
	sanitizedName := sanitizeFileName(header.Filename)
	userDir := filepath.Join("./data/uploads", userID)

	// 创建用户目录
	if err := os.MkdirAll(userDir, 0755); err != nil {
		errMsg := fmt.Sprintf("Failed to create user directory: %v", err)
		if auditLogger != nil {
			auditLogger.Log(security.AuditEvent{
				Timestamp: time.Now(),
				Level:     security.AuditLevelCritical,
				UserID:    userID,
				EventType: "file_upload_failed",
				Message:   errMsg,
				Metadata:  map[string]interface{}{"client_ip": clientIP, "filename": header.Filename, "file_id": fileID, "error": "create_directory_failed"},
			})
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   errMsg,
		})
		return
	}

	// 保存加密文件（添加.enc后缀）
	filePath := filepath.Join(userDir, fileID+".enc")
	if err := os.WriteFile(filePath, encryptedContent, 0644); err != nil {
		errMsg := fmt.Sprintf("Failed to save encrypted file: %v", err)
		fmt.Printf("❌ [文件上传] 保存加密文件失败: %v\n", err)
		if auditLogger != nil {
			auditLogger.Log(security.AuditEvent{
				Timestamp: time.Now(),
				Level:     security.AuditLevelCritical,
				UserID:    userID,
				EventType: "file_upload_failed",
				Message:   errMsg,
				Metadata:  map[string]interface{}{"client_ip": clientIP, "filename": header.Filename, "file_id": fileID, "error": "save_file_failed"},
			})
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   errMsg,
		})
		return
	}

	fmt.Printf("✅ [文件上传] 文件保存成功: %s\n", filePath)

	// 计算原始文件哈希
	fileHash := sha256.Sum256(fileContent)
	fileHashStr := hex.EncodeToString(fileHash[:])

	// 🔍 步骤2: 检测敏感信息（仅对文本文件）
	var sensitiveTypes []string
	if ext == ".txt" {
		detector := security.NewDetector()
		sensitiveInfos := detector.Detect(string(fileContent))

		// 提取敏感信息类型
		typeSet := make(map[string]bool)
		for _, info := range sensitiveInfos {
			typeSet[string(info.Type)] = true
		}
		for t := range typeSet {
			sensitiveTypes = append(sensitiveTypes, t)
		}
	}

	// 保存文件元数据
	metadata := &FileMetadata{
		FileID:         fileID,
		UserID:         userID,
		FileName:       sanitizedName,
		OriginalName:   header.Filename,
		FileSize:       header.Size,
		FileType:       ext,
		MimeType:       header.Header.Get("Content-Type"),
		FilePath:       filePath,
		FileHash:       fileHashStr,
		UploadTime:     time.Now().Unix(),
		SensitiveTypes: sensitiveTypes,
	}
	fileMetadataStore[fileID] = metadata

	// 📝 步骤3: 记录审计日志
	if auditLogger != nil {
		auditLogger.Log(security.AuditEvent{
			Timestamp: time.Now(),
			Level:     security.AuditLevelInfo,
			UserID:    userID,
			EventType: "file_upload_success",
			Message:   fmt.Sprintf("File uploaded successfully: %s", header.Filename),
			Metadata: map[string]interface{}{
				"client_ip":       clientIP,
				"filename":        header.Filename,
				"file_id":         fileID,
				"file_size":       header.Size,
				"sensitive_types": sensitiveTypes,
			},
		})
	}

	fmt.Printf("✅ [文件上传] 文件上传成功: FileID=%s, FileName=%s, Size=%d\n", fileID, sanitizedName, header.Size)

	// 返回响应
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": FileUploadResponse{
			FileID:           fileID,
			FileName:         sanitizedName,
			FileSize:         header.Size,
			FileType:         ext,
			FileURL:          fmt.Sprintf("/api/files/%s", fileID),
			UploadTime:       metadata.UploadTime,
			SensitiveTypes:   sensitiveTypes,
			HasSensitiveData: len(sensitiveTypes) > 0,
		},
	})
}

// SecureFileDownload 安全文件下载处理器（自动解密+脱敏）
func SecureFileDownload(c *gin.Context) {
	initAuditLogger()

	fileID := c.Param("file_id")
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		return
	}
	clientIP := c.ClientIP()

	// 获取文件元数据
	metadata, exists := fileMetadataStore[fileID]
	if !exists {
		if auditLogger != nil {
			auditLogger.Log(security.AuditEvent{
				Timestamp: time.Now(),
				Level:     security.AuditLevelWarning,
				UserID:    userID,
				EventType: "file_download_failed",
				Message:   "File not found",
				Metadata:  map[string]interface{}{"client_ip": clientIP, "file_id": fileID},
			})
		}
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "File not found",
		})
		return
	}

	// 🔒 访问权限验证：只能下载自己的文件
	if metadata.UserID != userID {
		errMsg := "Access denied: You can only download your own files"
		if auditLogger != nil {
			auditLogger.Log(security.AuditEvent{
				Timestamp: time.Now(),
				Level:     security.AuditLevelWarning,
				UserID:    userID,
				EventType: "file_download_failed",
				Message:   "File not found",
				Metadata:  map[string]interface{}{"client_ip": clientIP, "file_id": fileID},
			})
		}
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   errMsg,
		})
		return
	}

	// 读取加密文件
	encryptedContent, err := os.ReadFile(metadata.FilePath)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to read file: %v", err)
		if auditLogger != nil {
			auditLogger.Log(security.AuditEvent{
				Timestamp: time.Now(),
				Level:     security.AuditLevelWarning,
				UserID:    userID,
				EventType: "file_download_failed",
				Message:   "File not found",
				Metadata:  map[string]interface{}{"client_ip": clientIP, "file_id": fileID},
			})
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   errMsg,
		})
		return
	}

	// 🔓 解密文件内容
	encryption := security.NewFileEncryption(userID)
	fileContent, err := encryption.Decrypt(encryptedContent)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to decrypt file: %v", err)
		if auditLogger != nil {
			auditLogger.Log(security.AuditEvent{
				Timestamp: time.Now(),
				Level:     security.AuditLevelWarning,
				UserID:    userID,
				EventType: "file_download_failed",
				Message:   "File not found",
				Metadata:  map[string]interface{}{"client_ip": clientIP, "file_id": fileID},
			})
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   errMsg,
		})
		return
	}

	// 🎭 如果是文本文件且包含敏感信息，自动脱敏
	if metadata.FileType == ".txt" && len(metadata.SensitiveTypes) > 0 {
		detector := security.NewDetector()
		sensitiveInfos := detector.Detect(string(fileContent))

		// 替换敏感信息为脱敏值
		contentStr := string(fileContent)
		for _, info := range sensitiveInfos {
			// 根据类型生成脱敏值
			maskedValue := "***"
			switch info.Type {
			case security.SensitiveTypeIDCard:
				maskedValue = info.Value[:6] + "********" + info.Value[len(info.Value)-4:]
			case security.SensitiveTypePhone:
				maskedValue = info.Value[:3] + "****" + info.Value[len(info.Value)-4:]
			case security.SensitiveTypeBankCard:
				maskedValue = info.Value[:4] + "********" + info.Value[len(info.Value)-4:]
			default:
				maskedValue = "***"
			}
			contentStr = strings.ReplaceAll(contentStr, info.Value, maskedValue)
		}
		fileContent = []byte(contentStr)
	}

	// 📝 记录审计日志
	if auditLogger != nil {
		auditLogger.Log(security.AuditEvent{
			Timestamp: time.Now(),
			Level:     security.AuditLevelInfo,
			UserID:    userID,
			EventType: "file_download_success",
			Message:   fmt.Sprintf("File downloaded successfully: %s", metadata.FileName),
			Metadata: map[string]interface{}{
				"client_ip":     clientIP,
				"file_id":       fileID,
				"filename":      metadata.FileName,
				"has_sensitive": len(metadata.SensitiveTypes) > 0,
				"redacted":      metadata.FileType == ".txt" && len(metadata.SensitiveTypes) > 0,
			},
		})
	}

	// 返回文件
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", metadata.FileName))
	c.Header("Content-Type", metadata.MimeType)
	c.Data(http.StatusOK, metadata.MimeType, fileContent)
}

// sanitizeFileName 清理文件名，防止路径遍历攻击
func sanitizeFileName(filename string) string {
	// 移除危险字符
	dangerous := []string{"..", "/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range dangerous {
		filename = strings.ReplaceAll(filename, char, "_")
	}

	// 限制文件名长度
	if len(filename) > 100 {
		ext := filepath.Ext(filename)
		filename = filename[:100-len(ext)] + ext
	}

	return filename
}
