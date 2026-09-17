//go:build http

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/contextkeeper/service/internal/api"
	"github.com/contextkeeper/service/internal/config"
	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval/knowledge"
	"github.com/contextkeeper/service/internal/llm"
	"github.com/contextkeeper/service/internal/middleware"
	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/security"
	"github.com/contextkeeper/service/internal/services"
	"github.com/contextkeeper/service/internal/store"
	"github.com/contextkeeper/service/internal/utils"
	"github.com/contextkeeper/service/pkg/aliyun"
	"github.com/contextkeeper/service/pkg/vectorstore"
)

func main() {
	log.Println("启动 Context-Keeper Streamable HTTP MCP 服务器...")

	// 设置Streamable HTTP模式环境变量
	os.Setenv("HTTP_MODE", "true")
	os.Setenv("STREAMABLE_HTTP_MODE", "true")

	// HTTP模式：日志直接输出到标准输出（不再需要文件日志）
	// 这样云端部署时可以通过 docker logs 直接查看业务日志
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("✅ HTTP模式：日志输出到标准输出（便于云端查看）")

	// 初始化TraceID系统
	utils.InitTraceIDSystem()

	// 初始化共享组件（🔥 修改：现在返回LLMDrivenContextService以支持LLM驱动智能功能）
	llmDrivenContextService, _, cancelCleanup := initializeServices()
	defer cancelCleanup()

	// 加载配置
	cfg := config.Load()

	// 设置Gin模式
	if getEnv("GIN_MODE", cfg.GinMode) == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建Gin路由器
	router := gin.New()

	// 添加中间件
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	// 🔥 【新增】添加TraceID中间件
	router.Use(utils.TraceIDMiddleware())

	// 配置CORS
	config_cors := cors.DefaultConfig()

	// 🔥 从环境变量读取允许的源（安全配置）
	allowedOrigins := getEnv("ALLOWED_ORIGINS", "")
	if allowedOrigins != "" {
		config_cors.AllowOrigins = strings.Split(allowedOrigins, ",")
		log.Printf("✅ CORS配置: 使用环境变量指定的源: %v", config_cors.AllowOrigins)
	} else {
		// 开发环境默认配置（包括端口8889的静态文件服务器）
		config_cors.AllowOrigins = []string{
			"http://localhost:8088",
			"http://127.0.0.1:8088",
			"http://localhost:8889",
			"http://127.0.0.1:8889",
			"http://localhost:8000",
			"http://127.0.0.1:8000",
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			"http://localhost:5500",
			"http://127.0.0.1:5500",
		}
		log.Printf("⚠️ CORS配置: 使用开发环境默认源: %v", config_cors.AllowOrigins)
	}
	config_cors.AllowAllOrigins = false

	config_cors.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	config_cors.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "Accept", "Cache-Control", "X-Requested-With", "Last-Event-ID", "X-Trace-ID"}
	config_cors.AllowCredentials = true
	config_cors.ExposeHeaders = []string{"Content-Length", "X-Trace-ID"}
	config_cors.MaxAge = 12 * time.Hour
	router.Use(cors.New(config_cors))

	// 🔥 【新逻辑】初始化向量存储工厂
	log.Println("🏭 [向量存储工厂] 开始初始化向量存储工厂...")
	factory, err := vectorstore.InitializeFactoryFromEnv()
	if err != nil {
		log.Printf("❌ [向量存储工厂] 工厂初始化失败: %v", err)
		log.Printf("⚠️ [向量存储工厂] 将回退到传统阿里云VectorService")

		// 回退到传统方式
		embeddingAPIURL := getEnv("EMBEDDING_API_URL", cfg.EmbeddingAPIURL)
		embeddingAPIKey := getEnv("EMBEDDING_API_KEY", cfg.EmbeddingAPIKey)
		vectorDBURL := getEnv("VECTOR_DB_URL", cfg.VectorDBURL)
		vectorDBAPIKey := getEnv("VECTOR_DB_API_KEY", cfg.VectorDBAPIKey)
		vectorDBCollection := getEnv("VECTOR_DB_COLLECTION", cfg.VectorDBCollection)
		vectorDBDimension := getIntEnv("VECTOR_DB_DIMENSION", cfg.VectorDBDimension)
		vectorDBMetric := getEnv("VECTOR_DB_METRIC", cfg.VectorDBMetric)
		similarityThreshold := getFloatEnv("SIMILARITY_THRESHOLD", cfg.SimilarityThreshold)

		// 检查用户仓库类型，只有在使用向量存储类型时才创建 vectorService
		userRepoType := os.Getenv("USER_REPOSITORY_TYPE")
		log.Printf("🔍 [调试] USER_REPOSITORY_TYPE环境变量: '%s'", userRepoType)

		var vectorService *aliyun.VectorService
		if userRepoType == "aliyun" || userRepoType == "" {
			// 只有在使用 aliyun 或未指定类型时才创建向量服务
			vectorService = aliyun.NewVectorService(
				embeddingAPIURL,
				embeddingAPIKey,
				vectorDBURL,
				vectorDBAPIKey,
				vectorDBCollection,
				vectorDBDimension,
				vectorDBMetric,
				similarityThreshold,
			)
			log.Printf("✅ [调试] 传统向量服务创建成功：vectorService非nil")
			log.Printf("🔍 [调试] 向量服务配置 - URL: %s, APIKey: %s, Collection: %s",
				vectorDBURL,
				"***"+vectorDBAPIKey[len(vectorDBAPIKey)-4:], // 只显示最后4位
				vectorDBCollection)
		} else {
			log.Printf("ℹ️ [调试] 使用 %s 用户仓库类型，不创建向量服务", userRepoType)
		}

		// 创建用户存储仓库（使用工厂模式）
		log.Printf("🏭 [调试] 开始创建用户存储仓库...")
		userRepository, err := services.CreateUserRepositoryWithAutoDetection(vectorService)
		if err != nil {
			log.Fatalf("创建用户存储仓库失败: %v", err)
		}
		log.Printf("✅ [调试] 用户存储仓库创建成功")

		// 初始化用户存储仓库
		if err := userRepository.InitRepository(); err != nil {
			log.Printf("警告：初始化用户存储仓库失败: %v", err)
		} else {
			log.Println("用户存储仓库初始化成功")
		}

		// 🔥 初始化安全服务
		log.Println("🔒 [安全服务] 初始化安全服务...")
		securityService, err := initSecurityService()
		if err != nil {
			log.Printf("⚠️ [安全服务] 初始化失败: %v，安全功能将不可用", err)
			securityService = nil
		} else {
			log.Println("✅ [安全服务] 安全服务初始化成功")
		}

		// 创建API处理器
		handler := api.NewHandler(llmDrivenContextService, vectorService, userRepository, cfg, securityService)

		// 注册路由并启动服务器
		setupRoutesAndStartServer(router, handler, cfg)
		return
	}

	// 🔥 【新逻辑】工厂初始化成功，获取当前配置的向量存储
	log.Println("✅ [向量存储工厂] 工厂初始化成功，获取当前向量存储实例...")

	currentVectorStore, err := factory.GetCurrentVectorStore()
	if err != nil {
		log.Printf("⚠️ [向量存储工厂] 获取当前向量存储失败: %v (HTTP模式继续运行)", err)
		// 使用默认的向量服务，不中断服务启动
		currentVectorStore = nil
	}

	// 检查向量存储类型
	vectorStoreType := getEnv("VECTOR_STORE_TYPE", cfg.VectorStoreType)
	log.Printf("✅ [向量存储工厂] 成功加载向量存储类型: %s", vectorStoreType)

	// 初始化向量数据库和表空间
	if currentVectorStore != nil {
		log.Printf("🔧 [向量存储工厂] 开始初始化向量数据库和表空间...")
		collectionName := getEnv("VECTOR_DB_COLLECTION", cfg.VectorDBCollection)
		if err := currentVectorStore.EnsureCollection(collectionName); err != nil {
			log.Printf("⚠️ [向量存储工厂] 向量集合初始化失败: %v (HTTP模式继续运行)", err)
		} else {
			log.Printf("✅ [向量存储工厂] 向量集合初始化成功")
		}
	} else {
		log.Printf("⚠️ [向量存储工厂] 向量存储不可用，跳过集合初始化")
	}

	// 🔥 【重要】根据USER_REPOSITORY_TYPE创建对应的客户端
	userRepositoryType := getEnv("USER_REPOSITORY_TYPE", cfg.UserRepositoryType)
	log.Printf("🔍 [用户存储仓库] 检测到用户存储类型: %s", userRepositoryType)

	var userRepository models.UserRepository
	var compatibilityVectorService *aliyun.VectorService

	switch userRepositoryType {
	case "vearch":
		log.Printf("🔧 [用户存储仓库] 使用Vearch存储，从工厂获取VearchClient...")
		// 从向量存储工厂获取VearchClient
		vearchClient, err := factory.GetVearchClient()
		if err != nil {
			log.Printf("⚠️ [用户存储仓库] 获取VearchClient失败: %v，使用内存存储", err)
			// 降级到内存存储
			userRepository = store.NewMemoryUserRepository()
		} else {
			log.Printf("✅ [用户存储仓库] 成功获取VearchClient")

			// 创建Vearch用户存储仓库
			userRepository, err = services.CreateUserRepositoryWithAutoDetection(vearchClient)
			if err != nil {
				log.Printf("⚠️ [用户存储仓库] 创建Vearch用户存储仓库失败: %v，使用内存存储", err)
				userRepository = store.NewMemoryUserRepository()
			} else {
				log.Printf("✅ [用户存储仓库] Vearch用户存储仓库创建成功")

				// 为了兼容性，仍然需要创建阿里云VectorService用于API Handler
				embeddingAPIURL := getEnv("EMBEDDING_API_URL", cfg.EmbeddingAPIURL)
				embeddingAPIKey := getEnv("EMBEDDING_API_KEY", cfg.EmbeddingAPIKey)
				vectorDBURL := getEnv("VECTOR_DB_URL", cfg.VectorDBURL)
				vectorDBAPIKey := getEnv("VECTOR_DB_API_KEY", cfg.VectorDBAPIKey)
				vectorDBCollection := getEnv("VECTOR_DB_COLLECTION", cfg.VectorDBCollection)
				vectorDBDimension := getIntEnv("VECTOR_DB_DIMENSION", cfg.VectorDBDimension)
				vectorDBMetric := getEnv("VECTOR_DB_METRIC", cfg.VectorDBMetric)
				similarityThreshold := getFloatEnv("SIMILARITY_THRESHOLD", cfg.SimilarityThreshold)

				compatibilityVectorService = aliyun.NewVectorService(
					embeddingAPIURL,
					embeddingAPIKey,
					vectorDBURL,
					vectorDBAPIKey,
					vectorDBCollection,
					vectorDBDimension,
					vectorDBMetric,
					similarityThreshold,
				)
				log.Printf("✅ [用户存储仓库] 创建阿里云实例用于API兼容性")
			}
		}

	case "aliyun":
		log.Printf("🔧 [用户存储仓库] 使用阿里云存储...")
		// 创建传统阿里云VectorService用于用户存储仓库
		embeddingAPIURL := getEnv("EMBEDDING_API_URL", cfg.EmbeddingAPIURL)
		embeddingAPIKey := getEnv("EMBEDDING_API_KEY", cfg.EmbeddingAPIKey)
		vectorDBURL := getEnv("VECTOR_DB_URL", cfg.VectorDBURL)
		vectorDBAPIKey := getEnv("VECTOR_DB_API_KEY", cfg.VectorDBAPIKey)
		vectorDBCollection := getEnv("VECTOR_DB_COLLECTION", cfg.VectorDBCollection)
		vectorDBDimension := getIntEnv("VECTOR_DB_DIMENSION", cfg.VectorDBDimension)
		vectorDBMetric := getEnv("VECTOR_DB_METRIC", cfg.VectorDBMetric)
		similarityThreshold := getFloatEnv("SIMILARITY_THRESHOLD", cfg.SimilarityThreshold)

		aliyunVectorService := aliyun.NewVectorService(
			embeddingAPIURL,
			embeddingAPIKey,
			vectorDBURL,
			vectorDBAPIKey,
			vectorDBCollection,
			vectorDBDimension,
			vectorDBMetric,
			similarityThreshold,
		)
		log.Printf("✅ [用户存储仓库] 创建阿里云实例用于用户存储")

		// 创建用户存储仓库（使用阿里云实例）
		userRepository, err = services.CreateUserRepositoryWithAutoDetection(aliyunVectorService)
		if err != nil {
			log.Fatalf("❌ [用户存储仓库] 创建阿里云用户存储仓库失败: %v", err)
		}
		log.Printf("✅ [用户存储仓库] 阿里云用户存储仓库创建成功")

		// 兼容性VectorService就是阿里云本身
		compatibilityVectorService = aliyunVectorService

	default:
		log.Printf("🔧 [用户存储仓库] 使用默认存储类型: %s", userRepositoryType)
		// 其他类型（memory, mysql, tencent等）不需要特殊的客户端
		userRepository, err = services.CreateUserRepositoryWithAutoDetection(nil)
		if err != nil {
			log.Fatalf("❌ [用户存储仓库] 创建%s用户存储仓库失败: %v", userRepositoryType, err)
		}
		log.Printf("✅ [用户存储仓库] %s用户存储仓库创建成功", userRepositoryType)

		// 创建阿里云VectorService用于API兼容性（仅在需要时）
		if userRepositoryType == "aliyun" {
			embeddingAPIURL := getEnv("EMBEDDING_API_URL", cfg.EmbeddingAPIURL)
			embeddingAPIKey := getEnv("EMBEDDING_API_KEY", cfg.EmbeddingAPIKey)
			vectorDBURL := getEnv("VECTOR_DB_URL", cfg.VectorDBURL)
			vectorDBAPIKey := getEnv("VECTOR_DB_API_KEY", cfg.VectorDBAPIKey)
			vectorDBCollection := getEnv("VECTOR_DB_COLLECTION", cfg.VectorDBCollection)
			vectorDBDimension := getIntEnv("VECTOR_DB_DIMENSION", cfg.VectorDBDimension)
			vectorDBMetric := getEnv("VECTOR_DB_METRIC", cfg.VectorDBMetric)
			similarityThreshold := getFloatEnv("SIMILARITY_THRESHOLD", cfg.SimilarityThreshold)

			compatibilityVectorService = aliyun.NewVectorService(
				embeddingAPIURL,
				embeddingAPIKey,
				vectorDBURL,
				vectorDBAPIKey,
				vectorDBCollection,
				vectorDBDimension,
				vectorDBMetric,
				similarityThreshold,
			)
			log.Printf("✅ [用户存储仓库] 创建阿里云实例用于API兼容性")
		} else {
			log.Printf("ℹ️ [用户存储仓库] 使用 %s 类型，不创建向量服务", userRepositoryType)
		}
	}

	// 初始化用户存储仓库
	if err := userRepository.InitRepository(); err != nil {
		log.Printf("警告：初始化用户存储仓库失败: %v", err)
	} else {
		log.Println("用户存储仓库初始化成功")
	}

	// 🔥 【重要】修改LLMDrivenContextService以使用新的向量存储工厂
	log.Printf("🔧 [向量存储工厂] 更新LLMDrivenContextService以使用新的向量存储...")
	llmDrivenContextService.GetContextService().SetVectorStore(currentVectorStore)
	log.Printf("✅ [向量存储工厂] LLMDrivenContextService更新完成")

	// 🔥 重要：向量存储设置完成后，重新进行延迟赋值
	log.Printf("🔧 [延迟赋值] 重新设置MultiDimensionalRetriever的向量引擎...")
	llmDrivenContextService.ReinitializeVectorEngine()
	log.Printf("✅ [延迟赋值] 向量引擎重新设置完成")

	// 🔥 初始化安全服务
	log.Println("🔒 [安全服务] 初始化安全服务...")
	securityService, err := initSecurityService()
	if err != nil {
		log.Printf("⚠️ [安全服务] 初始化失败: %v，安全功能将不可用", err)
		securityService = nil
	} else {
		log.Println("✅ [安全服务] 安全服务初始化成功")
	}

	// 创建API处理器（使用兼容性VectorService）
	handler := api.NewHandler(llmDrivenContextService, compatibilityVectorService, userRepository, cfg, securityService)

	// 注册路由并启动服务器
	setupRoutesAndStartServer(router, handler, cfg)
}

