"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || function (mod) {
    if (mod && mod.__esModule) return mod;
    var result = {};
    if (mod != null) for (var k in mod) if (k !== "default" && Object.prototype.hasOwnProperty.call(mod, k)) __createBinding(result, mod, k);
    __setModuleDefault(result, mod);
    return result;
};
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.ContextKeeperExtension = void 0;
const vscode = __importStar(require("vscode"));
const path = __importStar(require("path"));
const fs = __importStar(require("fs"));
const os = __importStar(require("os"));
const ws_1 = __importDefault(require("ws"));
class ContextKeeperExtension {
    constructor(context) {
        this.connectionRetryCount = 0;
        this.context = context;
        // 创建输出通道
        this.outputChannel = vscode.window.createOutputChannel('Context Keeper');
        // 创建状态栏项
        this.statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
        this.statusBarItem.text = "$(brain) Context Keeper";
        this.statusBarItem.command = 'contextKeeper.showDashboard';
        this.statusBarItem.show();
        // 加载配置
        this.loadConfig();
        this.log('Context-Keeper 扩展已初始化');
    }
    // 激活扩展
    async activate() {
        try {
            // 注册命令
            this.registerCommands();
            // 注册事件监听器
            this.registerEventListeners();
            // 如果配置了自动连接，则连接到服务器
            if (this.config.autoConnect) {
                await this.connectToServer();
            }
            this.log('Context-Keeper 扩展已激活');
            this.updateStatusBar('🔗 准备连接', '点击连接到服务器');
        }
        catch (error) {
            this.logError('扩展激活失败', error);
            this.updateStatusBar('❌ 激活失败', '扩展激活过程中出现错误');
        }
    }
    // 注册VSCode命令
    registerCommands() {
        const commands = [
            vscode.commands.registerCommand('contextKeeper.showDashboard', () => this.showDashboard()),
            vscode.commands.registerCommand('contextKeeper.connectToServer', () => this.connectToServer()),
            vscode.commands.registerCommand('contextKeeper.disconnectFromServer', () => this.disconnectFromServer()),
            vscode.commands.registerCommand('contextKeeper.showSettings', () => this.showSettings()),
            vscode.commands.registerCommand('contextKeeper.testConnection', () => this.testConnection()),
            vscode.commands.registerCommand('contextKeeper.showLogs', () => this.showLogs()),
        ];
        commands.forEach(disposable => this.context.subscriptions.push(disposable));
    }
    // 注册文件系统事件监听器
    registerEventListeners() {
        // 文件保存事件
        const saveListener = vscode.workspace.onDidSaveTextDocument(async (document) => {
            if (this.isConnected()) {
                await this.handleFileSave(document);
            }
        });
        // 工作区变更事件
        const workspaceListener = vscode.workspace.onDidChangeWorkspaceFolders(async (event) => {
            if (this.isConnected()) {
                await this.handleWorkspaceChange(event);
            }
        });
        this.context.subscriptions.push(saveListener, workspaceListener);
    }
    // WebSocket连接管理
    async connectToServer() {
        try {
            if (this.websocket && this.websocket.readyState === ws_1.default.OPEN) {
                this.log('已经连接到服务器');
                return;
            }
            this.updateStatusBar('🔄 连接中...', '正在连接到Context-Keeper服务器');
            // 获取用户ID
            await this.ensureUserID();
            // 建立WebSocket连接
            // 🔧 修复：正确处理https到wss的转换
            const wsUrl = `${this.config.serverURL.replace(/^https?/, this.config.serverURL.startsWith('https') ? 'wss' : 'ws')}/ws?userId=${this.userID}`;
            this.websocket = new ws_1.default(wsUrl);
            // 设置事件处理器
            this.setupWebSocketHandlers();
            // 等待连接建立
            await this.waitForConnection();
            this.log(`已连接到服务器: ${this.config.serverURL}`);
            this.updateStatusBar('✅ 已连接', `用户: ${this.userID}`);
            this.connectionRetryCount = 0;
        }
        catch (error) {
            this.logError('连接服务器失败', error);
            this.updateStatusBar('❌ 连接失败', '无法连接到服务器');
            // 启动重连机制
            this.scheduleReconnect();
        }
    }
    // 设置WebSocket事件处理器
    setupWebSocketHandlers() {
        if (!this.websocket)
            return;
        this.websocket.on('open', () => {
            this.log('WebSocket连接已建立');
        });
        this.websocket.on('message', async (data) => {
            try {
                const instruction = JSON.parse(data.toString());
                await this.executeLocalInstruction(instruction);
            }
            catch (error) {
                this.logError('处理WebSocket消息失败', error);
            }
        });
        this.websocket.on('close', (code, reason) => {
            this.log(`WebSocket连接已关闭: ${code} - ${reason}`);
            this.updateStatusBar('⚠️ 连接断开', '与服务器的连接已断开');
            this.scheduleReconnect();
        });
        this.websocket.on('error', (error) => {
            this.logError('WebSocket连接错误', error);
        });
    }
    // 本地指令执行引擎
    async executeLocalInstruction(instruction) {
        this.log(`执行本地指令: ${instruction.type} -> ${instruction.target}`);
        let result;
        try {
            switch (instruction.type) {
                case 'short_memory':
                    result = await this.executeShortMemory(instruction);
                    break;
                case 'session_store':
                    result = await this.executeSessionStore(instruction);
                    break;
                case 'user_config':
                    result = await this.executeUserConfig(instruction);
                    break;
                case 'preferences':
                    result = await this.executePreferences(instruction);
                    break;
                default:
                    throw new Error(`未知的指令类型: ${instruction.type}`);
            }
        }
        catch (error) {
            result = {
                success: false,
                message: `执行指令失败: ${error.message}`,
                timestamp: new Date().toISOString()
            };
        }
        // 发送回调结果
        await this.sendCallback(instruction.callbackId, result);
        // 更新状态栏
        if (result.success) {
            this.updateStatusBar('✅ 已连接', `最后操作: ${instruction.type}`);
        }
        else {
            this.updateStatusBar('⚠️ 操作失败', result.message);
        }
    }
    // 执行短期记忆存储
    async executeShortMemory(instruction) {
        const targetPath = this.expandPath(instruction.target);
        // 确保目录存在
        if (instruction.options?.createDir) {
            await this.ensureDirectory(path.dirname(targetPath));
        }
        // 处理合并选项
        let finalContent = instruction.content;
        if (instruction.options?.merge && await this.fileExists(targetPath)) {
            const existingContent = await this.readJSONFile(targetPath);
            if (Array.isArray(existingContent) && Array.isArray(instruction.content)) {
                finalContent = [...existingContent, ...instruction.content];
            }
        }
        // 写入文件
        await this.writeJSONFile(targetPath, finalContent);
        // 清理旧数据
        if (instruction.options?.cleanupOld && instruction.options?.maxAge) {
            await this.cleanupOldFiles(path.dirname(targetPath), instruction.options.maxAge);
        }
        return {
            success: true,
            message: '短期记忆存储成功',
            data: { filePath: targetPath, size: JSON.stringify(finalContent).length },
            timestamp: new Date().toISOString()
        };
    }
    // 执行会话存储
    async executeSessionStore(instruction) {
        const targetPath = this.expandPath(instruction.target);
        await this.ensureDirectory(path.dirname(targetPath));
        await this.writeJSONFile(targetPath, instruction.content);
        return {
            success: true,
            message: '会话数据存储成功',
            data: { filePath: targetPath },
            timestamp: new Date().toISOString()
        };
    }
    // 执行用户配置更新
    async executeUserConfig(instruction) {
        const targetPath = this.expandPath(instruction.target);
        await this.ensureDirectory(path.dirname(targetPath));
        await this.writeJSONFile(targetPath, instruction.content);
        return {
            success: true,
            message: '用户配置更新成功',
            data: { filePath: targetPath },
            timestamp: new Date().toISOString()
        };
    }
    // 执行偏好设置
    async executePreferences(instruction) {
        const targetPath = this.expandPath(instruction.target);
        await this.ensureDirectory(path.dirname(targetPath));
        // 处理合并选项
        let finalContent = instruction.content;
        if (instruction.options?.merge && await this.fileExists(targetPath)) {
            const existingData = await this.readJSONFile(targetPath);
            finalContent = { ...existingData, ...instruction.content };
        }
        await this.writeJSONFile(targetPath, finalContent);
        return {
            success: true,
            message: '偏好设置更新成功',
            data: { filePath: targetPath },
            timestamp: new Date().toISOString()
        };
    }
    // 发送回调结果
    async sendCallback(callbackId, result) {
        if (!this.websocket || this.websocket.readyState !== ws_1.default.OPEN) {
            this.logError('无法发送回调', new Error('WebSocket连接未建立'));
            return;
        }
        const message = {
            type: 'callback',
            callbackId,
            ...result
        };
        this.websocket.send(JSON.stringify(message));
        this.log(`回调已发送: ${callbackId} - ${result.success ? '成功' : '失败'}`);
    }
    // 显示仪表板
    async showDashboard() {
        const panel = vscode.window.createWebviewPanel('contextKeeperDashboard', 'Context Keeper Dashboard', vscode.ViewColumn.One, {
            enableScripts: true,
            retainContextWhenHidden: true
        });
        panel.webview.html = this.getDashboardHTML();
        // 处理来自WebView的消息
        panel.webview.onDidReceiveMessage(async (message) => {
            switch (message.command) {
                case 'connect':
                    await this.connectToServer();
                    break;
                case 'disconnect':
                    await this.disconnectFromServer();
                    break;
                case 'testConnection':
                    await this.testConnection();
                    break;
                case 'showLogs':
                    this.showLogs();
                    break;
            }
        });
    }
    // 工具方法
    expandPath(filePath) {
        return filePath.replace(/^~/, os.homedir());
    }
    async fileExists(filePath) {
        try {
            await fs.promises.access(filePath);
            return true;
        }
        catch {
            return false;
        }
    }
    async ensureDirectory(dirPath) {
        await fs.promises.mkdir(dirPath, { recursive: true });
    }
    async readJSONFile(filePath) {
        const content = await fs.promises.readFile(filePath, 'utf8');
        return JSON.parse(content);
    }
    async writeJSONFile(filePath, data) {
        const content = JSON.stringify(data, null, 2);
        await fs.promises.writeFile(filePath, content, 'utf8');
    }
    async cleanupOldFiles(dirPath, maxAge) {
        try {
            const files = await fs.promises.readdir(dirPath);
            const now = Date.now();
            for (const file of files) {
                const filePath = path.join(dirPath, file);
                const stats = await fs.promises.stat(filePath);
                if (now - stats.mtime.getTime() > maxAge * 1000) {
                    await fs.promises.unlink(filePath);
                    this.log(`已清理旧文件: ${filePath}`);
                }
            }
        }
        catch (error) {
            this.logError('清理旧文件失败', error);
        }
    }
    isConnected() {
        return this.websocket?.readyState === ws_1.default.OPEN;
    }
    async ensureUserID() {
        if (!this.userID) {
            // 从配置中获取或生成新的用户ID
            this.userID = this.config.userId || this.generateUserID();
            // 保存到配置
            await this.saveConfig({ ...this.config, userId: this.userID });
        }
    }
    generateUserID() {
        return 'user_' + Math.random().toString(36).substr(2, 9);
    }
    loadConfig() {
        const config = vscode.workspace.getConfiguration('contextKeeper');
        this.config = {
            serverURL: config.get('serverURL', 'http://localhost:8088'),
            userId: config.get('userId'),
            autoConnect: config.get('autoConnect', true),
            retryConfig: config.get('retryConfig', {
                maxRetries: 3,
                retryDelay: 5000,
                backoffMultiplier: 2
            })
        };
    }
    async saveConfig(newConfig) {
        const config = vscode.workspace.getConfiguration('contextKeeper');
        await config.update('serverURL', newConfig.serverURL, vscode.ConfigurationTarget.Global);
        await config.update('userId', newConfig.userId, vscode.ConfigurationTarget.Global);
        await config.update('autoConnect', newConfig.autoConnect, vscode.ConfigurationTarget.Global);
        this.config = newConfig;
    }
    updateStatusBar(text, tooltip) {
        this.statusBarItem.text = `$(brain) ${text}`;
        if (tooltip) {
            this.statusBarItem.tooltip = tooltip;
        }
    }
    log(message) {
        const timestamp = new Date().toISOString();
        this.outputChannel.appendLine(`[${timestamp}] ${message}`);
    }
    logError(message, error) {
        const timestamp = new Date().toISOString();
        this.outputChannel.appendLine(`[${timestamp}] ERROR: ${message}`);
        if (error) {
            this.outputChannel.appendLine(`[${timestamp}] ${error.toString()}`);
        }
    }
    showLogs() {
        this.outputChannel.show();
    }
    async waitForConnection() {
        return new Promise((resolve, reject) => {
            if (!this.websocket) {
                reject(new Error('WebSocket未初始化'));
                return;
            }
            const timeout = setTimeout(() => {
                reject(new Error('连接超时'));
            }, 10000);
            this.websocket.once('open', () => {
                clearTimeout(timeout);
                resolve();
            });
            this.websocket.once('error', (error) => {
                clearTimeout(timeout);
                reject(error);
            });
        });
    }
    scheduleReconnect() {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
        }
        if (this.connectionRetryCount >= this.config.retryConfig.maxRetries) {
            this.log('达到最大重连次数，停止重连');
            this.updateStatusBar('❌ 连接失败', '达到最大重连次数');
            return;
        }
        const delay = this.config.retryConfig.retryDelay *
            Math.pow(this.config.retryConfig.backoffMultiplier, this.connectionRetryCount);
        this.log(`${delay}ms后尝试重连 (第${this.connectionRetryCount + 1}次)`);
        this.reconnectTimer = setTimeout(async () => {
            this.connectionRetryCount++;
            await this.connectToServer();
        }, delay);
    }
    async disconnectFromServer() {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = undefined;
        }
        if (this.websocket) {
            this.websocket.close();
            this.websocket = undefined;
        }
        this.updateStatusBar('⚠️ 已断开', '已主动断开与服务器的连接');
        this.log('已断开与服务器的连接');
    }
    async testConnection() {
        try {
            const response = await fetch(`${this.config.serverURL}/health`);
            if (response.ok) {
                vscode.window.showInformationMessage('服务器连接正常');
            }
            else {
                vscode.window.showErrorMessage('服务器响应异常');
            }
        }
        catch (error) {
            vscode.window.showErrorMessage(`连接测试失败: ${error.message}`);
        }
    }
    showSettings() {
        vscode.commands.executeCommand('workbench.action.openSettings', 'contextKeeper');
    }
    async handleFileSave(document) {
        // 这里可以添加文件保存时的自动化逻辑
        this.log(`文件已保存: ${document.fileName}`);
    }
    async handleWorkspaceChange(event) {
        // 这里可以添加工作区变更时的处理逻辑
        this.log(`工作区已变更: +${event.added.length} -${event.removed.length}`);
    }
    getDashboardHTML() {
        return `
        <!DOCTYPE html>
        <html>
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
            <title>Context Keeper Dashboard</title>
            <style>
                body { font-family: var(--vscode-font-family); margin: 20px; }
                .card { background: var(--vscode-editor-background); padding: 20px; margin: 10px 0; border-radius: 5px; }
                .button { background: var(--vscode-button-background); color: var(--vscode-button-foreground); border: none; padding: 10px 20px; margin: 5px; cursor: pointer; border-radius: 3px; }
                .button:hover { background: var(--vscode-button-hoverBackground); }
                .status { font-weight: bold; }
                .connected { color: var(--vscode-terminal-ansiGreen); }
                .disconnected { color: var(--vscode-terminal-ansiRed); }
            </style>
        </head>
        <body>
            <h1>🧠 Context Keeper Dashboard</h1>
            
            <div class="card">
                <h3>连接状态</h3>
                <p class="status" id="status">检查中...</p>
                <button class="button" onclick="connect()">连接</button>
                <button class="button" onclick="disconnect()">断开</button>
                <button class="button" onclick="testConnection()">测试连接</button>
            </div>
            
            <div class="card">
                <h3>操作</h3>
                <button class="button" onclick="showLogs()">查看日志</button>
                <button class="button" onclick="openSettings()">打开设置</button>
            </div>
            
            <script>
                const vscode = acquireVsCodeApi();
                
                function connect() {
                    vscode.postMessage({ command: 'connect' });
                }
                
                function disconnect() {
                    vscode.postMessage({ command: 'disconnect' });
                }
                
                function testConnection() {
                    vscode.postMessage({ command: 'testConnection' });
                }
                
                function showLogs() {
                    vscode.postMessage({ command: 'showLogs' });
                }
                
                function openSettings() {
                    vscode.postMessage({ command: 'openSettings' });
                }
            </script>
        </body>
        </html>
        `;
    }
    // 扩展停用时清理资源
    async deactivate() {
        await this.disconnectFromServer();
        this.statusBarItem.dispose();
        this.outputChannel.dispose();
        this.log('Context-Keeper 扩展已停用');
    }
}
exports.ContextKeeperExtension = ContextKeeperExtension;
//# sourceMappingURL=context-keeper-extension.js.map