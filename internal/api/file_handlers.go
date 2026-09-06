package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/security"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func getAuthenticatedUserID(c *gin.Context) (string, bool) {
	userID, exists := c.Get(CtxKeyUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return "", false
	}

	userIDStr, ok := userID.(string)
	if !ok || strings.TrimSpace(userIDStr) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "invalid authenticated user",
		})
		return "", false
	}

	return userIDStr, true
}

// FileUploadResponse 文件上传响应结构
type FileUploadResponse struct {
	FileID           string   `json:"file_id"`            // 文件ID
	FileName         string   `json:"file_name"`          // 文件名
	FileSize         int64    `json:"file_size"`          // 文件大小（字节）
	FileType         string   `json:"file_type"`          // 文件类型
	FileURL          string   `json:"file_url"`           // 文件访问URL
	UploadTime       int64    `json:"upload_time"`        // 上传时间戳
	SensitiveTypes   []string `json:"sensitive_types"`    // 检测到的敏感信息类型
	HasSensitiveData bool     `json:"has_sensitive_data"` // 是否包含敏感信息
}

// FileMetadata 文件元数据
type FileMetadata struct {
	FileID         string   `json:"file_id"`
	UserID         string   `json:"user_id"`
	FileName       string   `json:"file_name"`
	OriginalName   string   `json:"original_name"`
	FileSize       int64    `json:"file_size"`
	FileType       string   `json:"file_type"`
	MimeType       string   `json:"mime_type"`
	FilePath       string   `json:"file_path"`
	FileHash       string   `json:"file_hash"`
	UploadTime     int64    `json:"upload_time"`
	SensitiveTypes []string `json:"sensitive_types"`
}

const (
	// 最大文件大小：10MB
	MaxFileSize = 10 * 1024 * 1024

	// 允许的文件类型
	AllowedImageTypes = ".jpg,.jpeg,.png,.gif,.bmp"
	AllowedDocTypes   = ".pdf,.doc,.docx,.txt"
)

// 全局文件元数据存储（实际应该使用数据库）
var fileMetadataStore = make(map[string]*FileMetadata)

// RegisterFileRoutes 注册文件上传路由（使用安全处理器）
func RegisterFileRoutes(router gin.IRouter) {
	// 🔐 统一使用加密的安全处理器
	router.POST("/upload", SecureFileUpload)
	router.GET("/:file_id", SecureFileDownload)
	router.GET("/user/:user_id", HandleListUserFiles)
}

// HandleFileUpload 处理文件上传
// ⚠️ 已弃用：此处理器不加密文件，存在安全风险
// 请使用 SecureFileUpload（在 secure_file_handlers.go 中）
// 保留此函数仅用于向后兼容，不应在新代码中使用
func HandleFileUpload(c *gin.Context) {
	// 获取用户ID
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		return
	}

	// 获取上传的文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to get file: %v", err),
		})
		return
	}
	defer file.Close()

	// 验证文件大小
	if header.Size > MaxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("File size exceeds maximum limit of %d MB", MaxFileSize/(1024*1024)),
		})
		return
	}

	// 验证文件类型
	fileExt := strings.ToLower(filepath.Ext(header.Filename))
	if !isAllowedFileType(fileExt) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": fmt.Sprintf("File type %s is not allowed. Allowed types: %s, %s",
				fileExt, AllowedImageTypes, AllowedDocTypes),
		})
		return
	}

	// Sanitize文件名，防止路径遍历攻击
	sanitizedFilename := sanitizeFilename(header.Filename)

	// 生成唯一文件ID
	fileID := uuid.New().String()

	// 生成新的文件名（保留扩展名）
	newFilename := fmt.Sprintf("%s_%s%s", fileID, sanitizedFilename, fileExt)

	// 创建用户上传目录（使用相对路径）
	uploadDir := filepath.Join("./data/uploads", userID)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to create upload directory: %v", err),
		})
		return
	}

	// 完整文件路径
	filePath := filepath.Join(uploadDir, newFilename)

	// 保存文件到磁盘
	out, err := os.Create(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to create file: %v", err),
		})
		return
	}
	defer out.Close()

	// 复制文件内容并计算哈希
	fileHash, fileSize, err := copyAndHash(file, out)
	if err != nil {
		os.Remove(filePath) // 清理失败的文件
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to save file: %v", err),
		})
		return
	}

	// 检测文件中的敏感信息（如果是文本或图片）
	sensitiveTypes := detectFileSensitiveInfo(filePath, fileExt)

	// 保存文件元数据
	metadata := &FileMetadata{
		FileID:         fileID,
		UserID:         userID,
		FileName:       newFilename,
		OriginalName:   sanitizedFilename,
		FileSize:       fileSize,
		FileType:       getFileType(fileExt),
		MimeType:       header.Header.Get("Content-Type"),
		FilePath:       filePath,
		FileHash:       fileHash,
		UploadTime:     time.Now().Unix(),
		SensitiveTypes: sensitiveTypes,
	}
	fileMetadataStore[fileID] = metadata

	// 构建文件访问URL
	fileURL := fmt.Sprintf("/api/files/%s", fileID)

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": FileUploadResponse{
			FileID:           fileID,
			FileName:         sanitizedFilename + fileExt,
			FileSize:         fileSize,
			FileType:         getFileType(fileExt),
			FileURL:          fileURL,
			UploadTime:       metadata.UploadTime,
			SensitiveTypes:   sensitiveTypes,
			HasSensitiveData: len(sensitiveTypes) > 0,
		},
	})
}