// setupRoutesAndStartServer 注册路由并启动服务器
func setupRoutesAndStartServer(router *gin.Engine, handler *api.Handler, cfg *config.Config) {
	// 🔥 静态文件服务（公开访问）
	router.Static("/web", "./web")
	router.StaticFile("/", "./web/role_selection.html")
	router.Static("/temp", "./data/temp")
	log.Println("✅ 静态文件服务器注册成功")

	// 🔥 健康检查端点（公开访问）
	router.GET("/health", handler.HandleHealth)
	log.Println("✅ 健康检查端点注册成功: /health")

	// 🔥 公开路由组（无需认证）
	public := router.Group("/api")
	{
		// 认证端点
		public.POST("/auth/login", api.LoginHandler)
		public.POST("/role/login", api.RoleLoginHandler)
	}
	log.Println("✅ 公开路由注册成功: /api/auth/login, /api/role/login")

	// 🔥 从环境变量读取速率限制配置
	rateLimitPerMin := getIntEnv("RATE_LIMIT_PER_MIN", 60)
	rateLimitBurst := getIntEnv("RATE_LIMIT_BURST", 100)
	log.Printf("✅ 速率限制配置: %d请求/分钟, 突发%d", rateLimitPerMin, rateLimitBurst)

	// 🔥 受保护路由组（需要JWT认证 + 速率限制）
	protected := router.Group("/api")
	protected.Use(middleware.JWTAuth())
	protected.Use(middleware.RateLimitMiddleware(rateLimitPerMin, rateLimitBurst))
	{
		// Chat路由
		chatGroup := protected.Group("/chat")
		handler.RegisterChatRoutes(chatGroup)

		// 文件路由
		filesGroup := protected.Group("/files")
		handler.RegisterFileRoutes(filesGroup)

		// 健康记录路由
		handler.RegisterHealthRoutes(protected)

		// 护理记录采用独立的轻量写入契约，避免把保存操作误当作聊天检索。
		nursingRecords := protected.Group("/v1/nursing")
		nursingRecords.POST("/records", handler.HandleNursingRecord)

		// Competition Agent endpoint: JWT-protected and limited to the explicit
		// read-only tool allowlist. It cannot persist or mutate nursing data.
		controlledAgent := protected.Group("/v1/agent")
		controlledAgent.POST("/execute", handler.HandleControlledAgent)

		// 安全检测路由
		securityGroup := protected.Group("/security")
		{
			securityGroup.POST("/scan", handler.HandleSecurityScan)
			securityGroup.POST("/detect", handler.HandleSecurityDetect)
			securityGroup.POST("/redact", handler.HandleSecurityRedact)
			securityGroup.GET("/stats", handler.HandleSecurityStats)

			// A versioned, JWT-protected evaluation endpoint. It executes only the
			// pre-generation security pipeline and is reserved for reproducible tests.
			securityEvaluation := protected.Group("/v1/security")
			securityEvaluation.POST("/evaluate-input", handler.HandleSecurityInputEvaluation)
			securityEvaluation.POST("/ablation", handler.HandleSecurityAblationEvaluation)

			experimentEvidence := protected.Group("/v1/experiments")
			experimentEvidence.POST("/threeway/evidence", handler.HandleStructuredThreeWayEvidence)
		}

		// Session管理路由
		protected.GET("/sessions", handler.HandleGetSessionsList)
		protected.GET("/users/:userId/sessions", handler.HandleGetUserSessionDetail)

		// 角色特定路由（仪表盘、告警、推荐等）
		protected.GET("/dashboard", api.GetDashboardHandler)
		protected.GET("/alerts", api.GetAlertsHandler)
		protected.GET("/recommendations", api.GetRecommendationsHandler)
		protected.POST("/call-caregiver", api.CallCaregiverHandler)
		protected.POST("/family-message", api.FamilyMessageHandler)
	}

	log.Println("✅ 受保护路由注册成功: JWT认证 + 速率限制已应用")
	log.Printf("   - Chat路由: /api/chat/*")
	log.Printf("   - 文件路由: /api/files/*")
	log.Printf("   - 安全检测路由: /api/security/*")
	log.Printf("   - Session管理路由: /api/sessions, /api/users/:userId/sessions")
	log.Printf("   - 角色路由: /api/dashboard, /api/alerts, /api/recommendations")

	log.Println("[Routes] Registering causal reasoning and unlearning routes")
	if err := registerAdvancedRoutes(protected, handler, cfg); err != nil {
		log.Printf("[Routes] Advanced routes unavailable: %v", err)
	} else {
		log.Println("[Routes] Advanced routes registered under /api/v1")
	}

	// 🔥 WebSocket和管理路由（保持原有逻辑，无认证）
	handler.RegisterWebSocketRoutes(router)
	handler.RegisterManagementRoutes(router)

	// 🔥 MCP工具路由（需要JWT认证）- /api/mcp路径
	mcpGroup := protected.Group("/mcp")
	{
		mcpGroup.POST("/tools/retrieve_context", handler.HandleMCPRetrieveContext)
		mcpGroup.POST("/tools/programming_context", handler.HandleMCPProgrammingContext)
		mcpGroup.POST("/tools/associate_file", handler.HandleMCPAssociateFile)
		mcpGroup.POST("/tools/record_edit", handler.HandleMCPRecordEdit)
		mcpGroup.POST("/tools/local_operation_callback", handler.HandleLocalOperationCallback)
		mcpGroup.POST("/tools/list", handler.HandleMCPToolsList)
		mcpGroup.POST("/tools/call", handler.HandleMCPToolCall)
	}
	log.Println("✅ MCP工具路由注册成功（/api/mcp路径）: JWT认证已应用")

	// 🔥 根级别MCP工具路由（需要JWT认证）- /mcp路径（实验脚本使用）
	rootMcpGroup := router.Group("/mcp")
	rootMcpGroup.Use(middleware.JWTAuth())
	rootMcpGroup.Use(middleware.RateLimitMiddleware(rateLimitPerMin, rateLimitBurst))
	{
		rootMcpGroup.POST("/tools/create_context", handler.HandleStoreContext)
		rootMcpGroup.POST("/tools/read_context", handler.HandleRetrieveContext)
		rootMcpGroup.POST("/tools/retrieve_context", handler.HandleMCPRetrieveContext)
	}
	log.Println("✅ 根级别MCP工具路由注册成功（/mcp路径）: JWT认证已应用")

	// 🔥 批量embedding路由
	if handler.GetBatchEmbeddingHandler() != nil {
		handler.GetBatchEmbeddingHandler().RegisterBatchEmbeddingRoutes(router)
		log.Println("✅ 批量Embedding路由注册成功")
	} else {
		log.Println("⚠️ 批量Embedding服务未初始化，跳过路由注册")
	}

	// 创建并注册Streamable HTTP处理器 - 这是HTTP模式的主要协议
	streamableHandler := api.NewStreamableHTTPHandler(handler)
	streamableHandler.RegisterStreamableHTTPRoutes(router)

	// 获取端口配置 - 优先使用HTTP_SERVER_PORT，兼容PORT
	port := getEnv("HTTP_SERVER_PORT", getEnv("PORT", cfg.HTTPServerPort))
	host := getEnv("HOST", cfg.Host)
	addr := fmt.Sprintf("%s:%s", host, port)

	// 创建HTTP服务器
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  2 * time.Minute, // 增加到2分钟，支持长时间LLM调用
		WriteTimeout: 2 * time.Minute, // 增加到2分钟，支持长时间响应
		IdleTimeout:  5 * time.Minute, // 增加空闲超时
	}

	// 优雅关闭处理
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		log.Println("正在关闭服务器...")

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("服务器关闭时出错: %v", err)
		}
		log.Println("服务器已关闭")
	}()

	// 启动HTTP服务器
	log.Printf("Context-Keeper Streamable HTTP MCP 服务器启动在 %s", addr)
	log.Printf("服务信息: http://%s/", addr)
	log.Printf("健康检查: http://%s/health", addr)
	log.Printf("MCP协议端点: http://%s/mcp", addr)
	log.Printf("能力查询端点: http://%s/mcp/capabilities", addr)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP服务器启动失败: %v", err)
	}
}

// initSecurityService 初始化安全服务
func initSecurityService() (*security.SecurityService, error) {
	// 配置文件路径
	configPath := getEnv("SECURITY_CONFIG_PATH", "./config/security_policy.yaml")
	auditLogPath := getEnv("SECURITY_AUDIT_LOG_PATH", "./data/security_audit.log")

	// 创建安全服务
	securityService, err := security.NewSecurityService(configPath, auditLogPath)
	if err != nil {
		return nil, fmt.Errorf("创建安全服务失败: %w", err)
	}

	log.Printf("✅ [安全服务] 安全服务初始化成功")
	log.Printf("   - 配置文件: %s", configPath)
	log.Printf("   - 审计日志: %s", auditLogPath)

	return securityService, nil
}

// registerAdvancedRoutes 注册高级路由（因果推理、机器遗忘）
func registerAdvancedRoutes(router *gin.RouterGroup, handler *api.Handler, cfg *config.Config) error {
	// 创建v1路由组
	v1 := router.Group("/v1")

	// 1. 初始化因果推理Handler所需依赖
	log.Println("🔧 [因果推理] 初始化因果推理Handler依赖...")

	// 获取LLM客户端
	llmClient, err := initOllamaClient(cfg)
	if err != nil {
		log.Printf("⚠️ [因果推理] Ollama客户端初始化失败: %v", err)
		return fmt.Errorf("Ollama客户端初始化失败: %w", err)
	}
	log.Println("✅ [因果推理] Ollama客户端初始化成功")

	// 获取Neo4j引擎
	kgEngine, err := initNeo4jEngine(cfg)
	if err != nil {
		// PCCM extraction is analysis-only and must remain available even when
		// the optional graph store is unavailable. Persistence is disabled by
		// the handler, while graph-dependent inference remains unavailable.
		log.Printf("⚠️ [因果推理] Neo4j引擎不可用，启用仅分析模式: %v", err)
		kgEngine = nil
	} else {
		log.Println("✅ [因果推理] Neo4j引擎初始化成功")
	}

	// 创建因果推理Handler
	ollamaClient, ok := llmClient.(*llm.OllamaLocalClient)
	if !ok {
		return fmt.Errorf("LLM客户端类型不匹配，需要OllamaLocalClient")
	}
	causalHandler := api.NewCausalReasoningHandler(ollamaClient, kgEngine, handler.GetSecurityService())
	api.RegisterCausalReasoningRoutes(v1, causalHandler)
	log.Println("✅ [因果推理] 路由注册成功")

	// 2. 初始化机器遗忘Handler所需依赖
	log.Println("🔧 [机器遗忘] 初始化机器遗忘Handler依赖...")

	// 获取向量存储
	vectorStore, err := initVectorStore(cfg)
	if err != nil {
		log.Printf("⚠️ [机器遗忘] 向量存储初始化失败: %v", err)
		return fmt.Errorf("向量存储初始化失败: %w", err)
	}
	log.Println("✅ [机器遗忘] 向量存储初始化成功")

	// 创建机器遗忘服务配置
	unlearningConfig := &models.UnlearningConfig{
		Enabled:                     true,
		DefaultEpsilon:              1.0,
		DefaultLearningRate:         0.001,
		DefaultMaxIterations:        50,
		DefaultConvergenceThreshold: 1e-6,
		DefaultRetainedSampleRatio:  100,
		MaxPrivacyBudgetPerUser:     10.0,
	}

	// 创建差分隐私服务
	dpService := security.NewDifferentialPrivacy(1.0, 1e-5)

	unlearningService := services.NewMachineUnlearningService(vectorStore, dpService, unlearningConfig)
	log.Println("✅ [机器遗忘] 机器遗忘服务创建成功")

	// 创建机器遗忘Handler
	unlearningHandler := api.NewUnlearningHandler(unlearningService, true)
	api.RegisterUnlearningRoutes(v1, unlearningHandler)
	log.Println("✅ [机器遗忘] 路由注册成功")

	return nil
}