// HandleFileDownload 处理文件下载
// ⚠️ 已弃用：此处理器不解密文件，无法读取加密文件
// 请使用 SecureFileDownload（在 secure_file_handlers.go 中）
// 保留此函数仅用于向后兼容，不应在新代码中使用
func HandleFileDownload(c *gin.Context) {
	fileID := c.Param("file_id")

	// 查找文件元数据
	metadata, exists := fileMetadataStore[fileID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "File not found",
		})
		return
	}

	// 验证文件是否存在
	if _, err := os.Stat(metadata.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "File not found on disk",
		})
		return
	}

	// 设置响应头
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", metadata.OriginalName))
	c.Header("Content-Type", metadata.MimeType)

	// 发送文件
	c.File(metadata.FilePath)
}

// HandleListUserFiles 列出用户的所有文件
func HandleListUserFiles(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		return
	}

	var userFiles []*FileMetadata
	for _, metadata := range fileMetadataStore {
		if metadata.UserID == userID {
			userFiles = append(userFiles, metadata)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user_id": userID,
			"files":   userFiles,
			"count":   len(userFiles),
		},
	})
}

// isAllowedFileType 检查文件类型是否允许
func isAllowedFileType(ext string) bool {
	allowedTypes := AllowedImageTypes + "," + AllowedDocTypes
	return strings.Contains(allowedTypes, ext)
}

// sanitizeFilename 清理文件名，防止路径遍历攻击
func sanitizeFilename(filename string) string {
	// 移除扩展名
	ext := filepath.Ext(filename)
	nameWithoutExt := strings.TrimSuffix(filename, ext)

	// 移除危险字符
	nameWithoutExt = strings.ReplaceAll(nameWithoutExt, "..", "")
	nameWithoutExt = strings.ReplaceAll(nameWithoutExt, "/", "")
	nameWithoutExt = strings.ReplaceAll(nameWithoutExt, "\\", "")
	nameWithoutExt = strings.ReplaceAll(nameWithoutExt, ":", "")
	nameWithoutExt = strings.ReplaceAll(nameWithoutExt, "*", "")
	nameWithoutExt = strings.ReplaceAll(nameWithoutExt, "?", "")
	nameWithoutExt = strings.ReplaceAll(nameWithoutExt, "\"", "")
	nameWithoutExt = strings.ReplaceAll(nameWithoutExt, "<", "")
	nameWithoutExt = strings.ReplaceAll(nameWithoutExt, ">", "")
	nameWithoutExt = strings.ReplaceAll(nameWithoutExt, "|", "")

	// 限制长度
	if len(nameWithoutExt) > 100 {
		nameWithoutExt = nameWithoutExt[:100]
	}

	// 如果清理后为空，使用默认名称
	if nameWithoutExt == "" {
		nameWithoutExt = "file"
	}

	return nameWithoutExt
}

// copyAndHash 复制文件内容并计算SHA256哈希
func copyAndHash(src io.Reader, dst io.Writer) (string, int64, error) {
	hash := sha256.New()
	multiWriter := io.MultiWriter(dst, hash)

	size, err := io.Copy(multiWriter, src)
	if err != nil {
		return "", 0, err
	}

	hashSum := hex.EncodeToString(hash.Sum(nil))
	return hashSum, size, nil
}

// getFileType 根据扩展名获取文件类型
func getFileType(ext string) string {
	ext = strings.ToLower(ext)

	imageTypes := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp"}
	for _, imgExt := range imageTypes {
		if ext == imgExt {
			return "image"
		}
	}

	docTypes := []string{".pdf", ".doc", ".docx", ".txt"}
	for _, docExt := range docTypes {
		if ext == docExt {
			return "document"
		}
	}

	return "unknown"
}

// detectFileSensitiveInfo 检测文件中的敏感信息
func detectFileSensitiveInfo(filePath string, fileExt string) []string {
	var sensitiveTypes []string

	// 对于文本文件，读取内容并检测
	if fileExt == ".txt" {
		content, err := os.ReadFile(filePath)
		if err == nil {
			detector := security.NewDetector()
			result := detector.Detect(string(content))

			for _, item := range result {
				// 去重
				found := false
				for _, t := range sensitiveTypes {
					if t == string(item.Type) {
						found = true
						break
					}
				}
				if !found {
					sensitiveTypes = append(sensitiveTypes, string(item.Type))
				}
			}
		}
	}

	// 对于图片文件，可以在这里集成OCR服务
	// TODO: 集成OCR服务来识别图片中的文字和敏感信息

	// 对于PDF文件，可以在这里集成PDF解析
	// TODO: 集成PDF解析来提取文本并检测敏感信息

	return sensitiveTypes
}

// copyFileToWriter 将multipart.File复制到io.Writer
func copyFileToWriter(file multipart.File, writer io.Writer) error {
	_, err := io.Copy(writer, file)
	return err
}