// initOllamaClient 初始化Ollama LLM客户端
func initOllamaClient(cfg *config.Config) (llm.LLMClient, error) {
	ollamaHost := getEnv("OLLAMA_HOST", "http://localhost:11434")
	ollamaModel := getEnv("OLLAMA_MODEL", "qwen2.5:3b") // 默认值与 docker-compose.yml 一致

	llmConfig := &llm.LLMConfig{
		Provider: "ollama_local",
		Model:    ollamaModel,
		BaseURL:  ollamaHost,
		Timeout:  30 * time.Second,
		Extra: map[string]interface{}{
			"max_tokens":  2000,
			"temperature": 0.7,
		},
	}

	client, err := llm.NewOllamaLocalClient(llmConfig)
	if err != nil {
		return nil, fmt.Errorf("Ollama客户端创建失败: %w", err)
	}

	log.Printf("✅ Ollama客户端配置: Host=%s, Model=%s", ollamaHost, ollamaModel)
	return client, nil
}

// initNeo4jEngine 初始化Neo4j知识图谱引擎
func initNeo4jEngine(cfg *config.Config) (*knowledge.Neo4jEngine, error) {
	knowledgeConfig := neo4jEngineConfigFromEnv()

	engine, err := knowledge.NewNeo4jEngine(knowledgeConfig)
	if err != nil {
		return nil, fmt.Errorf("Neo4j引擎创建失败: %w", err)
	}

	log.Printf("✅ Neo4j引擎配置: URI=%s, Username=%s", knowledgeConfig.URI, knowledgeConfig.Username)
	return engine, nil
}

func neo4jEngineConfigFromEnv() *knowledge.Neo4jConfig {
	return &knowledge.Neo4jConfig{
		URI:                     getEnv("NEO4J_URI", "bolt://localhost:7687"),
		Username:                getEnv("NEO4J_USERNAME", "neo4j"),
		Password:                getEnv("NEO4J_PASSWORD", "neo4j_password"),
		Database:                getEnv("NEO4J_DATABASE", "neo4j"),
		MaxConnectionPoolSize:   getIntEnv("NEO4J_MAX_CONNECTION_POOL_SIZE", 50),
		ConnectionTimeout:       getDurationEnv("NEO4J_CONNECTION_TIMEOUT", 30*time.Second),
		MaxTransactionRetryTime: getDurationEnv("NEO4J_MAX_TRANSACTION_RETRY_TIME", 15*time.Second),
	}
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	value := getEnv(key, "")
	if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
		return parsed
	}
	return defaultValue
}

// initVectorStore 初始化向量存储
func initVectorStore(cfg *config.Config) (models.VectorStore, error) {
	vectorStoreType := getEnv("VECTOR_STORE_TYPE", cfg.VectorStoreType)

	log.Printf("🔍 [向量存储] 检测到向量存储类型: %s", vectorStoreType)

	// 使用向量存储工厂
	factory, err := vectorstore.InitializeFactoryFromEnv()
	if err != nil {
		return nil, fmt.Errorf("向量存储工厂初始化失败: %w", err)
	}

	currentVectorStore, err := factory.GetCurrentVectorStore()
	if err != nil {
		return nil, fmt.Errorf("获取当前向量存储失败: %w", err)
	}

	if currentVectorStore == nil {
		return nil, fmt.Errorf("向量存储不可用")
	}

	// 确保集合存在
	collectionName := getEnv("VECTOR_DB_COLLECTION", cfg.VectorDBCollection)
	if err := currentVectorStore.EnsureCollection(collectionName); err != nil {
		log.Printf("⚠️ [向量存储] 集合初始化警告: %v", err)
	}

	log.Printf("✅ 向量存储配置: Type=%s, Collection=%s", vectorStoreType, collectionName)
	return currentVectorStore, nil
}
